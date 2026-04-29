// Package nodes 提供节点选择器
package nodes

import (
	"log"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalanceStrategy 负载均衡策略
type LoadBalanceStrategy string

const (
	StrategyRoundRobin LoadBalanceStrategy = "round_robin"
	StrategyRandom     LoadBalanceStrategy = "random"
	StrategyLeastConn  LoadBalanceStrategy = "least_conn"
	StrategyWeighted   LoadBalanceStrategy = "weighted"
)

// ============================================
// NodeBlacklist - 节点短期黑名单
// ============================================

// NodeBlacklist 节点黑名单（短期排除刚失败的节点）
type NodeBlacklist struct {
	mu       sync.RWMutex
	entries  map[string]time.Time // node_id -> expiry_time
	duration time.Duration        // 默认黑名单持续时间
}

// NewNodeBlacklist 创建黑名单
func NewNodeBlacklist(defaultDuration time.Duration) *NodeBlacklist {
	return &NodeBlacklist{
		entries:  make(map[string]time.Time),
		duration: defaultDuration,
	}
}

// Add 添加到黑名单
func (b *NodeBlacklist) Add(nodeID string, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if duration == 0 {
		duration = b.duration
	}
	b.entries[nodeID] = time.Now().Add(duration)
}

// Remove 从黑名单移除
func (b *NodeBlacklist) Remove(nodeID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, nodeID)
}

// IsBlacklisted 检查是否在黑名单中
func (b *NodeBlacklist) IsBlacklisted(nodeID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	expiry, ok := b.entries[nodeID]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		return false
	}
	return true
}

// Cleanup 清理过期条目
func (b *NodeBlacklist) Cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for k, v := range b.entries {
		if now.After(v) {
			delete(b.entries, k)
		}
	}
}

// ============================================
// NodeScore - 多维度节点评分
// ============================================

// NodeScore 节点评分
type NodeScore struct {
	NodeID           string
	TotalScore       float64
	HealthScore      float64 // 0-100
	LoadScore        float64 // 0-100, 越低越好
	LatencyScore     float64 // 0-100, 越低越好
	SuccessRateScore float64 // 0-100
}

// CalculateTotal 计算总分
func (s *NodeScore) CalculateTotal(healthW, loadW, latencyW, successW float64) {
	s.TotalScore = s.HealthScore*healthW +
		(100-s.LoadScore)*loadW +
		(100-s.LatencyScore)*latencyW +
		s.SuccessRateScore*successW
}

// ============================================
// NodeStats - 节点统计
// ============================================

type nodeStats struct {
	TotalRequests int64
	SuccessCount  int64
	FailureCount  int64
	TimeoutCount  int64
	TotalLatency  float64
	LastSuccess   time.Time
	LastFailure   time.Time
}

// ============================================
// NodeSelector - 智能节点选择器
// ============================================

// NodeSelector 智能节点选择器（匹配 Python 的 NodeSelector）
type NodeSelector struct {
	registry        *Registry
	blacklist       *NodeBlacklist
	maxConcurrent   int
	stats           map[string]*nodeStats
	statsMu         sync.RWMutex
	selectionMu     sync.Mutex // 全局选择锁，防止竞态条件
	healthWeight    float64
	loadWeight      float64
	latencyWeight   float64
	successWeight   float64
}

// NewNodeSelector 创建智能节点选择器
func NewNodeSelector(registry *Registry, maxConcurrent int) *NodeSelector {
	return &NodeSelector{
		registry:      registry,
		blacklist:     NewNodeBlacklist(30 * time.Second),
		maxConcurrent: maxConcurrent,
		stats:         make(map[string]*nodeStats),
		healthWeight:  0.4,
		loadWeight:    0.3,
		latencyWeight: 0.15,
		successWeight: 0.15,
	}
}

// getOrCreateStats 获取或创建节点统计
func (ns *NodeSelector) getOrCreateStats(nodeID string) *nodeStats {
	ns.statsMu.Lock()
	defer ns.statsMu.Unlock()
	s, ok := ns.stats[nodeID]
	if !ok {
		s = &nodeStats{}
		ns.stats[nodeID] = s
	}
	return s
}

// GetAvailableNodes 获取可用节点列表（排除不健康、满载、黑名单、OOM节点）
func (ns *NodeSelector) GetAvailableNodes(exclude map[string]bool) []*Node {
	ns.blacklist.Cleanup()

	healthy := ns.registry.GetHealthy()
	available := make([]*Node, 0, len(healthy))

	for _, node := range healthy {
		if exclude != nil && exclude[node.ID] {
			continue
		}
		if ns.blacklist.IsBlacklisted(node.ID) {
			continue
		}
		if node.GetActiveRequests() >= ns.maxConcurrent {
			continue
		}
		// ★ VLM 任务需要 llama-server 健康
		node.mu.RLock()
		llamaHealthy := node.LlamaHealthy
		memPercent := 0.0
		gpuMem := 0.0
		if node.Resources != nil {
			memPercent = node.Resources["memory_percent"]
			gpuMem = node.Resources["gpu_memory_percent"]
		}
		node.mu.RUnlock()
		if !llamaHealthy {
			continue
		}
		// OOM 保护
		if memPercent > 90 {
			log.Printf("[NodeSelector] 节点 %s 内存不足 (%.0f%%)，跳过", node.ID, memPercent)
			continue
		}
		if gpuMem > 95 {
			log.Printf("[NodeSelector] 节点 %s GPU显存不足 (%.0f%%)，跳过", node.ID, gpuMem)
			continue
		}

		available = append(available, node)
	}
	return available
}

// ScoreNode 对节点进行多维度评分
func (ns *NodeSelector) ScoreNode(node *Node) *NodeScore {
	score := &NodeScore{NodeID: node.ID}
	stats := ns.getOrCreateStats(node.ID)

	// 1. 健康度评分
	if node.IsHealthy() {
		score.HealthScore = 100.0
	}

	// 2. 负载评分 (连接数占比)
	if ns.maxConcurrent > 0 {
		loadPercent := float64(node.GetActiveRequests()) / float64(ns.maxConcurrent) * 100
		if loadPercent > 100 {
			loadPercent = 100
		}
		score.LoadScore = loadPercent
	}

	// 3. 延迟评分
	ns.statsMu.RLock()
	if stats.SuccessCount > 0 {
		avgLatency := stats.TotalLatency / float64(stats.SuccessCount)
		latencyNorm := avgLatency / 60.0
		if latencyNorm > 1.0 {
			latencyNorm = 1.0
		}
		score.LatencyScore = latencyNorm * 100
	} else {
		score.LatencyScore = 50.0 // 无数据时中等评分
	}

	// 4. 成功率评分
	if stats.TotalRequests > 0 {
		successRate := float64(stats.SuccessCount) / float64(stats.TotalRequests)
		score.SuccessRateScore = successRate * 100
	} else {
		score.SuccessRateScore = 100.0 // 无数据时满分
	}
	ns.statsMu.RUnlock()

	score.CalculateTotal(ns.healthWeight, ns.loadWeight, ns.latencyWeight, ns.successWeight)
	return score
}

// Select 选择最优节点
func (ns *NodeSelector) Select(exclude map[string]bool) *Node {
	available := ns.GetAvailableNodes(exclude)
	if len(available) == 0 {
		return nil
	}

	// 评分并排序
	type scored struct {
		node  *Node
		score *NodeScore
	}
	scoredNodes := make([]scored, 0, len(available))
	for _, node := range available {
		scoredNodes = append(scoredNodes, scored{node: node, score: ns.ScoreNode(node)})
	}
	sort.Slice(scoredNodes, func(i, j int) bool {
		return scoredNodes[i].score.TotalScore > scoredNodes[j].score.TotalScore
	})

	// 当多个节点分数接近时（差距<5%），随机选择以实现负载均衡
	topScore := scoredNodes[0].score.TotalScore
	threshold := topScore * 0.95 // 95% 阈值
	candidates := make([]scored, 0)
	for _, s := range scoredNodes {
		if s.score.TotalScore >= threshold {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) > 1 {
		return candidates[rand.Intn(len(candidates))].node
	}
	return scoredNodes[0].node
}

// SelectAndAcquire 原子化选择并占用节点（防止竞态条件）
func (ns *NodeSelector) SelectAndAcquire(exclude map[string]bool) (*Node, bool) {
	ns.selectionMu.Lock()
	defer ns.selectionMu.Unlock()

	node := ns.Select(exclude)
	if node == nil {
		return nil, false
	}

	// 在锁内再次检查并占用
	if node.GetActiveRequests() < ns.maxConcurrent {
		node.IncrementActiveRequests()
		return node, true
	}
	return nil, false
}

// ReleaseNode 释放节点占用
func (ns *NodeSelector) ReleaseNode(node *Node) {
	if node != nil {
		node.DecrementActiveRequests()
	}
}

// RecordSuccess 记录成功
func (ns *NodeSelector) RecordSuccess(nodeID string, latency float64) {
	stats := ns.getOrCreateStats(nodeID)
	ns.statsMu.Lock()
	stats.TotalRequests++
	stats.SuccessCount++
	stats.TotalLatency += latency
	stats.LastSuccess = time.Now()
	ns.statsMu.Unlock()

	// 从黑名单移除
	ns.blacklist.Remove(nodeID)
}

// RecordFailure 记录失败
func (ns *NodeSelector) RecordFailure(nodeID string, isTimeout, isConnectError bool) {
	stats := ns.getOrCreateStats(nodeID)
	ns.statsMu.Lock()
	stats.TotalRequests++
	stats.FailureCount++
	stats.LastFailure = time.Now()
	if isTimeout {
		stats.TimeoutCount++
	}
	ns.statsMu.Unlock()

	// 根据错误类型决定黑名单时长
	var duration time.Duration
	if isConnectError {
		duration = 60 * time.Second
	} else if isTimeout {
		duration = 10 * time.Second
	} else {
		duration = 30 * time.Second
	}
	ns.blacklist.Add(nodeID, duration)
}

// ============================================
// Selector - 基础负载均衡选择器（保留兼容性）
// ============================================

// Selector 节点选择器
type Selector struct {
	registry *Registry
	strategy LoadBalanceStrategy
	counter  uint64
	mu       sync.RWMutex
}

// NewSelector 创建选择器
func NewSelector(registry *Registry, strategy LoadBalanceStrategy) *Selector {
	return &Selector{
		registry: registry,
		strategy: strategy,
	}
}

// Select 选择一个节点
func (s *Selector) Select() *Node {
	nodes := s.registry.GetAvailable()
	if len(nodes) == 0 {
		return nil
	}

	switch s.strategy {
	case StrategyRoundRobin:
		return s.roundRobin(nodes)
	case StrategyRandom:
		return s.random(nodes)
	case StrategyLeastConn:
		return s.leastConn(nodes)
	case StrategyWeighted:
		return s.weighted(nodes)
	default:
		return s.leastConn(nodes)
	}
}

// SelectHealthy 选择一个健康节点（不考虑容量）
func (s *Selector) SelectHealthy() *Node {
	nodes := s.registry.GetHealthy()
	if len(nodes) == 0 {
		return nil
	}

	switch s.strategy {
	case StrategyRoundRobin:
		return s.roundRobin(nodes)
	case StrategyRandom:
		return s.random(nodes)
	case StrategyLeastConn:
		return s.leastConn(nodes)
	case StrategyWeighted:
		return s.weighted(nodes)
	default:
		return s.leastConn(nodes)
	}
}

// roundRobin 轮询策略
func (s *Selector) roundRobin(nodes []*Node) *Node {
	idx := atomic.AddUint64(&s.counter, 1) % uint64(len(nodes))
	return nodes[idx]
}

// random 随机策略
func (s *Selector) random(nodes []*Node) *Node {
	return nodes[rand.Intn(len(nodes))]
}

// leastConn 最少连接策略
func (s *Selector) leastConn(nodes []*Node) *Node {
	var selected *Node
	minConn := int(^uint(0) >> 1) // MaxInt

	for _, node := range nodes {
		active := node.GetActiveRequests()
		if active < minConn {
			minConn = active
			selected = node
		}
	}

	return selected
}

// weighted 加权策略
func (s *Selector) weighted(nodes []*Node) *Node {
	totalWeight := 0
	for _, node := range nodes {
		totalWeight += node.Weight
	}

	if totalWeight == 0 {
		return s.random(nodes)
	}

	r := rand.Intn(totalWeight)
	for _, node := range nodes {
		r -= node.Weight
		if r < 0 {
			return node
		}
	}

	return nodes[0]
}

// SetStrategy 设置策略
func (s *Selector) SetStrategy(strategy LoadBalanceStrategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.strategy = strategy
}

// GetStrategy 获取策略
func (s *Selector) GetStrategy() LoadBalanceStrategy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.strategy
}
