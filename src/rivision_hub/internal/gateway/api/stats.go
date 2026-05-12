// Package api 提供统计 API
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// GetOverview 获取系统概览
// GET /api/v1/stats/overview
func (h *Handlers) GetOverview(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	var totalRequests, successRequests int64
	var totalResponseTime float64
	for _, node := range nodes {
		stats := node.GetStats()
		totalRequests += stats["total_requests"].(int64)
		successRequests += stats["success_requests"].(int64)
		totalResponseTime += stats["avg_response_time"].(float64)
	}

	successRate := float64(0)
	if totalRequests > 0 {
		successRate = float64(successRequests) / float64(totalRequests) * 100
	}

	avgResponseTime := float64(0)
	if len(nodes) > 0 {
		avgResponseTime = totalResponseTime / float64(len(nodes))
	}

	c.JSON(http.StatusOK, gin.H{
		"total_nodes":         len(nodes),
		"healthy_nodes":       registry.HealthyCount(),
		"unhealthy_nodes":     len(nodes) - registry.HealthyCount(),
		"total_requests":      totalRequests,
		"success_rate":        successRate,
		"avg_response_time":   avgResponseTime,
		"requests_per_minute": 0, // 需要实现时间窗口统计
		"task_distribution":   map[string]int{},
		"concurrent_per_node": h.cfg.MaxConcurrentPerNode,
	})
}

// GetNodeStats 获取节点统计
// GET /api/v1/stats/nodes
func (h *Handlers) GetNodeStats(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	nodeStats := make([]gin.H, 0, len(nodes))
	for _, node := range nodes {
		stats := node.GetStats()
		nodeStats = append(nodeStats, gin.H{
			"node_id":             stats["id"],
			"url":                 stats["url"],
			"is_healthy":          node.IsHealthy(),
			"total_requests":      stats["total_requests"],
			"success_count":       stats["success_requests"],
			"failed_count":        stats["total_requests"].(int64) - stats["success_requests"].(int64),
			"avg_response_time":   stats["avg_response_time"],
			"current_connections": stats["active_requests"],
			"weight":              node.Weight,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nodeStats,
	})
}

// GetPerformance 获取性能数据
// GET /api/v1/stats/performance
func (h *Handlers) GetPerformance(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	nodeLoad := make(map[string]float64)
	var totalResponseTime float64
	for _, node := range nodes {
		stats := node.GetStats()
		nodeLoad[node.ID] = float64(stats["active_requests"].(int)) / float64(node.MaxParallel)
		totalResponseTime += stats["avg_response_time"].(float64)
	}

	avgResponseTime := float64(0)
	if len(nodes) > 0 {
		avgResponseTime = totalResponseTime / float64(len(nodes))
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamp":           time.Now().Format(time.RFC3339),
		"requests_per_minute": 0, // 需要实现时间窗口统计
		"avg_response_time":   avgResponseTime,
		"node_load":           nodeLoad,
	})
}

// GetTasks 获取任务列表
// GET /api/v1/stats/tasks
func (h *Handlers) GetTasks(c *gin.Context) {
	// 简化实现：返回空列表
	// 实际实现需要任务追踪器
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    []interface{}{},
		"total":   0,
	})
}

// GetRunningTasks 获取运行中的任务
// GET /api/v1/stats/tasks/running
func (h *Handlers) GetRunningTasks(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	runningTasks := make([]gin.H, 0)
	for _, node := range nodes {
		activeRequests := node.GetActiveRequests()
		if activeRequests > 0 {
			runningTasks = append(runningTasks, gin.H{
				"node_id":         node.ID,
				"active_requests": activeRequests,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    runningTasks,
		"total":   len(runningTasks),
	})
}
