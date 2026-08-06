package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	Loops          *LoopTracker // 当前活跃的 Agent ReAct 循环（监控展示）

	// SelfID 和 SelfNickname 从 Adapter 获取后缓存
	SelfQQ       int64
	SelfNickname string

	// 发送者群内信息缓存（避免每条群消息都调 OneBot11 get_group_member_info）
	memberInfoMu    sync.RWMutex
	memberInfoCache map[string]memberInfoEntry

	// 消息批处理：同一 ChatArea 在短窗口内的消息合并为一次 Agent 处理
	batchMu sync.Mutex
	batches map[string]*pendingBatch

	// 相关性判断结果缓存（Redis，L2.3/L4.2）
	Cache *cache.Cache

	// 工具"仅管理员"名单（DB 驱动，Tools 页可切换）：admin_only=true 的工具
	// 只能由 Admins 列表内用户调用（防提示词注入诱导敏感操作）
	toolAdminOnlyMu sync.RWMutex
	toolAdminOnly   map[string]bool

	// 相关性判断并发闸门（L3.1）：限制全局并发，避免热聊时打爆 provider
	relevanceSem chan struct{}

	// 热聊统计（内存，L2.2/L4.1）：1s 滑动窗口消息计数，用于动态批窗口与刷屏降级
	hotMu    sync.Mutex
	hotStats map[string]*hotStat

	// 知识库 LRU（50 条，缓存对话前检索结果，加速匹配）
	knowledgeLRU *knowledgeLRU

	// EinoAgent 是 Eino ADK 的 ChatModelAgent，替代手写的 ReAct 循环。
	EinoAgent *adk.ChatModelAgent

	// msgDedup 消息去重器：过滤 WS 断线重连/多连接导致的同一条消息重复投递。
	msgDedup *msgDedup
}

// memberInfoTTL 群成员信息缓存有效期（角色变更不频繁，10 分钟足够）。
const memberInfoTTL = 10 * time.Minute

// memberInfoEntry 缓存条目。
type memberInfoEntry struct {
	info      *adapter.GroupMemberInfo
	expiresAt time.Time
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
		Providers:       provider.NewProviderGroup(),
		MCP:             mcp.NewMCPGroup(),
		Tools:           tool.NewToolRegistry(),
		Skills:          skill.NewSkillEngine(),
		CronJobEvents:   make(chan adapter.Event, 64),
		Loops:           NewLoopTracker(),
		memberInfoCache: make(map[string]memberInfoEntry),
		batches:         make(map[string]*pendingBatch),
		hotStats:        make(map[string]*hotStat),
		relevanceSem:    make(chan struct{}, relevanceSemLimit),
		toolAdminOnly:   make(map[string]bool),
		knowledgeLRU:    newKnowledgeLRU(50),
		msgDedup:        newMsgDedup(dedupWindow),
	}
}

// dedupWindow 消息去重窗口：需大于 WS 断线重连 + 重推积压的最长间隔。
const dedupWindow = 60 * time.Second

// Init 从 DB 加载配置并初始化所有子模块。
func (h *HagoCenter) Init(ctx context.Context, cfg Config) error {
	h.Adapter = cfg.Adapter
	h.WebhookAdapter = cfg.WebhookAdapter
	h.DAO = cfg.DAO
	h.ACL = cfg.ACL
	h.Providers = cfg.Providers
	h.MCP = cfg.MCPGroup
	h.Cache = cfg.Cache

	// 缓存机器人自己的 QQ 号和昵称
	h.SelfQQ = h.Adapter.SelfID()
	if info, err := h.Adapter.GetLoginInfo(); err == nil && info != nil {
		h.SelfNickname = info.Nickname
	}
	log.Info("机器人身份信息", "self_qq", h.SelfQQ, "self_nickname", h.SelfNickname)

	// 存储 T2I/Sandbox 运行时客户端
	h.SandboxClient = cfg.Sandbox
	h.T2IClient = cfg.T2I

	// Session 管理器: 同时维护 Postgres Session 表 + ChatRecord 表 + Redis (历史路径) + 每日 Token 统计
	h.Session = session.NewSessionManager(cfg.DAO.Session, cfg.DAO.ChatRecord, cfg.DAO.TokenUsageDaily, cfg.Cache)

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
		func(ctx context.Context, folder string, limit int) (string, error) {
			return h.listImagesForTool(ctx, folder, limit)
		},
		func(ctx context.Context) (string, error) {
			return h.listStickerTagsForTool(ctx)
		},
		func(ctx context.Context, tag string, page, pageSize int) (string, error) {
			return h.listStickersForTool(ctx, tag, page, pageSize)
		},
		func(ctx context.Context, keyword string, limit int) (string, error) {
			return h.searchStickersForTool(ctx, keyword, limit)
		},
	)

	// 内置工具"仅管理员"标志：seed 默认高危名单到 DB（幂等），并加载运行时权限表
	h.seedBuiltinToolGuard(ctx)

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
			ID:             p.ID,
			Name:           p.Name,
			Type:           provider.ModelType(p.Type),
			Endpoint:       p.Endpoint,
			Token:          p.Token,
			Model:          p.Model,
			Temperature:    p.Temperature,
			EnableThinking: p.EnableThinking,
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

// RebuildEinoAgent 重建 Eino Agent（MCP 热添加/移除后调用，同步工具列表）。
func (h *HagoCenter) RebuildEinoAgent(ctx context.Context) {
	if err := h.buildEinoAgent(ctx); err != nil {
		log.Error("重建 Eino Agent 失败", "err", err)
	} else {
		log.Info("Eino Agent 已重建（工具列表已同步）")
	}
}

// Start 启动 Agent 系统 (事件循环 + CronJob 调度器)。
func (h *HagoCenter) Start(ctx context.Context) error {
	go h.runEventLoop(ctx)
	go h.CronJobManager.Run(ctx)
	log.Info("HagoCenter 已启动")
	return nil
}

// getGroupMemberInfoCached 带缓存的群成员信息查询：命中缓存直接返回，未命中调 OneBot11 API 并缓存。
func (h *HagoCenter) getGroupMemberInfoCached(groupID, userID int64) (*adapter.GroupMemberInfo, error) {
	if h.Adapter == nil {
		return nil, nil
	}
	key := fmt.Sprintf("%d:%d", groupID, userID)
	now := time.Now()

	h.memberInfoMu.RLock()
	if e, ok := h.memberInfoCache[key]; ok && now.Before(e.expiresAt) {
		h.memberInfoMu.RUnlock()
		return e.info, nil
	}
	h.memberInfoMu.RUnlock()

	info, err := h.Adapter.GetGroupMemberInfo(groupID, userID)
	if err != nil {
		return nil, err
	}

	h.memberInfoMu.Lock()
	h.memberInfoCache[key] = memberInfoEntry{info: info, expiresAt: now.Add(memberInfoTTL)}
	// 防无界增长：超过上限时整体清空（简单策略）
	if len(h.memberInfoCache) > 2048 {
		h.memberInfoCache = make(map[string]memberInfoEntry)
	}
	h.memberInfoMu.Unlock()
	return info, nil
}

// seedBuiltinToolGuard 为内置工具幂等创建 ToolConfig 行（首次创建时写入默认
// "仅管理员"标志），并从 DB 加载运行时权限表。
func (h *HagoCenter) seedBuiltinToolGuard(ctx context.Context) {
	if h.DAO == nil {
		return
	}
	for _, t := range h.Tools.List() {
		if !t.IsBuiltin() {
			continue
		}
		if err := h.DAO.ToolConfig.EnsureBuiltin(ctx, t.Name(), t.Description(), adminOnlyToolNames[t.Name()]); err != nil {
			log.Warn("内置工具配置 seed 失败", "tool", t.Name(), "err", err)
		}
	}
	if err := h.loadToolAdminOnly(ctx); err != nil {
		log.Warn("工具管理员名单加载失败", "err", err)
	}
}

// loadToolAdminOnly 从 DB 加载"仅管理员"工具名单。
func (h *HagoCenter) loadToolAdminOnly(ctx context.Context) error {
	m, err := h.DAO.ToolConfig.ListAdminOnly(ctx)
	if err != nil {
		return err
	}
	h.toolAdminOnlyMu.Lock()
	h.toolAdminOnly = m
	h.toolAdminOnlyMu.Unlock()
	return nil
}

// RefreshToolAdminOnly 从 DB 重载"仅管理员"工具名单（Web 配置变更后调用）。
func (h *HagoCenter) RefreshToolAdminOnly(ctx context.Context) {
	if err := h.loadToolAdminOnly(ctx); err != nil {
		log.Error("刷新工具管理员名单失败", "err", err)
		return
	}
	h.toolAdminOnlyMu.RLock()
	defer h.toolAdminOnlyMu.RUnlock()
	log.Info("工具管理员名单已刷新", "count", len(h.toolAdminOnly))
}

// isToolAdminOnly 查询工具是否标记为"仅管理员"。
func (h *HagoCenter) isToolAdminOnly(name string) bool {
	h.toolAdminOnlyMu.RLock()
	defer h.toolAdminOnlyMu.RUnlock()
	return h.toolAdminOnly[name]
}

// Stop 停止 Agent 系统。
func (h *HagoCenter) Stop() {
	log.Info("HagoCenter 已停止")
}
