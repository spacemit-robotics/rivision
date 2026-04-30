// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
)

// handleProcessStatus 获取 llama.cpp 进程状态
// GET /api/v1/process/status
func handleProcessStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	sm := GetSystemMonitor()

	// 获取进程信息
	processInfo := pm.GetProcessInfo()
	allProcesses := pm.GetAllProcesses()

	// 获取系统信息
	systemStats := sm.GetStats()
	gpuStats := sm.GetGPUStats()

	var processResp interface{}
	if processInfo != nil {
		processResp = processInfo
	}

	var systemResp interface{}
	if systemStats != nil {
		systemResp = systemStats.ToMap()
	}

	jsonResponse(w, 200, map[string]interface{}{
		"running":       processInfo != nil,
		"process":       processResp,
		"all_processes": allProcesses,
		"process_count": len(allProcesses),
		"system":        systemResp,
		"gpu":           gpuStats,
	})
}

// handleProcessList 列出所有 llama.cpp 进程
// GET /api/v1/process/list
func handleProcessList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	processes := pm.GetAllProcesses()

	if processes == nil {
		processes = []*ProcessInfo{}
	}

	jsonResponse(w, 200, processes)
}

// handleProcessStart 启动 llama.cpp 服务
// POST /api/v1/process/start
func handleProcessStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	result := pm.Start()

	if !result.Success {
		jsonError(w, 500, result.Message)
		return
	}

	jsonResponse(w, 200, result)
}

// handleProcessStop 停止 llama.cpp 服务
// POST /api/v1/process/stop
func handleProcessStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	result := pm.Stop()

	if !result.Success {
		jsonError(w, 500, result.Message)
		return
	}

	jsonResponse(w, 200, result)
}

// handleProcessRestart 重启 llama.cpp 服务
// POST /api/v1/process/restart
func handleProcessRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pm := GetProcessManager()
	result := pm.Restart()

	if !result.Success {
		jsonError(w, 500, result.Message)
		return
	}

	jsonResponse(w, 200, result)
}

// handleProcessKill 终止 llama.cpp 进程
// POST /api/v1/process/kill
func handleProcessKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PID   int  `json:"pid"`
		Force bool `json:"force"`
	}

	// 允许无 body 请求
	readJSON(r, &req)

	pm := GetProcessManager()
	result := pm.Kill(req.PID, req.Force)

	if !result.Success {
		jsonError(w, 500, result.Message)
		return
	}

	jsonResponse(w, 200, result)
}

// handleProcessKillAll 终止所有 llama.cpp 进程
// POST /api/v1/process/kill-all
func handleProcessKillAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// 从 query 参数获取 force
	force := r.URL.Query().Get("force") == "true"

	pm := GetProcessManager()
	result := pm.Kill(0, force)

	if !result.Success {
		jsonError(w, 500, result.Message)
		return
	}

	jsonResponse(w, 200, result)
}
