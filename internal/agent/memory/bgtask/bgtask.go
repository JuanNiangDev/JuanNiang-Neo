package bgtask

import (
	"encoding/json"
	"sync"
)

// BackGroundTaskMetaInfo 后台任务元信息。
type BackGroundTaskMetaInfo struct {
	Running  bool            `json:"running"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// TaskResult 后台任务步骤执行结果。
type TaskResult struct {
	TaskID     string `json:"task_id"`
	StepID     string `json:"step_id"`
	ChatAreaID string `json:"chat_area_id"`
	Status     string `json:"status"` // running/done/failed
	Result     string `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BackGroundTaskMemory 管理后台任务的内存状态 + 结果缓冲管道。
type BackGroundTaskMemory struct {
	mu         sync.RWMutex
	taskMemory map[string]BackGroundTaskMetaInfo
	resultCh   chan TaskResult
}

func New() *BackGroundTaskMemory {
	return &BackGroundTaskMemory{
		taskMemory: make(map[string]BackGroundTaskMetaInfo),
		resultCh:   make(chan TaskResult, 256),
	}
}

func (b *BackGroundTaskMemory) Add(meta BackGroundTaskMetaInfo) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := string(meta.Metadata)
	b.taskMemory[key] = meta
	return key
}

func (b *BackGroundTaskMemory) Del(taskID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.taskMemory, taskID)
}

func (b *BackGroundTaskMemory) Get(taskID string) (BackGroundTaskMetaInfo, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	meta, ok := b.taskMemory[taskID]
	return meta, ok
}

func (b *BackGroundTaskMemory) List() map[string]BackGroundTaskMetaInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]BackGroundTaskMetaInfo, len(b.taskMemory))
	for k, v := range b.taskMemory {
		out[k] = v
	}
	return out
}

func (b *BackGroundTaskMemory) SetRunning(taskID string, running bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if meta, ok := b.taskMemory[taskID]; ok {
		meta.Running = running
		b.taskMemory[taskID] = meta
	}
}

// ResultChan 返回结果缓冲管道，供排水 Agent 消费。
func (b *BackGroundTaskMemory) ResultChan() <-chan TaskResult {
	return b.resultCh
}

// PushResult 向缓冲管道推送结果，非阻塞。
func (b *BackGroundTaskMemory) PushResult(r TaskResult) {
	select {
	case b.resultCh <- r:
	default:
	}
}
