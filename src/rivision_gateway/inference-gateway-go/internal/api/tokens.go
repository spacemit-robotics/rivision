// Package api 提供 Token 管理 API
package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/inference-gateway/internal/auth"
)

// GenerateToken 生成 Token
// POST /api/v1/admin/node-tokens/generate
func (h *Handlers) GenerateToken(c *gin.Context) {
	var req struct {
		NodeID        string `json:"node_id" binding:"required"`
		LifetimeHours int    `json:"lifetime_hours"`
		MaxUses       int    `json:"max_uses"`
		Description   string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	// 设置默认值
	if req.LifetimeHours == 0 {
		req.LifetimeHours = 24
	}
	if req.MaxUses == 0 {
		req.MaxUses = 999
	}

	fmt.Printf("[GenerateToken] node_id=%s, lifetime_hours=%d, max_uses=%d\n", req.NodeID, req.LifetimeHours, req.MaxUses)

	authManager := auth.GetNodeAuthManager()
	tokenStr := authManager.GenerateRegistrationToken(
		req.NodeID,
		req.LifetimeHours*3600,
		req.MaxUses,
		req.Description,
	)

	expiresAt := time.Now().Add(time.Duration(req.LifetimeHours) * time.Hour)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Token 生成成功",
		"token":      tokenStr,
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

// ListTokens 获取 Token 列表
// GET /api/v1/admin/node-tokens/list
func (h *Handlers) ListTokens(c *gin.Context) {
	authManager := auth.GetNodeAuthManager()
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

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   tokens,
	})
}

// RevokeToken 撤销 Token
// DELETE /api/v1/admin/node-tokens/:token
func (h *Handlers) RevokeToken(c *gin.Context) {
	tokenStr := c.Param("token")

	authManager := auth.GetNodeAuthManager()
	if !authManager.RevokeToken(tokenStr) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Token 不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Token 已撤销",
	})
}

// RevokeTokenByNodeID 按节点ID撤销Token
// DELETE /api/v1/admin/node-tokens/revoke-by-node/:nodeID
func (h *Handlers) RevokeTokenByNodeID(c *gin.Context) {
	nodeID := c.Param("nodeID")

	authManager := auth.GetNodeAuthManager()
	count := authManager.RevokeTokenByNodeID(nodeID)

	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "该节点没有Token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("已撤销 %d 个Token", count),
		"count":   count,
	})
}

// GetRegistrationStats 获取注册统计
// GET /api/v1/admin/node-tokens/stats
func (h *Handlers) GetRegistrationStats(c *gin.Context) {
	authManager := auth.GetNodeAuthManager()
	stats := authManager.GetStats()

	c.JSON(http.StatusOK, stats)
}

// ManageBlacklist 管理黑名单
// POST /api/v1/admin/node-tokens/blacklist
func (h *Handlers) ManageBlacklist(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Action string `json:"action" binding:"required"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的请求参数",
		})
		return
	}

	authManager := auth.GetNodeAuthManager()

	switch req.Action {
	case "add":
		authManager.AddToBlacklist(req.IP, req.Reason, 0)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "IP 已加入黑名单",
		})
	case "remove":
		authManager.RemoveFromBlacklist(req.IP)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "IP 已从黑名单移除",
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "无效的操作",
		})
	}
}

// GetBlacklist 获取黑名单
// GET /api/v1/admin/node-tokens/blacklist
func (h *Handlers) GetBlacklist(c *gin.Context) {
	authManager := auth.GetNodeAuthManager()
	blacklist := authManager.GetBlacklist()

	entries := make([]gin.H, 0, len(blacklist))
	for _, e := range blacklist {
		entries = append(entries, gin.H{
			"ip":         e.IP,
			"reason":     e.Reason,
			"created_at": e.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    entries,
	})
}

// CleanupTokens 清理过期 Token
// POST /api/v1/admin/node-tokens/cleanup
func (h *Handlers) CleanupTokens(c *gin.Context) {
	authManager := auth.GetNodeAuthManager()
	cleaned := authManager.CleanupExpiredTokens()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清理完成",
		"cleaned": cleaned,
	})
}
