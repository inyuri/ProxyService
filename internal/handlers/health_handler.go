package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Handle godoc
// @Summary     Health check
// @Tags        health
// @Produce     json
// @Success     200  {object} map[string]string
// @Router      /health [get]
func (h *HealthHandler) Handle(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"startedAt": time.Now().Format(time.RFC3339),
	})
}
