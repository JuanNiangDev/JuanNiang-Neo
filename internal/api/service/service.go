package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"

	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/core/dao"
	"JuanNiang-Neo/internal/core/models"

	"github.com/cloudwego/hertz/pkg/app"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	DAO *dao.Bundle
}

func New(dao *dao.Bundle) *Service {
	return &Service{DAO: dao}
}

func ok(c *app.RequestContext, data any) {
	c.JSON(http.StatusOK, map[string]any{"code": 0, "data": data})
}

func fail(c *app.RequestContext, code int, msg string) {
	c.JSON(code, map[string]any{"code": code, "msg": msg})
}

// ---------- Auth ----------

func (s *Service) Login(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "invalid request")
		return
	}

	user, err := s.DAO.User.GetByUsername(ctx, req.Username)
	if err != nil {
		fail(c, 401, "用户名或密码错误")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		fail(c, 401, "用户名或密码错误")
		return
	}

	token, err := middleware.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		fail(c, 500, "token generation failed")
		return
	}
	ok(c, map[string]string{"token": token})
}

func (s *Service) ChangePassword(ctx context.Context, c *app.RequestContext) {
	userID := c.GetUint("user_id")
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "invalid request")
		return
	}

	user, err := s.DAO.User.GetByUsername(ctx, "admin")
	if err != nil {
		fail(c, 404, "user not found")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)) != nil {
		fail(c, 400, "原密码错误")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		fail(c, 500, "password hash failed")
		return
	}
	if err := s.DAO.User.UpdatePassword(ctx, userID, string(hash)); err != nil {
		fail(c, 500, "password update failed")
		return
	}
	ok(c, nil)
}

// ---------- Adapter ----------

func (s *Service) GetAdapterStatus(ctx context.Context, c *app.RequestContext) {
	ok(c, map[string]any{"status": "running"})
}

// ---------- Provider ----------

func (s *Service) ListProviders(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Provider.List(ctx, "")
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) AddProvider(ctx context.Context, c *app.RequestContext) {
	var p models.Provider
	if err := c.BindJSON(&p); err != nil {
		fail(c, 400, "invalid request")
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
		fail(c, 400, "invalid request")
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
	id := c.Param("id")
	if err := s.DAO.Provider.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- MCP ----------

func (s *Service) ListMCPServers(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.MCPServer.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) AddMCPServer(ctx context.Context, c *app.RequestContext) {
	var m models.MCPServer
	if err := c.BindJSON(&m); err != nil {
		fail(c, 400, "invalid request")
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
		fail(c, 400, "invalid request")
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
	id := c.Param("id")
	if err := s.DAO.MCPServer.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- Skill ----------

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
		fail(c, 400, "invalid request")
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
		fail(c, 400, "invalid request")
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
	id := c.Param("id")
	if err := s.DAO.Skill.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- Prompt ----------

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
		fail(c, 400, "invalid request")
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
		fail(c, 400, "invalid request")
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
	id := c.Param("id")
	if err := s.DAO.Prompt.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- Tool ----------

func (s *Service) ListTools(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ToolConfig.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) ToggleTool(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var req struct{ IsActive bool `json:"is_active"` }
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "invalid request")
		return
	}
	if err := s.DAO.ToolConfig.SetActive(ctx, id, req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- Session ----------

func (s *Service) ListSessions(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Session.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) DeleteSession(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if err := s.DAO.Session.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- Plugin ----------

func (s *Service) ListPlugins(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.Plugin.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

func (s *Service) TogglePlugin(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var req struct{ IsActive bool `json:"is_active"` }
	if err := c.BindJSON(&req); err != nil {
		fail(c, 400, "invalid request")
		return
	}
	if err := s.DAO.Plugin.SetActive(ctx, id, req.IsActive); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

func (s *Service) DeletePlugin(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	if err := s.DAO.Plugin.Delete(ctx, id); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
}

// ---------- ACL ----------

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
		fail(c, 400, "invalid request")
		return
	}
	if err := s.DAO.ACL.Create(ctx, &r); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, r)
}

func (s *Service) DeleteACLRule(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	var req struct{ ID int64 `json:"id"` }
	c.BindJSON(&req)
	if req.ID == 0 {
		fail(c, 400, "invalid id")
		return
	}
	if err := s.DAO.ACL.Delete(ctx, req.ID); err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, nil)
	_ = id
}

// ---------- Chat Records ----------

func (s *Service) GetChatRecords(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	limit := c.DefaultQuery("limit", "20")
	offset := c.DefaultQuery("offset", "0")

	l := atoi(limit)
	o := atoi(offset)

	list, total, err := s.DAO.ChatRecord.ListByChatArea(ctx, chatAreaID, l, o)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, map[string]any{"total": total, "list": list})
}

func (s *Service) GetChatAreas(ctx context.Context, c *app.RequestContext) {
	list, err := s.DAO.ChatArea.List(ctx)
	if err != nil {
		fail(c, 500, err.Error())
		return
	}
	ok(c, list)
}

// ---------- Overview ----------

func (s *Service) GetOverview(ctx context.Context, c *app.RequestContext) {
	chatAreaCount, _ := s.DAO.ChatArea.Count(ctx)
	mcpCount := int64(0)
	pluginCount := int64(0)
	totalTokens, _ := s.DAO.ChatRecord.TotalTokenUsage(ctx)

	ok(c, map[string]any{
		"chat_area_count":   chatAreaCount,
		"mcp_count":         mcpCount,
		"adapter_count":     1,
		"plugin_count":      pluginCount,
		"total_token_usage": totalTokens,
	})
}

// ---------- Memory ----------

func (s *Service) GetShortTermMemoryConfig(ctx context.Context, c *app.RequestContext) {
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.ShortTermMemory.GetOrCreate(ctx, chatAreaID)
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
		fail(c, 400, "invalid request")
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
	chatAreaID := c.Param("chatAreaID")
	m, err := s.DAO.LongTermMemory.GetOrCreate(ctx, chatAreaID)
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
		fail(c, 400, "invalid request")
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
