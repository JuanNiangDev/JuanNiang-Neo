package tool

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
)

type ToolConfig struct {
	ID          string
	Name        string
	Description string
	Parameters  openai.FunctionParameters
	Executor    func(ctx context.Context, args json.RawMessage) (string, error)
	Timeout     *time.Duration
}

type Tool interface {
}

type ToolGroup struct {
	mu    sync.Mutex
	Tools []Tool
}

func NewToolGroup() *ToolGroup {
	return &ToolGroup{
		mu:    sync.Mutex{},
		Tools: make([]Tool, 0),
	}
}
