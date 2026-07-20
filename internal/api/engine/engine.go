package engine

import (
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/router"
	"JuanNiang-Neo/internal/api/service"
	"JuanNiang-Neo/internal/web"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// New 创建 Hertz 引擎: 注册全量业务路由 + 前端 SPA 兜底。
//
// webDir 指向前端构建产物目录 (通常 web/dist)。为空时禁用前端服务,
// 仅暴露 API 与 /health。
func New(addr string, webDir string, svc *service.Service) *server.Hertz {
	h := server.Default(
		server.WithHostPorts(addr),
	)
	h.Use(middleware.Recovery())
	h.Use(middleware.CORS())
	router.RegisterRoutes(h, svc)

	if webDir != "" {
		// NoRoute 兜底: /api/* -> 标准 404 信封, 其它 -> SPA 回退 index.html。
		h.NoRoute(web.SPAHandler(webDir))
	}

	return h
}