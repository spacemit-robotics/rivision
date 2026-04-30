// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package storage 提供数据存储功能
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Storage 存储接口
type Storage interface {
	// VLM 结果
	SaveVLMResult(result *VLMResult) error
	GetVLMResult(id string) (*VLMResult, error)
	ListVLMResults(cameraID string, limit int) ([]*VLMResult, error)
	DeleteVLMResult(id string) error

	// YOLO 检测
	SaveYOLODetection(detection *YOLODetection) error
	ListYOLODetections(cameraID string, limit int) ([]*YOLODetection, error)

	// 事件
	SaveEvent(event *Event) error
	ListEvents(eventType string, limit int) ([]*Event, error)

	// 清理
	Cleanup(olderThan time.Duration) error
}

// VLMResult VLM 分析结果
type VLMResult struct {
	ID              string    `json:"id"`
	CameraID        string    `json:"camera_id"`
	CameraName      string    `json:"camera_name"`
	Timestamp       time.Time `json:"timestamp"`
	TriggerMode     string    `json:"trigger_mode"`
	Prompt          string    `json:"prompt"`
	Text            string    `json:"text"`
	TokensUsed      int       `json:"tokens_used"`
	NodeID          string    `json:"node_id"`
	InferenceTimeMs int64     `json:"inference_time_ms"`
	ImageBase64     string    `json:"image_base64,omitempty"`
}

// YOLODetection YOLO 检测结果
type YOLODetection struct {
	ID              string      `json:"id"`
	CameraID        string      `json:"camera_id"`
	Timestamp       time.Time   `json:"timestamp"`
	FrameIndex      int         `json:"frame_index"`
	Detections      []Detection `json:"detections"`
	InferenceTimeMs int64       `json:"inference_time_ms"`
}

// Detection 单个检测结果
type Detection struct {
	ClassID    int     `json:"class_id"`
	ClassName  string  `json:"class_name"`
	Confidence float32 `json:"confidence"`
	BBox       [4]int  `json:"bbox"` // [x1, y1, x2, y2]
	TrackID    int     `json:"track_id,omitempty"`
}

// Event 系统事件
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	CameraID  string                 `json:"camera_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// FileStorage 基于文件的存储实现
type FileStorage struct {
	mu       sync.RWMutex
	basePath string

	// 内存缓存
	vlmResults     map[string]*VLMResult
	yoloDetections map[string]*YOLODetection
	events         []*Event
}

// NewFileStorage 创建文件存储
func NewFileStorage(basePath string) (*FileStorage, error) {
	// 创建目录结构
	dirs := []string{
		filepath.Join(basePath, "vlm"),
		filepath.Join(basePath, "yolo"),
		filepath.Join(basePath, "events"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}

	fs := &FileStorage{
		basePath:       basePath,
		vlmResults:     make(map[string]*VLMResult),
		yoloDetections: make(map[string]*YOLODetection),
		events:         make([]*Event, 0),
	}

	// 加载现有数据
	fs.loadExistingData()

	return fs, nil
}

// loadExistingData 加载现有数据
func (fs *FileStorage) loadExistingData() {
	// 加载 VLM 结果
	vlmDir := filepath.Join(fs.basePath, "vlm")
	files, _ := os.ReadDir(vlmDir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			data, err := os.ReadFile(filepath.Join(vlmDir, f.Name()))
			if err != nil {
				continue
			}
			var result VLMResult
			if json.Unmarshal(data, &result) == nil {
				fs.vlmResults[result.ID] = &result
			}
		}
	}
}

// SaveVLMResult 保存 VLM 结果
func (fs *FileStorage) SaveVLMResult(result *VLMResult) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// 生成 ID
	if result.ID == "" {
		result.ID = fmt.Sprintf("vlm-%s-%s", result.Timestamp.Format("20060102-150405"), result.CameraID)
	}

	// 保存到内存
	fs.vlmResults[result.ID] = result

	// 保存到文件
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(fs.basePath, "vlm", result.ID+".json")
	return os.WriteFile(filename, data, 0644)
}

// GetVLMResult 获取 VLM 结果
func (fs *FileStorage) GetVLMResult(id string) (*VLMResult, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	result, ok := fs.vlmResults[id]
	if !ok {
		return nil, fmt.Errorf("结果不存在: %s", id)
	}
	return result, nil
}

// ListVLMResults 列出 VLM 结果
func (fs *FileStorage) ListVLMResults(cameraID string, limit int) ([]*VLMResult, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []*VLMResult
	for _, r := range fs.vlmResults {
		if cameraID == "" || r.CameraID == cameraID {
			results = append(results, r)
		}
	}

	// 按时间倒序排序
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Timestamp.After(results[i].Timestamp) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// 限制数量
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// DeleteVLMResult 删除 VLM 结果
func (fs *FileStorage) DeleteVLMResult(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	delete(fs.vlmResults, id)

	filename := filepath.Join(fs.basePath, "vlm", id+".json")
	return os.Remove(filename)
}

// SaveYOLODetection 保存 YOLO 检测结果
func (fs *FileStorage) SaveYOLODetection(detection *YOLODetection) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if detection.ID == "" {
		detection.ID = fmt.Sprintf("yolo-%s-%d", detection.Timestamp.Format("20060102-150405"), detection.FrameIndex)
	}

	fs.yoloDetections[detection.ID] = detection

	// 限制内存中的检测结果数量
	if len(fs.yoloDetections) > 1000 {
		// 删除最旧的
		var oldest *YOLODetection
		for _, d := range fs.yoloDetections {
			if oldest == nil || d.Timestamp.Before(oldest.Timestamp) {
				oldest = d
			}
		}
		if oldest != nil {
			delete(fs.yoloDetections, oldest.ID)
		}
	}

	return nil
}

// ListYOLODetections 列出 YOLO 检测结果
func (fs *FileStorage) ListYOLODetections(cameraID string, limit int) ([]*YOLODetection, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []*YOLODetection
	for _, d := range fs.yoloDetections {
		if cameraID == "" || d.CameraID == cameraID {
			results = append(results, d)
		}
	}

	// 按时间倒序
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Timestamp.After(results[i].Timestamp) {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// SaveEvent 保存事件
func (fs *FileStorage) SaveEvent(event *Event) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if event.ID == "" {
		event.ID = fmt.Sprintf("event-%s-%d", event.Timestamp.Format("20060102-150405"), len(fs.events))
	}

	fs.events = append(fs.events, event)

	// 限制事件数量
	if len(fs.events) > 1000 {
		fs.events = fs.events[len(fs.events)-1000:]
	}

	return nil
}

// ListEvents 列出事件
func (fs *FileStorage) ListEvents(eventType string, limit int) ([]*Event, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var results []*Event
	for i := len(fs.events) - 1; i >= 0; i-- {
		e := fs.events[i]
		if eventType == "" || e.Type == eventType {
			results = append(results, e)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results, nil
}

// Cleanup 清理旧数据
func (fs *FileStorage) Cleanup(olderThan time.Duration) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)

	// 清理 VLM 结果
	for id, r := range fs.vlmResults {
		if r.Timestamp.Before(cutoff) {
			delete(fs.vlmResults, id)
			os.Remove(filepath.Join(fs.basePath, "vlm", id+".json"))
		}
	}

	// 清理 YOLO 检测
	for id, d := range fs.yoloDetections {
		if d.Timestamp.Before(cutoff) {
			delete(fs.yoloDetections, id)
		}
	}

	// 清理事件
	var newEvents []*Event
	for _, e := range fs.events {
		if !e.Timestamp.Before(cutoff) {
			newEvents = append(newEvents, e)
		}
	}
	fs.events = newEvents

	return nil
}
