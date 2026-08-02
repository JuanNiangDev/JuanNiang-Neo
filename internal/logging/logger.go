package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// ---------------------------------------------------------------------------
// Level
// ---------------------------------------------------------------------------

// Level 日志级别。
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelLLM // 大模型专用级别
)

// String 返回级别名称。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelLLM:
		return "LLM"
	default:
		return "UNKNOWN"
	}
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config 日志系统配置。
type Config struct {
	Debug       bool      // Debug 模式：显示完整 LLM 输出 + DEBUG 级别日志
	Output      io.Writer // 输出目标，默认 os.Stdout
	Hub         *Hub      // 日志中心，nil 则不推送 Web 日志
	LLMMaxChars int       // Prod 模式下 LLM 输出截取字符数，默认 300
}

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

// Logger 模块日志器。每个子系统创建一个带模块名的 Logger。
type Logger struct {
	module string
}

// global 配置（所有 Logger 共享）。
var (
	gMu        sync.RWMutex
	gDebug     bool
	gOutput    io.Writer = os.Stdout
	gHub       *Hub
	gLLMMaxLen int = 300
)

// 颜色实例（线程安全）。
var (
	colorDebug  = color.New(color.FgCyan)
	colorInfo   = color.New(color.FgGreen)
	colorWarn   = color.New(color.FgYellow)
	colorError  = color.New(color.FgRed, color.Bold)
	colorLLM    = color.New(color.FgMagenta)
	colorModule = color.New(color.FgBlue)
	colorTime   = color.New(color.Faint)
	colorKey    = color.New(color.FgHiWhite, color.Bold)
	colorJSON   = color.New(color.FgHiCyan)
	colorStack  = color.New(color.FgHiRed)
)

// Init 初始化全局日志配置。应在 main() 中调用一次。
func Init(cfg Config) {
	gMu.Lock()
	defer gMu.Unlock()
	gDebug = cfg.Debug
	if cfg.Output != nil {
		gOutput = cfg.Output
	}
	gHub = cfg.Hub
	if cfg.LLMMaxChars > 0 {
		gLLMMaxLen = cfg.LLMMaxChars
	}
}

// IsDebug 返回当前是否为 Debug 模式。
func IsDebug() bool {
	gMu.RLock()
	defer gMu.RUnlock()
	return gDebug
}

// NewModule 创建带模块名的 Logger。
func NewModule(module string) *Logger {
	return &Logger{module: module}
}

// ---------------------------------------------------------------------------
// Logger 方法
// ---------------------------------------------------------------------------

// Debug 输出一条 DEBUG 级别日志。
func (l *Logger) Debug(msg string, kvs ...any) {
	l.log(LevelDebug, msg, kvs, 2)
}

// Info 输出一条 INFO 级别日志。
func (l *Logger) Info(msg string, kvs ...any) {
	l.log(LevelInfo, msg, kvs, 2)
}

// Warn 输出一条 WARN 级别日志（包含调用栈）。
func (l *Logger) Warn(msg string, kvs ...any) {
	l.log(LevelWarn, msg, kvs, 2)
}

// Error 输出一条 ERROR 级别日志（包含调用栈）。
func (l *Logger) Error(msg string, kvs ...any) {
	l.log(LevelError, msg, kvs, 2)
}

// LLM 输出大模型相关的日志。Prod 模式截取内容，Debug 模式显示完整。
//
// direction 通常为 "request" 或 "response"。
func (l *Logger) LLM(direction string, content string, kvs ...any) {
	l.logLLM(direction, content, kvs, 2)
}

// Fatal 输出一条 ERROR 日志后退出进程。
func (l *Logger) Fatal(msg string, kvs ...any) {
	l.log(LevelError, msg, kvs, 2)
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// Package-level 便捷函数（module = ""）
// ---------------------------------------------------------------------------

var defaultLogger = &Logger{module: ""}

// Debug 输出一条 DEBUG 日志（无模块名）。
func Debug(msg string, kvs ...any) { defaultLogger.Debug(msg, kvs...) }

// Info 输出一条 INFO 日志（无模块名）。
func Info(msg string, kvs ...any) { defaultLogger.Info(msg, kvs...) }

// Warn 输出一条 WARN 日志（无模块名）。
func Warn(msg string, kvs ...any) { defaultLogger.Warn(msg, kvs...) }

// Error 输出一条 ERROR 日志（无模块名）。
func Error(msg string, kvs ...any) { defaultLogger.Error(msg, kvs...) }

// LLM 输出大模型相关日志（无模块名）。
func LLM(direction, content string, kvs ...any) { defaultLogger.LLM(direction, content, kvs...) }

// ---------------------------------------------------------------------------
// 核心 log 方法
// ---------------------------------------------------------------------------

func (l *Logger) log(level Level, msg string, kvs []any, skip int) {
	// Debug 级别在非 Debug 模式下跳过
	if level == LevelDebug && !IsDebug() {
		return
	}

	// 解析 kvs → map
	attrs := kvsToMap(kvs)

	// 调用栈：Warn 及以上级别捕获完整栈；其余仅捕获 caller
	var stack string
	if level >= LevelWarn {
		stack = captureStack(skip + 1)
	}

	entry := Entry{
		Time:    time.Now(),
		Level:   level.String(),
		Module:  l.module,
		Message: msg,
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
}

func (l *Logger) logLLM(direction, content string, kvs []any, skip int) {
	attrs := kvsToMap(kvs)
	attrs["direction"] = direction

	// Prod 模式截取内容
	displayContent := content
	if !IsDebug() && len(content) > gLLMMaxLen {
		displayContent = content[:gLLMMaxLen] + fmt.Sprintf(" ... [truncated, total %d chars]", len(content))
	}
	attrs["content"] = displayContent

	entry := Entry{
		Time:    time.Now(),
		Level:   LevelLLM.String(),
		Module:  l.module,
		Message: "LLM " + direction,
		Attrs:   attrs,
	}

	writeStdio(entry, LevelLLM)

	gMu.RLock()
	hub := gHub
	gMu.RUnlock()
	if hub != nil {
		hub.Push(entry)
	}
}

// ---------------------------------------------------------------------------
// stdio 格式化输出
// ---------------------------------------------------------------------------

func writeStdio(e Entry, level Level) {
	var buf bytes.Buffer

	// 1. 时间戳
	ts := e.Time.Format("15:04:05.000")
	colorTime.Fprintf(&buf, "%s", ts)
	buf.WriteString(" ")

	// 2. 级别（带颜色 + 固定宽度对齐）
	lvlStr := fmtLevel(level)
	switch level {
	case LevelDebug:
		colorDebug.Fprintf(&buf, "%s", lvlStr)
	case LevelInfo:
		colorInfo.Fprintf(&buf, "%s", lvlStr)
	case LevelWarn:
		colorWarn.Fprintf(&buf, "%s", lvlStr)
	case LevelError:
		colorError.Fprintf(&buf, "%s", lvlStr)
	case LevelLLM:
		colorLLM.Fprintf(&buf, "%s", lvlStr)
	}
	buf.WriteString(" ")

	// 3. 模块名
	if e.Module != "" {
		colorModule.Fprintf(&buf, "[%s]", e.Module)
		buf.WriteString(" ")
	}

	// 4. 消息（尝试 JSON 格式化）
	if isJSON(e.Message) {
		buf.WriteString(colorizeJSON(e.Message))
	} else {
		buf.WriteString(e.Message)
	}

	// 5. 元数据
	if len(e.Attrs) > 0 {
		buf.WriteString(" ")
		writeAttrs(&buf, e.Attrs)
	}

	// 6. 调用栈（Warn+）
	if e.Stack != "" {
		buf.WriteString("\n")
		colorStack.Fprintf(&buf, "%s", e.Stack)
	}

	buf.WriteString("\n")

	// 写到 output（加锁避免多 goroutine 交叉）
	gMu.RLock()
	w := gOutput
	gMu.RUnlock()
	fmt.Fprint(w, buf.String())
}

// fmtLevel 返回固定宽度的级别标识。
func fmtLevel(level Level) string {
	switch level {
	case LevelDebug:
		return "[DEBUG]"
	case LevelInfo:
		return "[INFO] "
	case LevelWarn:
		return "[WARN] "
	case LevelError:
		return "[ERROR]"
	case LevelLLM:
		return "[LLM]  "
	default:
		return "[?????]"
	}
}

// writeAttrs 将元数据写入 buf。JSON 类型的值自动格式化 + 着色。
func writeAttrs(buf *bytes.Buffer, attrs map[string]any) {
	first := true
	for k, v := range attrs {
		if !first {
			buf.WriteString(" ")
		}
		first = false
		colorKey.Fprintf(buf, "%s=", k)
		s := fmt.Sprintf("%v", v)
		if isJSON(s) {
			buf.WriteString(colorizeJSON(s))
		} else {
			buf.WriteString(s)
		}
	}
}

// ---------------------------------------------------------------------------
// JSON 检测 & 着色
// ---------------------------------------------------------------------------

// isJSON 判断字符串是否为 JSON（object 或 array）。
func isJSON(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return false
	}
	return (s[0] == '{' && s[len(s)-1] == '}') || (s[0] == '[' && s[len(s)-1] == ']')
}

// colorizeJSON 尝试格式化 JSON 并着色。非 JSON 原样返回。
func colorizeJSON(s string) string {
	var raw any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return s // 不是合法 JSON，原样返回
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return s
	}
	return colorJSON.Sprint(string(pretty))
}

// ---------------------------------------------------------------------------
// 调用栈捕获
// ---------------------------------------------------------------------------

// captureStack 捕获调用栈，跳过 skip 层 + logging 包自身帧。
func captureStack(skip int) string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(skip+2, pcs) // +2: Callers 自身 + captureStack
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var buf bytes.Buffer
	buf.WriteString("Stack trace:\n")
	count := 0
	for {
		frame, more := frames.Next()
		// 跳过 logging 包内部帧
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
	return buf.String()
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// kvsToMap 将 [key1, val1, key2, val2, ...] 转为 map。
func kvsToMap(kvs []any) map[string]any {
	if len(kvs) == 0 {
		return nil
	}
	m := make(map[string]any, len(kvs)/2)
	for i := 0; i+1 < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", kvs[i])
		}
		m[key] = kvs[i+1]
	}
	return m
}
