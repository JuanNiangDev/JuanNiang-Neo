package groupmgr

import (
	"context"
	"fmt"
	"sort"
	"time"

	"JuanNiang-Neo/internal/core/ragtag"
	"JuanNiang-Neo/internal/metrics"

	caller "JuanNiang-Neo/infrastructure/rag/handler"

	"github.com/google/uuid"
)

// sampleSetTTL 样本候选集缓存有效期（样本变更即失效；TTL 兜底防长期不同步）。
const sampleSetTTL = 5 * time.Minute

// ragSearchTimeout RAG 语义核实的硬超时：本地部署毫秒级，最坏 1s 不让消息卡死。
const ragSearchTimeout = time.Second

// sampleInfo 样本候选集条目（tag → 文本/类别/ID，检索命中过滤 + 直罚类别判断 + 命中计数）。
type sampleInfo struct {
	id       uint
	text     string
	category string
}

// buildSampleSet 构建样本候选集（本地 样本ID → 派生 tag），供检索命中过滤。
// 返回 nil 表示构建失败（调用方降级）。
func (m *Manager) buildSampleSet(ctx context.Context) map[uuid.UUID]sampleInfo {
	m.sampleMu.Lock()
	defer m.sampleMu.Unlock()
	if m.sampleSet != nil && time.Since(m.sampleSetAt) < sampleSetTTL {
		return m.sampleSet
	}
	samples, err := m.dao.SampleListAll(ctx)
	if err != nil {
		log.Warn("样本候选集构建失败，RAG 降级", "err", err)
		return nil
	}
	set := make(map[uuid.UUID]sampleInfo, len(samples))
	for _, s := range samples {
		set[ragtag.Sample(u32s(s.ID))] = sampleInfo{id: s.ID, text: s.Text, category: s.Category}
	}
	m.sampleSet = set
	m.sampleSetAt = time.Now()
	return set
}

// ragVerdict RAG 语义核实结果。
type ragVerdict struct {
	// ok 表示 RAG 路径可用且已核实；false = 不可用/无候选（调用方降级）
	ok bool
	// score 最高分（0~1）
	score float64
	// text 最高分样本文本
	text string
	// category 最高分样本类别（ad / sensitive）
	category string
	// tag 最高分样本 tag
	tag uuid.UUID
}

// verifyByRAG RAG 语义核实（第一核实人）：消息文本在违规样本集内检索 Top10。
// RAG 未配置/超时/无候选/无样本 → ok=false（调用方降级关键词路径）。
func (m *Manager) verifyByRAG(ctx context.Context, query string) ragVerdict {
	cli := m.getRAG()
	if cli == nil {
		return ragVerdict{}
	}
	owned := m.buildSampleSet(ctx)
	if owned == nil || len(owned) == 0 {
		return ragVerdict{}
	}
	cctx, cancel := context.WithTimeout(ctx, ragSearchTimeout)
	defer cancel()
	start := time.Now()
	hits, err := cli.Search(cctx, query, 10, nil)
	metrics.RAGSearchLatency.Observe(time.Since(start).Seconds())
	if err != nil || len(hits) == 0 {
		if err != nil {
			metrics.RAGSearchErrorsTotal.Inc()
		}
		return ragVerdict{}
	}
	// 命中过滤 + 按分数降序取最优
	type scored struct {
		tag   uuid.UUID
		score float64
	}
	var list []scored
	for _, h := range hits {
		if _, ok := owned[h.Tag]; ok {
			list = append(list, scored{tag: h.Tag, score: h.Score})
		}
	}
	if len(list) == 0 {
		return ragVerdict{}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	best := list[0]
	info := owned[best.tag]
	metrics.GroupMgrRAGScore.Observe(best.score)
	return ragVerdict{ok: true, score: best.score, tag: best.tag, text: info.text, category: info.category}
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
	wordTagOf := make(map[uuid.UUID]uint, len(words)) // tag → 词条 ID（样本 tag 不在其中）
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
	}
	for _, s := range samples {
		if s.WordID > 0 {
			continue // 词条派生样本已由词条循环处理（避免同 tag 重复 upsert）
		}
		seed = append(seed, caller.BatchItem{Tag: ragtag.Sample(u32s(s.ID)), Text: s.Text})
	}
	for i := 0; i < len(seed); i += 50 {
		end := i + 50
		if end > len(seed) {
			end = len(seed)
		}
		resp, err := cli.BatchUpsert(ctx, seed[i:end])
		if err != nil {
			failed += end - i
			log.Warn("RAG 批量同步失败", "err", err)
			continue
		}
		for idx, r := range resp.Results {
			if r.Error != nil {
				failed++
				continue
			}
			total++
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
	// 未成功写入的词条（失败批次/失败条目）标记为未同步，面板状态与实际对齐
	for _, id := range wordTagOf {
		if err := m.dao.WordMarkRAGSynced(ctx, id, false); err != nil {
			log.Warn("词条同步状态标记失败", "word_id", id, "err", err)
		}
	}
	// 样本候选集失效重建（同步后立即可检索）
	m.invalidateSampleSet()
	return total, failed, nil
}

// upsertRAGSample 单条样本写入 RAG（学习闭环/导入用）。
// 返回 (bool, error)：bool=true 表示已真实写入向量库；RAG 未配置/不可用返回 (false, nil)。
// 调用方必须以 bool 判定同步成功——nil error 不代表已写入（契约不再自相矛盾）。
func (m *Manager) upsertRAGSample(ctx context.Context, sampleID uint, text string) (bool, error) {
	cli := m.getRAG()
	if cli == nil {
		return false, nil // 未配置：未写入
	}
	if _, err := cli.Upsert(ctx, ragtag.Sample(u32s(sampleID)), text); err != nil {
		log.Warn("样本写入 RAG 失败（不影响处罚）", "sample", sampleID, "err", err)
		return false, err
	}
	m.invalidateSampleSet()
	return true, nil
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

func u32s(v uint) string {
	return fmt.Sprintf("%d", v)
}
