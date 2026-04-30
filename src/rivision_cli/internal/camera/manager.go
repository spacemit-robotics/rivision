// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package camera 提供摄像头管理功能
package camera

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/rivision/rivision-cli/internal/config"
)

// Manager 摄像头管理器
type Manager struct {
	cameras map[string]*Camera
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc

	// 帧回调
	onFrame func(cameraID string, frame *Frame)
}

// NewManager 创建摄像头管理器
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cameras: make(map[string]*Camera),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动管理器
func (m *Manager) Start() error {
	log.Println("[Camera] 管理器已启动")
	return nil
}

// Stop 停止管理器
func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cam := range m.cameras {
		cam.Stop()
	}

	log.Println("[Camera] 管理器已停止")
}

// Add 添加摄像头
func (m *Manager) Add(cfg config.CameraConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.cameras[cfg.ID]; exists {
		return fmt.Errorf("摄像头已存在: %s", cfg.ID)
	}

	cam := NewCamera(cfg)
	cam.SetFrameCallback(func(frame *Frame) {
		if m.onFrame != nil {
			m.onFrame(cfg.ID, frame)
		}
	})

	m.cameras[cfg.ID] = cam

	if cfg.Enabled {
		if err := cam.Start(m.ctx); err != nil {
			log.Printf("[Camera] 启动失败 [%s]: %v", cfg.ID, err)
		}
	}

	log.Printf("[Camera] 添加摄像头: %s (%s)", cfg.ID, cfg.Name)
	return nil
}

// Remove 移除摄像头
func (m *Manager) Remove(cameraID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cam, exists := m.cameras[cameraID]
	if !exists {
		return fmt.Errorf("摄像头不存在: %s", cameraID)
	}

	cam.Stop()
	delete(m.cameras, cameraID)

	log.Printf("[Camera] 移除摄像头: %s", cameraID)
	return nil
}

// Get 获取摄像头
func (m *Manager) Get(cameraID string) (*Camera, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cam, ok := m.cameras[cameraID]
	return cam, ok
}

// List 列出所有摄像头
func (m *Manager) List() []*Camera {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Camera, 0, len(m.cameras))
	for _, cam := range m.cameras {
		result = append(result, cam)
	}
	return result
}

// GetStatus 获取所有摄像头状态
func (m *Manager) GetStatus() map[string]*CameraStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CameraStatus)
	for id, cam := range m.cameras {
		result[id] = cam.GetStatus()
	}
	return result
}

// SetFrameCallback 设置帧回调
func (m *Manager) SetFrameCallback(callback func(cameraID string, frame *Frame)) {
	m.onFrame = callback
}

// CaptureFrame 捕获指定摄像头的当前帧
func (m *Manager) CaptureFrame(cameraID string) (*Frame, error) {
	m.mu.RLock()
	cam, exists := m.cameras[cameraID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("摄像头不存在: %s", cameraID)
	}

	return cam.CaptureFrame()
}

// LoadFromConfig 从配置加载摄像头
func (m *Manager) LoadFromConfig(cameras []config.CameraConfig) error {
	for _, cfg := range cameras {
		if err := m.Add(cfg); err != nil {
			log.Printf("[Camera] 加载失败 [%s]: %v", cfg.ID, err)
		}
	}
	return nil
}
