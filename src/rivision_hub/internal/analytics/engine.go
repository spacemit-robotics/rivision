// Package analytics 提供数据分析引擎
package analytics

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// Engine 分析引擎
type Engine struct {
	db    *sql.DB
	cache *StatsCache
	mu    sync.RWMutex
}

// StatsCache 统计缓存
type StatsCache struct {
	trafficByHour  map[string]*HourlyTraffic
	trafficByDay   map[string]*DailyTraffic
	heatmapData    map[string]*HeatmapData
	lastUpdate     time.Time
	mu             sync.RWMutex
}

// HourlyTraffic 小时客流
type HourlyTraffic struct {
	CameraID string    `json:"camera_id"`
	Hour     time.Time `json:"hour"`
	In       int       `json:"in"`
	Out      int       `json:"out"`
	Total    int       `json:"total"`
}

// DailyTraffic 日客流
type DailyTraffic struct {
	CameraID string    `json:"camera_id"`
	Date     time.Time `json:"date"`
	In       int       `json:"in"`
	Out      int       `json:"out"`
	Total    int       `json:"total"`
	Peak     int       `json:"peak"`
	PeakHour int       `json:"peak_hour"`
}

// HeatmapData 热区数据
type HeatmapData struct {
	CameraID string          `json:"camera_id"`
	Width    int             `json:"width"`
	Height   int             `json:"height"`
	Data     [][]int         `json:"data"`
	UpdateAt time.Time       `json:"update_at"`
}

// DetectionEvent 检测事件
type DetectionEvent struct {
	CameraID   string    `json:"camera_id"`
	Timestamp  time.Time `json:"timestamp"`
	Class      string    `json:"class"`
	Confidence float64   `json:"confidence"`
	BBox       []int     `json:"bbox"` // x, y, w, h
	Direction  string    `json:"direction,omitempty"` // in, out
}

// NewEngine 创建引擎
func NewEngine(db *sql.DB) *Engine {
	e := &Engine{
		db: db,
		cache: &StatsCache{
			trafficByHour: make(map[string]*HourlyTraffic),
			trafficByDay:  make(map[string]*DailyTraffic),
			heatmapData:   make(map[string]*HeatmapData),
		},
	}

	if db != nil {
		e.initTables()
	}

	return e
}

// initTables 初始化表
func (e *Engine) initTables() {
	// R4: Use SQLite-compatible DDL (no SERIAL, VARCHAR, JSONB).
	queries := []string{
		`CREATE TABLE IF NOT EXISTS traffic_hourly (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL,
			hour TEXT NOT NULL,
			count_in INTEGER DEFAULT 0,
			count_out INTEGER DEFAULT 0,
			total INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(camera_id, hour)
		)`,
		`CREATE TABLE IF NOT EXISTS traffic_daily (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL,
			date TEXT NOT NULL,
			count_in INTEGER DEFAULT 0,
			count_out INTEGER DEFAULT 0,
			total INTEGER DEFAULT 0,
			peak INTEGER DEFAULT 0,
			peak_hour INTEGER DEFAULT 0,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(camera_id, date)
		)`,
		`CREATE TABLE IF NOT EXISTS heatmap_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			camera_id TEXT NOT NULL,
			date TEXT NOT NULL,
			hour INTEGER NOT NULL,
			data TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(camera_id, date, hour)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_hourly_camera ON traffic_hourly(camera_id)`,
		`CREATE INDEX IF NOT EXISTS idx_traffic_daily_camera ON traffic_daily(camera_id)`,
	}

	for _, q := range queries {
		if _, err := e.db.Exec(q); err != nil {
			log.Printf("[Analytics] 初始化表失败: %v", err)
		}
	}
}

// ProcessDetection 处理检测事件
func (e *Engine) ProcessDetection(event *DetectionEvent) {
	// 更新客流计数
	if event.Class == "person" && event.Direction != "" {
		e.updateTraffic(event)
	}

	// 更新热区
	if len(event.BBox) >= 4 {
		e.updateHeatmap(event)
	}
}

// updateTraffic 更新客流统计
func (e *Engine) updateTraffic(event *DetectionEvent) {
	hourKey := fmt.Sprintf("%s_%s", event.CameraID, event.Timestamp.Format("2006010215"))
	dayKey := fmt.Sprintf("%s_%s", event.CameraID, event.Timestamp.Format("20060102"))

	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()

	// 更新小时统计
	hourly, ok := e.cache.trafficByHour[hourKey]
	if !ok {
		hourly = &HourlyTraffic{
			CameraID: event.CameraID,
			Hour:     event.Timestamp.Truncate(time.Hour),
		}
		e.cache.trafficByHour[hourKey] = hourly
	}

	if event.Direction == "in" {
		hourly.In++
	} else if event.Direction == "out" {
		hourly.Out++
	}
	hourly.Total = hourly.In + hourly.Out

	// 更新日统计
	daily, ok := e.cache.trafficByDay[dayKey]
	if !ok {
		daily = &DailyTraffic{
			CameraID: event.CameraID,
			Date:     event.Timestamp.Truncate(24 * time.Hour),
		}
		e.cache.trafficByDay[dayKey] = daily
	}

	if event.Direction == "in" {
		daily.In++
	} else if event.Direction == "out" {
		daily.Out++
	}
	daily.Total = daily.In + daily.Out

	// 更新峰值
	if hourly.Total > daily.Peak {
		daily.Peak = hourly.Total
		daily.PeakHour = event.Timestamp.Hour()
	}
}

// updateHeatmap 更新热区数据
func (e *Engine) updateHeatmap(event *DetectionEvent) {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()

	heatmap, ok := e.cache.heatmapData[event.CameraID]
	if !ok {
		// 创建 32x32 的热区网格
		heatmap = &HeatmapData{
			CameraID: event.CameraID,
			Width:    32,
			Height:   32,
			Data:     make([][]int, 32),
		}
		for i := range heatmap.Data {
			heatmap.Data[i] = make([]int, 32)
		}
		e.cache.heatmapData[event.CameraID] = heatmap
	}

	// 将 bbox 中心映射到热区网格
	cx := event.BBox[0] + event.BBox[2]/2
	cy := event.BBox[1] + event.BBox[3]/2

	// 假设原始分辨率 1920x1080，映射到 32x32
	gridX := cx * 32 / 1920
	gridY := cy * 32 / 1080

	if gridX >= 0 && gridX < 32 && gridY >= 0 && gridY < 32 {
		heatmap.Data[gridY][gridX]++
	}
	heatmap.UpdateAt = time.Now()
}

// FlushToDatabase 刷新到数据库
func (e *Engine) FlushToDatabase() error {
	if e.db == nil {
		return nil
	}

	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()

	// 保存小时统计
	for key, hourly := range e.cache.trafficByHour {
		_, err := e.db.Exec(`INSERT INTO traffic_hourly (camera_id, hour, count_in, count_out, total) VALUES (?, ?, ?, ?, ?) ON CONFLICT (camera_id, hour) DO UPDATE SET count_in=excluded.count_in, count_out=excluded.count_out, total=excluded.total`,
			hourly.CameraID, hourly.Hour.Format(time.RFC3339), hourly.In, hourly.Out, hourly.Total)
		if err != nil {
			log.Printf("[Analytics] 保存小时统计失败: %v", err)
		}
		delete(e.cache.trafficByHour, key)
	}

	// 保存日统计
	for key, daily := range e.cache.trafficByDay {
		_, err := e.db.Exec(`INSERT INTO traffic_daily (camera_id, date, count_in, count_out, total, peak, peak_hour) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (camera_id, date) DO UPDATE SET count_in=excluded.count_in, count_out=excluded.count_out, total=excluded.total, peak=excluded.peak, peak_hour=excluded.peak_hour`,
			daily.CameraID, daily.Date.Format("2006-01-02"), daily.In, daily.Out, daily.Total, daily.Peak, daily.PeakHour)
		if err != nil {
			log.Printf("[Analytics] 保存日统计失败: %v", err)
		}
		delete(e.cache.trafficByDay, key)
	}

	e.cache.lastUpdate = time.Now()
	return nil
}

// GetTrafficByHour 获取小时客流
func (e *Engine) GetTrafficByHour(cameraID string, start, end time.Time) ([]*HourlyTraffic, error) {
	if e.db == nil {
		return nil, nil
	}

	rows, err := e.db.Query(`SELECT camera_id, hour, count_in, count_out, total FROM traffic_hourly WHERE camera_id=? AND hour>=? AND hour<=? ORDER BY hour`,
		cameraID, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*HourlyTraffic, 0)
	for rows.Next() {
		t := &HourlyTraffic{}
		if err := rows.Scan(&t.CameraID, &t.Hour, &t.In, &t.Out, &t.Total); err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

// GetTrafficByDay 获取日客流
func (e *Engine) GetTrafficByDay(cameraID string, start, end time.Time) ([]*DailyTraffic, error) {
	if e.db == nil {
		return nil, nil
	}

	rows, err := e.db.Query(`SELECT camera_id, date, count_in, count_out, total, peak, peak_hour FROM traffic_daily WHERE camera_id=? AND date>=? AND date<=? ORDER BY date`,
		cameraID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]*DailyTraffic, 0)
	for rows.Next() {
		t := &DailyTraffic{}
		if err := rows.Scan(&t.CameraID, &t.Date, &t.In, &t.Out, &t.Total, &t.Peak, &t.PeakHour); err != nil {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

// GetHeatmap 获取热区数据
func (e *Engine) GetHeatmap(cameraID string) *HeatmapData {
	e.cache.mu.RLock()
	defer e.cache.mu.RUnlock()

	return e.cache.heatmapData[cameraID]
}

// GetTodaySummary 获取今日汇总
func (e *Engine) GetTodaySummary() map[string]interface{} {
	today := time.Now().Format("20060102")

	e.cache.mu.RLock()
	defer e.cache.mu.RUnlock()

	totalIn := 0
	totalOut := 0
	byCam := make(map[string]map[string]int)

	for key, daily := range e.cache.trafficByDay {
		if key[len(key)-8:] == today {
			totalIn += daily.In
			totalOut += daily.Out
			byCam[daily.CameraID] = map[string]int{
				"in":    daily.In,
				"out":   daily.Out,
				"total": daily.Total,
			}
		}
	}

	return map[string]interface{}{
		"date":       time.Now().Format("2006-01-02"),
		"total_in":   totalIn,
		"total_out":  totalOut,
		"by_camera":  byCam,
	}
}
