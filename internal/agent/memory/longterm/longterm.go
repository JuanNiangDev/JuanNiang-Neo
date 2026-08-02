package longterm

import (
	"context"
	"sync"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
)

var log = logging.NewModule("longterm")

// Config 长期记忆配置。
type Config struct {
	HotAreaSize int
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
	return &LongTermMemory{
		conf:    conf,
		hotArea: make(map[string][]*models.LongTermMemoryItem),
		dao:     itemDAO,
	}
}

func (m *LongTermMemory) Add(ctx context.Context, areaID, content string) error {
	item := &models.LongTermMemoryItem{
		ChatAreaID: areaID,
		Content:    content,
	}
	if err := m.dao.Create(ctx, item); err != nil {
		return err
	}
	m.addToHot(areaID, item)
	return nil
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
