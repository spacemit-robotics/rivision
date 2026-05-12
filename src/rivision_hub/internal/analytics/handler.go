// Package analytics 提供分析 API 处理器
package analytics

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler 分析处理器
type Handler struct {
	engine *Engine
}

// NewHandler 创建处理器
func NewHandler(engine *Engine) *Handler {
	return &Handler{engine: engine}
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes(r *gin.RouterGroup) {
	a := r.Group("/analytics")
	{
		a.GET("/overview", h.Overview)
		a.GET("/traffic/hourly", h.TrafficHourly)
		a.GET("/traffic/daily", h.TrafficDaily)
		a.GET("/traffic/summary", h.TrafficSummary)
		a.GET("/heatmap/:camera_id", h.Heatmap)
		a.GET("/dashboard", h.Dashboard)
	}
}

// Overview 统计概览（供 Analytics.vue 使用）
func (h *Handler) Overview(c *gin.Context) {
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

// TrafficHourly 小时客流
func (h *Handler) TrafficHourly(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 camera_id"})
		return
	}

	start, _ := time.Parse("2006-01-02", c.DefaultQuery("start", time.Now().Format("2006-01-02")))
	end, _ := time.Parse("2006-01-02", c.DefaultQuery("end", time.Now().Format("2006-01-02")))
	end = end.Add(24 * time.Hour)

	data, err := h.engine.GetTrafficByHour(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": cameraID,
		"start":     start.Format("2006-01-02"),
		"end":       end.Add(-24 * time.Hour).Format("2006-01-02"),
		"data":      data,
	})
}

// TrafficDaily 日客流
func (h *Handler) TrafficDaily(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 camera_id"})
		return
	}

	start, _ := time.Parse("2006-01-02", c.DefaultQuery("start", time.Now().AddDate(0, 0, -7).Format("2006-01-02")))
	end, _ := time.Parse("2006-01-02", c.DefaultQuery("end", time.Now().Format("2006-01-02")))

	data, err := h.engine.GetTrafficByDay(cameraID, start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": cameraID,
		"start":     start.Format("2006-01-02"),
		"end":       end.Format("2006-01-02"),
		"data":      data,
	})
}

// TrafficSummary 客流汇总
func (h *Handler) TrafficSummary(c *gin.Context) {
	c.JSON(http.StatusOK, h.engine.GetTodaySummary())
}

// Heatmap 热区图
func (h *Handler) Heatmap(c *gin.Context) {
	cameraID := c.Param("camera_id")

	data := h.engine.GetHeatmap(cameraID)
	if data == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "暂无热区数据"})
		return
	}

	c.JSON(http.StatusOK, data)
}

// Dashboard 仪表盘数据
func (h *Handler) Dashboard(c *gin.Context) {
	summary := h.engine.GetTodaySummary()

	c.JSON(http.StatusOK, gin.H{
		"traffic":    summary,
		"updated_at": time.Now().Format(time.RFC3339),
	})
}
