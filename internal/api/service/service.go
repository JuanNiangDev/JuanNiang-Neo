package service

import (
	"JuanNiang-Neo/internal/api/dto"
	"archive/zip"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/core/models"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
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

func (s *Service) ListAdminQQs(ctx context.Context, c *app.RequestContext) {
	raw, err := s.DAO.AdminQQ.List(ctx)

	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	list := dto.Model2Resp_AdminQQ(raw)

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, list))
}

func (s *Service) AddAdminQQ(ctx context.Context, c *app.RequestContext) {
	var data dto.AddAdminQQReq

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if data.QQ <= 0 {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.InvalidQQNumber, nil))
		return
	}
	if err := s.DAO.AdminQQ.Add(ctx, data.QQ); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	admins, err := s.DAO.AdminQQ.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	qqs := make([]int64, len(admins))
	for i, a := range admins {
		qqs[i] = a.ID
	}

	conf := s.Adapter.GetCurrentConfig()
	conf.Admins = qqs

	if err := s.Adapter.UpdateConfig(ctx, conf); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) DeleteAdminQQ(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	qq, _ := strconv.ParseInt(id, 10, 64)
	if qq <= 0 {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.InvalidQQNumber, nil))
		return
	}
	if err := s.DAO.AdminQQ.Remove(ctx, qq); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	admins, err := s.DAO.AdminQQ.List(ctx)
	if err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	qqs := make([]int64, len(admins))
	for i, a := range admins {
		qqs[i] = a.ID
	}

	conf := s.Adapter.GetCurrentConfig()
	conf.Admins = qqs

	if err := s.Adapter.UpdateConfig(ctx, conf); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) GetAdapterStatus(ctx context.Context, c *app.RequestContext) {
	raw := s.Adapter.Status()

	status := dto.ProviderStatus{
		Running:    raw.Running,
		ListenAddr: raw.ListenAddr,
		SelfID:     raw.SelfID,
		ConnCount:  raw.ConnCount,
		ConnIDs:    raw.ConnIDs,
	}

	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, status))
}

func (s *Service) UpdateAdapterConfig(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateAdapterConfigReq

	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if err := s.DAO.Onebot11Adapter.

	conf := s.Adapter.GetCurrentConfig()
	if conf.Token != data.Token {
		conf.Token
	}

	if err := s.AdapterCb.UpdateConfig(req.Token, req.Admins); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, s.AdapterCb.Status())
}

func (s *Service) RestartAdapter(ctx context.Context, c *app.RequestContext) {
	if s.AdapterCb == nil {
		fail(c, 500, "adapter 未初始化")
		return
	}
	if err := s.AdapterCb.Restart(); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, s.AdapterCb.Status())
}

// ====================================================================
// Provider
// ====================================================================

func (s *Service) ListProviders(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Provider.List(ctx, "")
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) GetProvider(ctx context.Context, c *app.RequestContext) {
	p, err := s.DAO.Provider.GetByID(ctx, c.Param("id"))
	if err != nil {
		fail(c, 404, "Provider 不存在")
		return
	}
	ok(c, p)
}

func (s *Service) AddProvider(ctx context.Context, c *app.RequestContext) {
	var p models.Provider
	if err := c.BindJSON(&p); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	p.ID = newUUID()
	if err := s.DAO.Provider.Create(ctx, &p); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, p)
}

func (s *Service) UpdateProvider(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var p models.Provider
	if err := c.BindJSON(&p); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	p.ID = id
	if err := s.DAO.Provider.Update(ctx, &p); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, p)
}

func (s *Service) DeleteProvider(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Provider.Delete(ctx, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

func (s *Service) ToggleProvider(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.Provider.SetActive(ctx, c.Param("id"), req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// MCP
// ====================================================================

func (s *Service) ListMCPServers(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.MCPServer.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) GetMCPServer(ctx context.Context, c *app.RequestContext) {
	m, err := s.DAO.MCPServer.GetByID(ctx, c.Param("id"))
	if err != nil {
		fail(c, 404, "MCP 服务器不存在")
		return
	}
	ok(c, m)
}

func (s *Service) AddMCPServer(ctx context.Context, c *app.RequestContext) {
	var m models.MCPServer
	if err := c.BindJSON(&m); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	m.ID = newUUID()
	if err := s.DAO.MCPServer.Create(ctx, &m); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

func (s *Service) UpdateMCPServer(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var m models.MCPServer
	if err := c.BindJSON(&m); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	m.ID = id
	if err := s.DAO.MCPServer.Update(ctx, &m); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

func (s *Service) DeleteMCPServer(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.MCPServer.Delete(ctx, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

func (s *Service) ToggleMCPServer(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.MCPServer.SetActive(ctx, c.Param("id"), req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// Skill
// ====================================================================

func (s *Service) ListSkills(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Skill.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) AddSkill(ctx context.Context, c *app.RequestContext) {
	var sk models.Skill
	if err := c.BindJSON(&sk); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	sk.ID = newUUID()
	if err := s.DAO.Skill.Create(ctx, &sk); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, sk)
}

func (s *Service) UpdateSkill(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var sk models.Skill
	if err := c.BindJSON(&sk); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	sk.ID = id
	if err := s.DAO.Skill.Update(ctx, &sk); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, sk)
}

func (s *Service) DeleteSkill(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Skill.Delete(ctx, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// Prompt
// ====================================================================

func (s *Service) ListPrompts(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Prompt.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) AddPrompt(ctx context.Context, c *app.RequestContext) {
	var p models.Prompt
	if err := c.BindJSON(&p); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	p.ID = newUUID()
	if err := s.DAO.Prompt.Create(ctx, &p); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, p)
}

func (s *Service) UpdatePrompt(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var p models.Prompt
	if err := c.BindJSON(&p); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	p.ID = id
	if err := s.DAO.Prompt.Update(ctx, &p); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, p)
}

func (s *Service) DeletePrompt(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Prompt.Delete(ctx, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

func (s *Service) TogglePrompt(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.Prompt.SetActive(ctx, c.Param("id"), req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// Tool
// ====================================================================

func (s *Service) ListTools(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ToolConfig.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) ToggleTool(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.ToolConfig.SetActive(ctx, c.Param("id"), req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// Session
// ====================================================================

func (s *Service) ListSessions(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Session.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) GetSession(ctx context.Context, c *app.RequestContext) {
	sess, err := s.DAO.Session.GetByID(ctx, c.Param("id"))
	if err != nil {
		fail(c, 404, "Session 不存在")
		return
	}
	ok(c, sess)
}

func (s *Service) DeleteSession(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Session.Delete(ctx, c.Param("id")); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// Plugin
// ====================================================================

func (s *Service) ListPlugins(ctx context.Context, c *app.RequestContext) {
	if s.PluginEngine != nil {
		ok(c, s.PluginEngine.List())
		return
	}
	list, err := s.DAO.Plugin.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) UploadPlugin(ctx context.Context, c *app.RequestContext) {
	file, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, "缺少上传文件")
		return
	}

	src, err := file.Open()
	if err != nil {
		fail(c, 500, "无法打开文件")
		return
	}
	defer src.Close()

	tmpFile, err := os.CreateTemp("", "pluggin-*.zip")
	if err != nil {
		fail(c, 500, "临时文件创建失败")
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, src); err != nil {
		fail(c, 500, "文件写入失败")
		return
	}
	tmpFile.Close()

	reader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		fail(c, 400, "无效的 ZIP 文件")
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
			fail(c, 500, "插件加载失败: "+err.Error())
			return
		}
	}

	ok(c, map[string]string{"name": pluginName, "status": "loaded"})
}

func (s *Service) TogglePlugin(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.Plugin.SetActive(ctx, c.Param("id"), req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

func (s *Service) DeletePlugin(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if s.PluginEngine != nil {
		s.PluginEngine.Unload(id)
	}
	if err := s.DAO.Plugin.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ====================================================================
// ACL
// ====================================================================

func (s *Service) ListACLRules(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ACL.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) AddACLRule(ctx context.Context, c *app.RequestContext) {
	var r models.ACLRule
	if err := c.BindJSON(&r); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	if err := s.DAO.ACL.Create(ctx, &r); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, r)
}

func (s *Service) DeleteACLRule(ctx context.Context, c *app.RequestContext) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		fail(c, 400, "无效的 ID")
		return
	}
	if err := s.DAO.ACL.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
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
			fail(c, 500, err.Error())
			return
		}
		ok(c, map[string]any{"total": total, "list": list})
		return
	}

	list, total, err := s.DAO.ChatRecord.ListByChatArea(ctx, chatAreaID, limit, offset)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, map[string]any{"total": total, "list": list})
}

func (s *Service) GetChatAreaTokenUsage(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	sess, err := s.DAO.Session.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, map[string]any{"chat_area_id": chatAreaID, "token_usage": sess.TokenUsage})
}

func (s *Service) GetChatAreas(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ChatArea.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
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

	providerCount, _ := s.DAO.Provider.List(ctx, "")
	skillCount, _ := s.DAO.Skill.List(ctx)
	sessionCount, _ := s.DAO.Session.List(ctx)

	ok(c, map[string]any{
		"chat_area_count":   chatAreaCount,
		"mcp_count":         mcpCount,
		"adapter_count":     1,
		"plugin_count":      pluginCount,
		"provider_count":    len(providerCount),
		"skill_count":       len(skillCount),
		"session_count":     len(sessionCount),
		"total_token_usage": totalTokens,
	})
}

// ====================================================================
// Memory
// ====================================================================

func (s *Service) GetShortTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	m, err := s.DAO.ShortTermMemory.GetOrCreate(ctx, c.Param("chatAreaID"))
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

func (s *Service) UpdateShortTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.ShortTermMemory.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	var req struct {
		WindowSize  int  `json:"window_size"`
		AutoCompact bool `json:"auto_compact"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	m.WindowSize = req.WindowSize
	m.AutoCompact = req.AutoCompact
	if err := s.DAO.ShortTermMemory.Update(ctx, m); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

func (s *Service) GetLongTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	m, err := s.DAO.LongTermMemory.GetOrCreate(ctx, c.Param("chatAreaID"))
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

func (s *Service) UpdateLongTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.LongTermMemory.GetOrCreate(ctx, chatAreaID)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	var req struct {
		HotAreaSize  int `json:"hot_area_size"`
		HotMemoryTTL int `json:"hot_memory_ttl"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "参数格式错误")
		return
	}
	m.HotAreaSize = req.HotAreaSize
	m.HotMemoryTTL = req.HotMemoryTTL
	if err := s.DAO.LongTermMemory.Update(ctx, m); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, m)
}

// helpers

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
