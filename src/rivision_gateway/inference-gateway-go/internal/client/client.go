// Package client 提供智能推理客户端
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rivision/inference-gateway/internal/config"
	"github.com/rivision/inference-gateway/internal/nodes"
)

// SmartClient 智能推理客户端
type SmartClient struct {
	cfg            *config.Config
	registry       *nodes.Registry
	selector       *nodes.Selector
	nodeSelector   *nodes.NodeSelector
	healthChecker  *nodes.HealthChecker
	httpClient     *http.Client
	yoloClient     *http.Client
	yoloRR         *YOLORoundRobin
	requestQueue   *RequestQueue
	mu             sync.RWMutex
	running        bool
	shuttingDown   int32 // atomic
}

// AnalyzeRequest 图像分析请求
type AnalyzeRequest struct {
	Image       string  `json:"image"`
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TaskType    string  `json:"task_type,omitempty"`
	TaskID      string  `json:"task_id,omitempty"`
	FrameIndex  int     `json:"frame_index,omitempty"`
	TotalFrames int     `json:"total_frames,omitempty"`
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest 对话请求
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

// InferenceResponse 推理响应
type InferenceResponse struct {
	Status  string                 `json:"status"`
	Content string                 `json:"content"`
	Model   string                 `json:"model,omitempty"`
	Usage   map[string]interface{} `json:"usage,omitempty"`
	NodeID  string                 `json:"node_id,omitempty"`
}

// NewSmartClient 创建智能客户端
func NewSmartClient(cfg *config.Config) *SmartClient {
	registry := nodes.NewRegistry()
	selector := nodes.NewSelector(registry, nodes.LoadBalanceStrategy(cfg.LoadBalanceStrategy))
	nodeSelector := nodes.NewNodeSelector(registry, cfg.MaxConcurrentPerNode)
	healthChecker := nodes.NewHealthChecker(
		registry,
		cfg.HealthCheckInterval,
		cfg.HealthCheckTimeout,
		cfg.UnhealthyThreshold,
	)

	// 共享 HTTP Transport（连接池复用）
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &SmartClient{
		cfg:          cfg,
		registry:     registry,
		selector:     selector,
		nodeSelector: nodeSelector,
		healthChecker: healthChecker,
		httpClient: &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: transport,
		},
		yoloClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
		yoloRR:       NewYOLORoundRobin(),
		requestQueue: NewRequestQueue(cfg.QueueMaxSize, cfg.QueueTimeout),
	}
}

// Start 启动客户端
func (c *SmartClient) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// ★ 静态节点配置已废弃，节点通过 node agent 动态注册
	// 不再尝试加载 nodes.yaml，避免不必要的错误日志

	// 启动健康检查
	if c.cfg.HealthCheckEnabled {
		c.healthChecker.Start()
	}

	c.running = true
	log.Printf("[SmartClient] 已启动，节点数: %d", c.registry.Count())
	return nil
}

// Stop 停止客户端
func (c *SmartClient) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	c.healthChecker.Stop()
	c.running = false
	log.Println("[SmartClient] 已停止")
}

// GetRegistry 获取注册表
func (c *SmartClient) GetRegistry() *nodes.Registry {
	return c.registry
}

// GetSelector 获取选择器
func (c *SmartClient) GetSelector() *nodes.Selector {
	return c.selector
}

// GetNodeSelector 获取智能选择器
func (c *SmartClient) GetNodeSelector() *nodes.NodeSelector {
	return c.nodeSelector
}

// GetYOLORoundRobin 获取 YOLO 轮询调度器
func (c *SmartClient) GetYOLORoundRobin() *YOLORoundRobin {
	return c.yoloRR
}

// GetYOLOClient 获取 YOLO HTTP 客户端
func (c *SmartClient) GetYOLOClient() *http.Client {
	return c.yoloClient
}

// IsShuttingDown 是否正在关闭
func (c *SmartClient) IsShuttingDown() bool {
	return atomic.LoadInt32(&c.shuttingDown) == 1
}

// requestWithNode 核心请求方法：原子选择节点、发送请求、重试
// 匹配 Python 的 request_with_node() 逻辑
func (c *SmartClient) requestWithNode(ctx context.Context, buildReqFn func(node *nodes.Node) ([]byte, error)) (map[string]interface{}, *nodes.Node, error) {
	if c.IsShuttingDown() {
		return nil, nil, fmt.Errorf("网关正在关闭")
	}

	maxRetries := c.cfg.MaxRetries
	excluded := make(map[string]bool)
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 使用 NodeSelector 原子选择并占用节点
		node, acquired := c.nodeSelector.SelectAndAcquire(excluded)
		if !acquired {
			// 等待可用节点（waitForNodeSmart 内部已原子占用）
			node = c.waitForNodeSmart(ctx, excluded)
			if node == nil {
				return nil, nil, fmt.Errorf("等待可用节点超时")
			}
			// 注意：waitForNodeSmart 已通过 SelectAndAcquire 占用节点，无需再次 Increment
		}

		startTime := time.Now()

		// 构建请求数据
		jsonData, err := buildReqFn(node)
		if err != nil {
			c.nodeSelector.ReleaseNode(node)
			return nil, nil, fmt.Errorf("序列化请求失败: %w", err)
		}

		// 发送 HTTP 请求
		url := node.URL + "/v1/chat/completions"
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			c.nodeSelector.ReleaseNode(node)
			return nil, nil, fmt.Errorf("创建请求失败: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		elapsed := time.Since(startTime).Seconds()

		if err != nil {
			c.nodeSelector.ReleaseNode(node)
			isTimeout := ctx.Err() != nil
			c.nodeSelector.RecordFailure(node.ID, isTimeout, true)
			excluded[node.ID] = true
			lastErr = fmt.Errorf("节点 %s 请求失败: %w", node.ID, err)
			log.Printf("[SmartClient] attempt=%d node=%s 失败: %v", attempt, node.ID, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			c.nodeSelector.ReleaseNode(node)
			c.nodeSelector.RecordFailure(node.ID, false, false)
			excluded[node.ID] = true
			lastErr = fmt.Errorf("读取响应失败: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			c.nodeSelector.ReleaseNode(node)
			c.nodeSelector.RecordFailure(node.ID, false, false)
			excluded[node.ID] = true
			lastErr = fmt.Errorf("推理失败: HTTP %d, %s", resp.StatusCode, string(body))
			log.Printf("[SmartClient] attempt=%d node=%s HTTP %d", attempt, node.ID, resp.StatusCode)
			continue
		}

		// 成功
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			c.nodeSelector.ReleaseNode(node)
			return nil, nil, fmt.Errorf("解析响应失败: %w", err)
		}

		c.nodeSelector.ReleaseNode(node)
		c.nodeSelector.RecordSuccess(node.ID, elapsed)
		node.RecordSuccess(float64(elapsed * 1000))

		result["_node_id"] = node.ID
		return result, node, nil
	}

	return nil, nil, fmt.Errorf("所有节点请求失败 (重试 %d 次): %v", maxRetries, lastErr)
}

// AnalyzeImage 分析图像
func (c *SmartClient) AnalyzeImage(ctx context.Context, req *AnalyzeRequest) (*InferenceResponse, error) {
	result, node, err := c.requestWithNode(ctx, func(node *nodes.Node) ([]byte, error) {
		openaiReq := map[string]interface{}{
			"model": node.Model,
			"messages": []map[string]interface{}{
				{
					"role":    "system",
					"content": c.cfg.DefaultSystemPrompt,
				},
				{
					"role": "user",
					"content": []map[string]interface{}{
						{
							"type": "image_url",
							"image_url": map[string]string{
								"url": "data:image/jpeg;base64," + req.Image,
							},
						},
						{
							"type": "text",
							"text": req.Prompt,
						},
					},
				},
			},
			"max_tokens":  req.MaxTokens,
			"temperature": req.Temperature,
		}
		return json.Marshal(openaiReq)
	})

	if err != nil {
		return nil, err
	}

	// 提取内容
	content := extractContent(result)

	var usage map[string]interface{}
	if u, ok := result["usage"].(map[string]interface{}); ok {
		usage = u
	}

	modelName := ""
	if node != nil {
		modelName = node.Model
	}
	if m, ok := result["model"].(string); ok {
		modelName = m
	}

	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	return &InferenceResponse{
		Status:  "success",
		Content: content,
		Model:   modelName,
		NodeID:  nodeID,
		Usage:   usage,
	}, nil
}

// Chat 对话
func (c *SmartClient) Chat(ctx context.Context, req *ChatRequest) (*InferenceResponse, error) {
	result, node, err := c.requestWithNode(ctx, func(node *nodes.Node) ([]byte, error) {
		messages := make([]map[string]interface{}, 0, len(req.Messages)+1)
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": c.cfg.DefaultSystemPrompt,
		})
		for _, m := range req.Messages {
			messages = append(messages, map[string]interface{}{
				"role":    m.Role,
				"content": m.Content,
			})
		}
		openaiReq := map[string]interface{}{
			"model":       node.Model,
			"messages":    messages,
			"max_tokens":  req.MaxTokens,
			"temperature": req.Temperature,
		}
		return json.Marshal(openaiReq)
	})

	if err != nil {
		return nil, err
	}

	content := extractContent(result)

	var usage map[string]interface{}
	if u, ok := result["usage"].(map[string]interface{}); ok {
		usage = u
	}

	modelName := ""
	if node != nil {
		modelName = node.Model
	}
	nodeID := ""
	if node != nil {
		nodeID = node.ID
	}

	return &InferenceResponse{
		Status:  "success",
		Content: content,
		Model:   modelName,
		NodeID:  nodeID,
		Usage:   usage,
	}, nil
}

// waitForNodeSmart 等待可用节点（使用 NodeSelector 原子选择）
func (c *SmartClient) waitForNodeSmart(ctx context.Context, exclude map[string]bool) *nodes.Node {
	timeout := time.After(c.cfg.WaitTimeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timeout:
			return nil
		case <-ticker.C:
			// 使用 SelectAndAcquire 原子选择并占用，避免竞态条件
			if node, acquired := c.nodeSelector.SelectAndAcquire(exclude); acquired {
				return node
			}
		}
	}
}

// extractContent 从 OpenAI 兼容响应中提取内容
func extractContent(result map[string]interface{}) string {
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if c, ok := message["content"].(string); ok && c != "" {
					return c
				}
				if c, ok := message["reasoning_content"].(string); ok {
					return c
				}
			}
		}
	}
	return ""
}

// YOLODetectResponse YOLO检测响应
type YOLODetectResponse struct {
	Success     bool            `json:"success"`
	Detections  []YOLODetection `json:"detections,omitempty"`
	InferenceMs int             `json:"inference_ms,omitempty"`
	NodeID      string          `json:"node_id,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// YOLODetection YOLO检测结果
type YOLODetection struct {
	BBox       []float64 `json:"bbox"`
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float64   `json:"confidence"`
}

// ForwardYOLODetect 转发YOLO检测请求到节点（base64模式）
func (c *SmartClient) ForwardYOLODetect(ctx context.Context, node *nodes.Node, imageBase64 string) (*YOLODetectResponse, error) {
	node.IncrementActiveRequests()
	defer node.DecrementActiveRequests()

	startTime := time.Now()

	// 构建请求
	reqBody := map[string]string{
		"image": imageBase64,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	yoloPort := node.YOLOPort
	if yoloPort == 0 {
		yoloPort = 9081
	}
	url := fmt.Sprintf("http://%s:%d/api/detect", node.Host, yoloPort)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// ★ 使用共享 yoloClient（连接池复用）
	resp, err := c.yoloClient.Do(httpReq)
	if err != nil {
		// ★ 修复: context 取消(客户端断开)不应增加节点失败计数
		if ctx.Err() == nil {
			node.IncrementFailCount()
		}
		return nil, fmt.Errorf("YOLO请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YOLO推理失败: HTTP %d, %s", resp.StatusCode, string(body))
	}

	var result YOLODetectResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	elapsed := time.Since(startTime).Milliseconds()
	node.RecordSuccess(float64(elapsed))

	return &result, nil
}

// ForwardYOLORaw 零拷贝转发原始 JSON body（★ 性能优化：跳过解析和序列化）
// 参考 Python: 直接将原始 JSON body 转发给节点
func (c *SmartClient) ForwardYOLORaw(ctx context.Context, node *nodes.Node, rawBody []byte) (*YOLODetectResponse, error) {
	node.IncrementActiveRequests()
	defer node.DecrementActiveRequests()

	startTime := time.Now()

	yoloPort := node.YOLOPort
	if yoloPort == 0 {
		yoloPort = 9081
	}
	url := fmt.Sprintf("http://%s:%d/api/detect", node.Host, yoloPort)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.yoloClient.Do(httpReq)
	if err != nil {
		// ★ 修复: context 取消(客户端断开)不应增加节点失败计数
		if ctx.Err() == nil {
			node.IncrementFailCount()
		}
		return nil, fmt.Errorf("YOLO请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YOLO推理失败: HTTP %d, %s", resp.StatusCode, string(body))
	}

	var result YOLODetectResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	elapsed := time.Since(startTime).Milliseconds()
	node.RecordSuccess(float64(elapsed))

	return &result, nil
}

// ForwardYOLOBinary 转发二进制YOLO请求（跳过base64，直接发送JPEG）
func (c *SmartClient) ForwardYOLOBinary(ctx context.Context, node *nodes.Node, image []byte) (*YOLODetectResponse, error) {
	node.IncrementActiveRequests()
	defer node.DecrementActiveRequests()

	startTime := time.Now()

	yoloPort := node.YOLOPort
	if yoloPort == 0 {
		yoloPort = 9081
	}
	// ★ 使用二进制端点，直接发送 JPEG 数据
	url := fmt.Sprintf("http://%s:%d/api/detect/binary", node.Host, yoloPort)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(image))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.yoloClient.Do(httpReq)
	if err != nil {
		// ★ 修复: context 取消或 HTTP 超时不应增加节点失败计数
		// HTTP Client Timeout 不会设置 ctx.Err()，需要额外检查 net.Error
		isTimeout := ctx.Err() != nil
		if !isTimeout {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				isTimeout = true
			}
		}
		if !isTimeout {
			node.IncrementFailCount()
		}
		return nil, fmt.Errorf("YOLO请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("YOLO推理失败: HTTP %d, %s", resp.StatusCode, string(body))
	}

	var result YOLODetectResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	elapsed := time.Since(startTime).Milliseconds()
	node.RecordSuccess(float64(elapsed))

	return &result, nil
}

// GracefulShutdown 优雅关闭
func (c *SmartClient) GracefulShutdown(timeout time.Duration) map[string]interface{} {
	atomic.StoreInt32(&c.shuttingDown, 1)

	stats := map[string]interface{}{
		"timeout": timeout.String(),
	}

	// 等待所有请求完成
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allIdle := true
		for _, node := range c.registry.GetAll() {
			if node.GetActiveRequests() > 0 {
				allIdle = false
				break
			}
		}
		if allIdle {
			stats["status"] = "completed"
			return stats
		}
		time.Sleep(100 * time.Millisecond)
	}

	stats["status"] = "timeout"
	return stats
}

// GetStatus 获取服务状态
func (c *SmartClient) GetStatus() map[string]interface{} {
	stats := c.registry.GetStats()
	stats["is_shutting_down"] = c.IsShuttingDown()
	stats["strategy"] = c.cfg.LoadBalanceStrategy
	stats["max_concurrent_per_node"] = c.cfg.MaxConcurrentPerNode
	return stats
}
