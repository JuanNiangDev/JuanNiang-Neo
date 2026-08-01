// Package logging 提供带颜色、模块化、双输出（stdio + SSE Hub）的日志系统。
//
// 标准输出格式：
//
//	2006-01-02 15:04:05.000 [INFO ] [module] message | k=v k2=v2
//
// 颜色方案：
//   - DEBUG: 暗色
//   - INFO : 绿色
//   - WARN : 黄色
//   - ERROR: 红色
//
// Hub 输出（SSE 推送）额外包含 rich_metadata，WARN/ERROR 携带更多诊断信息。
package logging

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// Level 日志级别。
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKN"
	}
}

// Fields 是结构化字段（用于 Web 端 richer metadata）。
type Fields map[string]any

// Logger 模块级日志记录器。
type Logger struct {
	module string
	mu     sync.Mutex
}

// 全局默认 Logger，用于尚未迁移的代码。
var defaultLogger = &Logger{module: "global"}

// ———————— 全局配置 ————————

var (
	stdout    io.Writer = os.Stdout
	globalHub *Hub      = Default
	verbosity Level     = DEBUG
)

// SetOutput 设置 stdio 输出目标（通常 os.Stdout）。
func SetOutput(w io.Writer) { stdout = w }

// SetHub 设置 Hub 实例（nil 禁用 SSE 推送）。
func SetHub(h *Hub) { globalHub = h }

// SetLevel 设置全局最低输出级别。
func SetLevel(l Level) { verbosity = l }

// ———————— 构造函数 ————————

// NewLogger 创建一个带模块名的 Logger。
func NewLogger(module string) *Logger {
	return &Logger{module: module}
}

// DefaultLogger 返回全局默认 Logger。
func DefaultLogger() *Logger { return defaultLogger }

// ———————— 全局便捷方法 ————————

func Debug(msg string, kvs ...any) { defaultLogger.Debug(msg, kvs...) }
func Info(msg string, kvs ...any)  { defaultLogger.Info(msg, kvs...) }
func Warn(msg string, kvs ...any)  { defaultLogger.Warn(msg, kvs...) }
func Error(msg string, kvs ...any) { defaultLogger.Error(msg, kvs...) }

// ———————— Logger 方法 ————————

func (l *Logger) Debug(msg string, kvs ...any) { l.log(DEBUG, msg, kvs) }
func (l *Logger) Info(msg string, kvs ...any)  { l.log(INFO, msg, kvs) }
func (l *Logger) Warn(msg string, kvs ...any)  { l.log(WARN, msg, kvs) }
func (l *Logger) Error(msg string, kvs ...any) { l.log(ERROR, msg, kvs) }

func (l *Logger) log(level Level, msg string, kvs []any) {
	if level < verbosity {
		return
	}

	now := time.Now()
	timestamp := now.Format("2006-01-02 15:04:05.000")

	// stdout 彩色输出
	l.writeStdout(level, timestamp, msg, kvs)

	// Hub 输出（SSE 推送）
	if globalHub != nil {
		l.pushHub(level, now, msg, kvs)
	}
}

func (l *Logger) writeStdout(level Level, ts, msg string, kvs []any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// [LEVEL] 着色
	levelStr := fmt.Sprintf("%-5s", level.String())
	var levelOut string
	switch level {
	case DEBUG:
		levelOut = color.New(color.FgCyan, color.Faint).Sprintf("[%s]", levelStr)
	case INFO:
		levelOut = color.New(color.FgGreen, color.Bold).Sprintf("[%s]", levelStr)
	case WARN:
		levelOut = color.New(color.FgYellow, color.Bold).Sprintf("[%s]", levelStr)
	case ERROR:
		levelOut = color.New(color.FgRed, color.Bold).Sprintf("[%s]", levelStr)
	default:
		levelOut = fmt.Sprintf("[%s]", levelStr)
	}

	// 模块名
	moduleOut := color.New(color.FgBlue).Sprintf("[%s]", l.module)

	// 消息（ERROR 加亮）
	var msgOut string
	if level == ERROR {
		msgOut = color.New(color.FgRed).Sprint(msg)
	} else if level == WARN {
		msgOut = color.New(color.FgYellow).Sprint(msg)
	} else {
		msgOut = msg
	}

	// 构建 k=v 额外信息
	extra := formatKeyVals(kvs)
	var extraOut string
	if extra != "" {
		extraOut = color.New(color.Faint).Sprintf(" | %s", extra)
	}

	fmt.Fprintf(stdout, "%s %s %s %s%s\n", ts, levelOut, moduleOut, msgOut, extraOut)
}

func (l *Logger) pushHub(level Level, now time.Time, msg string, kvs []any) {
	fields := make(Fields, len(kvs)/2+3)
	for i := 0; i+1 < len(kvs); i += 2 {
		key := fmt.Sprint(kvs[i])
		fields[key] = kvs[i+1]
	}

	entry := Entry{
		Time:    now,
		Level:   level.String(),
		Module:  l.module,
		Message: msg,
		Attrs:   fields,
	}

	// WARN / ERROR 携带更丰富的元数据
	if level >= WARN {
		rich := make(Fields, 4)
		pc, file, line, ok := runtime.Caller(2)
		if ok {
			if fn := runtime.FuncForPC(pc); fn != nil {
				rich["caller_func"] = fn.Name()
			}
			rich["caller_file"] = fmt.Sprintf("%s:%d", file, line)
		}
		rich["goroutines"] = runtime.NumGoroutine()
		entry.Rich = rich
	}

	globalHub.Push(entry)
}

func formatKeyVals(kvs []any) string {
	if len(kvs) == 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i+1 < len(kvs); i += 2 {
		if i > 0 {
			sb.WriteString(" ")
		}
		key := fmt.Sprint(kvs[i])
		val := kvs[i+1]
		// 字符串值加引号
		if s, ok := val.(string); ok {
			sb.WriteString(fmt.Sprintf("%s=%q", key, s))
		} else {
			sb.WriteString(fmt.Sprintf("%s=%v", key, val))
		}
	}
	// 奇数个参数，最后一个是独立值
	if len(kvs)%2 != 0 {
		sb.WriteString(fmt.Sprintf(" _extra=%v", kvs[len(kvs)-1]))
	}
	return sb.String()
}
