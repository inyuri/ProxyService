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

func (h *MonitoringHandler) Monitoring(c *gin.Context) {
	c.JSON(http.StatusOK, h.uc.Monitoring(10))
}
