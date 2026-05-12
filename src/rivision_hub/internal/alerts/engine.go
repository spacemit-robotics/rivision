// Package alerts 实现告警处理引擎
package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rivision/rivision-hub/pkg/models"
)

// Engine 告警引擎
type Engine struct {
	mu           sync.RWMutex
	db           *sql.DB
	deduplicator *Deduplicator
	dispatcher   *Dispatcher
	wsConns      map[*websocket.Conn]bool
	alertChan    chan *models.Alert
	stopChan     chan struct{}
}

// Config 引擎配置
type Config struct {
	DedupWindow    time.Duration
	MaxAlertBuffer int
	Integrations   IntegrationsConfig
}

// IntegrationsConfig 集成配置
type IntegrationsConfig struct {
	WeChat   *WeChartConfig
	DingTalk *DingTalkConfig
	Webhook  *WebhookConfig
	Email    *EmailConfig
}

// NewEngine 创建告警引擎
func NewEngine(db *sql.DB, cfg Config) *Engine {
	if cfg.DedupWindow == 0 {
		cfg.DedupWindow = 5 * time.Minute
	}
	if cfg.MaxAlertBuffer == 0 {
		cfg.MaxAlertBuffer = 1000
	}

	e := &Engine{
		db:           db,
		deduplicator: NewDeduplicator(cfg.DedupWindow),
		dispatcher:   NewDispatcher(cfg.Integrations),
		wsConns:      make(map[*websocket.Conn]bool),
		alertChan:    make(chan *models.Alert, cfg.MaxAlertBuffer),
		stopChan:     make(chan struct{}),
	}

	return e
}

// Start 启动引擎
func (e *Engine) Start(ctx context.Context) {
	log.Printf("[AlertEngine] 启动告警引擎")
	go e.processLoop(ctx)
	go e.deduplicator.CleanupLoop(ctx)
}

// Stop 停止引擎
func (e *Engine) Stop() {
	close(e.stopChan)
}

// processLoop 处理循环
func (e *Engine) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case alert := <-e.alertChan:
			e.handleAlert(alert)
		}
	}
}

// HandleEvent 处理来自Worker的事件
func (e *Engine) HandleEvent(event *models.AlertEvent) error {
	// 构建告警
	alert := &models.Alert{
		ID:          uuid.New().String(),
		NodeID:      event.NodeID,
		CameraID:    event.CameraID,
		Type:        event.Event.Type,
		Level:       event.Event.RiskLevel,
		Status:      models.AlertStatusPending,
		Title:       models.GetAlertTitle(event.Event.Type),
		Description: event.Event.Description,
		Thumbnail:   event.Event.Thumbnail,
		Detections:  event.Event.Detections,
		CreatedAt:   event.Timestamp,
	}

	// 去重检查
	if e.deduplicator.IsDuplicate(alert) {
		log.Printf("[AlertEngine] 告警去重: %s/%s/%s", alert.NodeID, alert.CameraID, alert.Type)
		return nil
	}

	// 记录去重
	e.deduplicator.Record(alert)

	// 发送到处理通道
	select {
	case e.alertChan <- alert:
	default:
		log.Printf("[AlertEngine] 告警通道已满，丢弃告警: %s", alert.ID)
	}

	return nil
}

// handleAlert 处理单个告警
func (e *Engine) handleAlert(alert *models.Alert) {
	// 1. 存储到数据库
	if err := e.saveAlert(alert); err != nil {
		log.Printf("[AlertEngine] 存储告警失败: %v", err)
	}

	// 2. 广播到WebSocket客户端
	e.broadcastAlert(alert)

	// 3. 分发通知
	go e.dispatcher.Dispatch(alert)

	log.Printf("[AlertEngine] 处理告警: ID=%s Type=%s Level=%s", alert.ID, alert.Type, alert.Level)
}

// saveAlert 存储告警
func (e *Engine) saveAlert(alert *models.Alert) error {
	if e.db == nil {
		return nil
	}

	detectionsJSON, _ := json.Marshal(alert.Detections)
	metadataJSON, _ := json.Marshal(alert.Metadata)

	_, err := e.db.Exec(`
		INSERT INTO alerts (id, node_id, camera_id, type, level, status, title, description, 
		                    thumbnail, vlm_result, detections, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		alert.ID, alert.NodeID, alert.CameraID, alert.Type, alert.Level, alert.Status,
		alert.Title, alert.Description, alert.Thumbnail, alert.VLMResult,
		string(detectionsJSON), string(metadataJSON), alert.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// broadcastAlert 广播告警到WebSocket
func (e *Engine) broadcastAlert(alert *models.Alert) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	msg, _ := json.Marshal(map[string]interface{}{
		"type":      "alert",
		"data":      alert,
		"timestamp": time.Now().Format(time.RFC3339),
	})

	for conn := range e.wsConns {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			go e.RemoveWSConn(conn)
		}
	}
}

// AddWSConn 添加WebSocket连接
func (e *Engine) AddWSConn(conn *websocket.Conn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.wsConns[conn] = true
	log.Printf("[AlertEngine] WebSocket连接数: %d", len(e.wsConns))
}

// RemoveWSConn 移除WebSocket连接
func (e *Engine) RemoveWSConn(conn *websocket.Conn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.wsConns, conn)
	conn.Close()
}

// GetAlerts 获取告警列表
func (e *Engine) GetAlerts(query *models.AlertQuery) ([]models.Alert, int64, error) {
	if e.db == nil {
		return []models.Alert{}, 0, nil
	}

	// R4: Build query with ? placeholders for SQLite.
	baseQuery := `FROM alerts WHERE 1=1`
	args := []interface{}{}

	if query.NodeID != "" {
		baseQuery += ` AND node_id = ?`
		args = append(args, query.NodeID)
	}
	if query.CameraID != "" {
		baseQuery += ` AND camera_id = ?`
		args = append(args, query.CameraID)
	}
	if query.Type != "" {
		baseQuery += ` AND type = ?`
		args = append(args, query.Type)
	}
	if query.Level != "" {
		baseQuery += ` AND level = ?`
		args = append(args, query.Level)
	}
	if query.Status != "" {
		baseQuery += ` AND status = ?`
		args = append(args, query.Status)
	}

	// 计数
	var total int64
	countQuery := `SELECT COUNT(*) ` + baseQuery
	e.db.QueryRow(countQuery, args...).Scan(&total)

	// 分页查询
	if query.Limit <= 0 {
		query.Limit = 50
	}
	selectQuery := `SELECT id, node_id, camera_id, type, level, status, title, description, 
	                       thumbnail, vlm_result, created_at, acked_at, resolved_at ` + baseQuery +
		` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, query.Limit, query.Offset)

	rows, err := e.db.Query(selectQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		err := rows.Scan(&a.ID, &a.NodeID, &a.CameraID, &a.Type, &a.Level, &a.Status,
			&a.Title, &a.Description, &a.Thumbnail, &a.VLMResult, &a.CreatedAt, &a.AckedAt, &a.ResolvedAt)
		if err != nil {
			continue
		}
		alerts = append(alerts, a)
	}

	return alerts, total, nil
}

// GetAlert 获取单个告警
func (e *Engine) GetAlert(id string) (*models.Alert, error) {
	if e.db == nil {
		return nil, sql.ErrNoRows
	}

	var a models.Alert
	err := e.db.QueryRow(`
		SELECT id, node_id, camera_id, type, level, status, title, description, 
		       thumbnail, vlm_result, created_at, acked_at, resolved_at
		FROM alerts WHERE id = ?`, id).Scan(
		&a.ID, &a.NodeID, &a.CameraID, &a.Type, &a.Level, &a.Status,
		&a.Title, &a.Description, &a.Thumbnail, &a.VLMResult, &a.CreatedAt, &a.AckedAt, &a.ResolvedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AckAlert 确认告警
func (e *Engine) AckAlert(id, ackedBy string) error {
	if e.db == nil {
		return nil
	}
	now := time.Now()
	_, err := e.db.Exec(`
		UPDATE alerts SET status = ?, acked_at = ?, acked_by = ? WHERE id = ?`,
		models.AlertStatusAcked, now.Format(time.RFC3339), ackedBy, id)
	return err
}

// ResolveAlert 解决告警
func (e *Engine) ResolveAlert(id string) error {
	if e.db == nil {
		return nil
	}
	now := time.Now()
	_, err := e.db.Exec(`
		UPDATE alerts SET status = ?, resolved_at = ? WHERE id = ?`,
		models.AlertStatusResolved, now.Format(time.RFC3339), id)
	return err
}

// GetStats 获取统计信息
func (e *Engine) GetStats() (*models.AlertStats, error) {
	stats := &models.AlertStats{
		ByLevel: make(map[string]int64),
		ByType:  make(map[string]int64),
	}

	if e.db == nil {
		return stats, nil
	}

	// 总数和状态统计
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts`).Scan(&stats.Total)
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE status = 'pending'`).Scan(&stats.Pending)
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE status = 'acked'`).Scan(&stats.Acked)
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE status = 'resolved'`).Scan(&stats.Resolved)

	// 时间范围统计
	now := time.Now()
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE created_at > ?`, now.Add(-24*time.Hour).Format(time.RFC3339)).Scan(&stats.Last24h)
	e.db.QueryRow(`SELECT COUNT(*) FROM alerts WHERE created_at > ?`, now.Add(-1*time.Hour).Format(time.RFC3339)).Scan(&stats.LastHour)

	// 按级别统计
	rows, _ := e.db.Query(`SELECT level, COUNT(*) FROM alerts GROUP BY level`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var level string
			var count int64
			rows.Scan(&level, &count)
			stats.ByLevel[level] = count
		}
	}

	// 按类型统计
	rows2, _ := e.db.Query(`SELECT type, COUNT(*) FROM alerts GROUP BY type`)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var typ string
			var count int64
			rows2.Scan(&typ, &count)
			stats.ByType[typ] = count
		}
	}

	return stats, nil
}
