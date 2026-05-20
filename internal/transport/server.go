package transport

import (
	"ProxyService2/internal/domain"
	"ProxyService2/internal/ports"
	adminservice "ProxyService2/internal/service/admin"
	proxyservice "ProxyService2/internal/service/proxy"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

const (
	ctxRateLeaseKey    = "rate_lease"
	ctxBlockedKey      = "blocked"
	ctxReasonKey       = "blocked_reason"
	ctxRequestBytesKey = "request_bytes"
	ctxCacheStatusKey  = "cache_status"
)

type Server struct {
	store   ports.ConfigStore
	admin   *adminservice.Service
	proxy   *proxyservice.Service
	metrics ports.ObservabilityService
	logger  ports.Logger
	engine  *gin.Engine
}

func NewServer(
	store ports.ConfigStore,
	admin *adminservice.Service,
	proxy *proxyservice.Service,
	metrics ports.ObservabilityService,
	logger ports.Logger,
) *Server {
	server := &Server{
		store:   store,
		admin:   admin,
		proxy:   proxy,
		metrics: metrics,
		logger:  logger,
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(server.corsMiddleware())
	engine.Use(server.telemetryMiddleware())
	engine.Use(server.accessMiddleware())
	engine.Use(server.rateLimitMiddleware())
	server.registerRoutes(engine)
	server.engine = engine
	return server
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) StartBackgroundWorkers(ctx context.Context) {
	go s.proxy.RunUpstreamChecks(ctx)
}

func (s *Server) registerRoutes(engine *gin.Engine) {
	engine.GET("/health", s.healthHandler)
	engine.GET("/openapi/doc.json", func(c *gin.Context) {
		c.File(filepath.Join("docs", "swagger.json"))
	})
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler, ginSwagger.URL("/openapi/doc.json")))

	api := engine.Group("/api/v1")
	{
		api.GET("/dashboard/overview", s.dashboardOverviewHandler)
		api.GET("/access/rules", s.listAccessRulesHandler)
		api.POST("/access/rules", s.createAccessRuleHandler)
		api.DELETE("/access/rules/:id", s.deleteAccessRuleHandler)
		api.GET("/access/check", s.checkAccessHandler)

		api.GET("/rate-limits", s.getRateLimitsHandler)
		api.PUT("/rate-limits", s.updateRateLimitsHandler)
		api.GET("/rate-limits/violations", s.rateLimitViolationsHandler)

		api.GET("/cache", s.cacheSnapshotHandler)
		api.POST("/cache/invalidate", s.invalidateCacheHandler)
		api.DELETE("/cache", s.clearCacheHandler)

		api.GET("/monitoring", s.monitoringHandler)
		api.GET("/logs", s.logsHandler)
		api.GET("/logs/export", s.logsExportHandler)
	}

	example := engine.Group("/example/ip_access")
	{
		example.GET("/allowlists", s.listAccessRulesHandler)
		example.POST("/allowlists", s.createAccessRuleHandler)
		example.DELETE("/allowlists/:id", s.deleteAccessRuleHandler)
		example.GET("/check", s.checkAccessHandler)
	}

	engine.Any("/proxy/*path", s.proxyHandler)
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := s.store.Current()
		origin := c.GetHeader("Origin")
		if origin != "" && originAllowed(origin, cfg.Frontend.AllowedOrigins) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Captcha-Token")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) telemetryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.NewString()
		startedAt := time.Now()
		clientIP := c.ClientIP()
		s.metrics.IncActive()
		c.Header("X-Request-ID", requestID)

		c.Next()

		s.metrics.DecActive()

		requestBytes, _ := c.Get(ctxRequestBytesKey)
		requestSize, ok := requestBytes.(int64)
		if !ok || requestSize < 0 {
			requestSize = maxInt64(c.Request.ContentLength, 0)
		}

		if leaseValue, ok := c.Get(ctxRateLeaseKey); ok {
			if lease, ok := leaseValue.(*domain.RateLimitLease); ok && lease != nil {
				s.proxy.ReleaseRateLease(lease, requestSize, int64(c.Writer.Size()))
			}
		}

		blocked, _ := c.Get(ctxBlockedKey)
		reason, _ := c.Get(ctxReasonKey)
		cacheStatus, _ := c.Get(ctxCacheStatusKey)
		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}

		latency := time.Since(startedAt)
		event := domain.RequestEvent{
			Timestamp:     startedAt,
			IP:            clientIP,
			Method:        c.Request.Method,
			Path:          c.Request.URL.Path,
			Status:        status,
			Latency:       latency,
			Blocked:       blocked == true,
			RequestBytes:  requestSize,
			ResponseBytes: int64(c.Writer.Size()),
		}
		if reasonValue, ok := reason.(string); ok {
			event.Reason = reasonValue
		}
		if cacheValue, ok := cacheStatus.(string); ok {
			event.CacheStatus = cacheValue
		}
		s.metrics.Record(event)

		logFields := map[string]any{
			"requestId": requestID,
			"ip":        clientIP,
			"method":    c.Request.Method,
			"path":      c.Request.URL.Path,
			"status":    status,
			"latencyMs": latency.Milliseconds(),
			"blocked":   blocked == true,
		}
		if event.Reason != "" {
			logFields["reason"] = event.Reason
		}
		if event.CacheStatus != "" {
			logFields["cacheStatus"] = event.CacheStatus
		}

		level := zerolog.InfoLevel
		switch {
		case status >= 500:
			level = zerolog.ErrorLevel
		case status >= 400:
			level = zerolog.WarnLevel
		}
		s.logger.Log(level, "request handled", logFields)
	}
}

func (s *Server) accessMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		decision := s.proxy.EvaluateAccess(c.ClientIP(), c.GetHeader(s.proxy.CaptchaHeader()))
		if decision.Allowed {
			c.Next()
			return
		}

		c.Set(ctxBlockedKey, true)
		c.Set(ctxReasonKey, decision.Reason)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"allowed":         false,
			"requiresCaptcha": decision.RequiresCaptcha,
			"reason":          decision.Reason,
			"ruleId":          decision.RuleID,
			"matchedValue":    decision.MatchedValue,
		})
	}
}

func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadHint := maxInt64(c.Request.ContentLength, 0)
		lease, violation := s.proxy.AcquireRateLease(c.ClientIP(), uploadHint, time.Now())
		if violation != nil {
			c.Set(ctxBlockedKey, true)
			c.Set(ctxReasonKey, "rate_limited")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":     "rate limit exceeded",
				"violation": violation,
			})
			return
		}
		c.Set(ctxRateLeaseKey, lease)
		c.Next()
	}
}

func (s *Server) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"startedAt": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) dashboardOverviewHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.DashboardOverview())
}

func (s *Server) listAccessRulesHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.ListAccessRules(c.Query("type")))
}

func (s *Server) createAccessRuleHandler(c *gin.Context) {
	var request adminservice.AccessRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := s.admin.CreateAccessRule(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (s *Server) deleteAccessRuleHandler(c *gin.Context) {
	removed, err := s.admin.DeleteAccessRule(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) checkAccessHandler(c *gin.Context) {
	decision := s.admin.CheckAccess(c.Query("ip"), c.Query("captcha"), c.ClientIP())
	c.JSON(http.StatusOK, decision)
}

func (s *Server) getRateLimitsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.GetRateLimits())
}

func (s *Server) updateRateLimitsHandler(c *gin.Context) {
	var request adminservice.RateLimitUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := s.admin.UpdateRateLimits(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (s *Server) rateLimitViolationsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.RateLimitViolations(100))
}

func (s *Server) cacheSnapshotHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.CacheSnapshot(100))
}

func (s *Server) invalidateCacheHandler(c *gin.Context) {
	var request domain.CacheInvalidationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	removed, err := s.admin.InvalidateCache(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"removed": removed})
}

func (s *Server) clearCacheHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"removed": s.admin.ClearCache()})
}

func (s *Server) monitoringHandler(c *gin.Context) {
	c.JSON(http.StatusOK, s.admin.Monitoring(10))
}

func (s *Server) logsHandler(c *gin.Context) {
	status, err := strconv.Atoi(c.DefaultQuery("status", "0"))
	if err != nil {
		status = 0
	}
	c.JSON(http.StatusOK, s.admin.Logs(c.Query("ip"), status, 200))
}

func (s *Server) logsExportHandler(c *gin.Context) {
	raw, err := s.admin.ExportLogs(c.Query("ip"), 0, 500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", `attachment; filename="proxy-logs.json"`)
	c.Data(http.StatusOK, "application/json", raw)
}

func (s *Server) proxyHandler(c *gin.Context) {
	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = c.Request.Body.Close()

	response, err := s.proxy.Forward(proxyservice.ForwardRequest{
		Context:       c.Request.Context(),
		Method:        c.Request.Method,
		RequestPath:   c.Param("path"),
		RawQuery:      c.Request.URL.RawQuery,
		Header:        c.Request.Header,
		Body:          requestBody,
		ContentLength: c.Request.ContentLength,
	})
	if err != nil {
		httpErr := &proxyservice.HTTPError{StatusCode: http.StatusBadGateway, Message: "proxy error"}
		if asProxyError(err, httpErr) {
			c.JSON(httpErr.StatusCode, gin.H{"error": httpErr.Message})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.Set(ctxRequestBytesKey, response.RequestBytes)
	c.Set(ctxCacheStatusKey, response.CacheStatus)
	copyResponseHeaders(c.Writer.Header(), response.Header)
	c.Header("X-Cache-Status", response.CacheStatus)
	c.Status(response.StatusCode)
	if c.Request.Method != http.MethodHead {
		_, _ = c.Writer.Write(response.Body)
	}
}

func asProxyError(err error, target *proxyservice.HTTPError) bool {
	value, ok := err.(*proxyservice.HTTPError)
	if !ok {
		return false
	}
	*target = *value
	return true
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if isHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func isHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == "*" || strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}

func maxInt64(value, fallback int64) int64 {
	if value >= 0 {
		return value
	}
	return fallback
}
