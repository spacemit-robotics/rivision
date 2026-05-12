package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/analytics"
)

// AnalyticsAPIHandler serves analytics endpoints (§5.7).
type AnalyticsAPIHandler struct {
	engine *analytics.Engine
}

// NewAnalyticsAPIHandler creates an analytics API handler.
func NewAnalyticsAPIHandler(engine *analytics.Engine) *AnalyticsAPIHandler {
	return &AnalyticsAPIHandler{engine: engine}
}

// SetupRoutes registers analytics endpoints.
func (h *AnalyticsAPIHandler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/analytics")
	{
		g.GET("/overview", h.overview)
		g.GET("/traffic", h.traffic)
		g.GET("/heatmap", h.heatmap)
		g.GET("/dwell", h.dwell)
		g.GET("/export", h.export)
	}
}

func (h *AnalyticsAPIHandler) overview(c *gin.Context) {
	summary := h.engine.GetTodaySummary()

	todayIn, _ := summary["total_in"].(int)
	todayOut, _ := summary["total_out"].(int)
	todayTotal := todayIn + todayOut

	c.JSON(http.StatusOK, gin.H{
		"traffic": gin.H{
			"today": todayTotal,
			"week":  0,
			"month": 0,
		},
		"events": gin.H{
			"total":    0,
			"alerts":   0,
			"resolved": 0,
		},
		"nodes": gin.H{
			"total":   0,
			"online":  0,
			"offline": 0,
		},
	})
}

func (h *AnalyticsAPIHandler) traffic(c *gin.Context) {
	cameraID := c.Query("camera_id")
	period := c.DefaultQuery("period", "24h")

	end := time.Now()
	start := end.Add(-24 * time.Hour)
	if period == "7d" {
		start = end.Add(-7 * 24 * time.Hour)
	}

	data, err := h.engine.GetTrafficByHour(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "camera_id": cameraID, "period": period})
}

func (h *AnalyticsAPIHandler) heatmap(c *gin.Context) {
	cameraID := c.Query("camera_id")

	data := h.engine.GetHeatmap(cameraID)
	c.JSON(http.StatusOK, gin.H{"data": data, "camera_id": cameraID})
}

func (h *AnalyticsAPIHandler) dwell(c *gin.Context) {
	cameraID := c.Query("camera_id")
	// Dwell time is part of the today summary for now
	summary := h.engine.GetTodaySummary()
	c.JSON(http.StatusOK, gin.H{"data": summary, "camera_id": cameraID})
}

func (h *AnalyticsAPIHandler) export(c *gin.Context) {
	format := c.DefaultQuery("format", "csv")
	c.JSON(http.StatusOK, gin.H{"status": "export_queued", "format": format})
}
