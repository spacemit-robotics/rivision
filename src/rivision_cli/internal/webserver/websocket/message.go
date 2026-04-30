package websocket

import "time"

// MessageType 消息类型
type MessageType string

const (
	// VLM 相关
	TypeVLMResult  MessageType = "vlm.result"
	TypeVLMError   MessageType = "vlm.error"
	TypeVLMStatus  MessageType = "vlm.status"

	// YOLO 相关
	TypeYOLODetection MessageType = "yolo.detection"
	TypeYOLOStatus    MessageType = "yolo.status"

	// 摄像头相关
	TypeCameraConnected    MessageType = "camera.connected"
	TypeCameraDisconnected MessageType = "camera.disconnected"
	TypeCameraError        MessageType = "camera.error"

	// 系统相关
	TypeGatewayStatus MessageType = "gateway.status"
	TypeSystemStatus  MessageType = "system.status"
)

// Message WebSocket 消息
type Message struct {
	Type       MessageType `json:"type"`
	Timestamp  time.Time   `json:"timestamp"`
	CameraID   string      `json:"camera_id,omitempty"`
	Data       interface{} `json:"data"`
	BinaryData []byte      `json:"-"` // 二进制数据（不序列化到JSON）
}

// NewMessage 创建新消息
func NewMessage(msgType MessageType, data interface{}) *Message {
	return &Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// NewCameraMessage 创建摄像头相关消息
func NewCameraMessage(msgType MessageType, cameraID string, data interface{}) *Message {
	return &Message{
		Type:      msgType,
		Timestamp: time.Now(),
		CameraID:  cameraID,
		Data:      data,
	}
}

// VLMResultData VLM 结果数据
type VLMResultData struct {
	ID              string `json:"id"`
	CameraName      string `json:"camera_name"`
	TriggerMode     string `json:"trigger_mode"`
	Text            string `json:"text"`
	TokensUsed      int    `json:"tokens_used"`
	NodeID          string `json:"node_id"`
	InferenceTimeMs int64  `json:"inference_time_ms"`
	ImageBase64     string `json:"image_base64,omitempty"`
}

// YOLODetectionData YOLO 检测数据
type YOLODetectionData struct {
	Detections      []Detection `json:"detections"`
	InferenceTimeMs int64       `json:"inference_time_ms"`
	FrameIndex      int64       `json:"frame_index"`
	FrameTime       float64     `json:"frame_time"`        // 帧时间戳（秒）
	ImageWidth      int         `json:"image_width"`
	ImageHeight     int         `json:"image_height"`
	FrameBase64     string      `json:"frame_base64,omitempty"` // 检测帧的 Base64 编码
	UseBinary       bool        `json:"use_binary,omitempty"`   // 是否使用二进制传输（下一条消息为二进制帧）
}

// Detection 单个检测结果
type Detection struct {
	ClassID    int        `json:"class_id"`
	ClassName  string     `json:"class_name"`
	Confidence float32    `json:"confidence"`
	BBox       [4]float64 `json:"bbox"` // x1, y1, x2, y2 (归一化坐标 0-1)
	TrackID    int        `json:"track_id,omitempty"`
	VelocityX  float64    `json:"velocity_x,omitempty"` // X方向速度（归一化/秒）
	VelocityY  float64    `json:"velocity_y,omitempty"` // Y方向速度（归一化/秒）
}

// CameraStatusData 摄像头状态数据
type CameraStatusData struct {
	CameraName string `json:"camera_name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// GatewayStatusData 网关状态数据
type GatewayStatusData struct {
	Connected    bool   `json:"connected"`
	NodeCount    int    `json:"node_count"`
	HealthyNodes int    `json:"healthy_nodes"`
	Error        string `json:"error,omitempty"`
}

// SystemStatusData 系统状态数据
type SystemStatusData struct {
	CameraCount   int     `json:"camera_count"`
	YOLOEnabled   bool    `json:"yolo_enabled"`
	VLMEnabled    bool    `json:"vlm_enabled"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
}
