package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"JuanNiang-Neo/internal/agent/provider"

	"JuanNiang-Neo/internal/logging"
	"github.com/openai/openai-go/v3"
)

var log = logging.NewModule("tool")

// Tool 接口定义所有工具必须实现的方法。
type Tool interface {
	ID() string
	Name() string
	Description() string
	Parameters() openai.FunctionParameters
	Execute(ctx context.Context, args json.RawMessage) (string, error)
	IsBuiltin() bool
	IsLongRunning() bool
}

// ToolRegistry 工具注册表，管理所有已注册的工具。
type ToolRegistry struct {
	mu       sync.RWMutex
	tools    map[string]Tool
	buildIns []string // 内置工具名称列表
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:    make(map[string]Tool),
		buildIns: make([]string, 0),
	}
}

func (tr *ToolRegistry) Register(t Tool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.tools[t.Name()] = t
	if t.IsBuiltin() {
		tr.buildIns = append(tr.buildIns, t.Name())
	}
}

func (tr *ToolRegistry) Unregister(name string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	delete(tr.tools, name)
}

func (tr *ToolRegistry) Get(name string) (Tool, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	t, ok := tr.tools[name]
	return t, ok
}

func (tr *ToolRegistry) List() []Tool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	list := make([]Tool, 0, len(tr.tools))
	for _, t := range tr.tools {
		list = append(list, t)
	}
	return list
}

// GetOpenAITools 返回 OpenAI tool 定义列表。
func (tr *ToolRegistry) GetOpenAITools() []provider.ToolDef {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tools := make([]provider.ToolDef, 0, len(tr.tools))
	for _, t := range tr.tools {
		paramsJSON, _ := json.Marshal(t.Parameters())
		tools = append(tools, provider.ToolDef{
			Type: "function",
			Function: provider.ToolDefFunc{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  json.RawMessage(paramsJSON),
			},
		})
	}
	return tools
}

// Execute 同步执行工具调用。
func (tr *ToolRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	t, ok := tr.Get(name)
	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}

	execCtx := ctx

	start := time.Now()
	log.Info("Tool 执行开始", "tool", name, "args", string(args))

	result, err := t.Execute(execCtx, args)
	elapsed := time.Since(start)
	if err != nil {
		log.Error("Tool 执行失败", "tool", name, "elapsed", elapsed, "err", err)
		return "", err
	}
	log.Info("Tool 执行完成", "tool", name, "elapsed", elapsed, "result_len", len(result))
	return result, nil
}

// IsLongRunning 判断工具是否为长时间运行工具。
func (tr *ToolRegistry) IsLongRunning(name string) bool {
	t, ok := tr.Get(name)
	if !ok {
		return false
	}
	return t.IsLongRunning()
}
