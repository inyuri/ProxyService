package handlers

import (
	"ProxyService2/internal/domain"
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CacheHandler struct {
	uc *usecase.AdminUseCase
}

func NewCacheHandler(uc *usecase.AdminUseCase) *CacheHandler {
	return &CacheHandler{uc: uc}
}

// Snapshot godoc
// @Summary     Get cache state and statistics
// @Tags        cache
// @Produce     json
// @Success     200  {object} domain.CacheSnapshot
// @Router      /api/v1/cache [get]
func (h *CacheHandler) Snapshot(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.CacheSnapshot(100))
}

// Invalidate godoc
// @Summary     Invalidate cache entries
// @Tags        cache
// @Accept      json
// @Produce     json
// @Param       req  body     domain.CacheInvalidationRequest  true  "Invalidation criteria"
// @Success     200  {object} map[string]int
// @Failure     400  {object} map[string]string
// @Router      /api/v1/cache/invalidate [post]
func (h *CacheHandler) Invalidate(c *gin.Context) {
	var req domain.CacheInvalidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	removed, err := h.uc.InvalidateCache(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"removed": removed})
}

// Clear godoc
// @Summary     Clear entire cache
// @Tags        cache
// @Produce     json
// @Success     200  {object} map[string]int
// @Router      /api/v1/cache [delete]
func (h *CacheHandler) Clear(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"removed": h.uc.ClearCache()})
}
