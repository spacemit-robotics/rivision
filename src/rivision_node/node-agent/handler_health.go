package main

import (
	"fmt"
	"net/http"
	"time"
)

// handleHealthCheck 节点代理健康检查
// GET /api/v1/health
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	sm := GetSystemMonitor()

	// ★ 根据配置决定是否检测 llama
	llamaRunning := false
	llamaHealthy := false
	llamaBusy := false

	if settings.LlamaEnabled {
		// 检查 llama.cpp 进程是否存在
		llamaRunning = pm.IsRunning()

		// 检查 llama.cpp 服务是否响应（带重试）
		if llamaRunning {
			// 重试配置：2 次尝试，每次 2 秒超时，间隔 0.5 秒
			maxRetries := 2
			retryDelay := 500 * time.Millisecond
			timeoutPerTry := 2 * time.Second

			for attempt := 0; attempt < maxRetries; attempt++ {
				client := &http.Client{Timeout: timeoutPerTry}
				resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", settings.LlamaPort))
				if err != nil {
					if isTimeout(err) {
						llamaBusy = true
						LogDebug("llama /health 超时 (尝试 %d)", attempt+1)
					} else {
						LogDebug("llama /health 异常: %v (尝试 %d)", err, attempt+1)
					}
				} else {
					resp.Body.Close()
					if resp.StatusCode == 200 {
						llamaHealthy = true
						break
					}
					LogDebug("llama /health 返回 %d (尝试 %d)", resp.StatusCode, attempt+1)
				}

				// 非最后一次尝试，等待后重试
				if attempt < maxRetries-1 {
					time.Sleep(retryDelay)
				}
			}
		}
	}

	// 获取系统状态
	systemStats := sm.GetStats()

	// ★ 检测 YOLO 服务状态
	yoloRunning := false
	yoloHealthy := false
	if settings.YoloEnabled {
		yoloPM := GetYOLOProcessManager()
		yoloRunning = yoloPM.IsRunning()
		if yoloRunning {
			yoloSvc := GetYOLOService()
			if yoloSvc != nil {
				yoloHealthy = yoloSvc.HealthCheck()
			}
		}
	}

	// ★ 状态判断：考虑所有启用的服务
	status := "healthy"  // 默认健康（如果没有启用任何服务）
	
	// 检查 llama 状态
	if settings.LlamaEnabled {
		if !llamaRunning {
			status = "unhealthy"
		} else if !llamaHealthy {
			status = "degraded"
		}
	}
	
	// 检查 yolo 状态（如果 llama 已经是 unhealthy，保持）
	if settings.YoloEnabled && status != "unhealthy" {
		if !yoloRunning {
			status = "unhealthy"
		} else if !yoloHealthy && status == "healthy" {
			status = "degraded"
		}
	}

	var systemInfo map[string]interface{}
	if systemStats != nil {
		systemInfo = map[string]interface{}{
			"cpu_percent":    systemStats.CPUPercent,
			"memory_percent": systemStats.MemoryPercent,
			"load_avg_1m":    systemStats.LoadAvg1m,
		}
	} else {
		systemInfo = map[string]interface{}{
			"cpu_percent":    nil,
			"memory_percent": nil,
			"load_avg_1m":    nil,
		}
	}

	jsonResponse(w, 200, map[string]interface{}{
		"status":        status,
		"node_id":       settings.NodeID,
		"agent_version": settings.AppVersion,
		"llama": map[string]interface{}{
			"enabled": settings.LlamaEnabled,
			"running": llamaRunning,
			"healthy": llamaHealthy,
			"busy":    llamaBusy,
			"port":    settings.LlamaPort,
		},
		"yolo": map[string]interface{}{
			"enabled": settings.YoloEnabled,
			"running": yoloRunning,
			"healthy": yoloHealthy,
			"port":    settings.YoloPort,
		},
		"system": systemInfo,
	})
}

// handleReadinessCheck 就绪检查
// GET /api/v1/ready
func handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// ★ 根据启用的服务决定就绪状态
	// 所有启用的服务都必须运行才算就绪
	ready := true
	
	if settings.LlamaEnabled {
		pm := GetProcessManager()
		if !pm.IsRunning() {
			ready = false
		}
	}
	
	if settings.YoloEnabled && ready {
		yoloPM := GetYOLOProcessManager()
		if !yoloPM.IsRunning() {
			ready = false
		}
	}

	jsonResponse(w, 200, map[string]interface{}{
		"ready":   ready,
		"node_id": settings.NodeID,
	})
}

// handleLivenessCheck 存活检查
// GET /api/v1/live
func handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	jsonResponse(w, 200, map[string]interface{}{
		"alive":   true,
		"node_id": settings.NodeID,
	})
}

// handleNodeStatus 获取节点状态（供Gateway主动发现调用）
// GET /api/v1/status
func handleNodeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	sm := GetSystemMonitor()

	// ★ 根据配置检查进程状态
	llamaRunning := false
	var processInfo *ProcessInfo
	if settings.LlamaEnabled {
		llamaRunning = pm.IsRunning()
		processInfo = pm.GetProcessInfo()
	}

	// 获取系统状态
	systemStats := sm.GetStats()
	acceleratorStats := sm.GetAcceleratorStats()

	llamaStatus := map[string]interface{}{
		"enabled":     settings.LlamaEnabled,
		"running":     llamaRunning,
		"pid":         nil,
		"cpu_percent": float64(0),
		"memory_mb":   float64(0),
	}
	if processInfo != nil {
		llamaStatus["pid"] = processInfo.PID
		llamaStatus["cpu_percent"] = processInfo.CPUPercent
		llamaStatus["memory_mb"] = processInfo.MemoryMB
	}

	// ★ 添加 YOLO 状态
	yoloStatus := map[string]interface{}{
		"enabled": settings.YoloEnabled,
		"running": false,
	}
	if settings.YoloEnabled {
		yoloPM := GetYOLOProcessManager()
		yoloProc := yoloPM.GetProcessInfo()
		yoloStatus["running"] = yoloProc != nil
		if yoloProc != nil {
			yoloStatus["pid"] = yoloProc.PID
			yoloStatus["cpu_percent"] = yoloProc.CPUPercent
			yoloStatus["memory_mb"] = yoloProc.MemoryMB
		}
	}

	systemStatsResp := map[string]interface{}{
		"cpu_percent":         float64(0),
		"memory_percent":      float64(0),
		"memory_available_gb": float64(0),
		"load_avg_1m":         float64(0),
	}
	if systemStats != nil {
		systemStatsResp["cpu_percent"] = systemStats.CPUPercent
		systemStatsResp["memory_percent"] = systemStats.MemoryPercent
		systemStatsResp["memory_available_gb"] = systemStats.MemoryAvailGB
		systemStatsResp["load_avg_1m"] = systemStats.LoadAvg1m
	}

	jsonResponse(w, 200, map[string]interface{}{
		"node_id":           settings.NodeID,
		"host":              "auto-detected",
		"port":              settings.LlamaPort,
		"agent_port":        settings.AgentPort,
		"weight":            settings.NodeWeight,
		"tags":              settings.NodeTags,
		"llama_status":      llamaStatus,
		"yolo_status":       yoloStatus,
		"system_stats":      systemStatsResp,
		"accelerator_stats": acceleratorStats,
		"version":           settings.AppVersion,
		"status":            "online",
	})
}
