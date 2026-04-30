// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package camera 提供 go2rtc 集成功能
package camera

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"time"
)

// Go2rtcClient go2rtc 客户端
type Go2rtcClient struct {
	baseURL    string
	httpClient *http.Client
}

// Go2rtcStream go2rtc 流信息
type Go2rtcStream struct {
	Name      string      `json:"name"`
	Sources   []string   `json:"sources"`
	Producers interface{} `json:"producers"` // 可能是数组
	Consumers interface{} `json:"consumers"` // 可能是数组或整数
}

// Producer represents a producer in the go2rtc stream format.
type Producer struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

// NewGo2rtcClient 创建 go2rtc 客户端
func NewGo2rtcClient(baseURL string) *Go2rtcClient {
	return &Go2rtcClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetStreams 获取所有流列表
func (c *Go2rtcClient) GetStreams() (map[string]Go2rtcStream, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/streams")
	if err != nil {
		return nil, fmt.Errorf("获取流列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取流列表失败: HTTP %d", resp.StatusCode)
	}

	var streams map[string]Go2rtcStream
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, fmt.Errorf("解析流列表失败: %w", err)
	}

	return streams, nil
}

// CaptureFrame 从 go2rtc 捕获一帧 JPEG 图像（带重试）
func (c *Go2rtcClient) CaptureFrame(streamName string) (*Frame, error) {
	// 使用 go2rtc 的 frame.jpeg 端点获取当前帧
	url := fmt.Sprintf("%s/api/frame.jpeg?src=%s", c.baseURL, streamName)
	
	var lastErr error
	maxRetries := 3
	
	for i := 0; i < maxRetries; i++ {
		resp, err := c.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("获取帧失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("获取帧失败: HTTP %d", resp.StatusCode)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 读取 JPEG 数据
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取帧数据失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// 检查数据长度
		if len(data) < 100 {
			lastErr = fmt.Errorf("帧数据太小: %d bytes", len(data))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// ★ 仅读取 JPEG 头获取尺寸（不解码像素，省 ~20-30ms/帧）
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			lastErr = fmt.Errorf("解析 JPEG 头失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return &Frame{
			CameraID:  streamName,
			Timestamp: time.Now(),
			Data:      data,
			Width:     cfg.Width,
			Height:    cfg.Height,
			Format:    "jpeg",
		}, nil
	}
	
	return nil, lastErr
}

// CaptureFrameResized 从 go2rtc 捕获一帧并由服务端缩放到指定宽度
// ★ YOLO 专用：YOLO 模型输入通常为 640×640，发送 1280×720 纯粹浪费
//   - 数据量减少 ~75%（250KB → 40-60KB）
//   - K3 RISC-V 推理加速 4-10×
//   - 网络传输时间大幅降低
func (c *Go2rtcClient) CaptureFrameResized(streamName string, width int) (*Frame, error) {
	url := fmt.Sprintf("%s/api/frame.jpeg?src=%s&width=%d", c.baseURL, streamName, width)

	var lastErr error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		resp, err := c.httpClient.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("获取帧失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("获取帧失败: HTTP %d", resp.StatusCode)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("读取帧数据失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(data) < 100 {
			lastErr = fmt.Errorf("帧数据太小: %d bytes", len(data))
			time.Sleep(100 * time.Millisecond)
			continue
		}

		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			lastErr = fmt.Errorf("解析 JPEG 头失败: %w", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return &Frame{
			CameraID:  streamName,
			Timestamp: time.Now(),
			Data:      data,
			Width:     cfg.Width,
			Height:    cfg.Height,
			Format:    "jpeg",
		}, nil
	}

	return nil, lastErr
}

// CaptureFrameRaw 从 go2rtc 捕获一帧原始图像数据
func (c *Go2rtcClient) CaptureFrameRaw(streamName string) (image.Image, error) {
	url := fmt.Sprintf("%s/api/frame.jpeg?src=%s", c.baseURL, streamName)
	
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("获取帧失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取帧失败: HTTP %d", resp.StatusCode)
	}

	img, err := jpeg.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 JPEG 失败: %w", err)
	}

	return img, nil
}

// IsStreamAvailable 检查流是否可用
func (c *Go2rtcClient) IsStreamAvailable(streamName string) bool {
	streams, err := c.GetStreams()
	if err != nil {
		return false
	}
	_, ok := streams[streamName]
	return ok
}

// AddStream 添加流
func (c *Go2rtcClient) AddStream(name string, source string) error {
	url := fmt.Sprintf("%s/api/streams?src=%s&name=%s", c.baseURL, source, name)
	
	req, err := http.NewRequest("PUT", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("添加流失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("添加流失败: HTTP %d", resp.StatusCode)
	}

	return nil
}

// RemoveStream 移除流
func (c *Go2rtcClient) RemoveStream(name string) error {
	url := fmt.Sprintf("%s/api/streams?src=%s", c.baseURL, name)
	
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("移除流失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("移除流失败: HTTP %d", resp.StatusCode)
	}

	return nil
}
