// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package services 提供网络分区处理
package services

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rivision/inference-gateway/internal/nodes"
)

// CachedNodeState 缓存的节点状态
type CachedNodeState struct {
	ID            string    `json:"id"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	AgentPort     int       `json:"agent_port"`
	URL           string    `json:"url"`
	Weight        int       `json:"weight"`
	Tags          []string  `json:"tags"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CachedAt      time.Time `json:"cached_at"`
}

// NetworkPartitionHandler 网络分区处理器
// 匹配 Python 的 NetworkPartitionHandler：
// - 定期缓存节点状态到磁盘
// - 检测分区（多个节点同时离线）
// - 尝试恢复分区节点
// - gateway 启动时从缓存恢复
type NetworkPartitionHandler struct {
	registry        *nodes.Registry
	discovery       *NodeDiscoveryService
	cacheDir        string
	cacheFile       string
	checkInterval   time.Duration
	heartbeatTimeout int // 秒
	cachedStates    map[string]*CachedNodeState
	stopChan        chan struct{}
	wg              sync.WaitGroup
	running         bool
	mu              sync.Mutex
}

// NewNetworkPartitionHandler 创建分区处理器
func NewNetworkPartitionHandler(registry *nodes.Registry, discovery *NodeDiscoveryService, dataDir string) *NetworkPartitionHandler {
	cacheDir := filepath.Join(dataDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	return &NetworkPartitionHandler{
		registry:         registry,
		discovery:        discovery,
		cacheDir:         cacheDir,
		cacheFile:        filepath.Join(cacheDir, "node_states.json"),
		checkInterval:    30 * time.Second,
		heartbeatTimeout: 60,
		cachedStates:     make(map[string]*CachedNodeState),
		stopChan:         make(chan struct{}),
	}
}

// Start 启动分区处理器
func (h *NetworkPartitionHandler) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	// 启动时加载缓存
	h.loadCache()

	h.wg.Add(1)
	go h.run()
	log.Printf("[PartitionHandler] 已启动，检查间隔: %v", h.checkInterval)
}

// Stop 停止分区处理器
func (h *NetworkPartitionHandler) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	h.mu.Unlock()

	close(h.stopChan)
	h.wg.Wait()

	// 停止前保存缓存
	h.saveCache()
	log.Println("[PartitionHandler] 已停止")
}

func (h *NetworkPartitionHandler) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAndHandle()
		case <-h.stopChan:
			return
		}
	}
}

// checkAndHandle 检查并处理分区
func (h *NetworkPartitionHandler) checkAndHandle() {
	// 1. 缓存当前健康节点状态
	h.cacheCurrentStates()

	// 2. 检测分区
	partitioned := h.detectPartition()
	if len(partitioned) > 0 {
		log.Printf("[PartitionHandler] 检测到 %d 个可能分区的节点", len(partitioned))
		h.recoverPartitioned(partitioned)
	}

	// 3. 定期保存缓存
	h.saveCache()
}

// cacheCurrentStates 缓存当前节点状态
func (h *NetworkPartitionHandler) cacheCurrentStates() {
	h.mu.Lock()
	defer h.mu.Unlock()

	allNodes := h.registry.GetAll()
	for _, node := range allNodes {
		if node.IsHealthy() && !node.IsHeartbeatTimeout(h.heartbeatTimeout) {
			h.cachedStates[node.ID] = &CachedNodeState{
				ID:            node.ID,
				Host:          node.Host,
				Port:          node.Port,
				AgentPort:     node.AgentPort,
				URL:           node.URL,
				Weight:        node.Weight,
				Tags:          node.Tags,
				Status:        "healthy",
				LastHeartbeat: time.Now(),
				CachedAt:      time.Now(),
			}
		}
	}
}

// detectPartition 检测分区节点
func (h *NetworkPartitionHandler) detectPartition() []string {
	allNodes := h.registry.GetAll()
	partitioned := make([]string, 0)
	offlineCount := 0

	for _, node := range allNodes {
		if node.IsHeartbeatTimeout(h.heartbeatTimeout) {
			offlineCount++
			partitioned = append(partitioned, node.ID)
		}
	}

	totalNodes := len(allNodes)
	if totalNodes == 0 {
		return nil
	}

	// 如果超过50%节点离线，可能是网络分区而非节点故障
	offlineRatio := float64(offlineCount) / float64(totalNodes)
	if offlineRatio > 0.5 && offlineCount > 1 {
		log.Printf("[PartitionHandler] 警告: %.0f%% 节点离线 (%d/%d)，疑似网络分区",
			offlineRatio*100, offlineCount, totalNodes)
	}

	return partitioned
}

// recoverPartitioned 尝试恢复分区节点
func (h *NetworkPartitionHandler) recoverPartitioned(nodeIDs []string) {
	if h.discovery == nil {
		return
	}

	for _, nodeID := range nodeIDs {
		node, ok := h.registry.Get(nodeID)
		if !ok {
			continue
		}

		// 尝试探测
		if h.discovery.probeNode(node.Host, node.Port) {
			log.Printf("[PartitionHandler] 节点 %s 已恢复可达", nodeID)
			node.MarkHealthy()
			node.UpdateHeartbeat()
		}
	}
}

// RestoreFromCache 从缓存恢复节点（gateway 启动时调用）
func (h *NetworkPartitionHandler) RestoreFromCache() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.cachedStates) == 0 {
		return 0
	}

	// 仅当 registry 为空时才从缓存恢复
	if h.registry.Count() > 0 {
		return 0
	}

	restored := 0
	for nodeID, cached := range h.cachedStates {
		// 缓存超过1小时的不恢复
		if time.Since(cached.CachedAt) > time.Hour {
			continue
		}

		if _, exists := h.registry.Get(nodeID); exists {
			continue
		}

		node := nodes.NewNode(nodeID, nodeID, cached.URL, "")
		node.Host = cached.Host
		node.Port = cached.Port
		node.AgentPort = cached.AgentPort
		node.Weight = cached.Weight
		node.Tags = cached.Tags
		node.Enabled = true
		// 标记为未知状态，等待心跳更新
		node.SetStatus(nodes.NodeStatusUnknown)

		h.registry.Register(node)
		restored++
		log.Printf("[PartitionHandler] 从缓存恢复节点: %s", nodeID)
	}

	if restored > 0 {
		log.Printf("[PartitionHandler] 共从缓存恢复 %d 个节点", restored)
	}
	return restored
}

// loadCache 从磁盘加载缓存
func (h *NetworkPartitionHandler) loadCache() {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(h.cacheFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[PartitionHandler] 读取缓存失败: %v", err)
		}
		return
	}

	var states map[string]*CachedNodeState
	if err := json.Unmarshal(data, &states); err != nil {
		log.Printf("[PartitionHandler] 解析缓存失败: %v", err)
		return
	}

	h.cachedStates = states
	log.Printf("[PartitionHandler] 从磁盘加载 %d 个缓存节点状态", len(states))
}

// saveCache 保存缓存到磁盘
func (h *NetworkPartitionHandler) saveCache() {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := json.MarshalIndent(h.cachedStates, "", "  ")
	if err != nil {
		log.Printf("[PartitionHandler] 序列化缓存失败: %v", err)
		return
	}

	if err := os.WriteFile(h.cacheFile, data, 0644); err != nil {
		log.Printf("[PartitionHandler] 保存缓存失败: %v", err)
	}
}
