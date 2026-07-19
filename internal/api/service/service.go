package service

import (
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/mcp"
	"JuanNiang-Neo/internal/agent/memory"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/agent/skill"
	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/core/models"
	"JuanNiang-Neo/internal/logging"
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) Login(ctx context.Context, c *app.RequestContext) {
	var data dto.LoginReq

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	user, err := s.DAO.User.GetByUsername(ctx, data.Username)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.UserOrPasswordWrong, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(data.Password)) != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.UserOrPasswordWrong, nil))
		return
	}

	token, err := middleware.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.GenTokenFail, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.TokenResp{Token: token}))
}

func (s *Service) ChangePassword(ctx context.Context, c *app.RequestContext) {
	var data dto.ChangePasswordReq

	userID := c.GetUint("user_id")

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	user, err := s.DAO.User.GetByUsername(ctx, "admin")
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.UserNotExists, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(data.OldPassword)) != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OriginPasswordWrong, nil))
		return
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(data.NewPassword), bcrypt.DefaultCost)
	if err := s.DAO.User.UpdatePassword(ctx, userID, string(hash)); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.UpdatePasswordFail, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) GetAdapterStatus(ctx context.Context, c *app.RequestContext) {
	raw := s.Adapter.Status()

	status := dto.AdapterStatus{
		Running:    raw.Running,
		ListenAddr: raw.ListenAddr,
		SelfID:     raw.SelfID,
		ConnCount:  raw.ConnCount,
		ConnIDs:    raw.ConnIDs,
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, status))
}

func (s *Service) GetAdapterConfig(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.Onebot11Adapter.GetAdapterConfig(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	data := dto.AdapterConfig{
		Addr:           raw.Addr,
		Port:           raw.Port,
		Token:          raw.Token,
		AdminQQNumbers: raw.AdminQQNumbers,
		Enabled:        raw.Enabled,
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, data))
}

func (s *Service) UpdateAdapterConfig(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateAdapterConfigReq

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	conf := adapter.Config{
		Addr:   data.Addr,
		Port:   data.Port,
		Token:  data.Token,
		Admins: data.AdminQQNumbers,
		Enable: data.Enabled,
	}

	if err := s.DAO.Onebot11Adapter.UpdateAdapterConfig(ctx, &models.Onebot11Adapter{
		Addr:           data.Addr,
		Port:           data.Port,
		Token:          data.Token,
		AdminQQNumbers: data.AdminQQNumbers,
		Enabled:        data.Enabled,
	}); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if err := s.Adapter.SyncConfig(ctx, conf); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.UpdateAdapterConfigFail, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) RestartAdapter(ctx context.Context, c *app.RequestContext) {
	if err := s.Adapter.Restart(ctx); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ListProviders(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.Provider.List(ctx, "")
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	list := dto.RawProviderList2Resp(raw)

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, list))
}

func (s *Service) GetProvider(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.Provider.GetByID(ctx, c.Param("id"))

	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ProviderNotExist, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	data := dto.ProviderResp{
		ID:          raw.ID,
		CreatedAt:   raw.CreatedAt,
		Name:        raw.Name,
		Type:        raw.Type,
		Endpoint:    raw.Endpoint,
		Token:       raw.Token,
		Model:       raw.Model,
		Temperature: raw.Temperature,
		IsActive:    raw.IsActive,
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, data))
}

func (s *Service) AddProvider(ctx context.Context, c *app.RequestContext) {
	var data dto.AddProviderReq

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	id := newUUID()
	providerConfig := models.Provider{
		ID:          id,
		Name:        data.Name,
		Type:        data.Type,
		Endpoint:    data.Endpoint,
		Token:       data.Token,
		Model:       data.Model,
		Temperature: data.Temperature,
		IsActive:    data.IsActive,
	}

	// 同类型只能有一个 Active：激活前先停用同类型其他 Provider
	if data.IsActive {
		if err := s.DAO.Provider.DeactivateByType(ctx, data.Type, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	if err := s.DAO.Provider.Create(ctx, &providerConfig); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：先移除同类型旧的运行时 Provider，再添加新的
	if data.IsActive && s.ProviderGroup != nil {
		provType := provider.ModelType(data.Type)
		for _, p := range s.ProviderGroup.ListProviders() {
			if p.Type() == provType {
				s.ProviderGroup.DelProvider(p.ID())
			}
		}
		s.ProviderGroup.AddProvider(provider.NewProvider(provider.ProviderConfig{
			ID:          id,
			Name:        data.Name,
			Type:        provType,
			Endpoint:    data.Endpoint,
			Token:       data.Token,
			Model:       data.Model,
			Temperature: data.Temperature,
		}))
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, data))
}

func (s *Service) UpdateProvider(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateProviderReq

	id := c.Param("id")

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 同类型只能有一个 Active：激活前先停用同类型其他 Provider
	if data.IsActive {
		if err := s.DAO.Provider.DeactivateByType(ctx, data.Type, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	providerConfig := models.Provider{
		ID:          id,
		Name:        data.Name,
		Type:        data.Type,
		Endpoint:    data.Endpoint,
		Token:       data.Token,
		Model:       data.Model,
		Temperature: data.Temperature,
		IsActive:    data.IsActive,
	}

	provType := provider.ModelType(data.Type)
	providerConfig_ := provider.ProviderConfig{
		ID:          id,
		Name:        data.Name,
		Type:        provType,
		Endpoint:    data.Endpoint,
		Token:       data.Token,
		Model:       data.Model,
		Temperature: data.Temperature,
	}

	// 运行时同步：先移除同类型旧的，再同步新配置
	if s.ProviderGroup != nil {
		if data.IsActive {
			for _, p := range s.ProviderGroup.ListProviders() {
				if p.Type() == provType && p.ID() != id {
					s.ProviderGroup.DelProvider(p.ID())
				}
			}
		}
		s.ProviderGroup.SyncConfig(providerConfig_)
	}

	if err := s.DAO.Provider.Update(ctx, &providerConfig); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) DeleteProvider(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	if err := s.DAO.Provider.Delete(ctx, id); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	s.ProviderGroup.DelProvider(id)

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ToggleProvider(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleProviderReq

	id := c.Param("id")

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if data.IsActive {
		// 获取当前 Provider 信息以确认类型
		raw, err := s.DAO.Provider.GetByID(ctx, id)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ProviderNotExist, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}

		// 同类型只能有一个 Active：先停用同类型其他 Provider
		if err := s.DAO.Provider.DeactivateByType(ctx, raw.Type, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}

		// 运行时：移除同类型旧的，添加新的
		if s.ProviderGroup != nil {
			provType := provider.ModelType(raw.Type)
			for _, p := range s.ProviderGroup.ListProviders() {
				if p.Type() == provType {
					s.ProviderGroup.DelProvider(p.ID())
				}
			}
			s.ProviderGroup.AddProvider(provider.NewProvider(provider.ProviderConfig{
				ID:          id,
				Name:        raw.Name,
				Type:        provType,
				Endpoint:    raw.Endpoint,
				Token:       raw.Token,
				Model:       raw.Model,
				Temperature: raw.Temperature,
			}))
		}
	} else {
		if s.ProviderGroup != nil {
			s.ProviderGroup.DelProvider(id)
		}
	}

	if err := s.DAO.Provider.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ListMCPServers(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.MCPServer.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServerList2Resp(list)))
}

func (s *Service) GetMCPServer(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.MCPServer.GetByID(ctx, c.Param("id"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.MCPNotExist, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServer2Resp(raw)))
}

func (s *Service) AddMCPServer(ctx context.Context, c *app.RequestContext) {
	var data dto.AddMCPServerReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	m := models.MCPServer{
		ID:            newUUID(),
		Name:          data.Name,
		ServerURL:     data.ServerURL,
		Headers:       data.Headers,
		Timeout:       data.Timeout,
		RetryCount:    data.RetryCount,
		ToolFilter:    data.ToolFilter,
		AutoReconnect: data.AutoReconnect,
		IsActive:      data.IsActive,
	}
	if err := s.DAO.MCPServer.Create(ctx, &m); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：如果 Active，创建 MCP 客户端并连接
	if data.IsActive && s.MCPGroup != nil {
		client := mcp.NewSSEMCPClient(buildMcpSSEConfig(&m))
		if err := client.Connect(ctx); err != nil {
			// 连接失败不影响 DB 写入结果，仅记录日志
			slog.Error("MCP 连接失败", "name", m.Name, "err", err)
		} else {
			s.MCPGroup.AddMCP(client)
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServer2Resp(&m)))
}

func (s *Service) UpdateMCPServer(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateMCPServerReq
	id := c.Param("id")
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	m := models.MCPServer{
		ID:            id,
		Name:          data.Name,
		ServerURL:     data.ServerURL,
		Headers:       data.Headers,
		Timeout:       data.Timeout,
		RetryCount:    data.RetryCount,
		ToolFilter:    data.ToolFilter,
		AutoReconnect: data.AutoReconnect,
		IsActive:      data.IsActive,
	}
	if err := s.DAO.MCPServer.Update(ctx, &m); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：断开旧连接，若 Active 则重连
	if s.MCPGroup != nil {
		if old, ok := s.MCPGroup.GetMCP(id); ok {
			old.Disconnect()
			s.MCPGroup.DelMCP(id)
		}
		if data.IsActive {
			client := mcp.NewSSEMCPClient(buildMcpSSEConfig(&m))
			if err := client.Connect(ctx); err != nil {
				slog.Error("MCP 重连失败", "name", m.Name, "err", err)
			} else {
				s.MCPGroup.AddMCP(client)
			}
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServer2Resp(&m)))
}

func (s *Service) DeleteMCPServer(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	// 运行时同步：断开连接并移除
	if s.MCPGroup != nil {
		if old, ok := s.MCPGroup.GetMCP(id); ok {
			old.Disconnect()
			s.MCPGroup.DelMCP(id)
		}
	}

	if err := s.DAO.MCPServer.Delete(ctx, id); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ToggleMCPServer(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleMCPServerReq
	id := c.Param("id")
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步
	if s.MCPGroup != nil {
		if data.IsActive {
			raw, err := s.DAO.MCPServer.GetByID(ctx, id)
			if err == nil {
				client := mcp.NewSSEMCPClient(buildMcpSSEConfig(raw))
				if err := client.Connect(ctx); err != nil {
					slog.Error("MCP 连接失败", "id", id, "err", err)
				} else {
					s.MCPGroup.AddMCP(client)
				}
			}
		} else {
			if old, ok := s.MCPGroup.GetMCP(id); ok {
				old.Disconnect()
				s.MCPGroup.DelMCP(id)
			}
		}
	}

	if err := s.DAO.MCPServer.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Skill
// ====================================================================

func (s *Service) ListSkills(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Skill.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSkillList2Resp(list)))
}

func (s *Service) AddSkill(ctx context.Context, c *app.RequestContext) {
	var data dto.AddSkillReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	sk := models.Skill{
		ID:           newUUID(),
		Name:         data.Name,
		Description:  data.Description,
		Keywords:     data.Keywords,
		RegexPattern: data.RegexPattern,
		PromptRef:    data.PromptRef,
		ToolRefs:     data.ToolRefs,
		McpRefs:      data.McpRefs,
		IsActive:     data.IsActive,
		IsSystem:     data.IsSystem,
		Priority:     data.Priority,
	}
	if err := s.DAO.Skill.Create(ctx, &sk); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步
	if s.SkillEngine != nil {
		s.SkillEngine.AddSkill(buildSkillConfig(&sk))
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSkill2Resp(&sk)))
}

func (s *Service) UpdateSkill(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateSkillReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	sk := models.Skill{
		ID:           c.Param("id"),
		Name:         data.Name,
		Description:  data.Description,
		Keywords:     data.Keywords,
		RegexPattern: data.RegexPattern,
		PromptRef:    data.PromptRef,
		ToolRefs:     data.ToolRefs,
		McpRefs:      data.McpRefs,
		IsActive:     data.IsActive,
		IsSystem:     data.IsSystem,
		Priority:     data.Priority,
	}
	if err := s.DAO.Skill.Update(ctx, &sk); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：AddSkill 会按 ID 覆盖已有 Skill
	if s.SkillEngine != nil {
		s.SkillEngine.AddSkill(buildSkillConfig(&sk))
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSkill2Resp(&sk)))
}

func (s *Service) DeleteSkill(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	// 运行时同步
	if s.SkillEngine != nil {
		s.SkillEngine.DeleteSkill(id)
	}

	if err := s.DAO.Skill.Delete(ctx, id); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Prompt
// ====================================================================

func (s *Service) ListPrompts(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Prompt.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawPromptList2Resp(list)))
}

func (s *Service) AddPrompt(ctx context.Context, c *app.RequestContext) {
	var data dto.AddPromptReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	p := models.Prompt{
		ID:        newUUID(),
		Name:      data.Name,
		Content:   data.Content,
		Type:      data.Type,
		IsActive:  data.IsActive,
		Variables: data.Variables,
	}
	if err := s.DAO.Prompt.Create(ctx, &p); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawPrompt2Resp(&p)))
}

func (s *Service) UpdatePrompt(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdatePromptReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	p := models.Prompt{
		ID:        c.Param("id"),
		Name:      data.Name,
		Content:   data.Content,
		Type:      data.Type,
		IsActive:  data.IsActive,
		Variables: data.Variables,
	}
	if err := s.DAO.Prompt.Update(ctx, &p); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawPrompt2Resp(&p)))
}

func (s *Service) DeletePrompt(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Prompt.Delete(ctx, c.Param("id")); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) TogglePrompt(ctx context.Context, c *app.RequestContext) {
	var data dto.TogglePromptReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if err := s.DAO.Prompt.SetActive(ctx, c.Param("id"), data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Tool
// ====================================================================

func (s *Service) ListTools(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ToolConfig.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawToolConfigList2Resp(list)))
}

func (s *Service) ToggleTool(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleToolReq
	id := c.Param("id")
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：停用时从注册表移除；启用时内置工具已在 init 时注册，无需重复
	if s.ToolRegistry != nil && !data.IsActive {
		raw, err := s.DAO.ToolConfig.GetByID(ctx, id)
		if err == nil {
			s.ToolRegistry.Unregister(raw.Name)
		}
	}

	if err := s.DAO.ToolConfig.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Session
// ====================================================================

func (s *Service) ListSessions(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Session.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSessionList2Resp(list)))
}

func (s *Service) GetSession(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.Session.GetByID(ctx, c.Param("id"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.SessionNotExist, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSession2Resp(raw)))
}

func (s *Service) DeleteSession(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	// 使用 SessionManager 删除（同时清除 DB + Redis 缓存）
	if s.SessionMgr != nil {
		if err := s.SessionMgr.DeleteSession(ctx, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	} else {
		if err := s.DAO.Session.Delete(ctx, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Plugin
// ====================================================================

func (s *Service) ListPlugins(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Plugin.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawPluginList2Resp(list)))
}

func (s *Service) UploadPlugin(ctx context.Context, c *app.RequestContext) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.EmptyFileToUpload, nil))
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: "无法打开文件"}))
		return
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "pluggin-*.zip")
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.TempFileCreateFail, nil))
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, src); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.WriteFileFail, nil))
		return
	}
	tmpFile.Close()

	reader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.InvalidZipFile, nil))
		return
	}
	defer reader.Close()

	var pluginName string
	for _, f := range reader.File {
		if f.Name == "pluggin.yaml" || filepath.Base(f.Name) == "pluggin.yaml" {
			pluginName = filepath.Base(filepath.Dir(f.Name))
			if pluginName == "." {
				pluginName = filepath.Base(tmpFile.Name())
				pluginName = pluginName[:len(pluginName)-4]
			}
			break
		}
	}
	if pluginName == "" {
		pluginName = file.Filename
		pluginName = pluginName[:len(pluginName)-len(filepath.Ext(pluginName))]
	}

	destDir := filepath.Join("data/pluggins", pluginName)
	os.RemoveAll(destDir)
	os.MkdirAll(destDir, 0755)

	for _, f := range reader.File {
		path := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		dst, err := os.Create(path)
		if err != nil {
			continue
		}
		rc, _ := f.Open()
		io.Copy(dst, rc)
		rc.Close()
		dst.Close()
	}

	if s.PluginEngine != nil {
		if err := s.PluginEngine.Load(pluginName); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PluginLoadFail, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.PluginUploadResp{Name: pluginName, Status: "loaded"}))
}

func (s *Service) TogglePlugin(ctx context.Context, c *app.RequestContext) {
	var data dto.TogglePluginReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	id := c.Param("id")

	// 运行时同步：启用时加载插件，停用时卸载
	if s.PluginEngine != nil {
		if data.IsActive {
			if err := s.PluginEngine.Load(id); err != nil {
				slog.Error("插件加载失败", "id", id, "err", err)
			}
		} else {
			s.PluginEngine.Unload(id)
		}
	}

	if err := s.DAO.Plugin.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) DeletePlugin(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	// 运行时同步：先卸载再删除 DB 记录
	if s.PluginEngine != nil {
		s.PluginEngine.Unload(id)
	}

	if err := s.DAO.Plugin.Delete(ctx, id); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// ACL
// ====================================================================

func (s *Service) ListACLRules(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ACL.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawACLRuleList2Resp(list)))
}

func (s *Service) AddACLRule(ctx context.Context, c *app.RequestContext) {
	var data dto.AddACLRuleReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	r := models.ACLRule{
		ChatAreaID: data.ChatAreaID,
		Scope:      data.Scope,
		Permission: data.Permission,
		TargetType: data.TargetType,
		UserIDs:    data.UserIDs,
	}

	// 运行时同步：使用 ACL.AddRule（存在则更新）
	if s.ACLMgr != nil {
		if err := s.ACLMgr.AddRule(ctx, &r); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	} else {
		if err := s.DAO.ACL.Create(ctx, &r); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawACLRule2Resp(&r)))
}

func (s *Service) DeleteACLRule(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.InvalidACLID, nil))
		return
	}

	// 运行时同步
	if s.ACLMgr != nil {
		if err := s.ACLMgr.RemoveRule(ctx, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	} else {
		if err := s.DAO.ACL.Delete(ctx, id); err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Chat Records
// ====================================================================

func (s *Service) GetChatRecords(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	limit := atoi(c.DefaultQuery("limit", "20"))
	offset := atoi(c.DefaultQuery("offset", "0"))
	role := c.Query("role")

	if role != "" {
		list, total, err := s.DAO.ChatRecord.ListByChatAreaAndRole(ctx, chatAreaID, role, limit, offset)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.ChatRecordListResp{
			Total: total,
			List:  dto.RawChatRecordList2Resp(list),
		}))
		return
	}

	list, total, err := s.DAO.ChatRecord.ListByChatArea(ctx, chatAreaID, limit, offset)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.ChatRecordListResp{
		Total: total,
		List:  dto.RawChatRecordList2Resp(list),
	}))
}

func (s *Service) GetChatAreaTokenUsage(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	sess, err := s.DAO.Session.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSession2Resp(sess)))
}

func (s *Service) GetChatAreas(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ChatArea.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawChatAreaList2Resp(list)))
}

// ====================================================================
// Overview
// ====================================================================

func (s *Service) GetOverview(ctx context.Context, c *app.RequestContext) {
	chatAreaCount, _ := s.DAO.ChatArea.Count(ctx)

	mcpList, _ := s.DAO.MCPServer.List(ctx)
	mcpCount := int64(len(mcpList))

	pluginList, _ := s.DAO.Plugin.List(ctx)
	pluginCount := int64(len(pluginList))

	totalTokens, _ := s.DAO.ChatRecord.TotalTokenUsage(ctx)

	providerList, _ := s.DAO.Provider.List(ctx, "")
	skillList, _ := s.DAO.Skill.List(ctx)
	sessionList, _ := s.DAO.Session.List(ctx)

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.OverviewResp{
		ChatAreaCount:   chatAreaCount,
		MCPCount:        mcpCount,
		AdapterCount:    1,
		PluginCount:     pluginCount,
		ProviderCount:   len(providerList),
		SkillCount:      len(skillList),
		SessionCount:    len(sessionList),
		TotalTokenUsage: totalTokens,
	}))
}

// ====================================================================
// Memory
// ====================================================================

func (s *Service) GetShortTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	m, err := s.DAO.ShortTermMemory.GetOrCreate(ctx, c.Param("chatAreaID"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawShortTermMemory2Resp(m)))
}

func (s *Service) UpdateShortTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.ShortTermMemory.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	var data dto.UpdateShortTermMemoryReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	m.WindowSize = data.WindowSize
	m.AutoCompact = data.AutoCompact
	if err := s.DAO.ShortTermMemory.Update(ctx, m); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步
	if s.MemoryGroup != nil {
		s.MemoryGroup.UpdateShortTermConfig(memory.ShortTermMemoryConfig{
			WindowSize:  int64(data.WindowSize),
			AutoCompact: data.AutoCompact,
		})
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawShortTermMemory2Resp(m)))
}

func (s *Service) GetLongTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	m, err := s.DAO.LongTermMemory.GetOrCreate(ctx, c.Param("chatAreaID"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawLongTermMemory2Resp(m)))
}

func (s *Service) UpdateLongTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.LongTermMemory.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	var data dto.UpdateLongTermMemoryReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	m.HotAreaSize = data.HotAreaSize
	m.HotMemoryTTL = data.HotMemoryTTL
	if err := s.DAO.LongTermMemory.Update(ctx, m); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步
	if s.MemoryGroup != nil {
		s.MemoryGroup.UpdateLongTermConfig(memory.LongTermMemoryConfig{
			HotAreaSize:  data.HotAreaSize,
			HotMemoryTTL: time.Duration(data.HotMemoryTTL) * time.Second,
		})
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawLongTermMemory2Resp(m)))
}

// ---------- T2I ----------

func (s *Service) GetT2IConfig(ctx context.Context, c *app.RequestContext) {
	cfg, err := s.DAO.T2I.GetConfig(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.T2IConfigNotFound, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	healthy := false
	if s.T2IClient != nil {
		healthy = s.T2IClient.HealthCheck() == nil
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawT2IConfig2Resp(cfg, healthy)))
}

func (s *Service) UpdateT2IConfig(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateT2IConfigReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	cfg := &models.T2IConfig{
		ID:       1,
		BaseURL:  data.BaseURL,
		Timeout:  data.Timeout,
		IsActive: data.IsActive,
	}
	if err := s.DAO.T2I.UpdateConfig(ctx, cfg); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：创建新客户端并同步到 HagoCenter
	if data.IsActive {
		client := t2iClientFactory(data.BaseURL, data.Timeout)
		s.T2IClient = client
		if s.OnUpdateT2I != nil {
			s.OnUpdateT2I(client)
		}
	} else {
		s.T2IClient = nil
		if s.OnUpdateT2I != nil {
			s.OnUpdateT2I(nil)
		}
	}

	healthy := false
	if s.T2IClient != nil {
		healthy = s.T2IClient.HealthCheck() == nil
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawT2IConfig2Resp(cfg, healthy)))
}

func (s *Service) CheckT2IHealth(ctx context.Context, c *app.RequestContext) {
	if s.T2IClient == nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]bool{"healthy": false}))
		return
	}
	err := s.T2IClient.HealthCheck()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]bool{"healthy": err == nil}))
}

// ---------- Sandbox ----------

func (s *Service) GetSandboxConfig(ctx context.Context, c *app.RequestContext) {
	cfg, err := s.DAO.Sandbox.GetConfig(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.SandboxConfigNotFound, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	healthy := false
	if s.SandboxClient != nil {
		healthy = s.SandboxClient.HealthCheck() == nil
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSandboxConfig2Resp(cfg, healthy)))
}

func (s *Service) UpdateSandboxConfig(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateSandboxConfigReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	cfg := &models.SandboxConfig{
		ID:       1,
		BaseURL:  data.BaseURL,
		APIKey:   data.APIKey,
		Timeout:  data.Timeout,
		IsActive: data.IsActive,
	}
	if err := s.DAO.Sandbox.UpdateConfig(ctx, cfg); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步：创建新客户端并同步到 HagoCenter
	if data.IsActive {
		client := sandboxClientFactory(data.BaseURL, data.APIKey, data.Timeout)
		s.SandboxClient = client
		if s.OnUpdateSandbox != nil {
			s.OnUpdateSandbox(client)
		}
	} else {
		s.SandboxClient = nil
		if s.OnUpdateSandbox != nil {
			s.OnUpdateSandbox(nil)
		}
	}

	healthy := false
	if s.SandboxClient != nil {
		healthy = s.SandboxClient.HealthCheck() == nil
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSandboxConfig2Resp(cfg, healthy)))
}

func (s *Service) CheckSandboxHealth(ctx context.Context, c *app.RequestContext) {
	if s.SandboxClient == nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]bool{"healthy": false}))
		return
	}
	err := s.SandboxClient.HealthCheck()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]bool{"healthy": err == nil}))
}

// ---------- Webhook ----------

func (s *Service) GetWebhookConfig(ctx context.Context, c *app.RequestContext) {
	cfg, err := s.DAO.Webhook.GetConfig(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	running := s.WebhookAdapter != nil && s.WebhookAdapter.IsRunning()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.WebhookConfigResp{
		Addr:    cfg.Addr,
		Port:    cfg.Port,
		Token:   cfg.Token,
		Enabled: cfg.Enabled,
		Running: running,
	}))
}

func (s *Service) UpdateWebhookConfig(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateWebhookConfigReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	cfg := &models.WebhookConfig{
		ID:      1,
		Addr:    data.Addr,
		Port:    data.Port,
		Token:   data.Token,
		Enabled: data.Enabled,
	}
	if err := s.DAO.Webhook.UpdateConfig(ctx, cfg); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 运行时同步
	if s.WebhookAdapter != nil {
		conf := adapter.WebhookConfig{
			Addr:    data.Addr,
			Port:    data.Port,
			Token:   data.Token,
			Enable:  data.Enabled,
			Admins:  s.Adapter.Admins(),
		}
		if err := s.WebhookAdapter.SyncConfig(ctx, conf); err != nil {
			slog.Error("webhook adapter 配置同步失败", "err", err)
		}
	}

	running := s.WebhookAdapter != nil && s.WebhookAdapter.IsRunning()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.WebhookConfigResp{
		Addr:    cfg.Addr,
		Port:    cfg.Port,
		Token:   cfg.Token,
		Enabled: cfg.Enabled,
		Running: running,
	}))
}

// ---------- Logs ----------

// GetLogs 返回最近 250 条日志。
func (s *Service) GetLogs(ctx context.Context, c *app.RequestContext) {
	if s.LogHub == nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, []dto.LogEntryResp{}))
		return
	}
	entries := s.LogHub.Recent()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawLogEntryList2Resp(entries)))
}

// StreamLogs 通过 SSE 推送实时日志。
//
// 连接建立后：
//  1. 先发送最近 250 条历史日志（按时间顺序）
//  2. 然后订阅 Hub，实时推送新日志
//  3. 客户端断开或服务停止时退出
func (s *Service) StreamLogs(ctx context.Context, c *app.RequestContext) {
	if s.LogHub == nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: "log hub 未初始化"}))
		return
	}

	w := sse.NewWriter(c)
	defer w.Close()

	// 1. 推送历史日志
	for _, entry := range s.LogHub.Recent() {
		if err := writeLogEvent(w, entry); err != nil {
			return
		}
	}

	// 2. 订阅新日志
	ch := s.LogHub.Subscribe()
	defer s.LogHub.Unsubscribe(ch)

	// 心跳定时器：保持代理/中间件不会因为空闲而关连接，
	// 同时让写失败能被快速检测到（客户端已断开）。
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if err := writeLogEvent(w, entry); err != nil {
				return
			}
		case <-ticker.C:
			if err := w.WriteKeepAlive(); err != nil {
				return
			}
		}
	}
}

// writeLogEvent 把单条日志序列化为 JSON 并通过 SSE 发出。
func writeLogEvent(w *sse.Writer, entry logging.Entry) error {
	data, err := json.Marshal(dto.LogEntryResp{
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
		Attrs:   entry.Attrs,
	})
	if err != nil {
		return err
	}
	return w.WriteEvent("", "log", data)
}

// ---------- helpers ----------

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func atoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// buildMcpSSEConfig 从 DB model 构建 MCP SSE 客户端配置。
func buildMcpSSEConfig(m *models.MCPServer) mcp.McpSSEConfig {
	headers := make(map[string]string)
	for k, v := range m.Headers {
		if str, ok := v.(string); ok {
			headers[k] = str
		}
	}
	return mcp.McpSSEConfig{
		ID:            m.ID,
		Name:          m.Name,
		ServerURL:     m.ServerURL,
		Headers:       headers,
		Timeout:       0,
		RetryCount:    m.RetryCount,
		ToolFilter:    m.ToolFilter,
		AutoReconnect: m.AutoReconnect,
	}
}

// buildSkillConfig 从 DB model 构建 Skill 运行时配置。
func buildSkillConfig(s *models.Skill) *skill.SkillConfig {
	return &skill.SkillConfig{
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
	}
}
