package yolo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// DistributedDetector 分布式YOLO检测器
// 将YOLO推理任务分发到远程算力节点执行
type DistributedDetector struct {
	gatewayURL    string
	client        *http.Client
	enabled       bool
	requestCount  int64 // 原子计数器：用于首帧诊断
}

// DistributedConfig 分布式检测配置
type DistributedConfig struct {
	GatewayURL string        // Gateway地址
	Timeout    time.Duration // 请求超时时间
	Retry      int           // 重试次数
}

// NewDistributedDetector 创建分布式检测器
func NewDistributedDetector(config DistributedConfig) *DistributedDetector {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second // ★ K3 YOLO 推理 20-100ms，3s 足够含网络传输；10s 太长，失败时阻塞 worker
	}

	// 自定义 Transport: 提高连接池容量
	// 默认 MaxIdleConnsPerHost=2，当 N 个 worker 并行请求同一 Gateway 时
	// 只有 2 个连接可复用，其余需反复新建/销毁 TCP 连接
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20, // ★ 支持最多 20 个 worker 并行复用连接
		MaxConnsPerHost:     0,  // 不限制
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	return &DistributedDetector{
		gatewayURL: config.GatewayURL,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		enabled:  config.GatewayURL != "",
	}
}

// remoteDetectRequest 远程检测请求
type remoteDetectRequest struct {
	Image string `json:"image"` // Base64编码的JPEG图片
}

// remoteDetectResponse 远程检测响应
type remoteDetectResponse struct {
	Success     bool              `json:"success"`
	Detections  []remoteDetection `json:"detections,omitempty"`
	InferenceMs int               `json:"inference_ms,omitempty"`
	NodeID      string            `json:"node_id,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// remoteDetection 远程检测结果
type remoteDetection struct {
	BBox       []float64 `json:"bbox"`       // [x1, y1, x2, y2] 归一化坐标
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float64   `json:"confidence"`
}

// Detect 执行分布式检测
func (d *DistributedDetector) Detect(jpegData []byte, width, height int) (*DetectionResult, error) {
	if !d.enabled {
		return nil, fmt.Errorf("分布式检测未启用")
	}

	// 尝试分布式检测
	result, err := d.detectRemote(jpegData, width, height)
	if err == nil {
		return result, nil
	}

	return nil, fmt.Errorf("分布式检测失败: %w", err)
}

// detectRemote 执行远程检测
// ★ B6 优化: 使用 multipart/form-data 发送原始 JPEG 二进制
//   省去 Go 端 base64 编码 (~1-2ms) + 减少传输量 33%
func (d *DistributedDetector) detectRemote(jpegData []byte, width, height int) (*DetectionResult, error) {
	startTime := time.Now()
	seqNum := atomic.AddInt64(&d.requestCount, 1)

	// ★ 构建 multipart body（零 base64，直接发送 JPEG 二进制）
	encodeStart := time.Now()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "frame.jpg")
	if err != nil {
		return nil, fmt.Errorf("创建 multipart 失败: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(jpegData)); err != nil {
		return nil, fmt.Errorf("写入 multipart 失败: %w", err)
	}
	writer.Close()
	encodeMs := time.Since(encodeStart).Milliseconds()

	// 发送请求
	httpStart := time.Now()
	resp, err := d.client.Post(
		d.gatewayURL+"/api/v1/yolo/detect",
		writer.FormDataContentType(),
		&body,
	)
	httpMs := time.Since(httpStart).Milliseconds()
	if err != nil {
		if seqNum <= 5 || seqNum%200 == 0 {
			fmt.Printf("[DistributedYOLO] ❌ #%d 请求失败: pack=%dms http=%dms size=%dKB err=%v\n",
				seqNum, encodeMs, httpMs, body.Len()/1024, err)
		}
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		if seqNum <= 5 || seqNum%200 == 0 {
			fmt.Printf("[DistributedYOLO] ❌ #%d HTTP %d: pack=%dms http=%dms size=%dKB\n",
				seqNum, resp.StatusCode, encodeMs, httpMs, body.Len()/1024)
		}
		return nil, fmt.Errorf("远程服务返回错误: HTTP %d", resp.StatusCode)
	}

	// 解析响应
	var result remoteDetectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("远程检测失败: %s", result.Error)
	}

	// 转换检测结果：远程返回归一化坐标[0,1]，转换为像素坐标
	detections := make([]Detection, len(result.Detections))
	imgW := float64(width)
	imgH := float64(height)
	if imgW <= 0 {
		imgW = 640 // 默认宽度
	}
	if imgH <= 0 {
		imgH = 480 // 默认高度
	}
	
	for i, det := range result.Detections {
		if len(det.BBox) >= 4 {
			// 远程返回归一化坐标[0,1]，转换为像素坐标
			detections[i] = Detection{
				BBox: BBox{
					X1: int(det.BBox[0] * imgW),
					Y1: int(det.BBox[1] * imgH),
					X2: int(det.BBox[2] * imgW),
					Y2: int(det.BBox[3] * imgH),
				},
				Confidence: float32(det.Confidence),
				ClassID:    det.ClassID,
				ClassName:  det.ClassName,
			}
		}
	}

	totalTime := time.Since(startTime).Milliseconds()

	// ★ 首 5 次成功请求或每 200 次输出完整诊断（定位分布式耗时分布）
	if seqNum <= 5 || seqNum%200 == 0 {
		fmt.Printf("[DistributedYOLO] ✅ #%d: pack=%dms http=%dms infer=%dms total=%dms dets=%d node=%s size=%dKB\n",
			seqNum, encodeMs, httpMs, result.InferenceMs, totalTime,
			len(detections), result.NodeID, body.Len()/1024)
	}

	return &DetectionResult{
		Detections:      detections,
		InferenceTimeMs: int64(result.InferenceMs),
		TotalTimeMs:     totalTime,
		NodeID:          result.NodeID,
	}, nil
}

// IsEnabled 返回分布式检测是否启用
func (d *DistributedDetector) IsEnabled() bool {
	return d.enabled
}

// SetEnabled 设置分布式检测启用状态
func (d *DistributedDetector) SetEnabled(enabled bool) {
	d.enabled = enabled
}

// CheckGatewayStatus 检查 Gateway 是否在线
func (d *DistributedDetector) CheckGatewayStatus() (online bool, latencyMs int64, err error) {
	if d.gatewayURL == "" {
		return false, 0, fmt.Errorf("Gateway URL 未配置")
	}

	startTime := time.Now()
	resp, err := d.client.Get(d.gatewayURL + "/api/v1/health")
	latencyMs = time.Since(startTime).Milliseconds()

	if err != nil {
		return false, latencyMs, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, latencyMs, nil
	}
	return false, latencyMs, fmt.Errorf("Gateway 返回状态码: %d", resp.StatusCode)
}

// GetGatewayURL 返回 Gateway URL
func (d *DistributedDetector) GetGatewayURL() string {
	return d.gatewayURL
}

// GetHealthyNodeCount 查询 Gateway 获取健康节点数量
// 用于 M<N 场景自适应：计算每摄像头需要的 worker 数
func (d *DistributedDetector) GetHealthyNodeCount() (int, error) {
	if d.gatewayURL == "" {
		return 0, fmt.Errorf("Gateway URL 未配置")
	}

	resp, err := d.client.Get(d.gatewayURL + "/api/v1/yolo/health")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		HealthyNodes int `json:"healthy_nodes"`
		TotalNodes   int `json:"total_nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return result.HealthyNodes, nil
}

// Close 关闭分布式检测器
func (d *DistributedDetector) Close() error {
	// HTTP客户端不需要显式关闭
	return nil
}
