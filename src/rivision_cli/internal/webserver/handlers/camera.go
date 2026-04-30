package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/camera"
	"github.com/rivision/rivision-cli/internal/config"
)

// CameraHandler 摄像头处理器
type CameraHandler struct {
	manager *camera.Manager
}

// NewCameraHandler 创建摄像头处理器
func NewCameraHandler(manager *camera.Manager) *CameraHandler {
	return &CameraHandler{
		manager: manager,
	}
}

// List 列出所有摄像头
// GET /api/cameras
func (h *CameraHandler) List(c *gin.Context) {
	cameras := h.manager.List()

	result := make([]gin.H, len(cameras))
	for i, cam := range cameras {
		status := cam.GetStatus()
		result[i] = gin.H{
			"id":          status.ID,
			"name":        status.Name,
			"source":      status.Source,
			"state":       status.State,
			"enabled":     status.Enabled,
			"frame_count": status.FrameCount,
			"uptime":      status.Uptime,
			"last_error":  status.LastError,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// Get 获取单个摄像头
// GET /api/cameras/:id
func (h *CameraHandler) Get(c *gin.Context) {
	cameraID := c.Param("id")

	cam, ok := h.manager.Get(cameraID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "摄像头不存在",
		})
		return
	}

	status := cam.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":          status.ID,
			"name":        status.Name,
			"source":      status.Source,
			"state":       status.State,
			"enabled":     status.Enabled,
			"frame_count": status.FrameCount,
			"uptime":      status.Uptime,
			"last_error":  status.LastError,
		},
	})
}

// Add 添加摄像头
// POST /api/cameras
func (h *CameraHandler) Add(c *gin.Context) {
	var req struct {
		ID      string `json:"id" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Source  string `json:"source" binding:"required"`
		Enabled bool   `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	cfg := config.CameraConfig{
		ID:      req.ID,
		Name:    req.Name,
		Source:  req.Source,
		Enabled: req.Enabled,
	}

	if err := h.manager.Add(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "摄像头已添加",
	})
}

// Remove 移除摄像头
// DELETE /api/cameras/:id
func (h *CameraHandler) Remove(c *gin.Context) {
	cameraID := c.Param("id")

	if err := h.manager.Remove(cameraID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "摄像头已移除",
	})
}

// GetStatus 获取所有摄像头状态
// GET /api/cameras/status
func (h *CameraHandler) GetStatus(c *gin.Context) {
	status := h.manager.GetStatus()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// CaptureFrame 捕获摄像头帧
// POST /api/cameras/:id/capture
func (h *CameraHandler) CaptureFrame(c *gin.Context) {
	cameraID := c.Param("id")

	frame, err := h.manager.CaptureFrame(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"camera_id": frame.CameraID,
			"timestamp": frame.Timestamp,
			"index":     frame.Index,
			"width":     frame.Width,
			"height":    frame.Height,
			"size":      frame.Size(),
		},
	})
}

// Snapshot 获取快照（规格书 4.1）
// POST /api/cameras/:id/snapshot
func (h *CameraHandler) Snapshot(c *gin.Context) {
	cameraID := c.Param("id")

	frame, err := h.manager.CaptureFrame(cameraID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// 如果有图像数据，返回 base64
	if len(frame.Data) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"camera_id":    frame.CameraID,
				"timestamp":    frame.Timestamp,
				"width":        frame.Width,
				"height":       frame.Height,
				"format":       frame.Format,
				"image_base64": frame.Data, // 实际应该 base64 编码
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"camera_id": frame.CameraID,
			"timestamp": frame.Timestamp,
			"width":     frame.Width,
			"height":    frame.Height,
		},
	})
}

// Update 更新摄像头配置（规格书 4.1）
// PUT /api/cameras/:id
func (h *CameraHandler) Update(c *gin.Context) {
	cameraID := c.Param("id")

	var req struct {
		Name    string `json:"name"`
		Source  string `json:"source"`
		Enabled *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	cam, ok := h.manager.Get(cameraID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "摄像头不存在",
		})
		return
	}

	// 更新配置
	if req.Name != "" {
		cam.Config.Name = req.Name
	}
	if req.Source != "" {
		cam.Config.Source = req.Source
	}
	if req.Enabled != nil {
		cam.Config.Enabled = *req.Enabled
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "摄像头配置已更新",
	})
}

// GetStream 获取视频流 URL（规格书 4.1）
// GET /api/cameras/:id/stream
func (h *CameraHandler) GetStream(c *gin.Context) {
	cameraID := c.Param("id")

	cam, ok := h.manager.Get(cameraID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "摄像头不存在",
		})
		return
	}

	// 返回 go2rtc 流 URL
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"camera_id": cameraID,
			"name":      cam.Config.Name,
			"streams": gin.H{
				"mp4":   "/api/go2rtc/stream.mp4?src=" + cameraID,
				"webrtc": "/api/go2rtc/webrtc?src=" + cameraID,
				"hls":   "/api/go2rtc/stream.m3u8?src=" + cameraID,
			},
		},
	})
}
