// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package webserver

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/rivision/rivision-cli/internal/camera"
	"github.com/rivision/rivision-cli/internal/owl"
	"github.com/rivision/rivision-cli/internal/semantic"
	"github.com/rivision/rivision-cli/internal/webserver/handlers"
	"github.com/rivision/rivision-cli/internal/webserver/websocket"
)

// go2rtc WebSocket 代理升级器
var go2rtcWSUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 64 * 1024, // 64KB — fMP4 segment 可能较大
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// OWL /streams 日志节流，避免接口 404 时刷屏
var owlStreamsLogState struct {
	sync.Mutex
	lastAt time.Time
	hit    int
}

func logOwlStreamsFallback(err error) {
	owlStreamsLogState.Lock()
	defer owlStreamsLogState.Unlock()
	now := time.Now()
	owlStreamsLogState.hit++
	if now.Sub(owlStreamsLogState.lastAt) >= 60*time.Second {
		log.Printf("[OWL] /streams get channels failed (x%d in last window), fallback to bridge/go2rtc: %v", owlStreamsLogState.hit, err)
		owlStreamsLogState.lastAt = now
		owlStreamsLogState.hit = 0
	}
}

//go:embed frontend/dist
var frontendFS embed.FS

type Config struct {
	Port       int
	GatewayURL string
	Go2rtcURL  string

	// 新增模块（可选）
	CameraManager interface{} // *camera.Manager
	YOLOTracker   interface{} // *yolo.Tracker
	VLMTrigger    interface{} // *vlm.Trigger
	WSHub         interface{} // *websocket.Hub
	OWLClient     *owl.Client  // OWL GB28181 客户端
	OWLStreamBridge *camera.OWLStreamBridge // OWL→go2rtc 流桥接
	SemanticStore *semantic.Store // VLM 语义搜索（SQLite）
}

type Server struct {
	config Config
	engine *gin.Engine
}

func New(config Config) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	s := &Server{
		config: config,
		engine: engine,
	}

	s.setupRoutes()

	// 注意：OWL 路由不在此直接注册。
	// 原因：本文件已注册 /api/*path 通配路由，gin 不允许与 /api/owl/* 共存，会触发 panic。
	// 统一在 /api/*path 内部分发 OWL 子路由。

	// 设置扩展路由（如果提供了相关模块）
	s.setupExtendedRoutes()

	return s
}

// setupExtendedRoutes 设置扩展路由
func (s *Server) setupExtendedRoutes() {
	// 此方法在 routes.go 中通过 SetupRoutes 函数实现
	// 这里仅作为占位符，实际集成时由外部调用 SetupRoutes
}

// safeProxyServe 安全地调用 ReverseProxy.ServeHTTP
// httputil.ReverseProxy 在客户端断开连接时会 panic(http.ErrAbortHandler)
// 不捕获会导致 gin.Recovery() 记录大量堆栈日志（每次客户端离开页面都触发）
func safeProxyServe(proxy *httputil.ReverseProxy, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			// 只忽略 http.ErrAbortHandler（客户端断开）
			if rec == http.ErrAbortHandler {
				return
			}
			// 其他 panic 重新抛出
			panic(rec)
		}
	}()
	proxy.ServeHTTP(w, r)
}

// headersSent 检查 ResponseWriter 是否已经发送了 header
// 如果已发送，不能再写 HTTP 错误码
func headersSent(w http.ResponseWriter) bool {
	// gin.ResponseWriter 实现了 Written() 方法
	if gw, ok := w.(interface{ Written() bool }); ok {
		return gw.Written()
	}
	return false
}

// isClientDisconnectError 判断是否为客户端主动断开连接的错误
func isClientDisconnectError(err error) bool {
	if err == context.Canceled {
		return true
	}
	if netErr, ok := err.(*net.OpError); ok {
		return netErr.Op == "write" || netErr.Op == "read"
	}
	errStr := err.Error()
	return strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "client disconnected") ||
		strings.Contains(errStr, "request canceled")
}

func (s *Server) setupRoutes() {
	// ★ 自定义 Transport：YOLO 5fps×4cam=20req/s 需要足够的连接池
	// Go 默认 MaxIdleConnsPerHost=2 → 90%请求新建TCP连接 → 延迟增加+连接耗尽
	gatewayTransport := &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,  // ★ 默认2远不够20req/s
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 600 * time.Second, // VLM推理最长600s
		DisableCompression:    true,               // 内网无需压缩
	}

	// API 代理到 Gateway
	gatewayURL, _ := url.Parse(s.config.GatewayURL)
	gatewayProxy := httputil.NewSingleHostReverseProxy(gatewayURL)
	gatewayProxy.Transport = gatewayTransport
	// ★ 错误处理：区分客户端断开(静默) vs Gateway不可达(返回502)
	gatewayProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if isClientDisconnectError(err) {
			return // 客户端断开是正常情况
		}
		log.Printf("[Gateway Proxy] %s %s → %v", r.Method, r.URL.Path, err)
		if !headersSent(w) {
			http.Error(w, "Gateway unavailable", http.StatusBadGateway)
		}
	}

	// go2rtc 代理
	go2rtcURL, _ := url.Parse(s.config.Go2rtcURL)
	go2rtcProxy := httputil.NewSingleHostReverseProxy(go2rtcURL)
	// ★ 视频流 Transport：长连接，无超时（流可能持续数小时）
	go2rtcProxy.Transport = &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       0, // 不超时（视频流长连接）
		ResponseHeaderTimeout: 0, // 不限制响应头超时
		DisableCompression:    true,
	}
	// ★ 关键：立即 flush 每个 upstream 写入
	// 默认 FlushInterval=0 时，代理使用 32KB 内部缓冲，导致直播流数据延迟到达浏览器
	// 产生 "突发数据→播放几秒→缓冲耗尽→卡顿→等待下一批" 的周期性循环
	// -1 表示每次 upstream Write 后立即 Flush，确保帧数据实时到达浏览器
	go2rtcProxy.FlushInterval = -1
	// 上游返回 4xx/5xx 时记录正文片段（便于排查 stream.mp4 500：多为 RTSP 拉流失败 / 缺 ffmpeg）
	go2rtcProxy.ModifyResponse = func(resp *http.Response) error {
		if resp == nil || resp.StatusCode < 400 {
			return nil
		}
		p := resp.Request.URL.Path
		if !strings.Contains(p, "stream.mp4") && !strings.Contains(p, "frame.jpeg") && !strings.Contains(p, "webrtc") {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil
		}
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		log.Printf("[go2rtc upstream] %s %s?%s → HTTP %d body=%q", resp.Request.Method, p, resp.Request.URL.RawQuery, resp.StatusCode, snippet)
		return nil
	}
	// 忽略客户端断开连接的错误（视频流场景更频繁）
	go2rtcProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if isClientDisconnectError(err) {
			return
		}
		log.Printf("[go2rtc Proxy] %s %s → %v", r.Method, r.URL.Path, err)
		if !headersSent(w) {
			http.Error(w, fmt.Sprintf("go2rtc unavailable (%s): %v", s.config.Go2rtcURL, err), http.StatusBadGateway)
		}
	}

	// go2rtc API 代理
	s.engine.Any("/go2rtc/*path", func(c *gin.Context) {
		c.Request.URL.Path = strings.TrimPrefix(c.Request.URL.Path, "/go2rtc")
		safeProxyServe(go2rtcProxy, c.Writer, c.Request)
	})

	// Gateway API 代理（处理 /api/* 请求，内部判断是否转发到 go2rtc 或本地处理）
	s.engine.Any("/api/*path", func(c *gin.Context) {
		path := c.Param("path")
		// Gin *path 在少数版本/场景下可能无前导斜杠，统一归一化避免误走 Gateway（/apigo2rtc/...）
		if path != "" && path[0] != '/' {
			path = "/" + path
		}

		// YOLO 控制 API（本地处理）
		if strings.HasPrefix(path, "/yolo/") || path == "/yolo" {
			s.handleYOLOControl(c, path)
			return
		}
		
		// 本地 API 端点（不依赖 Gateway）
		if c.Request.Method == "GET" {
			switch path {
			case "/status":
				s.handleLocalStatus(c)
				return
			case "/inference/nodes":
				s.handleLocalNodes(c)
				return
			// 注意：/inference/history 需要代理到Gateway获取真实历史记录
			// 不再在本地返回空数据
			}
		}

		// 本地语义搜索 API（SQLite）
		if s.config.SemanticStore != nil {
			semanticHandler := handlers.NewSemanticSearchHandler(s.config.SemanticStore)
			if c.Request.Method == "GET" && path == "/vlm/search/stats" {
				semanticHandler.Stats(c)
				return
			}
			if c.Request.Method == "GET" && path == "/vlm/search/provider/status" {
				semanticHandler.ProviderStatus(c)
				return
			}
			if c.Request.Method == "POST" && path == "/vlm/search" {
				semanticHandler.Search(c)
				return
			}
			if c.Request.Method == "POST" && path == "/vlm/search/image" {
				semanticHandler.SearchByImage(c)
				return
			}
			if c.Request.Method == "POST" && path == "/vlm/search/hybrid" {
				semanticHandler.HybridSearch(c)
				return
			}
							if c.Request.Method == "POST" && path == "/vlm/search/reindex" {
					semanticHandler.ReindexFromHistory(c)
					return
				}
				if c.Request.Method == "POST" && path == "/vlm/search/backfill" {
					semanticHandler.Backfill(c)
					return
				}
			}

			// OWL API（在 /api/*path 内部分发，避免与通配路由冲突）
		if strings.HasPrefix(path, "/owl/") || path == "/owl" {
			s.handleOWLAPI(c, path)
			return
		}
		
		// 前端使用 /api/go2rtc/* 路径访问 go2rtc
		// path 格式: /go2rtc/streams (带前导斜杠)
		if strings.HasPrefix(path, "/go2rtc/") || path == "/go2rtc" {
			// ★ WebSocket 请求走专用 WS 代理（httputil.ReverseProxy 不支持 WebSocket）
			// 前端 MSE 播放器通过 /api/go2rtc/ws?src=xxx 建立 WebSocket 连接
			if path == "/go2rtc/ws" && c.GetHeader("Upgrade") == "websocket" {
				s.handleGo2rtcWebSocket(c)
				return
			}
			// 移除 /go2rtc 前缀，保留后面的路径（如 /streams）
			newPath := strings.TrimPrefix(path, "/go2rtc")
			if newPath == "" {
				newPath = "/"
			}
			c.Request.URL.Path = "/api" + newPath
			safeProxyServe(go2rtcProxy, c.Writer, c.Request)
			return
		}
		c.Request.URL.Path = "/api" + path
		safeProxyServe(gatewayProxy, c.Writer, c.Request)
	})

	// YOLO 演示页面
	s.engine.GET("/yolo-demo", func(c *gin.Context) {
		file, err := frontendFS.Open("frontend/dist/yolo-demo.html")
		if err != nil {
			c.String(http.StatusNotFound, "YOLO demo page not found")
			return
		}
		defer file.Close()
		content, _ := io.ReadAll(file)
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, string(content))
	})

	// 静态文件服务 - SPA 路由
	s.engine.GET("/", s.serveIndex)
	s.engine.GET("/monitor", s.serveIndex)
	s.engine.GET("/dashboard", s.serveIndex)
	s.engine.GET("/nodes", s.serveIndex)
	s.engine.GET("/tasks", s.serveIndex)
	s.engine.GET("/analysis", s.serveIndex)
	s.engine.GET("/settings", s.serveIndex)
	s.engine.GET("/login", s.serveIndex)

	// 静态资源
	s.engine.GET("/assets/*filepath", s.serveStatic)
	s.engine.GET("/static/*filepath", s.serveStatic)
	
	// YOLO WebSocket 端点
	s.engine.GET("/ws/yolo/:camera_id", s.handleYOLOWebSocket)      // 单摄像头（向后兼容）
	s.engine.GET("/ws/yolo", s.handleYOLOWebSocketMux)               // 多路复用（解决浏览器连接数限制）
	
	// Inference WebSocket 端点（用于推理状态通知）
	s.engine.GET("/ws/inference", s.handleInferenceWebSocket)

	// 健康检查
	s.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}

func (s *Server) serveIndex(c *gin.Context) {
	file, err := frontendFS.Open("frontend/dist/index.html")
	if err != nil {
		// 返回默认页面
		c.Header("Content-Type", "text/html")
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.String(http.StatusOK, defaultIndexHTML)
		return
	}
	defer file.Close()

	content, _ := io.ReadAll(file)
	c.Header("Content-Type", "text/html")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.String(http.StatusOK, string(content))
}

func (s *Server) serveStatic(c *gin.Context) {
	path := "frontend/dist" + c.Request.URL.Path
	file, err := frontendFS.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()

	stat, _ := file.Stat()
	if stat.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	// 设置 Content-Type
	contentType := "application/octet-stream"
	if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(path, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".jpeg") {
		contentType = "image/jpeg"
	}

	content, _ := io.ReadAll(file)
	c.Header("Content-Type", contentType)
	c.String(http.StatusOK, string(content))
}

func (s *Server) Run() error {
	return s.engine.Run(fmt.Sprintf(":%d", s.config.Port))
}

// Engine 返回 gin.Engine 实例（用于外部设置路由）
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// handleYOLOWebSocket 处理 YOLO WebSocket 连接（单摄像头，向后兼容）
func (s *Server) handleYOLOWebSocket(c *gin.Context) {
	cameraID := c.Param("camera_id")
	
	if s.config.WSHub != nil {
		if hub, ok := s.config.WSHub.(*websocket.Hub); ok {
			websocket.HandleYOLOWebSocket(c, hub, cameraID)
			return
		}
	}
	
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "WebSocket hub not available",
	})
}

// handleYOLOWebSocketMux 多路复用 YOLO WebSocket（单连接支持多摄像头）
func (s *Server) handleYOLOWebSocketMux(c *gin.Context) {
	if s.config.WSHub != nil {
		if hub, ok := s.config.WSHub.(*websocket.Hub); ok {
			websocket.HandleYOLOWebSocketMux(c, hub)
			return
		}
	}
	
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "WebSocket hub not available",
	})
}

// GetFrontendFS 返回嵌入的前端文件系统（用于外部访问）
func GetFrontendFS() fs.FS {
	subFS, _ := fs.Sub(frontendFS, "frontend/dist")
	return subFS
}

const defaultIndexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>RiVision - AI视频推理分析</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #fff;
        }
        .container {
            text-align: center;
            padding: 2rem;
        }
        h1 {
            font-size: 3rem;
            margin-bottom: 1rem;
            background: linear-gradient(90deg, #00d4ff, #7b2cbf);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        p {
            color: #888;
            margin-bottom: 2rem;
        }
        .status {
            display: inline-flex;
            align-items: center;
            gap: 0.5rem;
            padding: 0.5rem 1rem;
            background: rgba(255,255,255,0.1);
            border-radius: 20px;
            margin-bottom: 2rem;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            background: #00ff88;
            border-radius: 50%;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .info {
            background: rgba(255,255,255,0.05);
            border-radius: 10px;
            padding: 1.5rem;
            text-align: left;
            max-width: 400px;
            margin: 0 auto;
        }
        .info h3 {
            color: #00d4ff;
            margin-bottom: 1rem;
        }
        .info ul {
            list-style: none;
        }
        .info li {
            padding: 0.5rem 0;
            border-bottom: 1px solid rgba(255,255,255,0.1);
        }
        .info li:last-child {
            border-bottom: none;
        }
        code {
            background: rgba(0,212,255,0.2);
            padding: 0.2rem 0.5rem;
            border-radius: 4px;
            font-size: 0.9rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎬 RiVision</h1>
        <p>AI视频推理分析平台</p>
        <div class="status">
            <span class="status-dot"></span>
            <span>服务运行中</span>
        </div>
        <div class="info">
            <h3>📋 快速开始</h3>
            <ul>
                <li>构建前端: <code>cd go2rtc-vue && npm run build</code></li>
                <li>复制到嵌入目录: <code>cp -r dist/* rivision-cli/internal/webserver/frontend/dist/</code></li>
                <li>重新编译: <code>go build</code></li>
            </ul>
        </div>
    </div>
</body>
</html>`

// handleLocalStatus 处理本地状态请求
func (s *Server) handleLocalStatus(c *gin.Context) {
	// 检查 Gateway 状态
	gatewayStatus := "offline"
	if s.config.GatewayURL != "" {
		// 尝试连接 Gateway
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(s.config.GatewayURL + "/api/v1/health")
		if err == nil && resp.StatusCode == 200 {
			gatewayStatus = "healthy"
			resp.Body.Close()
		}
	}

	go2rtcStatus := "offline"
	go2rtcDetail := ""
	if s.config.Go2rtcURL != "" {
		g2c := &http.Client{Timeout: 2 * time.Second}
		resp, err := g2c.Get(s.config.Go2rtcURL + "/api/streams")
		if err != nil {
			go2rtcDetail = err.Error()
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				go2rtcStatus = "healthy"
			} else {
				go2rtcStatus = fmt.Sprintf("http_%d", resp.StatusCode)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"gateway": gin.H{
			"status": gatewayStatus,
			"url":    s.config.GatewayURL,
		},
		"go2rtc": gin.H{
			"status":  go2rtcStatus,
			"url":     s.config.Go2rtcURL,
			"detail":  go2rtcDetail,
			"hint":    "若视频 500：请确认已用 --with-go2rtc --with-rtsp-server 启动，mp4 在 --mp4-dir 下，且系统已安装 ffmpeg（go2rtc 拉 RTSP 依赖）",
		},
	})
}

// handleLocalNodes 处理本地节点列表请求
func (s *Server) handleLocalNodes(c *gin.Context) {
	// 尝试从 Gateway 获取节点列表
	if s.config.GatewayURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(s.config.GatewayURL + "/api/v1/nodes")
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var data map[string]interface{}
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				c.JSON(http.StatusOK, data)
				return
			}
		}
	}

	// Gateway 不可用时返回空节点列表
	c.JSON(http.StatusOK, gin.H{
		"nodes": []interface{}{},
	})
}

// handleLocalHistory 处理本地历史记录请求
func (s *Server) handleLocalHistory(c *gin.Context) {
	// 返回空历史记录（实际可从本地存储获取）
	c.JSON(http.StatusOK, gin.H{
		"history": []interface{}{},
		"total":   0,
	})
}

// handleOWLAPI 在 /api/*path 通配路由内分发 OWL 接口（避免路由冲突 panic）
func (s *Server) handleOWLAPI(c *gin.Context, path string) {
	if s.config.OWLClient == nil || !s.config.OWLClient.IsEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "owl is not enabled"})
		return
	}

	method := c.Request.Method
	client := s.config.OWLClient

	// POST /api/owl/login
	if method == http.MethodPost && path == "/owl/login" {
		if err := client.Login(); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// GET /api/owl/capabilities
	if method == http.MethodGet && path == "/owl/capabilities" {
		caps := gin.H{
			"login":                 false,
			"server_info":           false,
			"channels":              false,
			"devices":               false,
			"devices_with_channels": false,
			"play":                  false,
			"snapshot":              false,
			"ptz":                   false,
		}
		detail := gin.H{}

		if err := client.Login(); err == nil {
			caps["login"] = true
		} else {
			detail["login"] = err.Error()
		}
		if _, err := client.GetServerInfo(); err == nil {
			caps["server_info"] = true
		} else {
			detail["server_info"] = err.Error()
		}
		if _, _, err := client.GetChannels(1, 1, ""); err == nil {
			caps["channels"] = true
		} else {
			detail["channels"] = err.Error()
		}
		if _, _, err := client.GetDevices(1, 1); err == nil {
			caps["devices"] = true
		} else {
			detail["devices"] = err.Error()
		}
		if _, _, err := client.GetDevicesWithChannels(1, 1); err == nil {
			caps["devices_with_channels"] = true
		} else {
			detail["devices_with_channels"] = err.Error()
		}
		// 动态能力：若有至少一个通道则可判定 play/snapshot/ptz 可用
		caps["play"] = caps["channels"] == true || caps["devices_with_channels"] == true
		caps["snapshot"] = caps["play"]
		caps["ptz"] = caps["play"]

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"capabilities": caps,
			"detail":       detail,
		})
		return
	}

	// GET /api/owl/server/info
	if method == http.MethodGet && path == "/owl/server/info" {
		info, err := client.GetServerInfo()
		if err == nil {
			c.JSON(http.StatusOK, info)
			return
		}

		// 某些 OWL 实现不提供 /api/server/info（但登录与播放可用），
		// 这里尝试重登并返回“degraded connected”，避免前端误判为完全未连接。
		if loginErr := client.Login(); loginErr == nil {
			info2, err2 := client.GetServerInfo()
			if err2 == nil {
				c.JSON(http.StatusOK, info2)
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"status":   "connected",
				"degraded": true,
				"detail":   err2.Error(),
			})
			return
		}

		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// GET /api/owl/devices
	if method == http.MethodGet && path == "/owl/devices" {
		page := 1
		pageSize := 100
		if v := c.Query("page"); v != "" {
			fmt.Sscanf(v, "%d", &page)
		}
		if v := c.Query("page_size"); v != "" {
			fmt.Sscanf(v, "%d", &pageSize)
		}
		items, total, err := client.GetDevices(page, pageSize)
		if err != nil {
			if loginErr := client.Login(); loginErr == nil {
				items, total, err = client.GetDevices(page, pageSize)
			}
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": err.Error(), "degraded": true})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
		return
	}

	// GET /api/owl/devices/:id
	if method == http.MethodGet && strings.HasPrefix(path, "/owl/devices/") && !strings.HasSuffix(path, "/channels") {
		deviceID := strings.TrimPrefix(path, "/owl/devices/")
		deviceID = strings.Trim(deviceID, "/")
		if deviceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing device id"})
			return
		}
		device, err := client.GetDevice(deviceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, device)
		return
	}

	// GET /api/owl/devices/:id/channels
	if method == http.MethodGet && strings.HasPrefix(path, "/owl/devices/") && strings.HasSuffix(path, "/channels") {
		raw := strings.TrimPrefix(path, "/owl/devices/")
		deviceID := strings.TrimSuffix(raw, "/channels")
		deviceID = strings.Trim(deviceID, "/")
		if deviceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing device id"})
			return
		}
		page := 1
		pageSize := 100
		if v := c.Query("page"); v != "" {
			fmt.Sscanf(v, "%d", &page)
		}
		if v := c.Query("page_size"); v != "" {
			fmt.Sscanf(v, "%d", &pageSize)
		}
		items, total, err := client.GetChannels(page, pageSize, deviceID)
		if err != nil {
			if loginErr := client.Login(); loginErr == nil {
				items, total, err = client.GetChannels(page, pageSize, deviceID)
			}
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
		return
	}

	// GET /api/owl/channels
	if method == http.MethodGet && path == "/owl/channels" {
		page := 1
		pageSize := 100
		if v := c.Query("page"); v != "" {
			fmt.Sscanf(v, "%d", &page)
		}
		if v := c.Query("page_size"); v != "" {
			fmt.Sscanf(v, "%d", &pageSize)
		}
		deviceID := c.Query("device_id")
		items, total, err := client.GetChannels(page, pageSize, deviceID)
		if err != nil {
			if loginErr := client.Login(); loginErr == nil {
				items, total, err = client.GetChannels(page, pageSize, deviceID)
			}
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		// 统一返回兼容结构，避免前端/旧代码口径不一致
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "success": true, "data": gin.H{"channels": items, "total": total}})
		return
	}

	// GET /api/owl/streams
	if method == http.MethodGet && path == "/owl/streams" {
		channels, err := client.GetAllChannels()
		if err != nil {
			// token 失效或服务端临时错误时，尝试重新登录后重试一次
			if loginErr := client.Login(); loginErr == nil {
				channels, err = client.GetAllChannels()
			}
		}

		streams := make(map[string]camera.Go2rtcStream)
		if err == nil {
			for _, ch := range channels {
				name := ch.ID
				if name == "" {
					name = ch.ChannelID
				}
				onlineStatus := "offline"
				if ch.IsOnline {
					onlineStatus = "online"
				}
				streams[name] = camera.Go2rtcStream{
					Name:    ch.Name,
					Sources: []string{fmt.Sprintf("proxy://owl/%s", ch.ID)},
					Producers: []camera.Producer{{Type: ch.Type, URL: onlineStatus}},
				}
			}
		} else {
			logOwlStreamsFallback(err)
		}

		// 无论 OWL 通道接口是否可用，都尽量返回 bridge 已知流，避免前端空白或 curl 无输出
		if s.config.OWLStreamBridge != nil {
			if native, nerr := s.config.OWLStreamBridge.GetAllStreams(); nerr == nil {
				for k, v := range native {
					streams[k] = v
				}
			}
		}

		c.JSON(http.StatusOK, streams)
		return
	}

	// POST /api/owl/channels/:id/play
	if method == http.MethodPost && strings.HasPrefix(path, "/owl/channels/") && strings.HasSuffix(path, "/play") {
		channelID := strings.TrimSuffix(strings.TrimPrefix(path, "/owl/channels/"), "/play")
		channelID = strings.Trim(channelID, "/")
		if channelID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing channel id"})
			return
		}

		output, err := client.Play(channelID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if s.config.OWLStreamBridge != nil {
			if _, err := s.config.OWLStreamBridge.EnsureStream(channelID); err != nil {
				log.Printf("[OWL] EnsureStream(%s) warning: %v", channelID, err)
			}
		}
		c.JSON(http.StatusOK, output)
		return
	}

	// POST /api/owl/channels/:id/stop
	if method == http.MethodPost && strings.HasPrefix(path, "/owl/channels/") && strings.HasSuffix(path, "/stop") {
		channelID := strings.TrimSuffix(strings.TrimPrefix(path, "/owl/channels/"), "/stop")
		channelID = strings.Trim(channelID, "/")
		if channelID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing channel id"})
			return
		}
		if err := client.Stop(channelID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if s.config.OWLStreamBridge != nil {
			s.config.OWLStreamBridge.StopStream(channelID)
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	// POST /api/owl/channels/:id/snapshot
	if method == http.MethodPost && strings.HasPrefix(path, "/owl/channels/") && strings.HasSuffix(path, "/snapshot") {
		channelID := strings.TrimSuffix(strings.TrimPrefix(path, "/owl/channels/"), "/snapshot")
		channelID = strings.Trim(channelID, "/")
		if channelID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing channel id"})
			return
		}
		data, err := client.SnapshotWithAuth(channelID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Type", "image/jpeg")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "image/jpeg", data)
		return
	}

	// POST /api/owl/ptz/control
	if method == http.MethodPost && path == "/owl/ptz/control" {
		var req struct {
			ChannelID string `json:"channel_id"`
			Command   string `json:"command"`
			Speed     int    `json:"speed"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.ChannelID) == "" || strings.TrimSpace(req.Command) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "channel_id and command are required"})
			return
		}
		if req.Speed <= 0 {
			req.Speed = 4
		}
		if err := client.PTZControl(req.ChannelID, req.Command, req.Speed); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "unknown owl endpoint"})
}

// handleYOLOControl 处理 YOLO 控制 API 请求
func (s *Server) handleYOLOControl(c *gin.Context, path string) {
	ctrl := GetYOLOControl()
	
	// 解析路径: /yolo/status, /yolo/enable/:id, /yolo/disable/:id, /yolo/check/:id, /yolo/settings
	parts := strings.Split(strings.TrimPrefix(path, "/yolo/"), "/")
	if len(parts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid path"})
		return
	}
	
	action := parts[0]
	cameraID := ""
	if len(parts) > 1 {
		cameraID = parts[1]
	}
	
	switch action {
	case "status":
		if c.Request.Method == "GET" {
			c.JSON(http.StatusOK, gin.H{
				"enabled_cameras": ctrl.GetEnabledCameras(),
				"sync_frame_rate": ctrl.GetSyncFrameRate(),
				"jpeg_quality":    ctrl.GetJPEGQuality(),
			})
		} else {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		}
	case "enable":
		if c.Request.Method == "POST" && cameraID != "" {
			ctrl.Enable(cameraID)
			c.JSON(http.StatusOK, gin.H{"success": true, "camera_id": cameraID, "enabled": true})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		}
	case "disable":
		if c.Request.Method == "POST" && cameraID != "" {
			ctrl.Disable(cameraID)
			c.JSON(http.StatusOK, gin.H{"success": true, "camera_id": cameraID, "enabled": false})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		}
	case "check":
		if c.Request.Method == "GET" && cameraID != "" {
			c.JSON(http.StatusOK, gin.H{"camera_id": cameraID, "enabled": ctrl.IsEnabled(cameraID)})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		}
	case "pipeline-stats":
		if c.Request.Method == "GET" {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    ctrl.GetPipelineStats(),
			})
		} else {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		}
	case "settings":
		if c.Request.Method == "POST" {
			var req struct {
				SyncFrameRate  int `json:"sync_frame_rate"`
				JPEGQuality    int `json:"jpeg_quality"`
				VLMIntervalSec int `json:"vlm_interval_sec"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if req.SyncFrameRate > 0 {
				ctrl.SetSyncFrameRate(req.SyncFrameRate)
			}
			if req.JPEGQuality > 0 {
				ctrl.SetJPEGQuality(req.JPEGQuality)
			}
			if req.VLMIntervalSec > 0 {
				ctrl.SetVLMInterval(req.VLMIntervalSec)
			}
			c.JSON(http.StatusOK, gin.H{
				"success":          true,
				"sync_frame_rate":  ctrl.GetSyncFrameRate(),
				"jpeg_quality":     ctrl.GetJPEGQuality(),
				"vlm_interval_sec": ctrl.GetVLMInterval(),
			})
		} else {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		}
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown action"})
	}
}

// handleGo2rtcWebSocket 代理 WebSocket 连接到 go2rtc 的 /api/ws 端点
// 用于 MSE 视频播放：前端通过 WebSocket 接收 fMP4 segments
// 优于 HTTP Fetch 方式：
//   - 每个 WS 消息 = 完整 fMP4 segment（天然对齐帧边界）
//   - 无 HTTP 代理缓冲问题（不需要 FlushInterval=-1）
//   - 支持编解码器协商（服务端告知实际 codec）
func (s *Server) handleGo2rtcWebSocket(c *gin.Context) {
	// 构建 go2rtc WebSocket URL
	go2rtcURL, err := url.Parse(s.config.Go2rtcURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid go2rtc URL"})
		return
	}

	wsScheme := "ws"
	if go2rtcURL.Scheme == "https" {
		wsScheme = "wss"
	}
	backendURL := fmt.Sprintf("%s://%s/api/ws?%s", wsScheme, go2rtcURL.Host, c.Request.URL.RawQuery)

	// 升级前端连接为 WebSocket
	frontConn, err := go2rtcWSUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer frontConn.Close()

	// 连接到 go2rtc 后端（使用较大读缓冲区减少大视频 segment 的内存分配次数）
	backDialer := gorillaws.Dialer{
		ReadBufferSize:  128 * 1024, // 128KB — I-帧 segment 可达 50-100KB+
		WriteBufferSize: 4096,       // 4KB — 客户端消息较小（codec 请求等）
	}
	backConn, _, err := backDialer.Dial(backendURL, nil)
	if err != nil {
		frontConn.WriteMessage(gorillaws.CloseMessage,
			gorillaws.FormatCloseMessage(gorillaws.CloseInternalServerErr, "backend connection failed"))
		return
	}
	defer backConn.Close()

	// 双向中继
	done := make(chan struct{})

	// 前端 → 后端（客户端发送 codec 请求等）
	go func() {
		defer close(done)
		for {
			msgType, data, err := frontConn.ReadMessage()
			if err != nil {
				return
			}
			if err := backConn.WriteMessage(msgType, data); err != nil {
				return
			}
		}
	}()

	// 后端 → 前端（fMP4 segments）
	for {
		msgType, data, err := backConn.ReadMessage()
		if err != nil {
			break
		}
		if err := frontConn.WriteMessage(msgType, data); err != nil {
			break
		}
	}

	<-done
}

// handleInferenceWebSocket 处理推理 WebSocket 连接
func (s *Server) handleInferenceWebSocket(c *gin.Context) {
	// 如果有 WSHub，使用它处理
	if s.config.WSHub != nil {
		if hub, ok := s.config.WSHub.(*websocket.Hub); ok {
			websocket.HandleInferenceWebSocket(c, hub)
			return
		}
	}

	// 否则返回错误
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "Inference WebSocket hub not available",
	})
}
