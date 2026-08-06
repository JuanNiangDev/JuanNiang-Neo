package agent

import (
	"sync"
	"time"
)

// msgDedup 消息去重器：基于 OneBot11 message_id 过滤重复投递的消息。
// 背景：WS 断线重连/多连接时，OneBot 客户端可能把同一条消息重复推送到
// events channel（adapter 层不做去重），导致下游 Agent 重复消费。
// message_id 在同一消息来源（群/私聊各自独立递增）内稳定，可作幂等键。
type msgDedup struct {
	mu   sync.Mutex
	seen map[string]time.Time // key -> 首次记录时间（惰性过期清理）
	ttl  time.Duration
}

// newMsgDedup 创建去重器，ttl 为去重窗口（需大于 WS 重连/重推的最长间隔）。
func newMsgDedup(ttl time.Duration) *msgDedup {
	return &msgDedup{
		seen: make(map[string]time.Time),
		ttl:  ttl,
	}
}

// seenBefore 判断 key 是否在去重窗口内已出现过；未出现过则记录并返回 false。
func (d *msgDedup) seenBefore(key string) bool {
	now := time.Now()
	d.mu.Lock()
	defer d.mu.Unlock()

	if ts, ok := d.seen[key]; ok && now.Sub(ts) < d.ttl {
		return true
	}
	// 惰性清理：map 达到上限时删除过期条目，防止无界增长
	if len(d.seen) >= 8192 {
		for k, ts := range d.seen {
			if now.Sub(ts) >= d.ttl {
				delete(d.seen, k)
			}
		}
	}
	d.seen[key] = now
	return false
}
