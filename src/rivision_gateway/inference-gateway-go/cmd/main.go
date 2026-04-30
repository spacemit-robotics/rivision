// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package main 推理网关入口
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/inference-gateway/internal/api"
	"github.com/rivision/inference-gateway/internal/auth"
	"github.com/rivision/inference-gateway/internal/client"
	"github.com/rivision/inference-gateway/internal/config"
	"github.com/rivision/inference-gateway/internal/nodes"
	"github.com/rivision/inference-gateway/internal/services"
)

func main() {
	// 加载配置
	cfg := config.LoadFromEnv()

	// 设置 Gin 模式
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 初始化认证管理器
	authManager := auth.GetNodeAuthManager()
	if authManager != nil {
		log.Printf("[Gateway] 认证管理器已初始化")
	}

	// 创建智能客户端
	smartClient := client.NewSmartClient(cfg)
	if err := smartClient.Start(); err != nil {
		log.Fatalf("启动智能客户端失败: %v", err)
	}

	// ★ 设置节点认证成功回调：立即将节点注册到 Registry
	// 解决问题：节点连接认证成功后，node list 命令无法立即看到节点
	registry := smartClient.GetRegistry()
	authManager.SetOnNodeRegistered(func(regNode *auth.RegisteredNode) {
		url := fmt.Sprintf("http://%s:%d", regNode.Host, regNode.Port)
		node := nodes.NewNode(regNode.NodeID, regNode.NodeID, url, "")
		node.Host = regNode.Host
		node.Port = regNode.Port
		node.AgentPort = regNode.AgentPort
		node.Weight = regNode.Weight
		node.Tags = regNode.Tags
		node.Enabled = true
		node.SetStatus(nodes.NodeStatusHealthy)
		node.UpdateHeartbeat()
		registry.Register(node)
		log.Printf("[Gateway] 节点 %s 已同步到 Registry (Host: %s, Port: %d)", regNode.NodeID, regNode.Host, regNode.Port)
	})

	// 启动节点发现服务
	discovery := services.NewNodeDiscoveryService(smartClient.GetRegistry(), authManager)
	if ranges := os.Getenv("DISCOVERY_RANGES"); ranges != "" {
		// 格式: "192.168.1.0/24,10.0.0.0/24"
		var rangeList []string
		for _, r := range splitAndTrim(ranges) {
			rangeList = append(rangeList, r)
		}
		discovery.SetKnownRanges(rangeList)
	}

	// ★ 修复：先创建并恢复缓存，再启动 Discovery
	// 避免 PartitionHandler.RestoreFromCache() 和 Discovery.RecoverFromAuthManager() 竞争
	partitionHandler := services.NewNetworkPartitionHandler(
		smartClient.GetRegistry(), discovery, cfg.DataDir,
	)
	partitionHandler.RestoreFromCache() // 同步恢复，先完成
	partitionHandler.Start()            // 再启动周期检查

	discovery.Start() // 最后启动 Discovery（此时节点已在 registry，不会重复恢复）

	// 创建 Gin 引擎
	engine := gin.New()
	engine.Use(gin.Recovery())

	// 日志中间件
	if cfg.Debug {
		engine.Use(gin.Logger())
	}

	// CORS 中间件
	engine.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// 设置路由
	api.SetupRoutes(engine, cfg, smartClient)

	// 创建 HTTP 服务器
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	// 启动服务器
	go func() {
		log.Printf("[Gateway] 启动 %s v%s", cfg.AppName, cfg.AppVersion)
		log.Printf("[Gateway] 监听地址: %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务器失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Gateway] 正在关闭...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 关闭服务（反序）
	partitionHandler.Stop()
	discovery.Stop()

	stats := smartClient.GracefulShutdown(30 * time.Second)
	log.Printf("[Gateway] 优雅关闭统计: %v", stats)
	smartClient.Stop()

	// 关闭 HTTP 服务器
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[Gateway] 关闭服务器失败: %v", err)
	}

	log.Println("[Gateway] 已关闭")
}

// splitAndTrim 按逗号分割并去除空白
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
