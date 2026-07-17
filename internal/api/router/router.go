package router

import (
	"context"

	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/service"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func RegisterRoutes(h *server.Hertz, svc *service.Service) {
	auth := middleware.JWTAuth()

	api := h.Group("/api/v1")

	// Auth (no JWT required)
	api.POST("/login", svc.Login)
	api.POST("/change-password", auth, svc.ChangePassword)

	// Adapter
	api.GET("/adapter", auth, svc.GetAdapterStatus)

	// Providers
	api.GET("/providers", auth, svc.ListProviders)
	api.POST("/providers", auth, svc.AddProvider)
	api.PUT("/providers/:id", auth, svc.UpdateProvider)
	api.DELETE("/providers/:id", auth, svc.DeleteProvider)

	// MCP
	api.GET("/mcp", auth, svc.ListMCPServers)
	api.POST("/mcp", auth, svc.AddMCPServer)
	api.PUT("/mcp/:id", auth, svc.UpdateMCPServer)
	api.DELETE("/mcp/:id", auth, svc.DeleteMCPServer)

	// Memory
	api.GET("/memory/:chatAreaID/short-term", auth, svc.GetShortTermMemoryConfig)
	api.PUT("/memory/:chatAreaID/short-term", auth, svc.UpdateShortTermMemoryConfig)
	api.GET("/memory/:chatAreaID/long-term", auth, svc.GetLongTermMemoryConfig)
	api.PUT("/memory/:chatAreaID/long-term", auth, svc.UpdateLongTermMemoryConfig)

	// Prompts
	api.GET("/prompts", auth, svc.ListPrompts)
	api.POST("/prompts", auth, svc.AddPrompt)
	api.PUT("/prompts/:id", auth, svc.UpdatePrompt)
	api.DELETE("/prompts/:id", auth, svc.DeletePrompt)

	// Sessions
	api.GET("/sessions", auth, svc.ListSessions)
	api.DELETE("/sessions/:id", auth, svc.DeleteSession)

	// Skills
	api.GET("/skills", auth, svc.ListSkills)
	api.POST("/skills", auth, svc.AddSkill)
	api.PUT("/skills/:id", auth, svc.UpdateSkill)
	api.DELETE("/skills/:id", auth, svc.DeleteSkill)

	// Tools
	api.GET("/tools", auth, svc.ListTools)
	api.PUT("/tools/:id/toggle", auth, svc.ToggleTool)

	// Plugins
	api.GET("/plugins", auth, svc.ListPlugins)
	api.PUT("/plugins/:id/toggle", auth, svc.TogglePlugin)
	api.DELETE("/plugins/:id", auth, svc.DeletePlugin)

	// ACL
	api.GET("/acl", auth, svc.ListACLRules)
	api.POST("/acl", auth, svc.AddACLRule)
	api.DELETE("/acl/:id", auth, svc.DeleteACLRule)

	// Chat Records
	api.GET("/chat-records/:chatAreaID", auth, svc.GetChatRecords)

	// Overview
	api.GET("/overview", auth, svc.GetOverview)

	// Chat Areas
	api.GET("/chat-areas", auth, svc.GetChatAreas)

	// Health
	h.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]string{"status": "ok"})
	})
}
