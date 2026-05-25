package server

import (
	"ProxyService2/internal/ports"
	"ProxyService2/internal/usecase"
	"context"

	"github.com/gin-gonic/gin"
)

type Server struct {
	proxy  *usecase.ProxyUseCase
	engine *gin.Engine
}

func NewServer(
	store ports.ConfigStore,
	admin *usecase.AdminUseCase,
	proxy *usecase.ProxyUseCase,
	metrics ports.ObservabilityService,
	logger ports.Logger,
) *Server {
	engine := gin.New()
	registerRoutes(engine, store, admin, proxy, metrics, logger)
	return &Server{
		proxy:  proxy,
		engine: engine,
	}
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) StartBackgroundWorkers(ctx context.Context) {
	go s.proxy.RunUpstreamChecks(ctx)
}
