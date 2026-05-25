package handlers

import (
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RateLimitHandler struct {
	uc *usecase.AdminUseCase
}

func NewRateLimitHandler(uc *usecase.AdminUseCase) *RateLimitHandler {
	return &RateLimitHandler{uc: uc}
}

func (h *RateLimitHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.GetRateLimits())
}

func (h *RateLimitHandler) Update(c *gin.Context) {
	var req usecase.RateLimitUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.uc.UpdateRateLimits(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *RateLimitHandler) Violations(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.RateLimitViolations(100))
}
