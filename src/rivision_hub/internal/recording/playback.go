// Package recording 录像回放与告警录像
package recording

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RecordingType 录像类型
type RecordingType string

const (
	RecordingContinuous RecordingType = "continuous" // 连续录像
	RecordingAlert      RecordingType = "alert"      // 告警录像
	RecordingManual     RecordingType = "manual"     // 手动录像
)

// Recording 录像记录
type Recording struct {
	ID         string        `json:"id" db:"id"`
	CameraID   string        `json:"camera_id" db:"camera_id"`
	CameraName string        `json:"camera_name" db:"camera_name"`
	Type       RecordingType `json:"type" db:"type"`
	AlertID    string        `json:"alert_id,omitempty" db:"alert_id"`
	StartTime  time.Time     `json:"start_time" db:"start_time"`
	EndTime    time.Time     `json:"end_time" db:"end_time"`
	Duration   int           `json:"duration_seconds" db:"duration"`
	FilePath   string        `json:"file_path" db:"file_path"`
	FileSize   int64         `json:"file_size" db:"file_size"`
	Thumbnail  string        `json:"thumbnail,omitempty" db:"thumbnail"`
	NodeID     string        `json:"node_id" db:"node_id"`
}

// RecordingManager 录像管理器
type RecordingManager struct {
	mu            sync.RWMutex
	storageDir    string
	recordings    map[string]*Recording
	activeRecords map[string]*activeRecord
	retentionDays int
}

type activeRecord struct {
	recording *Recording
	file      *os.File
	started   time.Time
}

// RecordingConfig 配置
type RecordingConfig struct {
	StorageDir     string `yaml:"storage_dir"`
	RetentionDays  int    `yaml:"retention_days"`
	PreAlertSec    int    `yaml:"pre_alert_seconds"`  // 告警前录像秒数
	PostAlertSec   int    `yaml:"post_alert_seconds"` // 告警后录像秒数
	MaxFileSize    int64  `yaml:"max_file_size_mb"`
	SegmentMinutes int    `yaml:"segment_minutes"`
}

// DefaultRecordingConfig 默认配置
func DefaultRecordingConfig() RecordingConfig {
	return RecordingConfig{
		StorageDir:     "/data/recordings",
		RetentionDays:  30,
		PreAlertSec:    10,
		PostAlertSec:   30,
		MaxFileSize:    1024, // 1GB
		SegmentMinutes: 15,
	}
}

// NewRecordingManager 创建录像管理器
func NewRecordingManager(cfg RecordingConfig) (*RecordingManager, error) {
	rm := &RecordingManager{
		storageDir:    cfg.StorageDir,
		recordings:    make(map[string]*Recording),
		activeRecords: make(map[string]*activeRecord),
		retentionDays: cfg.RetentionDays,
	}

	// 创建存储目录
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	log.Printf("[Recording] 初始化完成: dir=%s, retention=%d days", cfg.StorageDir, cfg.RetentionDays)
	return rm, nil
}

// StartAlertRecording 开始告警录像
func (rm *RecordingManager) StartAlertRecording(cameraID, alertID, nodeID string, preBuffer []byte) (*Recording, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	recID := fmt.Sprintf("rec_%s_%d", cameraID, time.Now().Unix())
	filePath := rm.generateFilePath(cameraID, RecordingAlert)

	f, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	// 写入预缓冲数据
	if len(preBuffer) > 0 {
		f.Write(preBuffer)
	}

	rec := &Recording{
		ID:        recID,
		CameraID:  cameraID,
		Type:      RecordingAlert,
		AlertID:   alertID,
		StartTime: time.Now(),
		FilePath:  filePath,
		NodeID:    nodeID,
	}

	rm.activeRecords[recID] = &activeRecord{
		recording: rec,
		file:      f,
		started:   time.Now(),
	}

	rm.recordings[recID] = rec
	log.Printf("[Recording] 开始告警录像: %s, alert=%s", recID, alertID)

	return rec, nil
}

// StopRecording 停止录像
func (rm *RecordingManager) StopRecording(recID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	ar, ok := rm.activeRecords[recID]
	if !ok {
		return fmt.Errorf("recording not found: %s", recID)
	}

	ar.file.Close()
	delete(rm.activeRecords, recID)

	// 更新录像信息
	rec := ar.recording
	rec.EndTime = time.Now()
	rec.Duration = int(rec.EndTime.Sub(rec.StartTime).Seconds())

	if fi, err := os.Stat(rec.FilePath); err == nil {
		rec.FileSize = fi.Size()
	}

	log.Printf("[Recording] 停止录像: %s, duration=%ds, size=%d", recID, rec.Duration, rec.FileSize)
	return nil
}

// WriteFrame 写入帧数据
func (rm *RecordingManager) WriteFrame(recID string, data []byte) error {
	rm.mu.RLock()
	ar, ok := rm.activeRecords[recID]
	rm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("recording not active: %s", recID)
	}

	_, err := ar.file.Write(data)
	return err
}

// generateFilePath 生成文件路径
func (rm *RecordingManager) generateFilePath(cameraID string, recType RecordingType) string {
	now := time.Now()
	dir := filepath.Join(rm.storageDir, cameraID, now.Format("2006-01-02"))
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("%s_%s_%s.mp4", cameraID, recType, now.Format("150405"))
	return filepath.Join(dir, filename)
}

// GetRecording 获取录像
func (rm *RecordingManager) GetRecording(id string) (*Recording, bool) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	rec, ok := rm.recordings[id]
	return rec, ok
}

// ListRecordings 列出录像
func (rm *RecordingManager) ListRecordings(cameraID string, startTime, endTime time.Time, limit int) []*Recording {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*Recording, 0)
	for _, rec := range rm.recordings {
		if cameraID != "" && rec.CameraID != cameraID {
			continue
		}
		if !startTime.IsZero() && rec.StartTime.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && rec.StartTime.After(endTime) {
			continue
		}
		result = append(result, rec)
	}

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// GetPlaybackURL 获取回放URL
func (rm *RecordingManager) GetPlaybackURL(recID string) (string, error) {
	rec, ok := rm.GetRecording(recID)
	if !ok {
		return "", fmt.Errorf("recording not found")
	}

	// 返回流媒体URL
	return fmt.Sprintf("/api/v1/recordings/%s/stream", rec.ID), nil
}

// StreamRecording 流式传输录像
func (rm *RecordingManager) StreamRecording(ctx context.Context, recID string, w io.Writer) error {
	rec, ok := rm.GetRecording(recID)
	if !ok {
		return fmt.Errorf("recording not found")
	}

	f, err := os.Open(rec.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}

// CleanupOldRecordings 清理过期录像
func (rm *RecordingManager) CleanupOldRecordings() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -rm.retentionDays)
	deleted := 0

	for id, rec := range rm.recordings {
		if rec.EndTime.Before(cutoff) {
			os.Remove(rec.FilePath)
			delete(rm.recordings, id)
			deleted++
		}
	}

	if deleted > 0 {
		log.Printf("[Recording] 清理过期录像: %d", deleted)
	}

	return deleted
}

// Stats 统计信息
func (rm *RecordingManager) Stats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var totalSize int64
	byType := make(map[string]int)

	for _, rec := range rm.recordings {
		totalSize += rec.FileSize
		byType[string(rec.Type)]++
	}

	return map[string]interface{}{
		"total_recordings": len(rm.recordings),
		"active_recordings": len(rm.activeRecords),
		"total_size_gb":    float64(totalSize) / (1024 * 1024 * 1024),
		"by_type":          byType,
		"storage_dir":      rm.storageDir,
		"retention_days":   rm.retentionDays,
	}
}
