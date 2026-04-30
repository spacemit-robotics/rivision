// Package nodes 提供节点注册表
package nodes

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry 节点注册表
type Registry struct {
	mu          sync.RWMutex
	nodes       map[string]*Node
	clusterName string
	settings    map[string]interface{}
}

// NodesConfig 节点配置文件结构
type NodesConfig struct {
	Cluster  ClusterConfig          `yaml:"cluster"`
	Settings map[string]interface{} `yaml:"settings"`
	Nodes    []NodeConfig           `yaml:"nodes"`
}

// ClusterConfig 集群配置
type ClusterConfig struct {
	Name string `yaml:"name"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID          string            `yaml:"id"`
	Name        string            `yaml:"name"`
	Host        string            `yaml:"host"`
	Port        int               `yaml:"port"`
	AgentPort   int               `yaml:"agent_port"`
	URL         string            `yaml:"url"`
	Model       string            `yaml:"model"`
	Weight      int               `yaml:"weight"`
	MaxParallel int               `yaml:"max_parallel"`
	Enabled     *bool             `yaml:"enabled"`
	Tags        []string          `yaml:"tags"`
	Metadata    map[string]string `yaml:"metadata"`
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		nodes:       make(map[string]*Node),
		clusterName: "default",
		settings:    make(map[string]interface{}),
	}
}

// LoadFromFile 从文件加载节点配置
func (r *Registry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取节点配置文件失败: %w", err)
	}

	var config NodesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("解析节点配置文件失败: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 加载集群信息
	if config.Cluster.Name != "" {
		r.clusterName = config.Cluster.Name
	}

	// 加载设置
	if config.Settings != nil {
		r.settings = config.Settings
	}

	// 清空并重新加载节点
	r.nodes = make(map[string]*Node)

	for _, nc := range config.Nodes {
		// 跳过禁用的节点
		if nc.Enabled != nil && !*nc.Enabled {
			continue
		}

		url := nc.URL
		if url == "" && nc.Host != "" {
			port := nc.Port
			if port == 0 {
				port = 9080
			}
			url = fmt.Sprintf("http://%s:%d", nc.Host, port)
		}

		node := NewNode(nc.ID, nc.Name, url, nc.Model)
		node.Host = nc.Host
		if nc.Port > 0 {
			node.Port = nc.Port
		} else {
			node.Port = 9080
		}
		if nc.AgentPort > 0 {
			node.AgentPort = nc.AgentPort
		} else {
			node.AgentPort = 9090
		}
		if nc.Weight > 0 {
			node.Weight = nc.Weight
		}
		if nc.MaxParallel > 0 {
			node.MaxParallel = nc.MaxParallel
		}
		node.Tags = nc.Tags
		node.Metadata = nc.Metadata
		r.nodes[nc.ID] = node
	}

	return nil
}

// Register 注册节点（保留运行时状态）
func (r *Registry) Register(node *Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 如果节点已存在，保留 ActiveRequests 和 Load
	if existing, ok := r.nodes[node.ID]; ok {
		node.ActiveRequests = existing.ActiveRequests
		node.Load = existing.Load
	}
	r.nodes[node.ID] = node
}

// Unregister 注销节点
func (r *Registry) Unregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeID)
}

// Get 获取节点
func (r *Registry) Get(nodeID string) (*Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	node, ok := r.nodes[nodeID]
	return node, ok
}

// GetAll 获取所有节点（优化版本）
func (r *Registry) GetAll() []*Node {
	r.mu.RLock()
	nodes := make([]*Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, node)
	}
	r.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// GetHealthy 获取健康节点（优化版本）
func (r *Registry) GetHealthy() []*Node {
	r.mu.RLock()
	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
			nodes = append(nodes, node)
		}
	}
	r.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// GetHealthyWithLlama 获取健康且 llama 服务可用的节点
// ★ 必须同时检查 LlamaEnabled (节点配置) 和 LlamaHealthy (服务状态)
func (r *Registry) GetHealthyWithLlama() []*Node {
	r.mu.RLock()
	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
			node.mu.RLock()
			llamaEnabled := node.LlamaEnabled
			llamaHealthy := node.LlamaHealthy
			node.mu.RUnlock()
			// ★ 只选择启用了 llama 服务且服务健康的节点
			if llamaEnabled && llamaHealthy {
				nodes = append(nodes, node)
			}
		}
	}
	r.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// GetHealthyWithYolo 获取健康且 yolo 服务可用的节点
// ★ 必须同时检查 YoloEnabled (节点配置) 和 YoloHealthy (服务状态)
func (r *Registry) GetHealthyWithYolo() []*Node {
	r.mu.RLock()
	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
			node.mu.RLock()
			yoloEnabled := node.YoloEnabled
			yoloHealthy := node.YoloHealthy
			node.mu.RUnlock()
			// ★ 只选择启用了 yolo 服务且服务健康的节点
			if yoloEnabled && yoloHealthy {
				nodes = append(nodes, node)
			}
		}
	}
	r.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// GetAvailable 获取有容量的健康节点（优化版本）
func (r *Registry) GetAvailable() []*Node {
	r.mu.RLock()
	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.HasCapacity() {
			nodes = append(nodes, node)
		}
	}
	r.mu.RUnlock()

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// Count 节点数量
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

// HealthyCount 健康节点数量
func (r *Registry) HealthyCount() int {
	return len(r.GetHealthy())
}

// SetNodeEnabled 设置节点启用状态
func (r *Registry) SetNodeEnabled(nodeID string, enabled bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if node, ok := r.nodes[nodeID]; ok {
		node.mu.Lock()
		node.Enabled = enabled
		node.mu.Unlock()
		return true
	}
	return false
}

// GetNodesByTag 按标签获取节点
func (r *Registry) GetNodesByTag(tag string) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		for _, t := range node.Tags {
			if t == tag && node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
				nodes = append(nodes, node)
				break
			}
		}
	}
	return nodes
}

// GetClusterName 获取集群名称
func (r *Registry) GetClusterName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clusterName
}

// GetSettings 获取配置
func (r *Registry) GetSettings() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := make(map[string]interface{}, len(r.settings))
	for k, v := range r.settings {
		copy[k] = v
	}
	return copy
}

// GetStats 获取统计信息（匹配 Python 的 get_stats）
func (r *Registry) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := len(r.nodes)
	healthy := 0
	disabled := 0
	unhealthy := 0
	totalConns := 0

	for _, node := range r.nodes {
		totalConns += node.ActiveRequests
		if !node.Enabled {
			disabled++
			continue
		}
		if node.IsHealthy() && !node.IsHeartbeatTimeout(60) {
			healthy++
		} else {
			unhealthy++
		}
	}

	return map[string]interface{}{
		"cluster":           r.clusterName,
		"total":             total,
		"healthy":           healthy,
		"disabled":          disabled,
		"unhealthy":         unhealthy,
		"total_connections": totalConns,
	}
}
