package engine

import (
	"context"

	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/router"
	"JuanNiang-Neo/internal/api/service"
	"JuanNiang-Neo/internal/metrics"
	"JuanNiang-Neo/internal/web"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
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
	// Prometheus 监控中间件：API 请求数/耗时（/metrics 自身不受 JWT 限制）
	h.Use(metrics.HTTPMiddleware)
	router.RegisterRoutes(h, svc)

	// Prometheus 监控端点（root 路由，与 /health 同级，不参与 JWT）
	h.GET("/metrics", func(ctx context.Context, c *app.RequestContext) {
		r, err := adaptor.GetCompatRequest(&c.Request)
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
		w := adaptor.GetCompatResponseWriter(&c.Response)
		metrics.Handler().ServeHTTP(w, r)
	})

	if webDir != "" {
		// NoRoute 兜底: /api/* -> 标准 404 信封, 其它 -> SPA 回退 index.html。
		h.NoRoute(web.SPAHandler(webDir))
	}

	return h
}
