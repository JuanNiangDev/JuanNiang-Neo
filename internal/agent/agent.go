package agent

import (
	"context"
	"log/slog"

	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/session"
	"JuanNiang-Neo/internal/agent/skill"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/pluggin"
)

// HagoCenter 是 Agent 系统的中央调度器，聚合所有子模块。
type HagoCenter struct {
	Adapter   *adapter.Adapter
	Providers *provider.ProviderGroup
	MCP       *mcp.MCPGroup
	Memory    *memory.MemoryGroup
	Prompt    *prompt.PromptManager
	Session   *session.SessionManager
	Tools     *tool.ToolRegistry
	Skills    *skill.SkillEngine
	ACL       *acl.ACL
	DAO       *dao.Bundle

	// T2I 和 Sandbox 运行时客户端（可通过 API 热更新）
	SandboxClient *sandboxcaller.Client
	T2IClient     *t2icaller.Client

	BgTaskExecutor *BackgroundTaskExecutor
	Drainer        *DrainerAgent
	OutputChan     chan DrainerOutput
	PluginEngine   *pluggin.PluginEngine
}

// Config HagoCenter 初始化配置。
type Config struct {
	Adapter   *adapter.Adapter
	Sandbox   *sandboxcaller.Client
	T2I       *t2icaller.Client
	Providers *provider.ProviderGroup
	MCPGroup  *mcp.MCPGroup
	DAO       *dao.Bundle
	ACL       *acl.ACL
	Cache     interface {
		Client() interface{}
	}
}

// NewHagoCenter 创建并初始化 HagoCenter。
func NewHagoCenter() *HagoCenter {
	return &HagoCenter{
		Providers:  provider.NewProviderGroup(),
		MCP:        mcp.NewMCPGroup(),
		Tools:      tool.NewToolRegistry(),
		Skills:     skill.NewSkillEngine(),
		OutputChan: make(chan DrainerOutput, 128),
	}
}

// Init 从 DB 加载配置并初始化所有子模块。
func (h *HagoCenter) Init(ctx context.Context, cfg Config) error {
	h.Adapter = cfg.Adapter
	h.DAO = cfg.DAO
	h.ACL = cfg.ACL
	h.Providers = cfg.Providers
	h.MCP = cfg.MCPGroup

	// 存储 T2I/Sandbox 运行时客户端
	h.SandboxClient = cfg.Sandbox
	h.T2IClient = cfg.T2I

	h.Session = session.NewSessionManager(cfg.DAO.Session, nil)
	h.Prompt = prompt.NewPromptManager(cfg.DAO.Prompt)
	h.Skills = skill.NewSkillEngine()

	// 使用函数 getter 注册工具，支持运行时客户端热更新
	tool.RegisterBuiltinTools(h.Tools, cfg.Adapter,
		func() *sandboxcaller.Client { return h.SandboxClient },
		func() *t2icaller.Client { return h.T2IClient },
		h.Providers.SelectModel(provider.ModelTypeImage))

	if err := h.loadProviders(ctx); err != nil {
		return err
	}
	if err := h.loadMCPs(ctx); err != nil {
		return err
	}
	if err := h.loadSkills(ctx); err != nil {
		return err
	}

	h.BgTaskExecutor = NewBackgroundTaskExecutor(h.Tools, h.DAO.BackgroundTask, h.OutputChan)
	h.Drainer = NewDrainerAgent(h.OutputChan, h.Providers, h.Adapter, h.Session, h.Prompt, h.Memory)

	return nil
}

func (h *HagoCenter) loadProviders(ctx context.Context) error {
	list, err := h.DAO.Provider.List(ctx, "")
	if err != nil {
		return err
	}
	for _, p := range list {
		if !p.IsActive {
			continue
		}
		pr := provider.NewProvider(provider.ProviderConfig{
			ID:          p.ID,
			Name:        p.Name,
			Type:        provider.ModelType(p.Type),
			Endpoint:    p.Endpoint,
			Token:       p.Token,
			Model:       p.Model,
			Temperature: p.Temperature,
		})
		h.Providers.AddProvider(pr)
	}
	slog.Info("Provider 加载完成", "count", len(list))
	return nil
}

func (h *HagoCenter) loadMCPs(ctx context.Context) error {
	list, err := h.DAO.MCPServer.List(ctx)
	if err != nil {
		return err
	}
	for _, srv := range list {
		if !srv.IsActive {
			continue
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
			continue
		}
		h.MCP.AddMCP(client)
	}
	slog.Info("MCP 加载完成", "count", len(list))
	return nil
}

func (h *HagoCenter) loadSkills(ctx context.Context) error {
	list, err := h.DAO.Skill.List(ctx)
	if err != nil {
		return err
	}
	for _, s := range list {
		h.Skills.AddSkill(&skill.SkillConfig{
			ID:           s.ID,
			Name:         s.Name,
			Description:  s.Description,
			Keywords:     s.Keywords,
			RegexPattern: s.RegexPattern,
			PromptRef:    s.PromptRef,
			ToolRefs:     s.ToolRefs,
			McpRefs:      s.McpRefs,
			IsActive:     s.IsActive,
			IsSystem:     s.IsSystem,
			Priority:     s.Priority,
		})
	}
	slog.Info("Skill 加载完成", "count", len(list))
	return nil
}

// Start 启动 Agent 系统 (后台任务执行器 + 排水 Agent + 事件循环)。
func (h *HagoCenter) Start(ctx context.Context) error {
	go h.BgTaskExecutor.Run(ctx)
	go h.Drainer.Run(ctx)
	go h.runEventLoop(ctx)
	slog.Info("HagoCenter 已启动")
	return nil
}

// Stop 停止 Agent 系统。
func (h *HagoCenter) Stop() {
	close(h.OutputChan)
	slog.Info("HagoCenter 已停止")
}
