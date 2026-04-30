package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // 允许所有来源
		},
	}
	clientCounter uint64
)

// HandleYOLOWebSocket 处理 YOLO WebSocket 连接（单摄像头，已废弃，保留向后兼容）
func HandleYOLOWebSocket(c *gin.Context, hub *Hub, cameraID string) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	clientID := fmt.Sprintf("yolo-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&clientCounter, 1))
	channel := "yolo:" + cameraID

	client := NewClient(clientID, channel, hub, conn)
	hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

// yoloMuxCommand 多路复用 WebSocket 命令
type yoloMuxCommand struct {
	Action   string `json:"action"`    // "subscribe" | "unsubscribe"
	CameraID string `json:"camera_id"`
}

// HandleYOLOWebSocketMux 多路复用 YOLO WebSocket：单连接支持多摄像头订阅
// 解决浏览器每域名 TCP 连接数限制（6）问题
func HandleYOLOWebSocketMux(c *gin.Context, hub *Hub) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	clientID := fmt.Sprintf("yolo-mux-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&clientCounter, 1))
	// 不设置初始频道，通过 subscribe 动态加入
	client := NewClient(clientID, "", hub, conn)
	hub.Register(client)

	log.Printf("[WebSocket] YOLO多路复用客户端连接: %s", clientID)

	// WritePump 正常启动（发送 YOLO 数据 + ping）
	go client.WritePump()

	// 自定义 ReadPump：处理 subscribe/unsubscribe 命令
	go func() {
		defer func() {
			hub.UnsubscribeAllChannels(client)
			hub.Unregister(client)
			conn.Close()
			log.Printf("[WebSocket] YOLO多路复用客户端断开: %s", clientID)
		}()

		conn.SetReadLimit(maxMessageSize)
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("[WebSocket] YOLO多路复用读取错误: %v", err)
				}
				break
			}

			var cmd yoloMuxCommand
			if err := json.Unmarshal(msg, &cmd); err != nil {
				continue
			}

			channel := "yolo:" + cmd.CameraID
			switch cmd.Action {
			case "subscribe":
				hub.SubscribeChannel(client, channel)
				log.Printf("[WebSocket] %s 订阅 %s", clientID, channel)
			case "unsubscribe":
				hub.UnsubscribeChannel(client, channel)
				log.Printf("[WebSocket] %s 取消订阅 %s", clientID, channel)
			}
		}
	}()
}

// HandleInferenceWebSocket 处理推理状态 WebSocket 连接
func HandleInferenceWebSocket(c *gin.Context, hub *Hub) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// 生成客户端 ID
	clientID := fmt.Sprintf("inference-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&clientCounter, 1))
	channel := "inference"

	client := NewClient(clientID, channel, hub, conn)
	hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
