package agent

import (
	"context"
	"sort"
	"time"

	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/core/ragtag"

	"github.com/google/uuid"
)

// ---------- RAG 候选集（本地条目 ID → v5 派生 tag 映射） ----------
//
// RAG-Service 的检索不区分集合（知识与记忆共用同一向量库），命中 tag 后必须
// 用本地"全部条目 ID → 派生 tag"的集合过滤出属于本集合的结果，再反查 DB 内容。
// 候选集是内存态（丢失可重建）：知识侧随知识变更全量失效；记忆侧短 TTL 兜底。

// memoryRagSetTTL 记忆候选集缓存有效期：记忆条目写入只在 Compact 时发生，
// 5 分钟 TTL 足够，避免每次消息都全量 SELECT id。
const memoryRagSetTTL = 5 * time.Minute

// knowledgeRagTagSet 返回知识候选集（懒构建 + 缓存，知识变更时由
// InvalidateKnowledgeLRU 同步失效；构建失败返回 false → 调用方降级）。
func (h *HagoCenter) knowledgeRagTagSet(ctx context.Context) (map[uuid.UUID]string, bool) {
	h.knowledgeRagSetMu.RLock()
	cached := h.knowledgeRagSet
	h.knowledgeRagSetMu.RUnlock()
	if cached != nil {
		return cached, true
	}
	ids, err := h.DAO.Knowledge.ListAllIDs(ctx)
	if err != nil {
		log.Warn("知识 RAG 候选集构建失败，降级 SQL 匹配", "err", err)
		return nil, false
	}
	set := make(map[uuid.UUID]string, len(ids))
	for _, id := range ids {
		set[ragtag.Knowledge(id)] = id
	}
	h.knowledgeRagSetMu.Lock()
	h.knowledgeRagSet = set
	h.knowledgeRagSetMu.Unlock()
	return set, true
}

// invalidateKnowledgeRagSet 知识候选集失效（知识增删改时调用）。
func (h *HagoCenter) invalidateKnowledgeRagSet() {
	h.knowledgeRagSetMu.Lock()
	h.knowledgeRagSet = nil
	h.knowledgeRagSetMu.Unlock()
}

// memoryRagTagSet 返回记忆候选集（TTL 缓存；构建失败返回 false → 调用方降级）。
func (h *HagoCenter) memoryRagTagSet(ctx context.Context) (map[uuid.UUID]string, bool) {
	h.memoryRagSetMu.RLock()
	cached, at := h.memoryRagSet, h.memoryRagSetAt
	h.memoryRagSetMu.RUnlock()
	if cached != nil && time.Since(at) < memoryRagSetTTL {
		return cached, true
	}
	ids, err := h.DAO.LongTermMemItem.ListAllIDs(ctx)
	if err != nil {
		log.Warn("记忆 RAG 候选集构建失败，降级", "err", err)
		return nil, false
	}
	set := make(map[uuid.UUID]string, len(ids))
	for _, id := range ids {
		set[ragtag.Memory(id)] = id
	}
	h.memoryRagSetMu.Lock()
	h.memoryRagSet = set
	h.memoryRagSetAt = time.Now()
	h.memoryRagSetMu.Unlock()
	return set, true
}

// ---------- 语义召回（RAG 首选） ----------

// ragHit 命中条目（tag → 本地 ID，按分数降序）。
type ragHit struct {
	score float64
	id    string
}

// filterRagHits 按候选集过滤 RAG 命中并按分数降序，返回本地条目 ID 列表。
func filterRagHits(hits []ragHitWithTag, owner map[uuid.UUID]string) []string {
	var matches []ragHit
	for _, hit := range hits {
		if id, ok := owner[hit.tag]; ok {
			matches = append(matches, ragHit{score: hit.score, id: id})
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	ids := make([]string, len(matches))
	for i, m := range matches {
		ids[i] = m.id
	}
	return ids
}

// ragHitWithTag RAG 检索命中的 tag + 分数。
type ragHitWithTag struct {
	tag   uuid.UUID
	score float64
}

// tryKnowledgeRAGRecall 知识库语义召回：命中按 RAG 分数排序注入。
// 返回 (条目列表, 是否走了 RAG 路径)。RAG 未配置/不可用/无候选 → false（调用方降级 SQL）。
func (h *HagoCenter) tryKnowledgeRAGRecall(ctx context.Context, query string) ([]models.KnowledgeItem, bool) {
	if h.RAGClient == nil {
		return nil, false
	}
	owned, ok := h.knowledgeRagTagSet(ctx)
	if !ok || len(owned) == 0 {
		return nil, false
	}
	searchHits, err := h.RAGClient.Search(ctx, query, 10, nil)
	if err != nil {
		log.Warn("知识 RAG 检索失败，降级 SQL 匹配", "err", err)
		return nil, false
	}
	if len(searchHits) == 0 {
		return nil, false
	}
	hits := make([]ragHitWithTag, 0, len(searchHits))
	for _, hit := range searchHits {
		hits = append(hits, ragHitWithTag{tag: hit.Tag, score: hit.Score})
	}
	ids := filterRagHits(hits, owned)
	if len(ids) == 0 {
		return nil, false
	}
	items, err := h.DAO.Knowledge.GetByIDs(ctx, ids)
	if err != nil {
		log.Warn("知识 RAG 命中反查失败，降级 SQL 匹配", "err", err)
		return nil, false
	}
	// 维持 RAG 分数顺序（GetByIDs 返回无序）
	byID := make(map[string]models.KnowledgeItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	ordered := make([]models.KnowledgeItem, 0, len(ids))
	for _, id := range ids {
		if it, ok := byID[id]; ok {
			ordered = append(ordered, it)
		}
	}
	if len(ordered) == 0 {
		return nil, false
	}
	return ordered, true
}

// tryMemoryRAGRecall 长期记忆语义召回：命中按 RAG 分数排序返回内容。
// 返回 (内容列表, 是否走了 RAG 路径)。降级链由调用方（memoryRecall）负责。
func (h *HagoCenter) tryMemoryRAGRecall(ctx context.Context, query string) ([]string, bool) {
	if h.RAGClient == nil {
		return nil, false
	}
	owned, ok := h.memoryRagTagSet(ctx)
	if !ok || len(owned) == 0 {
		return nil, false
	}
	searchHits, err := h.RAGClient.Search(ctx, query, 20, nil)
	if err != nil {
		log.Warn("记忆 RAG 检索失败，降级", "err", err)
		return nil, false
	}
	if len(searchHits) == 0 {
		return nil, false
	}
	hits := make([]ragHitWithTag, 0, len(searchHits))
	for _, hit := range searchHits {
		hits = append(hits, ragHitWithTag{tag: hit.Tag, score: hit.Score})
	}
	ids := filterRagHits(hits, owned)
	if len(ids) == 0 {
		return nil, false
	}
	items, err := h.DAO.LongTermMemItem.GetByIDs(ctx, ids)
	if err != nil {
		log.Warn("记忆 RAG 命中反查失败，降级", "err", err)
		return nil, false
	}
	byID := make(map[string]models.LongTermMemoryItem, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	content := make([]string, 0, len(ids))
	for _, id := range ids {
		if it, ok := byID[id]; ok {
			content = append(content, it.Content)
		}
	}
	if len(content) == 0 {
		return nil, false
	}
	return content, true
}
