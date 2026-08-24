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

// sampleInfo 样本候选集条目（tag → 文本/类别，检索命中过滤 + 直罚类别判断）。
type sampleInfo struct {
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
		set[ragtag.Sample(u32s(s.ID))] = sampleInfo{text: s.Text, category: s.Category}
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
func (m *Manager) syncRAG(ctx context.Context) (int, int, error) {
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

	// 词条 → 样本（关键词导入/种子词条都以文本形式入库；幂等由样本表 text 唯一兜底）
	total, failed := 0, 0
	seed := make([]caller.BatchItem, 0, len(words)+len(samples))
	wordIDs := make([]uint, 0, len(words)) // 记录词条 ID，同步成功后标记 RAGSynced
	for _, w := range words {
		seed = append(seed, caller.BatchItem{Tag: ragtag.Word(u32s(w.ID)), Text: w.Word})
		wordIDs = append(wordIDs, w.ID)
	}
	for _, s := range samples {
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
		for _, r := range resp.Results {
			if r.Error != nil {
				failed++
			} else {
				total++
			}
		}
	}
	// 全量同步成功 → 所有词条标记已同步（面板展示 RAG 同步状态）
	for _, id := range wordIDs {
		if err := m.dao.WordMarkRAGSynced(ctx, id, true); err != nil {
			log.Warn("词条同步状态标记失败", "word_id", id, "err", err)
		}
	}
	// 样本候选集失效重建（同步后立即可检索）
	m.invalidateSampleSet()
	return total, failed, nil
}

// upsertRAGSample 单条样本写入 RAG（学习闭环/导入用，RAG 不可用静默跳过）。
func (m *Manager) upsertRAGSample(ctx context.Context, sampleID uint, text string) {
	cli := m.getRAG()
	if cli == nil {
		return
	}
	if _, err := cli.Upsert(ctx, ragtag.Sample(u32s(sampleID)), text); err != nil {
		log.Warn("样本写入 RAG 失败（不影响处罚）", "sample", sampleID, "err", err)
		return
	}
	m.invalidateSampleSet()
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
