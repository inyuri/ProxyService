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

// List godoc
// @Summary     List IP access rules
// @Tags        access
// @Produce     json
// @Param       type  query   string  false  "Filter by type: allow, deny, grey"
// @Success     200   {array} usecase.AccessRuleView
// @Router      /api/v1/access/rules [get]
func (h *AccessHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.ListAccessRules(c.Query("type")))
}

// Create godoc
// @Summary     Add IP access rule
// @Tags        access
// @Accept      json
// @Produce     json
// @Param       rule  body    usecase.AccessRuleRequest  true  "Rule to create"
// @Success     201   {object} usecase.AccessRuleView
// @Failure     400   {object} map[string]string
// @Router      /api/v1/access/rules [post]
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

// Delete godoc
// @Summary     Remove IP access rule
// @Tags        access
// @Param       id  path  string  true  "Rule ID"
// @Success     204
// @Failure     400  {object} map[string]string
// @Failure     404  {object} map[string]string
// @Router      /api/v1/access/rules/{id} [delete]
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

// Check godoc
// @Summary     Check access for an IP
// @Tags        access
// @Produce     json
// @Param       ip       query  string  false  "IP address to check (defaults to caller IP)"
// @Param       captcha  query  string  false  "Captcha token for greylist verification"
// @Success     200  {object} domain.AccessDecision
// @Router      /api/v1/access/check [get]
func (h *AccessHandler) Check(c *gin.Context) {
	decision := h.uc.CheckAccess(c.Query("ip"), c.Query("captcha"), c.ClientIP())
	c.JSON(http.StatusOK, decision)
}
