package handlers

import (
	"ProxyService2/internal/usecase"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type LogsHandler struct {
	uc *usecase.AdminUseCase
}

func NewLogsHandler(uc *usecase.AdminUseCase) *LogsHandler {
	return &LogsHandler{uc: uc}
}

func (h *LogsHandler) Logs(c *gin.Context) {
	status, err := strconv.Atoi(c.DefaultQuery("status", "0"))
	if err != nil {
		status = 0
	}
	c.JSON(http.StatusOK, h.uc.Logs(c.Query("ip"), status, 200))
}

func (h *LogsHandler) Export(c *gin.Context) {
	raw, err := h.uc.ExportLogs(c.Query("ip"), 0, 500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", `attachment; filename="proxy-logs.json"`)
	c.Data(http.StatusOK, "application/json", raw)
}
