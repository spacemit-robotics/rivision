// Package integrations 实现各通知渠道集成
package integrations

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/rivision/rivision-hub/pkg/models"
)

// DingTalkConfig 钉钉配置
type DingTalkConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
	Secret     string `yaml:"secret"`
}

// DingTalkNotifier 钉钉通知器
type DingTalkNotifier struct {
	config *DingTalkConfig
	client *http.Client
}

// NewDingTalkNotifier 创建钉钉通知器
func NewDingTalkNotifier(cfg *DingTalkConfig) *DingTalkNotifier {
	return &DingTalkNotifier{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name 返回渠道名称
func (d *DingTalkNotifier) Name() string {
	return "dingtalk"
}

// IsEnabled 是否启用
func (d *DingTalkNotifier) IsEnabled() bool {
	return d.config != nil && d.config.Enabled && d.config.WebhookURL != ""
}

// Send 发送告警
func (d *DingTalkNotifier) Send(alert *models.Alert) error {
	if !d.IsEnabled() {
		return nil
	}

	// 构建签名URL
	webhookURL := d.config.WebhookURL
	if d.config.Secret != "" {
		webhookURL = d.signURL(webhookURL)
	}

	// 构建消息
	title := fmt.Sprintf("[%s] %s", alert.Level, alert.Title)
	text := fmt.Sprintf(`### %s

- **告警ID**: %s
- **节点**: %s
- **摄像头**: %s
- **级别**: %s
- **时间**: %s

%s`,
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
			"title": title,
			"text":  text,
		},
	}

	body, _ := json.Marshal(msg)
	resp, err := d.client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("发送失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if errcode, ok := result["errcode"].(float64); ok && errcode != 0 {
			return fmt.Errorf("钉钉错误: %v", result["errmsg"])
		}
	}

	return nil
}

// signURL 签名URL
func (d *DingTalkNotifier) signURL(webhookURL string) string {
	timestamp := time.Now().UnixMilli()
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, d.config.Secret)

	h := hmac.New(sha256.New, []byte(d.config.Secret))
	h.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%s&timestamp=%d&sign=%s", webhookURL, timestamp, url.QueryEscape(sign))
}
