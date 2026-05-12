// Package knowledge 实现知识库管理
package knowledge

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rivision/rivision-hub/pkg/models"
)

// Store 知识库存储
type Store struct {
	mu      sync.RWMutex
	db      *sql.DB
	version int
}

// NewStore 创建知识库存储
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{
		db:      db,
		version: 1,
	}

	if err := s.initSchema(); err != nil {
		return nil, err
	}

	// 加载当前版本
	s.loadVersion()

	return s, nil
}

// initSchema 初始化数据库表
func (s *Store) initSchema() error {
	if s.db == nil {
		return nil
	}

	// R4: SQLite-compatible DDL.
	schema := `
	CREATE TABLE IF NOT EXISTS knowledge_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		priority INTEGER DEFAULT 0,
		conditions TEXT,
		actions TEXT,
		vlm_prompt TEXT,
		camera_ids TEXT,
		schedule TEXT,
		version INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS knowledge_prompts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		scenario TEXT NOT NULL,
		template TEXT NOT NULL,
		variables TEXT,
		description TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS knowledge_targets (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		description TEXT,
		tags TEXT,
		image_url TEXT,
		metadata TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS knowledge_version (
		id INTEGER PRIMARY KEY,
		version INTEGER NOT NULL,
		updated_at TEXT DEFAULT (datetime('now'))
	);

	INSERT OR IGNORE INTO knowledge_version (id, version) VALUES (1, 1);
	`

	_, err := s.db.Exec(schema)
	return err
}

// loadVersion 加载当前版本
func (s *Store) loadVersion() {
	if s.db == nil {
		return
	}
	s.db.QueryRow(`SELECT version FROM knowledge_version WHERE id = 1`).Scan(&s.version)
}

// GetVersion 获取当前版本
func (s *Store) GetVersion() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// incrementVersion 增加版本号
func (s *Store) incrementVersion() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version++
	if s.db != nil {
		s.db.Exec(`UPDATE knowledge_version SET version = ?, updated_at = ? WHERE id = 1`,
			s.version, time.Now().Format(time.RFC3339))
	}
}

// CreateRule 创建规则
func (s *Store) CreateRule(req *models.CreateRuleRequest) (*models.Rule, error) {
	rule := &models.Rule{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Type:        req.Type,
		Enabled:     true,
		Priority:    req.Priority,
		Conditions:  req.Conditions,
		Actions:     req.Actions,
		VLMPrompt:   req.VLMPrompt,
		CameraIDs:   req.CameraIDs,
		Schedule:    req.Schedule,
		Version:     s.GetVersion() + 1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if s.db != nil {
		conditionsJSON, _ := json.Marshal(rule.Conditions)
		actionsJSON, _ := json.Marshal(rule.Actions)
		cameraIDsJSON, _ := json.Marshal(rule.CameraIDs)
		scheduleJSON, _ := json.Marshal(rule.Schedule)

		_, err := s.db.Exec(`
			INSERT INTO knowledge_rules (id, name, description, type, enabled, priority, 
			                             conditions, actions, vlm_prompt, camera_ids, schedule, version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			rule.ID, rule.Name, rule.Description, rule.Type, rule.Enabled, rule.Priority,
			string(conditionsJSON), string(actionsJSON), rule.VLMPrompt, string(cameraIDsJSON), string(scheduleJSON),
			rule.Version, rule.CreatedAt.Format(time.RFC3339), rule.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return nil, fmt.Errorf("创建规则失败: %w", err)
		}
	}

	s.incrementVersion()
	log.Printf("[Knowledge] 创建规则: %s (%s)", rule.Name, rule.ID)
	return rule, nil
}

// UpdateRule 更新规则
func (s *Store) UpdateRule(id string, req *models.UpdateRuleRequest) (*models.Rule, error) {
	rule, err := s.GetRule(id)
	if err != nil {
		return nil, err
	}

	// 应用更新
	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.Description != nil {
		rule.Description = *req.Description
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.Conditions != nil {
		rule.Conditions = req.Conditions
	}
	if req.Actions != nil {
		rule.Actions = req.Actions
	}
	if req.VLMPrompt != nil {
		rule.VLMPrompt = *req.VLMPrompt
	}
	if req.CameraIDs != nil {
		rule.CameraIDs = req.CameraIDs
	}
	if req.Schedule != nil {
		rule.Schedule = req.Schedule
	}
	if req.Priority != nil {
		rule.Priority = *req.Priority
	}
	rule.UpdatedAt = time.Now()
	rule.Version = s.GetVersion() + 1

	if s.db != nil {
		conditionsJSON, _ := json.Marshal(rule.Conditions)
		actionsJSON, _ := json.Marshal(rule.Actions)
		cameraIDsJSON, _ := json.Marshal(rule.CameraIDs)
		scheduleJSON, _ := json.Marshal(rule.Schedule)

		_, err := s.db.Exec(`
			UPDATE knowledge_rules SET name=?, description=?, enabled=?, priority=?,
			       conditions=?, actions=?, vlm_prompt=?, camera_ids=?, schedule=?,
			       version=?, updated_at=?
			WHERE id=?`,
			rule.Name, rule.Description, rule.Enabled, rule.Priority,
			string(conditionsJSON), string(actionsJSON), rule.VLMPrompt, string(cameraIDsJSON), string(scheduleJSON),
			rule.Version, rule.UpdatedAt.Format(time.RFC3339), id,
		)
		if err != nil {
			return nil, fmt.Errorf("更新规则失败: %w", err)
		}
	}

	s.incrementVersion()
	log.Printf("[Knowledge] 更新规则: %s", id)
	return rule, nil
}

// DeleteRule 删除规则
func (s *Store) DeleteRule(id string) error {
	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM knowledge_rules WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("删除规则失败: %w", err)
		}
	}
	s.incrementVersion()
	log.Printf("[Knowledge] 删除规则: %s", id)
	return nil
}

// GetRule 获取单个规则
func (s *Store) GetRule(id string) (*models.Rule, error) {
	if s.db == nil {
		return nil, sql.ErrNoRows
	}

	var rule models.Rule
	var conditionsJSON, actionsJSON, cameraIDsJSON, scheduleJSON []byte

	err := s.db.QueryRow(`
		SELECT id, name, description, type, enabled, priority, conditions, actions,
		       vlm_prompt, camera_ids, schedule, version, created_at, updated_at
		FROM knowledge_rules WHERE id = ?`, id).Scan(
		&rule.ID, &rule.Name, &rule.Description, &rule.Type, &rule.Enabled, &rule.Priority,
		&conditionsJSON, &actionsJSON, &rule.VLMPrompt, &cameraIDsJSON, &scheduleJSON,
		&rule.Version, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(conditionsJSON, &rule.Conditions)
	json.Unmarshal(actionsJSON, &rule.Actions)
	json.Unmarshal(cameraIDsJSON, &rule.CameraIDs)
	json.Unmarshal(scheduleJSON, &rule.Schedule)

	return &rule, nil
}

// GetRules 获取所有规则
func (s *Store) GetRules() ([]models.Rule, error) {
	if s.db == nil {
		return []models.Rule{}, nil
	}

	rows, err := s.db.Query(`
		SELECT id, name, description, type, enabled, priority, conditions, actions,
		       vlm_prompt, camera_ids, schedule, version, created_at, updated_at
		FROM knowledge_rules ORDER BY priority DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []models.Rule
	for rows.Next() {
		var rule models.Rule
		var conditionsJSON, actionsJSON, cameraIDsJSON, scheduleJSON []byte

		err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Description, &rule.Type, &rule.Enabled, &rule.Priority,
			&conditionsJSON, &actionsJSON, &rule.VLMPrompt, &cameraIDsJSON, &scheduleJSON,
			&rule.Version, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			continue
		}

		json.Unmarshal(conditionsJSON, &rule.Conditions)
		json.Unmarshal(actionsJSON, &rule.Actions)
		json.Unmarshal(cameraIDsJSON, &rule.CameraIDs)
		json.Unmarshal(scheduleJSON, &rule.Schedule)

		rules = append(rules, rule)
	}

	return rules, nil
}

// GetKnowledgeBase 获取完整知识库（用于同步）
func (s *Store) GetKnowledgeBase() (*models.KnowledgeBase, error) {
	rules, err := s.GetRules()
	if err != nil {
		return nil, err
	}

	prompts, err := s.GetPrompts()
	if err != nil {
		return nil, err
	}

	return &models.KnowledgeBase{
		Version:   s.GetVersion(),
		Rules:     rules,
		Prompts:   prompts,
		UpdatedAt: time.Now(),
	}, nil
}

// GetPrompts 获取所有提示词模板
func (s *Store) GetPrompts() ([]models.VLMPromptTemplate, error) {
	if s.db == nil {
		// 返回默认模板
		prompts := make([]models.VLMPromptTemplate, 0)
		for name, template := range models.DefaultPromptTemplates {
			prompts = append(prompts, models.VLMPromptTemplate{
				ID:       name,
				Name:     name,
				Scenario: name,
				Template: template,
			})
		}
		return prompts, nil
	}

	rows, err := s.db.Query(`
		SELECT id, name, scenario, template, variables, description, created_at, updated_at
		FROM knowledge_prompts ORDER BY scenario, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prompts []models.VLMPromptTemplate
	for rows.Next() {
		var p models.VLMPromptTemplate
		var variablesJSON []byte

		err := rows.Scan(&p.ID, &p.Name, &p.Scenario, &p.Template, &variablesJSON, &p.Description, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			continue
		}

		json.Unmarshal(variablesJSON, &p.Variables)
		prompts = append(prompts, p)
	}

	return prompts, nil
}

// ========================================
// Targets CRUD
// ========================================

// GetTargets 获取所有特征目标
func (s *Store) GetTargets() ([]models.Target, error) {
	if s.db == nil {
		return []models.Target{}, nil
	}

	rows, err := s.db.Query(`
		SELECT id, name, type, description, tags, image_url, metadata, created_at, updated_at
		FROM knowledge_targets ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []models.Target
	for rows.Next() {
		var t models.Target
		var tagsJSON, metadataJSON []byte
		var imageURL, description sql.NullString

		err := rows.Scan(&t.ID, &t.Name, &t.Type, &description, &tagsJSON, &imageURL, &metadataJSON, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			continue
		}

		if description.Valid {
			t.Description = description.String
		}
		if imageURL.Valid {
			t.ImageURL = imageURL.String
		}
		json.Unmarshal(tagsJSON, &t.Tags)
		json.Unmarshal(metadataJSON, &t.Metadata)
		if t.Tags == nil {
			t.Tags = []string{}
		}

		targets = append(targets, t)
	}

	return targets, nil
}

// GetTarget 获取单个特征目标
func (s *Store) GetTarget(id string) (*models.Target, error) {
	if s.db == nil {
		return nil, sql.ErrNoRows
	}

	var t models.Target
	var tagsJSON, metadataJSON []byte
	var imageURL, description sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, type, description, tags, image_url, metadata, created_at, updated_at
		FROM knowledge_targets WHERE id = ?`, id).Scan(
		&t.ID, &t.Name, &t.Type, &description, &tagsJSON, &imageURL, &metadataJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if description.Valid {
		t.Description = description.String
	}
	if imageURL.Valid {
		t.ImageURL = imageURL.String
	}
	json.Unmarshal(tagsJSON, &t.Tags)
	json.Unmarshal(metadataJSON, &t.Metadata)
	if t.Tags == nil {
		t.Tags = []string{}
	}

	return &t, nil
}

// CreateTarget 创建特征目标
func (s *Store) CreateTarget(req *models.CreateTargetRequest) (*models.Target, error) {
	t := &models.Target{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Tags:        req.Tags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}

	if s.db != nil {
		tagsJSON, _ := json.Marshal(t.Tags)

		_, err := s.db.Exec(`
			INSERT INTO knowledge_targets (id, name, type, description, tags, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.Name, t.Type, t.Description, string(tagsJSON), t.CreatedAt.Format(time.RFC3339), t.UpdatedAt.Format(time.RFC3339),
		)
		if err != nil {
			return nil, fmt.Errorf("创建目标失败: %w", err)
		}
	}

	log.Printf("[Knowledge] 创建目标: %s (%s, type=%s)", t.Name, t.ID, t.Type)
	return t, nil
}

// DeleteTarget 删除特征目标
func (s *Store) DeleteTarget(id string) error {
	if s.db != nil {
		_, err := s.db.Exec(`DELETE FROM knowledge_targets WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("删除目标失败: %w", err)
		}
	}
	log.Printf("[Knowledge] 删除目标: %s", id)
	return nil
}
