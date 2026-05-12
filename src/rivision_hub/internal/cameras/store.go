// Package cameras 提供摄像头管理
package cameras

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// Camera 摄像头
type Camera struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	URL         string                 `json:"url" db:"url"`
	Protocol    string                 `json:"protocol" db:"protocol"` // rtsp, onvif, gb28181
	GroupID     string                 `json:"group_id,omitempty" db:"group_id"`
	NodeID      string                 `json:"node_id,omitempty" db:"node_id"`
	Status      string                 `json:"status" db:"status"` // online, offline, error
	Enabled     bool                   `json:"enabled" db:"enabled"`
	Location    string                 `json:"location,omitempty" db:"location"`
	Description string                 `json:"description,omitempty" db:"description"`
	Config      map[string]interface{} `json:"config,omitempty" db:"-"`
	ConfigJSON  string                 `json:"-" db:"config_json"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`

	// 运行时状态
	LastSeen    time.Time `json:"last_seen,omitempty"`
	StreamInfo  *StreamInfo `json:"stream_info,omitempty"`
}

// StreamInfo 流信息
type StreamInfo struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FrameRate float64 `json:"frame_rate"`
	Codec     string  `json:"codec"`
}

// Group 摄像头分组
type Group struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	ParentID    string    `json:"parent_id,omitempty" db:"parent_id"`
	Description string    `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// Store 摄像头存储
type Store struct {
	db      *sql.DB
	cameras map[string]*Camera
	groups  map[string]*Group
	mu      sync.RWMutex
}

// NewStore 创建存储
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{
		db:      db,
		cameras: make(map[string]*Camera),
		groups:  make(map[string]*Group),
	}

	if db != nil {
		if err := s.initTables(); err != nil {
			return nil, err
		}
		if err := s.loadFromDB(); err != nil {
			log.Printf("[CameraStore] 从数据库加载失败: %v", err)
		}
	}

	return s, nil
}

// initTables 初始化表
func (s *Store) initTables() error {
	// R4: SQLite-compatible DDL.
	queries := []string{
		`CREATE TABLE IF NOT EXISTS cameras (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			protocol TEXT DEFAULT 'rtsp',
			group_id TEXT,
			node_id TEXT,
			status TEXT DEFAULT 'offline',
			enabled INTEGER DEFAULT 1,
			location TEXT,
			description TEXT,
			config_json TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS camera_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			parent_id TEXT,
			description TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cameras_group ON cameras(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cameras_node ON cameras(node_id)`,
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
	// 加载分组
	rows, err := s.db.Query(`SELECT id, name, parent_id, description, created_at FROM camera_groups`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		g := &Group{}
		var parentID sql.NullString
		if err := rows.Scan(&g.ID, &g.Name, &parentID, &g.Description, &g.CreatedAt); err != nil {
			continue
		}
		if parentID.Valid {
			g.ParentID = parentID.String
		}
		s.groups[g.ID] = g
	}

	// 加载摄像头
	rows2, err := s.db.Query(`SELECT id, name, url, protocol, group_id, node_id, status, enabled, location, description, config_json, created_at, updated_at FROM cameras`)
	if err != nil {
		return err
	}
	defer rows2.Close()

	for rows2.Next() {
		c := &Camera{}
		var groupID, nodeID, location, description, configJSON sql.NullString
		if err := rows2.Scan(&c.ID, &c.Name, &c.URL, &c.Protocol, &groupID, &nodeID, &c.Status, &c.Enabled, &location, &description, &configJSON, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		if groupID.Valid {
			c.GroupID = groupID.String
		}
		if nodeID.Valid {
			c.NodeID = nodeID.String
		}
		if location.Valid {
			c.Location = location.String
		}
		if description.Valid {
			c.Description = description.String
		}
		if configJSON.Valid && configJSON.String != "" {
			json.Unmarshal([]byte(configJSON.String), &c.Config)
		}
		s.cameras[c.ID] = c
	}

	log.Printf("[CameraStore] 加载 %d 个分组, %d 个摄像头", len(s.groups), len(s.cameras))
	return nil
}

// CreateCamera 创建摄像头
func (s *Store) CreateCamera(cam *Camera) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cameras[cam.ID]; exists {
		return fmt.Errorf("摄像头已存在: %s", cam.ID)
	}

	cam.CreatedAt = time.Now()
	cam.UpdatedAt = time.Now()
	if cam.Status == "" {
		cam.Status = "offline"
	}
	if cam.Protocol == "" {
		cam.Protocol = "rtsp"
	}

	if s.db != nil {
		configJSON := ""
		if cam.Config != nil {
			data, _ := json.Marshal(cam.Config)
			configJSON = string(data)
		}

		_, err := s.db.Exec(`INSERT INTO cameras (id, name, url, protocol, group_id, node_id, status, enabled, location, description, config_json, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			cam.ID, cam.Name, cam.URL, cam.Protocol, nullString(cam.GroupID), nullString(cam.NodeID), cam.Status, cam.Enabled, nullString(cam.Location), nullString(cam.Description), configJSON, cam.CreatedAt.Format(time.RFC3339), cam.UpdatedAt.Format(time.RFC3339))
		if err != nil {
			return err
		}
	}

	s.cameras[cam.ID] = cam
	log.Printf("[CameraStore] 创建摄像头: %s", cam.ID)
	return nil
}

// UpdateCamera 更新摄像头
func (s *Store) UpdateCamera(cam *Camera) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.cameras[cam.ID]; !exists {
		return fmt.Errorf("摄像头不存在: %s", cam.ID)
	}

	cam.UpdatedAt = time.Now()

	if s.db != nil {
		configJSON := ""
		if cam.Config != nil {
			data, _ := json.Marshal(cam.Config)
			configJSON = string(data)
		}

		_, err := s.db.Exec(`UPDATE cameras SET name=?, url=?, protocol=?, group_id=?, node_id=?, status=?, enabled=?, location=?, description=?, config_json=?, updated_at=? WHERE id=?`,
			cam.Name, cam.URL, cam.Protocol, nullString(cam.GroupID), nullString(cam.NodeID), cam.Status, cam.Enabled, nullString(cam.Location), nullString(cam.Description), configJSON, cam.UpdatedAt.Format(time.RFC3339), cam.ID)
		if err != nil {
			return err
		}
	}

	s.cameras[cam.ID] = cam
	return nil
}

// DeleteCamera 删除摄像头
func (s *Store) DeleteCamera(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		if _, err := s.db.Exec(`DELETE FROM cameras WHERE id=?`, id); err != nil {
			return err
		}
	}

	delete(s.cameras, id)
	log.Printf("[CameraStore] 删除摄像头: %s", id)
	return nil
}

// GetCamera 获取摄像头
func (s *Store) GetCamera(id string) (*Camera, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cam, ok := s.cameras[id]
	return cam, ok
}

// ListCameras 列出摄像头
func (s *Store) ListCameras(filter CameraFilter) []*Camera {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Camera, 0)
	for _, cam := range s.cameras {
		if filter.GroupID != "" && cam.GroupID != filter.GroupID {
			continue
		}
		if filter.NodeID != "" && cam.NodeID != filter.NodeID {
			continue
		}
		if filter.Status != "" && cam.Status != filter.Status {
			continue
		}
		if filter.EnabledOnly && !cam.Enabled {
			continue
		}
		result = append(result, cam)
	}
	return result
}

// CameraFilter 过滤条件
type CameraFilter struct {
	GroupID     string
	NodeID      string
	Status      string
	EnabledOnly bool
}

// CountByNode 统计每个节点的摄像头数量
func (s *Store) CountByNode() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int)
	for _, cam := range s.cameras {
		if cam.NodeID != "" {
			counts[cam.NodeID]++
		}
	}
	return counts
}

// AssignToNode 分配到节点
func (s *Store) AssignToNode(cameraID, nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cam, exists := s.cameras[cameraID]
	if !exists {
		return fmt.Errorf("摄像头不存在: %s", cameraID)
	}

	cam.NodeID = nodeID
	cam.UpdatedAt = time.Now()

	if s.db != nil {
		_, err := s.db.Exec(`UPDATE cameras SET node_id=?, updated_at=? WHERE id=?`, nullString(nodeID), cam.UpdatedAt.Format(time.RFC3339), cameraID)
		if err != nil {
			return err
		}
	}

	log.Printf("[CameraStore] 分配摄像头 %s 到节点 %s", cameraID, nodeID)
	return nil
}

// UpdateStatus 更新状态
func (s *Store) UpdateStatus(cameraID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cam, exists := s.cameras[cameraID]
	if !exists {
		return fmt.Errorf("摄像头不存在: %s", cameraID)
	}

	cam.Status = status
	cam.LastSeen = time.Now()

	if s.db != nil {
		_, err := s.db.Exec(`UPDATE cameras SET status=? WHERE id=?`, status, cameraID)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateConfig 更新摄像头配置
func (s *Store) UpdateConfig(cameraID string, config map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cam, exists := s.cameras[cameraID]
	if !exists {
		return fmt.Errorf("摄像头不存在: %s", cameraID)
	}

	cam.Config = config
	cam.UpdatedAt = time.Now()

	if s.db != nil {
		configJSON, err := json.Marshal(config)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`UPDATE cameras SET config_json=?, updated_at=? WHERE id=?`,
			string(configJSON), cam.UpdatedAt.Format(time.RFC3339), cameraID)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateGroup 创建分组
func (s *Store) CreateGroup(g *Group) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.groups[g.ID]; exists {
		return fmt.Errorf("分组已存在: %s", g.ID)
	}

	g.CreatedAt = time.Now()

	if s.db != nil {
		_, err := s.db.Exec(`INSERT INTO camera_groups (id, name, parent_id, description, created_at) VALUES (?,?,?,?,?)`,
			g.ID, g.Name, nullString(g.ParentID), nullString(g.Description), g.CreatedAt.Format(time.RFC3339))
		if err != nil {
			return err
		}
	}

	s.groups[g.ID] = g
	return nil
}

// DeleteGroup 删除分组
func (s *Store) DeleteGroup(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否有摄像头在该分组
	for _, cam := range s.cameras {
		if cam.GroupID == id {
			return fmt.Errorf("分组下有摄像头，无法删除")
		}
	}

	if s.db != nil {
		if _, err := s.db.Exec(`DELETE FROM camera_groups WHERE id=?`, id); err != nil {
			return err
		}
	}

	delete(s.groups, id)
	return nil
}

// ListGroups 列出分组
func (s *Store) ListGroups() []*Group {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Group, 0, len(s.groups))
	for _, g := range s.groups {
		result = append(result, g)
	}
	return result
}

// GetStats 获取统计
func (s *Store) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	online := 0
	offline := 0
	byNode := make(map[string]int)

	for _, cam := range s.cameras {
		if cam.Status == "online" {
			online++
		} else {
			offline++
		}
		if cam.NodeID != "" {
			byNode[cam.NodeID]++
		}
	}

	return map[string]interface{}{
		"total":   len(s.cameras),
		"online":  online,
		"offline": offline,
		"groups":  len(s.groups),
		"by_node": byNode,
	}
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
