// Package api 实现API处理器
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/alerts"
	"github.com/rivision/rivision-hub/pkg/models"
)

// AlertsHandler 告警API处理器
type AlertsHandler struct {
	engine *alerts.Engine
}

// NewAlertsHandler 创建处理器
func NewAlertsHandler(engine *alerts.Engine) *AlertsHandler {
	return &AlertsHandler{engine: engine}
}

// SetupRoutes 设置路由
func (h *AlertsHandler) SetupRoutes(r *gin.RouterGroup) {
	alerts := r.Group("/alerts")
	{
		alerts.GET("", h.List)
		alerts.GET("/:id", h.Get)
		alerts.POST("/:id/ack", h.Ack)
		alerts.POST("/:id/resolve", h.Resolve)
		alerts.GET("/stats", h.Stats)
	}
}

// List 获取告警列表
func (h *AlertsHandler) List(c *gin.Context) {
	var query models.AlertQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	alerts, total, err := h.engine.GetAlerts(&query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alerts": alerts,
		"total":  total,
		"limit":  query.Limit,
		"offset": query.Offset,
	})
}

// Get 获取告警详情
func (h *AlertsHandler) Get(c *gin.Context) {
	id := c.Param("id")
	alert, err := h.engine.GetAlert(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "告警不存在"})
		return
	}
	c.JSON(http.StatusOK, alert)
}

// Ack 确认告警
func (h *AlertsHandler) Ack(c *gin.Context) {
	id := c.Param("id")
	
	var req struct {
		AckedBy string `json:"acked_by"`
	}
	c.ShouldBindJSON(&req)

	if err := h.engine.AckAlert(id, req.AckedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Resolve 解决告警
func (h *AlertsHandler) Resolve(c *gin.Context) {
	id := c.Param("id")
	if err := h.engine.ResolveAlert(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Stats 告警统计
func (h *AlertsHandler) Stats(c *gin.Context) {
	stats, err := h.engine.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}
