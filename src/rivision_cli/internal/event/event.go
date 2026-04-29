// Package event 提供事件管理功能
package event

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType 事件类型
type EventType string

const (
	EventTypeYOLODetection EventType = "yolo.detection"
	EventTypeVLMResult     EventType = "vlm.result"
	EventTypeCameraOnline  EventType = "camera.online"
	EventTypeCameraOffline EventType = "camera.offline"
	EventTypeCameraError   EventType = "camera.error"
	EventTypeSystemStart   EventType = "system.start"
	EventTypeSystemStop    EventType = "system.stop"
	EventTypeSystemError   EventType = "system.error"
)

// Event 事件结构
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	CameraID  string                 `json:"camera_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// EventHandler 事件处理函数
type EventHandler func(event *Event)

// EventBus 事件总线
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
	allHandlers []EventHandler
	eventChan chan *Event
	stopChan  chan struct{}
	history   []*Event
	maxHistory int
}

// NewEventBus 创建事件总线
func NewEventBus() *EventBus {
	eb := &EventBus{
		handlers:    make(map[EventType][]EventHandler),
		allHandlers: make([]EventHandler, 0),
		eventChan:   make(chan *Event, 100),
		stopChan:    make(chan struct{}),
		history:     make([]*Event, 0),
		maxHistory:  100,
	}

	go eb.run()
	return eb
}

// run 运行事件分发循环
func (eb *EventBus) run() {
	for {
		select {
		case event := <-eb.eventChan:
			eb.dispatch(event)
		case <-eb.stopChan:
			return
		}
	}
}

// dispatch 分发事件
func (eb *EventBus) dispatch(event *Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// 保存到历史
	eb.history = append(eb.history, event)
	if len(eb.history) > eb.maxHistory {
		eb.history = eb.history[1:]
	}

	// 调用特定类型的处理器
	if handlers, ok := eb.handlers[event.Type]; ok {
		for _, h := range handlers {
			go h(event)
		}
	}

	// 调用全局处理器
	for _, h := range eb.allHandlers {
		go h(event)
	}
}

// Subscribe 订阅特定类型的事件
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// SubscribeAll 订阅所有事件
func (eb *EventBus) SubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.allHandlers = append(eb.allHandlers, handler)
}

// Publish 发布事件
func (eb *EventBus) Publish(event *Event) {
	if event.ID == "" {
		event.ID = generateEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	select {
	case eb.eventChan <- event:
	default:
		// 队列满了，丢弃事件
	}
}

// PublishYOLODetection 发布 YOLO 检测事件
func (eb *EventBus) PublishYOLODetection(cameraID string, detectionCount int, inferenceTimeMs int64) {
	eb.Publish(&Event{
		Type:      EventTypeYOLODetection,
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"detection_count":   detectionCount,
			"inference_time_ms": inferenceTimeMs,
		},
	})
}

// PublishVLMResult 发布 VLM 结果事件
func (eb *EventBus) PublishVLMResult(cameraID string, triggerMode string, inferenceTimeMs int64) {
	eb.Publish(&Event{
		Type:      EventTypeVLMResult,
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"trigger_mode":      triggerMode,
			"inference_time_ms": inferenceTimeMs,
		},
	})
}

// PublishCameraOnline 发布摄像头上线事件
func (eb *EventBus) PublishCameraOnline(cameraID string, cameraName string) {
	eb.Publish(&Event{
		Type:      EventTypeCameraOnline,
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"camera_name": cameraName,
		},
	})
}

// PublishCameraOffline 发布摄像头离线事件
func (eb *EventBus) PublishCameraOffline(cameraID string, reason string) {
	eb.Publish(&Event{
		Type:      EventTypeCameraOffline,
		CameraID:  cameraID,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"reason": reason,
		},
	})
}

// PublishSystemError 发布系统错误事件
func (eb *EventBus) PublishSystemError(component string, err error) {
	eb.Publish(&Event{
		Type:      EventTypeSystemError,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"component": component,
			"error":     err.Error(),
		},
	})
}

// GetHistory 获取事件历史
func (eb *EventBus) GetHistory(limit int) []*Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if limit <= 0 || limit > len(eb.history) {
		limit = len(eb.history)
	}

	// 返回最近的事件
	result := make([]*Event, limit)
	start := len(eb.history) - limit
	copy(result, eb.history[start:])

	return result
}

// GetHistoryByType 按类型获取事件历史
func (eb *EventBus) GetHistoryByType(eventType EventType, limit int) []*Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	var result []*Event
	for i := len(eb.history) - 1; i >= 0 && len(result) < limit; i-- {
		if eb.history[i].Type == eventType {
			result = append(result, eb.history[i])
		}
	}

	return result
}

// Stop 停止事件总线
func (eb *EventBus) Stop() {
	close(eb.stopChan)
}

// ToJSON 将事件转换为 JSON
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// generateEventID 生成事件 ID
func generateEventID() string {
	return time.Now().Format("20060102150405.000000")
}
