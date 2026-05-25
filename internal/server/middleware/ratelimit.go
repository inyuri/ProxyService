package middleware

import (
	"ProxyService2/internal/usecase"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func RateLimitMiddleware(proxy *usecase.ProxyUseCase) gin.HandlerFunc {
	return func(c *gin.Context) {
		uploadHint := maxInt64(c.Request.ContentLength, 0)
		lease, violation := proxy.AcquireRateLease(c.ClientIP(), uploadHint, time.Now())
		if violation != nil {
			c.Set(CtxBlockedKey, true)
			c.Set(CtxReasonKey, "rate_limited")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":     "rate limit exceeded",
				"violation": violation,
			})
			return
		}
		c.Set(CtxRateLeaseKey, lease)
		c.Next()
	}
}
