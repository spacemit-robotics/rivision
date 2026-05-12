// Package api 提供 WebSocket API
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WSConnectionManager WebSocket 连接管理器
type WSConnectionManager struct {
	mu            sync.RWMutex
	connections   map[*websocket.Conn]map[string]bool // conn -> subscribed channels
	pushInterval  time.Duration
	running       bool
	stopChan      chan struct{}
}

// NewWSConnectionManager 创建连接管理器
func NewWSConnectionManager() *WSConnectionManager {
	return &WSConnectionManager{
		connections:  make(map[*websocket.Conn]map[string]bool),
		pushInterval: 3 * time.Second,
		stopChan:     make(chan struct{}),
	}
}

// Connect 接受新连接
func (m *WSConnectionManager) Connect(conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[conn] = map[string]bool{
		"nodes": true,
		"stats": true,
		"tasks": true,
	}
	log.Printf("[WS] 新连接建立，当前连接数: %d", len(m.connections))
}

// Disconnect 断开连接
func (m *WSConnectionManager) Disconnect(conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.connections, conn)
	conn.Close()
	log.Printf("[WS] 连接断开，当前连接数: %d", len(m.connections))
}

// Broadcast 广播消息
func (m *WSConnectionManager) Broadcast(channel string, data interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	message := map[string]interface{}{
		"channel":   channel,
		"data":      data,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return
	}

	for conn, channels := range m.connections {
		if channels[channel] {
			if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
				go m.Disconnect(conn)
			}
		}
	}
}

// StartPushLoop 启动推送循环
func (m *WSConnectionManager) StartPushLoop(h *Handlers) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(m.pushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.pushAllData(h)
			case <-m.stopChan:
				return
			}
		}
	}()

	log.Println("[WS] 推送循环已启动")
}

// StopPushLoop 停止推送循环
func (m *WSConnectionManager) StopPushLoop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		m.running = false
		close(m.stopChan)
		log.Println("[WS] 推送循环已停止")
	}
}

// pushAllData 推送所有数据
func (m *WSConnectionManager) pushAllData(h *Handlers) {
	m.mu.RLock()
	connCount := len(m.connections)
	m.mu.RUnlock()

	if connCount == 0 {
		return
	}

	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	// 推送节点状态
	nodesData := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		nodesData = append(nodesData, node.GetStats())
	}
	m.Broadcast("nodes", map[string]interface{}{
		"nodes": nodesData,
		"stats": map[string]interface{}{
			"total":   len(nodes),
			"healthy": registry.HealthyCount(),
		},
	})

	// 推送系统统计
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

	m.Broadcast("stats", map[string]interface{}{
		"total_requests":    totalRequests,
		"success_rate":      successRate,
		"avg_response_time": avgResponseTime,
		"running_tasks":     0,
	})

	// ★ 推送 VLM 任务列表（使用 vlmAsyncTaskCount）
	vlmRunning := atomic.LoadInt64(&vlmAsyncTaskCount)
	vlmTasks := make([]map[string]interface{}, 0)
	for i := int64(0); i < vlmRunning; i++ {
		vlmTasks = append(vlmTasks, map[string]interface{}{
			"id":       fmt.Sprintf("vlm-task-%d", i+1),
			"type":     "image",
			"status":   "running",
			"progress": 50,
		})
	}
	m.Broadcast("tasks", map[string]interface{}{
		"tasks":       vlmTasks,
		"total":       len(vlmTasks),
		"vlm_running": vlmRunning,
	})
}

// ConnectionCount 获取连接数
func (m *WSConnectionManager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// BroadcastResult 广播 VLM 推理结果（特殊格式）
func (m *WSConnectionManager) BroadcastResult(data map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	message := map[string]interface{}{
		"type": "inference_result",
		"data": data,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return
	}

	for conn := range m.connections {
		if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
			go m.Disconnect(conn)
		}
	}
	log.Printf("[WS] 广播 VLM 结果到 %d 个连接", len(m.connections))
}

var wsManager = NewWSConnectionManager()

// HandleWebSocket 处理 WebSocket 连接
// GET /api/v1/ws/realtime
func (h *Handlers) HandleWebSocket(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] 升级连接失败: %v", err)
		return
	}

	wsManager.Connect(conn)

	// 启动推送循环（如果还没启动）
	wsManager.StartPushLoop(h)

	// 发送欢迎消息
	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": "已连接到实时数据流",
	}
	if data, err := json.Marshal(welcomeMsg); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	// 读取循环
	go func() {
		defer wsManager.Disconnect(conn)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// 处理订阅消息
			var msg map[string]interface{}
			if json.Unmarshal(message, &msg) == nil {
				if action, ok := msg["action"].(string); ok {
					switch action {
					case "subscribe":
						if channel, ok := msg["channel"].(string); ok {
							wsManager.mu.Lock()
							if channels, ok := wsManager.connections[conn]; ok {
								channels[channel] = true
							}
							wsManager.mu.Unlock()
						}
					case "unsubscribe":
						if channel, ok := msg["channel"].(string); ok {
							wsManager.mu.Lock()
							if channels, ok := wsManager.connections[conn]; ok {
								delete(channels, channel)
							}
							wsManager.mu.Unlock()
						}
					}
				}
			}
		}
	}()
}

// GetWSStatus 获取 WebSocket 状态
// GET /api/v1/ws/status
func (h *Handlers) GetWSStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"connection_count": wsManager.ConnectionCount(),
		"push_interval":    wsManager.pushInterval.String(),
	})
}
