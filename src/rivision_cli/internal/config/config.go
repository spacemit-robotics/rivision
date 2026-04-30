// Package config 提供 RiVision-CLI 的配置管理功能
package config

import (
	"time"
)

// Config 主配置结构
type Config struct {
	Node    NodeConfig    `yaml:"node" mapstructure:"node"`
	Server  ServerConfig  `yaml:"server" mapstructure:"server"`
	Go2rtc  Go2rtcConfig  `yaml:"go2rtc" mapstructure:"go2rtc"`
	Cameras []CameraConfig `yaml:"cameras" mapstructure:"cameras"`
	YOLO    YOLOConfig    `yaml:"yolo" mapstructure:"yolo"`
	VLM     VLMConfig     `yaml:"vlm" mapstructure:"vlm"`
	Semantic SemanticConfig `yaml:"semantic" mapstructure:"semantic"`
	Storage StorageConfig `yaml:"storage" mapstructure:"storage"`
	Logging LoggingConfig `yaml:"logging" mapstructure:"logging"`
	OWL     OWLConfig     `yaml:"owl" mapstructure:"owl"`
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID   string `yaml:"id" mapstructure:"id"`
	Role string `yaml:"role" mapstructure:"role"` // master, yolo, compute
	Name string `yaml:"name" mapstructure:"name"`
}

// ServerConfig Web 服务器配置
type ServerConfig struct {
	Host  string `yaml:"host" mapstructure:"host"`
	Port  int    `yaml:"port" mapstructure:"port"`
	Debug bool   `yaml:"debug" mapstructure:"debug"`
}

// Go2rtcConfig go2rtc 配置
type Go2rtcConfig struct {
	Enabled    bool   `yaml:"enabled" mapstructure:"enabled"`
	Port       int    `yaml:"port" mapstructure:"port"`
	ConfigPath string `yaml:"config_path" mapstructure:"config_path"`
}

// CameraConfig 摄像头配置
type CameraConfig struct {
	ID      string           `yaml:"id" mapstructure:"id"`
	Name    string           `yaml:"name" mapstructure:"name"`
	Source  string           `yaml:"source" mapstructure:"source"`
	Enabled bool             `yaml:"enabled" mapstructure:"enabled"`
	YOLO    CameraYOLOConfig `yaml:"yolo" mapstructure:"yolo"`
	VLM     CameraVLMConfig  `yaml:"vlm" mapstructure:"vlm"`
}

// CameraYOLOConfig 摄像头级别的 YOLO 配置
type CameraYOLOConfig struct {
	Enabled    bool    `yaml:"enabled" mapstructure:"enabled"`
	Model      string  `yaml:"model" mapstructure:"model"`
	ConfThresh float64 `yaml:"conf_thresh" mapstructure:"conf_thresh"`
	TargetFPS  int     `yaml:"target_fps" mapstructure:"target_fps"`
}

// CameraVLMConfig 摄像头级别的 VLM 配置
type CameraVLMConfig struct {
	Enabled     bool          `yaml:"enabled" mapstructure:"enabled"`
	TriggerMode string        `yaml:"trigger_mode" mapstructure:"trigger_mode"` // interval, yolo, manual
	Interval    time.Duration `yaml:"interval" mapstructure:"interval"`
	MinObjects  int           `yaml:"min_objects" mapstructure:"min_objects"`
	Prompt      string        `yaml:"prompt" mapstructure:"prompt"`
}

// YOLOConfig 全局 YOLO 配置
type YOLOConfig struct {
	Enabled       bool          `yaml:"enabled" mapstructure:"enabled"`
	ModelDir      string        `yaml:"model_dir" mapstructure:"model_dir"`
	ClassNamesDir string        `yaml:"class_names_dir" mapstructure:"class_names_dir"`
	DefaultModel  string        `yaml:"default_model" mapstructure:"default_model"`
	UseNPU        bool          `yaml:"use_npu" mapstructure:"use_npu"`
	InputWidth    int           `yaml:"input_width" mapstructure:"input_width"`
	InputHeight   int           `yaml:"input_height" mapstructure:"input_height"`
	ConfThresh    float64       `yaml:"conf_thresh" mapstructure:"conf_thresh"`
	NMSThresh     float64       `yaml:"nms_thresh" mapstructure:"nms_thresh"`
	MaxDetections int           `yaml:"max_detections" mapstructure:"max_detections"`
	HotReload     bool          `yaml:"hot_reload" mapstructure:"hot_reload"`
	Tracker       TrackerConfig `yaml:"tracker" mapstructure:"tracker"`
}

// TrackerConfig ByteTrack 跟踪器配置
type TrackerConfig struct {
	Enabled     bool    `yaml:"enabled" mapstructure:"enabled"`
	TrackThresh float64 `yaml:"track_thresh" mapstructure:"track_thresh"`
	MatchThresh float64 `yaml:"match_thresh" mapstructure:"match_thresh"`
	TrackBuffer int     `yaml:"track_buffer" mapstructure:"track_buffer"`
}

// VLMConfig 全局 VLM 配置
type VLMConfig struct {
	GatewayURL    string        `yaml:"gateway_url" mapstructure:"gateway_url"`
	Timeout       time.Duration `yaml:"timeout" mapstructure:"timeout"`
	Retry         int           `yaml:"retry" mapstructure:"retry"`
	DefaultPrompt string        `yaml:"default_prompt" mapstructure:"default_prompt"`
	MaxConcurrent int           `yaml:"max_concurrent" mapstructure:"max_concurrent"`
}

// SemanticConfig 语义检索与 embedding provider 配置
type SemanticConfig struct {
	EnableRemote bool   `yaml:"enable_remote" mapstructure:"enable_remote"`
	RemoteURL    string `yaml:"remote_url" mapstructure:"remote_url"`
	RemoteAPIKey string `yaml:"remote_api_key" mapstructure:"remote_api_key"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type       string            `yaml:"type" mapstructure:"type"` // sqlite, file
	Path       string            `yaml:"path" mapstructure:"path"`
	VLMResults VLMResultsConfig  `yaml:"vlm_results" mapstructure:"vlm_results"`
	Snapshots  SnapshotsConfig   `yaml:"snapshots" mapstructure:"snapshots"`
}

// VLMResultsConfig VLM 结果存储配置
type VLMResultsConfig struct {
	MaxCount      int `yaml:"max_count" mapstructure:"max_count"`
	RetentionDays int `yaml:"retention_days" mapstructure:"retention_days"`
}

// SnapshotsConfig 快照存储配置
type SnapshotsConfig struct {
	Path          string `yaml:"path" mapstructure:"path"`
	RetentionDays int    `yaml:"retention_days" mapstructure:"retention_days"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string `yaml:"level" mapstructure:"level"`
	File       string `yaml:"file" mapstructure:"file"`
	MaxSize    string `yaml:"max_size" mapstructure:"max_size"`
	MaxBackups int    `yaml:"max_backups" mapstructure:"max_backups"`
	Compress   bool   `yaml:"compress" mapstructure:"compress"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Node: NodeConfig{
			ID:   "master-1",
			Role: "master",
			Name: "RiVision 管理节点",
		},
		Server: ServerConfig{
			Host:  "0.0.0.0",
			Port:  8280,
			Debug: false,
		},
		Go2rtc: Go2rtcConfig{
			Enabled:    true,
			Port:       1984,
			ConfigPath: "",
		},
		Cameras: []CameraConfig{},
		YOLO: YOLOConfig{
			Enabled:       false,
			ModelDir:      "./models",
			DefaultModel:  "yolov8n.onnx",
			UseNPU:        false,
			ConfThresh:    0.5,
			NMSThresh:     0.45,
			MaxDetections: 100,
			Tracker: TrackerConfig{
				Enabled:     true,
				TrackThresh: 0.5,
				MatchThresh: 0.8,
				TrackBuffer: 30,
			},
		},
		VLM: VLMConfig{
			GatewayURL:    "http://localhost:8081",
			Timeout:       30 * time.Second,
			Retry:         2,
			DefaultPrompt: "请描述图片中的场景",
			MaxConcurrent: 4,
		},
		Semantic: SemanticConfig{
			EnableRemote: true,
			RemoteURL:    "",
		},
		Storage: StorageConfig{
			Type: "sqlite",
			Path: "./data/rivision.db",
			VLMResults: VLMResultsConfig{
				MaxCount:      10000,
				RetentionDays: 30,
			},
			Snapshots: SnapshotsConfig{
				Path:          "./data/snapshots",
				RetentionDays: 7,
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "./logs/rivision.log",
			MaxSize:    "100MB",
			MaxBackups: 7,
			Compress:   true,
		},
		OWL: OWLConfig{
			Enabled:  false,
			URL:      "http://127.0.0.1:15123",
			Username: "admin",
			Password: "admin",
		},
	}
}

// OWLConfig OWL (GB28181) 配置
type OWLConfig struct {
Enabled  bool   `yaml:"enabled" mapstructure:"enabled"`
URL      string `yaml:"url" mapstructure:"url"`
Username string `yaml:"username" mapstructure:"username"`
Password string `yaml:"password" mapstructure:"password"`
}
