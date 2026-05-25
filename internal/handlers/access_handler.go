package handlers

import (
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AccessHandler struct {
	uc *usecase.AdminUseCase
}

func NewAccessHandler(uc *usecase.AdminUseCase) *AccessHandler {
	return &AccessHandler{uc: uc}
}

func (h *AccessHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.ListAccessRules(c.Query("type")))
}

func (h *AccessHandler) Create(c *gin.Context) {
	var req usecase.AccessRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	created, err := h.uc.CreateAccessRule(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *AccessHandler) Delete(c *gin.Context) {
	removed, err := h.uc.DeleteAccessRule(c.Param("id"))
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

func (h *AccessHandler) Check(c *gin.Context) {
	decision := h.uc.CheckAccess(c.Query("ip"), c.Query("captcha"), c.ClientIP())
	c.JSON(http.StatusOK, decision)
}
