package agent

import (
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/provider"
)

type HagoCenter struct {
	Pvd provider.ProviderGroup
	Mcp mcp.MCPGroup
}
