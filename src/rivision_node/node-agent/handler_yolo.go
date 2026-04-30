// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
)

// handleYOLODetect 执行 YOLO 目标检测
// POST /api/v1/yolo/detect
func handleYOLODetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !settings.YoloEnabled {
		jsonError(w, 503, "YOLO 服务未启用")
		return
	}

	var req struct {
		Image string `json:"image"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, 400, "无效的请求格式")
		return
	}

	yolo := GetYOLOService()

	// 健康检查
	if !yolo.IsHealthy() {
		healthy := yolo.HealthCheck()
		if !healthy {
			jsonError(w, 503, "YOLO 服务不可用")
			return
		}
	}

	// 执行检测
	result := yolo.Detect(req.Image)

	success, _ := result["success"].(bool)
	if !success {
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			errMsg = "未知错误"
		}
		jsonResponse(w, 200, map[string]interface{}{
			"success": false,
			"error":   errMsg,
		})
		return
	}

	// 转换检测结果
	detections := []map[string]interface{}{}
	if dets, ok := result["detections"].([]interface{}); ok {
		for _, d := range dets {
			if det, ok := d.(map[string]interface{}); ok {
				detections = append(detections, map[string]interface{}{
					"bbox":       det["bbox"],
					"class_id":   det["class_id"],
					"class_name": det["class_name"],
					"confidence": det["confidence"],
				})
			}
		}
	}

	jsonResponse(w, 200, map[string]interface{}{
		"success":      true,
		"detections":   detections,
		"inference_ms": result["inference_ms"],
	})
}

// handleYOLOHealth 获取 YOLO 服务健康状态
// GET /api/v1/yolo/health
func handleYOLOHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	healthy := false
	yolo := GetYOLOService()
	if yolo != nil {
		healthy = yolo.HealthCheck()
	}

	jsonResponse(w, 200, map[string]interface{}{
		"healthy": healthy,
		"enabled": settings.YoloEnabled,
		"host":    settings.YoloHost,
		"port":    settings.YoloPort,
	})
}

// handleYOLOStatus 获取 YOLO 服务详细状态
// GET /api/v1/yolo/status
func handleYOLOStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !settings.YoloEnabled {
		jsonResponse(w, 200, map[string]interface{}{
			"enabled": false,
			"message": "YOLO 服务未启用",
		})
		return
	}

	yolo := GetYOLOService()
	yoloStatus := yolo.GetStatus()

	jsonResponse(w, 200, map[string]interface{}{
		"enabled": true,
		"healthy": yolo.IsHealthy(),
		"config": map[string]interface{}{
			"host":       settings.YoloHost,
			"port":       settings.YoloPort,
			"model_path": settings.YoloModelPath,
		},
		"status": yoloStatus,
	})
}
