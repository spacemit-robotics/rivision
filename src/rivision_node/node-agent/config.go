package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Settings 应用配置 - 通过 .env 文件和环境变量配置
type Settings struct {
	// 基本信息
	AppName    string `json:"app_name"`
	AppVersion string `json:"app_version"`

	// 服务配置
	AgentHost string `json:"agent_host"`
	AgentPort int    `json:"agent_port"`

	// 节点标识
	NodeID     string   `json:"node_id"`
	NodeWeight int      `json:"node_weight"`
	NodeTags   []string `json:"node_tags"`

	// llama.cpp 配置 (VLM)
	LlamaEnabled     bool   `json:"llama_enabled"`
	LlamaServiceName string `json:"llama_service_name"`
	LlamaProcessName string `json:"llama_process_name"`
	LlamaHost        string `json:"llama_host"`
	LlamaPort        int    `json:"llama_port"`
	LlamaBinaryPath  string `json:"llama_binary_path"`
	LlamaModelPath   string `json:"llama_model_path"`

	// YOLO 服务配置
	YoloEnabled     bool   `json:"yolo_enabled"`
	YoloHost        string `json:"yolo_host"`
	YoloPort        int    `json:"yolo_port"`
	YoloServiceName string `json:"yolo_service_name"`
	YoloProcessName string `json:"yolo_process_name"`
	YoloBinaryPath  string `json:"yolo_binary_path"`
	YoloModelPath   string `json:"yolo_model_path"`

	// Gateway 配置
	GatewayURL        string `json:"gateway_url"`
	HeartbeatInterval int    `json:"heartbeat_interval"`

	// 节点信息
	NodeHost string `json:"node_host"` // auto 表示自动检测 IP

	// Token 认证
	RegistrationToken string `json:"registration_token"`

	// 日志配置
	LogLevel       string `json:"log_level"`
	LogEnable      bool   `json:"log_enable"`
	LogFile        string `json:"log_file"`
	LogBackupCount int    `json:"log_backup_count"`
}

// Host 兼容属性
func (s *Settings) Host() string {
	return s.AgentHost
}

// Port 兼容属性
func (s *Settings) Port() int {
	return s.AgentPort
}

// loadEnvFile 从 .env 文件加载环境变量
func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return // .env 文件不存在是正常的
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 解析 KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		// 去掉引号
		value = strings.Trim(value, `"'`)
		// 只在环境变量未设置时才设置
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
}

// getEnv 获取环境变量，带默认值
func getEnv(key, defaultVal string) string {
	if val, exists := os.LookupEnv(key); exists {
		return val
	}
	return defaultVal
}

// getEnvInt 获取整型环境变量
func getEnvInt(key string, defaultVal int) int {
	if val, exists := os.LookupEnv(key); exists {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}

// getEnvBool 获取布尔型环境变量
func getEnvBool(key string, defaultVal bool) bool {
	if val, exists := os.LookupEnv(key); exists {
		val = strings.ToLower(strings.TrimSpace(val))
		switch val {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultVal
}

// getEnvTags 获取标签列表环境变量（逗号分隔）
func getEnvTags(key string, defaultVal []string) []string {
	if val, exists := os.LookupEnv(key); exists {
		val = strings.TrimSpace(val)
		if val == "" || strings.HasPrefix(val, "__") {
			return []string{}
		}
		// 支持 JSON 数组格式
		if strings.HasPrefix(val, "[") {
			var tags []string
			if err := json.Unmarshal([]byte(val), &tags); err == nil {
				return tags
			}
		}
		// 逗号分隔格式
		tags := []string{}
		for _, t := range strings.Split(val, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}
	return defaultVal
}

// getHostname 获取主机名作为默认节点ID
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// NewSettings 创建配置实例
func NewSettings() *Settings {
	// 加载 .env 文件
	loadEnvFile("../.env")
	loadEnvFile(".env")

	s := &Settings{
		AppName:    "RiVision Node Agent",
		AppVersion: "1.0.0",

		AgentHost: getEnv("AGENT_HOST", "0.0.0.0"),
		AgentPort: getEnvInt("AGENT_PORT", 9090),

		NodeID:     getEnv("NODE_ID", ""),
		NodeWeight: getEnvInt("NODE_WEIGHT", 1),
		NodeTags:   getEnvTags("NODE_TAGS", []string{"vm", "cpu"}),

		LlamaEnabled:     getEnvBool("LLAMA_ENABLED", true),
		LlamaServiceName: getEnv("LLAMA_SERVICE_NAME", "rivision-llama"),
		LlamaProcessName: getEnv("LLAMA_PROCESS_NAME", "llama-server"),
		LlamaHost:        getEnv("LLAMA_HOST", "127.0.0.1"),
		LlamaPort:        getEnvInt("LLAMA_PORT", 9080),
		LlamaBinaryPath:  getEnv("LLAMA_BINARY_PATH", "/home/node/rivision/llama.cpp/build/bin/llama-server"),
		LlamaModelPath:   getEnv("LLAMA_MODEL_PATH", "/home/node/rivision/models/MiniCPM-V-2_6-Q8_0.gguf"),

		YoloEnabled:     getEnvBool("YOLO_ENABLED", true),
		YoloHost:        getEnv("YOLO_HOST", "127.0.0.1"),
		YoloPort:        getEnvInt("YOLO_PORT", 9081),
		YoloServiceName: getEnv("YOLO_SERVICE_NAME", "rivision-yolo"),
		YoloProcessName: getEnv("YOLO_PROCESS_NAME", "yolo-server"),
		YoloBinaryPath:  getEnv("YOLO_BINARY_PATH", "/home/node/rivision/yolo-server/build/yolo-server"),
		YoloModelPath:   getEnv("YOLO_MODEL_PATH", "/home/node/rivision/models/yolov8n.onnx"),

		GatewayURL:        getEnv("GATEWAY_URL", "http://192.168.122.1:8081"),
		HeartbeatInterval: getEnvInt("HEARTBEAT_INTERVAL", 10),

		NodeHost: getEnv("NODE_HOST", "auto"),

		RegistrationToken: getEnv("REGISTRATION_TOKEN", ""),

		LogLevel:       getEnv("LOG_LEVEL", "INFO"),
		LogEnable:      getEnvBool("LOG_ENABLE", true),
		LogFile:        getEnv("LOG_FILE", "logs/node-agent.log"),
		LogBackupCount: getEnvInt("LOG_BACKUP_COUNT", 30),
	}

	// 如果 NODE_ID 为空，使用主机名
	if s.NodeID == "" {
		s.NodeID = getHostname()
	}

	// ★ 检查关键配置是否显式设置（帮助调试配置问题）
	if os.Getenv("LLAMA_ENABLED") == "" {
		fmt.Println("[config] ⚠️  LLAMA_ENABLED 未设置，使用默认值 true")
	}
	if os.Getenv("YOLO_ENABLED") == "" {
		fmt.Println("[config] ⚠️  YOLO_ENABLED 未设置，使用默认值 true")
	}

	return s
}

// String 打印配置摘要
func (s *Settings) String() string {
	return fmt.Sprintf("Settings{NodeID=%s, Host=%s:%d, Gateway=%s, LlamaPort=%d}",
		s.NodeID, s.AgentHost, s.AgentPort, s.GatewayURL, s.LlamaPort)
}
