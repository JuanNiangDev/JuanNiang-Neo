package mcp

func (m *MCPGroup) AddMCP(conf *McpSSEConfig) error {}

func (m *MCPGroup) DelMCP(mcpID string) error {}

func (m *MCPGroup) GetMCP(mcpID string) (MCP, error) {}

func (m *MCPGroup) ListMCP(mcpID string) ([]MCP, error) {}

func (m *MCPGroup) EditMCPSSEConfig(mcpID string, conf *McpSSEConfig) error {}
