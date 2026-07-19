package logging

import (
	"context"
	"io"
	"log/slog"
)

// Handler 是 slog.Handler 装饰器：同时把日志写入底层 stdio handler 和 Hub。
//
// 用于实现"日志同时输出到 stdio 与前端"的需求：
//   - stdio 路径继续走原 slog.TextHandler（向后兼容）
//   - Hub 路径维护最近 250 条 + 实时推送给 SSE 订阅者
type Handler struct {
	stdio  slog.Handler // 底层 stdio handler
	hub    *Hub         // 日志广播中心
	preAttrs []slog.Attr // WithAttrs 累积的属性
}

// NewHandler 创建双输出 handler。
//
// 若 hub 为 nil，则等价于直接使用 stdio handler（用于测试或临时禁用前端推送）。
func NewHandler(w io.Writer, hub *Hub, opts *slog.HandlerOptions) *Handler {
	return &Handler{
		stdio: slog.NewTextHandler(w, opts),
		hub:   hub,
	}
}

// Enabled 透传给底层 stdio handler。
func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.stdio.Enabled(ctx, l)
}

// Handle 先写 stdio，再推送 Hub。
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.stdio.Handle(ctx, r); err != nil {
		return err
	}

	if h.hub == nil {
		return nil
	}

	entry := Entry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   make(map[string]any, r.NumAttrs()+len(h.preAttrs)),
	}

	// 先填入 WithAttrs 累积的属性
	for _, attr := range h.preAttrs {
		entry.Attrs[attr.Key] = attr.Value.Any()
	}
	// 再填入本次记录的属性（覆盖同名键）
	r.Attrs(func(attr slog.Attr) bool {
		entry.Attrs[attr.Key] = attr.Value.Any()
		return true
	})

	h.hub.Push(entry)
	return nil
}

// WithAttrs 同时为 stdio 路径和 Hub 路径保留属性。
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newPre := make([]slog.Attr, 0, len(h.preAttrs)+len(attrs))
	newPre = append(newPre, h.preAttrs...)
	newPre = append(newPre, attrs...)
	return &Handler{
		stdio:    h.stdio.WithAttrs(attrs),
		hub:      h.hub,
		preAttrs: newPre,
	}
}

// WithGroup 目前仅透传给 stdio 路径；Hub 路径不维护 group 前缀。
//
// 这样做的原因：前端通常按 attr key 直接定位字段，引入 group 前缀反而会让消费方
// 处理变复杂。如果未来需要 group 支持，可以在此扩展。
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		stdio:    h.stdio.WithGroup(name),
		hub:      h.hub,
		preAttrs: h.preAttrs,
	}
}

// 编译期断言：确保实现 slog.Handler 接口。
var _ slog.Handler = (*Handler)(nil)
