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

	// Auth
	api.POST("/login", svc.Login)
	api.POST("/change-password", auth, svc.ChangePassword)

	// Adapter
	api.GET("/adapter", auth, svc.GetAdapterStatus)
	api.GET("/adapter/config", auth, svc.GetAdapterConfig)
	api.PUT("/adapter", auth, svc.UpdateAdapterConfig)
	api.POST("/adapter/restart", auth, svc.RestartAdapter)

	// Providers
	api.GET("/providers", auth, svc.ListProviders)
	api.GET("/providers/:id", auth, svc.GetProvider)
	api.POST("/providers", auth, svc.AddProvider)
	api.PUT("/providers/:id", auth, svc.UpdateProvider)
	api.DELETE("/providers/:id", auth, svc.DeleteProvider)
	api.PUT("/providers/:id/toggle", auth, svc.ToggleProvider)

	// MCP
	api.GET("/mcp", auth, svc.ListMCPServers)
	api.GET("/mcp/:id", auth, svc.GetMCPServer)
	api.POST("/mcp", auth, svc.AddMCPServer)
	api.PUT("/mcp/:id", auth, svc.UpdateMCPServer)
	api.DELETE("/mcp/:id", auth, svc.DeleteMCPServer)
	api.PUT("/mcp/:id/toggle", auth, svc.ToggleMCPServer)
	api.GET("/mcp/:id/check", auth, svc.CheckMCPServer)

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
	api.PUT("/prompts/:id/toggle", auth, svc.TogglePrompt)

	// Sessions
	api.GET("/sessions", auth, svc.ListSessions)
	api.GET("/sessions/:id", auth, svc.GetSession)
	api.DELETE("/sessions/:id", auth, svc.DeleteSession)

	// Skills
	api.GET("/skills", auth, svc.ListSkills)
	api.POST("/skills", auth, svc.AddSkill)
	api.PUT("/skills/:id", auth, svc.UpdateSkill)
	api.DELETE("/skills/:id", auth, svc.DeleteSkill)

	// Tools
	api.GET("/tools", auth, svc.ListTools)
	api.PUT("/tools/:id/toggle", auth, svc.ToggleTool)
	api.PUT("/tools/:id/admin-only", auth, svc.UpdateToolAdminOnly)

	// Plugins
	api.GET("/plugins", auth, svc.ListPlugins)
	api.POST("/plugins/upload", auth, svc.UploadPlugin)
	api.POST("/plugins/reload", auth, svc.ReloadAllPlugins)
	api.PUT("/plugins/:id/toggle", auth, svc.TogglePlugin)
	api.DELETE("/plugins/:id", auth, svc.DeletePlugin)

	// ACL
	api.GET("/acl", auth, svc.ListACLRules)
	api.POST("/acl", auth, svc.AddACLRule)
	api.DELETE("/acl/:id", auth, svc.DeleteACLRule)

	// Chat Records
	api.GET("/chat-records/:chatAreaID", auth, svc.GetChatRecords)
	api.GET("/chat-records/:chatAreaID/token-usage", auth, svc.GetChatAreaTokenUsage)

	// Overview
	api.GET("/overview", auth, svc.GetOverview)
	api.GET("/overview/daily-token-usage", auth, svc.GetDailyTokenUsage)

	// Chat Areas
	api.GET("/chat-areas", auth, svc.GetChatAreas)

	// Health
	h.GET("/health", func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]string{"status": "ok"})
	})

	// T2I
	api.GET("/t2i/config", auth, svc.GetT2IConfig)
	api.PUT("/t2i/config", auth, svc.UpdateT2IConfig)
	api.GET("/t2i/health", auth, svc.CheckT2IHealth)

	// Sandbox
	api.GET("/sandbox/config", auth, svc.GetSandboxConfig)
	api.PUT("/sandbox/config", auth, svc.UpdateSandboxConfig)
	api.GET("/sandbox/health", auth, svc.CheckSandboxHealth)

	// Webhook
	api.GET("/webhook/config", auth, svc.GetWebhookConfig)
	api.PUT("/webhook/config", auth, svc.UpdateWebhookConfig)

	// Logs
	api.GET("/logs", auth, svc.GetLogs)
	api.GET("/logs/stream", auth, svc.StreamLogs)

	// Agent 活跃循环（原后台任务页改造）
	api.GET("/agent/loops", auth, svc.ListAgentLoops)

	// CronJob
	api.GET("/cronjobs", auth, svc.ListCronJobs)
	api.GET("/cronjobs/:id", auth, svc.GetCronJob)
	api.POST("/cronjobs", auth, svc.AddCronJob)
	api.PUT("/cronjobs/:id", auth, svc.UpdateCronJob)
	api.DELETE("/cronjobs/:id", auth, svc.DeleteCronJob)
	api.PUT("/cronjobs/:id/toggle", auth, svc.ToggleCronJob)

	// Reply Strategy
	api.GET("/reply-strategy", auth, svc.GetReplyStrategy)
	api.PUT("/reply-strategy", auth, svc.UpdateReplyStrategy)

	// Knowledge 知识库
	api.GET("/knowledge", auth, svc.ListKnowledge)
	api.GET("/knowledge/:id", auth, svc.GetKnowledge)
	api.POST("/knowledge", auth, svc.AddKnowledge)
	api.PUT("/knowledge/:id", auth, svc.UpdateKnowledge)
	api.DELETE("/knowledge/:id", auth, svc.DeleteKnowledge)
	api.POST("/knowledge/:id/re-extract", auth, svc.ReExtractKnowledge)

	// 图床
	api.GET("/images", auth, svc.ListImages)
	api.GET("/images/:id", auth, svc.GetImage)
	api.GET("/images/:id/file", auth, svc.GetImageFile)
	api.POST("/images", auth, svc.UploadImage)
	api.PUT("/images/:id", auth, svc.UpdateImage)
	api.DELETE("/images/:id", auth, svc.DeleteImage)
	api.GET("/image-folders", auth, svc.ListImageFolders)
	api.POST("/image-folders", auth, svc.CreateImageFolder)
	api.DELETE("/image-folders/:id", auth, svc.DeleteImageFolder)

	// 表情包库
	api.GET("/stickers", auth, svc.ListStickers)
	api.GET("/stickers/:id", auth, svc.GetSticker)
	api.POST("/stickers", auth, svc.CreateSticker)
	api.PUT("/stickers/:id", auth, svc.UpdateSticker)
	api.DELETE("/stickers/:id", auth, svc.DeleteSticker)
	api.GET("/sticker-tags", auth, svc.ListStickerTags)
	api.POST("/sticker-tags", auth, svc.CreateStickerTag)
	api.DELETE("/sticker-tags/:id", auth, svc.DeleteStickerTag)
}
