// Package nodes Worker能力配置管理
// 支持不同类型的Worker节点配置 (yolo-only, vlm-only, yolo+vlm等)
//
// API 端点:
//   - GET  /api/v1/nodes/capabilities          获取集群能力汇总
//   - GET  /api/v1/nodes/:id/capabilities      获取单节点能力
//   - GET  /api/v1/nodes/by-capability/:cap    按能力筛选节点
//   - POST /api/v1/nodes/select                为任务选择最佳节点
//
// 使用示例:
//   registry.GetNodesWithCapability(CapabilityYOLO)
//   registry.SelectNodeForTask("detect")
//   registry.GetCapabilitySummary()
package nodes

import (
	"log"
	"strings"
)

// WorkerCapability Worker能力类型
type WorkerCapability string

const (
	CapabilityYOLO       WorkerCapability = "yolo"        // YOLO目标检测
	CapabilityVLM        WorkerCapability = "vlm"         // VLM视觉语言
	CapabilityCLIP       WorkerCapability = "clip"        // CLIP嵌入
	CapabilityASR        WorkerCapability = "asr"         // 语音识别
	CapabilityAction     WorkerCapability = "action"      // 动作识别
	CapabilityFace       WorkerCapability = "face"        // 人脸识别
	CapabilityOCR        WorkerCapability = "ocr"         // 文字识别
)

// WorkerType 预定义Worker类型
type WorkerType string

const (
	WorkerTypeYOLOOnly   WorkerType = "yolo-only"    // 仅YOLO检测
	WorkerTypeVLMOnly    WorkerType = "vlm-only"     // 仅VLM分析
	WorkerTypeYOLOVLM    WorkerType = "yolo+vlm"     // YOLO + VLM
	WorkerTypeFull       WorkerType = "full"         // 全功能
	WorkerTypeEmbed      WorkerType = "embed"        // 仅嵌入服务
	WorkerTypeCustom     WorkerType = "custom"       // 自定义配置
)

// WorkerTypeConfig Worker类型配置
type WorkerTypeConfig struct {
	Type         WorkerType         `json:"type" yaml:"type"`
	Capabilities []WorkerCapability `json:"capabilities" yaml:"capabilities"`
	Description  string             `json:"description" yaml:"description"`
}

// 预定义Worker类型配置
var WorkerTypeConfigs = map[WorkerType]*WorkerTypeConfig{
	WorkerTypeYOLOOnly: {
		Type:         WorkerTypeYOLOOnly,
		Capabilities: []WorkerCapability{CapabilityYOLO},
		Description:  "仅YOLO目标检测，适合实时监控场景",
	},
	WorkerTypeVLMOnly: {
		Type:         WorkerTypeVLMOnly,
		Capabilities: []WorkerCapability{CapabilityVLM},
		Description:  "仅VLM视觉语言模型，适合语义理解场景",
	},
	WorkerTypeYOLOVLM: {
		Type:         WorkerTypeYOLOVLM,
		Capabilities: []WorkerCapability{CapabilityYOLO, CapabilityVLM},
		Description:  "YOLO + VLM，检测+理解组合",
	},
	WorkerTypeFull: {
		Type:         WorkerTypeFull,
		Capabilities: []WorkerCapability{CapabilityYOLO, CapabilityVLM, CapabilityCLIP, CapabilityAction},
		Description:  "全功能节点，支持所有能力",
	},
	WorkerTypeEmbed: {
		Type:         WorkerTypeEmbed,
		Capabilities: []WorkerCapability{CapabilityCLIP},
		Description:  "仅嵌入服务，用于搜索索引",
	},
}

// NodeCapabilities 节点能力信息
type NodeCapabilities struct {
	Type         WorkerType           `json:"type"`
	Capabilities []WorkerCapability   `json:"capabilities"`
	Services     map[string]bool      `json:"services"`     // 服务状态 (yolo: true, vlm: false...)
	Models       map[string]string    `json:"models"`       // 加载的模型
	Hardware     *HardwareInfo        `json:"hardware"`     // 硬件信息
}

// HardwareInfo 硬件信息
type HardwareInfo struct {
	Platform     string  `json:"platform"`       // riscv64 (K3 SpacemiT)
	CPUCores     int     `json:"cpu_cores"`
	MemoryGB     float64 `json:"memory_gb"`
	HasGPU       bool    `json:"has_gpu"`
	GPUModel     string  `json:"gpu_model,omitempty"`
	HasNPU       bool    `json:"has_npu"`
	NPUModel     string  `json:"npu_model,omitempty"` // SpacemiT, Rockchip...
}

// GetNodeCapabilities 从Node获取能力信息
func GetNodeCapabilities(node *Node) *NodeCapabilities {
	if node == nil {
		return nil
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	caps := &NodeCapabilities{
		Type:         DetermineWorkerType(node),
		Capabilities: make([]WorkerCapability, 0),
		Services:     make(map[string]bool),
		Models:       make(map[string]string),
	}

	// 根据服务状态添加能力
	if node.YoloEnabled && node.YoloHealthy {
		caps.Capabilities = append(caps.Capabilities, CapabilityYOLO)
		caps.Services["yolo"] = true
	}

	if node.LlamaEnabled && node.LlamaHealthy {
		caps.Capabilities = append(caps.Capabilities, CapabilityVLM)
		caps.Services["vlm"] = true
	}

	// 从Metadata获取更多能力
	if node.Metadata != nil {
		if v, ok := node.Metadata["clip_enabled"]; ok && v == "true" {
			caps.Capabilities = append(caps.Capabilities, CapabilityCLIP)
			caps.Services["clip"] = true
		}
		if v, ok := node.Metadata["asr_enabled"]; ok && v == "true" {
			caps.Capabilities = append(caps.Capabilities, CapabilityASR)
			caps.Services["asr"] = true
		}
		if v, ok := node.Metadata["model_yolo"]; ok {
			caps.Models["yolo"] = v
		}
		if v, ok := node.Metadata["model_vlm"]; ok {
			caps.Models["vlm"] = v
		}
	}

	return caps
}

// DetermineWorkerType 根据服务状态推断Worker类型
func DetermineWorkerType(node *Node) WorkerType {
	if node == nil {
		return WorkerTypeCustom
	}

	hasYolo := node.YoloEnabled
	hasVlm := node.LlamaEnabled

	if hasYolo && hasVlm {
		return WorkerTypeYOLOVLM
	}
	if hasYolo && !hasVlm {
		return WorkerTypeYOLOOnly
	}
	if !hasYolo && hasVlm {
		return WorkerTypeVLMOnly
	}

	return WorkerTypeCustom
}

// HasCapability 检查节点是否有指定能力
func HasCapability(node *Node, cap WorkerCapability) bool {
	if node == nil {
		return false
	}

	node.mu.RLock()
	defer node.mu.RUnlock()

	switch cap {
	case CapabilityYOLO:
		return node.YoloEnabled && node.YoloHealthy
	case CapabilityVLM:
		return node.LlamaEnabled && node.LlamaHealthy
	case CapabilityCLIP:
		if node.Metadata != nil {
			return node.Metadata["clip_enabled"] == "true"
		}
		return false
	default:
		return false
	}
}

// GetNodesWithCapability 获取具有指定能力的节点
func (r *Registry) GetNodesWithCapability(cap WorkerCapability) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
			if HasCapability(node, cap) {
				nodes = append(nodes, node)
			}
		}
	}

	return nodes
}

// GetNodesWithCapabilities 获取具有多个能力的节点
func (r *Registry) GetNodesWithCapabilities(caps []WorkerCapability) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if !node.IsHealthy() || !node.Enabled || node.IsHeartbeatTimeout(60) {
			continue
		}

		hasAll := true
		for _, cap := range caps {
			if !HasCapability(node, cap) {
				hasAll = false
				break
			}
		}

		if hasAll {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

// GetNodesByType 按Worker类型获取节点
func (r *Registry) GetNodesByType(workerType WorkerType) []*Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]*Node, 0)
	for _, node := range r.nodes {
		if node.IsHealthy() && node.Enabled && !node.IsHeartbeatTimeout(60) {
			if DetermineWorkerType(node) == workerType {
				nodes = append(nodes, node)
			}
		}
	}

	return nodes
}

// SelectNodeForTask 根据任务类型选择最佳节点
func (r *Registry) SelectNodeForTask(taskType string) *Node {
	var requiredCaps []WorkerCapability

	// 根据任务类型确定所需能力
	switch strings.ToLower(taskType) {
	case "detect", "detection", "yolo":
		requiredCaps = []WorkerCapability{CapabilityYOLO}
	case "describe", "vlm", "chat", "analyze":
		requiredCaps = []WorkerCapability{CapabilityVLM}
	case "search", "embed", "embedding":
		requiredCaps = []WorkerCapability{CapabilityCLIP}
	case "full", "detect+analyze":
		requiredCaps = []WorkerCapability{CapabilityYOLO, CapabilityVLM}
	default:
		// 默认返回任意健康节点
		healthy := r.GetHealthy()
		if len(healthy) > 0 {
			return healthy[0]
		}
		return nil
	}

	// 获取满足能力要求的节点
	candidates := r.GetNodesWithCapabilities(requiredCaps)
	if len(candidates) == 0 {
		log.Printf("[NodeSelector] 没有满足 %v 能力要求的节点", requiredCaps)
		return nil
	}

	// 选择负载最低的节点
	var bestNode *Node
	var lowestLoad float64 = 100

	for _, node := range candidates {
		node.mu.RLock()
		load := node.Load
		active := node.ActiveRequests
		maxParallel := node.MaxParallel
		node.mu.RUnlock()

		// 跳过满载节点
		if active >= maxParallel {
			continue
		}

		// 选择负载最低的
		if bestNode == nil || load < lowestLoad {
			bestNode = node
			lowestLoad = load
		}
	}

	return bestNode
}

// GetCapabilitySummary 获取集群能力汇总
func (r *Registry) GetCapabilitySummary() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary := map[string]int{
		"total":    0,
		"healthy":  0,
		"yolo":     0,
		"vlm":      0,
		"yolo+vlm": 0,
		"clip":     0,
	}

	for _, node := range r.nodes {
		summary["total"]++

		if !node.IsHealthy() || !node.Enabled || node.IsHeartbeatTimeout(60) {
			continue
		}
		summary["healthy"]++

		hasYolo := HasCapability(node, CapabilityYOLO)
		hasVlm := HasCapability(node, CapabilityVLM)

		if hasYolo {
			summary["yolo"]++
		}
		if hasVlm {
			summary["vlm"]++
		}
		if hasYolo && hasVlm {
			summary["yolo+vlm"]++
		}
		if HasCapability(node, CapabilityCLIP) {
			summary["clip"]++
		}
	}

	return summary
}
