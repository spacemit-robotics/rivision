// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// YOLOService YOLO 推理服务客户端
type YOLOService struct {
	host    string
	port    int
	baseURL string
	healthy bool
}

// NewYOLOService 创建 YOLO 服务客户端
func NewYOLOService(host string, port int) *YOLOService {
	svc := &YOLOService{
		host:    host,
		port:    port,
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		healthy: false,
	}
	log.Printf("[YOLOService] 初始化: %s", svc.baseURL)
	return svc
}

// HealthCheck 健康检查
func (y *YOLOService) HealthCheck() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(y.baseURL + "/health")
	if err != nil {
		log.Printf("[YOLOService] 健康检查失败: %v", err)
		y.healthy = false
		return false
	}
	defer resp.Body.Close()

	y.healthy = resp.StatusCode == 200
	if y.healthy {
		log.Printf("[YOLOService] 健康检查通过")
	}
	return y.healthy
}

// Detect 执行 YOLO 检测
func (y *YOLOService) Detect(imageBase64 string) map[string]interface{} {
	client := &http.Client{Timeout: 10 * time.Second}

	payload, _ := json.Marshal(map[string]string{"image": imageBase64})
	resp, err := client.Post(y.baseURL+"/api/detect", "application/json", bytes.NewReader(payload))
	if err != nil {
		if isTimeout(err) {
			log.Printf("[YOLOService] 检测超时")
			return map[string]interface{}{"success": false, "error": "检测超时"}
		}
		log.Printf("[YOLOService] 检测异常: %v", err)
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			return result
		}
	}

	log.Printf("[YOLOService] 检测失败: HTTP %d", resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	return map[string]interface{}{
		"success": false,
		"error":   fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
	}
}

// GetStatus 获取 YOLO 服务状态（同时更新健康状态）
func (y *YOLOService) GetStatus() map[string]interface{} {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(y.baseURL + "/status")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)
			var result map[string]interface{}
			if err := json.Unmarshal(body, &result); err == nil {
				// ★ 同步更新健康状态：如果服务响应成功，标记为健康
				y.healthy = true
				// 如果响应中有 healthy 字段，使用它
				if h, ok := result["healthy"].(bool); ok {
					y.healthy = h
				}
				// ★ 修复：确保返回的 map 包含 healthy 字段
				result["healthy"] = y.healthy
				return result
			}
		}
	}

	// ★ 服务无响应或错误，标记为不健康
	y.healthy = false
	return map[string]interface{}{
		"healthy": false,
		"host":    y.host,
		"port":    y.port,
	}
}

// IsHealthy 服务是否健康
func (y *YOLOService) IsHealthy() bool {
	return y.healthy
}

// isTimeout 检查是否为超时错误
func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	// net.Error 接口有 Timeout() 方法
	type timeoutError interface {
		Timeout() bool
	}
	if te, ok := err.(timeoutError); ok {
		return te.Timeout()
	}
	return false
}

// 全局单例
var (
	yoloServiceInstance *YOLOService
	yoloServiceOnce     sync.Once
)

// GetYOLOService 获取 YOLO 服务单例
// ★ 仅在 YOLO_ENABLED=true 时初始化，否则返回 nil
func GetYOLOService() *YOLOService {
	if !settings.YoloEnabled {
		return nil
	}
	yoloServiceOnce.Do(func() {
		yoloServiceInstance = NewYOLOService(settings.YoloHost, settings.YoloPort)
	})
	return yoloServiceInstance
}
