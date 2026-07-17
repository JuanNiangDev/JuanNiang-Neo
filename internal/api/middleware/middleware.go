package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
)

func Recovery() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		defer func() {
			if err := recover(); err != nil {
				c.AbortWithStatusJSON(500, map[string]any{
					"code": 500,
					"msg":  "internal server error",
				})
			}
		}()
		c.Next(ctx)
	}
}

func CORS() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if string(c.Method()) == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next(ctx)
	}
}
