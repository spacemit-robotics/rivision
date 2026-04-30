// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/owl"
)

// OWLHandler OWL 处理器
type OWLHandler struct {
	client *owl.Client
}

// NewOWLHandler 创建 OWL 处理器
func NewOWLHandler(client *owl.Client) *OWLHandler {
	return &OWLHandler{client: client}
}

// GetStatus 获取 OWL 服务状态
// GET /api/owl/status
func (h *OWLHandler) GetStatus(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"enabled":   false,
				"connected": false,
				"message":   "OWL 服务未配置",
			},
		})
		return
	}

	status, err := h.client.GetStatus()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"enabled":   true,
				"connected": false,
				"error":     err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":   true,
			"connected": true,
			"server":    status,
		},
	})
}

// GetDevices 获取设备列表
// GET /api/owl/devices
func (h *OWLHandler) GetDevices(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))

	devices, total, err := h.client.GetDevices(page, pageSize)
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
			"devices": devices,
			"total":   total,
		},
	})
}

// GetDevicesWithChannels 获取设备列表（包含通道）
// GET /api/owl/devices/channels
func (h *OWLHandler) GetDevicesWithChannels(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))

	devices, total, err := h.client.GetDevicesWithChannels(page, pageSize)
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
			"devices": devices,
			"total":   total,
		},
	})
}

// GetDevice 获取单个设备信息
// GET /api/owl/devices/:id
func (h *OWLHandler) GetDevice(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	deviceID := c.Param("id")
	device, err := h.client.GetDevice(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    device,
	})
}

// GetChannels 获取通道列表
// GET /api/owl/channels
func (h *OWLHandler) GetChannels(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	deviceID := c.Query("device_id")

	channels, total, err := h.client.GetChannels(page, pageSize, deviceID)
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
			"channels": channels,
			"total":    total,
		},
	})
}

// RefreshCatalog 刷新设备目录
// POST /api/owl/devices/:id/catalog
func (h *OWLHandler) RefreshCatalog(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	deviceID := c.Param("id")
	if err := h.client.RefreshCatalog(deviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "目录刷新请求已发送",
	})
}

// Play 播放通道
// POST /api/owl/channels/:id/play
func (h *OWLHandler) Play(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	channelID := c.Param("id")
	output, err := h.client.Play(channelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    output,
	})
}

// PTZControl 云台控制
// POST /api/owl/ptz
func (h *OWLHandler) PTZControl(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	var req struct {
		ChannelID string `json:"channel_id" binding:"required"`
		Command   string `json:"command" binding:"required"` // left/right/up/down/zoom_in/zoom_out/stop
		Speed     int    `json:"speed"`                      // 1-8
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	if req.Speed <= 0 {
		req.Speed = 4
	}
	if req.Speed > 8 {
		req.Speed = 8
	}

	if err := h.client.PTZControl(req.ChannelID, req.Command, req.Speed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "云台控制指令已发送",
	})
}

// DiscoverONVIF 发现 ONVIF 设备
// GET /api/owl/onvif/discover
func (h *OWLHandler) DiscoverONVIF(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	devices, err := h.client.DiscoverONVIF()
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
			"devices": devices,
			"count":   len(devices),
		},
	})
}

// AddToRivision 将 OWL 通道添加到 RiVision 监控
// POST /api/owl/add-to-rivision
func (h *OWLHandler) AddToRivision(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	var req struct {
		ChannelID   string `json:"channel_id" binding:"required"`
		Name        string `json:"name"`
		YOLOEnabled bool   `json:"yolo_enabled"`
		VLMEnabled  bool   `json:"vlm_enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	// 获取 RTSP 流地址
	rtspURL, err := h.client.GetRTSPURL(req.ChannelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "获取流地址失败: " + err.Error(),
		})
		return
	}

	// 生成摄像头 ID
	cameraID := "owl_" + req.ChannelID
	if req.Name == "" {
		req.Name = "OWL_" + req.ChannelID
	}

	cameraConfig := gin.H{
		"id":          cameraID,
		"name":        req.Name,
		"source_type": "owl",
		"rtsp_url":    rtspURL,
		"owl": gin.H{
			"channel_id": req.ChannelID,
		},
		"enabled":      true,
		"yolo_enabled": req.YOLOEnabled,
		"vlm_enabled":  req.VLMEnabled,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    cameraConfig,
		"message": "请使用返回的配置调用 /api/cameras 添加摄像头",
	})
}

// EnableAI 启用通道 AI 检测
// POST /api/owl/channels/:id/ai/enable
func (h *OWLHandler) EnableAI(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	channelID := c.Param("id")
	if err := h.client.EnableAI(channelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "AI 检测已启用",
	})
}

// DisableAI 禁用通道 AI 检测
// POST /api/owl/channels/:id/ai/disable
func (h *OWLHandler) DisableAI(c *gin.Context) {
	if h.client == nil || !h.client.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "OWL 服务未配置",
		})
		return
	}

	channelID := c.Param("id")
	if err := h.client.DisableAI(channelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "AI 检测已禁用",
	})
}
