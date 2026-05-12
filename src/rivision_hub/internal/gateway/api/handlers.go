// Package api 提供 HTTP API 处理器
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nfnt/resize"
	"github.com/rivision/rivision-hub/internal/gateway/client"
	"github.com/rivision/rivision-hub/internal/gateway/config"
	"github.com/rivision/rivision-hub/internal/gateway/nodes"
)

// ★ VLM 异步任务并发限制
// 注意：每个节点的 LLM 是串行处理的，同时只能处理 1 个 VLM 请求
var (
	vlmAsyncTaskCount int64 // 当前异步任务数
	vlmAsyncMaxLimit  int64 = 8 // 硬上限（防止内存爆炸）
)

// ============================================
// VLM 历史存储
// ============================================

// VLMHistoryItem VLM 分析历史记录
type VLMHistoryItem struct {
	TaskID         string `json:"task_id"`
	CameraID       string `json:"camera_id"`
	StreamName     string `json:"stream_name"`
	Result         string `json:"result"`
	Thumbnail      string `json:"thumbnail,omitempty"`
	NodeID         string `json:"node_id"`
	ProcessingTime int64  `json:"processing_time"`
	Resolution     string `json:"resolution"`
	Timestamp      int64  `json:"timestamp"`
}

// VLMHistoryStore VLM 历史存储
type VLMHistoryStore struct {
	mu       sync.RWMutex
	items    []VLMHistoryItem
	maxItems int
}

// 全局 VLM 历史存储
var vlmHistoryStore = &VLMHistoryStore{
	items:    make([]VLMHistoryItem, 0),
	maxItems: 500, // 最多存储 500 条记录
}

// Add 添加历史记录
func (s *VLMHistoryStore) Add(item VLMHistoryItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 插入到头部（最新的在前）
	s.items = append([]VLMHistoryItem{item}, s.items...)
	
	// 限制总数
	if len(s.items) > s.maxItems {
		s.items = s.items[:s.maxItems]
	}
}

// GetByCamera 按摄像头获取历史记录
func (s *VLMHistoryStore) GetByCamera(cameraID string, limit int) []VLMHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if limit <= 0 {
		limit = 100
	}
	
	if cameraID == "" {
		// 返回所有
		if limit > len(s.items) {
			limit = len(s.items)
		}
		result := make([]VLMHistoryItem, limit)
		copy(result, s.items[:limit])
		return result
	}
	
	// 按摄像头过滤
	result := make([]VLMHistoryItem, 0, limit)
	for _, item := range s.items {
		if item.CameraID == cameraID || item.StreamName == cameraID {
			result = append(result, item)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

// GetAll 获取所有历史记录
func (s *VLMHistoryStore) GetAll(limit int) []VLMHistoryItem {
	return s.GetByCamera("", limit)
}

// Count 获取记录数
func (s *VLMHistoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// getVLMAsyncLimit 动态计算 VLM 异步并发上限
// ★ 统一规则：每个 llama 节点最多 1 个任务（llama 是串行的）
func (h *Handlers) getVLMAsyncLimit() int64 {
	registry := h.client.GetRegistry()
	// ★ 使用 GetHealthyWithLlama() 获取真正可用的 llama 节点数
	llamaNodes := registry.GetHealthyWithLlama()
	llamaCount := int64(len(llamaNodes))
	if llamaCount == 0 {
		return 1 // 至少允许1个任务（等待节点恢复）
	}
	// ★ 统一：每 llama 节点 1 个任务，与前端和清理检查保持一致
	// 1 节点 = 1 任务
	// 2 节点 = 2 任务
	if llamaCount > vlmAsyncMaxLimit {
		return vlmAsyncMaxLimit
	}
	return llamaCount
}

// broadcastTasksUpdate 实时推送任务状态变化（无需轮询）
func broadcastTasksUpdate() {
	vlmRunning := atomic.LoadInt64(&vlmAsyncTaskCount)
	tasks := make([]map[string]interface{}, 0)
	for i := int64(0); i < vlmRunning; i++ {
		tasks = append(tasks, map[string]interface{}{
			"id":       fmt.Sprintf("vlm-task-%d", i+1),
			"type":     "image",
			"status":   "running",
			"progress": 50,
		})
	}
	wsManager.Broadcast("tasks", map[string]interface{}{
		"tasks":       tasks,
		"total":       len(tasks),
		"vlm_running": vlmRunning,
	})
}

// Handlers API 处理器
type Handlers struct {
	cfg    *config.Config
	client *client.SmartClient
}

// NewHandlers 创建处理器
func NewHandlers(cfg *config.Config, c *client.SmartClient) *Handlers {
	return &Handlers{
		cfg:    cfg,
		client: c,
	}
}

// AnalyzeRequest 图像分析请求
type AnalyzeRequest struct {
	Image       string  `json:"image" binding:"required"`
	Prompt      string  `json:"prompt" binding:"required"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TaskType    string  `json:"task_type"`
	TaskID      string  `json:"task_id"`
	FrameIndex  int     `json:"frame_index"`
	TotalFrames int     `json:"total_frames"`
	CameraID    string  `json:"camera_id"`
	StreamName  string  `json:"stream_name"`
}

// ChatMessage 对话消息
type ChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ChatRequest 对话请求
type ChatRequest struct {
	Messages    []ChatMessage `json:"messages" binding:"required"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

// AnalyzeImage 分析图像
// POST /api/v1/inference/analyze
func (h *Handlers) AnalyzeImage(c *gin.Context) {
	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 设置默认值
	if req.MaxTokens == 0 {
		req.MaxTokens = 500
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	// 检查是否异步模式
	isAsync := c.Query("async") == "true"
	
	if isAsync {
		// ★ 并发限制检查（参考 Python: 防止 OOM）
		limit := h.getVLMAsyncLimit()
		current := atomic.LoadInt64(&vlmAsyncTaskCount)
		if current >= limit {
			log.Printf("[VLM Async] 队列已满 (%d/%d), 拒绝请求", current, limit)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "rejected",
				"message": "推理队列已满，请稍后重试",
			})
			return
		}
		
		// ★ 预检查：确保有健康的 llama 节点可用
		availableNodes := h.client.GetRegistry().GetHealthyWithLlama()
		if len(availableNodes) == 0 {
			log.Printf("[VLM Async] 无可用的 llama 节点，拒绝请求")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "rejected",
				"message": "无可用的推理节点，请稍后重试",
			})
			return
		}
		
		// 异步模式：立即返回 202，后台处理
		taskID := req.TaskID
		if taskID == "" {
			taskID = fmt.Sprintf("vlm_%d", time.Now().UnixNano())
		}
		
		// ★ 递增计数器并立即推送任务状态
		atomic.AddInt64(&vlmAsyncTaskCount, 1)
		log.Printf("[VLM Async] 任务入队: %s (%d/%d), 可用节点: %d", taskID, atomic.LoadInt64(&vlmAsyncTaskCount), limit, len(availableNodes))
		broadcastTasksUpdate() // ★ 实时推送任务变化
		
		// 后台执行推理
		go func(cameraID, streamName, imageBase64 string) {
			// ★ 完成时递减计数器并推送状态
			defer func() {
				atomic.AddInt64(&vlmAsyncTaskCount, -1)
				log.Printf("[VLM Async] 任务完成: %s (剩余 %d)", taskID, atomic.LoadInt64(&vlmAsyncTaskCount))
				broadcastTasksUpdate() // ★ 实时推送任务变化
			}()
			
			startTime := time.Now()
			// ★ 超时改为 120s（更快检测节点故障）
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			
			// ★ 后台监控：每 5 秒检查 llama 节点状态和任务槽位
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						llamaNodes := h.client.GetRegistry().GetHealthyWithLlama()
						llamaCount := int64(len(llamaNodes))
						currentTasks := atomic.LoadInt64(&vlmAsyncTaskCount)
						
						// ★ 情况1：无可用 llama 节点，取消任务
						if llamaCount == 0 {
							log.Printf("[VLM Async] task=%s 无可用 llama 节点，取消任务", taskID)
							cancel()
							return
						}
						
						// ★ 情况2：任务数超过可用槽位 + 1（容错缓冲）
						// 允许短暂超出1个任务，避免竞态条件下误取消
						if currentTasks > llamaCount+1 {
							log.Printf("[VLM Async] task=%s 任务数(%d)显著超过可用槽位(%d)，取消任务", taskID, currentTasks, llamaCount)
							cancel()
							return
						}
					}
				}
			}()
			
			// ★ 生成缩略图（参考 Python 实现）
			thumbnail, resolution := generateThumbnail(imageBase64)
			
			result, err := h.client.AnalyzeImage(ctx, &client.AnalyzeRequest{
				Image:       imageBase64,
				Prompt:      req.Prompt,
				MaxTokens:   req.MaxTokens,
				Temperature: req.Temperature,
				TaskType:    req.TaskType,
				TaskID:      taskID,
				FrameIndex:  req.FrameIndex,
				TotalFrames: req.TotalFrames,
			})
			
			processingTime := time.Since(startTime).Milliseconds()
			
			if err != nil {
				// ★ G2: 区分超时和其他错误类型
				errorType := "error"
				errorMsg := err.Error()
				if ctx.Err() == context.DeadlineExceeded {
					errorType = "timeout"
					errorMsg = "推理超时(120s)"
					log.Printf("[VLM Async] task=%s 超时(120s)", taskID)
				} else if ctx.Err() == context.Canceled {
					errorType = "cancelled"
					errorMsg = "任务已取消"
					log.Printf("[VLM Async] task=%s 已取消: %v", taskID, err)
				} else {
					log.Printf("[VLM Async] task=%s 失败: %v", taskID, err)
				}
				wsManager.BroadcastResult(map[string]interface{}{
					"task_id":     taskID,
					"camera_id":   cameraID,
					"stream_name": streamName,
					"error":       errorMsg,
					"error_type":  errorType,
					"timestamp":   time.Now().UnixMilli(),
				})
				return
			}
			
			log.Printf("[VLM Async] task=%s 完成: %s", taskID, truncateString(result.Content, 50))
			
			timestamp := time.Now().UnixMilli()
			
			// ★ 保存到历史存储
			vlmHistoryStore.Add(VLMHistoryItem{
				TaskID:         taskID,
				CameraID:       cameraID,
				StreamName:     streamName,
				Result:         result.Content,
				Thumbnail:      thumbnail,
				NodeID:         result.NodeID,
				ProcessingTime: processingTime,
				Resolution:     resolution,
				Timestamp:      timestamp,
			})
			log.Printf("[VLM History] 已保存历史记录, 当前总数: %d", vlmHistoryStore.Count())
			
			// ★ 通过 WebSocket 推送结果（包含 thumbnail）
			wsManager.BroadcastResult(map[string]interface{}{
				"task_id":         taskID,
				"camera_id":       cameraID,
				"stream_name":     streamName,
				"result":          result.Content,
				"thumbnail":       thumbnail,
				"node_id":         result.NodeID,
				"processing_time": processingTime,
				"resolution":      resolution,
				"timestamp":       timestamp,
			})
		}(req.CameraID, req.StreamName, req.Image)
		
		c.JSON(http.StatusAccepted, gin.H{
			"status":  "accepted",
			"task_id": taskID,
			"message": "任务已提交，结果将通过 WebSocket 推送",
		})
		return
	}

	// 同步模式：等待推理完成
	result, err := h.client.AnalyzeImage(c.Request.Context(), &client.AnalyzeRequest{
		Image:       req.Image,
		Prompt:      req.Prompt,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TaskType:    req.TaskType,
		TaskID:      req.TaskID,
		FrameIndex:  req.FrameIndex,
		TotalFrames: req.TotalFrames,
	})

	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "等待可用节点超时" {
			statusCode = http.StatusServiceUnavailable
		}
		c.JSON(statusCode, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// generateThumbnail 生成缩略图 (320x200)
// 参考 Python: img.thumbnail((320, 200), Image.Resampling.LANCZOS)
func generateThumbnail(imageBase64 string) (thumbnail string, resolution string) {
	log.Printf("[Thumbnail] 开始生成, 输入长度: %d", len(imageBase64))
	
	imgData, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		log.Printf("[Thumbnail] base64 解码失败: %v", err)
		return "", ""
	}
	log.Printf("[Thumbnail] base64 解码成功, 图片数据: %d bytes", len(imgData))

	img, format, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Printf("[Thumbnail] 图片解码失败: %v", err)
		return "", ""
	}
	log.Printf("[Thumbnail] 图片解码成功, 格式: %s", format)

	// 获取原始分辨率
	bounds := img.Bounds()
	resolution = fmt.Sprintf("%dx%d", bounds.Dx(), bounds.Dy())
	log.Printf("[Thumbnail] 原始分辨率: %s", resolution)

	// 缩放到 320x200（保持比例）
	thumb := resize.Thumbnail(320, 200, img, resize.Lanczos3)
	thumbBounds := thumb.Bounds()
	log.Printf("[Thumbnail] 缩放后: %dx%d", thumbBounds.Dx(), thumbBounds.Dy())

	// 编码为 JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 85}); err != nil {
		log.Printf("[Thumbnail] JPEG 编码失败: %v", err)
		return "", resolution
	}

	thumbnail = base64.StdEncoding.EncodeToString(buf.Bytes())
	log.Printf("[Thumbnail] 生成成功, 缩略图长度: %d", len(thumbnail))
	return thumbnail, resolution
}

// GetInferenceHistory 获取推理历史
// GET /api/v1/inference/history
// 支持查询参数:
//   - camera_id: 按摄像头ID过滤
//   - limit: 返回数量限制 (默认100)
func (h *Handlers) GetInferenceHistory(c *gin.Context) {
	cameraID := c.Query("camera_id")
	limit := 100
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}
	
	items := vlmHistoryStore.GetByCamera(cameraID, limit)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   items,
		"total":   len(items),
	})
}

// Chat 对话
// POST /api/v1/inference/chat
func (h *Handlers) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "无效的请求参数: " + err.Error(),
		})
		return
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = 500
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	// 转换消息格式
	messages := make([]client.ChatMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = client.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	result, err := h.client.Chat(c.Request.Context(), &client.ChatRequest{
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	})

	if err != nil {
		statusCode := http.StatusInternalServerError
		if err.Error() == "等待可用节点超时" {
			statusCode = http.StatusServiceUnavailable
		}
		c.JSON(statusCode, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Health 健康检查
// GET /health
func (h *Handlers) Health(c *gin.Context) {
	registry := h.client.GetRegistry()
	
	c.JSON(http.StatusOK, gin.H{
		"status":        "healthy",
		"total_nodes":   registry.Count(),
		"healthy_nodes": registry.HealthyCount(),
	})
}

// GetNodes 获取节点列表
// GET /api/v1/nodes
func (h *Handlers) GetNodes(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodeList := registry.GetAll()

	data := make([]map[string]interface{}, len(nodeList))
	for i, node := range nodeList {
		data[i] = node.ToDict()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"nodes":   data, // CLI 使用此字段
		"data":    data, // 兼容旧格式
		"total":   len(data),
	})
}

// GetNode 获取单个节点
// GET /api/v1/nodes/:id
func (h *Handlers) GetNode(c *gin.Context) {
	nodeID := c.Param("id")
	registry := h.client.GetRegistry()

	node, ok := registry.Get(nodeID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "节点不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    node.ToDict(),
	})
}

// GetStats 获取统计信息
// GET /api/v1/stats
func (h *Handlers) GetStats(c *gin.Context) {
	registry := h.client.GetRegistry()
	nodes := registry.GetAll()

	var totalRequests, successRequests int64
	var totalResponseTime float64
	for _, node := range nodes {
		stats := node.GetStats()
		totalRequests += stats["total_requests"].(int64)
		successRequests += stats["success_requests"].(int64)
		totalResponseTime += stats["avg_response_time"].(float64)
	}

	avgResponseTime := float64(0)
	if len(nodes) > 0 {
		avgResponseTime = totalResponseTime / float64(len(nodes))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"total_nodes":       len(nodes),
			"healthy_nodes":     registry.HealthyCount(),
			"total_requests":    totalRequests,
			"success_requests":  successRequests,
			"avg_response_time": avgResponseTime,
		},
	})
}

// GetConfig 获取配置
// GET /api/v1/config
func (h *Handlers) GetConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"app_name":              h.cfg.AppName,
			"app_version":           h.cfg.AppVersion,
			"load_balance_strategy": h.cfg.LoadBalanceStrategy,
			"health_check_enabled":  h.cfg.HealthCheckEnabled,
			"health_check_interval": h.cfg.HealthCheckInterval.String(),
			"queue_enabled":         h.cfg.QueueEnabled,
			"queue_max_size":        h.cfg.QueueMaxSize,
			"rate_limit_enabled":    h.cfg.RateLimitEnabled,
			"rate_limit_rpm":        h.cfg.RateLimitRPM,
		},
	})
}

// Root 根路径
// GET /
func (h *Handlers) Root(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"name":    h.cfg.AppName,
		"version": h.cfg.AppVersion,
		"docs":    "/docs",
		"api":     "/api/v1",
	})
}

// YOLODetectRequest YOLO检测请求
type YOLODetectRequest struct {
	Image string `json:"image" binding:"required"`
}

// YOLODetection YOLO检测结果
type YOLODetection struct {
	BBox       []float64 `json:"bbox"`
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float64   `json:"confidence"`
}

// YOLODetectResponse YOLO检测响应
type YOLODetectResponse struct {
	Success     bool            `json:"success"`
	Detections  []YOLODetection `json:"detections,omitempty"`
	InferenceMs int             `json:"inference_ms,omitempty"`
	NodeID      string          `json:"node_id,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// YOLODetect YOLO目标检测（使用专用 Round-Robin 调度器）
// POST /api/v1/yolo/detect
// ★ 性能优化（参考 Python）：
//   - JSON: 零拷贝直接转发原始 body
//   - FormData: 读取 JPEG 后 base64 编码转发
func (h *Handlers) YOLODetect(c *gin.Context) {
	// 使用 YOLO 专用 Round-Robin 调度器选择节点
	allNodes := h.client.GetRegistry().GetAll()
	yoloRR := h.client.GetYOLORoundRobin()
	node := yoloRR.Select(allNodes)
	if node == nil {
		c.JSON(http.StatusServiceUnavailable, YOLODetectResponse{
			Success: false,
			Error:   "无可用的YOLO推理节点",
		})
		return
	}

	var result *client.YOLODetectResponse
	var err error

	contentType := c.ContentType()
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// FormData: 读取 JPEG 后 base64 编码（兼容旧节点）
		file, fileErr := c.FormFile("image")
		if fileErr != nil {
			c.JSON(http.StatusBadRequest, YOLODetectResponse{
				Success: false,
				Error:   "无效的图片文件: " + fileErr.Error(),
			})
			return
		}
		f, fileErr := file.Open()
		if fileErr != nil {
			c.JSON(http.StatusBadRequest, YOLODetectResponse{
				Success: false,
				Error:   "无法打开图片文件: " + fileErr.Error(),
			})
			return
		}
		defer f.Close()
		data, fileErr := io.ReadAll(f)
		if fileErr != nil {
			c.JSON(http.StatusBadRequest, YOLODetectResponse{
				Success: false,
				Error:   "读取图片失败: " + fileErr.Error(),
			})
			return
		}
		// ★ 使用二进制端点（节点已支持）
		result, err = h.client.ForwardYOLOBinary(c.Request.Context(), node, data)
	} else {
		// ★ JSON: 零拷贝直接转发原始 body（跳过解析和序列化）
		rawBody, readErr := io.ReadAll(c.Request.Body)
		if readErr != nil {
			c.JSON(http.StatusBadRequest, YOLODetectResponse{
				Success: false,
				Error:   "读取请求体失败: " + readErr.Error(),
			})
			return
		}
		result, err = h.client.ForwardYOLORaw(c.Request.Context(), node, rawBody)
	}

	if err != nil {
		// ★ 修复: 区分客户端断开/超时与真正的节点故障
		// 1. 客户端断开 (context cancelled)
		// 2. HTTP 客户端超时 (net.Error.Timeout())
		// 以上两种情况不应导致节点加入黑名单
		ctx := c.Request.Context()
		isClientIssue := ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded
		if !isClientIssue {
			// 检查是否是 HTTP 超时错误
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				isClientIssue = true
			}
		}
		if isClientIssue {
			// 客户端问题或超时，静默返回（不记录节点失败）
			return
		}
		// 真正的节点错误，加入黑名单
		yoloRR.RecordFailure(node.ID, 3.0)
		c.JSON(http.StatusInternalServerError, YOLODetectResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	yoloRR.RecordSuccess(node.ID, result.InferenceMs)
	result.NodeID = node.ID
	c.JSON(http.StatusOK, result)
}

// YOLOStatus YOLO服务状态
// GET /api/v1/yolo/status
func (h *Handlers) YOLOStatus(c *gin.Context) {
	registry := h.client.GetRegistry()
	allNodes := registry.GetAll()

	yoloNodes := make([]gin.H, 0)
	healthyCount := 0
	for _, node := range allNodes {
		yoloPort := node.YOLOPort
		if yoloPort == 0 {
			yoloPort = 9081
		}
		yoloNodes = append(yoloNodes, gin.H{
			"id":           node.ID,
			"host":         node.Host,
			"yolo_port":    yoloPort,
			"yolo_healthy": node.YoloHealthy,
			"yolo_url":     fmt.Sprintf("http://%s:%d", node.Host, yoloPort),
			"enabled":      node.Enabled,
		})
		if node.YoloHealthy && node.IsHealthy() && node.Enabled {
			healthyCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"total_nodes":   len(yoloNodes),
		"healthy_nodes": healthyCount,
		"nodes":         yoloNodes,
		"rr_stats":      h.client.GetYOLORoundRobin().GetStats(),
	})
}

// YOLOHealth YOLO服务健康检查
// GET /api/v1/yolo/health
// ★ 参考 Python: 返回 cluster_capacity_fps 供前端计算每摄像头 FPS 上限
func (h *Handlers) YOLOHealth(c *gin.Context) {
	registry := h.client.GetRegistry()
	allNodes := registry.GetAll()
	yoloRR := h.client.GetYOLORoundRobin()
	stats := yoloRR.GetStats()

	healthyCount := 0
	for _, node := range allNodes {
		if node.IsHealthy() && node.Enabled {
			healthyCount++
		}
	}

	status := "ok"
	if healthyCount == 0 {
		status = "degraded"
	}

	// ★ 计算集群 YOLO 总吞吐量
	// 每个节点: capacity = (1000ms / avg_ms) × workers
	// workers 从节点 /health 端点获取
	var clusterCapacityFPS float64
	perNodeCapacity := make(map[string]float64)
	var totalRequests, totalSuccess, totalFail int64

	// 预先获取每个节点的 worker 数量
	nodeWorkers := make(map[string]int)
	for _, node := range allNodes {
		if node.IsHealthy() && node.Enabled {
			workers := h.getNodeYOLOWorkers(node)
			if workers > 0 {
				nodeWorkers[node.ID] = workers
			}
		}
	}

	for nodeID, nodeStat := range stats {
		total := nodeStat["total"].(int64)
		success := nodeStat["success"].(int64)
		fail := nodeStat["fail"].(int64)
		avgMs := nodeStat["avg_ms"].(float64)

		totalRequests += total
		totalSuccess += success
		totalFail += fail

		// 至少 10 次成功才算可靠
		if avgMs > 0 && success >= 10 {
			// ★ 修复: 容量 = 单次推理速率 × worker 数量
			workers := nodeWorkers[nodeID]
			if workers < 1 {
				workers = 1 // 默认至少 1 个 worker
			}
			nodeFPS := (1000.0 / avgMs) * float64(workers)
			clusterCapacityFPS += nodeFPS
			perNodeCapacity[nodeID] = nodeFPS
		}
	}

	// 冷启动时按健康节点数 * 5fps 估算
	if clusterCapacityFPS == 0 && healthyCount > 0 {
		clusterCapacityFPS = float64(healthyCount) * 5.0
	}

	var successRate float64
	if totalRequests > 0 {
		successRate = float64(totalSuccess) / float64(totalRequests) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"status":              status,
		"healthy_nodes":       healthyCount,
		"total_nodes":         len(allNodes),
		"cluster_capacity_fps": clusterCapacityFPS,
		"per_node_capacity":   perNodeCapacity,
		"yolo_stats": gin.H{
			"total_requests": totalRequests,
			"total_success":  totalSuccess,
			"total_fail":     totalFail,
			"success_rate":   successRate,
			"per_node":       stats,
		},
	})
}

// getNodeYOLOWorkers 查询节点 YOLO worker 数量
func (h *Handlers) getNodeYOLOWorkers(node *nodes.Node) int {
	yoloPort := 9081
	if node.YOLOPort > 0 {
		yoloPort = node.YOLOPort
	}

	url := fmt.Sprintf("http://%s:%d/health", node.Host, yoloPort)
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 1 // 默认 1 worker
	}
	defer resp.Body.Close()

	var data struct {
		Workers int `json:"workers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 1
	}
	if data.Workers < 1 {
		return 1
	}
	return data.Workers
}

// GetPendingTasks 获取待处理任务数
// GET /api/v1/stats/tasks/pending
func (h *Handlers) GetPendingTasks(c *gin.Context) {
	// ★ 使用 VLM 异步任务计数器
	vlmRunning := atomic.LoadInt64(&vlmAsyncTaskCount)

	// ★ 构建前端期望的 tasks 数组格式
	tasks := make([]gin.H, 0)
	for i := int64(0); i < vlmRunning; i++ {
		tasks = append(tasks, gin.H{
			"id":        fmt.Sprintf("vlm-task-%d", i+1),
			"type":      "image",
			"status":    "running",
			"progress":  50,
			"createdAt": time.Now().Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"pending":     vlmRunning,
		"queued":      0,
		"tasks":       tasks,
		"vlm_running": vlmRunning,
		"vlm_limit":   atomic.LoadInt64(&vlmAsyncMaxLimit),
	})
}

// HandleLiveWebSocket VLM 结果实时推送 WebSocket
// GET /api/v1/ws/live
func (h *Handlers) HandleLiveWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 注册到全局管理器
	wsManager.Connect(conn)
	defer wsManager.Disconnect(conn)

	// 发送欢迎消息
	conn.WriteJSON(gin.H{
		"type":      "welcome",
		"message":   "WebSocket连接成功",
		"channels":  []string{"nodes", "stats", "tasks"},
		"timestamp": time.Now().Format(time.RFC3339),
	})

	// 定期推送节点状态
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// 推送节点状态
			registry := h.client.GetRegistry()
			allNodes := registry.GetAll()
			nodeData := make([]gin.H, 0, len(allNodes))
			for _, node := range allNodes {
				nodeData = append(nodeData, gin.H{
					"id":            node.ID,
					"status":        string(node.Status),
					"llama_healthy": node.LlamaHealthy,
					"yolo_healthy":  node.YoloHealthy,
					"enabled":       node.Enabled,
				})
			}
			conn.WriteJSON(gin.H{
				"type":      "nodes",
				"channel":   "nodes",
				"data":      nodeData,
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
	}
}
