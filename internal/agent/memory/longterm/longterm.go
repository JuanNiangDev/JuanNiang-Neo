package longterm

import (
	"context"
	"sync"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewModule("longterm")

// RecallMode 长期记忆对话召回模式。
type RecallMode string

const (
	// RecallModeRecent 按最近写入召回（旧行为）：HotArea 内存直返，DB 时间倒序兕底。
	RecallModeRecent RecallMode = "recent"
	// RecallModeSemantic 按消息语义召回（默认）：消息 gram 走 pg_trgm GIN 倒排候选，
	// 候选内 similarity 排序；空候选/异常自动回退最近。
	RecallModeSemantic RecallMode = "semantic"
)

// Config 长期记忆配置。
type Config struct {
	HotAreaSize int
	// RecallMode 对话召回模式（默认语义召回；可通过环境变量 LTM_RECALL_MODE=recent 关闭）
	RecallMode RecallMode
}

// LongTermMemory 长期记忆，Postgres 存储 + 内存 HotArea (LRU)。
// 按 ChatArea 隔离 HotArea，实例本身可跨 ChatArea 共享。
type LongTermMemory struct {
	conf    Config
	mu      sync.RWMutex
	hotArea map[string][]*models.LongTermMemoryItem // areaID -> hot items (最新在前)
	dao     *dao.LongTermMemoryItemDAO
}

func New(conf Config, itemDAO *dao.LongTermMemoryItemDAO) *LongTermMemory {
	if conf.HotAreaSize <= 0 {
		conf.HotAreaSize = 10
	}
	if conf.RecallMode == "" {
		conf.RecallMode = RecallModeSemantic // 默认语义召回
	}
	return &LongTermMemory{
		conf:    conf,
		hotArea: make(map[string][]*models.LongTermMemoryItem),
		dao:     itemDAO,
	}
}

// Add 写入一条长期记忆（Postgres + 内存热区），返回含 ID 的条目（供上层同步向量）。
func (m *LongTermMemory) Add(ctx context.Context, areaID, content string) (*models.LongTermMemoryItem, error) {
	item := &models.LongTermMemoryItem{
		ChatAreaID: areaID,
		Content:    content,
	}
	if err := m.dao.Create(ctx, item); err != nil {
		return nil, err
	}
	m.addToHot(areaID, item)
	return item, nil
}

// Search 查询长期记忆。优先从 HotArea 缓存读取，缓存不足时回退到 DB。
// query 非空时使用关键词搜索（ILIKE），否则按时间倒序返回最近条目。
func (m *LongTermMemory) Search(ctx context.Context, areaID, query string, limit int) ([]models.LongTermMemoryItem, error) {
	// 关键词搜索：直接查 DB（HotArea 不支持关键词过滤）
	if query != "" {
		return m.dao.SearchByContent(ctx, areaID, query, limit)
	}

	// 无关键词：优先读 HotArea
	hot := m.GetHot(areaID)
	if len(hot) >= limit {
		return hot[:limit], nil
	}

	// HotArea 不足，回退 DB 查询
	return m.dao.ListByChatArea(ctx, areaID, limit)
}

// Recall 对话语义召回（主链路）：
//  1. recent 模式 / gram 为空（短消息、纯表情）→ 直接回退最近条目（Search 空 query）
//  2. semantic 模式：gram OR 候选 + similarity 排序；
//     候选为空或检索异常 → 回退最近条目，保证召回质量不劣于旧行为
func (m *LongTermMemory) Recall(ctx context.Context, areaID string, grams []string, query string, limit int) ([]models.LongTermMemoryItem, error) {
	if m.conf.RecallMode == RecallModeRecent || len(grams) == 0 {
		return m.Search(ctx, areaID, "", limit)
	}

	items, err := m.dao.SemanticSearch(ctx, areaID, grams, query, limit)
	if err != nil {
		log.Warn("长期记忆语义召回失败，回退最近", "area_id", areaID, "err", err)
		return m.Search(ctx, areaID, "", limit)
	}
	if len(items) == 0 {
		// 新话题无字面重叠：回退最近，避免语义召回把记忆"清空"
		return m.Search(ctx, areaID, "", limit)
	}
	return items, nil
}

// GetHot 返回指定 ChatArea 的热区记忆条目（最新在前）。
func (m *LongTermMemory) GetHot(areaID string) []models.LongTermMemoryItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	items := m.hotArea[areaID]
	out := make([]models.LongTermMemoryItem, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out
}

func (m *LongTermMemory) UpdateConfig(conf Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conf = conf
}

// Warmup 预热指定 ChatArea 的热区。
func (m *LongTermMemory) Warmup(ctx context.Context, areaID string) error {
	items, err := m.dao.ListByChatArea(ctx, areaID, m.conf.HotAreaSize)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	hot := make([]*models.LongTermMemoryItem, 0, len(items))
	for i := range items {
		item := items[i]
		hot = append(hot, &item)
	}
	m.hotArea[areaID] = hot
	return nil
}

// addToHot 将新条目加入热区，LRU 策略：新的在前，超出容量直接丢弃最旧的。
func (m *LongTermMemory) addToHot(areaID string, item *models.LongTermMemoryItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := append([]*models.LongTermMemoryItem{item}, m.hotArea[areaID]...)
	if len(items) > m.conf.HotAreaSize {
		items = items[:m.conf.HotAreaSize]
	}
	m.hotArea[areaID] = items
}
