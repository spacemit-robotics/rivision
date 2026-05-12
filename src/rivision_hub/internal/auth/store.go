// Package auth 提供用户认证授权
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User 用户
type User struct {
	ID           string    `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Email        string    `json:"email,omitempty" db:"email"`
	Role         string    `json:"role" db:"role"` // admin, operator, viewer
	Enabled      bool      `json:"enabled" db:"enabled"`
	LastLogin    time.Time `json:"last_login,omitempty" db:"last_login"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Session 会话
type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// Store 用户存储
type Store struct {
	db       *sql.DB
	users    map[string]*User
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewStore 创建存储
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{
		db:       db,
		users:    make(map[string]*User),
		sessions: make(map[string]*Session),
	}

	if db != nil {
		if err := s.initTables(); err != nil {
			return nil, err
		}
		if err := s.loadFromDB(); err != nil {
			log.Printf("[AuthStore] 从数据库加载失败: %v", err)
		}
	}

	// 确保有默认管理员
	s.ensureDefaultAdmin()

	return s, nil
}

// initTables 初始化表
func (s *Store) initTables() error {
	// R4: SQLite-compatible DDL.
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			email TEXT,
			role TEXT DEFAULT 'viewer',
			enabled INTEGER DEFAULT 1,
			last_login TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT,
			username TEXT,
			action TEXT NOT NULL,
			resource TEXT,
			detail TEXT,
			ip_address TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(created_at)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("初始化表失败: %w", err)
		}
	}

	return nil
}

// loadFromDB 从数据库加载
func (s *Store) loadFromDB() error {
	rows, err := s.db.Query(`SELECT id, username, password_hash, email, role, enabled, last_login, created_at, updated_at FROM users`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		u := &User{}
		var email sql.NullString
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &email, &u.Role, &u.Enabled, &lastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		if email.Valid {
			u.Email = email.String
		}
		if lastLogin.Valid {
			u.LastLogin = lastLogin.Time
		}
		s.users[u.ID] = u
	}

	log.Printf("[AuthStore] 加载 %d 个用户", len(s.users))
	return nil
}

// ensureDefaultAdmin 确保有默认管理员
func (s *Store) ensureDefaultAdmin() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否有管理员
	hasAdmin := false
	for _, u := range s.users {
		if u.Role == "admin" {
			hasAdmin = true
			break
		}
	}

	if !hasAdmin {
		// 创建默认管理员
		admin := &User{
			ID:        "admin",
			Username:  "admin",
			Role:      "admin",
			Enabled:   true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		admin.PasswordHash, _ = hashPassword("admin123")

		if s.db != nil {
			s.db.Exec(`INSERT OR IGNORE INTO users (id, username, password_hash, role, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
				admin.ID, admin.Username, admin.PasswordHash, admin.Role, admin.Enabled, admin.CreatedAt.Format(time.RFC3339), admin.UpdatedAt.Format(time.RFC3339))
		}

		s.users[admin.ID] = admin
		log.Printf("[AuthStore] 创建默认管理员: admin/admin123")
	}
}

// CreateUser 创建用户
func (s *Store) CreateUser(u *User, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查用户名是否存在
	for _, existing := range s.users {
		if existing.Username == u.Username {
			return fmt.Errorf("用户名已存在")
		}
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	u.PasswordHash = hash
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	if u.Role == "" {
		u.Role = "viewer"
	}

	if s.db != nil {
		_, err := s.db.Exec(`INSERT INTO users (id, username, password_hash, email, role, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			u.ID, u.Username, u.PasswordHash, nullString(u.Email), u.Role, u.Enabled, u.CreatedAt.Format(time.RFC3339), u.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return err
		}
	}

	s.users[u.ID] = u
	log.Printf("[AuthStore] 创建用户: %s", u.Username)
	return nil
}

// UpdateUser 更新用户
func (s *Store) UpdateUser(u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[u.ID]; !exists {
		return fmt.Errorf("用户不存在")
	}

	u.UpdatedAt = time.Now()

	if s.db != nil {
		_, err := s.db.Exec(`UPDATE users SET username=?, email=?, role=?, enabled=?, updated_at=? WHERE id=?`,
			u.Username, nullString(u.Email), u.Role, u.Enabled, u.UpdatedAt.Format(time.RFC3339), u.ID)
		if err != nil {
			return err
		}
	}

	s.users[u.ID] = u
	return nil
}

// UpdatePassword 更新密码
func (s *Store) UpdatePassword(userID, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, exists := s.users[userID]
	if !exists {
		return fmt.Errorf("用户不存在")
	}

	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.UpdatedAt = time.Now()

	if s.db != nil {
		_, err := s.db.Exec(`UPDATE users SET password_hash=?, updated_at=? WHERE id=?`,
			hash, user.UpdatedAt.Format(time.RFC3339), userID)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteUser 删除用户
func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		if _, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
			return err
		}
	}

	delete(s.users, id)
	return nil
}

// GetUser 获取用户
func (s *Store) GetUser(id string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	return u, ok
}

// GetUserByUsername 通过用户名获取
func (s *Store) GetUserByUsername(username string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if u.Username == username {
			return u, true
		}
	}
	return nil, false
}

// ListUsers 列出用户
func (s *Store) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	return users
}

// Authenticate 验证用户
func (s *Store) Authenticate(username, password string) (*User, error) {
	user, ok := s.GetUserByUsername(username)
	if !ok {
		return nil, fmt.Errorf("用户不存在")
	}

	if !user.Enabled {
		return nil, fmt.Errorf("用户已禁用")
	}

	if !checkPassword(password, user.PasswordHash) {
		return nil, fmt.Errorf("密码错误")
	}

	// 更新最后登录时间
	user.LastLogin = time.Now()
	if s.db != nil {
		s.db.Exec(`UPDATE users SET last_login=? WHERE id=?`, user.LastLogin.Format(time.RFC3339), user.ID)
	}

	return user, nil
}

// CreateSession 创建会话
func (s *Store) CreateSession(user *User, duration time.Duration) *Session {
	token := generateToken()

	session := &Session{
		Token:     token,
		UserID:    user.ID,
		Username:  user.Username,
		Role:      user.Role,
		ExpiresAt: time.Now().Add(duration),
		CreatedAt: time.Now(),
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()

	return session
}

// ValidateSession 验证会话
func (s *Store) ValidateSession(token string) (*Session, bool) {
	s.mu.RLock()
	session, ok := s.sessions[token]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(session.ExpiresAt) {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
		return nil, false
	}

	return session, true
}

// DeleteSession 删除会话
func (s *Store) DeleteSession(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// LogAudit 记录审计日志
func (s *Store) LogAudit(userID, username, action, resource, detail, ip string) {
	if s.db == nil {
		return
	}

	s.db.Exec(`INSERT INTO audit_logs (user_id, username, action, resource, detail, ip_address) VALUES (?,?,?,?,?,?)`,
		userID, username, action, resource, detail, ip)
}

// GetAuditLogs 获取审计日志
func (s *Store) GetAuditLogs(limit int) ([]map[string]interface{}, error) {
	if s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`SELECT user_id, username, action, resource, detail, ip_address, created_at FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var userID, username, action, resource, detail, ip sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&userID, &username, &action, &resource, &detail, &ip, &createdAt); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"user_id":    userID.String,
			"username":   username.String,
			"action":     action.String,
			"resource":   resource.String,
			"detail":     detail.String,
			"ip_address": ip.String,
			"created_at": createdAt,
		})
	}
	return logs, nil
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func checkPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:])
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
