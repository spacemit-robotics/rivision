// Package main RiVision Hub 入口
// 整合 rivision_gateway + 扩展模块，完全符合 RiVision_Architecture_Design.md
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"

	// Gateway 核心模块 (完整复用 rivision_gateway)
	nodeauth "github.com/rivision/rivision-hub/internal/gateway/auth"
	"github.com/rivision/rivision-hub/internal/gateway/nodes"
	"github.com/rivision/rivision-hub/internal/gateway/tasks"

	// Hub 扩展模块
	"github.com/rivision/rivision-hub/internal/alerts"
	"github.com/rivision/rivision-hub/internal/analytics"
	hubapi "github.com/rivision/rivision-hub/internal/api"
	"github.com/rivision/rivision-hub/internal/auth"
	"github.com/rivision/rivision-hub/internal/cameras"
	"github.com/rivision/rivision-hub/internal/knowledge"
	"github.com/rivision/rivision-hub/internal/owl"
	"github.com/rivision/rivision-hub/internal/search"
	"github.com/rivision/rivision-hub/internal/websocket"
	"github.com/rivision/rivision-hub/pkg/models"
)

// HubConfig Hub配置
type HubConfig struct {
	Server struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`

	Database struct {
		Type string `yaml:"type"`
		URL  string `yaml:"url"`
	} `yaml:"database"`

	Redis struct {
		URL string `yaml:"url"`
	} `yaml:"redis"`

	Nodes struct {
		HeartbeatTimeout    int    `yaml:"heartbeat_timeout"`
		HealthCheckInterval int    `yaml:"health_check_interval"`
		ConfigPath          string `yaml:"config_path"`
		TokenAuthEnabled    bool   `yaml:"token_auth_enabled"`    // 是否启用 Token 认证
		TokenPersistence    bool   `yaml:"token_persistence"`     // 是否持久化 Token
	} `yaml:"nodes"`

	Alerts struct {
		DedupWindow  int `yaml:"dedup_window"`
		Integrations struct {
			WeChat struct {
				Enabled    bool   `yaml:"enabled"`
				WebhookURL string `yaml:"webhook_url"`
			} `yaml:"wechat"`
			DingTalk struct {
				Enabled    bool   `yaml:"enabled"`
				WebhookURL string `yaml:"webhook_url"`
				Secret     string `yaml:"secret"`
			} `yaml:"dingtalk"`
			Webhook struct {
				Enabled bool   `yaml:"enabled"`
				URL     string `yaml:"url"`
				Secret  string `yaml:"secret"`
			} `yaml:"webhook"`
		} `yaml:"integrations"`
	} `yaml:"alerts"`

	Knowledge struct {
		SyncInterval int `yaml:"sync_interval"`
	} `yaml:"knowledge"`

	Auth struct {
		TokenSecret string `yaml:"token_secret"`
		TokenExpiry int    `yaml:"token_expiry_hours"`
	} `yaml:"auth"`

	Owl struct {
		Enabled    bool   `yaml:"enabled"`
		URL        string `yaml:"url"`
		Username   string `yaml:"username"`
		Password   string `yaml:"password"`
		UseRSAAuth bool   `yaml:"use_rsa_auth"`
		CacheTTL   int    `yaml:"cache_ttl"` // seconds
	} `yaml:"owl"`
}

// Event 事件
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`      // detection, alert, system
	Title     string    `json:"title"`
	NodeID    string    `json:"node_id"`
	CameraID  string    `json:"camera_id,omitempty"`
	Level     string    `json:"level"` // info, warning, critical
	Data      any       `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// EventStore 事件存储
type EventStore struct {
	events []Event
	mu     sync.RWMutex
	maxLen int
}

// NewEventStore 创建事件存储
func NewEventStore(maxLen int) *EventStore {
	return &EventStore{
		events: make([]Event, 0),
		maxLen: maxLen,
	}
}

// Add 添加事件
func (s *EventStore) Add(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if e.ID == "" {
		e.ID = fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	
	s.events = append([]Event{e}, s.events...)
	if len(s.events) > s.maxLen {
		s.events = s.events[:s.maxLen]
	}
}

// List 获取事件列表
func (s *EventStore) List(limit int, eventType string) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	
	if eventType == "" || eventType == "all" {
		return s.events[:limit]
	}
	
	// 过滤类型
	result := make([]Event, 0)
	for _, e := range s.events {
		if e.Type == eventType {
			result = append(result, e)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func main() {
	os.Setenv("TZ", "Asia/Shanghai")

	log.Printf("========================================")
	log.Printf("  RiVision Hub v2.0.0")
	log.Printf("  基于 rivision_gateway 完整复用")
	log.Printf("========================================")

	// 加载配置
	hubCfg := loadHubConfig()

	// 初始化数据库
	var db *sql.DB
	if hubCfg.Database.URL != "" {
		var err error
		dbDriver := "postgres"
		dbURL := hubCfg.Database.URL
		if hubCfg.Database.Type == "sqlite" {
			dbDriver = "sqlite3"
			// SQLite 需要确保目录存在
			if idx := strings.LastIndex(dbURL, "/"); idx > 0 {
				os.MkdirAll(dbURL[:idx], 0755)
			}
		}
		db, err = sql.Open(dbDriver, dbURL)
		if err != nil {
			log.Printf("[Hub] 数据库连接失败: %v，使用内存模式", err)
		} else {
			if hubCfg.Database.Type != "sqlite" {
				db.SetMaxOpenConns(25)
				db.SetMaxIdleConns(5)
			}
			log.Printf("[Hub] 数据库连接成功 (%s)", dbDriver)
		}
	}

	// ========================================
	// 复用 Gateway 核心组件 (完整复制)
	// ========================================
	
	// 节点注册表 (完整复用 rivision_gateway/nodes)
	registry := nodes.NewRegistry()
	nodesConfigPath := hubCfg.Nodes.ConfigPath
	if nodesConfigPath == "" {
		nodesConfigPath = "data/nodes_registry.yaml"
	}
	os.MkdirAll("data", 0755)
	if err := registry.LoadFromFile(nodesConfigPath); err != nil {
		log.Printf("[Hub] 加载节点配置: %v (首次启动为正常)", err)
	}
	registry.SetPersistPath(nodesConfigPath)

	// 心跳超时配置
	heartbeatTimeout := hubCfg.Nodes.HeartbeatTimeout
	if heartbeatTimeout == 0 {
		heartbeatTimeout = 60
	}

	// 任务追踪器 (复用 rivision_gateway/tasks)
	taskTracker := tasks.NewTaskTracker(1000)

	// ========================================
	// 初始化 Hub 扩展模块
	// ========================================

	// 告警引擎
	alertEngine := alerts.NewEngine(db, alerts.Config{
		DedupWindow: time.Duration(hubCfg.Alerts.DedupWindow) * time.Second,
		Integrations: alerts.IntegrationsConfig{
			WeChat: &alerts.WeChartConfig{
				Enabled:    hubCfg.Alerts.Integrations.WeChat.Enabled,
				WebhookURL: hubCfg.Alerts.Integrations.WeChat.WebhookURL,
			},
			DingTalk: &alerts.DingTalkConfig{
				Enabled:    hubCfg.Alerts.Integrations.DingTalk.Enabled,
				WebhookURL: hubCfg.Alerts.Integrations.DingTalk.WebhookURL,
				Secret:     hubCfg.Alerts.Integrations.DingTalk.Secret,
			},
			Webhook: &alerts.WebhookConfig{
				Enabled: hubCfg.Alerts.Integrations.Webhook.Enabled,
				URL:     hubCfg.Alerts.Integrations.Webhook.URL,
				Secret:  hubCfg.Alerts.Integrations.Webhook.Secret,
			},
		},
	})

	// 知识库
	knowledgeStore, err := knowledge.NewStore(db)
	if err != nil {
		log.Printf("[Hub] 知识库初始化失败: %v (知识库功能不可用)", err)
	}
	var knowledgeSyncer *knowledge.Syncer
	if knowledgeStore != nil {
		knowledgeSyncer = knowledge.NewSyncer(knowledgeStore, time.Duration(hubCfg.Knowledge.SyncInterval)*time.Second)
	}

	// 联邦搜索
	federatedSearch := search.NewFederatedSearch(1000, 5*time.Minute)

	// 摄像头管理
	cameraStore, _ := cameras.NewStore(db)

	// 分析引擎
	analyticsEngine := analytics.NewEngine(db)

	// 事件存储 (保存最近1000条事件)
	eventStore := NewEventStore(1000)
	
	// 添加一些示例事件
	eventStore.Add(Event{Type: "system", Title: "Hub 启动", Level: "info"})

	// 用户认证
	authStore, _ := auth.NewStore(db)

	// WebSocket Hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Worker 事件接收器
	eventReceiver := websocket.NewEventReceiver(wsHub)
	eventReceiver.RegisterHandler("alert", func(event *websocket.WorkerEvent) {
		handleWorkerAlertEvent(alertEngine, analyticsEngine, event)
	})
	eventReceiver.RegisterHandler("detection", func(event *websocket.WorkerEvent) {
		handleWorkerDetectionEvent(analyticsEngine, wsHub, event)
	})
	eventReceiver.RegisterHandler("camera_status", func(event *websocket.WorkerEvent) {
		handleWorkerCameraStatusEvent(cameraStore, wsHub, event)
	})

	// ========================================
	// 启动 Gin 服务
	// ========================================
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(corsMiddleware())

	// 认证中间件和RBAC权限控制
	authHandler := auth.NewHandler(authStore)
	rbac := auth.NewRBAC()
	
	// 根据环境变量启用认证 (默认启用)
	requireAuth := os.Getenv("REQUIRE_AUTH") != "false"
	if requireAuth {
		log.Printf("[Hub] 认证已启用，业务API需要登录")
		engine.Use(globalAuthMiddleware(authHandler, rbac))
	} else {
		log.Printf("[Hub] 认证已禁用 (REQUIRE_AUTH=false)")
	}

	// ========================================
	// API 路由 (符合 RiVision_Architecture_Design.md)
	// ========================================
	v1 := engine.Group("/api/v1")
	{
		// --- 搜索 API (POST /api/v1/search/*) ---
		searchHandler := hubapi.NewSearchHandler(federatedSearch, func() []search.NodeInfo {
			healthyNodes := registry.GetHealthy()
			result := make([]search.NodeInfo, len(healthyNodes))
			for i, n := range healthyNodes {
				result[i] = search.NodeInfo{
					ID:   n.ID,
					Host: n.Host,
					Port: n.AgentPort,
				}
			}
			return result
		})
		searchHandler.SetupRoutes(v1)

		// --- 告警 API (GET/POST /api/v1/alerts/*) ---
		alertsHandler := hubapi.NewAlertsHandler(alertEngine)
		alertsHandler.SetupRoutes(v1)

		// --- 知识库 API (/api/v1/knowledge/*) ---
		knowledgeHandler := hubapi.NewKnowledgeHandler(knowledgeStore, knowledgeSyncer)
		knowledgeHandler.SetupRoutes(v1)

		// --- 规则 API (完整 CRUD /api/v1/rules/*) ---
		// 使用 knowledgeHandler 提供的完整 Rules API
		rulesGroup := v1.Group("/rules")
		{
			rulesGroup.GET("", func(c *gin.Context) {
				rules, err := knowledgeStore.GetRules()
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(200, gin.H{"rules": rules, "version": knowledgeStore.GetVersion()})
			})
			rulesGroup.POST("", func(c *gin.Context) {
				var req models.CreateRuleRequest
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(400, gin.H{"error": err.Error()})
					return
				}
				rule, err := knowledgeStore.CreateRule(&req)
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(201, rule)
			})
			rulesGroup.GET("/:id", func(c *gin.Context) {
				rule, err := knowledgeStore.GetRule(c.Param("id"))
				if err != nil {
					c.JSON(404, gin.H{"error": "规则不存在"})
					return
				}
				c.JSON(200, rule)
			})
			rulesGroup.PUT("/:id", func(c *gin.Context) {
				var req models.UpdateRuleRequest
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(400, gin.H{"error": err.Error()})
					return
				}
				rule, err := knowledgeStore.UpdateRule(c.Param("id"), &req)
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(200, rule)
			})
			rulesGroup.DELETE("/:id", func(c *gin.Context) {
				if err := knowledgeStore.DeleteRule(c.Param("id")); err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(200, gin.H{"success": true})
			})
		}

		// --- 节点管理 API (复用 Gateway API) ---
		// Forward-declare camerasHandler — assigned below, but captured by closure
		// (safe: no requests arrive until server starts after all setup is done)
		var camerasHandler *cameras.Handler
		tokenAuthEnabled := hubCfg.Nodes.TokenAuthEnabled
		nodesGroup := v1.Group("/nodes")
		{
			// POST /api/v1/nodes/register
			nodesGroup.POST("/register", func(c *gin.Context) {
				handleNodeRegister(c, registry, cameraStore, knowledgeStore, camerasHandler, tokenAuthEnabled)
			})
			// POST /api/v1/nodes/:id/heartbeat (符合设计文档)
			nodesGroup.POST("/:id/heartbeat", func(c *gin.Context) {
				handleNodeHeartbeat(c, registry, heartbeatTimeout)
			})
			// GET /api/v1/nodes
			nodesGroup.GET("", func(c *gin.Context) {
				handleNodeList(c, registry, cameraStore, heartbeatTimeout)
			})
			// GET /api/v1/nodes/:id
			nodesGroup.GET("/:id", func(c *gin.Context) {
				handleNodeDetail(c, registry, heartbeatTimeout)
			})
			// DELETE /api/v1/nodes/:id
			nodesGroup.DELETE("/:id", func(c *gin.Context) {
				handleNodeUnregister(c, registry)
			})
		}

		// --- 节点 Token 管理 API ---
		adminGroup := v1.Group("/admin")
		// 管理员API需要认证 (复用 Hub 的认证逻辑)
		if authStore != nil {
			adminGroup.Use(func(c *gin.Context) {
				// 验证 Bearer Token
				authHeader := c.GetHeader("Authorization")
				if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
					c.JSON(401, gin.H{"error": "需要认证"})
					c.Abort()
					return
				}
				token := strings.TrimPrefix(authHeader, "Bearer ")
				user, valid := authStore.ValidateSession(token)
				if !valid || user == nil {
					c.JSON(401, gin.H{"error": "无效的 Token"})
					c.Abort()
					return
				}
				c.Set("user", user)
				c.Next()
			})
		}
		{
			// POST /api/v1/admin/node-tokens/generate
			adminGroup.POST("/node-tokens/generate", func(c *gin.Context) {
				handleGenerateToken(c)
			})
			// GET /api/v1/admin/node-tokens/list
			adminGroup.GET("/node-tokens/list", func(c *gin.Context) {
				handleListTokens(c)
			})
			// DELETE /api/v1/admin/node-tokens/:token
			adminGroup.DELETE("/node-tokens/:token", func(c *gin.Context) {
				handleRevokeToken(c)
			})
		}

		// --- 摄像头管理 API (/api/v1/cameras/*) ---
		camerasHandler = cameras.NewHandler(cameraStore, func() []cameras.NodeInfo {
			healthyNodes := registry.GetHealthy()
			result := make([]cameras.NodeInfo, len(healthyNodes))
			for i, n := range healthyNodes {
				agentPort := n.AgentPort
				if agentPort == 0 {
					agentPort = 9181 // Worker 默认端口
				}
				result[i] = cameras.NodeInfo{
					ID:          n.ID,
					Host:        n.Host,
					AgentPort:   agentPort,
					OwlURL:      n.OwlURL,
					ZLMHost:     n.ZLMHost,
					ZLMHTTPPort: n.ZLMHTTPPort,
					ZLMRTSPPort: n.ZLMRTSPPort,
					Healthy:     n.Status == nodes.NodeStatusHealthy,
				}
			}
			return result
		})
		camerasHandler.SetupRoutes(v1)

		// Wire OWL integration: Hub OWL for discovery + SIP-media split (PlayRemote)
		if hubCfg.Owl.Enabled && hubCfg.Owl.URL != "" {
			owlCfg := owl.Config{
				URL:        hubCfg.Owl.URL,
				Username:   hubCfg.Owl.Username,
				Password:   hubCfg.Owl.Password,
				UseRSAAuth: hubCfg.Owl.UseRSAAuth,
				Enabled:    true,
			}
			if hubCfg.Owl.CacheTTL > 0 {
				owlCfg.CacheTTL = time.Duration(hubCfg.Owl.CacheTTL) * time.Second
			}
			hubOwlClient := owl.NewClient(owlCfg)
			camerasHandler.SetOwlClient(hubOwlClient)
			camerasHandler.SetOwlConfig(owlCfg) // shared auth for per-node OWL clients
			log.Printf("[Hub] OWL integration enabled: %s", hubCfg.Owl.URL)
		}

		// --- 流媒体 API (/api/v1/streams/*) ---
		// 优化方案：Hub 不代理视频流，仅返回 Worker 流地址，前端直连 Worker
		streamsGroup := v1.Group("/streams")
		{
			// 获取 Worker 流地址信息的通用函数
			getWorkerStreamInfo := func(cameraID string) (workerHost string, go2rtcPort int, err error) {
				// 获取摄像头信息
				cam, ok := cameraStore.GetCamera(cameraID)
				if !ok {
					return "", 0, fmt.Errorf("摄像头不存在: %s", cameraID)
				}
				
				// 获取分配的节点
				if cam.NodeID == "" {
					return "", 0, fmt.Errorf("摄像头未分配到节点")
				}
				
				// 查找节点信息
				node, ok := registry.Get(cam.NodeID)
				if !ok {
					return "", 0, fmt.Errorf("节点不存在: %s", cam.NodeID)
				}
				
				return node.Host, 1984, nil // go2rtc 默认端口 1984
			}
			
			// GET /api/v1/streams/:id/url - 获取流地址（前端直连 Worker）
			streamsGroup.GET("/:id/url", func(c *gin.Context) {
				cameraID := c.Param("id")
				
				workerHost, go2rtcPort, err := getWorkerStreamInfo(cameraID)
				if err != nil {
					c.JSON(400, gin.H{"error": err.Error()})
					return
				}
				
				// 返回 Worker 的 go2rtc 流地址
				c.JSON(200, gin.H{
					"camera_id":   cameraID,
					"worker_host": workerHost,
					"go2rtc_port": go2rtcPort,
					"streams": gin.H{
						"mp4":    fmt.Sprintf("http://%s:%d/api/stream.mp4?src=%s", workerHost, go2rtcPort, cameraID),
						"mjpeg":  fmt.Sprintf("http://%s:%d/api/stream.mjpeg?src=%s", workerHost, go2rtcPort, cameraID),
						"frame":  fmt.Sprintf("http://%s:%d/api/frame.jpeg?src=%s", workerHost, go2rtcPort, cameraID),
						"webrtc": fmt.Sprintf("http://%s:%d/api/webrtc?src=%s", workerHost, go2rtcPort, cameraID),
						"rtsp":   fmt.Sprintf("rtsp://%s:8554/%s", workerHost, cameraID),
					},
				})
			})
			
		}

		// --- 统计分析 API (/api/v1/analytics/*) ---
		analyticsHandler := analytics.NewHandler(analyticsEngine)
		analyticsHandler.SetupRoutes(v1)

		// --- 推理 API ---
		inferenceGroup := v1.Group("/inference")
		{
			// POST /api/v1/inference/chat - 转发到健康的 Worker 节点
			inferenceGroup.POST("/chat", func(c *gin.Context) {
				handleInferenceChat(c, registry)
			})
			// POST /api/v1/inference/detect - 转发到健康的 Worker 节点
			inferenceGroup.POST("/detect", func(c *gin.Context) {
				handleInferenceDetect(c, registry)
			})
			// GET /api/v1/inference/tasks/:id
			inferenceGroup.GET("/tasks/:id", func(c *gin.Context) {
				handleTaskStatus(c, taskTracker)
			})
		}

		// --- 事件 API (/api/v1/events/*) ---
		eventsGroup := v1.Group("/events")
		{
			// GET /api/v1/events - 获取事件列表
			eventsGroup.GET("", func(c *gin.Context) {
				limit := 50
				if l := c.Query("limit"); l != "" {
					if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
						limit = parsed
					}
				}
				eventType := c.Query("type")
				events := eventStore.List(limit, eventType)
				c.JSON(200, gin.H{"events": events, "total": len(events)})
			})
			// GET /api/v1/events/:id - 获取单个事件
			eventsGroup.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				for _, e := range eventStore.List(1000, "") {
					if e.ID == id {
						c.JSON(200, e)
						return
					}
				}
				c.JSON(404, gin.H{"error": "事件不存在"})
			})
			// GET /api/v1/system/stats - 统计数据
			v1.GET("/system/stats", func(c *gin.Context) {
				events := eventStore.List(1000, "")
				alertCount := 0
				for _, e := range events {
					if e.Type == "alert" {
						alertCount++
					}
				}
				camStats := cameraStore.GetStats()
				nodes := registry.GetHealthy()
				c.JSON(200, gin.H{
					"total_cameras":  camStats["total"],
					"active_cameras": camStats["online"],
					"today_events":   len(events),
					"today_alerts":   alertCount,
					"total_nodes":    len(registry.GetAll()),
					"online_nodes":   len(nodes),
				})
			})
		}

		// --- 系统 API (/api/v1/system/*) ---
		systemGroup := v1.Group("/system")
		{
			systemGroup.GET("/health", handleSystemHealth)
			systemGroup.GET("/version", handleSystemVersion)
			systemGroup.GET("/config", func(c *gin.Context) {
				c.JSON(200, gin.H{
					"heartbeat_timeout":     hubCfg.Nodes.HeartbeatTimeout,
					"health_check_interval": hubCfg.Nodes.HealthCheckInterval,
				})
			})
		}

		// --- 认证 API (/api/v1/auth/*) ---
		authGroup := v1.Group("/auth")
		{
			authGroup.POST("/login", authHandler.Login)
			authGroup.POST("/logout", authHandler.Logout)
			authGroup.GET("/me", func(c *gin.Context) {
				user, exists := c.Get("user")
				if !exists {
					c.JSON(401, gin.H{"error": "not authenticated"})
					return
				}
				c.JSON(200, user)
			})
		}
	}

	// --- WebSocket 端点 ---
	// /ws/events - 前端实时事件
	engine.GET("/ws/events", wsHub.HandleWebSocket)
	// /ws/worker - Worker 事件上报
	engine.GET("/ws/worker", eventReceiver.HandleWorkerWebSocket)
	// /api/v1/ws/events - 兼容旧路径
	engine.GET("/api/v1/ws/events", eventReceiver.HandleWorkerWebSocket)

	// --- 静态文件服务 (Web UI) ---
	// 设置正确的 MIME 类型
	engine.Use(mimeTypeMiddleware())
	// 静态资源 (JS/CSS/图片等)
	engine.Static("/assets", "./web/assets")
	// 其他静态文件
	engine.StaticFile("/favicon.ico", "./web/favicon.ico")
	engine.StaticFile("/vite.svg", "./web/vite.svg")
	// SPA 入口：所有非 API 路由返回 index.html
	engine.NoRoute(func(c *gin.Context) {
		// API 路由返回 404
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
			strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		// 其他路由返回 index.html (SPA)
		c.File("./web/index.html")
	})

	// ========================================
	// 启动后台服务
	// ========================================
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动健康检查循环 (复用 Gateway 逻辑)
	go healthCheckLoop(ctx, registry, heartbeatTimeout)

	// 启动知识库同步
	if knowledgeSyncer != nil {
		go knowledgeSyncer.Start(ctx, func() []knowledge.NodeInfo {
			healthyNodes := registry.GetHealthy()
			result := make([]knowledge.NodeInfo, len(healthyNodes))
			for i, n := range healthyNodes {
				result[i] = knowledge.NodeInfo{
					ID:   n.ID,
					Host: n.Host,
					Port: n.AgentPort,
				}
			}
			return result
		})
	}

	// 启用任务追踪器 (保持引用避免未使用变量错误)
	_ = taskTracker

	// ========================================
	// 启动 HTTP 服务器
	// ========================================
	addr := fmt.Sprintf("%s:%d", hubCfg.Server.Host, hubCfg.Server.Port)
	log.Printf("[Hub] 启动 HTTP 服务: %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: engine,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Hub] HTTP 服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Hub] 正在关闭服务...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Hub] 服务关闭错误: %v", err)
	}

	if db != nil {
		db.Close()
	}

	log.Println("[Hub] 服务已关闭")
}

// ========================================
// 节点管理处理函数 (复用 Gateway 逻辑)
// ========================================

func handleNodeRegister(c *gin.Context, registry *nodes.Registry, cameraStore *cameras.Store, knowledgeStore *knowledge.Store, camerasHandler *cameras.Handler, tokenAuthEnabled bool) {
	var req struct {
		NodeID            string                 `json:"node_id"`
		Name              string                 `json:"name"`
		Type              string                 `json:"type"`
		IP                string                 `json:"ip"`
		Host              string                 `json:"host"`
		Port              int                    `json:"port"`
		AgentPort         int                    `json:"agent_port"`
		YoloEnabled       bool                   `json:"yolo_enabled"`
		VlmEnabled        bool                   `json:"vlm_enabled"`
		Capabilities      map[string]interface{} `json:"capabilities"`
		Version           string                 `json:"version"`
		RegistrationToken string                 `json:"registration_token"` // Token 认证
		OwlURL            string                 `json:"owl_url"`             // Worker OWL API URL
		ZLMHost           string                 `json:"zlm_host"`            // Worker ZLM IP for RTP
		ZLMHTTPPort       int                    `json:"zlm_http_port"`       // Worker ZLM HTTP API port
		ZLMRTSPPort       int                    `json:"zlm_rtsp_port"`       // Worker ZLM RTSP port
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = fmt.Sprintf("worker_%s_%d", req.IP, req.Port)
	}

	host := req.Host
	if host == "" {
		host = req.IP
	}

	// Token 认证
	if tokenAuthEnabled {
		authManager := nodeauth.GetNodeAuthManager()
		clientIP := c.ClientIP()

		// 检查黑名单
		if authManager.IsBlacklisted(clientIP) {
			c.JSON(403, gin.H{"error": "IP 在黑名单中", "success": false})
			return
		}

		// 验证 Token (如果提供了)
		if req.RegistrationToken != "" {
			valid, msg := authManager.ValidateToken(req.RegistrationToken, nodeID)
			if !valid {
				c.JSON(401, gin.H{"error": msg, "success": false})
				return
			}
		} else {
			// 检查是否为已注册节点重连 (允许不带 Token 重连)
			if _, registered := authManager.GetRegisteredNode(nodeID); !registered {
				c.JSON(401, gin.H{"error": "需要注册 Token", "success": false})
				return
			}
		}
	}

	nodeName := req.Name
	if nodeName == "" {
		nodeName = nodeID
	}

	nodeType := req.Type
	if nodeType == "" {
		nodeType = "worker"
	}

	node := nodes.NewNode(nodeID, nodeName, "", "")
	node.Type = nodeType
	node.Host = host
	node.Port = req.Port
	node.AgentPort = req.AgentPort
	node.YoloEnabled = req.YoloEnabled
	node.LlamaEnabled = req.VlmEnabled // VLM 使用 LlamaEnabled 字段
	node.Capabilities = req.Capabilities
	node.OwlURL = req.OwlURL
	node.ZLMHost = req.ZLMHost
	node.ZLMHTTPPort = req.ZLMHTTPPort
	node.ZLMRTSPPort = req.ZLMRTSPPort
	node.Enabled = true
	node.MarkHealthy()
	node.UpdateHeartbeat() // 注册时设置心跳时间，避免立即被判定为超时

	registry.Register(node)

	// 获取分配的摄像头（包含完整配置）
	assignedCameras := []string{}
	assignedCamerasConfig := []map[string]interface{}{}
	if cameraStore != nil {
		filter := cameras.CameraFilter{NodeID: nodeID}
		cams := cameraStore.ListCameras(filter)
		for _, cam := range cams {
			assignedCameras = append(assignedCameras, cam.ID)
			// 构建摄像头配置，包含AI设置
			camConfig := map[string]interface{}{
				"id":       cam.ID,
				"name":     cam.Name,
				"url":      cam.URL,
				"protocol": cam.Protocol,
				"enabled":  cam.Enabled,
			}
			// 合并AI配置
			if cam.Config != nil {
				for k, v := range cam.Config {
					camConfig[k] = v
				}
			}
			assignedCamerasConfig = append(assignedCamerasConfig, camConfig)
		}
	}

	// 获取知识库版本
	kbVersion := 0
	if knowledgeStore != nil {
		kbVersion = knowledgeStore.GetVersion()
	}

	log.Printf("[Hub] 节点注册: %s (%s:%d)", nodeID, host, req.Port)

	// Re-activate hub_sip_remote streams asynchronously (SIP sessions lost after restart)
	if camerasHandler != nil && cameraStore != nil {
		go func() {
			filter := cameras.CameraFilter{NodeID: nodeID}
			nodeCams := cameraStore.ListCameras(filter)
			for _, cam := range nodeCams {
				mode, _ := cam.Config["stream_mode"].(string)
				if mode != "hub_sip_remote" {
					continue
				}
				ctx := context.Background()
				if rtspURL, err := camerasHandler.ActivateStream(ctx, cam); err != nil {
					log.Printf("[Hub] re-activate stream %s failed: %v", cam.ID, err)
				} else if rtspURL != "" && rtspURL != cam.URL {
					cam.URL = rtspURL
					cameraStore.UpdateCamera(cam)
					log.Printf("[Hub] re-activated stream %s → %s", cam.ID, rtspURL)
				} else {
					log.Printf("[Hub] re-activated stream %s (URL unchanged)", cam.ID)
				}
			}
		}()
	}

	// 响应符合设计文档
	c.JSON(200, gin.H{
		"status":                  "registered",
		"node_id":                 nodeID,
		"assigned_streams":        assignedCameras,
		"assigned_cameras_config": assignedCamerasConfig, // 包含完整AI配置
		"knowledge_base_version":  kbVersion,
		"heartbeat_interval":      30,
	})
}

// ========================================
// Token 管理处理函数
// ========================================

func handleGenerateToken(c *gin.Context) {
	var req struct {
		NodeID        string `json:"node_id" binding:"required"`
		LifetimeHours int    `json:"lifetime_hours"`
		MaxUses       int    `json:"max_uses"`
		Description   string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "无效的请求参数"})
		return
	}

	if req.LifetimeHours == 0 {
		req.LifetimeHours = 24
	}
	if req.MaxUses == 0 {
		req.MaxUses = 999
	}

	authManager := nodeauth.GetNodeAuthManager()
	tokenStr := authManager.GenerateRegistrationToken(
		req.NodeID,
		req.LifetimeHours*3600,
		req.MaxUses,
		req.Description,
	)

	expiresAt := time.Now().Add(time.Duration(req.LifetimeHours) * time.Hour)

	log.Printf("[Hub] 生成节点Token: node_id=%s, expires=%s", req.NodeID, expiresAt.Format(time.RFC3339))

	c.JSON(200, gin.H{
		"success":    true,
		"message":    "Token 生成成功",
		"token":      tokenStr,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

func handleListTokens(c *gin.Context) {
	authManager := nodeauth.GetNodeAuthManager()
	tokenList := authManager.GetTokens()

	tokens := make([]gin.H, 0, len(tokenList))
	for _, t := range tokenList {
		tokens = append(tokens, gin.H{
			"token":       t.Token,
			"node_id":     t.NodeID,
			"created_at":  t.CreatedAt.Format(time.RFC3339),
			"expires_at":  t.ExpiresAt.Format(time.RFC3339),
			"max_uses":    t.MaxUses,
			"used_count":  t.UsedCount,
			"is_valid":    t.IsValid(),
			"description": t.Description,
		})
	}

	c.JSON(200, gin.H{
		"success": true,
		"tokens":  tokens,
	})
}

func handleRevokeToken(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(400, gin.H{"success": false, "error": "缺少 token 参数"})
		return
	}

	authManager := nodeauth.GetNodeAuthManager()
	authManager.RevokeToken(token)

	log.Printf("[Hub] 撤销节点Token: %s", token)

	c.JSON(200, gin.H{
		"success": true,
		"message": "Token 已撤销",
	})
}

func handleNodeHeartbeat(c *gin.Context, registry *nodes.Registry, heartbeatTimeout int) {
	nodeID := c.Param("id")

	var data nodes.HeartbeatData
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	node, ok := registry.Get(nodeID)
	if !ok {
		c.JSON(404, gin.H{"error": "node not found", "should_register": true})
		return
	}

	node.UpdateStatus(&data)
	node.MarkHealthy()

	c.JSON(200, gin.H{
		"status":        "ok",
		"server_time":   time.Now().UnixMilli(),
		"next_interval": 30,
	})
}

func handleNodeList(c *gin.Context, registry *nodes.Registry, cameraStore *cameras.Store, heartbeatTimeout int) {
	allNodes := registry.GetAll()
	result := make([]map[string]interface{}, 0, len(allNodes))

	// 获取摄像头统计
	cameraCounts := make(map[string]int)
	if cameraStore != nil {
		cameraCounts = cameraStore.CountByNode()
	}

	for _, n := range allNodes {
		nodeData := n.ToDict()
		// 添加摄像头数量
		nodeData["camera_count"] = cameraCounts[n.ID]
		result = append(result, nodeData)
	}

	c.JSON(200, gin.H{"nodes": result})
}

func handleNodeDetail(c *gin.Context, registry *nodes.Registry, heartbeatTimeout int) {
	nodeID := c.Param("id")
	node, ok := registry.Get(nodeID)
	if !ok {
		c.JSON(404, gin.H{"error": "node not found"})
		return
	}
	c.JSON(200, node.ToDict())
}

func handleNodeUnregister(c *gin.Context, registry *nodes.Registry) {
	nodeID := c.Param("id")
	registry.Unregister(nodeID)
	c.JSON(200, gin.H{"status": "unregistered", "node_id": nodeID})
}

// ========================================
// 事件处理函数
// ========================================

func handleWorkerAlertEvent(alertEngine *alerts.Engine, analyticsEngine *analytics.Engine, event *websocket.WorkerEvent) {
	// 将 Data 转换为 AlertEvent
	if eventData, ok := event.Data.(map[string]interface{}); ok {
		alertEvent := &models.AlertEvent{
			Type:      fmt.Sprintf("%v", eventData["type"]),
			NodeID:    event.NodeID,
			CameraID:  fmt.Sprintf("%v", eventData["camera_id"]),
			Timestamp: event.Timestamp,
		}
		alertEngine.HandleEvent(alertEvent)
	}
}

func handleWorkerDetectionEvent(analyticsEngine *analytics.Engine, wsHub *websocket.Hub, event *websocket.WorkerEvent) {
	// 将 Data 转换为 DetectionEvent
	if eventData, ok := event.Data.(map[string]interface{}); ok {
		detEvent := &analytics.DetectionEvent{
			CameraID:  fmt.Sprintf("%v", eventData["camera_id"]),
			Timestamp: event.Timestamp,
		}
		analyticsEngine.ProcessDetection(detEvent)

		// 广播检测结果到前端（用于 YOLO 框绘制）
		wsHub.SendDetection(eventData)
	}
}

func handleWorkerCameraStatusEvent(cameraStore *cameras.Store, wsHub *websocket.Hub, event *websocket.WorkerEvent) {
	var status models.CameraStatusEvent
	data, _ := json.Marshal(event.Data)
	json.Unmarshal(data, &status)

	if cameraStore != nil {
		cameraStore.UpdateStatus(status.CameraID, status.Status)
	}

	wsHub.Broadcast(&websocket.Message{
		Type: websocket.TypeCamStatus,
		Data: status,
	})
}

// ========================================
// 系统处理函数
// ========================================

func handleSystemHealth(c *gin.Context) {
	c.JSON(200, gin.H{
		"status":  "healthy",
		"service": "rivision-hub",
		"version": "2.0.0",
		"time":    time.Now().Format(time.RFC3339),
	})
}

func handleSystemVersion(c *gin.Context) {
	c.JSON(200, gin.H{
		"version":    "2.0.0",
		"build_date": "2024-04-27",
		"go_version": "1.21",
	})
}

// ========================================
// 后台服务
// ========================================

func healthCheckLoop(ctx context.Context, registry *nodes.Registry, heartbeatTimeout int) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			allNodes := registry.GetAll()
			for _, n := range allNodes {
				if n.IsHeartbeatTimeout(heartbeatTimeout) {
					n.MarkUnhealthy(3)
					log.Printf("[Hub] 节点心跳超时: %s", n.ID)
				}
			}
		}
	}
}

// ========================================
// 配置加载
// ========================================

func loadHubConfig() *HubConfig {
	cfg := &HubConfig{}

	// 默认值
	cfg.Server.Port = 9280
	cfg.Server.Host = "0.0.0.0"
	cfg.Nodes.HeartbeatTimeout = 60
	cfg.Nodes.HealthCheckInterval = 30
	cfg.Alerts.DedupWindow = 60
	cfg.Knowledge.SyncInterval = 60
	cfg.Auth.TokenExpiry = 24

	// 尝试加载配置文件
	configPath := os.Getenv("HUB_CONFIG")
	if configPath == "" {
		configPath = "config/hub.yaml"
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = "/opt/rivision/rivision_hub/config/hub.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err == nil {
		yaml.Unmarshal(data, cfg)
	}

	// 环境变量覆盖
	if v := os.Getenv("HUB_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Server.Port)
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.Database.URL = v
	}
	if v := os.Getenv("NODES_CONFIG_PATH"); v != "" {
		cfg.Nodes.ConfigPath = v
	}
	if v := os.Getenv("TOKEN_SECRET"); v != "" {
		cfg.Auth.TokenSecret = v
	}

	return cfg
}

// ========================================
// 中间件
// ========================================

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// mimeTypeMiddleware 修复静态文件 MIME 类型
func mimeTypeMiddleware() gin.HandlerFunc {
	mimeTypes := map[string]string{
		".js":    "application/javascript",
		".mjs":   "application/javascript",
		".css":   "text/css",
		".html":  "text/html",
		".json":  "application/json",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".svg":   "image/svg+xml",
		".ico":   "image/x-icon",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".eot":   "application/vnd.ms-fontobject",
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		
		// 只处理静态资源请求
		if strings.HasPrefix(path, "/assets/") || 
		   strings.HasSuffix(path, ".js") ||
		   strings.HasSuffix(path, ".css") {
			for ext, mime := range mimeTypes {
				if strings.HasSuffix(path, ext) {
					c.Header("Content-Type", mime)
					break
				}
			}
		}

		c.Next()
	}
}

// globalAuthMiddleware 全局认证和RBAC权限控制中间件
func globalAuthMiddleware(authHandler *auth.Handler, rbac *auth.RBAC) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 公开路径：不需要认证
		publicPaths := []string{
			"/health",
			"/ready",
			"/api/v1/system/health",
			"/api/v1/system/version",
			"/api/v1/auth/login",
			"/ws/",
			"/web/",
			"/",
		}
		for _, p := range publicPaths {
			if path == p || strings.HasPrefix(path, p) {
				c.Next()
				return
			}
		}

		// 节点API：使用节点Token认证（不走用户认证）
		nodeAPIPaths := []string{
			"/api/v1/nodes/register",
			"/api/v1/nodes/heartbeat",
		}
		for _, p := range nodeAPIPaths {
			if strings.Contains(path, p) || strings.Contains(path, "/heartbeat") {
				// 节点API通过其他机制认证，这里跳过用户认证
				c.Next()
				return
			}
		}

		// 提取Token
		token := extractAuthToken(c)
		if token == "" {
			c.JSON(401, gin.H{"error": "未提供认证令牌", "code": "AUTH_REQUIRED"})
			c.Abort()
			return
		}

		// 验证Session
		session, ok := authHandler.ValidateToken(token)
		if !ok {
			c.JSON(401, gin.H{"error": "无效或过期的令牌", "code": "INVALID_TOKEN"})
			c.Abort()
			return
		}

		// 设置用户上下文
		c.Set("session", session)
		c.Set("user_id", session.UserID)
		c.Set("username", session.Username)
		c.Set("role", session.Role)

		// 管理员跳过RBAC检查
		if session.Role == "admin" {
			c.Next()
			return
		}

		// RBAC权限检查
		if !auth.CheckPermission(rbac, c) {
			return
		}

		c.Next()
	}
}

// extractAuthToken 从请求中提取认证令牌
func extractAuthToken(c *gin.Context) string {
	// Authorization Header
	authHeader := c.GetHeader("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// Query Parameter
	if token := c.Query("token"); token != "" {
		return token
	}

	// Cookie
	if token, err := c.Cookie("token"); err == nil {
		return token
	}

	return ""
}

// ========================================
// 推理 API 处理函数
// ========================================

func handleInferenceChat(c *gin.Context, registry *nodes.Registry) {
	// 获取健康的带 Llama 服务的节点
	healthyNodes := registry.GetHealthyWithLlama()
	if len(healthyNodes) == 0 {
		c.JSON(503, gin.H{"error": "no healthy llama nodes available"})
		return
	}

	// 选择第一个健康节点 (简单负载均衡)
	node := healthyNodes[0]
	targetURL := fmt.Sprintf("http://%s:%d/v1/chat/completions", node.Host, node.Port)

	// 转发请求
	proxyRequest(c, targetURL)
}

func handleInferenceDetect(c *gin.Context, registry *nodes.Registry) {
	// 获取健康的带 YOLO 服务的节点
	healthyNodes := registry.GetHealthyWithYolo()
	if len(healthyNodes) == 0 {
		c.JSON(503, gin.H{"error": "no healthy yolo nodes available"})
		return
	}

	// 选择第一个健康节点
	node := healthyNodes[0]
	targetURL := fmt.Sprintf("http://%s:%d/detect", node.Host, node.YOLOPort)

	// 转发请求
	proxyRequest(c, targetURL)
}

func handleTaskStatus(c *gin.Context, tracker *tasks.TaskTracker) {
	taskID := c.Param("id")
	
	// 在运行中的任务中查找
	runningTasks := tracker.GetRunningTasks()
	for _, t := range runningTasks {
		if t["task_id"] == taskID {
			c.JSON(200, t)
			return
		}
	}
	
	// 在已完成的任务中查找
	completedTasks := tracker.GetCompletedTasks(100)
	for _, t := range completedTasks {
		if t["task_id"] == taskID {
			c.JSON(200, t)
			return
		}
	}
	
	c.JSON(404, gin.H{"error": "task not found"})
}

func proxyRequest(c *gin.Context, targetURL string) {
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": "failed to read request body"})
		return
	}

	req, err := http.NewRequest(c.Request.Method, targetURL, strings.NewReader(string(body)))
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create proxy request"})
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"error": fmt.Sprintf("proxy request failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	// 复制响应
	c.Status(resp.StatusCode)
	for k, v := range resp.Header {
		c.Header(k, v[0])
	}

	respBody := make([]byte, 0)
	respBody, _ = json.Marshal(resp.Body)
	c.Writer.Write(respBody)
}
