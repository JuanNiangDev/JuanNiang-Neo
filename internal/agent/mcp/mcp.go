package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------- 配置 ----------

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

// ---------- 接口 ----------

type MCP interface {
	ID() string
	Name() string
	Connect(ctx context.Context) error
	Disconnect() error
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	IsConnected() bool
}

// ---------- 工具定义 ----------

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ---------- 容器 ----------

type MCPGroup struct {
	mu   sync.RWMutex
	mcps map[string]MCP
}

func NewMCPGroup() *MCPGroup {
	return &MCPGroup{
		mcps: make(map[string]MCP),
	}
}

func (g *MCPGroup) AddMCP(m MCP) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mcps[m.ID()] = m
}

func (g *MCPGroup) DelMCP(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.mcps, id)
}

func (g *MCPGroup) GetMCP(id string) (MCP, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	m, ok := g.mcps[id]
	return m, ok
}

func (g *MCPGroup) ListMCPs() []MCP {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := make([]MCP, 0, len(g.mcps))
	for _, m := range g.mcps {
		list = append(list, m)
	}
	return list
}

// ListTools 列出所有已连接 MCP 的工具定义。
func (g *MCPGroup) ListTools(ctx context.Context) []ToolDefinition {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var all []ToolDefinition
	for _, m := range g.mcps {
		if !m.IsConnected() {
			continue
		}
		tools, err := m.ListTools(ctx)
		if err != nil {
			slog.Error("MCP ListTools 失败", "mcp", m.Name(), "err", err)
			continue
		}
		all = append(all, tools...)
	}
	return all
}

// HasTool 检查是否存在指定名称的 MCP 工具。
func (g *MCPGroup) HasTool(ctx context.Context, name string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, m := range g.mcps {
		if !m.IsConnected() {
			continue
		}
		tools, err := m.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if t.Name == name {
				return true
			}
		}
	}
	return false
}

// CallTool 按工具名分发调用到拥有该工具的 MCP 服务器。
func (g *MCPGroup) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, m := range g.mcps {
		if !m.IsConnected() {
			continue
		}
		tools, err := m.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if t.Name == name {
				return m.CallTool(ctx, name, args)
			}
		}
	}
	return "", fmt.Errorf("MCP tool %q not found", name)
}

// ---------- SDK 客户端封装 ----------

type sdkMCPClient struct {
	cfg McpSSEConfig
	cli *mcpclient.Client
	mu  sync.Mutex
}

func NewSSEMCPClient(cfg McpSSEConfig) MCP {
	return &sdkMCPClient{cfg: cfg}
}

func (c *sdkMCPClient) ID() string   { return c.cfg.ID }
func (c *sdkMCPClient) Name() string { return c.cfg.Name }

func (c *sdkMCPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cli != nil {
		return nil
	}

	opts := []transport.ClientOption{}
	if len(c.cfg.Headers) > 0 {
		opts = append(opts, mcpclient.WithHeaders(c.cfg.Headers))
	}

	cli, err := mcpclient.NewSSEMCPClient(c.cfg.ServerURL, opts...)
	if err != nil {
		return err
	}

	// SSE 传输层必须先 Start 再 Initialize
	if err := cli.Start(ctx); err != nil {
		cli.Close()
		return err
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "JuanNiang-Neo",
		Version: "1.0.0",
	}

	_, err = cli.Initialize(ctx, initReq)
	if err != nil {
		cli.Close()
		return err
	}

	c.cli = cli
	slog.Info("MCP 连接成功", "name", c.cfg.Name, "url", c.cfg.ServerURL)
	return nil
}

func (c *sdkMCPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cli != nil {
		c.cli.Close()
		c.cli = nil
	}
	return nil
}

func (c *sdkMCPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cli != nil
}

func (c *sdkMCPClient) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cli == nil {
		return nil, nil
	}

	result, err := c.cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	var tools []ToolDefinition
	for _, t := range result.Tools {
		td := ToolDefinition{
			Name:        t.Name,
			Description: t.Description,
		}
		if t.RawInputSchema != nil {
			td.InputSchema = t.RawInputSchema
		} else {
			schema, _ := json.Marshal(t.InputSchema)
			td.InputSchema = schema
		}
		tools = append(tools, td)
	}
	return tools, nil
}

func (c *sdkMCPClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cli == nil {
		return "", nil
	}

	var arguments map[string]any
	if len(args) > 0 {
		json.Unmarshal(args, &arguments)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = arguments

	result, err := c.cli.CallTool(ctx, req)
	if err != nil {
		return "", err
	}

	if result.IsError {
		text := mcp.GetTextFromContent(result.Content)
		return text, nil
	}

	return mcp.GetTextFromContent(result.Content), nil
}
