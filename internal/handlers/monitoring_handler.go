package handlers

import (
	"ProxyService2/internal/usecase"
	"net/http"

	"github.com/gin-gonic/gin"
)

type MonitoringHandler struct {
	uc *usecase.AdminUseCase
}

func NewMonitoringHandler(uc *usecase.AdminUseCase) *MonitoringHandler {
	return &MonitoringHandler{uc: uc}
}

// Monitoring godoc
// @Summary     Get monitoring snapshot (traffic, errors, top clients, upstreams)
// @Tags        monitoring
// @Produce     json
// @Success     200  {object} domain.MonitoringSnapshot
// @Router      /api/v1/monitoring [get]
func (h *MonitoringHandler) Monitoring(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.Monitoring(10))
}
