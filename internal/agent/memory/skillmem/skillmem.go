package skillmem

import (
	"context"
	"sync"

	"JuanNiang-Neo/internal/core/dao"
)

// SkillMemory 全局技能记忆管理器。
// 缓存从 DB 读取的内容，Compact 时通过 LLM 更新后写回 DB。
// 线程安全：使用 RWMutex 保护缓存读写。
type SkillMemory struct {
	mu      sync.RWMutex
	content string
	dao     *dao.SkillMemoryDAO
}

// New 创建 SkillMemory 并从 DB 预热缓存。
func New(d *dao.SkillMemoryDAO) *SkillMemory {
	return &SkillMemory{dao: d}
}

// Warmup 从 DB 加载技能记忆到内存缓存。应在 Init 时调用。
func (s *SkillMemory) Warmup(ctx context.Context) error {
	mem, err := s.dao.GetOrCreate(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.content = mem.Content
	s.mu.Unlock()
	return nil
}

// Get 返回当前技能记忆内容（线程安全，读缓存）。
func (s *SkillMemory) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.content
}

// Update 更新技能记忆：原子写入 DB + 更新内存缓存。
// 使用 Postgres UPSERT 保证并发安全。
func (s *SkillMemory) Update(ctx context.Context, newContent string) error {
	if err := s.dao.Upsert(ctx, newContent); err != nil {
		return err
	}
	s.mu.Lock()
	s.content = newContent
	s.mu.Unlock()
	return nil
}
