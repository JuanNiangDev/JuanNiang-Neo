package mcp

import (
	"sync"
	"time"
)

type McpSSEConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	ServerURL     string            `json:"server_url"`
	Headers       map[string]string `json:"headers,omitempty"`
	Timeout       time.Duration     `json:"timeout"`
	RetryCount    int               `json:"retry_count"`
	ToolFilter    []string          `json:"tool_filter,omitempty"`
	AutoReconnect bool              `json:"auto_reconnect"`
}

type MCP interface {
}

type MCPGroup struct {
	mu   sync.Mutex
	MCPs []MCP
}

func NewMCPGroup() *MCPGroup {
	return &MCPGroup{
		mu:   sync.Mutex{},
		MCPs: make([]MCP, 0),
	}
}
