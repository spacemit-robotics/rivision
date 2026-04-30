package gateway

type AnalysisResult struct {
	Content string `json:"content"`
	Model   string `json:"model,omitempty"`
}

type Frame struct {
	Base64    string  `json:"base64"`
	Timestamp float64 `json:"timestamp"`
	Index     int     `json:"frame_index"`
}

type SearchResult struct {
	Timestamp   float64 `json:"timestamp"`
	FrameIndex  int     `json:"frame_index"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
	Matched     bool    `json:"matched"`
}

type VideoInfo struct {
	Duration   float64 `json:"duration"`
	FPS        float64 `json:"fps"`
	FrameCount int     `json:"frame_count"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
}

type VideoSummary struct {
	TotalDuration  float64    `json:"total_duration"`
	SegmentCount   int        `json:"segment_count"`
	OverallSummary string     `json:"overall_summary"`
	KeyScenes      []KeyScene `json:"key_scenes"`
}

type KeyScene struct {
	Timestamp   float64 `json:"timestamp"`
	Description string  `json:"description"`
}

type Node struct {
	ID                string  `json:"id"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	Healthy           bool    `json:"healthy"`
	LlamaHealthy      bool    `json:"llama_healthy"`
	Load              float64 `json:"load"`
	AvgProcessingTime float64 `json:"avg_processing_time"`
	TotalProcessed    int     `json:"total_processed"`
	TotalFailed       int     `json:"total_failed"`
}

type TokenResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type TokenInfo struct {
	NodeID      string `json:"node_id"`
	Token       string `json:"token"`
	CreatedAt   string `json:"created_at"`
	IsValid     bool   `json:"is_valid"`
	UsedCount   int    `json:"used_count"`
	MaxUses     int    `json:"max_uses"`
	ExpiresAt   string `json:"expires_at"`
	Description string `json:"description"`
}

type RegistrationStats struct {
	RegisteredNodes int `json:"registered_nodes"`
	ActiveTokens    int `json:"active_tokens"`
	BlacklistedIPs  int `json:"blacklisted_ips"`
	TotalFailureIPs int `json:"total_failure_ips"`
}

type BlacklistItem struct {
	IP           string `json:"ip"`
	Reason       string `json:"reason"`
	AddedAt      string `json:"added_at"`
	FailureCount int    `json:"failure_count"`
}

// YOLODetection YOLO检测结果
type YOLODetection struct {
	BBox       []float64 `json:"bbox"`       // [x1, y1, x2, y2] 归一化坐标
	ClassID    int       `json:"class_id"`
	ClassName  string    `json:"class_name"`
	Confidence float64   `json:"confidence"`
}

// YOLODetectResult YOLO检测响应
type YOLODetectResult struct {
	Success     bool            `json:"success"`
	Detections  []YOLODetection `json:"detections,omitempty"`
	InferenceMs int             `json:"inference_ms,omitempty"`
	NodeID      string          `json:"node_id,omitempty"`
	Error       string          `json:"error,omitempty"`
}
