// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package config 提供配置管理
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 网关配置
type Config struct {
	// 服务配置
	AppName    string
	AppVersion string
	Host       string
	Port       int
	Debug      bool

	// 节点配置
	NodesConfigPath string

	// 负载均衡策略: round_robin, random, least_conn, weighted
	LoadBalanceStrategy string

	// 健康检查
	HealthCheckEnabled  bool
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
	UnhealthyThreshold  int

	// 请求配置
	RequestTimeout       time.Duration
	WaitTimeout          time.Duration
	FastTimeout          time.Duration
	AdaptiveTimeout      bool
	MaxRetries           int
	MaxConcurrentPerNode int

	// 日志配置
	LogLevel       string
	LogFile        string
	LogEnable      bool
	LogBackupCount int

	// CORS 配置
	CORSOrigins []string

	// API 认证
	APIKey      string
	BearerToken string

	// 性能配置
	MonitoringInterval  time.Duration
	StatsRetentionHours int

	// 队列配置
	QueueEnabled bool
	QueueMaxSize int
	QueueTimeout time.Duration

	// 限流配置
	RateLimitEnabled    bool
	RateLimitRPM        int
	RateLimitConcurrent int

	// 异步任务配置
	AsyncMaxConcurrent int
	AsyncTaskTimeout   time.Duration
	DataDir            string

	// 数据清理
	ResultRetentionDays int

	// 默认系统提示词
	DefaultSystemPrompt string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		AppName:              "RiVision Inference Gateway",
		AppVersion:           "1.0.0-go",
		Host:                 "0.0.0.0",
		Port:                 8081,
		Debug:                false,
		NodesConfigPath:      "./config/nodes.yaml",
		LoadBalanceStrategy:  "least_conn",
		HealthCheckEnabled:   true,
		HealthCheckInterval:  30 * time.Second,
		HealthCheckTimeout:   5 * time.Second,
		UnhealthyThreshold:   3,
		RequestTimeout:       600 * time.Second,
		WaitTimeout:          600 * time.Second,
		FastTimeout:          30 * time.Second,
		AdaptiveTimeout:      true,
		MaxRetries:           2,
		MaxConcurrentPerNode: 1,
		LogLevel:             "INFO",
		LogFile:              "./logs/gateway.log",
		LogEnable:            true,
		LogBackupCount:       30,
		CORSOrigins:          []string{"*"},
		MonitoringInterval:   10 * time.Second,
		StatsRetentionHours:  168,
		QueueEnabled:         true,
		QueueMaxSize:         100,
		QueueTimeout:         60 * time.Second,
		RateLimitEnabled:     true,
		RateLimitRPM:         1000,
		RateLimitConcurrent:  100,
		AsyncMaxConcurrent:   0,
		AsyncTaskTimeout:     600 * time.Second,
		DataDir:              "./data",
		ResultRetentionDays:  7,
		DefaultSystemPrompt:  "You are a helpful assistant. Answer directly and concisely. Do not output any thinking process or use think tags.",
	}
}

// LoadFromEnv 从环境变量加载配置
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	// 检测 RISC-V 环境，调整默认值
	if os.Getenv("GOARCH") == "riscv64" {
		cfg.HealthCheckInterval = 60 * time.Second
		cfg.HealthCheckTimeout = 10 * time.Second
		cfg.RequestTimeout = 300 * time.Second
		cfg.WaitTimeout = 300 * time.Second
	}

	if v := os.Getenv("GATEWAY_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("GATEWAY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		}
	}
	if v := os.Getenv("GATEWAY_DEBUG"); v != "" {
		cfg.Debug = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("NODES_CONFIG_PATH"); v != "" {
		cfg.NodesConfigPath = v
	}
	if v := os.Getenv("LOAD_BALANCE_STRATEGY"); v != "" {
		cfg.LoadBalanceStrategy = v
	}
	if v := os.Getenv("HEALTH_CHECK_ENABLED"); v != "" {
		cfg.HealthCheckEnabled = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("HEALTH_CHECK_INTERVAL"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			cfg.HealthCheckInterval = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("HEALTH_CHECK_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			cfg.HealthCheckTimeout = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("UNHEALTHY_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.UnhealthyThreshold = n
		}
	}
	if v := os.Getenv("REQUEST_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			cfg.RequestTimeout = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("WAIT_TIMEOUT"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			cfg.WaitTimeout = time.Duration(sec) * time.Second
		}
	}
	if v := os.Getenv("MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxRetries = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_PER_NODE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxConcurrentPerNode = n
		}
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		cfg.LogFile = v
	}
	if v := os.Getenv("LOG_ENABLE"); v != "" {
		cfg.LogEnable = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("BEARER_TOKEN"); v != "" {
		cfg.BearerToken = v
	}
	if v := os.Getenv("QUEUE_ENABLED"); v != "" {
		cfg.QueueEnabled = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("QUEUE_MAX_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.QueueMaxSize = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_ENABLED"); v != "" {
		cfg.RateLimitEnabled = strings.ToLower(v) == "true"
	}
	if v := os.Getenv("RATE_LIMIT_RPM"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimitRPM = n
		}
	}
	if v := os.Getenv("RATE_LIMIT_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RateLimitConcurrent = n
		}
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("DEFAULT_SYSTEM_PROMPT"); v != "" {
		cfg.DefaultSystemPrompt = v
	}

	return cfg
}
