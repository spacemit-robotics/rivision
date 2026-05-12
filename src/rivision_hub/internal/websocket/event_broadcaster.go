// Package websocket 提供 WebSocket 服务 — event_broadcaster.go
// v6_design §6.12: /api/v1/ws/stream — 前端实时推送
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// MessageType 消息类型
type MessageType string

const (
	TypeAlert      MessageType = "alert"
	TypeDetection  MessageType = "detection"
	TypeNodeStatus MessageType = "node_status"
	TypeCamStatus  MessageType = "camera_status"
	TypeSystem     MessageType = "system"
	TypePing       MessageType = "ping"
	TypePong       MessageType = "pong"
)

// Message WebSocket 消息
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// ClientFilters 客户端过滤条件
type ClientFilters struct {
	Cameras     map[string]bool `json:"cameras"`      // 只接收这些摄像头的事件
	AlertLevels map[string]bool `json:"alert_levels"` // 只接收这些级别的告警
}

// Client WebSocket 客户端
type Client struct {
	id      string
	hub     *Hub
	conn    *websocket.Conn
	send    chan []byte
	topics  map[string]bool // 订阅的主题
	filters ClientFilters   // M3: 过滤条件
	mu      sync.Mutex
}

// Hub WebSocket 中心 (事件广播器)
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	topics     map[string]map[*Client]bool // 主题 -> 订阅者
	mu         sync.RWMutex
}

// NewHub 创建 Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		topics:     make(map[string]map[*Client]bool),
	}
}

// Run 运行 Hub
func (h *Hub) Run() {
	log.Printf("[WebSocket] Hub 启动")

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端连接: %s, 总数: %d", client.id, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				// 从所有主题移除
				for topic := range client.topics {
					if subscribers, ok := h.topics[topic]; ok {
						delete(subscribers, client)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端断开: %s, 总数: %d", client.id, len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast 广播消息
func (h *Hub) Broadcast(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	select {
	case h.broadcast <- data:
	default:
		log.Printf("[WebSocket] 广播队列已满")
	}
}

// BroadcastToTopic 向特定主题广播
func (h *Hub) BroadcastToTopic(topic string, msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	subscribers, ok := h.topics[topic]
	h.mu.RUnlock()

	if !ok {
		return
	}

	for client := range subscribers {
		select {
		case client.send <- data:
		default:
		}
	}
}

// SendAlert 发送告警 (M3: topic+filter aware)
func (h *Hub) SendAlert(alert interface{}) {
	msg := &Message{
		Type:      TypeAlert,
		Timestamp: time.Now(),
		Data:      alert,
	}
	h.BroadcastFiltered(string(TypeAlert), msg, alert)
}

// SendDetection 发送检测结果 (M3: topic+filter aware)
func (h *Hub) SendDetection(detection interface{}) {
	msg := &Message{
		Type:      TypeDetection,
		Timestamp: time.Now(),
		Data:      detection,
	}
	h.BroadcastFiltered(string(TypeDetection), msg, detection)
}

// BroadcastFiltered 按订阅主题和过滤条件分发消息 (M3)
func (h *Hub) BroadcastFiltered(topic string, msg *Message, rawData interface{}) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	// 提取 camera_id 和 level 用于过滤
	var cameraID, level string
	if m, ok := rawData.(map[string]interface{}); ok {
		if v, ok := m["camera_id"].(string); ok {
			cameraID = v
		}
		if v, ok := m["level"].(string); ok {
			level = v
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if !client.shouldReceive(topic, cameraID, level) {
			continue
		}
		select {
		case client.send <- data:
		default:
		}
	}
}

// shouldReceive 判断客户端是否应接收此消息 (M3)
func (c *Client) shouldReceive(topic, cameraID, level string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 无订阅 = 接收所有 (向后兼容)
	if len(c.topics) == 0 {
		return true
	}
	// 检查 topic 订阅
	if !c.topics[topic] {
		return false
	}
	// 检查 camera 过滤 (空 = 不过滤)
	if len(c.filters.Cameras) > 0 && cameraID != "" && !c.filters.Cameras[cameraID] {
		return false
	}
	// 检查告警级别过滤
	if len(c.filters.AlertLevels) > 0 && level != "" && !c.filters.AlertLevels[level] {
		return false
	}
	return true
}

// SendNodeStatus 发送节点状态
func (h *Hub) SendNodeStatus(status interface{}) {
	h.Broadcast(&Message{
		Type:      TypeNodeStatus,
		Timestamp: time.Now(),
		Data:      status,
	})
}

// SendCameraStatus 发送摄像头状态
func (h *Hub) SendCameraStatus(status interface{}) {
	h.Broadcast(&Message{
		Type:      TypeCamStatus,
		Timestamp: time.Now(),
		Data:      status,
	})
}

// GetStats 获取统计
func (h *Hub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	topicStats := make(map[string]int)
	for topic, subs := range h.topics {
		topicStats[topic] = len(subs)
	}

	return map[string]interface{}{
		"clients": len(h.clients),
		"topics":  topicStats,
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (h *Hub) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] 升级失败: %v", err)
		return
	}

	clientID := c.Query("client_id")
	if clientID == "" {
		clientID = c.ClientIP()
	}

	client := &Client{
		id:     clientID,
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		topics: make(map[string]bool),
		filters: ClientFilters{
			Cameras:     make(map[string]bool),
			AlertLevels: make(map[string]bool),
		},
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump 读取消息
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(65536)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] 读取错误: %v", err)
			}
			break
		}

		// 处理客户端消息
		c.handleMessage(message)
	}
}

// writePump 发送消息
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send each message as a separate WebSocket frame.
			// Previously messages were concatenated with \n into one frame,
			// which broke browser-side JSON.parse().
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Drain queued messages — each as its own frame
			n := len(c.send)
			for i := 0; i < n; i++ {
				if err := c.conn.WriteMessage(websocket.TextMessage, <-c.send); err != nil {
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 处理客户端消息 (M3: 支持 filters)
func (c *Client) handleMessage(data []byte) {
	var msg struct {
		Action  string   `json:"action"`
		Topics  []string `json:"topics"`
		Filters struct {
			Cameras     []string `json:"cameras"`
			AlertLevels []string `json:"alert_levels"`
		} `json:"filters"`
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Action {
	case "subscribe":
		c.subscribe(msg.Topics)
		c.setFilters(msg.Filters.Cameras, msg.Filters.AlertLevels)
	case "unsubscribe":
		c.unsubscribe(msg.Topics)
	case "ping":
		c.sendPong()
	}
}

// setFilters 设置过滤条件 (M3)
func (c *Client) setFilters(cameras, alertLevels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.filters.Cameras = make(map[string]bool, len(cameras))
	for _, cam := range cameras {
		c.filters.Cameras[cam] = true
	}
	c.filters.AlertLevels = make(map[string]bool, len(alertLevels))
	for _, lvl := range alertLevels {
		c.filters.AlertLevels[lvl] = true
	}

	if len(cameras) > 0 || len(alertLevels) > 0 {
		log.Printf("[WebSocket] 客户端 %s 设置过滤: cameras=%v levels=%v", c.id, cameras, alertLevels)
	}
}

// subscribe 订阅主题
func (c *Client) subscribe(topics []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()

	for _, topic := range topics {
		c.topics[topic] = true
		if c.hub.topics[topic] == nil {
			c.hub.topics[topic] = make(map[*Client]bool)
		}
		c.hub.topics[topic][c] = true
	}

	log.Printf("[WebSocket] 客户端 %s 订阅: %v", c.id, topics)
}

// unsubscribe 取消订阅
func (c *Client) unsubscribe(topics []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()

	for _, topic := range topics {
		delete(c.topics, topic)
		if subscribers, ok := c.hub.topics[topic]; ok {
			delete(subscribers, c)
		}
	}
}

// sendPong 发送 pong
func (c *Client) sendPong() {
	msg := &Message{
		Type:      TypePong,
		Timestamp: time.Now(),
	}
	data, _ := json.Marshal(msg)
	select {
	case c.send <- data:
	default:
	}
}
