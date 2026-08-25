package agent

import (
	"context"
	"time"

	"JuanNiang-Neo/internal/core/ragtag"
)

// 长期记忆 GC：周期性清理长期未召回的条目（Postgres + RAG 向量双删）。
//
// 周期（默认 7 天）与使用窗口（默认 7 天，由 LongTermMemory.GCIntervalDays 配置）
// 均可在 Web 记忆页设置；每次执行删除全局最久未使用（last_recalled_at 最旧）
// 的 5 条。RAG 未配置/删除失败不影响 PG 删除（下次执行重试向量）。
const (
	memoryGCInterval    = time.Hour // 心跳检查间隔（每次检查是否到执行周期）
	memoryGCPageSize    = 5         // 每次执行删除条数上限
	memoryGCDefaultDays = 7         // 默认周期/使用窗口（天）
)

// StartLongTermMemoryGC 启动长期记忆 GC 循环（main.go 调用）。
// getCfg 提供当前周期配置（天）；独立 goroutine 运行，不阻塞主链路。
func (h *HagoCenter) StartLongTermMemoryGC(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(memoryGCInterval)
		defer ticker.Stop()
		lastRun := time.Now() // 启动时不立即执行，等待至少一个周期
		for {
			select {
			case <-ctx.Done():
				log.Info("长期记忆 GC 循环已停止")
				return
			case <-ticker.C:
				days := h.memoryGCDays(ctx)
				if time.Since(lastRun) < time.Duration(days)*24*time.Hour {
					continue
				}
				if err := h.runLongTermMemoryGC(ctx, days); err != nil {
					log.Warn("长期记忆 GC 执行失败", "err", err)
				} else {
					lastRun = time.Now()
				}
			}
		}
	}()
	log.Info("长期记忆 GC 循环已启动")
}

// memoryGCDays 读取记忆 GC 周期（天），无配置行/异常回退默认 7。
func (h *HagoCenter) memoryGCDays(ctx context.Context) int {
	cfg, err := h.DAO.LongTermMemory.First(ctx)
	if err != nil || cfg == nil || cfg.GCIntervalDays <= 0 {
		return memoryGCDefaultDays
	}
	return cfg.GCIntervalDays
}

// runLongTermMemoryGC 执行一次长期记忆 GC：删除最近 days 天未被召回的 5 条
// （PG 行 + RAG 向量双删）。RAG 删除失败保留 PG 行并标记未同步，下次执行重试
// （先删向量成功再删主库，避免孤儿向量）；RAG 未配置时直接删主库。
func (h *HagoCenter) runLongTermMemoryGC(ctx context.Context, days int) error {
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	items, err := h.DAO.LongTermMemItem.ListUnused(ctx, since, memoryGCPageSize)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	removed := 0
	ids := make(map[string]bool, len(items))
	for _, it := range items {
		// 先删 RAG 向量：失败则保留 PG 行并标记未同步，下次 GC 重试（防孤儿向量）
		if cli := h.RAGClient.Load(); cli != nil {
			if derr := cli.Delete(ctx, ragtag.Memory(it.ID)); derr != nil {
				log.Warn("记忆 GC：RAG 向量删除失败，保留 PG 行待重试", "mem_id", it.ID, "err", derr)
				if merr := h.DAO.LongTermMemItem.MarkRAGSynced(ctx, it.ID, false); merr != nil {
					log.Warn("记忆 GC：标记未同步失败", "mem_id", it.ID, "err", merr)
				}
				continue
			}
		}
		if derr := h.DAO.LongTermMemItem.Delete(ctx, it.ID); derr != nil {
			log.Warn("记忆 GC：Postgres 删除失败", "mem_id", it.ID, "err", derr)
			continue
		}
		ids[it.ID] = true
		removed++
	}
	if removed > 0 {
		log.Info("长期记忆 GC 完成", "removed", removed, "days", days)
		h.Memory.RemoveHotItems(ids)
	}
	return nil
}
