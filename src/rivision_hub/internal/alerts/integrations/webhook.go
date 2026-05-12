// Package integrations 实现各通知渠道集成
package integrations

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rivision/rivision-hub/pkg/models"
)

// WebhookConfig 通用Webhook配置
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Secret  string `yaml:"secret"`
	Headers map[string]string `yaml:"headers"`
}

// WebhookNotifier 通用Webhook通知器
type WebhookNotifier struct {
	config *WebhookConfig
	client *http.Client
}

// NewWebhookNotifier 创建Webhook通知器
func NewWebhookNotifier(cfg *WebhookConfig) *WebhookNotifier {
	return &WebhookNotifier{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name 返回渠道名称
func (w *WebhookNotifier) Name() string {
	return "webhook"
}

// IsEnabled 是否启用
func (w *WebhookNotifier) IsEnabled() bool {
	return w.config != nil && w.config.Enabled && w.config.URL != ""
}

// Send 发送告警
func (w *WebhookNotifier) Send(alert *models.Alert) error {
	if !w.IsEnabled() {
		return nil
	}

	// 构建Webhook payload
	payload := map[string]interface{}{
		"event_type": "alert",
		"timestamp":  time.Now().Format(time.RFC3339),
		"alert": map[string]interface{}{
			"id":          alert.ID,
			"node_id":     alert.NodeID,
			"camera_id":   alert.CameraID,
			"type":        alert.Type,
			"level":       alert.Level,
			"status":      alert.Status,
			"title":       alert.Title,
			"description": alert.Description,
			"thumbnail":   alert.Thumbnail,
			"vlm_result":  alert.VLMResult,
			"detections":  alert.Detections,
			"created_at":  alert.CreatedAt.Format(time.RFC3339),
		},
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", w.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "RiVision-Hub/1.0")

	// 添加自定义头
	for k, v := range w.config.Headers {
		req.Header.Set(k, v)
	}

	// 添加签名
	if w.config.Secret != "" {
		signature := w.sign(body)
		req.Header.Set("X-RiVision-Signature", signature)
		req.Header.Set("X-RiVision-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

// sign 签名请求体
func (w *WebhookNotifier) sign(body []byte) string {
	h := hmac.New(sha256.New, []byte(w.config.Secret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
