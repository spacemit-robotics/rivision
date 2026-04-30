// Package websocket 提供 WebSocket 连接管理
package websocket

import (
	"log"
	"sync"
)

// Hub 管理所有 WebSocket 连接
type Hub struct {
	// 已注册的客户端
	clients map[*Client]bool

	// 按频道分组的客户端
	channels map[string]map[*Client]bool

	// 已关闭send channel的客户端（防止double-close panic）
	closedSend map[*Client]bool

	// 注册请求
	register chan *Client

	// 注销请求
	unregister chan *Client

	// 广播消息
	broadcast chan *Message

	// 频道消息
	channelMsg chan *ChannelMessage

	mu sync.RWMutex
}

// ChannelMessage 频道消息
type ChannelMessage struct {
	Channel string
	Message *Message
}

// NewHub 创建新的 Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		channels:   make(map[string]map[*Client]bool),
		closedSend: make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		channelMsg: make(chan *ChannelMessage, 256),
	}
}

// safeCloseSend 安全关闭客户端send channel（防止double-close panic）
func (h *Hub) safeCloseSend(client *Client) {
	if !h.closedSend[client] {
		close(client.send)
		h.closedSend[client] = true
	}
}

// Run 运行 Hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			// 加入频道
			if client.Channel != "" {
				if h.channels[client.Channel] == nil {
					h.channels[client.Channel] = make(map[*Client]bool)
				}
				h.channels[client.Channel][client] = true
			}
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端连接: %s, 频道: %s", client.ID, client.Channel)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				// 从频道移除
				if client.Channel != "" {
					if ch, ok := h.channels[client.Channel]; ok {
						delete(ch, client)
						if len(ch) == 0 {
							delete(h.channels, client.Channel)
						}
					}
				}
				h.safeCloseSend(client)
			}
			delete(h.closedSend, client)
			h.mu.Unlock()
			log.Printf("[WebSocket] 客户端断开: %s", client.ID)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// 慢客户端：丢弃消息而非断开连接
					log.Printf("[WebSocket] 广播丢帧: %s (缓冲区满)", client.ID)
				}
			}
			h.mu.RUnlock()

		case cm := <-h.channelMsg:
			h.mu.RLock()
			if clients, ok := h.channels[cm.Channel]; ok {
				for client := range clients {
					select {
					case client.send <- cm.Message:
					default:
						// 慢客户端：丢弃消息而非断开连接（YOLO二进制帧较大，容易缓冲区满）
						log.Printf("[WebSocket] 频道 %s 丢帧: %s (缓冲区满)", cm.Channel, client.ID)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast 广播消息给所有客户端
func (h *Hub) Broadcast(msg *Message) {
	h.broadcast <- msg
}

// SendToChannel 发送消息到指定频道
func (h *Hub) SendToChannel(channel string, msg *Message) {
	h.channelMsg <- &ChannelMessage{
		Channel: channel,
		Message: msg,
	}
}

// Register 注册客户端
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 注销客户端
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// GetClientCount 获取客户端数量
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetChannelClientCount 获取频道客户端数量
func (h *Hub) GetChannelClientCount(channel string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if clients, ok := h.channels[channel]; ok {
		return len(clients)
	}
	return 0
}

// SubscribeChannel 将客户端加入指定频道（支持多路复用WebSocket动态订阅）
func (h *Hub) SubscribeChannel(client *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.channels[channel] == nil {
		h.channels[channel] = make(map[*Client]bool)
	}
	h.channels[channel][client] = true
}

// UnsubscribeChannel 将客户端从指定频道移除
func (h *Hub) UnsubscribeChannel(client *Client, channel string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.channels[channel]; ok {
		delete(ch, client)
		if len(ch) == 0 {
			delete(h.channels, channel)
		}
	}
}

// UnsubscribeAllChannels 将客户端从所有频道移除（断开时清理）
func (h *Hub) UnsubscribeAllChannels(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for channel, clients := range h.channels {
		if clients[client] {
			delete(clients, client)
			if len(clients) == 0 {
				delete(h.channels, channel)
			}
		}
	}
}
