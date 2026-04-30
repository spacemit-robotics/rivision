// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package api 提供监控指标 API
package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// MetricsCollector 指标收集器
type MetricsCollector struct {
	mu              sync.RWMutex
	requestsPerMin  []TimeSeriesPoint
	responseTimes   []TimeSeriesPoint
	errorRates      []TimeSeriesPoint
	nodeLoads       map[string][]TimeSeriesPoint
	lastCollectTime time.Time
	maxPoints       int
}

// TimeSeriesPoint 时间序列数据点
type TimeSeriesPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		requestsPerMin: make([]TimeSeriesPoint, 0, 60),
		responseTimes:  make([]TimeSeriesPoint, 0, 60),
		errorRates:     make([]TimeSeriesPoint, 0, 60),
		nodeLoads:      make(map[string][]TimeSeriesPoint),
		maxPoints:      60,
	}
}

// Collect 采集指标
func (m *MetricsCollector) Collect(h *Handlers) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	if now.Sub(m.lastCollectTime) < time.Minute {
		return
	}
	m.lastCollectTime = now
	timestamp := now.Format(time.RFC3339)

	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	// 采集请求数
	var totalRequests int64
	var successRequests int64
	var totalResponseTime float64
	for _, node := range nodes {
		stats := node.GetStats()
		totalRequests += stats["total_requests"].(int64)
		successRequests += stats["success_requests"].(int64)
		totalResponseTime += stats["avg_response_time"].(float64)
	}

	// 添加数据点
	m.requestsPerMin = append(m.requestsPerMin, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     float64(totalRequests),
	})
	if len(m.requestsPerMin) > m.maxPoints {
		m.requestsPerMin = m.requestsPerMin[1:]
	}

	avgResponseTime := float64(0)
	if len(nodes) > 0 {
		avgResponseTime = totalResponseTime / float64(len(nodes))
	}
	m.responseTimes = append(m.responseTimes, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     avgResponseTime,
	})
	if len(m.responseTimes) > m.maxPoints {
		m.responseTimes = m.responseTimes[1:]
	}

	errorRate := float64(0)
	if totalRequests > 0 {
		errorRate = float64(totalRequests-successRequests) / float64(totalRequests) * 100
	}
	m.errorRates = append(m.errorRates, TimeSeriesPoint{
		Timestamp: timestamp,
		Value:     errorRate,
	})
	if len(m.errorRates) > m.maxPoints {
		m.errorRates = m.errorRates[1:]
	}

	// 采集节点负载
	for _, node := range nodes {
		if _, ok := m.nodeLoads[node.ID]; !ok {
			m.nodeLoads[node.ID] = make([]TimeSeriesPoint, 0, 60)
		}
		load := float64(node.GetActiveRequests()) / float64(node.MaxParallel) * 100
		m.nodeLoads[node.ID] = append(m.nodeLoads[node.ID], TimeSeriesPoint{
			Timestamp: timestamp,
			Value:     load,
		})
		if len(m.nodeLoads[node.ID]) > m.maxPoints {
			m.nodeLoads[node.ID] = m.nodeLoads[node.ID][1:]
		}
	}
}

// GetData 获取指标数据
func (m *MetricsCollector) GetData() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"requests_per_minute": m.requestsPerMin,
		"response_times":      m.responseTimes,
		"error_rates":         m.errorRates,
		"node_loads":          m.nodeLoads,
	}
}

var metricsCollector = NewMetricsCollector()

// GetRealtimeMetrics 获取实时监控指标
// GET /api/v1/metrics/realtime
func (h *Handlers) GetRealtimeMetrics(c *gin.Context) {
	metricsCollector.Collect(h)

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

	successRate := float64(100)
	if totalRequests > 0 {
		successRate = float64(successRequests) / float64(totalRequests) * 100
	}

	avgResponseTime := float64(0)
	if len(nodes) > 0 {
		avgResponseTime = totalResponseTime / float64(len(nodes))
	}

	// 节点详情
	nodeDetails := make([]gin.H, 0, len(nodes))
	for _, node := range nodes {
		stats := node.GetStats()
		nodeDetails = append(nodeDetails, gin.H{
			"id":             node.ID,
			"healthy":        node.IsHealthy(),
			"running_tasks":  stats["active_requests"],
			"total_requests": stats["total_requests"],
			"success_rate":   100.0,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamp":         time.Now().Format(time.RFC3339),
		"total_nodes":       len(nodes),
		"healthy_nodes":     registry.HealthyCount(),
		"total_requests":    totalRequests,
		"success_rate":      successRate,
		"avg_response_time": avgResponseTime,
		"nodes":             nodeDetails,
	})
}

// GetMetricsTimeSeries 获取时间序列指标
// GET /api/v1/metrics/timeseries
func (h *Handlers) GetMetricsTimeSeries(c *gin.Context) {
	metricsCollector.Collect(h)
	c.JSON(http.StatusOK, metricsCollector.GetData())
}

// GetPrometheusMetrics 获取 Prometheus 格式指标
// GET /api/v1/metrics/prometheus
func (h *Handlers) GetPrometheusMetrics(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	var totalRequests, successRequests int64
	for _, node := range nodes {
		stats := node.GetStats()
		totalRequests += stats["total_requests"].(int64)
		successRequests += stats["success_requests"].(int64)
	}

	// Prometheus 格式输出
	output := fmt.Sprintf(`# HELP gateway_nodes_total Total number of nodes
# TYPE gateway_nodes_total gauge
gateway_nodes_total %d

# HELP gateway_nodes_healthy Number of healthy nodes
# TYPE gateway_nodes_healthy gauge
gateway_nodes_healthy %d

# HELP gateway_requests_total Total number of requests
# TYPE gateway_requests_total counter
gateway_requests_total %d

# HELP gateway_requests_success Total number of successful requests
# TYPE gateway_requests_success counter
gateway_requests_success %d
`, len(nodes), registry.HealthyCount(), totalRequests, successRequests)

	// 每个节点的指标
	for _, node := range nodes {
		stats := node.GetStats()
		output += fmt.Sprintf(`
# HELP gateway_node_requests_total Total requests for node
# TYPE gateway_node_requests_total counter
gateway_node_requests_total{node_id="%s"} %d

# HELP gateway_node_active_requests Active requests for node
# TYPE gateway_node_active_requests gauge
gateway_node_active_requests{node_id="%s"} %d
`, node.ID, stats["total_requests"], node.ID, stats["active_requests"])
	}

	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(output))
}
