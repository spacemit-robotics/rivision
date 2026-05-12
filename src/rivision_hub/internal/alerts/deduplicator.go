// Package alerts 实现告警去重
package alerts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rivision/rivision-hub/pkg/models"
)

// Deduplicator 告警去重器
type Deduplicator struct {
	mu       sync.RWMutex
	window   time.Duration
	records  map[string]time.Time
	maxItems int
}

// NewDeduplicator 创建去重器
func NewDeduplicator(window time.Duration) *Deduplicator {
	return &Deduplicator{
		window:   window,
		records:  make(map[string]time.Time),
		maxItems: 10000,
	}
}

// generateKey 生成告警唯一键
func (d *Deduplicator) generateKey(alert *models.Alert) string {
	// 使用节点ID + 摄像头ID + 告警类型 + 级别生成唯一键
	raw := fmt.Sprintf("%s:%s:%s:%s", alert.NodeID, alert.CameraID, alert.Type, alert.Level)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}

// IsDuplicate 检查是否重复
func (d *Deduplicator) IsDuplicate(alert *models.Alert) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	key := d.generateKey(alert)
	if lastTime, exists := d.records[key]; exists {
		// 在去重窗口内视为重复
		if time.Since(lastTime) < d.window {
			return true
		}
	}
	return false
}

// Record 记录告警
func (d *Deduplicator) Record(alert *models.Alert) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := d.generateKey(alert)
	d.records[key] = time.Now()
}

// CleanupLoop 清理过期记录
func (d *Deduplicator) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(d.window / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.cleanup()
		}
	}
}

// cleanup 清理过期记录
func (d *Deduplicator) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for key, recordTime := range d.records {
		if now.Sub(recordTime) > d.window {
			expired = append(expired, key)
		}
	}

	for _, key := range expired {
		delete(d.records, key)
	}

	// 防止内存无限增长
	if len(d.records) > d.maxItems {
		// 删除最老的一半
		keys := make([]string, 0, len(d.records))
		for k := range d.records {
			keys = append(keys, k)
		}
		for i := 0; i < len(keys)/2; i++ {
			delete(d.records, keys[i])
		}
	}
}

// Stats 获取统计信息
func (d *Deduplicator) Stats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return map[string]interface{}{
		"active_records": len(d.records),
		"window_seconds": d.window.Seconds(),
		"max_items":      d.maxItems,
	}
}
