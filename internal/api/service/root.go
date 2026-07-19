package service

import (
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/session"
	"JuanNiang-Neo/internal/agent/skill"
	"JuanNiang-Neo/internal/agent/tool"
	"JuanNiang-Neo/internal/core/acl"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/pluggin"
)

type Service struct {
	DAO           *dao.Bundle
	Adapter       *adapter.Adapter
	PluginEngine  *pluggin.PluginEngine
	ProviderGroup *provider.ProviderGroup
	MCPGroup      *mcp.MCPGroup
	MemoryGroup   *memory.MemoryGroup
	SessionMgr    *session.SessionManager
	ToolRegistry  *tool.ToolRegistry
	SkillEngine   *skill.SkillEngine
	ACLMgr        *acl.ACL
}

func New(dao *dao.Bundle, adapter *adapter.Adapter, pluginEngine *pluggin.PluginEngine) *Service {
	return &Service{DAO: dao, Adapter: adapter, PluginEngine: pluginEngine}
}
