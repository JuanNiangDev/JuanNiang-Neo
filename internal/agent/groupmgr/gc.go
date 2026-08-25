package groupmgr

import (
	"context"
	"time"
)

// 白名单语录 GC：周期性清理长期未命中的白名单语录（Postgres + RAG 向量双删）。
//
// 周期（默认 7 天，GroupMgrConfig.WhiteGCIntervalDays 可调，参数设置面板）
// 每次执行删除最近周期内 LastUsedAt 最旧的 5 条（从未命中优先）。
const (
	whiteGCInterval    = time.Hour // 心跳检查间隔
	whiteGCPageSize    = 5         // 每次执行删除条数上限
	whiteGCDefaultDays = 7         // 默认周期（天）
)

// StartWhiteGC 启动白名单语录 GC 循环（main.go 调用，独立 goroutine 不阻塞检测）。
func (m *Manager) StartWhiteGC(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(whiteGCInterval)
		defer ticker.Stop()
		lastRun := time.Now() // 启动时不立即执行，等待至少一个周期
		for {
			select {
			case <-ctx.Done():
				log.Info("白名单语录 GC 循环已停止")
				return
			case <-ticker.C:
				days := m.whiteGCDays(ctx)
				if time.Since(lastRun) < time.Duration(days)*24*time.Hour {
					continue
				}
				if err := m.runWhiteGC(ctx, days); err != nil {
					log.Warn("白名单语录 GC 执行失败", "err", err)
				} else {
					lastRun = time.Now()
				}
			}
		}
	}()
	log.Info("白名单语录 GC 循环已启动")
}

// whiteGCDays 读取白名单 GC 周期（天），无配置/异常回退默认 7。
func (m *Manager) whiteGCDays(ctx context.Context) int {
	cfg, err := m.dao.GetConfig(ctx)
	if err != nil || cfg == nil || cfg.WhiteGCIntervalDays <= 0 {
		return whiteGCDefaultDays
	}
	return cfg.WhiteGCIntervalDays
}

// runWhiteGC 执行一次白名单语录 GC：删除最近 days 天未命中的 5 条
// （PG 行 + RAG 向量双删，白名单 tag）。RAG 删除失败保留 PG 行并标记未同步，
// 下次执行重试（先删向量成功再删主库，避免孤儿向量）。
func (m *Manager) runWhiteGC(ctx context.Context, days int) error {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	samples, err := m.dao.SampleListUnused(ctx, "white", since, whiteGCPageSize)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}
	removed := 0
	for _, s := range samples {
		// 先删 RAG 向量：失败则保留 PG 行并标记未同步，下次 GC 重试（防孤儿向量）
		cli := m.getRAG()
		if cli == nil {
			// RAG 不可用：仅未同步语录直接删主库，已同步语录保留待重试（防孤儿向量）
			if s.RAGSynced {
				log.Warn("白名单 GC：RAG 不可用，保留已同步语录待重试", "phrase", s.ID)
				continue
			}
		} else if rerr := m.deleteRAGPhrase(ctx, s.ID, "white"); rerr != nil {
			if merr := m.dao.SampleMarkRAGSynced(ctx, s.ID, false); merr != nil {
				log.Warn("白名单 GC：标记未同步失败", "phrase", s.ID, "err", merr)
			}
			continue
		}
		if derr := m.dao.SampleDelete(ctx, s.ID); derr != nil {
			log.Warn("白名单 GC：Postgres 删除失败", "phrase", s.ID, "err", derr)
			continue
		}
		removed++
	}
	if removed > 0 {
		log.Info("白名单语录 GC 完成", "removed", removed, "days", days)
	}
	m.invalidateSampleSet()
	return nil
}
