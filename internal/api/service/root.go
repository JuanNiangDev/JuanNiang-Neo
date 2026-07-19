package service

import (
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/pluggin"
)

type Service struct {
	DAO           *dao.Bundle
	Adapter       *adapter.Adapter
	PluginEngine  *pluggin.PluginEngine
	ProviderGroup *provider.ProviderGroup
}

func New(dao *dao.Bundle, adapter *adapter.Adapter, pluginEngine *pluggin.PluginEngine) *Service {
	return &Service{DAO: dao, Adapter: adapter, PluginEngine: pluginEngine}
}
