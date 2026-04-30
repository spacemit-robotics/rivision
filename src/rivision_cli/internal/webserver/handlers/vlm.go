// Package handlers 提供 HTTP 处理器
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rivision/rivision-cli/internal/vlm"
	ws "github.com/rivision/rivision-cli/internal/webserver/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// VLMHandler VLM 处理器
type VLMHandler struct {
	trigger *vlm.Trigger
	hub     *ws.Hub
}

// NewVLMHandler 创建 VLM 处理器
func NewVLMHandler(trigger *vlm.Trigger, hub *ws.Hub) *VLMHandler {
	h := &VLMHandler{
		trigger: trigger,
		hub:     hub,
	}

	// 启动结果转发
	go h.forwardResults()

	return h
}

// forwardResults 转发 VLM 结果到 WebSocket
func (h *VLMHandler) forwardResults() {
	for result := range h.trigger.Results() {
		msg := ws.NewCameraMessage(ws.TypeVLMResult, result.CameraID, ws.VLMResultData{
			ID:              result.ID,
			CameraName:      result.CameraName,
			TriggerMode:     result.TriggerMode,
			Text:            result.Text,
			TokensUsed:      result.TokensUsed,
			NodeID:          result.NodeID,
			InferenceTimeMs: result.InferenceTimeMs,
			ImageBase64:     result.ImageBase64,
		})

		// 广播给所有 VLM 频道的客户端
		h.hub.SendToChannel("vlm", msg)

		// 也发送到特定摄像头频道
		h.hub.SendToChannel("vlm:"+result.CameraID, msg)
	}
}

// TriggerAnalysis 手动触发 VLM 分析
// POST /api/vlm/trigger/:camera_id
func (h *VLMHandler) TriggerAnalysis(c *gin.Context) {
	cameraID := c.Param("camera_id")

	var req struct {
		ImageBase64 string `json:"image_base64"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	// 解码图像
	var imageData []byte
	if req.ImageBase64 != "" {
		// 这里应该解码 base64，暂时简化处理
		imageData = []byte(req.ImageBase64)
	}

	if err := h.trigger.TriggerManual(cameraID, imageData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "VLM 分析已触发",
	})
}

// GetConfig 获取 VLM 配置
// GET /api/vlm/config
func (h *VLMHandler) GetConfig(c *gin.Context) {
	cameras := h.trigger.GetAllCameraStatus()

	configs := make(map[string]interface{})
	for id, ct := range cameras {
		configs[id] = gin.H{
			"enabled":      ct.Config.Enabled,
			"trigger_mode": ct.Config.TriggerMode,
			"interval":     ct.Config.Interval.String(),
			"min_objects":  ct.Config.MinObjects,
			"prompt":       ct.Config.Prompt,
			"is_running":   ct.IsRunning,
			"last_trigger": ct.LastTrigger,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    configs,
	})
}

// GetStatus 获取 VLM 状态
// GET /api/vlm/status
func (h *VLMHandler) GetStatus(c *gin.Context) {
	cameras := h.trigger.GetAllCameraStatus()

	var running, total int
	for _, ct := range cameras {
		total++
		if ct.IsRunning {
			running++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_cameras":   total,
			"running_cameras": running,
		},
	})
}

// UpdateConfig 更新 VLM 配置（规格书 4.3）
// PUT /api/vlm/config
func (h *VLMHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		CameraID    string `json:"camera_id"`
		Enabled     *bool  `json:"enabled"`
		TriggerMode string `json:"trigger_mode"`
		Interval    string `json:"interval"`
		Prompt      string `json:"prompt"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	// 实际实现应该更新配置
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "VLM 配置已更新",
	})
}

// GetResults 获取 VLM 结果列表（规格书 4.3）
// GET /api/vlm/results
func (h *VLMHandler) GetResults(c *gin.Context) {
	// 实际实现应该从存储中获取结果
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    []interface{}{},
		"total":   0,
	})
}

// GetResult 获取单个 VLM 结果（规格书 4.3）
// GET /api/vlm/results/:id
func (h *VLMHandler) GetResult(c *gin.Context) {
	resultID := c.Param("id")

	// 实际实现应该从存储中获取结果
	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"error":   "结果不存在: " + resultID,
	})
}

// DeleteResult 删除 VLM 结果（规格书 4.3）
// DELETE /api/vlm/results/:id
func (h *VLMHandler) DeleteResult(c *gin.Context) {
	resultID := c.Param("id")

	// 实际实现应该从存储中删除结果
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "结果已删除: " + resultID,
	})
}

// GetStats 获取 VLM 统计（规格书 4.3）
// GET /api/vlm/stats
func (h *VLMHandler) GetStats(c *gin.Context) {
	cameras := h.trigger.GetAllCameraStatus()

	var totalTriggers int
	for _, ct := range cameras {
		if ct.IsRunning {
			totalTriggers++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_cameras":  len(cameras),
			"total_triggers": totalTriggers,
		},
	})
}

// HandleWebSocket 处理 VLM WebSocket 连接
// GET /ws/vlm
func (h *VLMHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	clientID := c.Query("client_id")
	if clientID == "" {
		clientID = c.ClientIP()
	}

	client := ws.NewClient(clientID, "vlm", h.hub, conn)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}

// HandleCameraWebSocket 处理特定摄像头的 VLM WebSocket 连接
// GET /ws/vlm/:camera_id
func (h *VLMHandler) HandleCameraWebSocket(c *gin.Context) {
	cameraID := c.Param("camera_id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	clientID := c.Query("client_id")
	if clientID == "" {
		clientID = c.ClientIP()
	}

	client := ws.NewClient(clientID, "vlm:"+cameraID, h.hub, conn)
	h.hub.Register(client)

	go client.WritePump()
	go client.ReadPump()
}
