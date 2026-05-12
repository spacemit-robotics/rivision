package tasks

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rivision/rivision-hub/internal/gateway/nodes"
)

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxRetries      int
	BaseDelay       float64
	MaxDelay        float64
	ExponentialBase float64
}

// NewRetryPolicy 创建重试策略
func NewRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:      3,
		BaseDelay:       1.0,
		MaxDelay:        30.0,
		ExponentialBase: 2.0,
	}
}

func (p RetryPolicy) GetDelay(retryCount int) float64 {
	delay := p.BaseDelay * math.Pow(p.ExponentialBase, float64(retryCount))
	if delay > p.MaxDelay {
		return p.MaxDelay
	}
	return delay
}

func (p RetryPolicy) ShouldRetry(retryCount int) bool {
	return retryCount < p.MaxRetries
}

// ExecuteFunc 任务执行函数
type ExecuteFunc func(task *GatewayTask, node *nodes.Node) (map[string]interface{}, error)

// TaskScheduler 任务调度器
type TaskScheduler struct {
	mu                  sync.Mutex
	registry            *nodes.Registry
	maxConcurrent       int
	taskTimeout         float64
	retryPolicy         RetryPolicy
	maxConcurrentPerNode int
	stats               map[string]int
}

// NewTaskScheduler 创建任务调度器
func NewTaskScheduler(registry *nodes.Registry, maxConcurrent int, taskTimeout float64, retryPolicy RetryPolicy) *TaskScheduler {
	concPerNode := 1
	if env := os.Getenv("MAX_CONCURRENT_PER_NODE"); env != "" {
		if v, err := strconv.Atoi(env); err == nil {
			concPerNode = v
		}
	}

	s := &TaskScheduler{
		registry:            registry,
		maxConcurrent:       maxConcurrent,
		taskTimeout:         taskTimeout,
		retryPolicy:         retryPolicy,
		maxConcurrentPerNode: concPerNode,
		stats: map[string]int{
			"total_dispatched": 0,
			"total_completed":  0,
			"total_failed":     0,
			"total_retried":    0,
			"total_cancelled":  0,
			"total_timeout":    0,
		},
	}
	log.Printf("[TaskScheduler] 初始化: max_concurrent=%d, timeout=%.0fs", maxConcurrent, taskTimeout)
	return s
}

// SelectNode 选择最优节点（最少连接数）
func (s *TaskScheduler) SelectNode() *nodes.Node {
	healthy := s.registry.GetHealthy()
	if len(healthy) == 0 {
		return nil
	}

	var available []*nodes.Node
	for _, n := range healthy {
		if n.GetActiveRequests() < s.maxConcurrentPerNode {
			available = append(available, n)
		}
	}
	if len(available) == 0 {
		return nil
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].GetActiveRequests() < available[j].GetActiveRequests()
	})
	return available[0]
}

// SelectAndAcquireNode 原子化选择并占用节点
func (s *TaskScheduler) SelectAndAcquireNode() *nodes.Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	node := s.SelectNode()
	if node != nil && node.GetActiveRequests() < s.maxConcurrentPerNode {
		node.IncrementActiveRequests()
		return node
	}
	return nil
}

// SubmitTask 提交任务执行
func (s *TaskScheduler) SubmitTask(task *GatewayTask, executeFunc ExecuteFunc) (map[string]interface{}, error) {
	cache := GetResultCache()
	queue := GetTaskQueueManager()

	// 检查缓存
	if cached, ok := cache.Get(task.DedupKey); ok {
		if result, ok := cached.(map[string]interface{}); ok {
			return result, nil
		}
	}

	// 入队
	if !queue.Enqueue(task) {
		existing := queue.GetTask(task.TaskID)
		if existing != nil && existing.Result != nil {
			return existing.Result, nil
		}
		return nil, nil
	}

	result, err := s.executeWithRetry(task, executeFunc)
	if err != nil {
		if _, ok := err.(*TaskCancelledException); ok {
			queue.UpdateTask(task.TaskID, StateCancelled, err.Error(), nil)
			return nil, err
		}
		queue.UpdateTask(task.TaskID, StateFailed, err.Error(), nil)
		return nil, err
	}

	if result != nil {
		cache.Set(task.DedupKey, result)
	}
	return result, nil
}

func (s *TaskScheduler) executeWithRetry(task *GatewayTask, executeFunc ExecuteFunc) (map[string]interface{}, error) {
	var lastErr error

	for attempt := 0; attempt <= s.retryPolicy.MaxRetries; attempt++ {
		if task.CancellationToken != nil && task.CancellationToken.IsCancelled() {
			return nil, &TaskCancelledException{Reason: task.CancellationToken.Reason()}
		}

		node := s.SelectAndAcquireNode()
		if node == nil {
			return nil, fmt.Errorf("没有可用的推理节点")
		}
		task.NodeID = node.ID

		task.State = StateRunning
		now := time.Now()
		task.StartedAt = &now
		s.mu.Lock()
		s.stats["total_dispatched"]++
		s.mu.Unlock()

		log.Printf("[TaskScheduler] 执行任务: %s, node=%s, attempt=%d", task.TaskID, node.ID, attempt+1)

		result, err := executeFunc(task, node)

		if task.CancellationToken != nil && task.CancellationToken.IsCancelled() {
			node.DecrementActiveRequests()
			return nil, &TaskCancelledException{Reason: task.CancellationToken.Reason()}
		}

		if err == nil {
			// 成功
			task.State = StateCompleted
			completedAt := time.Now()
			task.CompletedAt = &completedAt
			task.Result = result
			node.MarkHealthy()
			s.mu.Lock()
			s.stats["total_completed"]++
			s.mu.Unlock()

			GetTaskQueueManager().UpdateTask(task.TaskID, StateCompleted, "", result)
			node.DecrementActiveRequests()
			return result, nil
		}

		lastErr = err
		node.DecrementActiveRequests()
		node.IncrementFailCount()
		log.Printf("[TaskScheduler] 任务失败: %s, %v", task.TaskID, err)

		if !s.retryPolicy.ShouldRetry(attempt) {
			break
		}

		delay := s.retryPolicy.GetDelay(attempt)
		task.RetryCount = attempt + 1
		s.mu.Lock()
		s.stats["total_retried"]++
		s.mu.Unlock()

		// 分段等待，支持取消检查
		elapsed := 0.0
		for elapsed < delay {
			if task.CancellationToken != nil && task.CancellationToken.IsCancelled() {
				return nil, &TaskCancelledException{Reason: task.CancellationToken.Reason()}
			}
			wait := math.Min(1.0, delay-elapsed)
			time.Sleep(time.Duration(wait * float64(time.Second)))
			elapsed += wait
		}
	}

	task.State = StateFailed
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	s.mu.Lock()
	s.stats["total_failed"]++
	s.mu.Unlock()

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("任务执行失败")
}

// CancelTask 取消任务
func (s *TaskScheduler) CancelTask(taskID, reason string) bool {
	ok := GetTaskQueueManager().CancelTask(taskID, reason)
	if ok {
		s.mu.Lock()
		s.stats["total_cancelled"]++
		s.mu.Unlock()
	}
	return ok
}

// CancelMainTask 取消主任务
func (s *TaskScheduler) CancelMainTask(mainTaskID, reason string) int {
	count := GetTaskQueueManager().CancelMainTask(mainTaskID, reason)
	s.mu.Lock()
	s.stats["total_cancelled"] += count
	s.mu.Unlock()
	return count
}

// GetStats 获取统计
func (s *TaskScheduler) GetStats() map[string]interface{} {
	s.mu.Lock()
	result := make(map[string]interface{})
	for k, v := range s.stats {
		result[k] = v
	}
	s.mu.Unlock()

	queueStats := GetTaskQueueManager().GetStats()
	for k, v := range queueStats {
		result[k] = v
	}
	result["cache_size"] = GetResultCache().Size()
	return result
}

// 全局实例
var (
	globalTaskScheduler *TaskScheduler
	schedulerMu         sync.Mutex
)

func GetTaskScheduler() *TaskScheduler {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	return globalTaskScheduler
}

func InitTaskScheduler(registry *nodes.Registry) {
	schedulerMu.Lock()
	defer schedulerMu.Unlock()
	globalTaskScheduler = NewTaskScheduler(registry, 4, 120.0, NewRetryPolicy())
	log.Println("[TaskScheduler] 任务调度器已初始化")
}
