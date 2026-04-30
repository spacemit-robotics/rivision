// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package camera

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rivision/rivision-cli/internal/config"
)

// CameraState 摄像头状态
type CameraState int

const (
	StateDisconnected CameraState = iota
	StateConnecting
	StateConnected
	StateError
)

func (s CameraState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Camera 摄像头
type Camera struct {
	Config config.CameraConfig

	mu          sync.RWMutex
	state       CameraState
	lastError   error
	lastFrame   *Frame
	frameCount  int64
	startTime   time.Time
	onFrame     func(*Frame)

	ctx    context.Context
	cancel context.CancelFunc
}

// CameraStatus 摄像头状态信息
type CameraStatus struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Source     string      `json:"source"`
	State      string      `json:"state"`
	Enabled    bool        `json:"enabled"`
	FrameCount int64       `json:"frame_count"`
	Uptime     string      `json:"uptime"`
	LastError  string      `json:"last_error,omitempty"`
}

// NewCamera 创建摄像头
func NewCamera(cfg config.CameraConfig) *Camera {
	return &Camera{
		Config: cfg,
		state:  StateDisconnected,
	}
}

// Start 启动摄像头
func (c *Camera) Start(parentCtx context.Context) error {
	c.mu.Lock()
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return nil
	}
	c.state = StateConnecting
	c.ctx, c.cancel = context.WithCancel(parentCtx)
	c.startTime = time.Now()
	c.mu.Unlock()

	// 模拟连接（实际实现需要连接 RTSP/go2rtc）
	go c.runFrameLoop()

	log.Printf("[Camera] 启动: %s (%s)", c.Config.ID, c.Config.Source)
	return nil
}

// Stop 停止摄像头
func (c *Camera) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}
	c.state = StateDisconnected

	log.Printf("[Camera] 停止: %s", c.Config.ID)
}

// runFrameLoop 帧循环（模拟实现）
func (c *Camera) runFrameLoop() {
	c.mu.Lock()
	c.state = StateConnected
	c.mu.Unlock()

	// 实际实现中，这里应该从 go2rtc 或 RTSP 流获取帧
	// 当前为模拟实现，仅用于架构验证
	ticker := time.NewTicker(time.Second / 2) // 2 FPS 模拟
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			c.frameCount++
			frame := &Frame{
				CameraID:  c.Config.ID,
				Timestamp: time.Now(),
				Index:     c.frameCount,
				// Data 字段在实际实现中应该包含 JPEG 数据
			}
			c.lastFrame = frame
			callback := c.onFrame
			c.mu.Unlock()

			if callback != nil {
				callback(frame)
			}
		}
	}
}

// SetFrameCallback 设置帧回调
func (c *Camera) SetFrameCallback(callback func(*Frame)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onFrame = callback
}

// CaptureFrame 捕获当前帧
func (c *Camera) CaptureFrame() (*Frame, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state != StateConnected {
		return nil, fmt.Errorf("摄像头未连接")
	}

	if c.lastFrame == nil {
		return nil, fmt.Errorf("无可用帧")
	}

	// 返回最后一帧的副本
	frame := &Frame{
		CameraID:  c.lastFrame.CameraID,
		Timestamp: c.lastFrame.Timestamp,
		Index:     c.lastFrame.Index,
		Data:      c.lastFrame.Data,
		Width:     c.lastFrame.Width,
		Height:    c.lastFrame.Height,
	}

	return frame, nil
}

// GetStatus 获取状态
func (c *Camera) GetStatus() *CameraStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := &CameraStatus{
		ID:         c.Config.ID,
		Name:       c.Config.Name,
		Source:     c.Config.Source,
		State:      c.state.String(),
		Enabled:    c.Config.Enabled,
		FrameCount: c.frameCount,
	}

	if !c.startTime.IsZero() && c.state == StateConnected {
		status.Uptime = time.Since(c.startTime).Round(time.Second).String()
	}

	if c.lastError != nil {
		status.LastError = c.lastError.Error()
	}

	return status
}

// GetState 获取状态
func (c *Camera) GetState() CameraState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// IsConnected 是否已连接
func (c *Camera) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateConnected
}
