// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// TaskMonitor 任务监控器
type TaskMonitor struct {
	taskTimeout        time.Duration
	cleanupInterval    time.Duration
	staleTaskThreshold time.Duration
	running            int32
	stopCh             chan struct{}
	wg                 sync.WaitGroup

	mu    sync.Mutex
	stats map[string]int64
}

// NewTaskMonitor 创建任务监控器
func NewTaskMonitor(taskTimeout, cleanupInterval, staleTaskThreshold time.Duration) *TaskMonitor {
	if taskTimeout == 0 {
		taskTimeout = 20 * time.Minute
	}
	if cleanupInterval == 0 {
		cleanupInterval = time.Minute
	}
	if staleTaskThreshold == 0 {
		staleTaskThreshold = 30 * time.Minute
	}
	m := &TaskMonitor{
		taskTimeout:        taskTimeout,
		cleanupInterval:    cleanupInterval,
		staleTaskThreshold: staleTaskThreshold,
		stopCh:             make(chan struct{}),
		stats: map[string]int64{
			"tasks_timed_out": 0,
			"tasks_cleaned":   0,
		},
	}
	log.Printf("[TaskMonitor] 初始化: timeout=%v, cleanup_interval=%v", taskTimeout, cleanupInterval)
	return m
}

func (m *TaskMonitor) Start() {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return
	}
	m.wg.Add(1)
	go m.monitorLoop()
	log.Println("[TaskMonitor] 监控已启动")
}

func (m *TaskMonitor) Stop() {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return
	}
	close(m.stopCh)
	m.wg.Wait()
	log.Println("[TaskMonitor] 监控已停止")
}

func (m *TaskMonitor) monitorLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()
	cleanupCounter := 0

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkTimeouts()
			m.cleanupStaleTasks()
			m.cleanupCache()

			cleanupCounter++
			if cleanupCounter >= 60 {
				m.cleanupOldData()
				cleanupCounter = 0
			}
		}
	}
}

func (m *TaskMonitor) checkTimeouts() {
	// 异步模式下禁用TaskMonitor的超时检测
	// 异步任务由AsyncExecutor自己管理超时
}

func (m *TaskMonitor) cleanupStaleTasks() {
	queue := GetTaskQueueManager()
	queue.CleanupCompleted(m.staleTaskThreshold)
}

func (m *TaskMonitor) cleanupCache() {
	GetResultCache().CleanupExpired()
}

func (m *TaskMonitor) cleanupOldData() {
	resultsDir := filepath.Join(".", "data", "tasks", "results")
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	deleted := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(resultsDir, entry.Name()))
				deleted++
			}
		}
	}
	if deleted > 0 {
		log.Printf("[TaskMonitor] 文件清理: 删除了 %d 个旧结果文件", deleted)
	}
}

func (m *TaskMonitor) GetStats() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]interface{})
	for k, v := range m.stats {
		result[k] = v
	}
	result["running"] = atomic.LoadInt32(&m.running) == 1
	return result
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	mu            sync.RWMutex
	nodeResources map[string]map[string]interface{}
}

func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		nodeResources: make(map[string]map[string]interface{}),
	}
}

func (r *ResourceMonitor) UpdateNodeResources(nodeID string, resources map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resources["updated_at"] = time.Now().Format(time.RFC3339)
	r.nodeResources[nodeID] = resources
}

func (r *ResourceMonitor) GetNodeResources(nodeID string) map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nodeResources[nodeID]
}

func (r *ResourceMonitor) CheckNodeAvailable(nodeID string, minMemoryMB int, maxCPUPercent float64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.nodeResources[nodeID]
	if !ok {
		return true
	}
	if mem, ok := res["available_memory_mb"].(float64); ok && mem < float64(minMemoryMB) {
		return false
	}
	if cpu, ok := res["cpu_percent"].(float64); ok && cpu > maxCPUPercent {
		return false
	}
	return true
}

func (r *ResourceMonitor) GetAllResources() map[string]map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]map[string]interface{}, len(r.nodeResources))
	for k, v := range r.nodeResources {
		result[k] = v
	}
	return result
}

// 全局实例
var (
	globalTaskMonitor     *TaskMonitor
	globalResourceMonitor *ResourceMonitor
	monitorOnce           sync.Once
)

func GetTaskMonitor() *TaskMonitor {
	monitorOnce.Do(func() {
		globalTaskMonitor = NewTaskMonitor(0, 0, 0)
		globalResourceMonitor = NewResourceMonitor()
	})
	return globalTaskMonitor
}

func GetResourceMonitor() *ResourceMonitor {
	monitorOnce.Do(func() {
		globalTaskMonitor = NewTaskMonitor(0, 0, 0)
		globalResourceMonitor = NewResourceMonitor()
	})
	return globalResourceMonitor
}

func StartMonitoring() {
	GetTaskMonitor().Start()
}

func StopMonitoring() {
	GetTaskMonitor().Stop()
}
