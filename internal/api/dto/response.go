package dto

import (
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/core/models"
	"time"
)

var (
	OK                      = Response{Status: 0, Info: "OK"}
	ServerInternalErr       = Response{Status: 50000, Info: "服务器内部错误"}
	BindJSONErr             = Response{Status: 40001, Info: "参数格式错误"}
	UserOrPasswordWrong     = Response{Status: 40002, Info: "用户名或密码错误"}
	GenTokenFail            = Response{Status: 40003, Info: "token 生成失败"}
	UserNotExists           = Response{Status: 40004, Info: "用户不存在"}
	OriginPasswordWrong     = Response{Status: 40005, Info: "原密码错误"}
	UpdatePasswordFail      = Response{Status: 40006, Info: "密码更新失败"}
	InvalidQQNumber         = Response{Status: 40007, Info: "无效的 QQ 号"}
	AdapterNotReady         = Response{Status: 40008, Info: "adapter 未初始化"}
	ProviderNotExist        = Response{Status: 40009, Info: "provider 不存在"}
	MCPNotExist             = Response{Status: 40010, Info: "MCP 服务器不存在"}
	SessionNotExist         = Response{Status: 40011, Info: "Session 不存在"}
	EmptyFileToUpload       = Response{Status: 40012, Info: "缺少上传文件"}
	TempFileCreateFail      = Response{Status: 40013, Info: "临时文件创建失败"}
	WriteFileFail           = Response{Status: 40014, Info: "文件写入失败"}
	InvalidZipFile          = Response{Status: 40015, Info: "无效的 ZIP 文件"}
	InvalidACLID            = Response{Status: 40016, Info: "无效的 ACL ID"}
	UpdateAdapterConfigFail = Response{Status: 40017, Info: "onebot11 适配器配置更新失败"}
	SkillNotExist           = Response{Status: 40018, Info: "Skill 不存在"}
	PromptNotExist          = Response{Status: 40019, Info: "Prompt 不存在"}
	ToolNotExist            = Response{Status: 40020, Info: "Tool 不存在"}
	PluginNotExist          = Response{Status: 40021, Info: "Plugin 不存在"}
	PluginLoadFail          = Response{Status: 40022, Info: "插件加载失败"}
	ChatAreaNotFound        = Response{Status: 40023, Info: "ChatArea 不存在"}
	MemoryConfigNotFound    = Response{Status: 40024, Info: "Memory 配置不存在"}
	AdapterConfigNotFound   = Response{Status: 40025, Info: "adapter 配置不存在"}
	T2IConfigNotFound       = Response{Status: 40026, Info: "T2I 配置不存在"}
	SandboxConfigNotFound   = Response{Status: 40027, Info: "Sandbox 配置不存在"}
	PluginIsSystem          = Response{Status: 40028, Info: "系统插件不允许删除或停用"}
	PromptIsSystem          = Response{Status: 40029, Info: "系统提示词不允许修改或删除"}
	ToolIsBuiltin           = Response{Status: 40030, Info: "内置工具运行时常驻, 不支持启停"}
)

type TokenResp struct {
	Token string `json:"token"`
}

type ErrorDetail struct {
	ErrorDetail string `json:"error_detail"`
}

type AdapterStatus struct {
	Running    bool                 `json:"running"`
	ListenAddr string               `json:"listen_addr"`
	SelfID     int64                `json:"self_id"`
	ConnCount  int                  `json:"conn_count"`
	ConnIDs    []int64              `json:"conn_ids"`
	Conns      []adapter.ConnDetail `json:"conns"`
}

type AdapterConfig struct {
	Addr           string   `json:"addr"`
	Port           int      `json:"port"`
	Token          string   `json:"token"`
	AdminQQNumbers []string `json:"admin_qq_numbers"`
	Enabled        bool     `json:"enabled"`
}

type ProviderResp struct {
	ID          string           `json:"id"`
	CreatedAt   time.Time        `json:"created_at"`
	Name        string           `json:"name"`
	Type        models.ModelType `json:"type"`
	Endpoint    string           `json:"endpoint"`
	Token       string           `json:"token"`
	Model       string           `json:"model"`
	Temperature float32          `json:"temperature"`
	IsActive    bool             `json:"is_active"`
}

type MCPServerResp struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	ServerURL     string           `json:"server_url"`
	Headers       models.JSONMap   `json:"headers"`
	Timeout       int              `json:"timeout"`
	RetryCount    int              `json:"retry_count"`
	ToolFilter    models.JSONSlice `json:"tool_filter"`
	AutoReconnect bool             `json:"auto_reconnect"`
	IsActive      bool             `json:"is_active"`
	CreatedAt     time.Time        `json:"created_at"`
}

type SkillResp struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Keywords     models.JSONSlice `json:"keywords"`
	RegexPattern string           `json:"regex_pattern"`
	PromptRef    string           `json:"prompt_ref"`
	ToolRefs     models.JSONSlice `json:"tool_refs"`
	McpRefs      models.JSONSlice `json:"mcp_refs"`
	IsActive     bool             `json:"is_active"`
	IsSystem     bool             `json:"is_system"`
	Priority     int              `json:"priority"`
	CreatedAt    time.Time        `json:"created_at"`
}

type PromptResp struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Type      models.PromptType `json:"type"`
	IsActive  bool              `json:"is_active"`
	IsSystem  bool              `json:"is_system"`
	CreatedAt time.Time         `json:"created_at"`
}

type ToolConfigResp struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  models.JSONMap `json:"parameters"`
	Timeout     int            `json:"timeout"`
	IsActive    bool           `json:"is_active"`
	IsBuiltin   bool           `json:"is_builtin"`
	CreatedAt   time.Time      `json:"created_at"`
}

type PluginResp struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Path      string         `json:"path"`
	Config    models.JSONMap `json:"config"`
	IsActive  bool           `json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
}

type ACLRuleResp struct {
	ID         int64                `json:"id"`
	ChatAreaID string               `json:"chat_area_id"`
	Scope      models.ACLScope      `json:"scope"`
	Permission models.ACLPermission `json:"permission"`
	TargetType models.ACLTargetType `json:"target_type"`
	UserIDs    models.JSONSlice     `json:"user_ids"`
	ToolIDs    models.JSONSlice     `json:"tool_ids"`
	MCPIDs     models.JSONSlice     `json:"mcp_ids"`
	CreatedAt  time.Time            `json:"created_at"`
}

type SessionResp struct {
	ID         string         `json:"id"`
	ChatAreaID string         `json:"chat_area_id"`
	Model      string         `json:"model"`
	TokenUsage int64          `json:"token_usage"`
	MetaData   models.JSONMap `json:"meta_data"`
	CreatedAt  time.Time      `json:"created_at"`
}

type ShortTermMemoryResp struct {
	ID          string    `json:"id"`
	ChatAreaID  string    `json:"chat_area_id"`
	WindowSize  int       `json:"window_size"`
	AutoCompact bool      `json:"auto_compact"`
	CreatedAt   time.Time `json:"created_at"`
}

type LongTermMemoryResp struct {
	ID           string    `json:"id"`
	ChatAreaID   string    `json:"chat_area_id"`
	HotAreaSize  int       `json:"hot_area_size"`
	HotMemoryTTL int       `json:"hot_memory_ttl"`
	CreatedAt    time.Time `json:"created_at"`
}

type ChatAreaResp struct {
	ID        string          `json:"id"`
	AreaType  models.AreaType `json:"area_type"`
	TargetID  int64           `json:"target_id"`
	CreatedAt time.Time       `json:"created_at"`
}

type ChatRecordResp struct {
	ID         int64          `json:"id"`
	ChatAreaID string         `json:"chat_area_id"`
	UserID     int64          `json:"user_id"`
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	TokenCount int            `json:"token_count"`
	ToolCalls  models.JSONMap `json:"tool_calls"`
	CreatedAt  time.Time      `json:"created_at"`
}

type OverviewResp struct {
	ChatAreaCount   int64 `json:"chat_area_count"`
	MCPCount        int64 `json:"mcp_count"`
	AdapterCount    int64 `json:"adapter_count"`
	PluginCount     int64 `json:"plugin_count"`
	ProviderCount   int   `json:"provider_count"`
	SkillCount      int   `json:"skill_count"`
	SessionCount    int   `json:"session_count"`
	TotalTokenUsage int64 `json:"total_token_usage"`

	// 系统状态
	CPUCount          int    `json:"cpu_count"`            // 逻辑 CPU 核数
	GoroutineNum      int    `json:"goroutine_num"`        // 当前 goroutine 数
	MemAllocBytes     uint64 `json:"mem_alloc_bytes"`      // 堆已分配 (活跃对象)
	MemSysBytes       uint64 `json:"mem_sys_bytes"`        // 从 OS 获取的内存总量
	MemHeapInUseBytes uint64 `json:"mem_heap_inuse_bytes"` // 堆中正在使用
	GoVersion         string `json:"go_version"`           // Go 版本

	// Adapter 运行状态
	AdapterRunning bool `json:"adapter_running"` // OneBot11 反向 WS 是否运行中

	// T2I / Sandbox 状态
	T2IActive      bool `json:"t2i_active"`      // 客户端已加载
	T2IHealthy     bool `json:"t2i_healthy"`     // HealthCheck 通过
	SandboxActive  bool `json:"sandbox_active"`  // 客户端已加载
	SandboxHealthy bool `json:"sandbox_healthy"` // HealthCheck 通过
}

type ChatRecordListResp struct {
	Total int64            `json:"total"`
	List  []ChatRecordResp `json:"list"`
}

type PluginUploadResp struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type T2IConfigResp struct {
	BaseURL  string `json:"base_url"`
	Timeout  int    `json:"timeout"`
	IsActive bool   `json:"is_active"`
	Healthy  bool   `json:"healthy"`
}

type SandboxConfigResp struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Timeout  int    `json:"timeout"`
	IsActive bool   `json:"is_active"`
	Healthy  bool   `json:"healthy"`
}

type WebhookConfigResp struct {
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
	Running bool   `json:"running"`
}

// ---------- Logs ----------

// LogEntryResp 对应 internal/logging.Entry 的前端响应结构。
type LogEntryResp struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// ---------- Background Tasks ----------

// BackgroundTaskResp 后台任务前端响应。
type BackgroundTaskResp struct {
	ID          string         `json:"id"`
	ChatAreaID  string         `json:"chat_area_id"`
	Status      string         `json:"status"`
	MessageType string         `json:"message_type"`
	TargetID    int64          `json:"target_id"`
	UserPrompt  string         `json:"user_prompt"`
	Steps       models.JSONMap `json:"steps"`
	Results     models.JSONMap `json:"results"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ---------- CronJob ----------

type CronJobResp struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CronExpr    string     `json:"cron_expr"`
	Message     string     `json:"message"`
	MessageType string     `json:"message_type"`
	TargetID    int64      `json:"target_id"`
	IsActive    bool       `json:"is_active"`
	LastRunAt   *time.Time `json:"last_run_at"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ---------- 回复策略 ----------

type ReplyStrategyResp struct {
	Strategy           string  `json:"strategy"`
	RelevanceThreshold float64 `json:"relevance_threshold"`
}
