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

	// 调用栈 & 模块名自动检测
	var stack string
	module := h.module
	if level >= LevelWarn {
		// Warn 及以上：捕获完整调用栈 + 自动检测模块名
		stack, module = captureStackAndModule(r.PC, module)
	} else if module == "" {
		// 非 Warn 级别但模块名为空：仅从调用栈推导模块名（不生成完整栈）
		module = detectModule(r.PC)
	}

	entry := Entry{
		Time:    r.Time,
		Level:   level.String(),
		Module:  module,
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

// ---------------------------------------------------------------------------
// 调用栈捕获 & 模块名自动检测
// ---------------------------------------------------------------------------

// captureStackAndModule 从 slog.Record 的 PC 值出发，捕获完整调用栈并推导模块名。
//
// 原理：runtime.CallersFrames 对单个 PC 只能解析 1 帧（无法向上追溯调用链），
// 所以改用 runtime.Callers(skip, pcs) 获取当前 goroutine 的完整 PC 列表，
// 然后过滤掉 slog + logging 内部帧，保留用户代码帧。
func captureStackAndModule(pc uintptr, existingModule string) (stack string, module string) {
	module = existingModule

	// 获取当前调用栈（skip=3: Callers 自身 + captureStackAndModule + Handle + slog 内部）
	// 多取几帧确保覆盖到用户代码
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return "", module
	}

	frames := runtime.CallersFrames(pcs[:n])
	var buf bytes.Buffer
	buf.WriteString("Stack trace:\n")
	count := 0
	firstUserFrame := true

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
		// 跳过 log/slog 内部帧
		if strings.Contains(frame.Function, "log/slog") {
			if !more {
				break
			}
			continue
		}

		// 从第一个用户帧推导模块名
		if firstUserFrame && module == "" {
			module = deriveModuleFromFunc(frame.Function)
			firstUserFrame = false
		}

		fmt.Fprintf(&buf, "  %s\n    %s:%d\n", frame.Function, frame.File, frame.Line)
		count++
		if count >= 10 || !more {
			break
		}
	}

	if count == 0 {
		return "", module
	}
	return buf.String(), module
}

// detectModule 仅从 PC 推导模块名（不生成完整调用栈），用于非 Warn 级别的 slog 桥接。
func detectModule(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	frames := runtime.CallersFrames([]uintptr{pc})
	for {
		frame, more := frames.Next()
		if frame.Function == "" {
			break
		}
		if strings.Contains(frame.Function, "JuanNiang-Neo/internal/logging") ||
			strings.Contains(frame.Function, "log/slog") {
			if !more {
				break
			}
			continue
		}
		return deriveModuleFromFunc(frame.Function)
	}
	return ""
}

// deriveModuleFromFunc 从完整函数名推导模块名。
//
// 例如:
//
//	"JuanNiang-Neo/internal/adapter.(*Adapter).Start" → "adapter"
//	"JuanNiang-Neo/internal/agent.(*HagoCenter).handleMessage" → "agent"
//	"JuanNiang-Neo/internal/pluggin.(*PluginEngine).Load" → "plugin"
//	"JuanNiang-Neo/cmd/server.main" → "main"
func deriveModuleFromFunc(funcName string) string {
	// 只处理我们项目的函数
	if !strings.HasPrefix(funcName, "JuanNiang-Neo/") {
		return ""
	}

	// 去掉 "JuanNiang-Neo/" 前缀
	rest := funcName[len("JuanNiang-Neo/"):]

	// 取最后一段路径中的包名
	// "internal/agent.(*HagoCenter).handleMessage" → 取 "internal/agent"
	if idx := strings.Index(rest, "."); idx >= 0 {
		rest = rest[:idx]
	}

	// "internal/agent" → "agent"
	// "internal/agent/memory/shortterm" → "shortterm"
	// "cmd/server" → "main"
	if idx := strings.LastIndex(rest, "/"); idx >= 0 {
		rest = rest[idx+1:]
	}

	// 特殊映射
	if rest == "server" {
		return "main"
	}
	if rest == "pluggin" {
		return "plugin"
	}

	return rest
}
