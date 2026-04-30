// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package nodes 提供节点管理功能
package nodes

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// NodeStatus 节点状态
type NodeStatus string

const (
	NodeStatusHealthy   NodeStatus = "healthy"
	NodeStatusUnhealthy NodeStatus = "unhealthy"
	NodeStatusUnknown   NodeStatus = "unknown"
)

// Node 推理节点
type Node struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	URL         string            `json:"url" yaml:"url"`
	Model       string            `json:"model" yaml:"model"`
	Weight      int               `json:"weight" yaml:"weight"`
	MaxParallel int               `json:"max_parallel" yaml:"max_parallel"`
	Tags        []string          `json:"tags" yaml:"tags"`
	Metadata    map[string]string `json:"metadata" yaml:"metadata"`

	// 运行时状态
	Status           NodeStatus `json:"status"`
	LastHealthCheck  time.Time  `json:"last_health_check"`
	LastHeartbeat    time.Time  `json:"last_heartbeat"`
	FailCount        int        `json:"fail_count"`
	ActiveRequests   int        `json:"active_requests"`
	TotalRequests    int64      `json:"total_requests"`
	SuccessRequests  int64      `json:"success_requests"`
	AvgResponseTime  float64    `json:"avg_response_time_ms"`
	LastResponseTime float64    `json:"last_response_time_ms"`

	// 扩展状态 (来自心跳)
	Host             string             `json:"host"`
	Port             int                `json:"port"`              // llama-server 端口 (9080)
	AgentPort        int                `json:"agent_port"`        // Node Agent 端口 (9090)
	YOLOPort         int                `json:"yolo_port"`         // YOLO Server 端口 (9081)
	Enabled          bool               `json:"enabled"`
	Load             float64            `json:"load"`
	CPUPercent       float64            `json:"cpu_percent"`
	MemoryPercent    float64            `json:"memory_percent"`
	GpuMemoryPercent float64            `json:"gpu_memory_percent"`
	LlamaHealthy     bool               `json:"llama_healthy"`
	LlamaEnabled     bool               `json:"llama_enabled"`     // ★ 节点是否启用 llama 服务
	YoloHealthy      bool               `json:"yolo_healthy"`
	YoloEnabled      bool               `json:"yolo_enabled"`      // ★ 节点是否启用 yolo 服务
	SlotsIdle        int                `json:"slots_idle"`
	SlotsProc        int                `json:"slots_processing"`
	Resources        map[string]float64 `json:"resources"`

	mu sync.RWMutex
}

// NewNode 创建节点
func NewNode(id, name, url, model string) *Node {
	return &Node{
		ID:          id,
		Name:        name,
		URL:         url,
		Model:       model,
		Weight:      1,
		MaxParallel: 1,
		Status:      NodeStatusUnknown,
		Tags:        []string{},
		Metadata:    make(map[string]string),
		Enabled:     true,
	}
}

// IsHeartbeatTimeout 检查心跳是否超时
func (n *Node) IsHeartbeatTimeout(timeoutSeconds int) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.LastHeartbeat.IsZero() {
		return true
	}
	return time.Since(n.LastHeartbeat).Seconds() > float64(timeoutSeconds)
}

// UpdateHeartbeat 更新心跳时间
func (n *Node) UpdateHeartbeat() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.LastHeartbeat = time.Now()
}

// HeartbeatData 心跳数据
type HeartbeatData struct {
	Host          string                 `json:"host,omitempty"`           // ★ 节点当前 IP，用于检测 IP 变化
	Connections   int                    `json:"connections"`
	Load          float64                `json:"load"`
	LlamaStatus   map[string]interface{} `json:"llama_status"`
	YoloStatus    map[string]interface{} `json:"yolo_status"`
	SystemStats   map[string]interface{} `json:"system_stats"`
	Resources     map[string]interface{} `json:"resources"`
	GpuStats      map[string]interface{} `json:"gpu_stats"`
	NodeTimestamp int64                  `json:"node_timestamp,omitempty"` // ★ O3: Node本地时间戳(ms)，用于检测时间偏差
}

// UpdateStatus 更新节点状态（来自心跳）
// 注意：ActiveRequests 由 Gateway 内部维护（select_and_acquire/release_node），
// 节点报告的 connections 仅存储在 SlotsProc 供参考
func (n *Node) UpdateStatus(data *HeartbeatData) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Load = data.Load
	n.LastHeartbeat = time.Now()

	// ★ 修复：检测 IP 变化（节点重启后网络就绪）
	// 如果心跳报告了新 IP，且当前记录是 127.0.0.1 或空，则更新
	if data.Host != "" && data.Host != "127.0.0.1" {
		if n.Host == "" || n.Host == "127.0.0.1" {
			log.Printf("[Node] IP 更新: %s: %s -> %s", n.ID, n.Host, data.Host)
			n.Host = data.Host
			// 同时更新 URL
			if n.Port > 0 {
				n.URL = fmt.Sprintf("http://%s:%d", n.Host, n.Port)
			}
		}
	}

	// ★ O3: 检测时间偏差（如果Node发送了时间戳）
	// 支持任意大小的时间偏差（秒、分钟、小时、天）
	if data.NodeTimestamp > 0 {
		gatewayMs := time.Now().UnixMilli()
		driftMs := gatewayMs - data.NodeTimestamp
		// 偏差超过5秒时记录警告
		if driftMs > 5000 || driftMs < -5000 {
			// 格式化为完整日期时间（支持跨天偏差）
			gatewayTime := time.UnixMilli(gatewayMs).Format("2006-01-02 15:04:05")
			nodeTime := time.UnixMilli(data.NodeTimestamp).Format("2006-01-02 15:04:05")
			
			// 计算偏差方向
			direction := "快"
			absDriftMs := driftMs
			if driftMs < 0 {
				direction = "慢"
				absDriftMs = -driftMs
			}
			
			// 格式化偏差为人类可读形式
			driftStr := formatDuration(absDriftMs)
			
			log.Printf("[TimeSync] ⚠️ 时间偏差警告: node=%s | Gateway=%s | Node=%s | Node比Gateway%s %s",
				n.ID, gatewayTime, nodeTime, direction, driftStr)
		}
	}

	if data.LlamaStatus != nil {
		if v, ok := data.LlamaStatus["healthy"]; ok {
			if b, ok := v.(bool); ok {
				n.LlamaHealthy = b
			}
		}
		// ★ 解析 llama 服务启用状态
		if v, ok := data.LlamaStatus["enabled"]; ok {
			if b, ok := v.(bool); ok {
				n.LlamaEnabled = b
			}
		}
		if v, ok := data.LlamaStatus["slots_idle"]; ok {
			n.SlotsIdle = toInt(v)
		}
		if v, ok := data.LlamaStatus["slots_processing"]; ok {
			n.SlotsProc = toInt(v)
		}
	}

	if data.YoloStatus != nil {
		if v, ok := data.YoloStatus["healthy"]; ok {
			if b, ok := v.(bool); ok {
				n.YoloHealthy = b
			}
		}
		// ★ 解析 yolo 服务启用状态
		if v, ok := data.YoloStatus["enabled"]; ok {
			if b, ok := v.(bool); ok {
				n.YoloEnabled = b
			}
		}
	}

	if data.SystemStats != nil {
		n.CPUPercent = toFloat64(data.SystemStats["cpu_percent"])
		n.MemoryPercent = toFloat64(data.SystemStats["memory_percent"])
	}

	if data.Resources != nil {
		if n.Resources == nil {
			n.Resources = make(map[string]float64)
		}
		for k, v := range data.Resources {
			n.Resources[k] = toFloat64(v)
		}
		n.MemoryPercent = toFloat64(data.Resources["memory_percent"])
		n.GpuMemoryPercent = toFloat64(data.Resources["gpu_memory_percent"])
	}
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case int64:
		return int(val)
	default:
		return 0
	}
}

// GetRealStatus 获取真实状态（考虑心跳超时）
func (n *Node) GetRealStatus(heartbeatTimeout int) string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	// 直接检查心跳（不调用 IsHeartbeatTimeout 避免死锁）
	if n.LastHeartbeat.IsZero() || time.Since(n.LastHeartbeat).Seconds() > float64(heartbeatTimeout) {
		return "offline"
	}
	if n.Status != NodeStatusHealthy {
		return "unhealthy"
	}
	return "online"
}

// MarkHealthy 标记为健康
func (n *Node) MarkHealthy() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Status = NodeStatusHealthy
	n.FailCount = 0
	n.LastHealthCheck = time.Now()
}

// MarkUnhealthy 标记为不健康
func (n *Node) MarkUnhealthy(threshold int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailCount++
	n.LastHealthCheck = time.Now()
	if n.FailCount >= threshold {
		n.Status = NodeStatusUnhealthy
	}
}

// EffectiveWeight 计算有效权重
func (n *Node) EffectiveWeight() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.Status != NodeStatusHealthy || !n.LlamaHealthy {
		return 0.0
	}
	cpuFactor := max(0, 1-n.CPUPercent/100)
	memFactor := max(0, 1-n.MemoryPercent/100)
	connFactor := max(0.1, 1-float64(n.ActiveRequests)*0.2)
	effective := float64(n.Weight) * cpuFactor * memFactor * connFactor
	return max(0.1, effective)
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// IsHealthy 是否健康
func (n *Node) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Status == NodeStatusHealthy
}

// SetStatus 设置状态
func (n *Node) SetStatus(status NodeStatus) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Status = status
	n.LastHealthCheck = time.Now()
}

// IncrementFailCount 增加失败计数
func (n *Node) IncrementFailCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailCount++
	return n.FailCount
}

// ResetFailCount 重置失败计数
func (n *Node) ResetFailCount() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.FailCount = 0
}

// IncrementActiveRequests 增加活跃请求数
func (n *Node) IncrementActiveRequests() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ActiveRequests++
	n.TotalRequests++
	return n.ActiveRequests
}

// DecrementActiveRequests 减少活跃请求数
func (n *Node) DecrementActiveRequests() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ActiveRequests > 0 {
		n.ActiveRequests--
	}
	return n.ActiveRequests
}

// RecordSuccess 记录成功请求
func (n *Node) RecordSuccess(responseTimeMs float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.SuccessRequests++
	n.LastResponseTime = responseTimeMs
	// 计算移动平均
	if n.AvgResponseTime == 0 {
		n.AvgResponseTime = responseTimeMs
	} else {
		n.AvgResponseTime = n.AvgResponseTime*0.9 + responseTimeMs*0.1
	}
}

// GetActiveRequests 获取活跃请求数
func (n *Node) GetActiveRequests() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.ActiveRequests
}

// HasCapacity 是否有容量
func (n *Node) HasCapacity() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.ActiveRequests < n.MaxParallel
}

// GetStats 获取统计信息
func (n *Node) GetStats() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return map[string]interface{}{
		"id":                 n.ID,
		"name":               n.Name,
		"url":                n.URL,
		"model":              n.Model,
		"status":             n.Status,
		"active_requests":    n.ActiveRequests,
		"total_requests":     n.TotalRequests,
		"success_requests":   n.SuccessRequests,
		"avg_response_time":  n.AvgResponseTime,
		"last_response_time": n.LastResponseTime,
		"fail_count":         n.FailCount,
		"last_health_check":  n.LastHealthCheck,
	}
}

// ToDict 转换为字典（用于 API 响应，匹配 Python 的 to_dict）
func (n *Node) ToDict() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()

	realStatus := "online"
	if n.LastHeartbeat.IsZero() || time.Since(n.LastHeartbeat).Seconds() > 60 {
		realStatus = "offline"
	} else if n.Status != NodeStatusHealthy {
		realStatus = "unhealthy"
	}
	isReallyHealthy := realStatus == "online"

	var lastCheck, lastHB interface{}
	if !n.LastHealthCheck.IsZero() {
		lastCheck = n.LastHealthCheck.UTC().Format(time.RFC3339)
	}
	if !n.LastHeartbeat.IsZero() {
		lastHB = n.LastHeartbeat.UTC().Format(time.RFC3339)
	}

	url := n.URL
	if url == "" && n.Host != "" {
		url = fmt.Sprintf("http://%s:%d", n.Host, n.Port)
	}

	return map[string]interface{}{
		"id":               n.ID,
		"host":             n.Host,
		"port":             n.Port,
		"url":              url,
		"enabled":          n.Enabled,
		"healthy":          isReallyHealthy,
		"status":           realStatus,
		"connections":      n.ActiveRequests,
		"weight":           n.Weight,
		"tags":             n.Tags,
		"agent_port":       n.AgentPort,
		"fail_count":       n.FailCount,
		"last_check":       lastCheck,
		"load":             n.Load,
		"cpu_percent":      n.CPUPercent,
		"memory_percent":   n.MemoryPercent,
		"llama_enabled":    n.LlamaEnabled,    // ★ 节点是否启用 llama 服务
		"llama_healthy":    n.LlamaHealthy,
		"yolo_enabled":     n.YoloEnabled,     // ★ 节点是否启用 yolo 服务
		"yolo_healthy":     n.YoloHealthy,
		"slots_idle":       n.SlotsIdle,
		"slots_processing": n.SlotsProc,
		"last_heartbeat":   lastHB,
	}
}

// formatDuration 将毫秒偏差格式化为人类可读形式
// 支持极端偏差：秒、分钟、小时、天、年
func formatDuration(ms int64) string {
	sec := ms / 1000
	if sec < 60 {
		return fmt.Sprintf("%.1f秒", float64(ms)/1000.0)
	}
	if sec < 3600 {
		return fmt.Sprintf("%d分%d秒", sec/60, sec%60)
	}
	if sec < 86400 {
		return fmt.Sprintf("%d小时%d分", sec/3600, (sec%3600)/60)
	}
	days := sec / 86400
	if days < 365 {
		return fmt.Sprintf("%d天%d小时", days, (sec%86400)/3600)
	}
	years := days / 365
	return fmt.Sprintf("%d年%d天", years, days%365)
}
