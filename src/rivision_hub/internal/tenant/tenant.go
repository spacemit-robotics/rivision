// Package tenant 多租户支持
package tenant

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// Tenant 租户
type Tenant struct {
	ID          string            `json:"id" db:"id"`
	Name        string            `json:"name" db:"name"`
	DisplayName string            `json:"display_name" db:"display_name"`
	Domain      string            `json:"domain,omitempty" db:"domain"` // 租户域名
	Quota       *TenantQuota      `json:"quota,omitempty"`
	Settings    map[string]string `json:"settings,omitempty"`
	Enabled     bool              `json:"enabled" db:"enabled"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// TenantQuota 租户配额
type TenantQuota struct {
	MaxCameras     int   `json:"max_cameras"`
	MaxNodes       int   `json:"max_nodes"`
	MaxUsers       int   `json:"max_users"`
	MaxStorage     int64 `json:"max_storage_gb"`    // GB
	MaxApiCalls    int64 `json:"max_api_calls_day"` // 每日API调用
	MaxRetention   int   `json:"max_retention_days"`
	VLMEnabled     bool  `json:"vlm_enabled"`
	SearchEnabled  bool  `json:"search_enabled"`
	AnalyticsLevel int   `json:"analytics_level"` // 0=基础, 1=标准, 2=高级
}

// TenantUsage 租户使用量
type TenantUsage struct {
	TenantID    string    `json:"tenant_id"`
	Cameras     int       `json:"cameras"`
	Nodes       int       `json:"nodes"`
	Users       int       `json:"users"`
	StorageUsed int64     `json:"storage_used_gb"`
	ApiCalls    int64     `json:"api_calls_today"`
	LastUpdated time.Time `json:"last_updated"`
}

// TenantManager 租户管理器
type TenantManager struct {
	db      *sql.DB
	mu      sync.RWMutex
	tenants map[string]*Tenant
	usages  map[string]*TenantUsage
}

// NewTenantManager 创建租户管理器
func NewTenantManager(db *sql.DB) (*TenantManager, error) {
	tm := &TenantManager{
		db:      db,
		tenants: make(map[string]*Tenant),
		usages:  make(map[string]*TenantUsage),
	}

	if err := tm.initTables(); err != nil {
		return nil, err
	}

	if err := tm.loadTenants(); err != nil {
		log.Printf("[Tenant] 加载租户失败: %v", err)
	}

	// 确保有默认租户
	tm.ensureDefaultTenant()

	log.Printf("[Tenant] 初始化完成: tenants=%d", len(tm.tenants))
	return tm, nil
}

// initTables 初始化表
func (tm *TenantManager) initTables() error {
	// R4: SQLite-compatible DDL.
	queries := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			display_name TEXT,
			domain TEXT,
			quota TEXT,
			settings TEXT,
			enabled INTEGER DEFAULT 1,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_usages (
			tenant_id TEXT PRIMARY KEY,
			cameras INTEGER DEFAULT 0,
			nodes INTEGER DEFAULT 0,
			users INTEGER DEFAULT 0,
			storage_used INTEGER DEFAULT 0,
			api_calls INTEGER DEFAULT 0,
			last_updated TEXT DEFAULT (datetime('now'))
		)`,
	}

	for _, q := range queries {
		if _, err := tm.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// loadTenants 加载租户
func (tm *TenantManager) loadTenants() error {
	rows, err := tm.db.Query(`SELECT id, name, display_name, domain, enabled, created_at, updated_at FROM tenants`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		t := &Tenant{}
		if err := rows.Scan(&t.ID, &t.Name, &t.DisplayName, &t.Domain, &t.Enabled, &t.CreatedAt, &t.UpdatedAt); err != nil {
			continue
		}
		tm.tenants[t.ID] = t
	}

	return nil
}

// ensureDefaultTenant 确保默认租户存在
func (tm *TenantManager) ensureDefaultTenant() {
	if _, ok := tm.tenants["default"]; ok {
		return
	}

	defaultTenant := &Tenant{
		ID:          "default",
		Name:        "default",
		DisplayName: "默认租户",
		Quota: &TenantQuota{
			MaxCameras:     100,
			MaxNodes:       10,
			MaxUsers:       50,
			MaxStorage:     1000,
			MaxApiCalls:    100000,
			MaxRetention:   90,
			VLMEnabled:     true,
			SearchEnabled:  true,
			AnalyticsLevel: 2,
		},
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	tm.CreateTenant(defaultTenant)
}

// CreateTenant 创建租户
func (tm *TenantManager) CreateTenant(t *Tenant) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t.ID == "" {
		t.ID = fmt.Sprintf("tenant_%d", time.Now().UnixNano())
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()

	_, err := tm.db.Exec(`INSERT INTO tenants (id, name, display_name, domain, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.DisplayName, t.Domain, t.Enabled, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return err
	}

	tm.tenants[t.ID] = t
	log.Printf("[Tenant] 创建租户: %s (%s)", t.ID, t.Name)
	return nil
}

// GetTenant 获取租户
func (tm *TenantManager) GetTenant(id string) (*Tenant, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tenants[id]
	return t, ok
}

// GetTenantByName 通过名称获取租户
func (tm *TenantManager) GetTenantByName(name string) (*Tenant, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	for _, t := range tm.tenants {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// ListTenants 列出租户
func (tm *TenantManager) ListTenants() []*Tenant {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	result := make([]*Tenant, 0, len(tm.tenants))
	for _, t := range tm.tenants {
		result = append(result, t)
	}
	return result
}

// UpdateTenant 更新租户
func (tm *TenantManager) UpdateTenant(t *Tenant) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	t.UpdatedAt = time.Now()
	_, err := tm.db.Exec(`UPDATE tenants SET display_name=?, domain=?, enabled=?, updated_at=? WHERE id=?`,
		t.DisplayName, t.Domain, t.Enabled, t.UpdatedAt, t.ID)
	if err != nil {
		return err
	}

	tm.tenants[t.ID] = t
	return nil
}

// DeleteTenant 删除租户
func (tm *TenantManager) DeleteTenant(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if id == "default" {
		return fmt.Errorf("cannot delete default tenant")
	}

	tm.db.Exec(`DELETE FROM tenant_usages WHERE tenant_id = ?`, id)
	tm.db.Exec(`DELETE FROM tenants WHERE id = ?`, id)

	delete(tm.tenants, id)
	delete(tm.usages, id)

	return nil
}

// CheckQuota 检查配额
func (tm *TenantManager) CheckQuota(tenantID string, resource string) error {
	t, ok := tm.GetTenant(tenantID)
	if !ok {
		return fmt.Errorf("tenant not found")
	}

	if !t.Enabled {
		return fmt.Errorf("tenant disabled")
	}

	if t.Quota == nil {
		return nil // 无配额限制
	}

	usage, _ := tm.GetUsage(tenantID)
	if usage == nil {
		return nil
	}

	switch resource {
	case "camera":
		if usage.Cameras >= t.Quota.MaxCameras {
			return fmt.Errorf("camera quota exceeded: %d/%d", usage.Cameras, t.Quota.MaxCameras)
		}
	case "node":
		if usage.Nodes >= t.Quota.MaxNodes {
			return fmt.Errorf("node quota exceeded: %d/%d", usage.Nodes, t.Quota.MaxNodes)
		}
	case "user":
		if usage.Users >= t.Quota.MaxUsers {
			return fmt.Errorf("user quota exceeded: %d/%d", usage.Users, t.Quota.MaxUsers)
		}
	case "storage":
		if usage.StorageUsed >= t.Quota.MaxStorage {
			return fmt.Errorf("storage quota exceeded: %d/%d GB", usage.StorageUsed, t.Quota.MaxStorage)
		}
	case "api":
		if usage.ApiCalls >= t.Quota.MaxApiCalls {
			return fmt.Errorf("API call quota exceeded: %d/%d", usage.ApiCalls, t.Quota.MaxApiCalls)
		}
	}

	return nil
}

// GetUsage 获取使用量
func (tm *TenantManager) GetUsage(tenantID string) (*TenantUsage, error) {
	tm.mu.RLock()
	u, ok := tm.usages[tenantID]
	tm.mu.RUnlock()

	if ok {
		return u, nil
	}

	// 从数据库加载
	u = &TenantUsage{TenantID: tenantID}
	err := tm.db.QueryRow(`SELECT cameras, nodes, users, storage_used, api_calls, last_updated FROM tenant_usages WHERE tenant_id = ?`, tenantID).
		Scan(&u.Cameras, &u.Nodes, &u.Users, &u.StorageUsed, &u.ApiCalls, &u.LastUpdated)
	if err != nil {
		return nil, err
	}

	tm.mu.Lock()
	tm.usages[tenantID] = u
	tm.mu.Unlock()

	return u, nil
}

// IncrementUsage 增加使用量
func (tm *TenantManager) IncrementUsage(tenantID, resource string, delta int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	var col string
	switch resource {
	case "camera":
		col = "cameras"
	case "node":
		col = "nodes"
	case "user":
		col = "users"
	case "api":
		col = "api_calls"
	default:
		return fmt.Errorf("unknown resource: %s", resource)
	}

	query := fmt.Sprintf(`INSERT INTO tenant_usages (tenant_id, %s, last_updated) VALUES (?, ?, ?) 
		ON CONFLICT(tenant_id) DO UPDATE SET %s = %s + ?, last_updated = ?`, col, col, col)
	
	now := time.Now()
	_, err := tm.db.Exec(query, tenantID, delta, now, delta, now)
	
	// 更新缓存
	if u, ok := tm.usages[tenantID]; ok {
		switch resource {
		case "camera":
			u.Cameras += delta
		case "node":
			u.Nodes += delta
		case "user":
			u.Users += delta
		case "api":
			u.ApiCalls += int64(delta)
		}
		u.LastUpdated = now
	}

	return err
}

// TenantContext 租户上下文
type TenantContext struct {
	TenantID string
	Tenant   *Tenant
}

// FromContext 从context获取租户
func FromContext(ctx context.Context) (*TenantContext, bool) {
	tc, ok := ctx.Value("tenant").(*TenantContext)
	return tc, ok
}

// WithTenant 设置租户上下文
func WithTenant(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, "tenant", &TenantContext{
		TenantID: t.ID,
		Tenant:   t,
	})
}
