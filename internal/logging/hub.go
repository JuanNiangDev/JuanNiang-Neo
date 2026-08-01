package logging

import (
	"sync"
	"time"
)

// MaxBuffer 是环形缓冲区容量，对应"最近 250 条日志"。
const MaxBuffer = 250

// Entry 表示一条日志记录（stdout + Web SSE 统一格式）。
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Module  string         `json:"module"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Rich    map[string]any `json:"rich,omitempty"` // WARN/ERROR 额外诊断信息
}

// Hub 是日志中心：维护最近 MaxBuffer 条日志的环形缓冲，并把新日志广播给订阅者。
//
// 进程内单例 Default 通常被 slog Handler 持有；HTTP/SSE 端从同一个 Hub 读取历史
// 与订阅实时事件。
type Hub struct {
	mu          sync.RWMutex
	buffer      []Entry
	head        int  // 环形缓冲下一个写入位置
	full        bool // 缓冲是否已经写满
	subscribers map[chan Entry]struct{}
}

// NewHub 创建新的 Hub。
func NewHub() *Hub {
	return &Hub{
		buffer:      make([]Entry, MaxBuffer),
		subscribers: make(map[chan Entry]struct{}),
	}
}

// Push 写入一条日志并广播给所有订阅者。
//
// 慢订阅者（channel 满）会被跳过，避免拖慢日志路径。
func (h *Hub) Push(e Entry) {
	h.mu.Lock()
	h.buffer[h.head] = e
	h.head = (h.head + 1) % MaxBuffer
	if !h.full && h.head == 0 {
		h.full = true
	}
	// 复制订阅者列表以减小锁持有时间
	subs := make([]chan Entry, 0, len(h.subscribers))
	for ch := range h.subscribers {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// 慢订阅者：丢弃本条事件
		}
	}
}

// Recent 返回最近 MaxBuffer 条日志，按时间顺序排列。
//
// 若总数不足 MaxBuffer，则只返回实际存在的条目。
func (h *Hub) Recent() []Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if !h.full {
		// 未满：只取 head 之前的部分
		result := make([]Entry, h.head)
		copy(result, h.buffer[:h.head])
		return result
	}

	// 已满：从 head 开始按写入顺序返回
	result := make([]Entry, MaxBuffer)
	n := copy(result, h.buffer[h.head:])
	copy(result[n:], h.buffer[:h.head])
	return result
}

// Subscribe 订阅实时日志流。返回的 channel 缓冲为 64。
//
// 调用者必须在结束时调用 Unsubscribe 以释放资源。
func (h *Hub) Subscribe() chan Entry {
	ch := make(chan Entry, 64)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe 取消订阅并关闭 channel。
func (h *Hub) Unsubscribe(ch chan Entry) {
	h.mu.Lock()
	if _, ok := h.subscribers[ch]; ok {
		delete(h.subscribers, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// SubscriberCount 返回当前订阅者数量（主要用于诊断/监控）。
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// Default 是进程级 Hub 单例，供 slog Handler 与 HTTP 端共享。
var Default = NewHub()
