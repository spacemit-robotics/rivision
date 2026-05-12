// Package alerts 定义类型别名
package alerts

import "github.com/rivision/rivision-hub/internal/alerts/integrations"

// 类型别名，方便外部使用
type (
	WeChartConfig  = integrations.WeChatConfig
	DingTalkConfig = integrations.DingTalkConfig
	WebhookConfig  = integrations.WebhookConfig
	EmailConfig    = integrations.EmailConfig
)

// 构造函数别名
var (
	NewWeChatNotifier   = integrations.NewWeChatNotifier
	NewDingTalkNotifier = integrations.NewDingTalkNotifier
	NewWebhookNotifier  = integrations.NewWebhookNotifier
	NewEmailNotifier    = integrations.NewEmailNotifier
)
