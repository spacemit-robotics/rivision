// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package vlm 提供 VLM 推理触发功能
package vlm

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/rivision/rivision-cli/internal/config"
	"github.com/rivision/rivision-cli/internal/gateway"
)

// Trigger VLM 触发器
type Trigger struct {
	gateway *gateway.Client
	config  *config.VLMConfig

	mu       sync.RWMutex
	cameras  map[string]*CameraTrigger
	results  chan *Result
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// CameraTrigger 单个摄像头的触发器
type CameraTrigger struct {
	CameraID    string
	CameraName  string
	Config      config.CameraVLMConfig
	LastTrigger time.Time
	IsRunning   bool
	cancel      context.CancelFunc
}

// Result VLM 分析结果
type Result struct {
	ID             string    `json:"id"`
	CameraID       string    `json:"camera_id"`
	CameraName     string    `json:"camera_name"`
	Timestamp      time.Time `json:"timestamp"`
	TriggerMode    string    `json:"trigger_mode"`
	Prompt         string    `json:"prompt"`
	Text           string    `json:"text"`
	TokensUsed     int       `json:"tokens_used"`
	ImageBase64    string    `json:"image_base64,omitempty"`
	NodeID         string    `json:"node_id"`
	InferenceTimeMs int64    `json:"inference_time_ms"`
	Error          string    `json:"error,omitempty"`
}

// NewTrigger 创建 VLM 触发器
func NewTrigger(gatewayClient *gateway.Client, cfg *config.VLMConfig) *Trigger {
	ctx, cancel := context.WithCancel(context.Background())
	return &Trigger{
		gateway: gatewayClient,
		config:  cfg,
		cameras: make(map[string]*CameraTrigger),
		results: make(chan *Result, 100),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动触发器
func (t *Trigger) Start() error {
	log.Println("[VLM] 触发器已启动")
	return nil
}

// Stop 停止触发器
func (t *Trigger) Stop() {
	t.cancel()
	t.wg.Wait()
	close(t.results)
	log.Println("[VLM] 触发器已停止")
}

// RegisterCamera 注册摄像头
func (t *Trigger) RegisterCamera(cameraID, cameraName string, cfg config.CameraVLMConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !cfg.Enabled {
		return
	}

	ct := &CameraTrigger{
		CameraID:   cameraID,
		CameraName: cameraName,
		Config:     cfg,
	}
	t.cameras[cameraID] = ct

	// 如果是定时触发模式，启动定时器
	if cfg.TriggerMode == "interval" {
		t.startIntervalTrigger(ct)
	}

	log.Printf("[VLM] 注册摄像头: %s, 模式: %s", cameraID, cfg.TriggerMode)
}

// UnregisterCamera 注销摄像头
func (t *Trigger) UnregisterCamera(cameraID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if ct, ok := t.cameras[cameraID]; ok {
		if ct.cancel != nil {
			ct.cancel()
		}
		delete(t.cameras, cameraID)
		log.Printf("[VLM] 注销摄像头: %s", cameraID)
	}
}

// startIntervalTrigger 启动定时触发
func (t *Trigger) startIntervalTrigger(ct *CameraTrigger) {
	ctx, cancel := context.WithCancel(t.ctx)
	ct.cancel = cancel

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()

		ticker := time.NewTicker(ct.Config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 触发 VLM 分析
				t.triggerAnalysis(ct, nil, "interval")
			}
		}
	}()
}

// TriggerManual 手动触发 VLM 分析
func (t *Trigger) TriggerManual(cameraID string, imageData []byte) error {
	t.mu.RLock()
	ct, ok := t.cameras[cameraID]
	t.mu.RUnlock()

	if !ok {
		return fmt.Errorf("摄像头未注册: %s", cameraID)
	}

	return t.triggerAnalysis(ct, imageData, "manual")
}

// TriggerByYOLO YOLO 检测触发 VLM 分析
func (t *Trigger) TriggerByYOLO(cameraID string, detectionCount int, imageData []byte) error {
	t.mu.RLock()
	ct, ok := t.cameras[cameraID]
	t.mu.RUnlock()

	if !ok {
		return fmt.Errorf("摄像头未注册: %s", cameraID)
	}

	// 检查触发模式
	if ct.Config.TriggerMode != "yolo" {
		return nil
	}

	// 检查最小目标数
	if detectionCount < ct.Config.MinObjects {
		return nil
	}

	// 检查冷却时间（避免频繁触发）
	if time.Since(ct.LastTrigger) < ct.Config.Interval {
		return nil
	}

	return t.triggerAnalysis(ct, imageData, "yolo")
}

// triggerAnalysis 执行 VLM 分析
func (t *Trigger) triggerAnalysis(ct *CameraTrigger, imageData []byte, mode string) error {
	t.mu.Lock()
	if ct.IsRunning {
		t.mu.Unlock()
		return fmt.Errorf("摄像头 %s 正在分析中", ct.CameraID)
	}
	ct.IsRunning = true
	ct.LastTrigger = time.Now()
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		ct.IsRunning = false
		t.mu.Unlock()
	}()

	// 获取提示词
	prompt := ct.Config.Prompt
	if prompt == "" {
		prompt = t.config.DefaultPrompt
	}

	// 记录开始时间
	startTime := time.Now()

	// 调用 Gateway 进行分析
	var result *Result
	var analysisResult *gateway.AnalysisResult
	var err error

	if imageData != nil {
		analysisResult, err = t.gateway.AnalyzeImageBytes(imageData, prompt)
	} else {
		// 如果没有图像数据，需要从摄像头获取
		// 这里暂时返回错误，后续由 camera 模块提供
		err = fmt.Errorf("未提供图像数据")
	}

	// 构建结果
	result = &Result{
		ID:          fmt.Sprintf("vlm-%s-%d", ct.CameraID, time.Now().UnixNano()),
		CameraID:    ct.CameraID,
		CameraName:  ct.CameraName,
		Timestamp:   time.Now(),
		TriggerMode: mode,
		Prompt:      prompt,
		InferenceTimeMs: time.Since(startTime).Milliseconds(),
	}

	if err != nil {
		result.Error = err.Error()
		log.Printf("[VLM] 分析失败 [%s]: %v", ct.CameraID, err)
	} else {
		result.Text = analysisResult.Content
		result.NodeID = analysisResult.Model // 使用 Model 字段作为节点标识
		if imageData != nil {
			result.ImageBase64 = base64.StdEncoding.EncodeToString(imageData)
		}
		log.Printf("[VLM] 分析完成 [%s]: %d ms", ct.CameraID, result.InferenceTimeMs)
	}

	// 发送结果
	select {
	case t.results <- result:
	default:
		log.Printf("[VLM] 结果队列已满，丢弃结果")
	}

	return err
}

// Results 获取结果通道
func (t *Trigger) Results() <-chan *Result {
	return t.results
}

// GetCameraStatus 获取摄像头触发状态
func (t *Trigger) GetCameraStatus(cameraID string) (*CameraTrigger, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ct, ok := t.cameras[cameraID]
	return ct, ok
}

// GetAllCameraStatus 获取所有摄像头触发状态
func (t *Trigger) GetAllCameraStatus() map[string]*CameraTrigger {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]*CameraTrigger)
	for k, v := range t.cameras {
		result[k] = v
	}
	return result
}

// SetConfig 更新配置
func (t *Trigger) SetConfig(cameraID string, cfg config.CameraVLMConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ct, ok := t.cameras[cameraID]
	if !ok {
		return fmt.Errorf("摄像头未注册: %s", cameraID)
	}

	// 停止旧的定时器
	if ct.cancel != nil {
		ct.cancel()
	}

	// 更新配置
	ct.Config = cfg

	// 如果是定时触发模式，重新启动定时器
	if cfg.TriggerMode == "interval" && cfg.Enabled {
		t.startIntervalTrigger(ct)
	}

	return nil
}
