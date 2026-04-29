package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rivision/inference-gateway/internal/client"
)

// AsyncTaskExecutor 异步任务执行器
type AsyncTaskExecutor struct {
	mu              sync.Mutex
	client          *client.SmartClient
	concPerNode     int
	maxConcurrent   int
	dynamicMode     bool
	pollInterval    time.Duration
	taskTimeout     time.Duration
	persistenceDir  string
	running         int32 // atomic
	stopCh          chan struct{}
	wg              sync.WaitGroup
	runningCount    int32 // atomic: 当前运行中的任务数

	// 统计
	totalExecuted int64
	totalSuccess  int64
	totalFailed   int64
	totalTimeout  int64
}

// NewAsyncTaskExecutor 创建异步任务执行器
func NewAsyncTaskExecutor(smartClient *client.SmartClient, maxConcurrent int,
	pollInterval time.Duration, taskTimeout time.Duration, persistenceDir string) *AsyncTaskExecutor {

	concPerNode := 1
	if env := os.Getenv("MAX_CONCURRENT_PER_NODE"); env != "" {
		fmt.Sscanf(env, "%d", &concPerNode)
	}

	dynamic := false
	if maxConcurrent <= 0 {
		maxConcurrent = concPerNode
		dynamic = true
	}

	if persistenceDir == "" {
		persistenceDir = "./data/tasks"
	}
	os.MkdirAll(filepath.Join(persistenceDir, "results"), 0755)

	e := &AsyncTaskExecutor{
		client:         smartClient,
		concPerNode:    concPerNode,
		maxConcurrent:  maxConcurrent,
		dynamicMode:    dynamic,
		pollInterval:   pollInterval,
		taskTimeout:    taskTimeout,
		persistenceDir: persistenceDir,
		stopCh:         make(chan struct{}),
	}
	log.Printf("[AsyncExecutor] 初始化: max_concurrent=%d, timeout=%v, dynamic=%v",
		maxConcurrent, taskTimeout, dynamic)
	return e
}

// Start 启动执行器
func (e *AsyncTaskExecutor) Start() {
	if !atomic.CompareAndSwapInt32(&e.running, 0, 1) {
		return
	}
	e.updateMaxConcurrent()
	e.wg.Add(1)
	go e.runLoop()
	log.Printf("[AsyncExecutor] 执行器已启动, max_concurrent=%d", e.maxConcurrent)
}

// Stop 停止执行器
func (e *AsyncTaskExecutor) Stop() {
	if !atomic.CompareAndSwapInt32(&e.running, 1, 0) {
		return
	}
	close(e.stopCh)
	e.wg.Wait()
	log.Println("[AsyncExecutor] 执行器已停止")
}

func (e *AsyncTaskExecutor) updateMaxConcurrent() {
	if !e.dynamicMode {
		return
	}
	registry := e.client.GetRegistry()
	healthy := registry.GetHealthy()
	count := len(healthy)
	if count > 0 {
		newMax := count * e.concPerNode
		e.mu.Lock()
		if newMax != e.maxConcurrent {
			log.Printf("[AsyncExecutor] 动态调整并发数: %d -> %d (节点数=%d)",
				e.maxConcurrent, newMax, count)
			e.maxConcurrent = newMax
		}
		e.mu.Unlock()
	}
}

func (e *AsyncTaskExecutor) checkStuckTasks() {
	registry := e.client.GetRegistry()
	queue := GetTaskQueueManager()
	running := queue.GetTasksByState(StateRunning)

	for _, task := range running {
		if task.NodeID != "" {
			node, ok := registry.Get(task.NodeID)
			if !ok || node == nil || !node.IsHealthy() {
				if task.RetryCount < task.MaxRetries {
					task.RetryCount++
					task.State = StateQueued
					task.NodeID = ""
					task.StartedAt = nil
					queue.Enqueue(task)
					log.Printf("[AsyncExecutor] 节点下线，任务重分配: %s", task.TaskID)
				} else {
					queue.UpdateTask(task.TaskID, StateFailed,
						fmt.Sprintf("节点下线，已重试%d次", task.MaxRetries), nil)
				}
			}
		}
	}
}

func (e *AsyncTaskExecutor) runLoop() {
	defer e.wg.Done()
	queue := GetTaskQueueManager()
	updateCounter := 0
	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.updateMaxConcurrent()

			updateCounter++
			if updateCounter >= 10 {
				e.checkStuckTasks()
				updateCounter = 0
			}

			e.mu.Lock()
			currentMax := e.maxConcurrent
			e.mu.Unlock()

			if int(atomic.LoadInt32(&e.runningCount)) >= currentMax {
				continue
			}

			task := queue.Dequeue()
			if task == nil {
				continue
			}

			atomic.AddInt32(&e.runningCount, 1)
			e.wg.Add(1)
			go func(t *GatewayTask) {
				defer e.wg.Done()
				defer atomic.AddInt32(&e.runningCount, -1)
				e.executeTask(t)
			}(task)
		}
	}
}

func (e *AsyncTaskExecutor) executeTask(task *GatewayTask) {
	queue := GetTaskQueueManager()
	cache := GetResultCache()

	queue.UpdateTask(task.TaskID, StateRunning, "", nil)
	task.State = StateRunning
	now := time.Now()
	task.StartedAt = &now

	log.Printf("[AsyncExecutor] 开始执行: %s, frame=%d/%d", task.TaskID, task.FrameIndex, task.TotalFrames)

	result, err := e.doInference(task)
	if err != nil {
		task.RetryCount++
		if task.RetryCount < task.MaxRetries {
			task.State = StateQueued
			task.NodeID = ""
			task.StartedAt = nil
			task.Error = ""
			queue.Enqueue(task)
			log.Printf("[AsyncExecutor] 任务失败，重试 %d/%d: %s", task.RetryCount, task.MaxRetries, task.TaskID)
		} else {
			task.State = StateFailed
			task.Error = fmt.Sprintf("%v（已重试%d次）", err, task.MaxRetries)
			completedAt := time.Now()
			task.CompletedAt = &completedAt
			queue.UpdateTask(task.TaskID, StateFailed, task.Error, nil)
			atomic.AddInt64(&e.totalFailed, 1)
		}
		atomic.AddInt64(&e.totalExecuted, 1)
		return
	}

	if task.CancellationToken != nil && task.CancellationToken.IsCancelled() {
		atomic.AddInt64(&e.totalExecuted, 1)
		return
	}

	task.State = StateCompleted
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	task.Result = result
	if nodeID, ok := result["node_id"].(string); ok {
		task.NodeID = nodeID
	}

	queue.UpdateTask(task.TaskID, StateCompleted, "", result)
	cache.Set(task.TaskID, result, time.Hour)

	e.persistResult(task.TaskID, result)

	atomic.AddInt64(&e.totalSuccess, 1)
	atomic.AddInt64(&e.totalExecuted, 1)
	log.Printf("[AsyncExecutor] 任务完成: %s, elapsed=%.2fs", task.TaskID, task.ElapsedTime())
}

func (e *AsyncTaskExecutor) doInference(task *GatewayTask) (map[string]interface{}, error) {
	payload := task.Payload

	image, _ := payload["image"].(string)
	prompt, _ := payload["prompt"].(string)
	maxTokens := 500
	if mt, ok := payload["max_tokens"].(float64); ok {
		maxTokens = int(mt)
	}
	temp := 0.7
	if t, ok := payload["temperature"].(float64); ok {
		temp = t
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.taskTimeout)
	defer cancel()

	req := &client.AnalyzeRequest{
		Image:       image,
		Prompt:      prompt,
		MaxTokens:   maxTokens,
		Temperature: temp,
		TaskType:    task.TaskType,
		TaskID:      task.MainTaskID,
		FrameIndex:  task.FrameIndex,
		TotalFrames: task.TotalFrames,
	}

	resp, err := e.client.AnalyzeImage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("推理失败: %w", err)
	}

	return map[string]interface{}{
		"content": resp.Content,
		"model":   resp.Model,
		"usage":   resp.Usage,
		"node_id": resp.NodeID,
	}, nil
}

func (e *AsyncTaskExecutor) persistResult(taskID string, result map[string]interface{}) {
	filePath := filepath.Join(e.persistenceDir, "results", taskID+".json")
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	os.WriteFile(filePath, data, 0644)
}

// GetStats 获取统计
func (e *AsyncTaskExecutor) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_executed": atomic.LoadInt64(&e.totalExecuted),
		"total_success":  atomic.LoadInt64(&e.totalSuccess),
		"total_failed":   atomic.LoadInt64(&e.totalFailed),
		"total_timeout":  atomic.LoadInt64(&e.totalTimeout),
		"running_count":  atomic.LoadInt32(&e.runningCount),
		"is_running":     atomic.LoadInt32(&e.running) == 1,
	}
}

// StuckTaskDetector 死任务检测器
type StuckTaskDetector struct {
	maxRunningTime time.Duration
	checkInterval  time.Duration
	running        int32
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

func NewStuckTaskDetector(maxRunningTime, checkInterval time.Duration) *StuckTaskDetector {
	if maxRunningTime == 0 {
		maxRunningTime = 15 * time.Minute
	}
	if checkInterval == 0 {
		checkInterval = time.Minute
	}
	return &StuckTaskDetector{
		maxRunningTime: maxRunningTime,
		checkInterval:  checkInterval,
		stopCh:         make(chan struct{}),
	}
}

func (d *StuckTaskDetector) Start() {
	if !atomic.CompareAndSwapInt32(&d.running, 0, 1) {
		return
	}
	d.wg.Add(1)
	go d.runLoop()
	log.Println("[StuckTaskDetector] 死任务检测器已启动")
}

func (d *StuckTaskDetector) Stop() {
	if !atomic.CompareAndSwapInt32(&d.running, 1, 0) {
		return
	}
	close(d.stopCh)
	d.wg.Wait()
	log.Println("[StuckTaskDetector] 死任务检测器已停止")
}

func (d *StuckTaskDetector) runLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			queue := GetTaskQueueManager()
			running := queue.GetRunningTasks()
			now := time.Now()
			for _, task := range running {
				if task.StartedAt != nil {
					runningTime := now.Sub(*task.StartedAt)
					if runningTime > d.maxRunningTime {
						log.Printf("[StuckTaskDetector] 检测到死任务: %s, 已运行 %.0fs",
							task.TaskID, runningTime.Seconds())
						queue.UpdateTask(task.TaskID, StateFailed,
							fmt.Sprintf("任务卡死超时 (%.0fs)", runningTime.Seconds()), nil)
					}
				}
			}
		}
	}
}

// 全局实例
var (
	globalExecutor     *AsyncTaskExecutor
	globalDetector     *StuckTaskDetector
	executorMu         sync.Mutex
)

func InitAsyncExecutor(smartClient *client.SmartClient) {
	executorMu.Lock()
	defer executorMu.Unlock()

	globalExecutor = NewAsyncTaskExecutor(smartClient, 0,
		500*time.Millisecond, 10*time.Minute, "./data/tasks")
	globalExecutor.Start()

	globalDetector = NewStuckTaskDetector(15*time.Minute, time.Minute)
	globalDetector.Start()

	log.Println("[AsyncExecutor] 异步任务系统已初始化")
}

func ShutdownAsyncExecutor() {
	executorMu.Lock()
	defer executorMu.Unlock()
	if globalDetector != nil {
		globalDetector.Stop()
	}
	if globalExecutor != nil {
		globalExecutor.Stop()
	}
	log.Println("[AsyncExecutor] 异步任务系统已关闭")
}

func GetAsyncExecutor() *AsyncTaskExecutor {
	executorMu.Lock()
	defer executorMu.Unlock()
	return globalExecutor
}
