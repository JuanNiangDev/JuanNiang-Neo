package service

import (
	"context"
	"time"

	sandbox "JuanNiang-Neo/infrastructure/sandbox"
	sandboxcaller "JuanNiang-Neo/infrastructure/sandbox/handler"
	t2i "JuanNiang-Neo/infrastructure/t2i"
	t2icaller "JuanNiang-Neo/infrastructure/t2i/handler"
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent"
	cronjobmgr "JuanNiang-Neo/internal/agent/cronjob"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/prompt"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/session"
	"JuanNiang-Neo/internal/agent/skill"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/imgstore"
	"JuanNiang-Neo/internal/logging"
	"JuanNiang-Neo/internal/pluggin"
)

type Service struct {
	DAO            *dao.Bundle
	Adapter        *adapter.Adapter
	WebhookAdapter *adapter.WebhookAdapter
	PluginEngine   *pluggin.PluginEngine
	ProviderGroup  *provider.ProviderGroup
	MCPGroup       *mcp.MCPGroup
	MemoryGroup    *memory.MemoryGroup
	SessionMgr     *session.SessionManager
	ToolRegistry   *tool.ToolRegistry
	SkillEngine    *skill.SkillEngine
	ACLMgr         *acl.ACL

	// LogHub 是日志广播中心；前端通过 GET /logs 与 GET /logs/stream 消费。
	LogHub *logging.Hub

	// T2I / Sandbox 运行时客户端 + 同步回调
	T2IClient     *t2icaller.Client
	SandboxClient *sandboxcaller.Client
	// OnUpdateT2I 在 T2I 配置变更时调用，用于同步到 HagoCenter。
	OnUpdateT2I     func(client *t2icaller.Client)
	OnUpdateSandbox func(client *sandboxcaller.Client)
	// CronJobManager 在 CronJob 变更时调用 Reload 同步调度器。
	CronJobManager *cronjobmgr.Manager
	// OnRebuildAgent MCP/Provider/Tool 热变更后重建 Eino Agent 工具列表。
	OnRebuildAgent func()
	// OnUpdateToolAdminOnly 工具的"仅管理员"标志变更后刷新 Agent 运行时权限表。
	OnUpdateToolAdminOnly func()
	// OnKnowledgeChanged 知识库条目变更后失效 Agent 侧 LRU 缓存。
	OnKnowledgeChanged func()
	// OnExtractKnowledge 知识新增/编辑后触发异步关键词提取。
	OnExtractKnowledge func(id string)
	// LoopTracker 当前活跃的 Agent ReAct 循环（监控展示）。
	LoopTracker *agent.LoopTracker
	// PromptMgr 提示词管理器（缓存失效用）。
	PromptMgr *prompt.PromptManager
	// ImageStore 图床文件存储（data/imgs，二进制文件读写）。
	ImageStore *imgstore.Store
	// OnFishCalReload 摸鱼日历配置变更后重新调度。
	OnFishCalReload func()
	// OnFishCalTrigger 手动触发摸鱼日历立即执行一次。
	OnFishCalTrigger func(ctx context.Context) error
}

func New(dao *dao.Bundle, adapter *adapter.Adapter, webhookAdapter *adapter.WebhookAdapter, pluginEngine *pluggin.PluginEngine) *Service {
	return &Service{DAO: dao, Adapter: adapter, WebhookAdapter: webhookAdapter, PluginEngine: pluginEngine, LogHub: logging.DefaultHub}
}

// t2iClientFactory 根据配置创建 T2I 客户端。
func t2iClientFactory(baseURL string, timeoutSec int) *t2icaller.Client {
	client, err := t2i.NewClient(
		t2i.WithBaseURL(baseURL),
		t2i.WithTimeout(time.Duration(timeoutSec)*time.Second),
	)
	if err != nil {
		return nil
	}
	return client
}

// sandboxClientFactory 根据配置创建 Sandbox 客户端。
func sandboxClientFactory(baseURL, apiKey string, timeoutSec int) *sandboxcaller.Client {
	client, err := sandbox.NewClient(
		sandbox.WithBaseURL(baseURL),
		sandbox.WithAPIKey(apiKey),
		sandbox.WithTimeout(time.Duration(timeoutSec)*time.Second),
	)
	if err != nil {
		return nil
	}
	return client
}
