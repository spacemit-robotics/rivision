// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package handlers 提供系统 API 处理器
package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/event"
)

// SystemHandler 系统处理器
type SystemHandler struct {
	eventBus  *event.EventBus
	startTime time.Time
	version   string
}

// NewSystemHandler 创建系统处理器
func NewSystemHandler(eventBus *event.EventBus, version string) *SystemHandler {
	return &SystemHandler{
		eventBus:  eventBus,
		startTime: time.Now(),
		version:   version,
	}
}

// GetStatus 获取系统状态（规格书 4.4）
// GET /api/system/status
func (h *SystemHandler) GetStatus(c *gin.Context) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"version":     h.version,
			"uptime":      time.Since(h.startTime).String(),
			"uptime_secs": int64(time.Since(h.startTime).Seconds()),
			"go_version":  runtime.Version(),
			"os":          runtime.GOOS,
			"arch":        runtime.GOARCH,
			"num_cpu":     runtime.NumCPU(),
			"num_goroutine": runtime.NumGoroutine(),
			"memory": gin.H{
				"alloc_mb":       memStats.Alloc / 1024 / 1024,
				"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
				"sys_mb":         memStats.Sys / 1024 / 1024,
				"num_gc":         memStats.NumGC,
			},
		},
	})
}

// GetConfig 获取系统配置（规格书 4.4）
// GET /api/system/config
func (h *SystemHandler) GetConfig(c *gin.Context) {
	// 返回当前配置（实际实现应该从配置管理器获取）
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"node": gin.H{
				"id":   "master-1",
				"role": "master",
			},
			"server": gin.H{
				"port": 8280,
			},
		},
	})
}

// UpdateConfig 更新系统配置（规格书 4.4）
// PUT /api/system/config
func (h *SystemHandler) UpdateConfig(c *gin.Context) {
	var req map[string]interface{}
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
		"message": "配置已更新",
	})
}

// GetLogs 获取日志（规格书 4.4）
// GET /api/system/logs
func (h *SystemHandler) GetLogs(c *gin.Context) {
	// 实际实现应该从日志文件或缓冲区获取
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    []string{},
	})
}

// Restart 重启服务（规格书 4.4）
// POST /api/system/restart
func (h *SystemHandler) Restart(c *gin.Context) {
	// 实际实现应该触发服务重启
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "服务将在 3 秒后重启",
	})
}

// GetEvents 获取事件列表
// GET /api/system/events
func (h *SystemHandler) GetEvents(c *gin.Context) {
	limit := 50
	eventType := c.Query("type")

	var events []*event.Event
	if eventType != "" {
		events = h.eventBus.GetHistoryByType(event.EventType(eventType), limit)
	} else {
		events = h.eventBus.GetHistory(limit)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    events,
		"total":   len(events),
	})
}
