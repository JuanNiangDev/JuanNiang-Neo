package agent

import (
	"context"
	"fmt"

	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/pluggin"
)

// _ implements pluggin.AgentOperator
var _ pluggin.AgentOperator = (*HagoCenter)(nil)

func (h *HagoCenter) SetProviderActive(ctx context.Context, id string, active bool) error {
	if err := h.DAO.Provider.SetActive(ctx, id, active); err != nil {
		return err
	}
	if active {
		p, err := h.DAO.Provider.GetByID(ctx, id)
		if err != nil {
			return err
		}
		h.Providers.AddProvider(provider.NewProvider(provider.ProviderConfig{
			ID:          p.ID,
			Name:        p.Name,
			Type:        provider.ModelType(p.Type),
			Endpoint:    p.Endpoint,
			Token:       p.Token,
			Model:       p.Model,
			Temperature: p.Temperature,
		}))
	} else {
		h.Providers.DelProvider(id)
	}
	return nil
}

func (h *HagoCenter) SetMCPActive(ctx context.Context, id string, active bool) error {
	if err := h.DAO.MCPServer.SetActive(ctx, id, active); err != nil {
		return err
	}
	if !active {
		if m, ok := h.MCP.GetMCP(id); ok {
			m.Disconnect()
			h.MCP.DelMCP(id)
		}
	}
	return nil
}

func (h *HagoCenter) CompactMemory(ctx context.Context, chatAreaID string) error {
	if h.Memory == nil {
		return fmt.Errorf("memory 未初始化")
	}
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return fmt.Errorf("无可用 Text 模型")
	}
	return h.Memory.CompactShortTermMemory(ctx, llm)
}

func (h *HagoCenter) GetChatAreaID(userID, groupID int64, messageType string) string {
	var areaType models.AreaType
	var targetID int64
	if messageType == "private" {
		areaType = models.AreaTypePrivate
		targetID = userID
	} else {
		areaType = models.AreaTypeGroup
		targetID = groupID
	}
	area, err := h.DAO.ChatArea.GetOrCreate(context.Background(), areaType, targetID)
	if err != nil {
		return ""
	}
	return area.ID
}

// GetProviderGroup 返回 Provider 访问接口。
func (h *HagoCenter) GetProviderGroup() pluggin.ProviderGroupAccess {
	return &providerGroupAccess{h: h}
}

// GetMCPGroup 返回 MCP 访问接口。
func (h *HagoCenter) GetMCPGroup() pluggin.MCPGroupAccess {
	return &mcpGroupAccess{h: h}
}

// ---------- ProviderGroupAccess ----------

type providerGroupAccess struct{ h *HagoCenter }

func (p *providerGroupAccess) List() []pluggin.ProviderInfo {
	list := p.h.Providers.ListProviders()
	out := make([]pluggin.ProviderInfo, 0, len(list))
	for _, pr := range list {
		out = append(out, pluggin.ProviderInfo{
			ID:   pr.ID(),
			Name: pr.Name(),
			Type: string(pr.Type()),
		})
	}
	return out
}

func (p *providerGroupAccess) GetActive(id string) bool {
	_, ok := p.h.Providers.GetProvider(id)
	return ok
}

// ---------- MCPGroupAccess ----------

type mcpGroupAccess struct{ h *HagoCenter }

func (m *mcpGroupAccess) ListMCPs() []pluggin.MCPInfo {
	list := m.h.MCP.ListMCPs()
	out := make([]pluggin.MCPInfo, 0, len(list))
	for _, mc := range list {
		out = append(out, pluggin.MCPInfo{
			ID:     mc.ID(),
			Name:   mc.Name(),
			Active: mc.IsConnected(),
		})
	}
	return out
}

func (m *mcpGroupAccess) IsConnected(id string) bool {
	mcp, ok := m.h.MCP.GetMCP(id)
	if !ok {
		return false
	}
	return mcp.IsConnected()
}
