package agent

import (
	"context"
	"sync"
)

// ConcurrencyManager 控制每个 ChatArea 同时运行的 Agent ReAct 循环数量。
// 使用 buffered channel 作为信号量。
type ConcurrencyManager struct {
	mu       sync.RWMutex
	limits   map[string]int           // chatAreaID → max concurrent
	chans    map[string]chan struct{} // chatAreaID → semaphore
	default_ int
}

// NewConcurrencyManager 创建并发管理器，defaultLimit 为默认并发上限。
func NewConcurrencyManager(defaultLimit int) *ConcurrencyManager {
	if defaultLimit <= 0 {
		defaultLimit = 8
	}
	return &ConcurrencyManager{
		limits:   make(map[string]int),
		chans:    make(map[string]chan struct{}),
		default_: defaultLimit,
	}
}

// getOrCreateSem 获取或创建指定 ChatArea 的信号量。
func (cm *ConcurrencyManager) getOrCreateSem(chatAreaID string) chan struct{} {
	cm.mu.RLock()
	s, ok := cm.chans[chatAreaID]
	cm.mu.RUnlock()
	if ok {
		return s
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	// double-check
	if s, ok := cm.chans[chatAreaID]; ok {
		return s
	}
	limit := cm.default_
	if l, ok := cm.limits[chatAreaID]; ok {
		limit = l
	}
	s = make(chan struct{}, limit)
	cm.chans[chatAreaID] = s
	return s
}

// Acquire 获取执行令牌。阻塞直到有空位或 ctx 取消。
func (cm *ConcurrencyManager) Acquire(ctx context.Context, chatAreaID string) error {
	sem := cm.getOrCreateSem(chatAreaID)
	select {
	case sem <- struct{}{}:
		log.Debug("获取并发令牌", "area", chatAreaID, "available", len(sem), "cap", cap(sem))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 释放执行令牌。
func (cm *ConcurrencyManager) Release(chatAreaID string) {
	sem := cm.getOrCreateSem(chatAreaID)
	select {
	case <-sem:
		log.Debug("释放并发令牌", "area", chatAreaID, "available", len(sem), "cap", cap(sem))
	default:
		// 防止重复 Release
	}
}

// SetLimit 设置某个 ChatArea 的并发上限。如果当前有更大的信号量，会重建。
// 注意：重建信号量时，正在旧信号量上阻塞等待的 goroutine 不会被唤醒，
// 只能通过 ctx 取消退出。建议在低峰期调整并发限制。
func (cm *ConcurrencyManager) SetLimit(chatAreaID string, limit int) {
	if limit <= 0 {
		limit = cm.default_
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.limits[chatAreaID] = limit
	// 重建信号量
	cm.chans[chatAreaID] = make(chan struct{}, limit)
}

// GetLimit 获取某个 ChatArea 的并发上限。
func (cm *ConcurrencyManager) GetLimit(chatAreaID string) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if l, ok := cm.limits[chatAreaID]; ok {
		return l
	}
	return cm.default_
}

// ActiveCount 返回某个 ChatArea 当前正在执行的 Agent 数量。
func (cm *ConcurrencyManager) ActiveCount(chatAreaID string) int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if s, ok := cm.chans[chatAreaID]; ok {
		return len(s)
	}
	return 0
}
