package handlers

import (
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	uc *usecase.AdminUseCase
}

func NewDashboardHandler(uc *usecase.AdminUseCase) *DashboardHandler {
	return &DashboardHandler{uc: uc}
}

// Overview godoc
// @Summary     Get real-time dashboard overview metrics
// @Tags        dashboard
// @Produce     json
// @Success     200  {object} domain.DashboardOverview
// @Router      /api/v1/dashboard/overview [get]
func (h *DashboardHandler) Overview(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.DashboardOverview())
}
