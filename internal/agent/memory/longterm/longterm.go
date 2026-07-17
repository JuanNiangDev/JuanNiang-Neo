package longterm

import (
	"context"
	"sync"
	"time"

	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"
)

// Config 长期记忆配置。
type Config struct {
	HotAreaSize  int
	HotMemoryTTL time.Duration
}

// LongTermMemory 长期记忆，Postgres 存储 + 内存 HotArea (LRU)。
type LongTermMemory struct {
	conf     Config
	hotArea  map[string]*models.LongTermMemoryItem
	hotOrder []string
	mu       sync.RWMutex
	dao      *dao.LongTermMemoryItemDAO
	areaID   string
}

func New(conf Config, itemDAO *dao.LongTermMemoryItemDAO, areaID string) *LongTermMemory {
	if conf.HotAreaSize <= 0 {
		conf.HotAreaSize = 10
	}
	if conf.HotMemoryTTL <= 0 {
		conf.HotMemoryTTL = 24 * time.Hour
	}
	return &LongTermMemory{
		conf:    conf,
		hotArea: make(map[string]*models.LongTermMemoryItem),
		dao:     itemDAO,
		areaID:  areaID,
	}
}

func (m *LongTermMemory) Add(ctx context.Context, content string) error {
	item := &models.LongTermMemoryItem{
		ChatAreaID: m.areaID,
		Content:    content,
	}
	if err := m.dao.Create(ctx, item); err != nil {
		return err
	}
	m.addToHot(item)
	return nil
}

func (m *LongTermMemory) Search(ctx context.Context, query string, limit int) ([]models.LongTermMemoryItem, error) {
	if query == "" {
		return m.dao.ListByChatArea(ctx, m.areaID, limit)
	}
	return m.dao.SearchByContent(ctx, m.areaID, query, limit)
}

func (m *LongTermMemory) GetHot() []models.LongTermMemoryItem {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]models.LongTermMemoryItem, 0, len(m.hotOrder))
	for _, id := range m.hotOrder {
		if item, ok := m.hotArea[id]; ok {
			out = append(out, *item)
		}
	}
	return out
}

func (m *LongTermMemory) UpdateConfig(conf Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conf = conf
}

func (m *LongTermMemory) Warmup(ctx context.Context) error {
	items, err := m.dao.ListByChatArea(ctx, m.areaID, m.conf.HotAreaSize)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hotArea = make(map[string]*models.LongTermMemoryItem, m.conf.HotAreaSize)
	m.hotOrder = make([]string, 0, m.conf.HotAreaSize)
	for i := range items {
		item := items[i]
		m.hotArea[item.ID] = &item
		m.hotOrder = append(m.hotOrder, item.ID)
	}
	return nil
}

func (m *LongTermMemory) addToHot(item *models.LongTermMemoryItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hotArea[item.ID] = item
	m.hotOrder = append([]string{item.ID}, m.hotOrder...)
	if len(m.hotOrder) > m.conf.HotAreaSize {
		evictID := m.hotOrder[len(m.hotOrder)-1]
		m.hotOrder = m.hotOrder[:len(m.hotOrder)-1]
		delete(m.hotArea, evictID)
	}
}
