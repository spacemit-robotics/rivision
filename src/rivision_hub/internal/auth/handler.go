// Package auth 提供认证 API 处理器
package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler 认证处理器
type Handler struct {
	store          *Store
	sessionTimeout time.Duration
}

// NewHandler 创建处理器
func NewHandler(store *Store) *Handler {
	return &Handler{
		store:          store,
		sessionTimeout: 24 * time.Hour,
	}
}

// ValidateToken 验证令牌并返回会话信息
func (h *Handler) ValidateToken(token string) (*Session, bool) {
	return h.store.ValidateSession(token)
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes(r *gin.RouterGroup) {
	auth := r.Group("/auth")
	{
		auth.POST("/login", h.Login)
		auth.POST("/logout", h.Logout)
		auth.GET("/me", h.AuthRequired(), h.Me)
		auth.PUT("/password", h.AuthRequired(), h.ChangePassword)
	}

	users := r.Group("/users")
	users.Use(h.AuthRequired(), h.AdminRequired())
	{
		users.GET("", h.ListUsers)
		users.POST("", h.CreateUser)
		users.GET("/:id", h.GetUser)
		users.PUT("/:id", h.UpdateUser)
		users.DELETE("/:id", h.DeleteUser)
		users.PUT("/:id/password", h.ResetPassword)
	}

	system := r.Group("/system")
	system.Use(h.AuthRequired(), h.AdminRequired())
	{
		system.GET("/audit", h.AuditLogs)
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 登录
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	session := h.store.CreateSession(user, h.sessionTimeout)

	// 记录审计日志
	h.store.LogAudit(user.ID, user.Username, "login", "", "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{
		"token":      session.Token,
		"user":       user,
		"expires_at": session.ExpiresAt,
	})
}

// Logout 登出
func (h *Handler) Logout(c *gin.Context) {
	token := extractToken(c)
	if token != "" {
		if session, ok := h.store.ValidateSession(token); ok {
			h.store.LogAudit(session.UserID, session.Username, "logout", "", "", c.ClientIP())
		}
		h.store.DeleteSession(token)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Me 当前用户
func (h *Handler) Me(c *gin.Context) {
	session := c.MustGet("session").(*Session)
	user, _ := h.store.GetUser(session.UserID)
	c.JSON(http.StatusOK, user)
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword 修改密码
func (h *Handler) ChangePassword(c *gin.Context) {
	session := c.MustGet("session").(*Session)

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证旧密码
	_, err := h.store.Authenticate(session.Username, req.OldPassword)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "旧密码错误"})
		return
	}

	if err := h.store.UpdatePassword(session.UserID, req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.store.LogAudit(session.UserID, session.Username, "change_password", "", "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListUsers 列出用户
func (h *Handler) ListUsers(c *gin.Context) {
	users := h.store.ListUsers()
	c.JSON(http.StatusOK, gin.H{"users": users})
}

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// CreateUser 创建用户
func (h *Handler) CreateUser(c *gin.Context) {
	session := c.MustGet("session").(*Session)

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := &User{
		ID:       uuid.New().String()[:8],
		Username: req.Username,
		Email:    req.Email,
		Role:     req.Role,
		Enabled:  true,
	}

	if err := h.store.CreateUser(user, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.store.LogAudit(session.UserID, session.Username, "create_user", user.ID, user.Username, c.ClientIP())

	c.JSON(http.StatusCreated, user)
}

// GetUser 获取用户
func (h *Handler) GetUser(c *gin.Context) {
	id := c.Param("id")
	user, ok := h.store.GetUser(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Enabled  *bool  `json:"enabled"`
}

// UpdateUser 更新用户
func (h *Handler) UpdateUser(c *gin.Context) {
	session := c.MustGet("session").(*Session)
	id := c.Param("id")

	user, ok := h.store.GetUser(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Enabled != nil {
		user.Enabled = *req.Enabled
	}

	if err := h.store.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.store.LogAudit(session.UserID, session.Username, "update_user", user.ID, user.Username, c.ClientIP())

	c.JSON(http.StatusOK, user)
}

// DeleteUser 删除用户
func (h *Handler) DeleteUser(c *gin.Context) {
	session := c.MustGet("session").(*Session)
	id := c.Param("id")

	if id == session.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能删除自己"})
		return
	}

	user, ok := h.store.GetUser(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if err := h.store.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.store.LogAudit(session.UserID, session.Username, "delete_user", user.ID, user.Username, c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ResetPassword 重置密码
func (h *Handler) ResetPassword(c *gin.Context) {
	session := c.MustGet("session").(*Session)
	id := c.Param("id")

	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.UpdatePassword(id, req.NewPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.store.LogAudit(session.UserID, session.Username, "reset_password", id, "", c.ClientIP())

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AuditLogs 审计日志
func (h *Handler) AuditLogs(c *gin.Context) {
	logs, err := h.store.GetAuditLogs(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// AuthRequired 认证中间件
func (h *Handler) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证令牌"})
			c.Abort()
			return
		}

		session, ok := h.store.ValidateSession(token)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效或过期的令牌"})
			c.Abort()
			return
		}

		c.Set("session", session)
		c.Set("user_id", session.UserID)
		c.Set("username", session.Username)
		c.Set("role", session.Role)
		c.Next()
	}
}

// AdminRequired 管理员权限中间件
func (h *Handler) AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// extractToken 从请求中提取 token
func extractToken(c *gin.Context) string {
	// 从 Authorization header
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// 从 query parameter
	if token := c.Query("token"); token != "" {
		return token
	}

	// 从 cookie
	if token, err := c.Cookie("token"); err == nil {
		return token
	}

	return ""
}
