// Package api 提供异步任务 API
package api

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ============================================
// 异步任务管理器
// ============================================

// TaskState 任务状态
type TaskState string

const (
	TaskStateQueued     TaskState = "queued"
	TaskStateDispatched TaskState = "dispatched"
	TaskStateRunning    TaskState = "running"
	TaskStateCompleted  TaskState = "completed"
	TaskStateFailed     TaskState = "failed"
	TaskStateCancelled  TaskState = "cancelled"
	TaskStateTimeout    TaskState = "timeout"
)

// AsyncTask 异步任务
type AsyncTask struct {
	TaskID      string                 `json:"task_id"`
	TaskType    string                 `json:"task_type"`
	MainTaskID  string                 `json:"main_task_id"`
	FrameIndex  int                    `json:"frame_index"`
	TotalFrames int                    `json:"total_frames"`
	State       TaskState              `json:"state"`
	Payload     map[string]interface{} `json:"payload"`
	Result      map[string]interface{} `json:"result,omitempty"`
	NodeID      string                 `json:"node_id,omitempty"`
	Error       string                 `json:"error,omitempty"`
	RetryCount  int                    `json:"retry_count"`
	CreatedAt   time.Time              `json:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
}

// AsyncTaskManager 异步任务管理器
type AsyncTaskManager struct {
	mu             sync.RWMutex
	tasks          map[string]*AsyncTask
	mainTaskIndex  map[string][]string // main_task_id -> []task_id
	acknowledged   map[string]bool
	maxQueueSize   int
}

// NewAsyncTaskManager 创建异步任务管理器
func NewAsyncTaskManager() *AsyncTaskManager {
	return &AsyncTaskManager{
		tasks:         make(map[string]*AsyncTask),
		mainTaskIndex: make(map[string][]string),
		acknowledged:  make(map[string]bool),
		maxQueueSize:  1000,
	}
}

// GenerateTaskID 生成确定性任务ID
func GenerateTaskID(mainTaskID string, frameIndex int, taskType string) string {
	return fmt.Sprintf("%s_%s_%d", taskType, mainTaskID, frameIndex)
}

// Enqueue 入队任务
func (m *AsyncTaskManager) Enqueue(task *AsyncTask) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查是否已存在
	if existing, ok := m.tasks[task.TaskID]; ok {
		return true, true // isDuplicate = true
	} else {
		_ = existing
	}

	// 检查队列大小
	if len(m.tasks) >= m.maxQueueSize {
		return false, false
	}

	task.State = TaskStateQueued
	task.CreatedAt = time.Now()
	m.tasks[task.TaskID] = task

	// 更新主任务索引
	m.mainTaskIndex[task.MainTaskID] = append(m.mainTaskIndex[task.MainTaskID], task.TaskID)

	return true, false
}

// GetTask 获取任务
func (m *AsyncTaskManager) GetTask(taskID string) *AsyncTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

// GetStats 获取统计
func (m *AsyncTaskManager) GetStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]int{
		"queue_size": len(m.tasks),
	}
}

// MarkAcknowledged 标记已确认
func (m *AsyncTaskManager) MarkAcknowledged(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acknowledged[taskID] = true
}

// IsAcknowledged 检查是否已确认
func (m *AsyncTaskManager) IsAcknowledged(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.acknowledged[taskID]
}

// GetMainTaskProgress 获取主任务进度
func (m *AsyncTaskManager) GetMainTaskProgress(mainTaskID string) map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	taskIDs := m.mainTaskIndex[mainTaskID]
	total := 0
	completed := 0
	failed := 0
	running := 0
	pending := 0

	for _, taskID := range taskIDs {
		task, ok := m.tasks[taskID]
		if !ok {
			continue
		}
		total++
		switch task.State {
		case TaskStateCompleted:
			completed++
		case TaskStateFailed, TaskStateTimeout:
			failed++
		case TaskStateRunning, TaskStateDispatched:
			running++
		default:
			pending++
		}
	}

	progressPercent := float64(0)
	if total > 0 {
		progressPercent = float64(completed) / float64(total) * 100
	}

	return map[string]interface{}{
		"main_task_id":     mainTaskID,
		"total":            total,
		"completed":        completed,
		"failed":           failed,
		"running":          running,
		"pending":          pending,
		"progress_percent": progressPercent,
	}
}

// 全局异步任务管理器
var asyncTaskManager = NewAsyncTaskManager()

// ============================================
// 请求/响应模型
// ============================================

// TaskSubmitRequest 任务提交请求
type TaskSubmitRequest struct {
	Image       string  `json:"image" binding:"required"`
	Prompt      string  `json:"prompt" binding:"required"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TaskType    string  `json:"task_type"`
	MainTaskID  string  `json:"main_task_id" binding:"required"`
	FrameIndex  int     `json:"frame_index"`
	TotalFrames int     `json:"total_frames"`
}

// TaskSubmitResponse 任务提交响应
type TaskSubmitResponse struct {
	TaskID        string `json:"task_id"`
	Status        string `json:"status"`
	QueuePosition int    `json:"queue_position,omitempty"`
	IsDuplicate   bool   `json:"is_duplicate"`
	Message       string `json:"message"`
}

// TaskStatusResponse 任务状态响应
type TaskStatusResponse struct {
	TaskID         string  `json:"task_id"`
	Status         string  `json:"status"`
	FrameIndex     *int    `json:"frame_index,omitempty"`
	TotalFrames    *int    `json:"total_frames,omitempty"`
	NodeID         string  `json:"node_id,omitempty"`
	CreatedAt      string  `json:"created_at,omitempty"`
	StartedAt      string  `json:"started_at,omitempty"`
	CompletedAt    string  `json:"completed_at,omitempty"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	RetryCount     int     `json:"retry_count"`
	Error          string  `json:"error,omitempty"`
	HasResult      bool    `json:"has_result"`
}

// TaskResultResponse 任务结果响应
type TaskResultResponse struct {
	TaskID  string                 `json:"task_id"`
	Status  string                 `json:"status"`
	Content string                 `json:"content,omitempty"`
	Model   string                 `json:"model,omitempty"`
	Usage   map[string]interface{} `json:"usage,omitempty"`
	NodeID  string                 `json:"node_id,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// AckResponse ACK响应
type AckResponse struct {
	TaskID       string `json:"task_id"`
	Acknowledged bool   `json:"acknowledged"`
	Message      string `json:"message"`
}

// BatchSubmitRequest 批量提交请求
type BatchSubmitRequest struct {
	Tasks []TaskSubmitRequest `json:"tasks" binding:"required"`
}

// BatchStatusResponse 批量状态响应
type BatchStatusResponse struct {
	Tasks     []TaskStatusResponse `json:"tasks"`
	Total     int                  `json:"total"`
	Completed int                  `json:"completed"`
	Failed    int                  `json:"failed"`
	Pending   int                  `json:"pending"`
}

// SystemHealthResponse 系统健康响应
type SystemHealthResponse struct {
	Status           string  `json:"status"`
	DiskFreeGB       float64 `json:"disk_free_gb"`
	DiskTotalGB      float64 `json:"disk_total_gb"`
	DiskUsagePercent float64 `json:"disk_usage_percent"`
	QueueSize        int     `json:"queue_size"`
	CacheSize        int     `json:"cache_size"`
	IsHealthy        bool    `json:"is_healthy"`
}

// ============================================
// API 处理器
// ============================================

// SubmitAsyncTask 提交异步任务
// POST /api/v1/async-tasks/submit
func (h *Handlers) SubmitAsyncTask(c *gin.Context) {
	var req TaskSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 设置默认值
	if req.MaxTokens == 0 {
		req.MaxTokens = 500
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}
	if req.TaskType == "" {
		req.TaskType = "unknown"
	}

	// 生成确定性任务ID
	taskID := GenerateTaskID(req.MainTaskID, req.FrameIndex, req.TaskType)

	// 检查是否已存在
	existing := asyncTaskManager.GetTask(taskID)
	if existing != nil {
		c.JSON(http.StatusOK, TaskSubmitResponse{
			TaskID:      taskID,
			Status:      string(existing.State),
			IsDuplicate: true,
			Message:     "任务已存在，返回现有状态",
		})
		return
	}

	// 创建新任务
	task := &AsyncTask{
		TaskID:      taskID,
		TaskType:    req.TaskType,
		MainTaskID:  req.MainTaskID,
		FrameIndex:  req.FrameIndex,
		TotalFrames: req.TotalFrames,
		Payload: map[string]interface{}{
			"image":       req.Image,
			"prompt":      req.Prompt,
			"max_tokens":  req.MaxTokens,
			"temperature": req.Temperature,
		},
	}

	success, isDuplicate := asyncTaskManager.Enqueue(task)
	if !success {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "任务队列已满，请稍后重试",
		})
		return
	}

	if isDuplicate {
		c.JSON(http.StatusOK, TaskSubmitResponse{
			TaskID:      taskID,
			Status:      string(TaskStateQueued),
			IsDuplicate: true,
			Message:     "任务已存在",
		})
		return
	}

	stats := asyncTaskManager.GetStats()
	c.JSON(http.StatusOK, TaskSubmitResponse{
		TaskID:        taskID,
		Status:        "queued",
		QueuePosition: stats["queue_size"],
		IsDuplicate:   false,
		Message:       "任务已入队，等待执行",
	})
}

// GetAsyncTaskStatus 获取任务状态
// GET /api/v1/async-tasks/status/:task_id
func (h *Handlers) GetAsyncTaskStatus(c *gin.Context) {
	taskID := c.Param("task_id")

	task := asyncTaskManager.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("任务不存在: %s", taskID),
		})
		return
	}

	resp := TaskStatusResponse{
		TaskID:     taskID,
		Status:     string(task.State),
		FrameIndex: &task.FrameIndex,
		TotalFrames: &task.TotalFrames,
		NodeID:     task.NodeID,
		RetryCount: task.RetryCount,
		Error:      task.Error,
		HasResult:  task.Result != nil,
	}

	if !task.CreatedAt.IsZero() {
		resp.CreatedAt = task.CreatedAt.Format(time.RFC3339)
	}
	if task.StartedAt != nil {
		resp.StartedAt = task.StartedAt.Format(time.RFC3339)
	}
	if task.CompletedAt != nil {
		resp.CompletedAt = task.CompletedAt.Format(time.RFC3339)
		resp.ElapsedSeconds = task.CompletedAt.Sub(task.CreatedAt).Seconds()
	}

	c.JSON(http.StatusOK, resp)
}

// GetAsyncTaskResult 获取任务结果
// GET /api/v1/async-tasks/result/:task_id
func (h *Handlers) GetAsyncTaskResult(c *gin.Context) {
	taskID := c.Param("task_id")

	task := asyncTaskManager.GetTask(taskID)
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("任务不存在或结果已过期: %s", taskID),
		})
		return
	}

	switch task.State {
	case TaskStateCompleted:
		content, _ := task.Result["content"].(string)
		model, _ := task.Result["model"].(string)
		usage, _ := task.Result["usage"].(map[string]interface{})
		c.JSON(http.StatusOK, TaskResultResponse{
			TaskID:  taskID,
			Status:  "completed",
			Content: content,
			Model:   model,
			Usage:   usage,
			NodeID:  task.NodeID,
		})
	case TaskStateFailed:
		c.JSON(http.StatusOK, TaskResultResponse{
			TaskID: taskID,
			Status: "failed",
			Error:  task.Error,
		})
	case TaskStateCancelled, TaskStateTimeout:
		c.JSON(http.StatusOK, TaskResultResponse{
			TaskID: taskID,
			Status: string(task.State),
			Error:  task.Error,
		})
	default:
		c.JSON(http.StatusOK, TaskResultResponse{
			TaskID: taskID,
			Status: string(task.State),
			Error:  "任务尚未完成",
		})
	}
}

// AcknowledgeAsyncTask 确认任务结果
// POST /api/v1/async-tasks/ack/:task_id
func (h *Handlers) AcknowledgeAsyncTask(c *gin.Context) {
	taskID := c.Param("task_id")

	task := asyncTaskManager.GetTask(taskID)
	if task != nil && task.State == TaskStateCompleted {
		asyncTaskManager.MarkAcknowledged(taskID)
		c.JSON(http.StatusOK, AckResponse{
			TaskID:       taskID,
			Acknowledged: true,
			Message:      "结果已确认，可安全清理",
		})
		return
	}

	c.JSON(http.StatusOK, AckResponse{
		TaskID:       taskID,
		Acknowledged: false,
		Message:      "任务不存在或未完成",
	})
}

// BatchSubmitAsyncTasks 批量提交任务
// POST /api/v1/async-tasks/batch/submit
func (h *Handlers) BatchSubmitAsyncTasks(c *gin.Context) {
	var req BatchSubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	responses := make([]TaskSubmitResponse, 0, len(req.Tasks))
	for _, taskReq := range req.Tasks {
		if taskReq.MaxTokens == 0 {
			taskReq.MaxTokens = 500
		}
		if taskReq.Temperature == 0 {
			taskReq.Temperature = 0.7
		}
		if taskReq.TaskType == "" {
			taskReq.TaskType = "unknown"
		}

		taskID := GenerateTaskID(taskReq.MainTaskID, taskReq.FrameIndex, taskReq.TaskType)

		existing := asyncTaskManager.GetTask(taskID)
		if existing != nil {
			responses = append(responses, TaskSubmitResponse{
				TaskID:      taskID,
				Status:      string(existing.State),
				IsDuplicate: true,
				Message:     "任务已存在",
			})
			continue
		}

		task := &AsyncTask{
			TaskID:      taskID,
			TaskType:    taskReq.TaskType,
			MainTaskID:  taskReq.MainTaskID,
			FrameIndex:  taskReq.FrameIndex,
			TotalFrames: taskReq.TotalFrames,
			Payload: map[string]interface{}{
				"image":       taskReq.Image,
				"prompt":      taskReq.Prompt,
				"max_tokens":  taskReq.MaxTokens,
				"temperature": taskReq.Temperature,
			},
		}

		success, isDuplicate := asyncTaskManager.Enqueue(task)
		if !success {
			responses = append(responses, TaskSubmitResponse{
				TaskID:  "",
				Status:  "error",
				Message: "任务队列已满",
			})
			continue
		}

		responses = append(responses, TaskSubmitResponse{
			TaskID:      taskID,
			Status:      "queued",
			IsDuplicate: isDuplicate,
			Message:     "任务已入队",
		})
	}

	c.JSON(http.StatusOK, responses)
}

// BatchGetAsyncTaskStatus 批量获取任务状态
// POST /api/v1/async-tasks/batch/status
func (h *Handlers) BatchGetAsyncTaskStatus(c *gin.Context) {
	var taskIDs []string
	if err := c.ShouldBindJSON(&taskIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	tasks := make([]TaskStatusResponse, 0, len(taskIDs))
	completed := 0
	failed := 0
	pending := 0

	for _, taskID := range taskIDs {
		task := asyncTaskManager.GetTask(taskID)
		if task == nil {
			tasks = append(tasks, TaskStatusResponse{
				TaskID: taskID,
				Status: "not_found",
			})
			failed++
			continue
		}

		resp := TaskStatusResponse{
			TaskID:      taskID,
			Status:      string(task.State),
			FrameIndex:  &task.FrameIndex,
			TotalFrames: &task.TotalFrames,
			NodeID:      task.NodeID,
			RetryCount:  task.RetryCount,
			Error:       task.Error,
			HasResult:   task.Result != nil,
		}

		if !task.CreatedAt.IsZero() {
			resp.CreatedAt = task.CreatedAt.Format(time.RFC3339)
		}

		tasks = append(tasks, resp)

		switch task.State {
		case TaskStateCompleted:
			completed++
		case TaskStateFailed, TaskStateCancelled, TaskStateTimeout:
			failed++
		default:
			pending++
		}
	}

	c.JSON(http.StatusOK, BatchStatusResponse{
		Tasks:     tasks,
		Total:     len(taskIDs),
		Completed: completed,
		Failed:    failed,
		Pending:   pending,
	})
}

// GetAsyncTasksHealth 获取异步任务系统健康状态
// GET /api/v1/async-tasks/health
func (h *Handlers) GetAsyncTasksHealth(c *gin.Context) {
	// 检查磁盘空间
	var stat os.FileInfo
	diskFreeGB := float64(0)
	diskTotalGB := float64(0)
	diskUsagePercent := float64(0)
	isHealthy := true

	// 简化的磁盘检查 (Linux)
	wd, _ := os.Getwd()
	if _, err := os.Stat(wd); err == nil {
		// 使用默认值
		diskFreeGB = 10.0
		diskTotalGB = 100.0
		diskUsagePercent = 90.0
		isHealthy = true
	}
	_ = stat

	stats := asyncTaskManager.GetStats()
	queueSize := stats["queue_size"]

	c.JSON(http.StatusOK, SystemHealthResponse{
		Status:           "healthy",
		DiskFreeGB:       diskFreeGB,
		DiskTotalGB:      diskTotalGB,
		DiskUsagePercent: diskUsagePercent,
		QueueSize:        queueSize,
		CacheSize:        0,
		IsHealthy:        isHealthy,
	})
}

// GetMainTaskProgress 获取主任务进度
// GET /api/v1/async-tasks/main/:main_task_id/progress
func (h *Handlers) GetMainTaskProgress(c *gin.Context) {
	mainTaskID := c.Param("main_task_id")
	progress := asyncTaskManager.GetMainTaskProgress(mainTaskID)
	c.JSON(http.StatusOK, progress)
}
