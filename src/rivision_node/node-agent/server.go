package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// corsMiddleware CORS 中间件
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// jsonResponse 发送 JSON 响应
func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON 编码失败: %v", err)
	}
}

// jsonError 发送错误响应
func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]interface{}{
		"detail": message,
	})
}

// readJSON 读取请求 JSON body
func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// methodRouter 方法路由器 - 根据 HTTP 方法分发请求
type methodRouter struct {
	handlers map[string]http.HandlerFunc
}

func newMethodRouter() *methodRouter {
	return &methodRouter{handlers: make(map[string]http.HandlerFunc)}
}

func (mr *methodRouter) Get(handler http.HandlerFunc) *methodRouter {
	mr.handlers["GET"] = handler
	return mr
}

func (mr *methodRouter) Post(handler http.HandlerFunc) *methodRouter {
	mr.handlers["POST"] = handler
	return mr
}

func (mr *methodRouter) Put(handler http.HandlerFunc) *methodRouter {
	mr.handlers["PUT"] = handler
	return mr
}

func (mr *methodRouter) Delete(handler http.HandlerFunc) *methodRouter {
	mr.handlers["DELETE"] = handler
	return mr
}

func (mr *methodRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler, ok := mr.handlers[r.Method]; ok {
		handler(w, r)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// setupRoutes 注册所有路由
func setupRoutes(mux *http.ServeMux) {
	// 根路由
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		jsonResponse(w, 200, map[string]interface{}{
			"name":    settings.AppName,
			"version": settings.AppVersion,
			"node_id": settings.NodeID,
			"docs":    "/docs",
		})
	})

	// 健康检查 API
	mux.HandleFunc("/api/v1/health", handleHealthCheck)
	mux.HandleFunc("/api/v1/ready", handleReadinessCheck)
	mux.HandleFunc("/api/v1/live", handleLivenessCheck)
	mux.HandleFunc("/api/v1/status", handleNodeStatus)

	// 进程控制 API
	mux.HandleFunc("/api/v1/process/status", handleProcessStatus)
	mux.HandleFunc("/api/v1/process/list", handleProcessList)
	mux.HandleFunc("/api/v1/process/start", handleProcessStart)
	mux.HandleFunc("/api/v1/process/stop", handleProcessStop)
	mux.HandleFunc("/api/v1/process/restart", handleProcessRestart)
	mux.HandleFunc("/api/v1/process/kill", handleProcessKill)
	mux.HandleFunc("/api/v1/process/kill-all", handleProcessKillAll)

	// YOLO API
	mux.HandleFunc("/api/v1/yolo/detect", handleYOLODetect)
	mux.HandleFunc("/api/v1/yolo/health", handleYOLOHealth)
	mux.HandleFunc("/api/v1/yolo/status", handleYOLOStatus)
}

// extractPathParam 从 URL 路径提取参数
// 例如: /api/v1/nodes/{id}/heartbeat
func extractPathParam(path, prefix, suffix string) string {
	path = strings.TrimPrefix(path, prefix)
	if suffix != "" {
		path = strings.TrimSuffix(path, suffix)
	}
	return path
}
