// Package auth RBAC权限控制
package auth

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// Permission 权限定义
type Permission string

const (
	// 系统管理权限
	PermSystemAdmin   Permission = "system:admin"
	PermSystemConfig  Permission = "system:config"
	PermSystemAudit   Permission = "system:audit"

	// 用户管理权限
	PermUserRead   Permission = "user:read"
	PermUserWrite  Permission = "user:write"
	PermUserDelete Permission = "user:delete"

	// 节点管理权限
	PermNodeRead   Permission = "node:read"
	PermNodeWrite  Permission = "node:write"
	PermNodeDelete Permission = "node:delete"

	// 摄像头管理权限
	PermCameraRead   Permission = "camera:read"
	PermCameraWrite  Permission = "camera:write"
	PermCameraDelete Permission = "camera:delete"
	PermCameraStream Permission = "camera:stream"

	// 规则管理权限
	PermRuleRead   Permission = "rule:read"
	PermRuleWrite  Permission = "rule:write"
	PermRuleDelete Permission = "rule:delete"

	// 告警管理权限
	PermAlertRead  Permission = "alert:read"
	PermAlertWrite Permission = "alert:write"
	PermAlertAck   Permission = "alert:ack"

	// 搜索权限
	PermSearchText  Permission = "search:text"
	PermSearchImage Permission = "search:image"

	// 分析权限
	PermAnalyticsRead   Permission = "analytics:read"
	PermAnalyticsExport Permission = "analytics:export"

	// 知识库权限
	PermKnowledgeRead  Permission = "knowledge:read"
	PermKnowledgeWrite Permission = "knowledge:write"
)

// Role 角色定义
type Role struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
	IsSystem    bool         `json:"is_system"` // 系统角色不可删除
}

// 预定义角色
var (
	RoleAdmin = Role{
		Name:        "admin",
		DisplayName: "系统管理员",
		Description: "拥有所有权限",
		Permissions: []Permission{
			PermSystemAdmin, PermSystemConfig, PermSystemAudit,
			PermUserRead, PermUserWrite, PermUserDelete,
			PermNodeRead, PermNodeWrite, PermNodeDelete,
			PermCameraRead, PermCameraWrite, PermCameraDelete, PermCameraStream,
			PermRuleRead, PermRuleWrite, PermRuleDelete,
			PermAlertRead, PermAlertWrite, PermAlertAck,
			PermSearchText, PermSearchImage,
			PermAnalyticsRead, PermAnalyticsExport,
			PermKnowledgeRead, PermKnowledgeWrite,
		},
		IsSystem: true,
	}

	RoleOperator = Role{
		Name:        "operator",
		DisplayName: "运维人员",
		Description: "负责系统运维，可管理节点和摄像头",
		Permissions: []Permission{
			PermNodeRead, PermNodeWrite,
			PermCameraRead, PermCameraWrite, PermCameraStream,
			PermRuleRead, PermRuleWrite,
			PermAlertRead, PermAlertAck,
			PermSearchText, PermSearchImage,
			PermAnalyticsRead,
			PermKnowledgeRead,
		},
		IsSystem: true,
	}

	RoleAnalyst = Role{
		Name:        "analyst",
		DisplayName: "分析师",
		Description: "负责数据分析，可搜索和导出",
		Permissions: []Permission{
			PermCameraRead, PermCameraStream,
			PermAlertRead,
			PermSearchText, PermSearchImage,
			PermAnalyticsRead, PermAnalyticsExport,
		},
		IsSystem: true,
	}

	RoleViewer = Role{
		Name:        "viewer",
		DisplayName: "只读用户",
		Description: "只能查看，无修改权限",
		Permissions: []Permission{
			PermNodeRead,
			PermCameraRead, PermCameraStream,
			PermRuleRead,
			PermAlertRead,
			PermSearchText,
			PermAnalyticsRead,
			PermKnowledgeRead,
		},
		IsSystem: true,
	}
)

// RBAC 权限管理器
type RBAC struct {
	mu    sync.RWMutex
	roles map[string]*Role
}

// NewRBAC 创建RBAC管理器
func NewRBAC() *RBAC {
	rbac := &RBAC{
		roles: make(map[string]*Role),
	}
	// 注册预定义角色
	rbac.RegisterRole(&RoleAdmin)
	rbac.RegisterRole(&RoleOperator)
	rbac.RegisterRole(&RoleAnalyst)
	rbac.RegisterRole(&RoleViewer)
	return rbac
}

// RegisterRole 注册角色
func (r *RBAC) RegisterRole(role *Role) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[role.Name] = role
}

// GetRole 获取角色
func (r *RBAC) GetRole(name string) *Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.roles[name]
}

// GetAllRoles 获取所有角色
func (r *RBAC) GetAllRoles() []*Role {
	r.mu.RLock()
	defer r.mu.RUnlock()
	roles := make([]*Role, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, role)
	}
	return roles
}

// HasPermission 检查角色是否有权限
func (r *RBAC) HasPermission(roleName string, perm Permission) bool {
	role := r.GetRole(roleName)
	if role == nil {
		return false
	}
	for _, p := range role.Permissions {
		if p == perm || p == PermSystemAdmin {
			return true
		}
	}
	return false
}

// HasAnyPermission 检查角色是否有任一权限
func (r *RBAC) HasAnyPermission(roleName string, perms ...Permission) bool {
	for _, perm := range perms {
		if r.HasPermission(roleName, perm) {
			return true
		}
	}
	return false
}

// RequirePermission 创建权限检查中间件
func (h *Handler) RequirePermission(perms ...Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
			c.Abort()
			return
		}

		// admin角色拥有所有权限
		if role == "admin" {
			c.Next()
			return
		}

		rbac := NewRBAC()
		if !rbac.HasAnyPermission(role, perms...) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":       "权限不足",
				"required":    perms,
				"user_role":   role,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// APIPermissionMap API端点权限映射
var APIPermissionMap = map[string][]Permission{
	// 节点管理
	"GET /api/v1/nodes":           {PermNodeRead},
	"POST /api/v1/nodes/register": {PermNodeWrite},
	"DELETE /api/v1/nodes/:id":    {PermNodeDelete},

	// 摄像头管理
	"GET /api/v1/cameras":        {PermCameraRead},
	"POST /api/v1/cameras":       {PermCameraWrite},
	"PUT /api/v1/cameras/:id":    {PermCameraWrite},
	"DELETE /api/v1/cameras/:id": {PermCameraDelete},

	// 规则管理
	"GET /api/v1/rules":        {PermRuleRead},
	"POST /api/v1/rules":       {PermRuleWrite},
	"PUT /api/v1/rules/:id":    {PermRuleWrite},
	"DELETE /api/v1/rules/:id": {PermRuleDelete},

	// 告警管理
	"GET /api/v1/alerts":         {PermAlertRead},
	"POST /api/v1/alerts/:id/ack": {PermAlertAck},

	// 搜索
	"POST /api/v1/search":       {PermSearchText},
	"POST /api/v1/search/image": {PermSearchImage},

	// 分析
	"GET /api/v1/analytics":        {PermAnalyticsRead},
	"GET /api/v1/analytics/export": {PermAnalyticsExport},

	// 用户管理
	"GET /api/v1/users":        {PermUserRead},
	"POST /api/v1/users":       {PermUserWrite},
	"PUT /api/v1/users/:id":    {PermUserWrite},
	"DELETE /api/v1/users/:id": {PermUserDelete},

	// 系统管理
	"GET /api/v1/system/config":  {PermSystemConfig},
	"PUT /api/v1/system/config":  {PermSystemConfig},
	"GET /api/v1/system/audit":   {PermSystemAudit},
}

// AutoPermissionMiddleware 自动权限检查中间件 (用于Gin框架)
func AutoPermissionMiddleware(rbac *RBAC) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !CheckPermission(rbac, c) {
			return
		}
		c.Next()
	}
}

// CheckPermission 检查权限，返回是否通过 (用于手动调用)
func CheckPermission(rbac *RBAC, c *gin.Context) bool {
	// 公开API不需要权限
	path := c.Request.URL.Path
	if isPublicAPI(path) {
		return true
	}

	role := c.GetString("role")
	if role == "" {
		return true // 让AuthRequired处理
	}

	// admin角色跳过检查
	if role == "admin" {
		return true
	}

	// 查找API权限要求
	key := c.Request.Method + " " + normalizeAPIPath(path)
	perms, exists := APIPermissionMap[key]
	if !exists {
		// 未定义的API，默认允许通过（由其他中间件控制）
		return true
	}

	// 检查权限
	if !rbac.HasAnyPermission(role, perms...) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":     "权限不足",
			"required":  perms,
			"user_role": role,
		})
		c.Abort()
		return false
	}

	return true
}

// isPublicAPI 检查是否为公开API
func isPublicAPI(path string) bool {
	publicPaths := []string{
		"/health",
		"/ready",
		"/api/v1/auth/login",
		"/ws",
	}
	for _, p := range publicPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// normalizeAPIPath 标准化API路径（将参数替换为:param）
func normalizeAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		// 如果是UUID或数字ID，替换为:id
		if len(part) > 0 && (isUUID(part) || isNumericID(part)) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func isUUID(s string) bool {
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	if len(s) == 32 {
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	return false
}

func isNumericID(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
