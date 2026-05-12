// Package alerts 实现告警分发
package alerts

import (
	"log"
	"sync"

	"github.com/rivision/rivision-hub/pkg/models"
)

// NotificationChannel 通知渠道接口
type NotificationChannel interface {
	Name() string
	Send(alert *models.Alert) error
	IsEnabled() bool
}

// Dispatcher 告警分发器
type Dispatcher struct {
	mu       sync.RWMutex
	channels []NotificationChannel
}

// NewDispatcher 创建分发器
func NewDispatcher(cfg IntegrationsConfig) *Dispatcher {
	d := &Dispatcher{
		channels: make([]NotificationChannel, 0),
	}

	// 注册各通知渠道
	if cfg.WeChat != nil && cfg.WeChat.Enabled {
		d.channels = append(d.channels, NewWeChatNotifier(cfg.WeChat))
	}
	if cfg.DingTalk != nil && cfg.DingTalk.Enabled {
		d.channels = append(d.channels, NewDingTalkNotifier(cfg.DingTalk))
	}
	if cfg.Webhook != nil && cfg.Webhook.Enabled {
		d.channels = append(d.channels, NewWebhookNotifier(cfg.Webhook))
	}
	if cfg.Email != nil && cfg.Email.Enabled {
		d.channels = append(d.channels, NewEmailNotifier(cfg.Email))
	}

	log.Printf("[Dispatcher] 已注册 %d 个通知渠道", len(d.channels))
	return d
}

// Dispatch 分发告警到所有渠道
func (d *Dispatcher) Dispatch(alert *models.Alert) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 根据告警级别筛选渠道
	for _, ch := range d.channels {
		if !ch.IsEnabled() {
			continue
		}

		// 低级别告警只发送到部分渠道
		if alert.Level == models.AlertLevelLow {
			// 低级别告警不通过即时通讯
			if ch.Name() == "wechat" || ch.Name() == "dingtalk" {
				continue
			}
		}

		go func(channel NotificationChannel) {
			if err := channel.Send(alert); err != nil {
				log.Printf("[Dispatcher] %s 发送失败: %v", channel.Name(), err)
			} else {
				log.Printf("[Dispatcher] %s 发送成功: %s", channel.Name(), alert.ID)
			}
		}(ch)
	}
}

// AddChannel 添加通知渠道
func (d *Dispatcher) AddChannel(ch NotificationChannel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = append(d.channels, ch)
}

// GetChannels 获取所有渠道状态
func (d *Dispatcher) GetChannels() []map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(d.channels))
	for _, ch := range d.channels {
		result = append(result, map[string]interface{}{
			"name":    ch.Name(),
			"enabled": ch.IsEnabled(),
		})
	}
	return result
}
