// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package webserver

import (
	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/camera"
	"github.com/rivision/rivision-cli/internal/event"
	"github.com/rivision/rivision-cli/internal/vlm"
	"github.com/rivision/rivision-cli/internal/webserver/handlers"
	"github.com/rivision/rivision-cli/internal/webserver/websocket"
)

// RouteConfig 路由配置
type RouteConfig struct {
	CameraManager *camera.Manager
	VLMTrigger    *vlm.Trigger
	WSHub         *websocket.Hub
	EventBus      *event.EventBus
	Version       string
}

// SetupRoutes 设置路由
func SetupRoutes(engine *gin.Engine, cfg *RouteConfig) {
	// 注意: /health 端点已在 server.go 中注册，这里不重复注册

	// API 路由组
	api := engine.Group("/api")

	// 摄像头 API（规格书 4.1）
	if cfg.CameraManager != nil {
		cameraHandler := handlers.NewCameraHandler(cfg.CameraManager)
		cameras := api.Group("/cameras")
		{
			cameras.GET("", cameraHandler.List)
			cameras.GET("/status", cameraHandler.GetStatus)
			cameras.GET("/:id", cameraHandler.Get)
			cameras.POST("", cameraHandler.Add)
			cameras.PUT("/:id", cameraHandler.Update)
			cameras.DELETE("/:id", cameraHandler.Remove)
			cameras.GET("/:id/stream", cameraHandler.GetStream)
			cameras.POST("/:id/snapshot", cameraHandler.Snapshot)
			cameras.POST("/:id/capture", cameraHandler.CaptureFrame)
		}
	}

	// VLM API（规格书 4.3）
	if cfg.VLMTrigger != nil {
		vlmHandler := handlers.NewVLMHandler(cfg.VLMTrigger, cfg.WSHub)
		vlmAPI := api.Group("/vlm")
		{
			vlmAPI.GET("/config", vlmHandler.GetConfig)
			vlmAPI.PUT("/config", vlmHandler.UpdateConfig)
			vlmAPI.POST("/trigger/:camera_id", vlmHandler.TriggerAnalysis)
			vlmAPI.GET("/results", vlmHandler.GetResults)
			vlmAPI.GET("/results/:id", vlmHandler.GetResult)
			vlmAPI.DELETE("/results/:id", vlmHandler.DeleteResult)
			vlmAPI.GET("/stats", vlmHandler.GetStats)
			vlmAPI.GET("/status", vlmHandler.GetStatus)
		}

		// VLM WebSocket（规格书 4.5）
		engine.GET("/ws/vlm", vlmHandler.HandleWebSocket)
		engine.GET("/ws/vlm/:camera_id", vlmHandler.HandleCameraWebSocket)
	}

	// 系统 API（规格书 4.4）
	if cfg.EventBus != nil {
		systemHandler := handlers.NewSystemHandler(cfg.EventBus, cfg.Version)
		systemAPI := api.Group("/system")
		{
			systemAPI.GET("/status", systemHandler.GetStatus)
			systemAPI.GET("/config", systemHandler.GetConfig)
			systemAPI.PUT("/config", systemHandler.UpdateConfig)
			systemAPI.GET("/logs", systemHandler.GetLogs)
			systemAPI.POST("/restart", systemHandler.Restart)
			systemAPI.GET("/events", systemHandler.GetEvents)
		}

		// 事件 WebSocket（规格书 4.5）
		eventsHandler := handlers.NewEventsHandler(cfg.EventBus)
		engine.GET("/ws/events", eventsHandler.HandleWebSocket)
	}
}
