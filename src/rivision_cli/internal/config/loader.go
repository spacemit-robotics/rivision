package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Loader 配置加载器
type Loader struct {
	viper *viper.Viper
}

// NewLoader 创建配置加载器
func NewLoader() *Loader {
	return &Loader{
		viper: viper.New(),
	}
}

// Load 从文件加载配置
func (l *Loader) Load(configPath string) (*Config, error) {
	cfg := DefaultConfig()

	// 如果指定了配置文件路径
	if configPath != "" {
		l.viper.SetConfigFile(configPath)
	} else {
		// 默认搜索路径
		l.viper.SetConfigName("rivision")
		l.viper.SetConfigType("yaml")
		l.viper.AddConfigPath(".")
		l.viper.AddConfigPath("./configs")
		l.viper.AddConfigPath("/etc/rivision")
		l.viper.AddConfigPath("$HOME/.rivision")
	}

	// 设置环境变量前缀
	l.viper.SetEnvPrefix("RIVISION")
	l.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	l.viper.AutomaticEnv()

	// 绑定环境变量
	l.bindEnvVariables()

	// 读取配置文件
	if err := l.viper.ReadInConfig(); err != nil {
		// 如果配置文件不存在，使用默认配置
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	// 解析到结构体
	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return cfg, nil
}

// LoadFromEnv 仅从环境变量加载配置
func (l *Loader) LoadFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	l.viper.SetEnvPrefix("RIVISION")
	l.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	l.viper.AutomaticEnv()

	l.bindEnvVariables()

	if err := l.viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	return cfg, nil
}

// bindEnvVariables 绑定环境变量
func (l *Loader) bindEnvVariables() {
	// 服务器配置
	l.viper.BindEnv("server.host", "RIVISION_HOST")
	l.viper.BindEnv("server.port", "RIVISION_PORT")
	l.viper.BindEnv("server.debug", "RIVISION_DEBUG")

	// VLM 配置
	l.viper.BindEnv("vlm.gateway_url", "RIVISION_GATEWAY_URL")
	l.viper.BindEnv("vlm.timeout", "RIVISION_VLM_TIMEOUT")
	l.viper.BindEnv("vlm.default_prompt", "RIVISION_VLM_PROMPT")

	// YOLO 配置
	l.viper.BindEnv("yolo.enabled", "RIVISION_YOLO_ENABLED")
	l.viper.BindEnv("yolo.model_dir", "RIVISION_YOLO_MODEL_DIR")
	l.viper.BindEnv("yolo.default_model", "RIVISION_YOLO_MODEL")

	// 日志配置
	l.viper.BindEnv("logging.level", "RIVISION_LOG_LEVEL")
	l.viper.BindEnv("logging.file", "RIVISION_LOG_FILE")

	// 存储配置
	l.viper.BindEnv("storage.path", "RIVISION_DATA_PATH")
}

// SaveDefault 保存默认配置到文件
func (l *Loader) SaveDefault(path string) error {
	cfg := DefaultConfig()

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 设置配置值
	l.viper.Set("node", cfg.Node)
	l.viper.Set("server", cfg.Server)
	l.viper.Set("go2rtc", cfg.Go2rtc)
	l.viper.Set("cameras", cfg.Cameras)
	l.viper.Set("yolo", cfg.YOLO)
	l.viper.Set("vlm", cfg.VLM)
	l.viper.Set("storage", cfg.Storage)
	l.viper.Set("logging", cfg.Logging)

	// 写入文件
	l.viper.SetConfigFile(path)
	if err := l.viper.WriteConfig(); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// Watch 监听配置文件变化
func (l *Loader) Watch(callback func(*Config)) {
	l.viper.OnConfigChange(func(e fsnotify.Event) {
		cfg := DefaultConfig()
		if err := l.viper.Unmarshal(cfg); err == nil {
			callback(cfg)
		}
	})
	l.viper.WatchConfig()
}

// GetConfigPath 获取当前使用的配置文件路径
func (l *Loader) GetConfigPath() string {
	return l.viper.ConfigFileUsed()
}

// Load 便捷函数：加载配置
func Load(configPath string) (*Config, error) {
	loader := NewLoader()
	return loader.Load(configPath)
}

// MustLoad 便捷函数：加载配置，失败时 panic
func MustLoad(configPath string) *Config {
	cfg, err := Load(configPath)
	if err != nil {
		panic(fmt.Sprintf("加载配置失败: %v", err))
	}
	return cfg
}
