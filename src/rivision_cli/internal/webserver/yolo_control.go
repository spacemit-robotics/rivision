// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package webserver

import (
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// YOLOControl 管理YOLO检测的摄像头启用状态
type YOLOControl struct {
	mu              sync.RWMutex
	enabledCameras  map[string]bool
	syncFrameRate   int  // 同步模式帧率限制
	jpegQuality     int  // JPEG质量
	vlmIntervalSec  int  // VLM 分析间隔（秒）
	// ★ 后端抓帧开关（默认关闭，前端 DirectDetector 替代）
	backendCaptureEnabled int32 // 0=禁用(default), 1=启用
	// ★ 分布式 Pipeline 统计（atomic，由 webui.go 管理循环更新）
	ActiveWorkers  int32  // 当前活跃 worker 数
	DesiredWorkers int32  // 期望 worker 数
	HealthyNodes   int32  // 健康节点数
	TotalFrames    int64  // 总处理帧数
	TotalErrors    int64  // 总错误数
}

// 全局YOLO控制器实例
var globalYOLOControl = &YOLOControl{
	enabledCameras: make(map[string]bool),
	syncFrameRate:  30,  // ★ 默认30fps（用户可在前端调整）
	jpegQuality:    95,  // 默认95%质量
	vlmIntervalSec: 12,  // 默认12s（K3推理4-10s）
}

// GetYOLOControl 返回全局YOLO控制器
func GetYOLOControl() *YOLOControl {
	return globalYOLOControl
}

// Enable 启用指定摄像头的YOLO检测
func (y *YOLOControl) Enable(cameraID string) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.enabledCameras[cameraID] = true
}

// Disable 禁用指定摄像头的YOLO检测
func (y *YOLOControl) Disable(cameraID string) {
	y.mu.Lock()
	defer y.mu.Unlock()
	delete(y.enabledCameras, cameraID)
}

// IsEnabled 检查指定摄像头是否启用YOLO
func (y *YOLOControl) IsEnabled(cameraID string) bool {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return y.enabledCameras[cameraID]
}

// GetEnabledCameras 返回所有启用YOLO的摄像头列表
func (y *YOLOControl) GetEnabledCameras() []string {
	y.mu.RLock()
	defer y.mu.RUnlock()
	cameras := make([]string, 0, len(y.enabledCameras))
	for id := range y.enabledCameras {
		cameras = append(cameras, id)
	}
	return cameras
}

// DisableAll 禁用所有摄像头的YOLO检测
func (y *YOLOControl) DisableAll() int {
	y.mu.Lock()
	defer y.mu.Unlock()
	count := len(y.enabledCameras)
	y.enabledCameras = make(map[string]bool)
	return count
}

// SetSyncFrameRate 设置同步模式帧率 (1-60fps)
// M<N(节点多于摄像头)时可设置更高帧率以充分利用算力
func (y *YOLOControl) SetSyncFrameRate(fps int) {
	y.mu.Lock()
	defer y.mu.Unlock()
	if fps > 0 && fps <= 60 {
		y.syncFrameRate = fps
	}
}

// GetSyncFrameRate 获取同步模式帧率
func (y *YOLOControl) GetSyncFrameRate() int {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return y.syncFrameRate
}

// SetJPEGQuality 设置JPEG质量
func (y *YOLOControl) SetJPEGQuality(quality int) {
	y.mu.Lock()
	defer y.mu.Unlock()
	if quality >= 50 && quality <= 100 {
		y.jpegQuality = quality
	}
}

// GetJPEGQuality 获取JPEG质量
func (y *YOLOControl) GetJPEGQuality() int {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return y.jpegQuality
}

// SetVLMInterval 设置 VLM 分析间隔（秒, 范围 5-300）
func (y *YOLOControl) SetVLMInterval(sec int) {
	y.mu.Lock()
	defer y.mu.Unlock()
	if sec >= 5 && sec <= 300 {
		y.vlmIntervalSec = sec
	}
}

// GetVLMInterval 获取 VLM 分析间隔（秒）
func (y *YOLOControl) GetVLMInterval() int {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return y.vlmIntervalSec
}

// IsBackendCaptureEnabled 检查后端抓帧是否启用
// 默认禁用：前端 DirectDetector 从 <video> Canvas 抓帧，无需后端 go2rtc frame.jpeg
func (y *YOLOControl) IsBackendCaptureEnabled() bool {
	return atomic.LoadInt32(&y.backendCaptureEnabled) == 1
}

// SetBackendCaptureEnabled 设置后端抓帧开关
func (y *YOLOControl) SetBackendCaptureEnabled(enabled bool) {
	if enabled {
		atomic.StoreInt32(&y.backendCaptureEnabled, 1)
	} else {
		atomic.StoreInt32(&y.backendCaptureEnabled, 0)
	}
}

// GetPipelineStats 获取 Pipeline 统计（线程安全）
func (y *YOLOControl) GetPipelineStats() map[string]interface{} {
	y.mu.RLock()
	defer y.mu.RUnlock()
	return map[string]interface{}{
		"active_workers":  atomic.LoadInt32(&y.ActiveWorkers),
		"desired_workers": atomic.LoadInt32(&y.DesiredWorkers),
		"healthy_nodes":   atomic.LoadInt32(&y.HealthyNodes),
		"total_frames":    atomic.LoadInt64(&y.TotalFrames),
		"total_errors":    atomic.LoadInt64(&y.TotalErrors),
		"sync_frame_rate": y.syncFrameRate,
		"vlm_interval_sec": y.vlmIntervalSec,
		"enabled_cameras": len(y.enabledCameras),
	}
}

// RegisterYOLOControlRoutes 注册YOLO控制API路由
func RegisterYOLOControlRoutes(engine *gin.Engine) {
	yoloAPI := engine.Group("/api/yolo")
	{
		// 获取YOLO状态
		yoloAPI.GET("/status", func(c *gin.Context) {
			ctrl := GetYOLOControl()
			c.JSON(200, gin.H{
				"enabled_cameras": ctrl.GetEnabledCameras(),
				"sync_frame_rate": ctrl.GetSyncFrameRate(),
				"jpeg_quality":    ctrl.GetJPEGQuality(),
			})
		})

		// 启用指定摄像头的YOLO
		yoloAPI.POST("/enable/:camera_id", func(c *gin.Context) {
			cameraID := c.Param("camera_id")
			ctrl := GetYOLOControl()
			ctrl.Enable(cameraID)
			c.JSON(200, gin.H{
				"success":   true,
				"camera_id": cameraID,
				"enabled":   true,
			})
		})

		// 禁用指定摄像头的YOLO
		yoloAPI.POST("/disable/:camera_id", func(c *gin.Context) {
			cameraID := c.Param("camera_id")
			ctrl := GetYOLOControl()
			ctrl.Disable(cameraID)
			c.JSON(200, gin.H{
				"success":   true,
				"camera_id": cameraID,
				"enabled":   false,
			})
		})

		// 检查指定摄像头是否启用
		yoloAPI.GET("/check/:camera_id", func(c *gin.Context) {
			cameraID := c.Param("camera_id")
			ctrl := GetYOLOControl()
			c.JSON(200, gin.H{
				"camera_id": cameraID,
				"enabled":   ctrl.IsEnabled(cameraID),
			})
		})

		// ★ 禁用所有摄像头的YOLO检测（清除状态）
		yoloAPI.POST("/disable-all", func(c *gin.Context) {
			ctrl := GetYOLOControl()
			count := ctrl.DisableAll()
			c.JSON(200, gin.H{
				"success":          true,
				"disabled_count":   count,
				"enabled_cameras":  []string{},
			})
		})

		// ★ YOLO Pipeline 统计（供前端 GatewayPanel 展示）
		yoloAPI.GET("/pipeline-stats", func(c *gin.Context) {
			ctrl := GetYOLOControl()
			c.JSON(200, gin.H{
				"success": true,
				"data":    ctrl.GetPipelineStats(),
			})
		})

		// 设置同步参数
		yoloAPI.POST("/settings", func(c *gin.Context) {
			var req struct {
				SyncFrameRate  int `json:"sync_frame_rate"`
				JPEGQuality    int `json:"jpeg_quality"`
				VLMIntervalSec int `json:"vlm_interval_sec"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(400, gin.H{"error": err.Error()})
				return
			}
			ctrl := GetYOLOControl()
			if req.SyncFrameRate > 0 {
				ctrl.SetSyncFrameRate(req.SyncFrameRate)
			}
			if req.JPEGQuality > 0 {
				ctrl.SetJPEGQuality(req.JPEGQuality)
			}
			if req.VLMIntervalSec > 0 {
				ctrl.SetVLMInterval(req.VLMIntervalSec)
			}
			c.JSON(200, gin.H{
				"success":          true,
				"sync_frame_rate":  ctrl.GetSyncFrameRate(),
				"jpeg_quality":     ctrl.GetJPEGQuality(),
				"vlm_interval_sec": ctrl.GetVLMInterval(),
			})
		})
	}
}
