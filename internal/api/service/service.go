package service

import (
	"JuanNiang-Neo/internal/adapter"
	"JuanNiang-Neo/internal/agent/provider"
	"JuanNiang-Neo/internal/api/dto"
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/core/models"
	"archive/zip"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

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

	providerConfig := models.Provider{
		ID:          newUUID(),
		Name:        data.Name,
		Type:        data.Type,
		Endpoint:    data.Endpoint,
		Token:       data.Token,
		Model:       data.Model,
		Temperature: data.Temperature,
		IsActive:    data.IsActive,
	}

	if err := s.DAO.Provider.Create(ctx, &providerConfig); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
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

	providerConfig_ := provider.ProviderConfig{
		ID:          id,
		Name:        data.Name,
		Type:        provider.ModelType(data.Type),
		Endpoint:    data.Endpoint,
		Token:       data.Token,
		Model:       data.Model,
		Temperature: data.Temperature,
	}

	s.ProviderGroup.SyncConfig(providerConfig_)

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

	if err := s.DAO.Provider.SetActive(ctx, id, data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	if data.IsActive {
		raw, err := s.DAO.Provider.GetByID(ctx, id)
		if err != nil {
			c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
			return
		}

		provideConfig := provider.ProviderConfig{
			ID:          id,
			Name:        raw.Name,
			Type:        provider.ModelType(raw.Type),
			Endpoint:    raw.Endpoint,
			Token:       raw.Token,
			Model:       raw.Model,
			Temperature: raw.Temperature,
		}

		s.ProviderGroup.AddProvider(provider.NewProvider(provideConfig))
	} else {
		s.ProviderGroup.DelProvider(id)
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
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServer2Resp(&m)))
}

func (s *Service) UpdateMCPServer(ctx context.Context, c *app.RequestContext) {
	var data dto.UpdateMCPServerReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}

	m := models.MCPServer{
		ID:            c.Param("id"),
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
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawMCPServer2Resp(&m)))
}

func (s *Service) DeleteMCPServer(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.MCPServer.Delete(ctx, c.Param("id")); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) ToggleMCPServer(ctx context.Context, c *app.RequestContext) {
	var data dto.ToggleMCPServerReq
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if err := s.DAO.MCPServer.SetActive(ctx, c.Param("id"), data.IsActive); err != nil {
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
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawSkill2Resp(&sk)))
}

func (s *Service) DeleteSkill(ctx context.Context, c *app.RequestContext) {
	if err := s.DAO.Skill.Delete(ctx, c.Param("id")); err != nil {
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
	if err := c.BindJSON(&data); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.BindJSONErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	if err := s.DAO.ToolConfig.SetActive(ctx, c.Param("id"), data.IsActive); err != nil {
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
	if err := s.DAO.Session.Delete(ctx, c.Param("id")); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
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
	if err := s.DAO.Plugin.SetActive(ctx, c.Param("id"), data.IsActive); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, nil))
}

func (s *Service) DeletePlugin(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
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
		UserID:     data.UserID,
		ChatAreaID: data.ChatAreaID,
		Permission: data.Permission,
		Actions:    data.Actions,
	}
	if err := s.DAO.ACL.Create(ctx, &r); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
	}
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawACLRule2Resp(&r)))
}

func (s *Service) DeleteACLRule(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.InvalidACLID, nil))
		return
	}
	if err := s.DAO.ACL.Delete(ctx, id); err != nil {
		c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.ServerInternalErr, dto.ErrorDetail{ErrorDetail: err.Error()}))
		return
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
	c.JSON(consts.StatusOK, dto.GenFinalResponse(dto.OK, dto.RawLongTermMemory2Resp(m)))
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
