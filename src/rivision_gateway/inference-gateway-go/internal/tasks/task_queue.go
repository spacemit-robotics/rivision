// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"log"
	"sync"
	"time"
)

// TaskQueueManager 任务队列管理器
type TaskQueueManager struct {
	mu             sync.Mutex
	queue          []string                        // task_id队列
	tasks          map[string]*GatewayTask          // task_id -> task
	dedupSet       map[string]bool                  // 去重键集合
	mainTaskIndex  map[string][]string              // main_task_id -> [task_ids]
	maxQueueSize   int

	totalEnqueued  int
	totalCompleted int
	totalFailed    int
	totalCancelled int
}

// NewTaskQueueManager 创建队列管理器
func NewTaskQueueManager(maxQueueSize int) *TaskQueueManager {
	if maxQueueSize <= 0 {
		maxQueueSize = 10000
	}
	return &TaskQueueManager{
		queue:         make([]string, 0),
		tasks:         make(map[string]*GatewayTask),
		dedupSet:      make(map[string]bool),
		mainTaskIndex: make(map[string][]string),
		maxQueueSize:  maxQueueSize,
	}
}

// Enqueue 入队任务
func (q *TaskQueueManager) Enqueue(task *GatewayTask) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) >= q.maxQueueSize {
		log.Printf("[TaskQueue] 队列已满，拒绝任务: %s", task.TaskID)
		return false
	}
	if q.dedupSet[task.DedupKey] {
		return false
	}

	task.State = StateQueued
	q.queue = append(q.queue, task.TaskID)
	q.tasks[task.TaskID] = task
	q.dedupSet[task.DedupKey] = true
	q.mainTaskIndex[task.MainTaskID] = append(q.mainTaskIndex[task.MainTaskID], task.TaskID)
	q.totalEnqueued++
	return true
}

// Dequeue 出队任务
func (q *TaskQueueManager) Dequeue() *GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.queue) > 0 {
		taskID := q.queue[0]
		q.queue = q.queue[1:]

		task, ok := q.tasks[taskID]
		if !ok || task.State != StateQueued {
			continue
		}
		if task.CancellationToken != nil && task.CancellationToken.IsCancelled() {
			task.State = StateCancelled
			continue
		}
		task.State = StateDispatched
		now := time.Now()
		task.DispatchedAt = &now
		return task
	}
	return nil
}

// GetTask 获取任务
func (q *TaskQueueManager) GetTask(taskID string) *GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.tasks[taskID]
}

// UpdateTask 更新任务状态
func (q *TaskQueueManager) UpdateTask(taskID string, state TaskState, errMsg string, result map[string]interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.tasks[taskID]
	if !ok {
		return
	}
	if task.State == StateCancelled {
		return
	}
	task.State = state
	if errMsg != "" {
		task.Error = errMsg
	}
	if result != nil {
		task.Result = result
	}
	if state.IsTerminal() {
		now := time.Now()
		task.CompletedAt = &now
		switch state {
		case StateCompleted:
			q.totalCompleted++
		case StateFailed:
			q.totalFailed++
		case StateCancelled:
			q.totalCancelled++
		}
	}
}

// CancelTask 取消单个任务
func (q *TaskQueueManager) CancelTask(taskID, reason string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	task, ok := q.tasks[taskID]
	if !ok || task.IsTerminal() {
		return false
	}
	if task.CancellationToken != nil {
		task.CancellationToken.Cancel(reason)
	}
	task.State = StateCancelled
	task.Error = reason
	q.totalCancelled++
	return true
}

// CancelMainTask 取消主任务下的所有子任务
func (q *TaskQueueManager) CancelMainTask(mainTaskID, reason string) int {
	q.mu.Lock()
	taskIDs := make([]string, len(q.mainTaskIndex[mainTaskID]))
	copy(taskIDs, q.mainTaskIndex[mainTaskID])
	q.mu.Unlock()

	count := 0
	for _, tid := range taskIDs {
		if q.CancelTask(tid, reason) {
			count++
		}
	}
	return count
}

// GetTasksByMainID 获取主任务下的所有子任务
func (q *TaskQueueManager) GetTasksByMainID(mainTaskID string) []*GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*GatewayTask
	for _, tid := range q.mainTaskIndex[mainTaskID] {
		if t, ok := q.tasks[tid]; ok {
			result = append(result, t)
		}
	}
	return result
}

// GetRunningTasks 获取运行中的任务
func (q *TaskQueueManager) GetRunningTasks() []*GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*GatewayTask
	for _, t := range q.tasks {
		if t.State == StateDispatched || t.State == StateRunning {
			result = append(result, t)
		}
	}
	return result
}

// GetPendingTasks 获取等待中的任务
func (q *TaskQueueManager) GetPendingTasks() []*GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*GatewayTask
	for _, t := range q.tasks {
		if t.State == StatePending || t.State == StateQueued {
			result = append(result, t)
		}
	}
	return result
}

// GetTasksByState 按状态获取任务
func (q *TaskQueueManager) GetTasksByState(state TaskState) []*GatewayTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	var result []*GatewayTask
	for _, t := range q.tasks {
		if t.State == state {
			result = append(result, t)
		}
	}
	return result
}

// CleanupCompleted 清理已完成的旧任务
func (q *TaskQueueManager) CleanupCompleted(olderThan time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var toRemove []string
	for id, t := range q.tasks {
		if t.IsTerminal() && t.CompletedAt != nil && t.CompletedAt.Before(cutoff) {
			toRemove = append(toRemove, id)
		}
	}
	for _, id := range toRemove {
		t := q.tasks[id]
		delete(q.dedupSet, t.DedupKey)
		delete(q.tasks, id)
		// 清理mainTaskIndex
		idx := q.mainTaskIndex[t.MainTaskID]
		for i, tid := range idx {
			if tid == id {
				q.mainTaskIndex[t.MainTaskID] = append(idx[:i], idx[i+1:]...)
				break
			}
		}
	}
}

// QueueSize 队列长度
func (q *TaskQueueManager) QueueSize() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// TotalTasks 总任务数
func (q *TaskQueueManager) TotalTasks() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

// GetStats 获取统计
func (q *TaskQueueManager) GetStats() map[string]interface{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	return map[string]interface{}{
		"queue_size":      len(q.queue),
		"total_tasks":     len(q.tasks),
		"total_enqueued":  q.totalEnqueued,
		"total_completed": q.totalCompleted,
		"total_failed":    q.totalFailed,
		"total_cancelled": q.totalCancelled,
	}
}

// 全局实例
var (
	globalTaskQueue     *TaskQueueManager
	globalTaskQueueOnce sync.Once
)

func GetTaskQueueManager() *TaskQueueManager {
	globalTaskQueueOnce.Do(func() {
		globalTaskQueue = NewTaskQueueManager(10000)
	})
	return globalTaskQueue
}
