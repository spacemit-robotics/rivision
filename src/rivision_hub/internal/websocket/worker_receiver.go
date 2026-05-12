// Package websocket — worker_receiver.go
// v6_design §6.12: /api/v1/ws/events — Worker 事件接收
package websocket

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// EventReceiver Worker 事件接收器
type EventReceiver struct {
	workers  map[string]*WorkerConn
	mu       sync.RWMutex
	handlers map[string]EventHandler
	hub      *Hub // 用于转发到前端
}

// WorkerConn Worker 连接
type WorkerConn struct {
	NodeID   string
	Conn     *websocket.Conn
	LastPing time.Time
}

// WorkerEvent Worker 上报的事件
type WorkerEvent struct {
	Type      string      `json:"type"`
	NodeID    string      `json:"node_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// EventHandler 事件处理函数
type EventHandler func(event *WorkerEvent)

// NewEventReceiver 创建接收器
func NewEventReceiver(hub *Hub) *EventReceiver {
	return &EventReceiver{
		workers:  make(map[string]*WorkerConn),
		handlers: make(map[string]EventHandler),
		hub:      hub,
	}
}

// RegisterHandler 注册事件处理器
func (r *EventReceiver) RegisterHandler(eventType string, handler EventHandler) {
	r.handlers[eventType] = handler
}

// HandleWorkerWebSocket 处理 Worker WebSocket 连接
func (r *EventReceiver) HandleWorkerWebSocket(c *gin.Context) {
	nodeID := c.Query("node_id")
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 node_id"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[EventReceiver] WebSocket 升级失败: %v", err)
		return
	}

	workerConn := &WorkerConn{
		NodeID:   nodeID,
		Conn:     conn,
		LastPing: time.Now(),
	}

	r.mu.Lock()
	// 关闭旧连接
	if old, exists := r.workers[nodeID]; exists {
		old.Conn.Close()
	}
	r.workers[nodeID] = workerConn
	r.mu.Unlock()

	log.Printf("[EventReceiver] Worker 连接: %s", nodeID)

	// 通知前端节点状态变化
	if r.hub != nil {
		r.hub.SendNodeStatus(map[string]interface{}{
			"node_id": nodeID,
			"status":  "connected",
		})
	}

	// ★ 启动 Ping 循环保持连接活跃 (每 45 秒发送 Ping)
	go r.pingWorker(workerConn)
	go r.readWorkerMessages(workerConn)
}

// pingWorker 周期性发送 Ping 保持连接
func (r *EventReceiver) pingWorker(wc *WorkerConn) {
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	for {
		<-ticker.C
		r.mu.RLock()
		_, exists := r.workers[wc.NodeID]
		r.mu.RUnlock()
		if !exists {
			return // 连接已断开
		}
		if err := wc.Conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
			log.Printf("[EventReceiver] Ping Worker %s 失败: %v", wc.NodeID, err)
			return
		}
	}
}

// readWorkerMessages 读取 Worker 消息
func (r *EventReceiver) readWorkerMessages(wc *WorkerConn) {
	defer func() {
		r.mu.Lock()
		delete(r.workers, wc.NodeID)
		r.mu.Unlock()
		wc.Conn.Close()

		log.Printf("[EventReceiver] Worker 断开: %s", wc.NodeID)

		// 通知前端节点状态变化
		if r.hub != nil {
			r.hub.SendNodeStatus(map[string]interface{}{
				"node_id": wc.NodeID,
				"status":  "disconnected",
			})
		}
	}()

	wc.Conn.SetReadLimit(1024 * 1024) // 1MB
	wc.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	wc.Conn.SetPongHandler(func(string) error {
		wc.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		wc.LastPing = time.Now()
		return nil
	})

	for {
		_, message, err := wc.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[EventReceiver] Worker %s 读取错误: %v", wc.NodeID, err)
			}
			break
		}

		// Reset read deadline on every received message
		wc.Conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		r.processWorkerMessage(wc.NodeID, message)
	}
}

// processWorkerMessage 处理 Worker 消息
func (r *EventReceiver) processWorkerMessage(nodeID string, data []byte) {
	var event WorkerEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("[EventReceiver] 解析消息失败: %v", err)
		return
	}

	event.NodeID = nodeID
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// 调用注册的处理器
	if handler, ok := r.handlers[event.Type]; ok {
		handler(&event)
	}

	// 转发特定事件到前端
	if r.hub != nil {
		switch event.Type {
		case "alert":
			r.hub.SendAlert(event.Data)
		case "detection":
			r.hub.SendDetection(event.Data)
		}
	}
}

// SendToWorker 发送消息到 Worker
func (r *EventReceiver) SendToWorker(nodeID string, msg interface{}) error {
	r.mu.RLock()
	wc, ok := r.workers[nodeID]
	r.mu.RUnlock()

	if !ok {
		return nil // Worker 未连接
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	wc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return wc.Conn.WriteMessage(websocket.TextMessage, data)
}

// BroadcastToWorkers 广播到所有 Worker
func (r *EventReceiver) BroadcastToWorkers(msg interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, wc := range r.workers {
		wc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		wc.Conn.WriteMessage(websocket.TextMessage, data)
	}
}

// GetConnectedWorkers 获取已连接的 Worker
func (r *EventReceiver) GetConnectedWorkers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workers := make([]string, 0, len(r.workers))
	for nodeID := range r.workers {
		workers = append(workers, nodeID)
	}
	return workers
}

// IsWorkerConnected 检查 Worker 是否连接
func (r *EventReceiver) IsWorkerConnected(nodeID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.workers[nodeID]
	return ok
}

// StartPingLoop 启动 Ping 循环
func (r *EventReceiver) StartPingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pingWorkers()
		}
	}
}

// pingWorkers 向所有 Worker 发送 Ping
func (r *EventReceiver) pingWorkers() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, wc := range r.workers {
		wc.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := wc.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			log.Printf("[EventReceiver] Ping Worker %s 失败: %v", wc.NodeID, err)
		}
	}
}

// GetReceiverStats 获取接收器统计
func (r *EventReceiver) GetReceiverStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	workers := make([]map[string]interface{}, 0)
	for nodeID, wc := range r.workers {
		workers = append(workers, map[string]interface{}{
			"node_id":   nodeID,
			"last_ping": wc.LastPing,
		})
	}

	return map[string]interface{}{
		"connected_workers": len(r.workers),
		"workers":           workers,
	}
}
