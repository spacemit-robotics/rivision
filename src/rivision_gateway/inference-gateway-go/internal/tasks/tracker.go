package tasks

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// TaskTracker 任务追踪器
type TaskTracker struct {
	mu             sync.Mutex
	maxHistory     int
	tasks          map[string]map[string]interface{}
	completedTasks []map[string]interface{}
	requestTimes   []float64

	totalRequests     int64
	successCount      int64
	failedCount       int64
	totalResponseTime float64

	nodeStats      map[string]map[string]interface{}
	taskDetails    map[string]map[string]interface{}
	maxTaskDetails int
}

// NewTaskTracker 创建任务追踪器
func NewTaskTracker(maxHistory int) *TaskTracker {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &TaskTracker{
		maxHistory:     maxHistory,
		tasks:          make(map[string]map[string]interface{}),
		completedTasks: make([]map[string]interface{}, 0),
		requestTimes:   make([]float64, 0),
		nodeStats:      make(map[string]map[string]interface{}),
		taskDetails:    make(map[string]map[string]interface{}),
		maxTaskDetails: 2000,
	}
}

func (t *TaskTracker) getNodeStats(nodeID string) map[string]interface{} {
	ns, ok := t.nodeStats[nodeID]
	if !ok {
		ns = map[string]interface{}{
			"total_requests":      int64(0),
			"success_count":       int64(0),
			"failed_count":        int64(0),
			"total_response_time": float64(0),
			"running_count":       int64(0),
		}
		t.nodeStats[nodeID] = ns
	}
	return ns
}

// RecordRequestStart 记录请求开始
func (t *TaskTracker) RecordRequestStart(taskType, mainTaskID string, frameIndex, totalFrames int, nodeID string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	taskID := fmt.Sprintf("%s_%s_%d", taskType, mainTaskID, frameIndex)
	now := float64(time.Now().UnixNano()) / 1e9

	if existing, ok := t.tasks[taskID]; ok {
		oldNodeID, _ := existing["node_id"].(string)
		if oldNodeID != "" && oldNodeID != nodeID {
			ons := t.getNodeStats(oldNodeID)
			rc := toInt64(ons["running_count"])
			if rc > 0 {
				ons["running_count"] = rc - 1
			}
		}
		existing["node_id"] = nodeID
		existing["status"] = "running"
		existing["start_time"] = now
		existing["retry_count"] = toInt64(existing["retry_count"]) + 1

		ns := t.getNodeStats(nodeID)
		ns["running_count"] = toInt64(ns["running_count"]) + 1
		return taskID
	}

	t.tasks[taskID] = map[string]interface{}{
		"task_id":       taskID,
		"task_type":     taskType,
		"main_task_id":  mainTaskID,
		"frame_index":   frameIndex,
		"total_frames":  totalFrames,
		"node_id":       nodeID,
		"status":        "running",
		"start_time":    now,
		"end_time":      nil,
		"elapsed":       nil,
		"error":         nil,
		"retry_count":   int64(0),
	}

	ns := t.getNodeStats(nodeID)
	ns["running_count"] = toInt64(ns["running_count"]) + 1
	return taskID
}

// UpdateTaskStatus 更新任务状态
func (t *TaskTracker) UpdateTaskStatus(taskID, status string, details map[string]interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if task, ok := t.tasks[taskID]; ok {
		task["status"] = status
		if details != nil {
			for k, v := range details {
				task[k] = v
			}
		}
	}

	if _, ok := t.taskDetails[taskID]; !ok {
		t.taskDetails[taskID] = make(map[string]interface{})
	}
	t.taskDetails[taskID]["status"] = status
	t.taskDetails[taskID]["last_update"] = float64(time.Now().UnixNano()) / 1e9
	if details != nil {
		for k, v := range details {
			t.taskDetails[taskID][k] = v
		}
	}
}

// RecordRequestEnd 记录请求结束
func (t *TaskTracker) RecordRequestEnd(taskType, mainTaskID string, frameIndex int, nodeID string, success bool, elapsed float64, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := float64(time.Now().UnixNano()) / 1e9
	t.totalRequests++
	t.requestTimes = append(t.requestTimes, now)
	if len(t.requestTimes) > 10000 {
		t.requestTimes = t.requestTimes[1:]
	}

	if success {
		t.successCount++
	} else {
		t.failedCount++
	}
	t.totalResponseTime += elapsed

	ns := t.getNodeStats(nodeID)
	ns["total_requests"] = toInt64(ns["total_requests"]) + 1
	rc := toInt64(ns["running_count"])
	if rc > 0 {
		ns["running_count"] = rc - 1
	}
	if success {
		ns["success_count"] = toInt64(ns["success_count"]) + 1
	} else {
		ns["failed_count"] = toInt64(ns["failed_count"]) + 1
	}
	ns["total_response_time"] = toFloat64(ns["total_response_time"]) + elapsed

	taskID := fmt.Sprintf("%s_%s_%d", taskType, mainTaskID, frameIndex)

	if task, ok := t.tasks[taskID]; ok {
		if success {
			task["status"] = "completed"
		} else {
			task["status"] = "failed"
		}
		task["end_time"] = now
		task["elapsed"] = elapsed
		if errMsg != "" {
			task["error"] = errMsg
		} else {
			task["error"] = nil
		}
		task["node_id"] = nodeID

		copied := copyMap(task)
		t.completedTasks = append(t.completedTasks, copied)
		if len(t.completedTasks) > t.maxHistory {
			t.completedTasks = t.completedTasks[1:]
		}
		delete(t.tasks, taskID)
		delete(t.taskDetails, taskID)
	} else {
		status := "completed"
		if !success {
			status = "failed"
		}
		var errVal interface{} = nil
		if errMsg != "" {
			errVal = errMsg
		}
		task := map[string]interface{}{
			"task_id":      taskID,
			"task_type":    taskType,
			"main_task_id": mainTaskID,
			"frame_index":  frameIndex,
			"total_frames": 0,
			"node_id":      nodeID,
			"status":       status,
			"start_time":   now - elapsed,
			"end_time":     now,
			"elapsed":      elapsed,
			"error":        errVal,
			"retry_count":  int64(0),
		}
		t.completedTasks = append(t.completedTasks, task)
		if len(t.completedTasks) > t.maxHistory {
			t.completedTasks = t.completedTasks[1:]
		}
	}
}

// GetRunningTasks 获取运行中的任务
func (t *TaskTracker) GetRunningTasks() []map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cleanupStaleTasks(0)
	result := make([]map[string]interface{}, 0, len(t.tasks))
	for _, task := range t.tasks {
		result = append(result, task)
	}
	return result
}

// GetCompletedTasks 获取已完成的任务
func (t *TaskTracker) GetCompletedTasks(limit int) []map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	start := len(t.completedTasks) - limit
	if start < 0 {
		start = 0
	}
	return t.completedTasks[start:]
}

// GetRequestsPerMinute 获取每分钟请求数
func (t *TaskTracker) GetRequestsPerMinute() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := float64(time.Now().UnixNano()) / 1e9
	oneMinAgo := now - 60.0
	count := 0
	for _, rt := range t.requestTimes {
		if rt > oneMinAgo {
			count++
		}
	}
	return float64(count)
}

// GetAvgResponseTime 获取平均响应时间
func (t *TaskTracker) GetAvgResponseTime() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.totalRequests == 0 {
		return 0
	}
	return t.totalResponseTime / float64(t.totalRequests)
}

// GetSuccessRate 获取成功率
func (t *TaskTracker) GetSuccessRate() float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.totalRequests == 0 {
		return 100.0
	}
	return float64(t.successCount) / float64(t.totalRequests) * 100.0
}

// GetTaskDistribution 获取任务类型分布
func (t *TaskTracker) GetTaskDistribution() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	dist := map[string]int{"search": 0, "summary": 0, "report": 0}
	for _, task := range t.tasks {
		tt, _ := task["task_type"].(string)
		if _, ok := dist[tt]; ok {
			dist[tt]++
		}
	}
	limit := 100
	if limit > len(t.completedTasks) {
		limit = len(t.completedTasks)
	}
	for i := len(t.completedTasks) - limit; i < len(t.completedTasks); i++ {
		tt, _ := t.completedTasks[i]["task_type"].(string)
		if _, ok := dist[tt]; ok {
			dist[tt]++
		}
	}
	return dist
}

// GetNodeStats 获取节点统计
func (t *TaskTracker) GetNodeStats(nodeID string) map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.getNodeStats(nodeID)
}

// GetCurrentTaskStats 获取当前任务统计
func (t *TaskTracker) GetCurrentTaskStats(mainTaskID string) map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	running := 0
	completed := 0
	totalFrames := 0
	totalElapsed := 0.0

	for _, task := range t.tasks {
		if task["main_task_id"] == mainTaskID {
			running++
			if totalFrames == 0 {
				if tf, ok := task["total_frames"].(int); ok {
					totalFrames = tf
				}
			}
		}
	}
	for _, task := range t.completedTasks {
		if task["main_task_id"] == mainTaskID {
			completed++
			if e, ok := task["elapsed"].(float64); ok {
				totalElapsed += e
			}
			if totalFrames == 0 {
				if tf, ok := task["total_frames"].(int); ok {
					totalFrames = tf
				}
			}
		}
	}

	avgElapsed := 0.0
	if completed > 0 {
		avgElapsed = totalElapsed / float64(completed)
	}
	progress := 0.0
	if totalFrames > 0 {
		progress = float64(completed) / float64(totalFrames) * 100.0
	}

	return map[string]interface{}{
		"main_task_id":      mainTaskID,
		"running_count":     running,
		"completed_count":   completed,
		"total_frames":      totalFrames,
		"progress":          math.Round(progress*10) / 10,
		"avg_response_time": math.Round(avgElapsed*100) / 100,
	}
}

// ResetStats 重置统计
func (t *TaskTracker) ResetStats() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalRequests = 0
	t.successCount = 0
	t.failedCount = 0
	t.totalResponseTime = 0
	t.completedTasks = t.completedTasks[:0]
	t.requestTimes = t.requestTimes[:0]

	for nodeID, ns := range t.nodeStats {
		running := toInt64(ns["running_count"])
		t.nodeStats[nodeID] = map[string]interface{}{
			"total_requests":      int64(0),
			"success_count":       int64(0),
			"failed_count":        int64(0),
			"total_response_time": float64(0),
			"running_count":       running,
		}
	}
	log.Println("[TaskTracker] 统计数据已重置")
}

func (t *TaskTracker) cleanupStaleTasks(timeoutSeconds int) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 360
	}
	now := float64(time.Now().UnixNano()) / 1e9
	var stale []string
	for id, task := range t.tasks {
		if task["status"] == "running" {
			start := toFloat64(task["start_time"])
			if now-start > float64(timeoutSeconds) {
				stale = append(stale, id)
			}
		}
	}
	for _, id := range stale {
		task := t.tasks[id]
		task["status"] = "orphaned"
		task["end_time"] = now
		task["elapsed"] = now - toFloat64(task["start_time"])
		task["error"] = "任务状态丢失（HTTP回调未收到）"
		t.completedTasks = append(t.completedTasks, copyMap(task))
		if len(t.completedTasks) > t.maxHistory {
			t.completedTasks = t.completedTasks[1:]
		}
		delete(t.tasks, id)
		delete(t.taskDetails, id)
		log.Printf("[TaskTracker] 清理孤立任务: %s", id)
	}

	if len(t.taskDetails) > t.maxTaskDetails {
		var orphans []string
		for k := range t.taskDetails {
			if _, ok := t.tasks[k]; !ok {
				orphans = append(orphans, k)
			}
		}
		removeCount := len(orphans) / 2
		if removeCount < len(t.taskDetails)-t.maxTaskDetails {
			removeCount = len(t.taskDetails) - t.maxTaskDetails
		}
		for i := 0; i < removeCount && i < len(orphans); i++ {
			delete(t.taskDetails, orphans[i])
		}
	}
}

// helpers
func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	}
	return 0
}

func copyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// 全局实例
var (
	globalTaskTracker     *TaskTracker
	globalTaskTrackerOnce sync.Once
)

func GetTaskTracker() *TaskTracker {
	globalTaskTrackerOnce.Do(func() {
		globalTaskTracker = NewTaskTracker(1000)
		log.Println("[TaskTracker] 任务追踪器已初始化")
	})
	return globalTaskTracker
}
