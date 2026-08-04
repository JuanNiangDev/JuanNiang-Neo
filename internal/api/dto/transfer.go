package dto

import (
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
	"encoding/json"
)

func RawProviderList2Resp(raw []models.Provider) []ProviderResp {
	res := make([]ProviderResp, len(raw))

	for i, item := range raw {
		res[i].ID = item.ID
		res[i].CreatedAt = item.CreatedAt
		res[i].Name = item.Name
		res[i].Type = item.Type
		res[i].Endpoint = item.Endpoint
		res[i].Token = item.Token
		res[i].Model = item.Model
		res[i].Temperature = item.Temperature
		res[i].IsActive = item.IsActive
		res[i].EnableThinking = item.EnableThinking
	}

	return res
}

func RawMCPServer2Resp(raw *models.MCPServer) MCPServerResp {
	return MCPServerResp{
		ID:            raw.ID,
		Name:          raw.Name,
		ServerURL:     raw.ServerURL,
		Headers:       raw.Headers,
		Timeout:       raw.Timeout,
		RetryCount:    raw.RetryCount,
		ToolFilter:    raw.ToolFilter,
		AutoReconnect: raw.AutoReconnect,
		IsActive:      raw.IsActive,
		CreatedAt:     raw.CreatedAt,
	}
}

func RawMCPServerList2Resp(raw []models.MCPServer) []MCPServerResp {
	res := make([]MCPServerResp, len(raw))
	for i, item := range raw {
		res[i] = RawMCPServer2Resp(&item)
	}
	return res
}

func RawSkill2Resp(raw *models.Skill) SkillResp {
	return SkillResp{
		ID:           raw.ID,
		Name:         raw.Name,
		Description:  raw.Description,
		Keywords:     raw.Keywords,
		RegexPattern: raw.RegexPattern,
		PromptRefs:   raw.PromptRefs,
		ToolRefs:     raw.ToolRefs,
		McpRefs:      raw.McpRefs,
		IsActive:     raw.IsActive,
		IsSystem:     raw.IsSystem,
		Priority:     raw.Priority,
		CreatedAt:    raw.CreatedAt,
	}
}

func RawSkillList2Resp(raw []models.Skill) []SkillResp {
	res := make([]SkillResp, len(raw))
	for i, item := range raw {
		res[i] = RawSkill2Resp(&item)
	}
	return res
}

func RawPrompt2Resp(raw *models.Prompt) PromptResp {
	return PromptResp{
		ID:        raw.ID,
		Name:      raw.Name,
		Content:   raw.Content,
		Type:      raw.Type,
		IsActive:  raw.IsActive,
		IsSystem:  raw.IsSystem,
		CreatedAt: raw.CreatedAt,
	}
}

func RawPromptList2Resp(raw []models.Prompt) []PromptResp {
	res := make([]PromptResp, len(raw))
	for i, item := range raw {
		res[i] = RawPrompt2Resp(&item)
	}
	return res
}

func RawToolConfigList2Resp(raw []models.ToolConfig) []ToolConfigResp {
	res := make([]ToolConfigResp, len(raw))
	for i, item := range raw {
		res[i] = RawToolConfig2Resp(&item)
	}
	return res
}

func RawToolConfig2Resp(raw *models.ToolConfig) ToolConfigResp {
	return ToolConfigResp{
		ID:          raw.ID,
		Name:        raw.Name,
		Description: raw.Description,
		Parameters:  raw.Parameters,
		Timeout:     raw.Timeout,
		IsActive:    raw.IsActive,
		IsBuiltin:   raw.IsBuiltin,
		CreatedAt:   raw.CreatedAt,
	}
}

func RawPlugin2Resp(raw *models.Plugin) PluginResp {
	return PluginResp{
		ID:        raw.ID,
		Name:      raw.Name,
		Version:   raw.Version,
		Path:      raw.Path,
		Config:    raw.Config,
		IsActive:  raw.IsActive,
		CreatedAt: raw.CreatedAt,
	}
}

func RawPluginList2Resp(raw []models.Plugin) []PluginResp {
	res := make([]PluginResp, len(raw))
	for i, item := range raw {
		res[i] = RawPlugin2Resp(&item)
	}
	return res
}

func RawACLRule2Resp(raw *models.ACLRule) ACLRuleResp {
	return ACLRuleResp{
		ID:         raw.ID,
		ChatAreaID: raw.ChatAreaID,
		Scope:      raw.Scope,
		Permission: raw.Permission,
		TargetType: raw.TargetType,
		UserIDs:    raw.UserIDs,
		ToolIDs:    raw.ToolIDs,
		MCPIDs:     raw.MCPIDs,
		CreatedAt:  raw.CreatedAt,
	}
}

func RawACLRuleList2Resp(raw []models.ACLRule) []ACLRuleResp {
	res := make([]ACLRuleResp, len(raw))
	for i, item := range raw {
		res[i] = RawACLRule2Resp(&item)
	}
	return res
}

func RawSession2Resp(raw *models.Session) SessionResp {
	return SessionResp{
		ID:         raw.ID,
		ChatAreaID: raw.ChatAreaID,
		Model:      raw.Model,
		TokenUsage: raw.TokenUsage,
		MetaData:   raw.MetaData,
		CreatedAt:  raw.CreatedAt,
	}
}

func RawSessionList2Resp(raw []models.Session) []SessionResp {
	res := make([]SessionResp, len(raw))
	for i, item := range raw {
		res[i] = RawSession2Resp(&item)
	}
	return res
}

func RawChatAreaList2Resp(raw []models.ChatArea) []ChatAreaResp {
	res := make([]ChatAreaResp, len(raw))
	for i, item := range raw {
		res[i] = ChatAreaResp{
			ID:        item.ID,
			AreaType:  item.AreaType,
			TargetID:  item.TargetID,
			CreatedAt: item.CreatedAt,
		}
	}
	return res
}

func RawChatRecord2Resp(raw *models.ChatRecord) ChatRecordResp {
	return ChatRecordResp{
		ID:         raw.ID,
		ChatAreaID: raw.ChatAreaID,
		UserID:     raw.UserID,
		Role:       raw.Role,
		Content:    raw.Content,
		TokenCount: raw.TokenCount,
		ToolCalls:  raw.ToolCalls,
		CreatedAt:  raw.CreatedAt,
	}
}

func RawChatRecordList2Resp(raw []models.ChatRecord) []ChatRecordResp {
	res := make([]ChatRecordResp, len(raw))
	for i, item := range raw {
		res[i] = RawChatRecord2Resp(&item)
	}
	return res
}

func RawShortTermMemory2Resp(raw *models.ShortTermMemory) ShortTermMemoryResp {
	return ShortTermMemoryResp{
		ID:          raw.ID,
		ChatAreaID:  raw.ChatAreaID,
		WindowSize:  raw.WindowSize,
		AutoCompact: raw.AutoCompact,
		CreatedAt:   raw.CreatedAt,
	}
}

func RawLongTermMemory2Resp(raw *models.LongTermMemory) LongTermMemoryResp {
	return LongTermMemoryResp{
		ID:           raw.ID,
		ChatAreaID:   raw.ChatAreaID,
		HotAreaSize:  raw.HotAreaSize,
		HotMemoryTTL: raw.HotMemoryTTL,
		CreatedAt:    raw.CreatedAt,
	}
}

func RawT2IConfig2Resp(raw *models.T2IConfig, healthy bool) T2IConfigResp {
	return T2IConfigResp{
		BaseURL:  raw.BaseURL,
		Timeout:  raw.Timeout,
		IsActive: raw.IsActive,
		Healthy:  healthy,
	}
}

func RawSandboxConfig2Resp(raw *models.SandboxConfig, healthy bool) SandboxConfigResp {
	return SandboxConfigResp{
		BaseURL:  raw.BaseURL,
		APIKey:   raw.APIKey,
		Timeout:  raw.Timeout,
		IsActive: raw.IsActive,
		Healthy:  healthy,
	}
}

// RawLogEntry2Resp 把 internal/logging.Entry 转为前端响应。
func RawLogEntry2Resp(raw logging.Entry) LogEntryResp {
	return LogEntryResp{
		Time:    raw.Time,
		Level:   raw.Level,
		Module:  raw.Module,
		Message: raw.Message,
		Attrs:   raw.Attrs,
		Stack:   raw.Stack,
	}
}

// RawLogEntryList2Resp 批量转换。
func RawLogEntryList2Resp(raw []logging.Entry) []LogEntryResp {
	res := make([]LogEntryResp, len(raw))
	for i, item := range raw {
		res[i] = RawLogEntry2Resp(item)
	}
	return res
}

// ---------- CronJob ----------

func RawCronJob2Resp(raw *models.CronJob) CronJobResp {
	var pluginIDs []string
	if raw.PluginIDs != "" {
		json.Unmarshal([]byte(raw.PluginIDs), &pluginIDs)
	}
	return CronJobResp{
		ID:          raw.ID,
		Name:        raw.Name,
		CronExpr:    raw.CronExpr,
		Message:     raw.Message,
		MessageType: raw.MessageType,
		TargetID:    raw.TargetID,
		IsActive:    raw.IsActive,
		PluginIDs:   pluginIDs,
		Payload:     raw.Payload,
		LastRunAt:   raw.LastRunAt,
		LastError:   raw.LastError,
		CreatedAt:   raw.CreatedAt,
		UpdatedAt:   raw.UpdatedAt,
	}
}

func RawCronJobList2Resp(raw []models.CronJob) []CronJobResp {
	res := make([]CronJobResp, len(raw))
	for i, item := range raw {
		res[i] = RawCronJob2Resp(&item)
	}
	return res
}
