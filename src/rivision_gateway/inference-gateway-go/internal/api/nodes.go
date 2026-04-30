// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package api 提供节点管理 API
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/inference-gateway/internal/auth"
	"github.com/rivision/inference-gateway/internal/nodes"
)

// NodeRegisterRequest 节点注册请求
type NodeRegisterRequest struct {
	ID                string   `json:"id" binding:"required"`
	Host              string   `json:"host" binding:"required"`
	Port              int      `json:"port"`       // llama-server 端口 (默认 9080)
	AgentPort         int      `json:"agent_port"` // Node Agent 端口 (默认 9090)
	YOLOPort          int      `json:"yolo_port"`  // YOLO Server 端口 (默认 9081)
	Weight            int      `json:"weight"`
	Tags              []string `json:"tags"`
	RegistrationToken string   `json:"registration_token" binding:"required"`
}

// NodeHeartbeatRequest 节点心跳请求
type NodeHeartbeatRequest struct {
	Host        string                 `json:"host,omitempty"` // ★ 节点当前 IP，用于检测 IP 变化
	Connections int                    `json:"connections"`
	Load        float64                `json:"load"`
	LlamaStatus map[string]interface{} `json:"llama_status"`
	YoloStatus  map[string]interface{} `json:"yolo_status"`
	SystemStats map[string]interface{} `json:"system_stats"`
	Resources   map[string]interface{} `json:"resources"`
	GPUStats    map[string]interface{} `json:"gpu_stats"`
}

// RegisterNode 注册节点
// POST /api/v1/nodes/register
func (h *Handlers) RegisterNode(c *gin.Context) {
	var req NodeRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 获取客户端 IP
	clientIP := c.ClientIP()
	if clientIP == "" {
		clientIP = c.Request.RemoteAddr
	}

	// 使用认证管理器进行安全注册
	authManager := auth.GetNodeAuthManager()

	// 检查黑名单
	if authManager.IsBlacklisted(clientIP) {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   "IP 在黑名单中",
		})
		return
	}

	// 验证 Token
	valid, msg := authManager.ValidateToken(req.RegistrationToken, req.ID)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   msg,
		})
		return
	}

	// 设置默认值
	if req.Port == 0 {
		req.Port = 9080
	}
	if req.AgentPort == 0 {
		req.AgentPort = 9090
	}
	if req.YOLOPort == 0 {
		req.YOLOPort = 9081
	}
	if req.Weight == 0 {
		req.Weight = 1
	}

	// ★ 修复：如果节点报告的 host 是 127.0.0.1，使用连接的真实 IP
	// 这通常发生在节点启动时网络未就绪
	nodeHost := req.Host
	if nodeHost == "" || nodeHost == "127.0.0.1" {
		// 从 clientIP 提取 IP 地址（可能包含端口）
		if idx := strings.LastIndex(clientIP, ":"); idx > 0 {
			// IPv4 地址带端口
			nodeHost = clientIP[:idx]
		} else {
			nodeHost = clientIP
		}
		log.Printf("[RegisterNode] 节点 %s 报告 host=%s，使用连接 IP: %s", req.ID, req.Host, nodeHost)
	}

	// 通过认证管理器注册
	success, regMsg := authManager.RegisterNode(
		req.ID, clientIP, req.RegistrationToken,
		nodeHost, req.Port, req.AgentPort, req.Weight, req.Tags,
	)
	if !success {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"error":   regMsg,
		})
		return
	}

	// 创建节点并注册到 Registry
	url := fmt.Sprintf("http://%s:%d", nodeHost, req.Port)
	node := nodes.NewNode(req.ID, req.ID, url, "")
	node.Weight = req.Weight
	node.Tags = req.Tags
	node.Host = nodeHost
	node.Port = req.Port
	node.AgentPort = req.AgentPort
	node.YOLOPort = req.YOLOPort
	node.SetStatus(nodes.NodeStatusHealthy)
	node.UpdateHeartbeat()

	h.client.GetRegistry().Register(node)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "节点注册成功",
		"node_id": req.ID,
	})
}

// NodeHeartbeat 节点心跳
// POST /api/v1/nodes/:id/heartbeat
func (h *Handlers) NodeHeartbeat(c *gin.Context) {
	nodeID := c.Param("id")

	var req NodeHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	// 获取客户端 IP
	clientIP := c.ClientIP()
	if clientIP == "" {
		clientIP = c.Request.RemoteAddr
	}

	// 更新认证管理器中的心跳
	authManager := auth.GetNodeAuthManager()
	authManager.UpdateNodeHeartbeat(nodeID, clientIP)

	registry := h.client.GetRegistry()
	node, ok := registry.Get(nodeID)
	if !ok {
		// ★ 不直接返回404：尝试从持久化存储恢复节点
		// Gateway重启后registry可能为空，从 auth manager 恢复
		regNode, found := authManager.GetRegisteredNode(nodeID)
		if found {
			url := fmt.Sprintf("http://%s:%d", regNode.Host, regNode.Port)
			recovered := nodes.NewNode(nodeID, nodeID, url, "")
			recovered.Host = regNode.Host
			recovered.Port = regNode.Port
			recovered.AgentPort = regNode.AgentPort
			recovered.Weight = regNode.Weight
			recovered.Tags = regNode.Tags
			recovered.Enabled = true
			recovered.SetStatus(nodes.NodeStatusHealthy)
			registry.Register(recovered)
			node, ok = registry.Get(nodeID)
		}

		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   fmt.Sprintf("节点 %s 不存在", nodeID),
			})
			return
		}
	}

	// 更新节点状态
	node.MarkHealthy()

	// 使用 HeartbeatData 统一更新
	hbData := &nodes.HeartbeatData{
		Host:        req.Host, // ★ 传递 Host 用于 IP 变化检测
		Connections: req.Connections,
		Load:        req.Load,
		LlamaStatus: req.LlamaStatus,
		YoloStatus:  req.YoloStatus,
		SystemStats: req.SystemStats,
		Resources:   req.Resources,
		GpuStats:    req.GPUStats,
	}
	node.UpdateStatus(hbData)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"node_id":   nodeID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// DeleteNode 删除节点（匹配 Python 的 unregister_node）
// DELETE /api/v1/nodes/:id
func (h *Handlers) DeleteNode(c *gin.Context) {
	nodeID := c.Param("id")

	registry := h.client.GetRegistry()
	authManager := auth.GetNodeAuthManager()

	// 检查节点是否存在（registry或auth中任一存在即可）
	_, inRegistry := registry.Get(nodeID)
	_, inAuth := authManager.GetRegisteredNode(nodeID)

	if !inRegistry && !inAuth {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("节点 %s 不存在", nodeID),
		})
		return
	}

	// 1. 从registry中注销
	registry.Unregister(nodeID)

	// 2. 从auth_manager中删除（包括持久化存储）
	authManager.UnregisterNode(nodeID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "节点注销成功，持久化数据已清理",
		"node_id": nodeID,
	})
}

// EnableNode 启用节点
// POST /api/v1/nodes/:id/enable
func (h *Handlers) EnableNode(c *gin.Context) {
	nodeID := c.Param("id")

	registry := h.client.GetRegistry()
	if !registry.SetNodeEnabled(nodeID, true) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "节点不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "节点已启用",
	})
}

// DisableNode 禁用节点
// POST /api/v1/nodes/:id/disable
func (h *Handlers) DisableNode(c *gin.Context) {
	nodeID := c.Param("id")

	registry := h.client.GetRegistry()
	if !registry.SetNodeEnabled(nodeID, false) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "节点不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "节点已禁用",
	})
}

// ============================================
// 远程进程控制 API (通过 node-agent)
// ============================================

// RestartNodeLlama 远程重启节点的 llama.cpp 服务
// POST /api/v1/nodes/:id/restart
func (h *Handlers) RestartNodeLlama(c *gin.Context) {
	h.proxyNodeAgent(c, "restart", "POST", "/api/v1/process/restart", nil)
}

// StopNodeLlama 远程停止节点的 llama.cpp 服务
// POST /api/v1/nodes/:id/stop
func (h *Handlers) StopNodeLlama(c *gin.Context) {
	h.proxyNodeAgent(c, "stop", "POST", "/api/v1/process/stop", nil)
}

// StartNodeLlama 远程启动节点的 llama.cpp 服务
// POST /api/v1/nodes/:id/start
func (h *Handlers) StartNodeLlama(c *gin.Context) {
	h.proxyNodeAgent(c, "start", "POST", "/api/v1/process/start", nil)
}

// KillNodeLlama 远程终止节点的 llama.cpp 进程
// POST /api/v1/nodes/:id/kill
func (h *Handlers) KillNodeLlama(c *gin.Context) {
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	h.proxyNodeAgent(c, "kill", "POST", "/api/v1/process/kill", body)
}

// GetNodeProcessStatus 获取节点的 llama.cpp 进程状态
// GET /api/v1/nodes/:id/process-status
func (h *Handlers) GetNodeProcessStatus(c *gin.Context) {
	h.proxyNodeAgent(c, "status", "GET", "/api/v1/process/status", nil)
}

// proxyNodeAgent 代理请求到 node-agent
func (h *Handlers) proxyNodeAgent(c *gin.Context, action, method, path string, body interface{}) {
	nodeID := c.Param("id")

	registry := h.client.GetRegistry()
	node, ok := registry.Get(nodeID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   fmt.Sprintf("节点 %s 不存在", nodeID),
		})
		return
	}

	agentPort := node.AgentPort
	if agentPort == 0 {
		agentPort = 9090
	}
	agentURL := fmt.Sprintf("http://%s:%d%s", node.Host, agentPort, path)

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var httpReq *http.Request
	var err error

	if body != nil && method == "POST" {
		jsonBody, _ := json.Marshal(body)
		httpReq, err = http.NewRequestWithContext(c.Request.Context(), method, agentURL, bytes.NewBuffer(jsonBody))
	} else {
		httpReq, err = http.NewRequestWithContext(c.Request.Context(), method, agentURL, nil)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   fmt.Sprintf("无法连接到节点 %s 的 node-agent (端口 %d)", nodeID, agentPort),
		})
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": fmt.Sprintf("llama.cpp %s 成功", action),
			"node_id": nodeID,
			"action":  action,
		})
	} else {
		c.JSON(resp.StatusCode, gin.H{
			"success": false,
			"error":   fmt.Sprintf("%s 失败: %s", action, string(respBody)),
		})
	}
}

// CheckNode 检查节点
// POST /api/v1/nodes/:id/check
func (h *Handlers) CheckNode(c *gin.Context) {
	nodeID := c.Param("id")

	registry := h.client.GetRegistry()
	node, ok := registry.Get(nodeID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "节点不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"node_id": nodeID,
		"healthy": node.IsHealthy(),
		"status":  node.GetRealStatus(60),
	})
}

// ListNodesSnapshot 获取节点列表快照（缓存版本）
// GET /api/v1/nodes/snapshot
func (h *Handlers) ListNodesSnapshot(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodeList := registry.GetAll()

	nodes := make([]map[string]interface{}, len(nodeList))
	for i, node := range nodeList {
		nodes[i] = map[string]interface{}{
			"id":                  node.ID,
			"host":                node.Host,
			"port":                node.Port,
			"healthy":             node.IsHealthy(),
			"llama_healthy":       node.LlamaHealthy,
			"load":                node.Load,
			"avg_response_time":   node.AvgResponseTime,
			"total_requests":      node.TotalRequests,
			"success_requests":    node.SuccessRequests,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"nodes":     nodes,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"cache_ttl": 10,
	})
}
