package agent

import (
	"crypto/rand"
	"fmt"
	"sort"
	"sync"
	"time"
)

// AgentLoop 一条正在执行的 Agent ReAct 循环（监控展示用）。
type AgentLoop struct {
	ID          string    `json:"id"`
	ChatAreaID  string    `json:"chat_area_id"`
	MessageType string    `json:"message_type"` // private / group
	TargetID    int64     `json:"target_id"`    // 私聊: user_id; 群聊: group_id
	UserID      int64     `json:"user_id"`
	UserMsg     string    `json:"user_msg"`     // 用户原始请求
	CurrentTool string    `json:"current_tool"` // 当前正在执行的工具；空表示思考/生成中
	StartedAt   time.Time `json:"started_at"`   // 循环开始时间
}

// LoopTracker 跟踪当前活跃的 Agent ReAct 循环。
// 仅内存态（用于监控展示），不持久化；进程重启后清空。
type LoopTracker struct {
	mu    sync.RWMutex
	loops map[string]*AgentLoop
}

func NewLoopTracker() *LoopTracker {
	return &LoopTracker{loops: make(map[string]*AgentLoop)}
}

// Register 注册一条新循环，返回循环 ID。
func (t *LoopTracker) Register(l *AgentLoop) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if l.ID == "" {
		l.ID = newLoopID()
	}
	if l.StartedAt.IsZero() {
		l.StartedAt = time.Now()
	}
	t.loops[l.ID] = l
	return l.ID
}

// UpdateTool 更新指定循环当前正在执行的工具名。
func (t *LoopTracker) UpdateTool(loopID, tool string) {
	if t == nil || loopID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if l, ok := t.loops[loopID]; ok {
		l.CurrentTool = tool
	}
}

// Unregister 移除一条已结束的循环。
func (t *LoopTracker) Unregister(loopID string) {
	if t == nil || loopID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.loops, loopID)
}

// List 返回当前所有活跃循环（按开始时间升序）。
func (t *LoopTracker) List() []AgentLoop {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]AgentLoop, 0, len(t.loops))
	for _, l := range t.loops {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

// newLoopID 生成循环 ID（时间戳 + 随机字节，保证同 ChatArea 内多循环不冲突）。
func newLoopID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x", time.Now().UnixNano(), b)
}
