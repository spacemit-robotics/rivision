// Package integrations 实现各通知渠道集成
package integrations

import (
	"fmt"
	"net/smtp"
	"strings"

	"github.com/rivision/rivision-hub/pkg/models"
)

// EmailConfig 邮件配置
type EmailConfig struct {
	Enabled    bool     `yaml:"enabled"`
	SMTPHost   string   `yaml:"smtp_host"`
	SMTPPort   int      `yaml:"smtp_port"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	To         []string `yaml:"to"`
	UseTLS     bool     `yaml:"use_tls"`
}

// EmailNotifier 邮件通知器
type EmailNotifier struct {
	config *EmailConfig
}

// NewEmailNotifier 创建邮件通知器
func NewEmailNotifier(cfg *EmailConfig) *EmailNotifier {
	return &EmailNotifier{config: cfg}
}

// Name 返回渠道名称
func (e *EmailNotifier) Name() string {
	return "email"
}

// IsEnabled 是否启用
func (e *EmailNotifier) IsEnabled() bool {
	return e.config != nil && e.config.Enabled && e.config.SMTPHost != "" && len(e.config.To) > 0
}

// Send 发送告警
func (e *EmailNotifier) Send(alert *models.Alert) error {
	if !e.IsEnabled() {
		return nil
	}

	// 构建邮件
	subject := fmt.Sprintf("[RiVision告警][%s] %s", alert.Level, alert.Title)
	body := e.buildEmailBody(alert)

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n%s",
		e.config.From,
		strings.Join(e.config.To, ","),
		subject,
		body,
	)

	// 发送邮件
	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)
	auth := smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)

	err := smtp.SendMail(addr, auth, e.config.From, e.config.To, []byte(msg))
	if err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}

	return nil
}

// buildEmailBody 构建邮件正文
func (e *EmailNotifier) buildEmailBody(alert *models.Alert) string {
	levelColor := e.getLevelColor(alert.Level)

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: %s; color: white; padding: 15px; border-radius: 5px 5px 0 0; }
        .content { background: #f9f9f9; padding: 20px; border: 1px solid #ddd; border-top: none; }
        .info-row { margin: 10px 0; }
        .label { font-weight: bold; color: #666; }
        .thumbnail { max-width: 100%%; margin-top: 15px; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2 style="margin:0">%s</h2>
        </div>
        <div class="content">
            <div class="info-row"><span class="label">告警ID:</span> %s</div>
            <div class="info-row"><span class="label">节点:</span> %s</div>
            <div class="info-row"><span class="label">摄像头:</span> %s</div>
            <div class="info-row"><span class="label">级别:</span> %s</div>
            <div class="info-row"><span class="label">时间:</span> %s</div>
            <div class="info-row"><span class="label">描述:</span><br>%s</div>
        </div>
    </div>
</body>
</html>`,
		levelColor,
		alert.Title,
		alert.ID,
		alert.NodeID,
		alert.CameraID,
		alert.Level,
		alert.CreatedAt.Format("2006-01-02 15:04:05"),
		alert.Description,
	)

	return html
}

func (e *EmailNotifier) getLevelColor(level models.AlertLevel) string {
	switch level {
	case models.AlertLevelCritical:
		return "#dc3545"
	case models.AlertLevelHigh:
		return "#fd7e14"
	case models.AlertLevelMedium:
		return "#ffc107"
	default:
		return "#28a745"
	}
}
