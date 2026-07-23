package dto

import (
	"encoding/json"
	"fmt"
	"strconv"

	"JuanNiang-Neo/internal/core/models"
)

// FlexFloat32 支持 JSON 字符串和数字两种格式的反序列化。
// 前端 <input type="number"> 在某些情况下会将数字值序列化为 JSON 字符串。
type FlexFloat32 float32

func (f *FlexFloat32) UnmarshalJSON(data []byte) error {
	// 尝试作为数字解析
	var v float32
	if err := json.Unmarshal(data, &v); err == nil {
		*f = FlexFloat32(v)
		return nil
	}
	// 尝试作为字符串解析
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		v64, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return fmt.Errorf("FlexFloat32: 无法解析字符串 %q: %w", s, err)
		}
		*f = FlexFloat32(v64)
		return nil
	}
	return fmt.Errorf("FlexFloat32: 无法解析 %s", string(data))
}

func (f FlexFloat32) MarshalJSON() ([]byte, error) {
	return json.Marshal(float32(f))
}

type ChangePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AddAdminQQReq struct {
	QQ string `json:"qq"`
}

type UpdateAdapterConfigReq struct {
	Addr           string   `json:"addr"`
	Port           int      `json:"port"`
	Token          string   `json:"token"`
	AdminQQNumbers []string `json:"admin_qq_numbers"`
	Enabled        bool     `json:"enabled"`
}

type AddProviderReq struct {
	Name        string           `json:"name"`
	Type        models.ModelType `json:"type"`
	Endpoint    string           `json:"endpoint"`
	Token       string           `json:"token"`
	Model       string           `json:"model"`
	Temperature FlexFloat32      `json:"temperature"`
	IsActive    bool             `json:"isActive"`
}

type UpdateProviderReq struct {
	Name        string           `json:"name"`
	Type        models.ModelType `json:"type"`
	Endpoint    string           `json:"endpoint"`
	Token       string           `json:"token"`
	Model       string           `json:"model"`
	Temperature FlexFloat32      `json:"temperature"`
	IsActive    bool             `json:"isActive"`
}

type ToggleProviderReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- MCP ----------

type AddMCPServerReq struct {
	Name          string           `json:"name"`
	ServerURL     string           `json:"server_url"`
	Headers       models.JSONMap   `json:"headers"`
	Timeout       int              `json:"timeout"`
	RetryCount    int              `json:"retry_count"`
	ToolFilter    models.JSONSlice `json:"tool_filter"`
	AutoReconnect bool             `json:"auto_reconnect"`
	IsActive      bool             `json:"is_active"`
}

type UpdateMCPServerReq struct {
	Name          string           `json:"name"`
	ServerURL     string           `json:"server_url"`
	Headers       models.JSONMap   `json:"headers"`
	Timeout       int              `json:"timeout"`
	RetryCount    int              `json:"retry_count"`
	ToolFilter    models.JSONSlice `json:"tool_filter"`
	AutoReconnect bool             `json:"auto_reconnect"`
	IsActive      bool             `json:"is_active"`
}

type ToggleMCPServerReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- Skill ----------

type AddSkillReq struct {
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
}

type UpdateSkillReq struct {
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
}

// ---------- Prompt ----------

type AddPromptReq struct {
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Type      models.PromptType `json:"type"`
	IsActive  bool              `json:"is_active"`
}

type UpdatePromptReq struct {
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Type      models.PromptType `json:"type"`
	IsActive  bool              `json:"is_active"`
}

type TogglePromptReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- Tool ----------

type ToggleToolReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- Plugin ----------

type TogglePluginReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- ACL ----------

type AddACLRuleReq struct {
	ChatAreaID string               `json:"chat_area_id"`
	Scope      models.ACLScope      `json:"scope"`
	Permission models.ACLPermission `json:"permission"`
	TargetType models.ACLTargetType `json:"target_type"`
	UserIDs    models.JSONSlice     `json:"user_ids"`
	ToolIDs    models.JSONSlice     `json:"tool_ids"`
	MCPIDs     models.JSONSlice     `json:"mcp_ids"`
}

// ---------- Memory ----------

type UpdateShortTermMemoryReq struct {
	WindowSize  int  `json:"window_size"`
	AutoCompact bool `json:"auto_compact"`
}

type UpdateLongTermMemoryReq struct {
	HotAreaSize  int `json:"hot_area_size"`
	HotMemoryTTL int `json:"hot_memory_ttl"`
}

// ---------- T2I ----------

type UpdateT2IConfigReq struct {
	BaseURL  string `json:"base_url"`
	Timeout  int    `json:"timeout"`
	IsActive bool   `json:"is_active"`
}

// ---------- Sandbox ----------

type UpdateSandboxConfigReq struct {
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
	Timeout  int    `json:"timeout"`
	IsActive bool   `json:"is_active"`
}

// ---------- Webhook ----------

type UpdateWebhookConfigReq struct {
	Addr    string `json:"addr"`
	Port    int    `json:"port"`
	Token   string `json:"token"`
	Enabled bool   `json:"enabled"`
}

// ---------- CronJob ----------

type AddCronJobReq struct {
	Name        string `json:"name"`
	CronExpr    string `json:"cron_expr"`
	Message     string `json:"message"`
	MessageType string `json:"message_type"`
	TargetID    int64  `json:"target_id"`
	IsActive    bool   `json:"is_active"`
}

type UpdateCronJobReq struct {
	Name        string `json:"name"`
	CronExpr    string `json:"cron_expr"`
	Message     string `json:"message"`
	MessageType string `json:"message_type"`
	TargetID    int64  `json:"target_id"`
	IsActive    bool   `json:"is_active"`
}

type ToggleCronJobReq struct {
	IsActive bool `json:"is_active"`
}

// ---------- 回复策略 ----------

type UpdateReplyStrategyReq struct {
	Strategy           string  `json:"strategy"`
	RelevanceThreshold float64 `json:"relevance_threshold"`
	BotName            string  `json:"bot_name"`
	StripMarkdown      bool    `json:"strip_markdown"`
	AgentLite          bool    `json:"agent_lite"`
}
