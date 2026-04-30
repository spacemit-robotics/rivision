// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package tasks 任务管理系统
package tasks

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"
)

// TaskState 任务状态枚举
type TaskState string

const (
	StatePending    TaskState = "pending"
	StateQueued     TaskState = "queued"
	StateDispatched TaskState = "dispatched"
	StateRunning    TaskState = "running"
	StateCompleted  TaskState = "completed"
	StateFailed     TaskState = "failed"
	StateCancelled  TaskState = "cancelled"
	StateTimeout    TaskState = "timeout"
)

// IsTerminal 是否终态
func (s TaskState) IsTerminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled || s == StateTimeout
}

// CancellationToken 取消令牌
type CancellationToken struct {
	mu        sync.Mutex
	cancelled bool
	reason    string
	cancelCh  chan struct{}
}

// NewCancellationToken 创建取消令牌
func NewCancellationToken() *CancellationToken {
	return &CancellationToken{
		cancelCh: make(chan struct{}),
	}
}

func (ct *CancellationToken) IsCancelled() bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.cancelled
}

func (ct *CancellationToken) Reason() string {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.reason
}

func (ct *CancellationToken) Cancel(reason string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if !ct.cancelled {
		ct.cancelled = true
		ct.reason = reason
		close(ct.cancelCh)
	}
}

func (ct *CancellationToken) Done() <-chan struct{} {
	return ct.cancelCh
}

// TaskCancelledException 取消异常
type TaskCancelledException struct {
	Reason string
}

func (e *TaskCancelledException) Error() string {
	return "任务已取消: " + e.Reason
}

// GatewayTask 网关任务
type GatewayTask struct {
	TaskID      string                 `json:"task_id"`
	TaskType    string                 `json:"task_type"`
	MainTaskID  string                 `json:"main_task_id"`
	FrameIndex  int                    `json:"frame_index"`
	TotalFrames int                    `json:"total_frames"`
	Payload     map[string]interface{} `json:"payload"`

	State  TaskState `json:"state"`
	NodeID string    `json:"node_id"`

	CreatedAt    time.Time  `json:"created_at"`
	DispatchedAt *time.Time `json:"dispatched_at,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`

	Result map[string]interface{} `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`

	RetryCount int `json:"retry_count"`
	MaxRetries int `json:"max_retries"`

	CancellationToken *CancellationToken `json:"-"`
	DedupKey          string             `json:"dedup_key"`
}

// NewGatewayTask 创建新任务
func NewGatewayTask(taskID, taskType, mainTaskID string, frameIndex, totalFrames int,
	payload map[string]interface{}) *GatewayTask {
	t := &GatewayTask{
		TaskID:            taskID,
		TaskType:          taskType,
		MainTaskID:        mainTaskID,
		FrameIndex:        frameIndex,
		TotalFrames:       totalFrames,
		Payload:           payload,
		State:             StatePending,
		CreatedAt:         time.Now(),
		MaxRetries:        3,
		CancellationToken: NewCancellationToken(),
	}
	t.GenerateDedupKey()
	return t
}

func (t *GatewayTask) GenerateDedupKey() {
	data := fmt.Sprintf("%s:%s:%d", t.TaskType, t.MainTaskID, t.FrameIndex)
	hash := md5.Sum([]byte(data))
	t.DedupKey = fmt.Sprintf("%x", hash[:8])
}

func (t *GatewayTask) ElapsedTime() float64 {
	if t.StartedAt == nil {
		return 0
	}
	end := time.Now()
	if t.CompletedAt != nil {
		end = *t.CompletedAt
	}
	return end.Sub(*t.StartedAt).Seconds()
}

func (t *GatewayTask) IsTerminal() bool {
	return t.State.IsTerminal()
}

func (t *GatewayTask) ToDict() map[string]interface{} {
	d := map[string]interface{}{
		"task_id":       t.TaskID,
		"task_type":     t.TaskType,
		"main_task_id":  t.MainTaskID,
		"frame_index":   t.FrameIndex,
		"total_frames":  t.TotalFrames,
		"state":         string(t.State),
		"node_id":       t.NodeID,
		"created_at":    t.CreatedAt.Format(time.RFC3339),
		"elapsed":       t.ElapsedTime(),
		"retry_count":   t.RetryCount,
		"error":         t.Error,
	}
	if t.StartedAt != nil {
		d["started_at"] = t.StartedAt.Format(time.RFC3339)
	}
	if t.CompletedAt != nil {
		d["completed_at"] = t.CompletedAt.Format(time.RFC3339)
	}
	return d
}
