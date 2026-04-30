// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package services 提供节点发现和网络分区处理
package services

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rivision/inference-gateway/internal/auth"
	"github.com/rivision/inference-gateway/internal/nodes"
)

// NodeDiscoveryService 节点发现服务
// 匹配 Python 的 NodeDiscoveryService：
// - 扫描已知网段发现新节点
// - 检查已注册但不在 registry 的节点（gateway 重启恢复）
// - 周期性运行
type NodeDiscoveryService struct {
	registry    *nodes.Registry
	authManager *auth.NodeAuthManager
	knownRanges []string // 已知网段，如 "192.168.1.0/24"
	scanPorts   []int    // 扫描端口列表
	interval    time.Duration
	timeout     time.Duration
	stopChan    chan struct{}
	wg          sync.WaitGroup
	running     bool
	mu          sync.Mutex
}

// NewNodeDiscoveryService 创建发现服务
func NewNodeDiscoveryService(registry *nodes.Registry, authManager *auth.NodeAuthManager) *NodeDiscoveryService {
	return &NodeDiscoveryService{
		registry:    registry,
		authManager: authManager,
		knownRanges: []string{},
		scanPorts:   []int{9080, 9090},
		interval:    5 * time.Minute,
		timeout:     3 * time.Second,
		stopChan:    make(chan struct{}),
	}
}

// SetKnownRanges 设置已知网段
func (d *NodeDiscoveryService) SetKnownRanges(ranges []string) {
	d.knownRanges = ranges
}

// SetInterval 设置扫描间隔
func (d *NodeDiscoveryService) SetInterval(interval time.Duration) {
	d.interval = interval
}

// Start 启动发现服务
func (d *NodeDiscoveryService) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	d.wg.Add(1)
	go d.run()
	log.Printf("[Discovery] 已启动，间隔: %v, 网段: %v", d.interval, d.knownRanges)
}

// Stop 停止发现服务
func (d *NodeDiscoveryService) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	d.mu.Unlock()

	close(d.stopChan)
	d.wg.Wait()
	log.Println("[Discovery] 已停止")
}

func (d *NodeDiscoveryService) run() {
	defer d.wg.Done()

	// 启动时立即执行一次恢复
	d.RecoverFromAuthManager()

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.DiscoverAll()
		case <-d.stopChan:
			return
		}
	}
}

// RecoverFromAuthManager 从认证管理器恢复已注册的节点
// 场景：gateway 重启后 registry 为空，但 auth manager 有持久化数据
func (d *NodeDiscoveryService) RecoverFromAuthManager() {
	if d.authManager == nil {
		return
	}

	registered := d.authManager.GetAllRegisteredNodes()
	recovered := 0

	for nodeID, regNode := range registered {
		if _, exists := d.registry.Get(nodeID); exists {
			continue
		}

		url := fmt.Sprintf("http://%s:%d", regNode.Host, regNode.Port)
		node := nodes.NewNode(nodeID, nodeID, url, "")
		node.Host = regNode.Host
		node.Port = regNode.Port
		node.AgentPort = regNode.AgentPort
		node.Weight = regNode.Weight
		node.Tags = regNode.Tags
		node.Enabled = true

		// 尝试健康检查
		if d.probeNode(regNode.Host, regNode.Port) {
			node.SetStatus(nodes.NodeStatusHealthy)
			node.UpdateHeartbeat()
			d.registry.Register(node)
			recovered++
			log.Printf("[Discovery] 恢复节点: %s (%s:%d)", nodeID, regNode.Host, regNode.Port)
		} else {
			log.Printf("[Discovery] 节点不可达，跳过恢复: %s (%s:%d)", nodeID, regNode.Host, regNode.Port)
		}
	}

	if recovered > 0 {
		log.Printf("[Discovery] 共恢复 %d 个节点", recovered)
	}
}

// DiscoverAll 执行所有发现方式
func (d *NodeDiscoveryService) DiscoverAll() {
	d.RecoverFromAuthManager()
	d.ScanKnownRanges()
}

// ScanKnownRanges 扫描已知网段
func (d *NodeDiscoveryService) ScanKnownRanges() {
	for _, cidr := range d.knownRanges {
		hosts, err := expandCIDR(cidr)
		if err != nil {
			log.Printf("[Discovery] 解析网段 %s 失败: %v", cidr, err)
			continue
		}

		for _, host := range hosts {
			for _, port := range d.scanPorts {
				if d.probeNode(host, port) {
					// 检查是否已在 registry 中
					nodeID := fmt.Sprintf("%s:%d", host, port)
					if _, exists := d.registry.Get(nodeID); !exists {
						log.Printf("[Discovery] 发现新节点: %s", nodeID)
						// 只记录日志，不自动注册（需要 token 认证）
					}
				}
			}
		}
	}
}

// probeNode 探测节点是否可达
func (d *NodeDiscoveryService) probeNode(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)

	// 先尝试 TCP 连接
	conn, err := net.DialTimeout("tcp", addr, d.timeout)
	if err != nil {
		return false
	}
	conn.Close()

	// 尝试 HTTP 健康检查
	url := fmt.Sprintf("http://%s/health", addr)
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return true // TCP 可达但无法构建请求，仍认为可达
	}

	client := &http.Client{Timeout: d.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return true // TCP 可达但 HTTP 不通，仍认为可达
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// expandCIDR 展开 CIDR 为 IP 列表
func expandCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
	}

	// 移除网络地址和广播地址
	if len(ips) > 2 {
		return ips[1 : len(ips)-1], nil
	}
	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
