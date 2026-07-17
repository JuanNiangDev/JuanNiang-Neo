package engine

import (
	"JuanNiang-Neo/internal/api/middleware"
	"JuanNiang-Neo/internal/api/router"
	"JuanNiang-Neo/internal/api/service"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func New(addr string, svc *service.Service) *server.Hertz {
	h := server.Default(
		server.WithHostPorts(addr),
	)
	h.Use(middleware.Recovery())
	h.Use(middleware.CORS())
	router.RegisterRoutes(h, svc)
	return h
}
