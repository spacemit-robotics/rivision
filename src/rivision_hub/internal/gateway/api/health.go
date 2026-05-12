// Package api 提供健康检查 API
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DetailedHealth 详细健康检查
// GET /api/v1/health/detailed
func (h *Handlers) DetailedHealth(c *gin.Context) {
	registry := h.client.GetRegistry()
	stats := registry.GetStats()

	status := "healthy"
	if stats["healthy"].(int) == 0 && stats["total"].(int) > 0 {
		status = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   status,
		"version":  h.cfg.AppVersion,
		"cluster":  registry.GetClusterName(),
		"strategy": h.cfg.LoadBalanceStrategy,
		"stats":    stats,
		"health_checker": gin.H{
			"enabled":  h.cfg.HealthCheckEnabled,
			"interval": h.cfg.HealthCheckInterval.String(),
			"running":  true,
		},
		"service_status": h.client.GetStatus(),
	})
}

// TriggerHealthCheck 触发健康检查
// POST /api/v1/health/check
func (h *Handlers) TriggerHealthCheck(c *gin.Context) {
	// 触发健康检查
	// 实际实现应该调用 healthChecker.CheckNow()
	
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()
	
	results := make([]gin.H, 0, len(nodes))
	for _, node := range nodes {
		results = append(results, gin.H{
			"node_id": node.ID,
			"healthy": node.IsHealthy(),
			"status":  node.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "健康检查完成",
		"result":  results,
	})
}
