package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
)

// Handler 是 slog.Handler 的适配层：把 slog 日志桥接到新的 Logger 系统。
//
// 这样现有代码中的 slog.Info/Warn/Error/Debug 调用无需立即全部改写，
// 仍能享受新系统的彩色输出、JSON 格式化、调用栈、Hub 推送等功能。
//
// 新代码应优先使用 logging.NewModule("xxx").Info(...) 以获得模块名支持。
type Handler struct {
	module   string
	preAttrs []slog.Attr
	group    string
	level    slog.Level
}

// NewHandler 创建 slog.Handler 桥接器。
//
// w 和 opts 保留仅为兼容旧的调用签名；实际输出由全局 Logger 控制。
// hub 参数保留但不再使用（Hub 由全局配置管理）。
func NewHandler(w io.Writer, hub *Hub, opts *slog.HandlerOptions) *Handler {
	lvl := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		lvl = opts.Level.Level()
	}
	return &Handler{
		level: lvl,
	}
}

// NewHandlerWithModule 创建带模块名的 slog.Handler 桥接器。
func NewHandlerWithModule(module string, w io.Writer, hub *Hub, opts *slog.HandlerOptions) *Handler {
	lvl := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		lvl = opts.Level.Level()
	}
	return &Handler{
		module: module,
		level:  lvl,
	}
}

// Enabled 根据全局级别配置判断是否启用。
func (h *Handler) Enabled(_ context.Context, l slog.Level) bool {
	if l == slog.LevelDebug {
		return IsDebug()
	}
	return l >= h.level
}

// Handle 把 slog.Record 转为 Entry 并通过新系统输出。
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	level := slogToLevel(r.Level)

	attrs := make(map[string]any, r.NumAttrs()+len(h.preAttrs))

	// 先填入 WithAttrs 累积的属性
	for _, attr := range h.preAttrs {
		key := h.prefixKey(attr.Key)
		attrs[key] = attr.Value.Any()
	}
	// 再填入本次记录的属性
	r.Attrs(func(attr slog.Attr) bool {
		key := h.prefixKey(attr.Key)
		attrs[key] = attr.Value.Any()
		return true
	})

	// 调用栈：Warn 及以上
	var stack string
	if level >= LevelWarn {
		// r.PC 是 slog 记录的调用点，skip=0 从 r.PC 开始
		stack = captureStackFromPC(r.PC)
	}

	entry := Entry{
		Time:    r.Time,
		Level:   level.String(),
		Module:  h.module,
		Message: r.Message,
		Attrs:   attrs,
		Stack:   stack,
	}

	// 写入 stdio
	writeStdio(entry, level)

	// 推送 Hub
	gMu.RLock()
	hub := gHub
	gMu.RUnlock()
	if hub != nil {
		hub.Push(entry)
	}

	return nil
}

// WithAttrs 返回携带额外属性的新 Handler。
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newPre := make([]slog.Attr, 0, len(h.preAttrs)+len(attrs))
	newPre = append(newPre, h.preAttrs...)
	newPre = append(newPre, attrs...)
	return &Handler{
		module:   h.module,
		preAttrs: newPre,
		group:    h.group,
		level:    h.level,
	}
}

// WithGroup 返回带 group 前缀的新 Handler。
func (h *Handler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}
	return &Handler{
		module:   h.module,
		preAttrs: h.preAttrs,
		group:    g,
		level:    h.level,
	}
}

// 编译期断言。
var _ slog.Handler = (*Handler)(nil)

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

func (h *Handler) prefixKey(key string) string {
	if h.group != "" {
		return h.group + "." + key
	}
	return key
}

// slogToLevel 把 slog.Level 映射到 logging.Level。
func slogToLevel(l slog.Level) Level {
	switch {
	case l >= slog.LevelError:
		return LevelError
	case l >= slog.LevelWarn:
		return LevelWarn
	case l >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}

// captureStackFromPC 从 slog.Record 的 PC 值开始捕获调用栈。
// captureStackFromPC 从 slog.Record 的 PC 值开始捕获调用栈。
func captureStackFromPC(pc uintptr) string {
	if pc == 0 {
		return ""
	}

	frames := runtime.CallersFrames([]uintptr{pc})
	var buf bytes.Buffer
	buf.WriteString("Stack trace:\n")
	count := 0

	for {
		frame, more := frames.Next()
		if frame.Function == "" {
			break
		}
		// 跳过 logging 包自身帧
		if strings.Contains(frame.Function, "JuanNiang-Neo/internal/logging") {
			if !more {
				break
			}
			continue
		}
		fmt.Fprintf(&buf, "  %s\n    %s:%d\n", frame.Function, frame.File, frame.Line)
		count++
		if count >= 10 || !more {
			break
		}
	}

	if count == 0 {
		return ""
	}
	return buf.String()
}
