// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors 多个验证错误
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// Validate 验证配置
func (c *Config) Validate() error {
	var errors ValidationErrors

	// 验证节点配置
	if err := c.validateNode(); err != nil {
		errors = append(errors, err...)
	}

	// 验证服务器配置
	if err := c.validateServer(); err != nil {
		errors = append(errors, err...)
	}

	// 验证 VLM 配置
	if err := c.validateVLM(); err != nil {
		errors = append(errors, err...)
	}

	// 验证 YOLO 配置
	if err := c.validateYOLO(); err != nil {
		errors = append(errors, err...)
	}

	// 验证摄像头配置
	if err := c.validateCameras(); err != nil {
		errors = append(errors, err...)
	}

	// 验证存储配置
	if err := c.validateStorage(); err != nil {
		errors = append(errors, err...)
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

func (c *Config) validateNode() ValidationErrors {
	var errors ValidationErrors

	if c.Node.ID == "" {
		errors = append(errors, ValidationError{
			Field:   "node.id",
			Message: "节点 ID 不能为空",
		})
	}

	validRoles := map[string]bool{"master": true, "yolo": true, "compute": true}
	if !validRoles[c.Node.Role] {
		errors = append(errors, ValidationError{
			Field:   "node.role",
			Message: fmt.Sprintf("无效的节点角色: %s (有效值: master, yolo, compute)", c.Node.Role),
		})
	}

	return errors
}

func (c *Config) validateServer() ValidationErrors {
	var errors ValidationErrors

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errors = append(errors, ValidationError{
			Field:   "server.port",
			Message: fmt.Sprintf("无效的端口号: %d (有效范围: 1-65535)", c.Server.Port),
		})
	}

	return errors
}

func (c *Config) validateVLM() ValidationErrors {
	var errors ValidationErrors

	if c.VLM.GatewayURL == "" {
		errors = append(errors, ValidationError{
			Field:   "vlm.gateway_url",
			Message: "Gateway URL 不能为空",
		})
	} else {
		if _, err := url.Parse(c.VLM.GatewayURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "vlm.gateway_url",
				Message: fmt.Sprintf("无效的 URL: %s", c.VLM.GatewayURL),
			})
		}
	}

	if c.VLM.Timeout <= 0 {
		errors = append(errors, ValidationError{
			Field:   "vlm.timeout",
			Message: "超时时间必须大于 0",
		})
	}

	if c.VLM.MaxConcurrent < 1 {
		errors = append(errors, ValidationError{
			Field:   "vlm.max_concurrent",
			Message: "最大并发数必须大于 0",
		})
	}

	return errors
}

func (c *Config) validateYOLO() ValidationErrors {
	var errors ValidationErrors

	if !c.YOLO.Enabled {
		return errors
	}

	// 检查模型目录
	if c.YOLO.ModelDir != "" {
		if _, err := os.Stat(c.YOLO.ModelDir); os.IsNotExist(err) {
			errors = append(errors, ValidationError{
				Field:   "yolo.model_dir",
				Message: fmt.Sprintf("模型目录不存在: %s", c.YOLO.ModelDir),
			})
		}
	}

	// 检查默认模型文件
	if c.YOLO.DefaultModel != "" && c.YOLO.ModelDir != "" {
		modelPath := filepath.Join(c.YOLO.ModelDir, c.YOLO.DefaultModel)
		if _, err := os.Stat(modelPath); os.IsNotExist(err) {
			errors = append(errors, ValidationError{
				Field:   "yolo.default_model",
				Message: fmt.Sprintf("模型文件不存在: %s", modelPath),
			})
		}
	}

	// 验证阈值
	if c.YOLO.ConfThresh < 0 || c.YOLO.ConfThresh > 1 {
		errors = append(errors, ValidationError{
			Field:   "yolo.conf_thresh",
			Message: fmt.Sprintf("置信度阈值必须在 0-1 之间: %f", c.YOLO.ConfThresh),
		})
	}

	if c.YOLO.NMSThresh < 0 || c.YOLO.NMSThresh > 1 {
		errors = append(errors, ValidationError{
			Field:   "yolo.nms_thresh",
			Message: fmt.Sprintf("NMS 阈值必须在 0-1 之间: %f", c.YOLO.NMSThresh),
		})
	}

	return errors
}

func (c *Config) validateCameras() ValidationErrors {
	var errors ValidationErrors

	cameraIDs := make(map[string]bool)
	for i, cam := range c.Cameras {
		prefix := fmt.Sprintf("cameras[%d]", i)

		if cam.ID == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".id",
				Message: "摄像头 ID 不能为空",
			})
		} else if cameraIDs[cam.ID] {
			errors = append(errors, ValidationError{
				Field:   prefix + ".id",
				Message: fmt.Sprintf("摄像头 ID 重复: %s", cam.ID),
			})
		} else {
			cameraIDs[cam.ID] = true
		}

		if cam.Source == "" {
			errors = append(errors, ValidationError{
				Field:   prefix + ".source",
				Message: "摄像头源地址不能为空",
			})
		}

		// 验证 VLM 触发模式
		validModes := map[string]bool{"interval": true, "yolo": true, "manual": true}
		if cam.VLM.Enabled && !validModes[cam.VLM.TriggerMode] {
			errors = append(errors, ValidationError{
				Field:   prefix + ".vlm.trigger_mode",
				Message: fmt.Sprintf("无效的触发模式: %s (有效值: interval, yolo, manual)", cam.VLM.TriggerMode),
			})
		}
	}

	return errors
}

func (c *Config) validateStorage() ValidationErrors {
	var errors ValidationErrors

	validTypes := map[string]bool{"sqlite": true, "file": true}
	if !validTypes[c.Storage.Type] {
		errors = append(errors, ValidationError{
			Field:   "storage.type",
			Message: fmt.Sprintf("无效的存储类型: %s (有效值: sqlite, file)", c.Storage.Type),
		})
	}

	if c.Storage.Path == "" {
		errors = append(errors, ValidationError{
			Field:   "storage.path",
			Message: "存储路径不能为空",
		})
	}

	return errors
}

// ValidateAndFix 验证并尝试修复配置
func (c *Config) ValidateAndFix() error {
	// 设置默认值
	if c.Node.ID == "" {
		c.Node.ID = "master-1"
	}
	if c.Node.Role == "" {
		c.Node.Role = "master"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8280
	}
	if c.VLM.GatewayURL == "" {
		c.VLM.GatewayURL = "http://localhost:8081"
	}
	if c.VLM.Timeout == 0 {
		c.VLM.Timeout = 30 * 1e9 // 30 seconds in nanoseconds
	}
	if c.VLM.MaxConcurrent == 0 {
		c.VLM.MaxConcurrent = 4
	}
	if c.Storage.Type == "" {
		c.Storage.Type = "sqlite"
	}
	if c.Storage.Path == "" {
		c.Storage.Path = "./data/rivision.db"
	}

	// 再次验证
	return c.Validate()
}
