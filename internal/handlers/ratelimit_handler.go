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

// Get godoc
// @Summary     Get current rate-limit settings and top violators
// @Tags        rate-limits
// @Produce     json
// @Success     200  {object} usecase.RateLimitResponse
// @Router      /api/v1/rate-limits [get]
func (h *RateLimitHandler) Get(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.GetRateLimits())
}

// Update godoc
// @Summary     Update rate-limit settings
// @Tags        rate-limits
// @Accept      json
// @Produce     json
// @Param       settings  body     usecase.RateLimitUpdateRequest  true  "New limits"
// @Success     200       {object} domain.RateLimitSettings
// @Failure     400       {object} map[string]string
// @Failure     500       {object} map[string]string
// @Router      /api/v1/rate-limits [put]
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

// Violations godoc
// @Summary     Get recent rate-limit violations
// @Tags        rate-limits
// @Produce     json
// @Success     200  {array} domain.RateLimitViolation
// @Router      /api/v1/rate-limits/violations [get]
func (h *RateLimitHandler) Violations(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.RateLimitViolations(100))
}
