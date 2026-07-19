package dto

import (
	"JuanNiang-Neo/internal/core/models"
)

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
	Temperature float32          `json:"temperature"`
	IsActive    bool             `json:"isActive"`
}

type UpdateProviderReq struct {
	Name        string           `json:"name"`
	Type        models.ModelType `json:"type"`
	Endpoint    string           `json:"endpoint"`
	Token       string           `json:"token"`
	Model       string           `json:"model"`
	Temperature float32          `json:"temperature"`
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
	Variables models.JSONSlice  `json:"variables"`
}

type UpdatePromptReq struct {
	Name      string            `json:"name"`
	Content   string            `json:"content"`
	Type      models.PromptType `json:"type"`
	IsActive  bool              `json:"is_active"`
	Variables models.JSONSlice  `json:"variables"`
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
	UserID     int64                `json:"user_id"`
	ChatAreaID string               `json:"chat_area_id"`
	Permission models.ACLPermission `json:"permission"`
	Actions    models.JSONSlice     `json:"actions"`
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
