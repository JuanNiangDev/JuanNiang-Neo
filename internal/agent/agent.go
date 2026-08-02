package agent

import (
	"context"
	"fmt"

	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/cronjob"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/memory/longterm"
	"JuanNiang-Neo/internal/agent/memory/shortterm"
	"JuanNiang-Neo/internal/agent/memory/skillmem"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/session"
	"JuanNiang-Neo/internal/agent/skill"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/cache"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/pluggin"

	"github.com/cloudwego/eino/adk"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

// HagoCenter 是 Agent 系统的中央调度器，聚合所有子模块。
type HagoCenter struct {
	Adapter        *adapter.Adapter
	WebhookAdapter *adapter.WebhookAdapter
	Providers      *provider.ProviderGroup
	MCP            *mcp.MCPGroup
	Memory         *memory.MemoryGroup
	Prompt         *prompt.PromptManager
	Session        *session.SessionManager
	Tools          *tool.ToolRegistry
	Skills         *skill.SkillEngine
	ACL            *acl.ACL
	DAO            *dao.Bundle

	// T2I 和 Sandbox 运行时客户端（可通过 API 热更新）
	SandboxClient *sandboxcaller.Client
	T2IClient     *t2icaller.Client

	Concurrency    *ConcurrencyManager
	CronJobManager *cronjob.Manager
	CronJobEvents  chan adapter.Event // CronJob → 主 Agent 事件循环
	PluginEngine   *pluggin.PluginEngine

	// SelfID 和 SelfNickname 从 Adapter 获取后缓存
	SelfQQ       int64
	SelfNickname string

	// EinoAgent 是 Eino ADK 的 ChatModelAgent，替代手写的 ReAct 循环。
	EinoAgent *adk.ChatModelAgent
}

// Config HagoCenter 初始化配置。
type Config struct {
	Adapter        *adapter.Adapter
	WebhookAdapter *adapter.WebhookAdapter
	Sandbox        *sandboxcaller.Client
	T2I            *t2icaller.Client
	Providers      *provider.ProviderGroup
	MCPGroup       *mcp.MCPGroup
	DAO            *dao.Bundle
	ACL            *acl.ACL
	Cache          *cache.Cache
}

// NewHagoCenter 创建并初始化 HagoCenter。
func NewHagoCenter() *HagoCenter {
	return &HagoCenter{
		Providers:     provider.NewProviderGroup(),
		MCP:           mcp.NewMCPGroup(),
		Tools:         tool.NewToolRegistry(),
		Skills:        skill.NewSkillEngine(),
		CronJobEvents: make(chan adapter.Event, 64),
	}
}

// Init 从 DB 加载配置并初始化所有子模块。
func (h *HagoCenter) Init(ctx context.Context, cfg Config) error {
	h.Adapter = cfg.Adapter
	h.WebhookAdapter = cfg.WebhookAdapter
	h.DAO = cfg.DAO
	h.ACL = cfg.ACL
	h.Providers = cfg.Providers
	h.MCP = cfg.MCPGroup

	// 缓存机器人自己的 QQ 号和昵称
	h.SelfQQ = h.Adapter.SelfID()
	if info, err := h.Adapter.GetLoginInfo(); err == nil && info != nil {
		h.SelfNickname = info.Nickname
	}
	log.Info("机器人身份信息", "self_qq", h.SelfQQ, "self_nickname", h.SelfNickname)

	// 存储 T2I/Sandbox 运行时客户端
	h.SandboxClient = cfg.Sandbox
	h.T2IClient = cfg.T2I

	// Session 管理器: 同时维护 Postgres Session 表 + ChatRecord 表 + Redis (历史路径)
	h.Session = session.NewSessionManager(cfg.DAO.Session, cfg.DAO.ChatRecord, cfg.Cache)

	// Memory 组: 短期记忆 (Redis) + 长期记忆 (Postgres + 内存 HotArea)
	stConf := shortterm.Config{WindowSize: 100, AutoCompact: true}
	ltConf := longterm.Config{HotAreaSize: 10}
	st := shortterm.New(stConf, cfg.Cache)
	lt := longterm.New(ltConf, cfg.DAO.LongTermMemItem)
	sm := skillmem.New(cfg.DAO.SkillMemory)
	if err := sm.Warmup(ctx); err != nil {
		log.Warn("技能记忆预热失败", "err", err)
	}
	h.Memory = memory.NewMemoryGroup(st, lt, sm)
	// 设置 LLM Provider 供 Compact 中的技能记忆更新使用
	h.Memory.LLMProvider = h.Providers.SelectModel(provider.ModelTypeText)

	h.Prompt = prompt.NewPromptManager(cfg.DAO.Prompt)
	h.Skills = skill.NewSkillEngine()

	// 启动时种子系统锁定提示词（幂等：已存在则同步内容）
	if err := h.Prompt.EnsureSystemPrompt(ctx); err != nil {
		log.Warn("系统锁定提示词种子失败", "err", err)
	}

	// 使用函数 getter 注册工具，支持运行时客户端热更新
	// 注意：getSessionCtx / getCurrentMsg / getRecentMsgs 从 context 中读取 per-message 状态，
	// 避免 HagoCenter 共享字段导致的数据竞争。
	tool.RegisterBuiltinTools(h.Tools, cfg.Adapter,
		func() *sandboxcaller.Client { return h.SandboxClient },
		func() *t2icaller.Client { return h.T2IClient },
		h.Providers.SelectModel(provider.ModelTypeImage),
		func(ctx context.Context) string {
			if sc := GetMsgSessionCtx(ctx); sc != nil {
				return sc.SessionCtxStr
			}
			return ""
		},
		func(ctx context.Context) *adapter.MessageEvent {
			if sc := GetMsgSessionCtx(ctx); sc != nil {
				return sc.Msg
			}
			return nil
		},
		func(ctx context.Context, msgType string, targetID int64, limit int) ([]string, error) {
			return h.getRecentMessagesByMsgType(ctx, msgType, targetID, limit)
		},
	)

	if err := h.loadProviders(ctx); err != nil {
		return err
	}
	if err := h.loadMCPs(ctx); err != nil {
		return err
	}
	if err := h.loadSkills(ctx); err != nil {
		return err
	}

	h.Concurrency = NewConcurrencyManager(8)
	h.CronJobManager = cronjob.New(h.DAO.CronJob, h.CronJobEvents)

	// 构建 Eino ChatModelAgent（替代手写的 ReAct 循环）
	if err := h.buildEinoAgent(ctx); err != nil {
		log.Warn("Eino Agent 构建失败，将回退到旧模式", "err", err)
	}

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
	log.Info("Provider 加载完成", "count", len(list))
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
			log.Error("MCP 连接失败", "name", srv.Name, "err", err)
			continue
		}
		h.MCP.AddMCP(client)
	}
	log.Info("MCP 加载完成", "count", len(list))
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
			PromptRefs:   s.PromptRefs,
			ToolRefs:     s.ToolRefs,
			McpRefs:      s.McpRefs,
			IsActive:     s.IsActive,
			IsSystem:     s.IsSystem,
			Priority:     s.Priority,
		})
	}
	log.Info("Skill 加载完成", "count", len(list))
	return nil
}

// buildEinoAgent 构建 Eino ChatModelAgent（替代手写的 ReAct 循环）。
// 在 Init 末尾调用，要求 Providers 和 Tools 已就绪。
func (h *HagoCenter) buildEinoAgent(ctx context.Context) error {
	// 1. 选取文本模型并包装为 Eino 适配器
	llm := h.Providers.SelectModel(provider.ModelTypeText)
	if llm == nil {
		return fmt.Errorf("无可用 Text 模型，无法构建 Eino Agent")
	}
	modelAdapter := provider.NewEinoModelAdapter(llm)

	// 2. 将内置工具 + MCP 工具转换为 Eino InvokableTool
	einoInvTools := tool.BuildEinoTools(h.Tools, h.MCP, h.MCP)

	// 类型转换: []tool.InvokableTool → []tool.BaseTool (Eino 要求)
	einoBaseTools := make([]einotool.BaseTool, len(einoInvTools))
	for i, t := range einoInvTools {
		einoBaseTools[i] = t
	}

	// 3. 创建 Agent（Instruction 为空，每条消息由 middleware.BeforeAgent 动态注入）
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "juan-niang-neo",
		Description: "QQ 群聊 AI 助手",
		Model:       modelAdapter,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: einoBaseTools,
			},
		},
		MaxIterations: 20,
		Handlers: []adk.ChatModelAgentMiddleware{
			&JuanNiangMiddleware{h: h},
		},
	})
	if err != nil {
		return fmt.Errorf("创建 Eino ChatModelAgent 失败: %w", err)
	}

	h.EinoAgent = agent
	log.Info("Eino ChatModelAgent 已就绪", "tools", len(einoBaseTools))
	return nil
}

// Start 启动 Agent 系统 (事件循环 + CronJob 调度器)。
func (h *HagoCenter) Start(ctx context.Context) error {
	go h.runEventLoop(ctx)
	go h.CronJobManager.Run(ctx)
	log.Info("HagoCenter 已启动")
	return nil
}

// buildToolList 构建完整的工具列表（注册工具 + MCP 工具），供 LLM 使用。
func (h *HagoCenter) buildToolList(ctx context.Context) []provider.ToolDef {
	tools := h.Tools.GetOpenAITools()
	if h.MCP != nil {
		for _, t := range h.MCP.ListTools(ctx) {
			tools = append(tools, provider.ToolDef{
				Type: "function",
				Function: provider.ToolDefFunc{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
	}
	return tools
}

// Stop 停止 Agent 系统。
func (h *HagoCenter) Stop() {
	log.Info("HagoCenter 已停止")
}
