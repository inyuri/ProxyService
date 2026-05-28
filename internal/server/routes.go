package server

import (
	"ProxyService2/internal/handlers"
	"ProxyService2/internal/ports"
	"ProxyService2/internal/server/middleware"
	"ProxyService2/internal/usecase"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func registerRoutes(
	engine *gin.Engine,
	store ports.ConfigStore,
	admin *usecase.AdminUseCase,
	proxy *usecase.ProxyUseCase,
	metrics ports.ObservabilityService,
	logger ports.Logger,
) {
	engine.Use(gin.Recovery())
	engine.Use(middleware.CorsMiddleware(store))
	engine.Use(middleware.TelemetryMiddleware(metrics, proxy, logger))
	engine.Use(middleware.AccessMiddleware(proxy))
	engine.Use(middleware.RateLimitMiddleware(proxy))

	healthH := handlers.NewHealthHandler()
	dashboardH := handlers.NewDashboardHandler(admin)
	accessH := handlers.NewAccessHandler(admin)
	rateLimitH := handlers.NewRateLimitHandler(admin)
	cacheH := handlers.NewCacheHandler(admin)
	monitoringH := handlers.NewMonitoringHandler(admin)
	logsH := handlers.NewLogsHandler(admin)
	proxyH := handlers.NewProxyHandler(proxy)

	engine.GET("/health", healthH.Handle)
	engine.GET("/openapi/doc.json", func(c *gin.Context) {
		c.File(filepath.Join("docs", "swagger.json"))
	})
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler, ginSwagger.URL("/openapi/doc.json")))

	api := engine.Group("/api/v1")
	{
		api.GET("/dashboard/overview", dashboardH.Overview)
		api.GET("/access/rules", accessH.List)
		api.POST("/access/rules", accessH.Create)
		api.DELETE("/access/rules/:id", accessH.Delete)
		api.GET("/access/check", accessH.Check)

		api.GET("/rate-limits", rateLimitH.Get)
		api.PUT("/rate-limits", rateLimitH.Update)
		api.GET("/rate-limits/violations", rateLimitH.Violations)

		api.GET("/cache", cacheH.Snapshot)
		api.POST("/cache/invalidate", cacheH.Invalidate)
		api.DELETE("/cache", cacheH.Clear)

		api.GET("/monitoring", monitoringH.Monitoring)
		api.GET("/logs", logsH.Logs)
		api.GET("/logs/export", logsH.Export)
	}

	example := engine.Group("/example/ip_access")
	{
		example.GET("/allowlists", accessH.List)
		example.POST("/allowlists", accessH.Create)
		example.DELETE("/allowlists/:id", accessH.Delete)
		example.GET("/check", accessH.Check)
	}

	engine.Any("/proxy/*path", proxyH.Proxy)

	// Forward any /api/* paths that don't match the admin /api/v1/* group to the upstream.
	engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/api/v1/") {
			proxyH.ProxyNoRoute(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}
