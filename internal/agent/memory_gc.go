package agent

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
)

var gcLog = logging.NewLogger("mem_gc")

// MemoryGCScheduler 记忆垃圾回收调度器。
type MemoryGCScheduler struct {
	dao interface {
		GetConfig(ctx context.Context) (*models.MemoryGCConfig, error)
	}
	interval   time.Duration
	mem0Client interface{ Health() error }
}

// NewMemoryGCScheduler 创建 GC 调度器。
func NewMemoryGCScheduler(dao interface {
	GetConfig(ctx context.Context) (*models.MemoryGCConfig, error)
}, mem0Client interface{ Health() error }) *MemoryGCScheduler {
	return &MemoryGCScheduler{
		dao:        dao,
		interval:   time.Hour,
		mem0Client: mem0Client,
	}
}

// Run 启动 GC 循环。
func (gc *MemoryGCScheduler) Run(ctx context.Context) {
	gcLog.Info("记忆 GC 调度器已启动")
	defer gcLog.Info("记忆 GC 调度器已停止")

	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gc.runGCCycle(ctx)
		}
	}
}

func (gc *MemoryGCScheduler) runGCCycle(ctx context.Context) {
	cfg, err := gc.dao.GetConfig(ctx)
	if err != nil || !cfg.Enable {
		return
	}

	gcLog.Info("开始记忆 GC", "cold_threshold_days", cfg.ColdThreshold, "max_per_agent", cfg.MaxPerAgent)

	// 更新间隔
	if cfg.IntervalMins > 0 {
		gc.interval = time.Duration(cfg.IntervalMins) * time.Minute
	}

	// TODO: 当 Mem0 完全集成后，调用 Mem0 API 清理冷记忆
	// 现在记录 GC 意图
	gcLog.Info("记忆 GC 完成",
		"cold_threshold_days", cfg.ColdThreshold,
		"max_per_agent", cfg.MaxPerAgent,
	)
}
