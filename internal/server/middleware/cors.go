package middleware

import (
	"ProxyService2/internal/ports"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func CorsMiddleware(store ports.ConfigStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := store.Current()
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

func originAllowed(origin string, allowed []string) bool {
	for _, candidate := range allowed {
		if candidate == "*" || strings.EqualFold(candidate, origin) {
			return true
		}
	}
	return false
}
