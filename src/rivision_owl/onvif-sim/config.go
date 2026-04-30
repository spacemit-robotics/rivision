// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 配置文件结构
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Device   DeviceConfig   `yaml:"device"`
	Features FeaturesConfig `yaml:"features"`
	Profiles []ProfileDef   `yaml:"profiles"`
}

type ServerConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	RTSPPort int    `yaml:"rtsp_port"` // RTSP 服务端口
	Verbose  bool   `yaml:"verbose"`
}

type AuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type DeviceConfig struct {
	Manufacturer string `yaml:"manufacturer"`
	Model        string `yaml:"model"`
	Serial       string `yaml:"serial"`
}

type FeaturesConfig struct {
	PTZ     bool `yaml:"ptz"`
	Imaging bool `yaml:"imaging"`
}

type ProfileDef struct {
	Name      string `yaml:"name"`
	Width     int    `yaml:"width"`
	Height    int    `yaml:"height"`
	Framerate int    `yaml:"framerate"`
	Bitrate   int    `yaml:"bitrate"`
	Source    string `yaml:"source"`
	PTZ       bool   `yaml:"ptz"`
}

// LoadConfig 从 YAML 文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 15188
	}
	if cfg.Server.RTSPPort == 0 {
		cfg.Server.RTSPPort = 18555 // 默认 RTSP 端口 (避免与 rivision-cli 的 18554 冲突)
	}
	if cfg.Auth.Username == "" {
		cfg.Auth.Username = "admin"
	}
	if cfg.Auth.Password == "" {
		cfg.Auth.Password = "admin"
	}
	if cfg.Device.Manufacturer == "" {
		cfg.Device.Manufacturer = "RiVision"
	}
	if cfg.Device.Model == "" {
		cfg.Device.Model = "ONVIF-Sim Virtual Camera"
	}
	if cfg.Device.Serial == "" {
		cfg.Device.Serial = "ONVIF-SIM-001"
	}

	// 如果没有配置 profiles，使用默认
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = defaultProfiles()
	}

	return &cfg, nil
}

func defaultProfiles() []ProfileDef {
	return []ProfileDef{
		{Name: "主摄像头 - 高清", Width: 1920, Height: 1080, Framerate: 30, Bitrate: 4096, Source: "rtsp://localhost:8554/stream0", PTZ: true},
		{Name: "广角摄像头", Width: 1280, Height: 720, Framerate: 30, Bitrate: 2048, Source: "rtsp://localhost:8554/stream1", PTZ: false},
		{Name: "长焦摄像头", Width: 1920, Height: 1080, Framerate: 25, Bitrate: 6144, Source: "rtsp://localhost:8554/stream2", PTZ: true},
	}
}
