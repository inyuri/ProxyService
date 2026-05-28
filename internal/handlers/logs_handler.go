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

// Logs godoc
// @Summary     Query request logs
// @Tags        logs
// @Produce     json
// @Param       ip      query  string  false  "Filter by client IP (substring match)"
// @Param       status  query  int     false  "Filter by HTTP status code"
// @Success     200     {object} usecase.LogsResponse
// @Router      /api/v1/logs [get]
func (h *LogsHandler) Logs(c *gin.Context) {
	status, err := strconv.Atoi(c.DefaultQuery("status", "0"))
	if err != nil {
		status = 0
	}
	c.JSON(http.StatusOK, h.uc.Logs(c.Query("ip"), status, 200))
}

// Export godoc
// @Summary     Export request logs as JSON file
// @Tags        logs
// @Produce     application/json
// @Param       ip  query  string  false  "Filter by client IP"
// @Success     200
// @Failure     500  {object} map[string]string
// @Router      /api/v1/logs/export [get]
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
