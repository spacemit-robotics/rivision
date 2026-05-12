// Package api 提供路由配置
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/gateway/client"
	"github.com/rivision/rivision-hub/internal/gateway/config"
)

// SetupRoutes 设置路由
func SetupRoutes(engine *gin.Engine, cfg *config.Config, c *client.SmartClient) {
	handlers := NewHandlers(cfg, c)

	// 根路径
	engine.GET("/", handlers.Root)

	// 健康检查
	engine.GET("/health", handlers.Health)

	// API v1
	v1 := engine.Group("/api/v1")
	{
		// 推理 API
		inference := v1.Group("/inference")
		{
			inference.POST("/analyze", handlers.AnalyzeImage)
			inference.POST("/chat", handlers.Chat)
			inference.GET("/history", handlers.GetInferenceHistory)
		}

		// 节点 API
		nodes := v1.Group("/nodes")
		{
			nodes.GET("", handlers.GetNodes)
			nodes.GET("/snapshot", handlers.ListNodesSnapshot)
			// 能力管理 API (需放在 /:id 之前，避免路由冲突)
			nodes.GET("/capabilities", handlers.GetCapabilities)
			nodes.GET("/by-capability/:capability", handlers.GetNodesByCapability)
			nodes.POST("/select", handlers.SelectNodeForTask)
			// 单节点操作
			nodes.GET("/:id", handlers.GetNode)
			nodes.GET("/:id/capabilities", handlers.GetNodeCapabilities)
			nodes.POST("/register", handlers.RegisterNode)
			nodes.POST("/:id/heartbeat", handlers.NodeHeartbeat)
			nodes.PUT("/:id/heartbeat", handlers.NodeHeartbeat) // 兼容 Python node-agent
			nodes.DELETE("/:id", handlers.DeleteNode)
			nodes.POST("/:id/enable", handlers.EnableNode)
			nodes.POST("/:id/disable", handlers.DisableNode)
			nodes.POST("/:id/check", handlers.CheckNode)
			// 远程进程控制
			nodes.POST("/:id/restart", handlers.RestartNodeLlama)
			nodes.POST("/:id/stop", handlers.StopNodeLlama)
			nodes.POST("/:id/start", handlers.StartNodeLlama)
			nodes.POST("/:id/kill", handlers.KillNodeLlama)
			nodes.GET("/:id/process-status", handlers.GetNodeProcessStatus)
		}

		// 健康 API
		health := v1.Group("/health")
		{
			health.GET("", handlers.Health)
			health.GET("/detailed", handlers.DetailedHealth)
			health.POST("/check", handlers.TriggerHealthCheck)
		}

		// 统计 API
		stats := v1.Group("/stats")
		{
			stats.GET("", handlers.GetStats)
			stats.GET("/overview", handlers.GetOverview)
			stats.GET("/nodes", handlers.GetNodeStats)
			stats.GET("/performance", handlers.GetPerformance)
			stats.GET("/tasks", handlers.GetTasks)
			stats.GET("/tasks/running", handlers.GetRunningTasks)
			stats.GET("/tasks/pending", handlers.GetPendingTasks) // 前端轮询
		}

		// 监控指标 API
		metrics := v1.Group("/metrics")
		{
			metrics.GET("/realtime", handlers.GetRealtimeMetrics)
			metrics.GET("/timeseries", handlers.GetMetricsTimeSeries)
			metrics.GET("/prometheus", handlers.GetPrometheusMetrics)
		}

		// Token 管理 API
		admin := v1.Group("/admin/node-tokens")
		{
			admin.POST("/generate", handlers.GenerateToken)
			admin.GET("/list", handlers.ListTokens)
			admin.DELETE("/revoke-by-node/:nodeID", handlers.RevokeTokenByNodeID)
			admin.DELETE("/:token", handlers.RevokeToken)
			admin.GET("/stats", handlers.GetRegistrationStats)
			admin.POST("/blacklist", handlers.ManageBlacklist)
			admin.GET("/blacklist", handlers.GetBlacklist)
			admin.POST("/cleanup", handlers.CleanupTokens)
		}

		// WebSocket API
		ws := v1.Group("/ws")
		{
			ws.GET("/realtime", handlers.HandleWebSocket)
			ws.GET("/live", handlers.HandleLiveWebSocket) // VLM 结果推送
			ws.GET("/status", handlers.GetWSStatus)
		}

		// YOLO 分布式推理 API
		yolo := v1.Group("/yolo")
		{
			yolo.POST("/detect", handlers.YOLODetect)
			yolo.GET("/status", handlers.YOLOStatus)
			yolo.GET("/health", handlers.YOLOHealth)
		}

		// 异步任务 API
		asyncTasks := v1.Group("/async-tasks")
		{
			asyncTasks.POST("/submit", handlers.SubmitAsyncTask)
			asyncTasks.GET("/status/:task_id", handlers.GetAsyncTaskStatus)
			asyncTasks.GET("/result/:task_id", handlers.GetAsyncTaskResult)
			asyncTasks.POST("/ack/:task_id", handlers.AcknowledgeAsyncTask)
			asyncTasks.POST("/batch/submit", handlers.BatchSubmitAsyncTasks)
			asyncTasks.POST("/batch/status", handlers.BatchGetAsyncTaskStatus)
			asyncTasks.GET("/health", handlers.GetAsyncTasksHealth)
			asyncTasks.GET("/main/:main_task_id/progress", handlers.GetMainTaskProgress)
		}

		// 配置 API
		v1.GET("/config", handlers.GetConfig)
	}
}
