// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package handlers 提供事件 WebSocket 处理器
package handlers

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rivision/rivision-cli/internal/event"
)

// EventsHandler 事件 WebSocket 处理器
type EventsHandler struct {
	eventBus *event.EventBus
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
}

// NewEventsHandler 创建事件处理器
func NewEventsHandler(eventBus *event.EventBus) *EventsHandler {
	h := &EventsHandler{
		eventBus: eventBus,
		clients:  make(map[*websocket.Conn]bool),
	}

	// 订阅所有事件并广播给 WebSocket 客户端
	eventBus.SubscribeAll(func(e *event.Event) {
		h.broadcastEvent(e)
	})

	return h
}

// HandleWebSocket 处理事件 WebSocket 连接（规格书 4.5）
// GET /ws/events
func (h *EventsHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[Events WS] 升级连接失败: %v", err)
		return
	}

	// 注册客户端
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	log.Printf("[Events WS] 新客户端连接，当前连接数: %d", len(h.clients))

	// 发送欢迎消息
	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": "已连接到事件流",
	}
	if data, err := json.Marshal(welcomeMsg); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	// 发送最近的事件历史
	history := h.eventBus.GetHistory(10)
	for _, e := range history {
		if data, err := e.ToJSON(); err == nil {
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}

	// 读取循环（处理 ping/pong 和关闭）
	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
		log.Printf("[Events WS] 客户端断开，当前连接数: %d", len(h.clients))
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// broadcastEvent 广播事件给所有客户端
func (h *EventsHandler) broadcastEvent(e *event.Event) {
	data, err := e.ToJSON()
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[Events WS] 发送消息失败: %v", err)
		}
	}
}

// GetClientCount 获取当前连接数
func (h *EventsHandler) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
