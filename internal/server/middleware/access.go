package middleware

import (
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AccessMiddleware(proxy *usecase.ProxyUseCase) gin.HandlerFunc {
	return func(c *gin.Context) {
		decision := proxy.EvaluateAccess(c.ClientIP(), c.GetHeader(proxy.CaptchaHeader()))
		if decision.Allowed {
			c.Next()
			return
		}

		c.Set(CtxBlockedKey, true)
		c.Set(CtxReasonKey, decision.Reason)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"allowed":         false,
			"requiresCaptcha": decision.RequiresCaptcha,
			"reason":          decision.Reason,
			"ruleId":          decision.RuleID,
			"matchedValue":    decision.MatchedValue,
		})
	}
}
