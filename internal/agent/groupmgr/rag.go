package groupmgr

import (
	"context"
	"fmt"
	"time"

	"JuanNiang-Neo/internal/core/ragtag"
	"JuanNiang-Neo/internal/metrics"

	caller "JuanNiang-Neo/infrastructure/rag/handler"

	"github.com/google/uuid"
)

// sampleSetTTL 样本候选集缓存有效期（样本变更即失效；TTL 兜底防长期不同步）。
const sampleSetTTL = 5 * time.Minute

// ragSearchPhraseK 群管理黑白语录检索候选数：命中后仍需过阈值，30 足够。
// embedding 区分度有限时小 k 会把本集合命中挤出 top-k（日志表现「外来 tag」）；
// 候选集过滤会丢弃外来 tag，调大只增加本集合命中概率，不引入误判。
const ragSearchPhraseK = 30

// ragSearchTimeout RAG 语义核实的硬超时：本地部署毫秒级，但 bge 模型首次推理需加载
// （冷启动可能 3-5s），给 5s 余量不让消息卡死，热路径稳定后毫秒级。
const ragSearchTimeout = 5 * time.Second

// phraseInfo 语录候选集条目（tag → 集合/ID/文本/类别，检索命中过滤用）。
// listType: black（黑名单，命中处罚）/ white（白名单，命中放行）。
type phraseInfo struct {
	listType string
	id       uint
	text     string
	category string
}

// phraseSet 黑白语录候选集（tag → 条目）。
type phraseSet struct {
	black map[uuid.UUID]phraseInfo
	white map[uuid.UUID]phraseInfo
}

// buildPhraseSet 构建语录候选集（本地 语录ID → 派生 tag），供检索命中过滤。
// 返回 nil 表示构建失败（调用方降级）。空集合不缓存：首次为空时缓存 5 分钟
// 会导致同步向量库后仍降级（空 map != nil 绕过 TTL 检查）。
func (m *Manager) buildPhraseSet(ctx context.Context) *phraseSet {
	m.sampleMu.Lock()
	defer m.sampleMu.Unlock()
	if m.sampleSet != nil && (len(m.sampleSet.black) > 0 || len(m.sampleSet.white) > 0) && time.Since(m.sampleSetAt) < sampleSetTTL {
		return m.sampleSet
	}
	samples, err := m.dao.SampleListAll(ctx)
	if err != nil {
		log.Warn("语录候选集构建失败，RAG 降级", "err", err)
		return nil
	}
	set := &phraseSet{black: make(map[uuid.UUID]phraseInfo), white: make(map[uuid.UUID]phraseInfo)}
	for _, s := range samples {
		info := phraseInfo{listType: s.ListType, id: s.ID, text: s.Text, category: s.Category}
		if s.ListType == "white" {
			set.white[ragtag.WhitePhrase(u32s(s.ID))] = info
		} else {
			set.black[ragtag.Sample(u32s(s.ID))] = info
		}
	}
	// 空集合不缓存：同步向量库后应立即重新构建
	if len(set.black) == 0 && len(set.white) == 0 {
		m.sampleSet = nil // 不缓存空集，下次调用重新查 DB
		return set
	}
	m.sampleSet = set
	m.sampleSetAt = time.Now()
	return set
}

// phraseMatch 一次 RAG 检索后黑白最佳命中。
type phraseMatch struct {
	listType string // black / white
	tag      uuid.UUID
	id       uint
	score    float64
	text     string
	category string
}

// ragVerdict RAG 语义匹配结果。
type ragVerdict struct {
	// ok 表示 RAG 路径可用且已完成检索；false = 不可用/无候选（调用方降级关键词）
	ok bool
	// black / white 为对应集合的最佳命中（未达阈值仍可能非 nil，由调用方按阈值判定）
	black *phraseMatch
	white *phraseMatch
}

// verifyByRAG RAG 语义匹配（第一核实人）：消息文本在黑/白语录集内各取最优命中。
// ok 只表示 RAG 服务可用且已完成检索（调用方据此决定是否走 RAG 路径）；
// black/white 可能均为 nil（服务正常但无语录命中 → 送 LLM 判定，而不是降级关键词）。
func (m *Manager) verifyByRAG(ctx context.Context, query string) ragVerdict {
	cli := m.getRAG()
	if cli == nil {
		return ragVerdict{} // RAG 未配置 → 不可用
	}
	owned := m.buildPhraseSet(ctx)
	if owned == nil {
		return ragVerdict{} // 候选集构建失败（DB 错误）→ 不可用
	}
	cctx, cancel := context.WithTimeout(ctx, ragSearchTimeout)
	defer cancel()
	start := time.Now()
	hits, err := cli.Search(cctx, query, ragSearchPhraseK, nil)
	metrics.RAGSearchLatency.Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.RAGSearchErrorsTotal.Inc()
		log.Warn("RAG 检索失败，降级", "err", err)
		return ragVerdict{} // 检索出错 → 不可用（降级关键词）
	}
	// RAG 服务可用（即使无命中）：ok=true，black/white 由命中决定
	v := ragVerdict{ok: true}
	for _, h := range hits {
		if info, ok := owned.black[h.Tag]; ok {
			if v.black == nil || h.Score > v.black.score {
				v.black = &phraseMatch{listType: "black", tag: h.Tag, id: info.id, score: h.Score, text: info.text, category: info.category}
			}
			continue
		}
		if info, ok := owned.white[h.Tag]; ok {
			if v.white == nil || h.Score > v.white.score {
				v.white = &phraseMatch{listType: "white", tag: h.Tag, id: info.id, score: h.Score, text: info.text, category: info.category}
			}
		}
	}
	// RAG 核实分数分布（调阈值依据）：黑白各报最优分，命中即观测
	// （重构后曾丢失该上报，导致 Grafana 分数分布面板无数据）
	if v.black != nil {
		metrics.GroupMgrRAGScore.Observe(v.black.score)
	}
	if v.white != nil {
		metrics.GroupMgrRAGScore.Observe(v.white.score)
	}
	return v
}

// syncRAG 全量同步词条 + 样本到 RAG（幂等 upsert，50 条/批）。
// 返回 (成功条数, 失败条数)。RAG 未配置返回明确错误（Web 面板展示）。
// RAGSynced 标记与实际同步状态对齐：仅成功写入 RAG 的词条置 true，
// 失败/未同步的词条置 false（面板「已同步」状态可信）。
func (m *Manager) syncRAG(ctx context.Context) (int, int, error) {
	return m.syncRAGProgress(ctx, nil)
}

// syncRAGProgress 带进度回调的全量同步：每批处理后回调 onProgress(done, failed)，
// 回调返回非 nil 错误则中止（如 SSE 客户端断开）。供 Web 流式同步进度展示。
func (m *Manager) syncRAGProgress(ctx context.Context, onProgress func(done, failed int) error) (int, int, error) {
	cli := m.getRAG()
	if cli == nil {
		return 0, 0, fmt.Errorf("RAG-Service 未配置或未启用")
	}
	words, err := m.dao.WordListAll(ctx)
	if err != nil {
		return 0, 0, err
	}
	samples, err := m.dao.SampleListAll(ctx)
	if err != nil {
		return 0, 0, err
	}

	// 词条并入样本路径：先确保 seed 样本行存在（幂等，text 唯一），向量以 Sample tag 写入。
	// 这样样本表/候选集与词库恒对齐，RAG 核实（候选集=样本表）能命中全部词条向量——
	// 此前词条用 Word tag 写入但检索侧只过滤 Sample tag，词条向量是死数据，
	// 样本表为空时链路测试报「RAG 不可用」且词条不参与语义核实。
	total, failed := 0, 0
	seed := make([]caller.BatchItem, 0, len(words)+len(samples))
	wordTagOf := make(map[uuid.UUID]uint, len(words))     // tag → 词条 ID（样本 tag 不在其中）
	sampleTagOf := make(map[uuid.UUID]uint, len(samples)) // tag → 语录 ID（词条派生样本也记录）
	for _, w := range words {
		sid, err := m.dao.SampleAddWithWord(ctx, w.Word, sampleCategoryByWord(w.Category), "seed", w.ID)
		if err != nil {
			failed++
			log.Warn("词条样本行创建失败，跳过同步", "word", w.Word, "err", err)
			continue
		}
		tag := ragtag.Sample(u32s(sid))
		seed = append(seed, caller.BatchItem{Tag: tag, Text: w.Word})
		wordTagOf[tag] = w.ID
		sampleTagOf[tag] = sid
	}
	for _, s := range samples {
		if s.WordID > 0 {
			continue // 词条派生样本已由词条循环处理（避免同 tag 重复 upsert）
		}
		// 语录 tag 按集合选择：白名单语录用 WhitePhrase 前缀（检索侧按前缀归类）
		tag := ragtag.Sample(u32s(s.ID))
		if s.ListType == "white" {
			tag = ragtag.WhitePhrase(u32s(s.ID))
		}
		seed = append(seed, caller.BatchItem{Tag: tag, Text: s.Text})
		sampleTagOf[tag] = s.ID
	}
	for i := 0; i < len(seed); i += 50 {
		end := i + 50
		if end > len(seed) {
			end = len(seed)
		}
		resp, err := cli.BatchUpsert(ctx, seed[i:end])
		if err != nil {
			failed += end - i
			// 整批失败：本批涉及语录/样本全部置未同步
			for _, item := range seed[i:end] {
				if err := m.dao.SampleMarkRAGSynced(ctx, sampleTagOf[item.Tag], false); err != nil {
					log.Warn("样本同步状态标记失败", "tag", item.Tag, "err", err)
				}
			}
			log.Warn("RAG 批量同步失败", "err", err)
			continue
		}
		for idx, r := range resp.Results {
			// 边界校验：结果超出本批请求范围时安全跳过（防越界 panic/串批标记）
			if i+idx >= end {
				log.Warn("RAG 批量同步返回超出批次范围的结果，跳过", "idx", idx, "batch_size", end-i)
				continue
			}
			if r.Error != nil {
				failed++
				// 单条失败：该样本置未同步
				_ = m.dao.SampleMarkRAGSynced(ctx, sampleTagOf[seed[i+idx].Tag], false)
				continue
			}
			total++
			// 样本级：成功写回向量 → 标记已同步（语录面板状态可信）
			if sid, ok := sampleTagOf[seed[i+idx].Tag]; ok {
				if err := m.dao.SampleMarkRAGSynced(ctx, sid, true); err != nil {
					log.Warn("样本同步状态标记失败", "sample", sid, "err", err)
				}
			}
			// 词条写入成功 → 从待标记集合移除（剩余词条保持/置为未同步）
			if id, ok := wordTagOf[seed[i+idx].Tag]; ok {
				if err := m.dao.WordMarkRAGSynced(ctx, id, true); err != nil {
					log.Warn("词条同步状态标记失败", "word_id", id, "err", err)
				}
				delete(wordTagOf, seed[i+idx].Tag)
			}
		}
		// 每批后推送进度（客户端断开即中止）
		if onProgress != nil {
			if err := onProgress(total, failed); err != nil {
				return total, failed, err
			}
		}
	}
	// 未成功写入的词条/语录（失败批次/失败条目）标记为未同步，面板状态与实际对齐
	for _, id := range wordTagOf {
		if err := m.dao.WordMarkRAGSynced(ctx, id, false); err != nil {
			log.Warn("词条同步状态标记失败", "word_id", id, "err", err)
		}
	}
	// 样本候选集失效重建（同步后立即可检索）
	m.invalidateSampleSet()
	return total, failed, nil
}

// upsertRAGPhrase 单条语录写入 RAG（学习闭环/导入用）。
// listType 决定 tag 前缀（black → ragtag.Sample；white → ragtag.WhitePhrase）。
// 返回 (bool, error)：bool=true 表示已真实写入向量库；RAG 未配置/不可用返回 (false, nil)。
func (m *Manager) upsertRAGPhrase(ctx context.Context, phraseID uint, text, listType string) (bool, error) {
	cli := m.getRAG()
	if cli == nil {
		return false, nil // 未配置：未写入
	}
	tag := ragtag.Sample(u32s(phraseID))
	if listType == "white" {
		tag = ragtag.WhitePhrase(u32s(phraseID))
	}
	if _, err := cli.Upsert(ctx, tag, text); err != nil {
		log.Warn("语录写入 RAG 失败", "phrase", phraseID, "list", listType, "err", err)
		return false, err
	}
	m.invalidateSampleSet()
	return true, nil
}

// upsertRAGSample 单条样本写入 RAG（词条派生样本/黑名单语录，兼容旧调用）。
// 返回 (bool, error)：bool=true 表示已真实写入向量库；RAG 未配置/不可用返回 (false, nil)。
// 调用方必须以 bool 判定同步成功——nil error 不代表已写入（契约不再自相矛盾）。
func (m *Manager) upsertRAGSample(ctx context.Context, sampleID uint, text string) (bool, error) {
	return m.upsertRAGPhrase(ctx, sampleID, text, "black")
}

// deleteRAGSample 删除样本向量（双删，RAG 不可用静默跳过）。
func (m *Manager) deleteRAGSample(ctx context.Context, sampleID uint) {
	cli := m.getRAG()
	if cli == nil {
		return
	}
	if err := cli.Delete(ctx, ragtag.Sample(u32s(sampleID))); err != nil {
		log.Warn("样本从 RAG 删除失败", "sample", sampleID, "err", err)
	}
	m.invalidateSampleSet()
}

// u32s uint → 十进制字符串（RAG tag 派生用）。
func u32s(v uint) string {
	return fmt.Sprintf("%d", v)
}
