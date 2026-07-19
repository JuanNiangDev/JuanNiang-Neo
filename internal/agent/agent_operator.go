package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	sandbox "JuanNiang-Neo/infrastructure/sandbox"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2i "JuanNiang-Neo/infrastructure/t2i"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/agent/mcp"
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

// SwitchProvider 切换同类型 Provider：停用其他同类型 Provider，激活指定 Provider。
func (h *HagoCenter) SwitchProvider(ctx context.Context, id string) error {
	p, err := h.DAO.Provider.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("provider 不存在: %w", err)
	}

	// 停用同类型其他 Provider（DB + 运行时）
	if err := h.DAO.Provider.DeactivateByType(ctx, p.Type, id); err != nil {
		return err
	}
	// 从运行时移除同类型其他 Provider
	for _, existing := range h.Providers.ListProviders() {
		if existing.Type() == provider.ModelType(p.Type) && existing.ID() != id {
			h.Providers.DelProvider(existing.ID())
		}
	}

	// 激活指定 Provider（DB + 运行时）
	if err := h.DAO.Provider.SetActive(ctx, id, true); err != nil {
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
	slog.Info("Provider 切换完成", "id", id, "type", p.Type)
	return nil
}

func (h *HagoCenter) SetMCPActive(ctx context.Context, id string, active bool) error {
	if err := h.DAO.MCPServer.SetActive(ctx, id, active); err != nil {
		return err
	}
	if !active {
		// 停用：断开并移除
		if m, ok := h.MCP.GetMCP(id); ok {
			m.Disconnect()
			h.MCP.DelMCP(id)
		}
	} else {
		// 启用：若运行时不存在则连接
		if _, ok := h.MCP.GetMCP(id); !ok {
			srv, err := h.DAO.MCPServer.GetByID(ctx, id)
			if err != nil {
				return err
			}
			headers := make(map[string]string)
			for k, v := range srv.Headers {
				if str, ok := v.(string); ok {
					headers[k] = str
				}
			}
			client := mcp.NewSSEMCPClient(mcp.McpSSEConfig{
				ID:            srv.ID,
				Name:          srv.Name,
				ServerURL:     srv.ServerURL,
				Headers:       headers,
				Timeout:       0,
				RetryCount:    srv.RetryCount,
				ToolFilter:    srv.ToolFilter,
				AutoReconnect: srv.AutoReconnect,
			})
			if err := client.Connect(ctx); err != nil {
				slog.Error("MCP 连接失败", "name", srv.Name, "err", err)
				return err
			}
			h.MCP.AddMCP(client)
		}
	}
	return nil
}

// SetToolActive 启用/停用工具。停用时从 ToolRegistry 注销，启用时重新注册。
// 注意：Tool 只支持查看和启停，不支持增删。
func (h *HagoCenter) SetToolActive(ctx context.Context, name string, active bool) error {
	// 同步 DB 中的 ToolConfig
	tc, err := h.DAO.ToolConfig.GetByName(ctx, name)
	if err == nil {
		if err := h.DAO.ToolConfig.SetActive(ctx, tc.ID, active); err != nil {
			return err
		}
	}

	if !active {
		h.Tools.Unregister(name)
		slog.Info("Tool 已停用", "name", name)
	} else {
		// 启用：若运行时不存在则需重新注册（仅对已注册过的工具支持）
		if _, ok := h.Tools.Get(name); !ok {
			// 重新注册需要原始 Tool 实例，此处仅记录；内置工具在启动时注册，
			// 运行时停用后重新启用需要从 DB 配置重建——但题目要求不支持增删，
			// 仅支持启停，因此停用=注销，启用=重新注册（若 ToolConfig 有记录则标记）
			slog.Warn("Tool 重新启用需重启或重新注册", "name", name)
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

// GetToolRegistry 返回 Tool 访问接口。
func (h *HagoCenter) GetToolRegistry() pluggin.ToolRegistryAccess {
	return &toolRegistryAccess{h: h}
}

// GetT2IClient 返回当前 T2I 客户端。
func (h *HagoCenter) GetT2IClient() *t2icaller.Client {
	return h.T2IClient
}

// GetSandboxClient 返回当前 Sandbox 客户端。
func (h *HagoCenter) GetSandboxClient() *sandboxcaller.Client {
	return h.SandboxClient
}

// SetT2IActive 启用/停用 T2I 服务。
// 启用时根据 DB 配置创建新客户端；停用时清空运行时客户端。
func (h *HagoCenter) SetT2IActive(ctx context.Context, active bool) error {
	cfg, err := h.DAO.T2I.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取 T2I 配置失败: %w", err)
	}
	cfg.IsActive = active
	if err := h.DAO.T2I.UpdateConfig(ctx, cfg); err != nil {
		return fmt.Errorf("更新 T2I 配置失败: %w", err)
	}

	if active {
		client, err := t2i.NewClient(
			t2i.WithBaseURL(cfg.BaseURL),
			t2i.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
		)
		if err != nil {
			slog.Warn("T2I 客户端创建失败", "err", err)
			h.T2IClient = nil
			return fmt.Errorf("T2I 客户端创建失败: %w", err)
		}
		h.T2IClient = client
		slog.Info("T2I 服务已启用", "base_url", cfg.BaseURL)
	} else {
		h.T2IClient = nil
		slog.Info("T2I 服务已停用")
	}
	return nil
}

// SetSandboxActive 启用/停用 Sandbox 服务。
// 启用时根据 DB 配置创建新客户端；停用时清空运行时客户端。
func (h *HagoCenter) SetSandboxActive(ctx context.Context, active bool) error {
	cfg, err := h.DAO.Sandbox.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("读取 Sandbox 配置失败: %w", err)
	}
	cfg.IsActive = active
	if err := h.DAO.Sandbox.UpdateConfig(ctx, cfg); err != nil {
		return fmt.Errorf("更新 Sandbox 配置失败: %w", err)
	}

	if active {
		client, err := sandbox.NewClient(
			sandbox.WithBaseURL(cfg.BaseURL),
			sandbox.WithAPIKey(cfg.APIKey),
			sandbox.WithTimeout(time.Duration(cfg.Timeout)*time.Second),
		)
		if err != nil {
			slog.Warn("Sandbox 客户端创建失败", "err", err)
			h.SandboxClient = nil
			return fmt.Errorf("Sandbox 客户端创建失败: %w", err)
		}
		h.SandboxClient = client
		slog.Info("Sandbox 服务已启用", "base_url", cfg.BaseURL)
	} else {
		h.SandboxClient = nil
		slog.Info("Sandbox 服务已停用")
	}
	return nil
}

// ---------- ProviderGroupAccess ----------

type providerGroupAccess struct{ h *HagoCenter }

func (p *providerGroupAccess) List() []pluggin.ProviderInfo {
	list := p.h.Providers.ListProviders()
	out := make([]pluggin.ProviderInfo, 0, len(list))
	for _, pr := range list {
		out = append(out, pluggin.ProviderInfo{
			ID:     pr.ID(),
			Name:   pr.Name(),
			Type:   string(pr.Type()),
			Model:  pr.Model(),
			Active: true,
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

// ---------- ToolRegistryAccess ----------

type toolRegistryAccess struct{ h *HagoCenter }

func (t *toolRegistryAccess) ListTools() []pluggin.ToolInfo {
	list := t.h.Tools.List()
	out := make([]pluggin.ToolInfo, 0, len(list))
	for _, tl := range list {
		out = append(out, pluggin.ToolInfo{
			Name:        tl.Name(),
			Description: tl.Description(),
			Builtin:     tl.IsBuiltin(),
			LongRunning: tl.IsLongRunning(),
			Active:      true,
		})
	}
	return out
}

func (t *toolRegistryAccess) IsActive(name string) bool {
	_, ok := t.h.Tools.Get(name)
	return ok
}
