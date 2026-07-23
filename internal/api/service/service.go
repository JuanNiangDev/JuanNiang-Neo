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
	"runtime"
	"strconv"
	"strings"
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
		Conns:      raw.Conns,
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, status))
}

func (s *Service) GetAdapterConfig(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.Onebot11Adapter.GetAdapterConfig(ctx)
	if err != nil {
		// 数据库无配置 → 初始化默认配置再读取一次, 避免前端报 "record not found"
		if initErr := s.DAO.Onebot11Adapter.InitAdapterConfig(ctx); initErr != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: fmt.Sprintf("init: %v; query: %v", initErr, err)}))
			return
		}
		raw, err = s.DAO.Onebot11Adapter.GetAdapterConfig(ctx)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}
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
		Temperature: float32(data.Temperature),
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
			Temperature: float32(data.Temperature),
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
		Temperature: float32(data.Temperature),
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
		Temperature: float32(data.Temperature),
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
		Headers:       ensureNonNilMap(data.Headers),
		Timeout:       data.Timeout,
		RetryCount:    data.RetryCount,
		ToolFilter:    ensureNonNilSlice(data.ToolFilter),
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
		Headers:       ensureNonNilMap(data.Headers),
		Timeout:       data.Timeout,
		RetryCount:    data.RetryCount,
		ToolFilter:    ensureNonNilSlice(data.ToolFilter),
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

func (s *Service) CheckMCPServer(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	raw, err := s.DAO.MCPServer.GetByID(ctx, id)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.MCPNotExist, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	client := mcp.NewSSEMCPClient(buildMcpSSEConfig(raw))
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Connect(checkCtx); err != nil {
		client.Disconnect()
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]any{
			"healthy": false,
			"error":   err.Error(),
		}))
		return
	}
	client.Disconnect()
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, map[string]any{
		"healthy": true,
		"error":   "",
	}))
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

	// 禁止用户创建 system 类型（system 类型仅由系统锁定提示词使用）
	if data.Type == models.PromptTypeSystem {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PromptIsSystem, dto.ErrorDetail{ErrorDetail: "system 类型由系统保留，请使用 personality 或 custom"}))
		return
	}

	p := models.Prompt{
		ID:       newUUID(),
		Name:     data.Name,
		Content:  data.Content,
		Type:     data.Type,
		IsActive: data.IsActive,
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

	// 系统锁定提示词禁止修改
	id := c.Param("id")
	existing, err := s.DAO.Prompt.GetByID(ctx, id)
	if err == nil && existing != nil && existing.IsSystem {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PromptIsSystem, dto.ErrorDetail{ErrorDetail: "系统锁定提示词不允许修改"}))
		return
	}

	// 禁止用户将类型改为 system
	if data.Type == models.PromptTypeSystem {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PromptIsSystem, dto.ErrorDetail{ErrorDetail: "system 类型由系统保留，请使用 personality 或 custom"}))
		return
	}

	p := models.Prompt{
		ID:       id,
		Name:     data.Name,
		Content:  data.Content,
		Type:     data.Type,
		IsActive: data.IsActive,
	}
	if err := s.DAO.Prompt.Update(ctx, &p); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawPrompt2Resp(&p)))
}

func (s *Service) DeletePrompt(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")

	// 系统锁定提示词禁止删除
	existing, err := s.DAO.Prompt.GetByID(ctx, id)
	if err == nil && existing != nil && existing.IsSystem {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PromptIsSystem, dto.ErrorDetail{ErrorDetail: "系统锁定提示词不允许删除"}))
		return
	}

	if err := s.DAO.Prompt.Delete(ctx, id); err != nil {
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

	// 系统锁定提示词禁止停用（但允许启用）
	id := c.Param("id")
	if !data.IsActive {
		existing, err := s.DAO.Prompt.GetByID(ctx, id)
		if err == nil && existing != nil && existing.IsSystem {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PromptIsSystem, dto.ErrorDetail{ErrorDetail: "系统锁定提示词不允许停用"}))
			return
		}
	}

	if err := s.DAO.Prompt.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ====================================================================
// Tool
// ====================================================================

func (s *Service) ListTools(ctx context.Context, c *app.RequestContext) {
	// 1. 读取数据库中所有 ToolConfig (代表可启用/停用的条目, 包括用户自定义与历史保存的内置工具条目)
	dbList, err := s.DAO.ToolConfig.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	// name → resp, 便于运行时内置工具查表合并 DB 状态
	byName := make(map[string]dto.ToolConfigResp, len(dbList))
	for _, item := range dbList {
		byName[item.Name] = dto.RawToolConfig2Resp(&item)
	}

	// 2. 合并运行时 ToolRegistry 中的内置工具 (这部分工具始终启用, 不由 DB 控制启停)
	out := make([]dto.ToolConfigResp, 0, len(dbList)+8)
	seen := make(map[string]bool)
	if s.ToolRegistry != nil {
		for _, t := range s.ToolRegistry.List() {
			if !t.IsBuiltin() {
				continue
			}
			name := t.Name()
			seen[name] = true
			paramsJSON, _ := json.Marshal(t.Parameters())
			resp := dto.ToolConfigResp{
				ID:          "builtin:" + name,
				Name:        name,
				Description: t.Description(),
				Parameters:  models.JSONMap{},
				Timeout:     0,
				IsActive:    true, // 内置工具运行时常驻
				IsBuiltin:   true,
			}
			_ = json.Unmarshal(paramsJSON, &resp.Parameters)
			// 若 DB 中有对应 name 的 ToolConfig, 合并其状态
			if db, ok := byName[name]; ok {
				resp.ID = db.ID
				resp.IsActive = db.IsActive
				resp.CreatedAt = db.CreatedAt
			}
			out = append(out, resp)
		}
	}

	// 3. 追加 DB 中的非内置条目 (用户自定义工具)
	for _, item := range dbList {
		if seen[item.Name] {
			continue
		}
		out = append(out, dto.RawToolConfig2Resp(&item))
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, out))
}

func (s *Service) ToggleTool(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleToolReq
	id := c.Param("id")
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 内置工具: DB 不一定有对应记录, 仅当存在记录时才切换 DB 状态。
	// 内置工具运行时始终在注册表中, 不允许真正"停用"——DB 拒绝切换。
	if strings.HasPrefix(id, "builtin:") {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ToolIsBuiltin, dto.ErrorDetail{ErrorDetail: "内置工具运行时常驻, 不支持启停"}))
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
	// 优先使用 PluginEngine 的运行时列表（包含 manifest 信息与 is_system 标志）
	if s.PluginEngine != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, s.PluginEngine.ListMaps()))
		return
	}
	// 回退到 DB（仅静态记录）
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

	name := c.Param("id")

	// 系统插件禁止停用（但允许"启用"——幂等场景）
	if !data.IsActive && s.PluginEngine != nil && s.PluginEngine.IsSystem(name) {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PluginIsSystem, dto.ErrorDetail{ErrorDetail: "系统插件不允许停用"}))
		return
	}

	// 运行时同步：启用时加载插件，停用时卸载
	if s.PluginEngine != nil {
		if data.IsActive {
			if err := s.PluginEngine.Load(name); err != nil {
				slog.Error("插件加载失败", "name", name, "err", err)
			}
		} else {
			if err := s.PluginEngine.Unload(name); err != nil {
				slog.Error("插件卸载失败", "name", name, "err", err)
			}
		}
	}

	// 持久化启用/停用状态到插件自身的 pluggin.yaml
	if s.PluginEngine != nil {
		if err := s.PluginEngine.SetEnabled(name, data.IsActive); err != nil {
			slog.Warn("写入插件 enabled 状态失败", "name", name, "err", err)
		}
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) DeletePlugin(ctx context.Context, c *app.RequestContext) {
	name := c.Param("id")

	// 系统插件禁止删除
	if s.PluginEngine != nil && s.PluginEngine.IsSystem(name) {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.PluginIsSystem, dto.ErrorDetail{ErrorDetail: "系统插件不允许删除"}))
		return
	}

	// 运行时同步：先卸载
	if s.PluginEngine != nil {
		if err := s.PluginEngine.Unload(name); err != nil {
			// 非系统插件的卸载错误仅记录，不阻断删除流程
			slog.Warn("插件卸载失败（继续删除流程）", "name", name, "err", err)
		}
	}

	// 通过 name 查找 DB 中的 Plugin 记录获取 UUID，然后删除 DB 记录
	dbPlugin, err := s.DAO.Plugin.GetByName(ctx, name)
	if err == nil {
		if err := s.DAO.Plugin.Delete(ctx, dbPlugin.ID); err != nil {
			slog.Warn("插件 DB 记录删除失败", "name", name, "err", err)
		}
	}

	// 删除插件目录
	pluginDir := filepath.Join("data/pluggins", name)
	if err := os.RemoveAll(pluginDir); err != nil {
		slog.Warn("插件目录删除失败", "dir", pluginDir, "err", err)
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
		ToolIDs:    data.ToolIDs,
		MCPIDs:     data.MCPIDs,
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

	// Plugin 数量优先取运行时 PluginEngine (包含 manifest 加载的插件)，
	// 否则回退到 DB 记录数。
	var pluginCount int64
	if s.PluginEngine != nil {
		pluginCount = int64(len(s.PluginEngine.ListMaps()))
	} else {
		pluginList, _ := s.DAO.Plugin.List(ctx)
		pluginCount = int64(len(pluginList))
	}

	totalTokens, _ := s.DAO.ChatRecord.TotalTokenUsage(ctx)

	providerList, _ := s.DAO.Provider.List(ctx, "")
	skillList, _ := s.DAO.Skill.List(ctx)
	sessionList, _ := s.DAO.Session.List(ctx)

	// 系统状态
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// T2I / Sandbox 状态
	t2iActive := s.T2IClient != nil
	t2iHealthy := false
	if t2iActive {
		t2iHealthy = s.T2IClient.HealthCheck() == nil
	}
	sandboxActive := s.SandboxClient != nil
	sandboxHealthy := false
	if sandboxActive {
		sandboxHealthy = s.SandboxClient.HealthCheck() == nil
	}

	// Adapter 运行状态
	adapterRunning := s.Adapter.Status().Running

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.OverviewResp{
		ChatAreaCount:   chatAreaCount,
		MCPCount:        mcpCount,
		AdapterCount:    1,
		PluginCount:     pluginCount,
		ProviderCount:   len(providerList),
		SkillCount:      len(skillList),
		SessionCount:    len(sessionList),
		TotalTokenUsage: totalTokens,

		CPUCount:          runtime.NumCPU(),
		GoroutineNum:      runtime.NumGoroutine(),
		MemAllocBytes:     memStats.Alloc,
		MemSysBytes:       memStats.Sys,
		MemHeapInUseBytes: memStats.HeapInuse,
		GoVersion:         runtime.Version(),

		AdapterRunning: adapterRunning,
		T2IActive:      t2iActive,
		T2IHealthy:     t2iHealthy,
		SandboxActive:  sandboxActive,
		SandboxHealthy: sandboxHealthy,
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
			Addr:   data.Addr,
			Port:   data.Port,
			Token:  data.Token,
			Enable: data.Enabled,
			Admins: s.Adapter.Admins(),
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

// GetLogs 返回最近 250 条日志，最新的在最前。
func (s *Service) GetLogs(ctx context.Context, c *app.RequestContext) {
	if s.LogHub == nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, []dto.LogEntryResp{}))
		return
	}
	entries := s.LogHub.Recent()
	// 反转为倒序: 最新写入的排最前
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
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

// ---------- Background Tasks ----------

// ListBackgroundTasks 返回所有后台任务。
func (s *Service) ListBackgroundTasks(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.BackgroundTask.ListAll(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	resp := make([]dto.BackgroundTaskResp, 0, len(list))
	for _, t := range list {
		resp = append(resp, dto.BackgroundTaskResp{
			ID:          t.ID,
			ChatAreaID:  t.ChatAreaID,
			Status:      string(t.Status),
			MessageType: t.MessageType,
			TargetID:    t.TargetID,
			UserPrompt:  t.UserPrompt,
			Steps:       t.Steps,
			Results:     t.Results,
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		})
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, resp))
}

// GetBackgroundTask 返回单个后台任务详情。
func (s *Service) GetBackgroundTask(ctx context.Context, c *app.RequestContext) {
	t, err := s.DAO.BackgroundTask.GetByID(ctx, c.Param("id"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.BackgroundTaskResp{
		ID:          t.ID,
		ChatAreaID:  t.ChatAreaID,
		Status:      string(t.Status),
		MessageType: t.MessageType,
		TargetID:    t.TargetID,
		UserPrompt:  t.UserPrompt,
		Steps:       t.Steps,
		Results:     t.Results,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}))
}

// ---------- CronJob ----------

func (s *Service) ListCronJobs(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.CronJob.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawCronJobList2Resp(list)))
}

func (s *Service) GetCronJob(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.CronJob.GetByID(ctx, c.Param("id"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawCronJob2Resp(raw)))
}

func (s *Service) AddCronJob(ctx context.Context, c *app.RequestContext) {
	var data dto.AddCronJobReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if data.MessageType == "" {
		data.MessageType = "private"
	}

	m := models.CronJob{
		Name:        data.Name,
		CronExpr:    data.CronExpr,
		Message:     data.Message,
		MessageType: data.MessageType,
		TargetID:    data.TargetID,
		IsActive:    data.IsActive,
	}
	if err := s.DAO.CronJob.Create(ctx, &m); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 同步调度器
	if s.CronJobManager != nil {
		_ = s.CronJobManager.Reload(ctx)
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawCronJob2Resp(&m)))
}

func (s *Service) UpdateCronJob(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateCronJobReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	raw, err := s.DAO.CronJob.GetByID(ctx, c.Param("id"))
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if data.MessageType == "" {
		data.MessageType = "private"
	}

	raw.Name = data.Name
	raw.CronExpr = data.CronExpr
	raw.Message = data.Message
	raw.MessageType = data.MessageType
	raw.TargetID = data.TargetID
	raw.IsActive = data.IsActive

	if err := s.DAO.CronJob.Update(ctx, raw); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 同步调度器
	if s.CronJobManager != nil {
		_ = s.CronJobManager.Reload(ctx)
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawCronJob2Resp(raw)))
}

func (s *Service) DeleteCronJob(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.CronJob.Delete(ctx, c.Param("id")); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 同步调度器
	if s.CronJobManager != nil {
		_ = s.CronJobManager.Reload(ctx)
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ToggleCronJob(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleCronJobReq
	id := c.Param("id")
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if err := s.DAO.CronJob.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 同步调度器
	if s.CronJobManager != nil {
		_ = s.CronJobManager.Reload(ctx)
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

// ---------- 回复策略 ----------

func (s *Service) GetReplyStrategy(ctx context.Context, c *app.RequestContext) {
	cfg, err := s.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.ReplyStrategyResp{
		Strategy:           string(cfg.Strategy),
		RelevanceThreshold: cfg.RelevanceThreshold,
		BotName:            cfg.BotName,
	}))
}

func (s *Service) UpdateReplyStrategy(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateReplyStrategyReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	// 验证策略值
	validStrategies := map[string]bool{
		string(models.StrategyNeverReply): true,
		string(models.StrategyAtOnly):     true,
		string(models.StrategyAlways):     true,
		string(models.StrategyPluginOnly): true,
		string(models.StrategyRelevance):  true,
	}
	if !validStrategies[data.Strategy] {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 40031, Info: "无效的回复策略"}, nil))
		return
	}

	// 验证阈值
	if data.Strategy == string(models.StrategyRelevance) {
		if data.RelevanceThreshold < 0 || data.RelevanceThreshold > 1 {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.Response{Status: 40032, Info: "相关性阈值必须在 0-1 之间"}, nil))
			return
		}
		if data.RelevanceThreshold == 0 {
			data.RelevanceThreshold = 0.5
		}
	}

	cfg, err := s.DAO.ReplyStrategy.GetOrCreate(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	cfg.Strategy = models.ReplyStrategy(data.Strategy)
	cfg.RelevanceThreshold = data.RelevanceThreshold
	cfg.BotName = data.BotName

	if err := s.DAO.ReplyStrategy.Update(ctx, cfg); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.ReplyStrategyResp{
		Strategy:           string(cfg.Strategy),
		RelevanceThreshold: cfg.RelevanceThreshold,
		BotName:            cfg.BotName,
	}))
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

func ensureNonNilMap(m models.JSONMap) models.JSONMap {
	if m == nil {
		return make(models.JSONMap)
	}
	return m
}

func ensureNonNilSlice(s models.JSONSlice) models.JSONSlice {
	if s == nil {
		return make(models.JSONSlice, 0)
	}
	return s
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
