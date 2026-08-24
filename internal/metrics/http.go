package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

// HTTPMiddleware Hertz API 请求统计中间件：
// 记录请求数（method × 路由模板 path × 状态码）与耗时。
// path 使用 c.FullPath()（路由模板，如 /api/v1/providers/:id），
// 未命中路由时回退 c.Path()（低基数，安全）。
func HTTPMiddleware(ctx context.Context, c *app.RequestContext) {
	start := time.Now()
	c.Next(ctx)
	path := c.FullPath()
	if path == "" {
		path = string(c.Path())
	}
	HTTPRequestsTotal.WithLabelValues(string(c.Method()), path, strconv.Itoa(c.Response.StatusCode())).Inc()
	HTTPRequestDuration.Observe(time.Since(start).Seconds())
}
