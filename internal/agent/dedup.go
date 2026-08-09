package agent

import (
	"context"
	"sync"
	"time"

	"JuanNiang-Neo/internal/logging"
)

var dedupLog = logging.NewModule("dedup")

// Deduper 消息去重器接口：判定 key 是否在去重窗口内已出现过。
// 不同实现提供不同持久化/共享语义：
//   - memoryDedup: 进程内 map + Mutex，重启即丢、多实例独立（仅作 Redis 不可用时的 fallback）
//   - redisDedup:  Redis SET NX EX，重启不丢、多实例共享、原子无锁（生产默认）
//
// 调用方约定：返回 true 表示"已见过"（应丢弃），false 表示"首次出现"（应放行）。
// 实现需保证自身并发安全；调用方在 processEvent 入口同步调用即可。
type Deduper interface {
	SeenBefore(ctx context.Context, key string) bool
}

// ---------- 内存实现（fallback） ----------

// memoryDedup 进程内去重器：基于 OneBot11 message_id 过滤重复投递的消息。
// 背景：WS 断线重连/多连接时，OneBot 客户端可能把同一条消息重复推送到
// events channel（adapter 层不做去重），导致下游 Agent 重复消费。
// message_id 在同一消息来源（群/私聊各自独立递增）内稳定，可作幂等键。
//
// 局限：纯内存态，进程重启即清空；多实例间状态独立。
// 仅在 Redis 不可用时作为降级方案，生产环境应优先使用 redisDedup。
type memoryDedup struct {
	mu   sync.Mutex
	seen map[string]time.Time // key -> 首次记录时间（惰性过期清理）
	ttl  time.Duration
}

// newMemoryDedup 创建内存去重器，ttl 为去重窗口。
func newMemoryDedup(ttl time.Duration) *memoryDedup {
	return &memoryDedup{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// SeenBefore 判断 key 是否在去重窗口内已出现过；未出现过则记录并返回 false。
func (d *memoryDedup) SeenBefore(ctx context.Context, key string) bool {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	if ts, ok := d.seen[key]; ok && now.Sub(ts) < d.ttl {
		dedupLog.Info("memory 命中拦截",
			"key", key,
			"first_seen_ago", now.Sub(ts).String(),
			"ttl_remaining", (d.ttl - now.Sub(ts)).String(),
		)
		return true
	}
	// 惰性清理：map 达到上限时删除过期条目，防止无界增长
	if len(d.seen) >= 8192 {
		expired := 0
		for k, ts := range d.seen {
			if now.Sub(ts) >= d.ttl {
				delete(d.seen, k)
				expired++
			}
		}
		if expired > 0 {
			dedupLog.Debug("memory 惰性清理", "expired", expired, "remaining", len(d.seen))
		}
	}
	d.seen[key] = now
	dedupLog.Debug("memory 首次放行", "key", key)
	return false
}

// ---------- Redis 实现（生产默认） ----------

// redisDedup 基于 Redis SET NX EX 的去重器：
//   - 原子：单条 SET NX 命令完成"判定+记录"，无 TOCTOU
//   - 持久：进程重启不丢失去重状态（Redis 独立部署）
//   - 共享：多实例水平扩展时天然共享同一份去重状态
//   - TTL：由 Redis 自动过期，无需手动清理
//
// 降级策略：Redis 调用出错时返回 false（放行），避免去重器故障导致 Agent 整体不可用。
// 此时下游 Layer 2（短期记忆 Lua 原子幂等）仍能兜住一部分重复消费。
//
// 依赖抽象：redisDedup 只需要 cache 的 SetNX 一个方法，故抽出 dedupStore 接口，
// 便于测试用 fake 而非真实 Redis（无需 miniredis 等依赖）。
type dedupStore interface {
	SetNX(ctx context.Context, key string, val any, ttl time.Duration) (bool, error)
}

type redisDedup struct {
	cache dedupStore
	ttl   time.Duration
}

// newRedisDedup 创建 Redis 去重器。*cache.Cache 隐式实现 dedupStore 接口。
func newRedisDedup(c dedupStore, ttl time.Duration) *redisDedup {
	return &redisDedup{cache: c, ttl: ttl}
}

// SeenBefore 用 SET NX EX 原子占位：key 已存在返回 true（已见过），
// 新建返回 false（首次出现）。Redis 出错时返回 false（放行，降级）。
func (d *redisDedup) SeenBefore(ctx context.Context, key string) bool {
	const dedupKeyPrefix = "dedup:msg:"
	ok, err := d.cache.SetNX(ctx, dedupKeyPrefix+key, "1", d.ttl)
	if err != nil {
		// Redis 故障：放行而非丢弃，避免去重器故障让整体停摆。
		// 下游短期记忆的 Lua 原子幂等仍能兜住部分重复。
		dedupLog.Warn("redis 调用失败降级放行", "key", key, "ttl", d.ttl.String(), "err", err)
		return false
	}
	if ok {
		// SetNX 返回 true=新建（首次出现，放行）→ 返回 false
		dedupLog.Debug("redis 首次放行", "key", key, "ttl", d.ttl.String())
		return false
	}
	// SetNX 返回 false=已存在（重复，拦截）→ 返回 true
	dedupLog.Info("redis 命中拦截",
		"key", key,
		"ttl", d.ttl.String(),
	)
	return true
}
