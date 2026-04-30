package websocket

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// 写入等待时间
	writeWait = 10 * time.Second

	// 读取 pong 等待时间
	pongWait = 60 * time.Second

	// ping 发送间隔
	pingPeriod = (pongWait * 9) / 10

	// 最大消息大小 (增加到2MB以支持帧数据)
	maxMessageSize = 2 * 1024 * 1024
)

// Client WebSocket 客户端
type Client struct {
	ID      string
	Channel string
	hub     *Hub
	conn    *websocket.Conn
	send    chan *Message
}

// NewClient 创建新客户端
func NewClient(id, channel string, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		ID:      id,
		Channel: channel,
		hub:     hub,
		conn:    conn,
		send:    make(chan *Message, 256),
	}
}

// ReadPump 读取消息
func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] 读取错误: %v", err)
			}
			break
		}
		// 目前不处理客户端消息，仅用于保持连接
	}
}

// WritePump 写入消息
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 发送JSON消息
			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("[WebSocket] 写入错误 (%s): %v", c.ID, err)
				return
			}
			
			// 如果有二进制数据，紧接着发送二进制消息
			if len(message.BinaryData) > 0 {
				if err := c.conn.WriteMessage(websocket.BinaryMessage, message.BinaryData); err != nil {
					log.Printf("[WebSocket] 二进制写入错误 (%s): %v", c.ID, err)
					return
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send 发送消息
func (c *Client) Send(msg *Message) {
	select {
	case c.send <- msg:
	default:
		log.Printf("[WebSocket] 发送队列已满: %s", c.ID)
	}
}
