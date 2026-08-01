package agent

import (
	"sync"
	"time"

	"JuanNiang-Neo/internal/logging"
)

var autoBgtaskLog = logging.NewLogger("auto_bgtask")

// ToolLatencyTracker 跟踪工具调用延迟，自动检测后台任务候选。
type ToolLatencyTracker struct {
	mu               sync.Mutex
	history          map[string][]time.Duration // toolName → 最近 N 次延迟
	windowSize       int                        // 观察窗口大小 (默认 5)
	latencyThreshold time.Duration              // 延迟阈值 (默认 5s)
	slowThreshold    int                        // 标记阈值 (默认 3/5)
	markedTools      map[string]bool            // 已标记为长任务的工具
}

// NewToolLatencyTracker 创建延迟跟踪器。
func NewToolLatencyTracker() *ToolLatencyTracker {
	return &ToolLatencyTracker{
		history:          make(map[string][]time.Duration),
		windowSize:       5,
		latencyThreshold: 5 * time.Second,
		slowThreshold:    3,
		markedTools:      make(map[string]bool),
	}
}

// Record 记录一次工具调用延迟。
func (t *ToolLatencyTracker) Record(toolName string, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.history[toolName] = append(t.history[toolName], latency)
	if len(t.history[toolName]) > t.windowSize {
		t.history[toolName] = t.history[toolName][1:]
	}

	// 检查是否应自动标记
	if t.shouldAutoBgtask(toolName) {
		if !t.markedTools[toolName] {
			t.markedTools[toolName] = true
			autoBgtaskLog.Info("自动标记后台任务工具",
				"tool", toolName,
				"recent_latencies", t.history[toolName],
			)
		}
	}
}

// IsMarked 检查工具是否已被标记为后台任务。
func (t *ToolLatencyTracker) IsMarked(toolName string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.markedTools[toolName]
}

// MarkTool 手动标记/取消标记工具。
func (t *ToolLatencyTracker) MarkTool(toolName string, marked bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.markedTools[toolName] = marked
}

// MarkedTools 返回当前已标记的工具列表。
func (t *ToolLatencyTracker) MarkedTools() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	tools := make([]string, 0, len(t.markedTools))
	for name := range t.markedTools {
		tools = append(tools, name)
	}
	return tools
}

func (t *ToolLatencyTracker) shouldAutoBgtask(toolName string) bool {
	history := t.history[toolName]
	if len(history) < t.windowSize {
		return false
	}
	slowCount := 0
	for _, lat := range history {
		if lat > t.latencyThreshold {
			slowCount++
		}
	}
	return slowCount >= t.slowThreshold
}
