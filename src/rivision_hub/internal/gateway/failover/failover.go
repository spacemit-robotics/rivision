// Package failover 自动故障转移
package failover

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rivision/rivision-hub/internal/gateway/nodes"
)

// FailoverManager 故障转移管理器
type FailoverManager struct {
	registry          *nodes.Registry
	cameraStore       CameraStore
	reassigner        CameraReassigner // complete reassignment with stream lifecycle
	heartbeatTimeout  int           // 心跳超时时间(秒)
	checkInterval     time.Duration // 检查间隔
	failoverThreshold int           // 连续失败次数阈值

	mu              sync.RWMutex
	nodeFailCounts  map[string]int      // 节点连续失败次数
	nodeCameras     map[string][]string // 节点-摄像头映射
	cameraNodes     map[string]string   // 摄像头-节点映射
	lastCheck       time.Time
	failoverHistory []FailoverEvent

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CameraStore 摄像头存储接口
type CameraStore interface {
	GetCamerasByNode(nodeID string) []Camera
	AssignCamera(cameraID, nodeID string) error
	UpdateCameraStatus(cameraID, status string) error
}

// CameraReassigner handles the complete camera reassignment lifecycle:
//   1. Notify old Worker to remove stream
//   2. Deactivate SIP session (hub_sip_remote: SIP BYE + cache invalidation)
//   3. Store-level reassignment (update node_id)
//   4. Activate stream on new Worker (hub_sip_remote: PlayRemote to new ZLM)
//   5. Notify new Worker to add stream
//
// This ensures SIP sessions are properly torn down and re-established during failover,
// preventing session leaks when Workers go offline.
type CameraReassigner interface {
	// ReassignCamera performs the full reassignment flow for a single camera.
	// Returns an error if any critical step fails (store update, stream activation).
	// Worker notification failures are logged but not fatal.
	ReassignCamera(ctx context.Context, cameraID, newNodeID string) error
}

// Camera 摄像头信息
type Camera struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	NodeID   string `json:"node_id"`
	Status   string `json:"status"`
}

// FailoverEvent 故障转移事件
type FailoverEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	CameraID    string    `json:"camera_id"`
	FromNodeID  string    `json:"from_node_id"`
	ToNodeID    string    `json:"to_node_id"`
	Reason      string    `json:"reason"`
	Success     bool      `json:"success"`
	ErrorMsg    string    `json:"error_msg,omitempty"`
}

// Config 配置
type Config struct {
	HeartbeatTimeout  int           `yaml:"heartbeat_timeout"`  // 心跳超时(秒)
	CheckInterval     time.Duration `yaml:"check_interval"`     // 检查间隔
	FailoverThreshold int           `yaml:"failover_threshold"` // 故障阈值
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		HeartbeatTimeout:  60,
		CheckInterval:     30 * time.Second,
		FailoverThreshold: 3,
	}
}

// NewFailoverManager 创建故障转移管理器
func NewFailoverManager(registry *nodes.Registry, cameraStore CameraStore, cfg Config) *FailoverManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &FailoverManager{
		registry:          registry,
		cameraStore:       cameraStore,
		heartbeatTimeout:  cfg.HeartbeatTimeout,
		checkInterval:     cfg.CheckInterval,
		failoverThreshold: cfg.FailoverThreshold,
		nodeFailCounts:    make(map[string]int),
		nodeCameras:       make(map[string][]string),
		cameraNodes:       make(map[string]string),
		failoverHistory:   make([]FailoverEvent, 0),
		ctx:               ctx,
		cancel:            cancel,
	}
}

// SetReassigner sets the CameraReassigner for complete stream lifecycle management.
// When set, failover uses ReassignCamera instead of simple store AssignCamera,
// ensuring SIP sessions are properly torn down and re-established.
func (f *FailoverManager) SetReassigner(r CameraReassigner) {
	f.reassigner = r
}

// Start 启动故障转移监控
func (f *FailoverManager) Start() {
	f.wg.Add(1)
	go f.monitorLoop()
	log.Printf("[Failover] 故障转移管理器已启动, 检查间隔=%v, 超时=%ds, 阈值=%d",
		f.checkInterval, f.heartbeatTimeout, f.failoverThreshold)
}

// Stop 停止故障转移监控
func (f *FailoverManager) Stop() {
	f.cancel()
	f.wg.Wait()
	log.Printf("[Failover] 故障转移管理器已停止")
}

// monitorLoop 监控循环
func (f *FailoverManager) monitorLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.checkAndFailover()
		}
	}
}

// checkAndFailover 检查并执行故障转移
func (f *FailoverManager) checkAndFailover() {
	f.mu.Lock()
	f.lastCheck = time.Now()
	f.mu.Unlock()

	allNodes := f.registry.GetAll()
	
	for _, node := range allNodes {
		if !node.Enabled {
			continue
		}

		isTimeout := node.IsHeartbeatTimeout(f.heartbeatTimeout)
		isHealthy := node.IsHealthy()

		if isTimeout || !isHealthy {
			f.mu.Lock()
			f.nodeFailCounts[node.ID]++
			failCount := f.nodeFailCounts[node.ID]
			f.mu.Unlock()

			if failCount >= f.failoverThreshold {
				log.Printf("[Failover] 节点 %s 连续失败 %d 次，触发故障转移", node.ID, failCount)
				f.executeFailover(node.ID)
			} else {
				log.Printf("[Failover] 节点 %s 检测失败 (%d/%d)", node.ID, failCount, f.failoverThreshold)
			}
		} else {
			// 节点恢复，重置计数
			f.mu.Lock()
			if f.nodeFailCounts[node.ID] > 0 {
				log.Printf("[Failover] 节点 %s 已恢复", node.ID)
				f.nodeFailCounts[node.ID] = 0
			}
			f.mu.Unlock()
		}
	}
}

// executeFailover 执行故障转移
func (f *FailoverManager) executeFailover(failedNodeID string) {
	if f.cameraStore == nil {
		log.Printf("[Failover] 摄像头存储未配置，跳过故障转移")
		return
	}

	// 获取故障节点上的摄像头
	cameras := f.cameraStore.GetCamerasByNode(failedNodeID)
	if len(cameras) == 0 {
		log.Printf("[Failover] 节点 %s 没有分配摄像头", failedNodeID)
		return
	}

	log.Printf("[Failover] 开始迁移节点 %s 的 %d 个摄像头", failedNodeID, len(cameras))

	// 获取可用的健康节点
	healthyNodes := f.registry.GetHealthy()
	if len(healthyNodes) == 0 {
		log.Printf("[Failover] 没有可用的健康节点，无法执行故障转移")
		return
	}

	// 过滤掉故障节点
	availableNodes := make([]*nodes.Node, 0)
	for _, n := range healthyNodes {
		if n.ID != failedNodeID {
			availableNodes = append(availableNodes, n)
		}
	}

	if len(availableNodes) == 0 {
		log.Printf("[Failover] 没有其他可用节点，无法执行故障转移")
		return
	}

	// 分配摄像头到可用节点（轮询）
	for i, cam := range cameras {
		targetNode := availableNodes[i%len(availableNodes)]

		event := FailoverEvent{
			Timestamp:  time.Now(),
			CameraID:   cam.ID,
			FromNodeID: failedNodeID,
			ToNodeID:   targetNode.ID,
			Reason:     "node_failure",
		}

		// Use CameraReassigner if available (handles SIP BYE + Worker notify + stream activate).
		// Falls back to simple store assignment if reassigner is not set.
		var err error
		if f.reassigner != nil {
			ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
			f.cameraStore.UpdateCameraStatus(cam.ID, "migrating")
			err = f.reassigner.ReassignCamera(ctx, cam.ID, targetNode.ID)
			cancel()
		} else {
			// Legacy path: store-only assignment (no stream lifecycle control)
			err = f.cameraStore.AssignCamera(cam.ID, targetNode.ID)
			if err == nil {
				f.cameraStore.UpdateCameraStatus(cam.ID, "migrating")
			}
		}

		if err != nil {
			event.Success = false
			event.ErrorMsg = err.Error()
			log.Printf("[Failover] 摄像头 %s 迁移失败: %v", cam.ID, err)
		} else {
			event.Success = true
			log.Printf("[Failover] 摄像头 %s 已迁移: %s -> %s", cam.ID, failedNodeID, targetNode.ID)
		}

		f.mu.Lock()
		f.failoverHistory = append(f.failoverHistory, event)
		// 保留最近100条记录
		if len(f.failoverHistory) > 100 {
			f.failoverHistory = f.failoverHistory[len(f.failoverHistory)-100:]
		}
		f.mu.Unlock()
	}

	// 重置故障计数
	f.mu.Lock()
	f.nodeFailCounts[failedNodeID] = 0
	f.mu.Unlock()
}

// ManualFailover 手动触发故障转移
func (f *FailoverManager) ManualFailover(fromNodeID, toNodeID string, cameraIDs []string) []FailoverEvent {
	events := make([]FailoverEvent, 0)

	for _, camID := range cameraIDs {
		event := FailoverEvent{
			Timestamp:  time.Now(),
			CameraID:   camID,
			FromNodeID: fromNodeID,
			ToNodeID:   toNodeID,
			Reason:     "manual",
		}

		if f.reassigner != nil {
			ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
			err := f.reassigner.ReassignCamera(ctx, camID, toNodeID)
			cancel()
			if err != nil {
				event.Success = false
				event.ErrorMsg = err.Error()
			} else {
				event.Success = true
			}
		} else if f.cameraStore != nil {
			err := f.cameraStore.AssignCamera(camID, toNodeID)
			if err != nil {
				event.Success = false
				event.ErrorMsg = err.Error()
			} else {
				event.Success = true
			}
		} else {
			event.Success = false
			event.ErrorMsg = "camera store not configured"
		}

		events = append(events, event)

		f.mu.Lock()
		f.failoverHistory = append(f.failoverHistory, event)
		f.mu.Unlock()
	}

	return events
}

// GetStats 获取统计信息
func (f *FailoverManager) GetStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	failedNodes := make([]string, 0)
	for nodeID, count := range f.nodeFailCounts {
		if count >= f.failoverThreshold {
			failedNodes = append(failedNodes, nodeID)
		}
	}

	recentEvents := f.failoverHistory
	if len(recentEvents) > 20 {
		recentEvents = recentEvents[len(recentEvents)-20:]
	}

	return map[string]interface{}{
		"last_check":        f.lastCheck,
		"heartbeat_timeout": f.heartbeatTimeout,
		"check_interval":    f.checkInterval.String(),
		"threshold":         f.failoverThreshold,
		"failed_nodes":      failedNodes,
		"fail_counts":       f.nodeFailCounts,
		"recent_events":     recentEvents,
		"total_failovers":   len(f.failoverHistory),
	}
}

// GetHistory 获取故障转移历史
func (f *FailoverManager) GetHistory(limit int) []FailoverEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if limit <= 0 || limit > len(f.failoverHistory) {
		limit = len(f.failoverHistory)
	}

	// 返回最近的记录
	start := len(f.failoverHistory) - limit
	if start < 0 {
		start = 0
	}

	result := make([]FailoverEvent, limit)
	copy(result, f.failoverHistory[start:])
	return result
}

// RegisterNode 注册节点（节点恢复时调用）
func (f *FailoverManager) RegisterNode(nodeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodeFailCounts[nodeID] = 0
	log.Printf("[Failover] 节点 %s 已注册", nodeID)
}

// UnregisterNode 注销节点
func (f *FailoverManager) UnregisterNode(nodeID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.nodeFailCounts, nodeID)
	delete(f.nodeCameras, nodeID)
	log.Printf("[Failover] 节点 %s 已注销", nodeID)
}
