package agent

import (
	"JuanNiang-Neo/internal/metrics"
	"context"
	"sync"
	"time"
)

// ConcurrencyManager 控制每个 ChatArea 同时运行的 Agent ReAct 循环数量。
// 使用 buffered channel 作为信号量。支持可选的全局上限（防止多群同时活跃时
// goroutine 数无限增长导致 OOM / LLM provider 限流）。
type ConcurrencyManager struct {
	mu       sync.RWMutex
	limits   map[string]int           // chatAreaID → max concurrent
	chans    map[string]chan struct{} // chatAreaID → semaphore
	default_ int
	global   chan struct{} // 全局信号量（nil 表示不限制全局并发）
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

// SetGlobalLimit 设置全局并发上限（0 或负数 = 不限制）。
// 仅应在启动阶段（事件流开始前）调用，运行中调整会导致正在等待的 goroutine
// 无法感知新信号量。
func (cm *ConcurrencyManager) SetGlobalLimit(limit int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.global != nil {
		return // 已设置，忽略重复调用
	}
	if limit > 0 {
		cm.global = make(chan struct{}, limit)
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
// 若配置了全局上限，先获取全局令牌，再获取 ChatArea 令牌；
// 后者失败（ctx 取消）时归还已获取的全局令牌。
func (cm *ConcurrencyManager) Acquire(ctx context.Context, chatAreaID string) error {
	start := time.Now()
	defer func() {
		metrics.ConcurrencyWaitDuration.Observe(time.Since(start).Seconds())
	}()
	cm.mu.RLock()
	global := cm.global
	cm.mu.RUnlock()
	if global != nil {
		select {
		case global <- struct{}{}:
			log.Debug("获取全局并发令牌", "available", len(global), "cap", cap(global))
		case <-ctx.Done():
			metrics.ConcurrencyWaitsTotal.WithLabelValues("timeout").Inc()
			return ctx.Err()
		}
		if err := cm.acquireArea(ctx, chatAreaID); err != nil {
			// 区域令牌获取失败，归还全局令牌
			select {
			case <-global:
			default:
			}
			metrics.ConcurrencyWaitsTotal.WithLabelValues("timeout").Inc()
			return err
		}
		metrics.ConcurrencyWaitsTotal.WithLabelValues("acquired").Inc()
		return nil
	}
	if err := cm.acquireArea(ctx, chatAreaID); err != nil {
		metrics.ConcurrencyWaitsTotal.WithLabelValues("timeout").Inc()
		return err
	}
	metrics.ConcurrencyWaitsTotal.WithLabelValues("acquired").Inc()
	return nil
}

// acquireArea 获取指定 ChatArea 的令牌。
func (cm *ConcurrencyManager) acquireArea(ctx context.Context, chatAreaID string) error {
	sem := cm.getOrCreateSem(chatAreaID)
	select {
	case sem <- struct{}{}:
		log.Debug("获取并发令牌", "area", chatAreaID, "available", len(sem), "cap", cap(sem))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release 释放执行令牌（先区域后全局）。
func (cm *ConcurrencyManager) Release(chatAreaID string) {
	cm.releaseArea(chatAreaID)
	cm.releaseGlobal()
}

// releaseArea 释放指定 ChatArea 的令牌。
func (cm *ConcurrencyManager) releaseArea(chatAreaID string) {
	sem := cm.getOrCreateSem(chatAreaID)
	select {
	case <-sem:
		log.Debug("释放并发令牌", "area", chatAreaID, "available", len(sem), "cap", cap(sem))
	default:
		// 防止重复 Release
	}
}

// releaseGlobal 释放全局令牌。
func (cm *ConcurrencyManager) releaseGlobal() {
	cm.mu.RLock()
	global := cm.global
	cm.mu.RUnlock()
	if global != nil {
		select {
		case <-global:
		default:
			// 防止重复 Release
		}
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

// GlobalActive 返回全局并发槽当前占用数（0 = 未设置全局上限）。
func (cm *ConcurrencyManager) GlobalActive() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.global == nil {
		return 0
	}
	return len(cm.global)
}
