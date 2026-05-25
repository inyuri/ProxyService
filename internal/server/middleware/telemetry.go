package middleware

import (
	"ProxyService2/internal/domain"
	"ProxyService2/internal/ports"
	"ProxyService2/internal/usecase"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func TelemetryMiddleware(metrics ports.ObservabilityService, proxy *usecase.ProxyUseCase, logger ports.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.NewString()
		startedAt := time.Now()
		clientIP := c.ClientIP()
		metrics.IncActive()
		c.Header("X-Request-ID", requestID)

		c.Next()

		metrics.DecActive()

		requestBytes, _ := c.Get(CtxRequestBytesKey)
		requestSize, ok := requestBytes.(int64)
		if !ok || requestSize < 0 {
			requestSize = maxInt64(c.Request.ContentLength, 0)
		}

		if leaseValue, ok := c.Get(CtxRateLeaseKey); ok {
			if lease, ok := leaseValue.(*domain.RateLimitLease); ok && lease != nil {
				proxy.ReleaseRateLease(lease, requestSize, int64(c.Writer.Size()))
			}
		}

		blocked, _ := c.Get(CtxBlockedKey)
		reason, _ := c.Get(CtxReasonKey)
		cacheStatus, _ := c.Get(CtxCacheStatusKey)
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
		metrics.Record(event)

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
		logger.Log(level, "request handled", logFields)
	}
}

func maxInt64(value, fallback int64) int64 {
	if value >= 0 {
		return value
	}
	return fallback
}
