// Package api implements the Hub REST API router (v6_design §5.7).
//
// Handler files per §2.1.1:
//   router.go               — Dependencies, SetupRouter, health, CORS
//   search_handler.go       — POST /api/v1/search/{text,image,hybrid,video}
//   alerts_handler.go       — GET/PUT /api/v1/alerts
//   cameras_handler.go      — GET/POST/PUT/DELETE /api/v1/cameras
//   knowledge_handler.go    — /api/v1/rules, /targets, /prompts (CRUD)
//   nodes_handler.go        — GET /api/v1/nodes (proxy Gateway API)
//   analytics_handler.go    — GET /api/v1/analytics/{traffic,heatmap,dwell}
//   auth_handler.go         — POST /api/v1/auth/login, /tokens
//   config_handler.go       — [§5.10] GET/POST /api/v1/config/algorithm
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/analytics"
	"github.com/rivision/rivision-hub/internal/auth"
	"github.com/rivision/rivision-hub/internal/cameras"
	"github.com/rivision/rivision-hub/internal/nodes"
	"github.com/rivision/rivision-hub/internal/scheduler"
	"github.com/rivision/rivision-hub/internal/search"
)

// Dependencies bundles all components used by API handlers.
type Dependencies struct {
	StartTime      time.Time
	Search         *search.FederatedSearch
	AlertEngine    interface{} // *alerts.Engine — avoid circular import
	AnalyticsEng   *analytics.Engine
	CameraStore    *cameras.Store
	Scheduler      *scheduler.Scheduler
	AuthHandler    *auth.Handler
	GatewayClient  *nodes.GatewayClient
	GetNodes       func() []search.NodeInfo
}

// SetupRouter registers all v1 API routes on the Gin engine.
// This is the v6-compliant entry point (§5.7). The existing main.go may
// still register routes directly during the transition period.
func SetupRouter(engine *gin.Engine, deps *Dependencies) {
	v1 := engine.Group("/api/v1")
	{
		// Health & version
		v1.GET("/system/health", handleHealth(deps))
		v1.GET("/system/version", handleVersion)
	}
}

func handleHealth(deps *Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		uptimeS := int64(time.Since(deps.StartTime).Seconds())
		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"service":  "rivision-hub",
			"version":  "2.0.0",
			"uptime_s": uptimeS,
			"time":     time.Now().Format(time.RFC3339),
		})
	}
}

func handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    "2.0.0",
		"build_date": "2025-05-04",
		"go_version": "1.24",
	})
}

// CORSMiddleware returns a permissive CORS middleware (§5.7).
func CORSMiddleware() gin.HandlerFunc {
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
