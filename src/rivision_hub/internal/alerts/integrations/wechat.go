// Package integrations 实现各通知渠道集成
package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rivision/rivision-hub/pkg/models"
)

// WeChatConfig 企业微信配置
type WeChatConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// WeChatNotifier 企业微信通知器
type WeChatNotifier struct {
	config *WeChatConfig
	client *http.Client
}

// NewWeChatNotifier 创建企业微信通知器
func NewWeChatNotifier(cfg *WeChatConfig) *WeChatNotifier {
	return &WeChatNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 返回渠道名称
func (w *WeChatNotifier) Name() string {
	return "wechat"
}

// IsEnabled 是否启用
func (w *WeChatNotifier) IsEnabled() bool {
	return w.config != nil && w.config.Enabled && w.config.WebhookURL != ""
}

// Send 发送告警
func (w *WeChatNotifier) Send(alert *models.Alert) error {
	if !w.IsEnabled() {
		return nil
	}

	// 构建消息
	levelEmoji := w.getLevelEmoji(alert.Level)
	content := fmt.Sprintf(`%s **%s**

> **告警ID**: %s
> **节点**: %s
> **摄像头**: %s
> **级别**: %s
> **时间**: %s

%s`,
		levelEmoji,
		alert.Title,
		alert.ID[:8],
		alert.NodeID,
		alert.CameraID,
		alert.Level,
		alert.CreatedAt.Format("2006-01-02 15:04:05"),
		alert.Description,
	)

	msg := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	}

	body, _ := json.Marshal(msg)
	resp, err := w.client.Post(w.config.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return nil
}

func (w *WeChatNotifier) getLevelEmoji(level models.AlertLevel) string {
	switch level {
	case models.AlertLevelCritical:
		return "🔴"
	case models.AlertLevelHigh:
		return "🟠"
	case models.AlertLevelMedium:
		return "🟡"
	default:
		return "🟢"
	}
}
