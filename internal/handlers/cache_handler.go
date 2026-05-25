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

func (h *CacheHandler) Snapshot(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.CacheSnapshot(100))
}

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

func (h *CacheHandler) Clear(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"removed": h.uc.ClearCache()})
}
