package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"JuanNiang-Neo/internal/agent/mcp"
)

// ---------------------------------------------------------------------------
// EinoToolWrapper — 将内部 Tool 适配为 Eino InvokableTool
// ---------------------------------------------------------------------------

var _ tool.InvokableTool = (*EinoToolWrapper)(nil)

// EinoToolWrapper 包装一个 Tool 实现 Eino 的 InvokableTool 接口。
type EinoToolWrapper struct {
	inner Tool
}

// NewEinoToolWrapper 创建 EinoToolWrapper。
func NewEinoToolWrapper(t Tool) *EinoToolWrapper {
	return &EinoToolWrapper{inner: t}
}

// Info 返回 Eino 所需的 ToolInfo，参数描述通过 JSON Schema 转换。
func (w *EinoToolWrapper) Info(_ context.Context) (*schema.ToolInfo, error) {
	params := w.inner.Parameters() // openai.FunctionParameters == map[string]any
	paramsJSON, _ := json.Marshal(params)
	var js jsonschema.Schema
	_ = json.Unmarshal(paramsJSON, &js)

	return &schema.ToolInfo{
		Name:        w.inner.Name(),
		Desc:        w.inner.Description(),
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&js),
	}, nil
}

// InvokableRun 执行内部工具。
func (w *EinoToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	return w.inner.Execute(ctx, json.RawMessage(argumentsInJSON))
}

// IsLongRunning 返回内部工具是否为长时间运行工具。
func (w *EinoToolWrapper) IsLongRunning() bool {
	return w.inner.IsLongRunning()
}

// Inner 返回被包装的内部工具。
func (w *EinoToolWrapper) Inner() Tool {
	return w.inner
}

// ---------------------------------------------------------------------------
// MCPToolWrapper — 将 MCP 工具适配为 Eino InvokableTool
// ---------------------------------------------------------------------------

var _ tool.InvokableTool = (*MCPToolWrapper)(nil)

// MCPGroupCaller 是调用 MCP 工具所需的最小接口，*mcp.MCPGroup 满足此接口。
type MCPGroupCaller interface {
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
}

// MCPToolLister 是列出 MCP 工具所需的最小接口，*mcp.MCPGroup 满足此接口。
type MCPToolLister interface {
	ListTools(ctx context.Context) []mcp.ToolDefinition
}

// MCPToolWrapper 包装一个 MCP 工具定义及其调用入口，实现 Eino InvokableTool。
type MCPToolWrapper struct {
	name        string
	description string
	inputSchema json.RawMessage
	mcpGroup    MCPGroupCaller
}

// NewMCPToolWrapper 创建 MCPToolWrapper。
func NewMCPToolWrapper(name, desc string, inputSchema json.RawMessage, caller MCPGroupCaller) *MCPToolWrapper {
	return &MCPToolWrapper{
		name:        name,
		description: desc,
		inputSchema: inputSchema,
		mcpGroup:    caller,
	}
}

// Info 返回 Eino 所需的 ToolInfo，InputSchema 直接作为 JSON Schema 解析。
func (w *MCPToolWrapper) Info(_ context.Context) (*schema.ToolInfo, error) {
	var js jsonschema.Schema
	if len(w.inputSchema) > 0 {
		_ = json.Unmarshal(w.inputSchema, &js)
	}
	return &schema.ToolInfo{
		Name:        w.name,
		Desc:        w.description,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&js),
	}, nil
}

// InvokableRun 将调用委托给 MCPGroup。
func (w *MCPToolWrapper) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	return w.mcpGroup.CallTool(ctx, w.name, json.RawMessage(argumentsInJSON))
}

// Name 返回工具名称。
func (w *MCPToolWrapper) Name() string {
	return w.name
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// BuildEinoTools 将 ToolRegistry 中的所有内置工具和 MCPGroup 中的所有 MCP 工具
// 合并转换为 Eino InvokableTool 切片。
// mcpGroup 可以为 nil，此时不包含 MCP 工具。
func BuildEinoTools(registry *ToolRegistry, mcpGroup MCPGroupCaller, mcpToolLister MCPToolLister) []tool.InvokableTool {
	var tools []tool.InvokableTool

	// 内置工具
	for _, t := range registry.List() {
		tools = append(tools, NewEinoToolWrapper(t))
	}

	// MCP 工具
	if mcpToolLister != nil && mcpGroup != nil {
		for _, td := range mcpToolLister.ListTools(context.Background()) {
			tools = append(tools, NewMCPToolWrapper(
				td.Name,
				td.Description,
				td.InputSchema,
				mcpGroup,
			))
		}
	}

	return tools
}

// GetEinoToolByName 从已构建的 Eino 工具列表中按名称查找工具。
// 支持查找 EinoToolWrapper（通过 Inner().Name()）和 MCPToolWrapper（通过 Name()）。
func GetEinoToolByName(tools []tool.InvokableTool, name string) (tool.InvokableTool, bool) {
	for _, t := range tools {
		switch w := t.(type) {
		case *EinoToolWrapper:
			if w.Inner().Name() == name {
				return w, true
			}
		case *MCPToolWrapper:
			if w.Name() == name {
				return w, true
			}
		}
	}
	return nil, false
}

// IsEinoToolLongRunning 判断指定的 Eino 工具是否为长时间运行工具。
// 仅 EinoToolWrapper 可能为长时运行，MCP 工具默认不是。
func IsEinoToolLongRunning(t tool.InvokableTool) bool {
	if w, ok := t.(*EinoToolWrapper); ok {
		return w.IsLongRunning()
	}
	return false
}

// Compile-time assertion: *mcp.MCPGroup satisfies both MCPGroupCaller and MCPToolLister.
var (
	_ MCPGroupCaller = (*mcp.MCPGroup)(nil)
	_ MCPToolLister  = (*mcp.MCPGroup)(nil)
)

// Ensure fmt is used.
var _ = fmt.Sprintf
