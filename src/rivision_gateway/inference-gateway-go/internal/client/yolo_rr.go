// Package client 提供 YOLO 专用轮询调度器
package client

import (
	"log"
	"sync"
	"time"

	"github.com/rivision/inference-gateway/internal/nodes"
)

// YOLORoundRobin YOLO 专用轮询节点选择器
// 设计要点：
// - 不检查 node.connections（不被 VLM 阻塞）
// - Round-robin 保证所有节点均匀分配
// - 短时黑名单（5s）跳过故障节点
// - 统计每节点延迟用于监控
type YOLORoundRobin struct {
	index     int
	mu        sync.Mutex
	blacklist map[string]time.Time       // node_id -> expire_time
	stats     map[string]*yoloNodeStats
}

type yoloNodeStats struct {
	Total            int64
	Success          int64
	Fail             int64
	TotalMs          int64
	ConsecutiveFails int   // ★ 连续失败次数（用于指数退避）
}

// NewYOLORoundRobin 创建 YOLO 轮询调度器
func NewYOLORoundRobin() *YOLORoundRobin {
	return &YOLORoundRobin{
		blacklist: make(map[string]time.Time),
		stats:     make(map[string]*yoloNodeStats),
	}
}

// Select 轮询选择下一个健康且不在黑名单中的节点
func (rr *YOLORoundRobin) Select(allNodes []*nodes.Node) *nodes.Node {
	if len(allNodes) == 0 {
		return nil
	}

	rr.mu.Lock()
	defer rr.mu.Unlock()

	now := time.Now()

	// 清理过期黑名单
	for nid, exp := range rr.blacklist {
		if now.After(exp) {
			delete(rr.blacklist, nid)
		}
	}

	// 过滤可用节点：健康 + YOLO启用 + YOLO健康 + 不在黑名单
	available := make([]*nodes.Node, 0, len(allNodes))
	for _, n := range allNodes {
		// ★ 必须检查 YoloEnabled（节点是否启用YOLO服务）和 YoloHealthy（服务是否健康）
		if n.IsHealthy() && n.Enabled && n.YoloEnabled && n.YoloHealthy {
			if _, blacklisted := rr.blacklist[n.ID]; !blacklisted {
				available = append(available, n)
			}
		}
	}

	if len(available) == 0 {
		// 降级1: 忽略黑名单，但仍需 YOLO 启用且健康
		for _, n := range allNodes {
			if n.IsHealthy() && n.Enabled && n.YoloEnabled && n.YoloHealthy {
				available = append(available, n)
			}
		}
	}
	if len(available) == 0 {
		// ★ 降级2: YOLO 启用但不健康（可能正在启动中）
		// 注意：不再降级到"任何健康节点"，避免向 node_llama 发送 YOLO 请求
		log.Printf("[YOLO] 警告: 没有 YoloHealthy 节点")
		for _, n := range allNodes {
			if n.IsHealthy() && n.Enabled && n.YoloEnabled {
				if _, blacklisted := rr.blacklist[n.ID]; !blacklisted {
					available = append(available, n)
				}
			}
		}
	}
	if len(available) == 0 {
		return nil
	}

	node := available[rr.index%len(available)]
	rr.index++
	return node
}

// RecordSuccess 记录成功
func (rr *YOLORoundRobin) RecordSuccess(nodeID string, elapsedMs int) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	s := rr.getStats(nodeID)
	s.Total++
	s.Success++
	s.TotalMs += int64(elapsedMs)
	s.ConsecutiveFails = 0 // ★ 成功时重置连续失败计数
	// 成功时移出黑名单
	delete(rr.blacklist, nodeID)
}

// RecordFailure 记录失败，加入指数退避黑名单
// ★ 优化：使用指数退避，连续失败越多，黑名单时间越长
// 第1次失败: 3s, 第2次: 6s, 第3次: 12s, 最大: 60s
func (rr *YOLORoundRobin) RecordFailure(nodeID string, baseBlacklistSec float64) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	s := rr.getStats(nodeID)
	s.Total++
	s.Fail++
	s.ConsecutiveFails++
	
	// ★ 指数退避计算：base * 2^(fails-1)，最大 60 秒
	blacklistSec := baseBlacklistSec
	for i := 1; i < s.ConsecutiveFails && blacklistSec < 60; i++ {
		blacklistSec *= 2
	}
	if blacklistSec > 60 {
		blacklistSec = 60
	}
	
	rr.blacklist[nodeID] = time.Now().Add(time.Duration(blacklistSec * float64(time.Second)))
	log.Printf("[YOLO] 节点 %s 加入黑名单 %.0fs (连续失败 %d 次)", nodeID, blacklistSec, s.ConsecutiveFails)
}

// GetStats 获取统计信息（包含黑名单）
func (rr *YOLORoundRobin) GetStats() map[string]map[string]interface{} {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	now := time.Now()
	result := make(map[string]map[string]interface{})
	for nid, s := range rr.stats {
		avgMs := float64(0)
		if s.Success > 0 {
			avgMs = float64(s.TotalMs) / float64(s.Success)
		}
		// ★ 添加黑名单状态
		blacklisted := false
		blacklistRemainSec := float64(0)
		if exp, ok := rr.blacklist[nid]; ok && now.Before(exp) {
			blacklisted = true
			blacklistRemainSec = exp.Sub(now).Seconds()
		}
		result[nid] = map[string]interface{}{
			"total":                s.Total,
			"success":              s.Success,
			"fail":                 s.Fail,
			"avg_ms":               avgMs,
			"consecutive_fails":    s.ConsecutiveFails,
			"blacklisted":          blacklisted,
			"blacklist_remain_sec": blacklistRemainSec,
		}
	}
	return result
}

func (rr *YOLORoundRobin) getStats(nodeID string) *yoloNodeStats {
	s, ok := rr.stats[nodeID]
	if !ok {
		s = &yoloNodeStats{}
		rr.stats[nodeID] = s
	}
	return s
}
