// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// 全局配置
var settings *Settings

func main() {
	// 设置时区
	os.Setenv("TZ", "Asia/Shanghai")

	// 加载配置
	settings = NewSettings()

	// 初始化日志系统（支持日志轮转）
	if err := InitLogger(settings); err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer CloseLogger()

	log.Printf("启动 %s v%s", settings.AppName, settings.AppVersion)
	log.Printf("节点ID: %s", settings.NodeID)
	log.Printf("监听地址: %s:%d", settings.AgentHost, settings.AgentPort)
	// ★ 显示服务启用状态（便于调试）
	log.Printf("服务配置: LLAMA_ENABLED=%v, YOLO_ENABLED=%v", settings.LlamaEnabled, settings.YoloEnabled)

	// 创建 HTTP 路由
	mux := http.NewServeMux()
	setupRoutes(mux)

	// 创建 HTTP 服务器
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", settings.AgentHost, settings.AgentPort),
		Handler:      corsMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 启动 Gateway 客户端
	var gatewayClient *GatewayClient
	if settings.GatewayURL != "" {
		log.Printf("正在初始化 Gateway 客户端...")
		log.Printf("GATEWAY_URL: %s", settings.GatewayURL)
		log.Printf("NODE_HOST 配置: %s", settings.NodeHost)

		gatewayClient = NewGatewayClient(settings)
		go gatewayClient.Start()
		log.Printf("Gateway 客户端已启动: %s", settings.GatewayURL)
	} else {
		log.Printf("GATEWAY_URL 未配置，跳过 Gateway 客户端启动")
	}

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动 HTTP 服务器
	go func() {
		log.Printf("HTTP 服务器启动: %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务器错误: %v", err)
		}
	}()

	// 等待信号
	sig := <-sigCh
	log.Printf("收到信号 %v，准备退出...", sig)

	// 优雅关闭
	log.Printf("关闭 Node Agent...")

	// 停止 Gateway 客户端（注销节点）
	if gatewayClient != nil {
		gatewayClient.Stop()
	}

	// 关闭 HTTP 服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP 服务器关闭错误: %v", err)
	}

	log.Printf("Node Agent 已停止")
}
