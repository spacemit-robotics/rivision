// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package nodes 提供健康检查器
package nodes

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	registry           *Registry
	interval           time.Duration
	timeout            time.Duration
	unhealthyThreshold int
	client             *http.Client
	stopChan           chan struct{}
	wg                 sync.WaitGroup
	running            bool
	mu                 sync.Mutex
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(registry *Registry, interval, timeout time.Duration, unhealthyThreshold int) *HealthChecker {
	return &HealthChecker{
		registry:           registry,
		interval:           interval,
		timeout:            timeout,
		unhealthyThreshold: unhealthyThreshold,
		client: &http.Client{
			Timeout: timeout,
		},
		stopChan: make(chan struct{}),
	}
}

// Start 启动健康检查
func (h *HealthChecker) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	h.wg.Add(1)
	go h.run()
	log.Printf("[HealthChecker] 已启动，间隔: %v", h.interval)
}

// Stop 停止健康检查
func (h *HealthChecker) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	h.mu.Unlock()

	close(h.stopChan)
	h.wg.Wait()
	log.Println("[HealthChecker] 已停止")
}

// run 运行健康检查循环
func (h *HealthChecker) run() {
	defer h.wg.Done()

	// 立即执行一次检查
	h.checkAll()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.stopChan:
			return
		}
	}
}

// checkAll 检查所有节点（异步，不等待）
func (h *HealthChecker) checkAll() {
	nodes := h.registry.GetAll()

	for _, node := range nodes {
		go func(n *Node) {
			h.checkNode(n)
		}(node)
	}
}

// checkNode 检查单个节点
func (h *HealthChecker) checkNode(node *Node) {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	// 构建健康检查 URL
	healthURL := node.URL + "/health"
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		h.handleCheckFailure(node, err)
		return
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.handleCheckFailure(node, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		h.handleCheckSuccess(node)
	} else {
		h.handleCheckFailure(node, nil)
	}
}

// handleCheckSuccess 处理检查成功
func (h *HealthChecker) handleCheckSuccess(node *Node) {
	wasUnhealthy := !node.IsHealthy()
	node.SetStatus(NodeStatusHealthy)
	node.ResetFailCount()

	if wasUnhealthy {
		log.Printf("[HealthChecker] 节点恢复健康: %s (%s)", node.ID, node.URL)
	}
}

// handleCheckFailure 处理检查失败
func (h *HealthChecker) handleCheckFailure(node *Node, err error) {
	failCount := node.IncrementFailCount()

	if failCount >= h.unhealthyThreshold {
		wasHealthy := node.IsHealthy()
		node.SetStatus(NodeStatusUnhealthy)

		if wasHealthy {
			if err != nil {
				log.Printf("[HealthChecker] 节点不健康: %s (%s), 错误: %v", node.ID, node.URL, err)
			} else {
				log.Printf("[HealthChecker] 节点不健康: %s (%s), 状态码异常", node.ID, node.URL)
			}
		}
	}
}

// CheckNow 立即检查所有节点
func (h *HealthChecker) CheckNow() {
	h.checkAll()
}

// CheckNodeNow 立即检查指定节点
func (h *HealthChecker) CheckNodeNow(nodeID string) bool {
	node, ok := h.registry.Get(nodeID)
	if !ok {
		return false
	}
	h.checkNode(node)
	return node.IsHealthy()
}
