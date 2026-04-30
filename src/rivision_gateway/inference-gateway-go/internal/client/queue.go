// Package client 提供请求队列
package client

import (
	"sync"
	"time"
)

// RequestPriority 请求优先级
type RequestPriority int

const (
	PriorityLow    RequestPriority = 0
	PriorityNormal RequestPriority = 1
	PriorityHigh   RequestPriority = 2
)

// QueuedRequest 队列中的请求
type QueuedRequest struct {
	ID        string
	Priority  RequestPriority
	CreatedAt time.Time
	Done      chan struct{}
}

// RequestQueue 请求队列
type RequestQueue struct {
	maxSize int
	timeout time.Duration
	queue   []*QueuedRequest
	mu      sync.Mutex
}

// NewRequestQueue 创建请求队列
func NewRequestQueue(maxSize int, timeout time.Duration) *RequestQueue {
	return &RequestQueue{
		maxSize: maxSize,
		timeout: timeout,
		queue:   make([]*QueuedRequest, 0),
	}
}

// Enqueue 入队
func (q *RequestQueue) Enqueue(req *QueuedRequest) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) >= q.maxSize {
		return false
	}

	q.queue = append(q.queue, req)
	return true
}

// Dequeue 出队
func (q *RequestQueue) Dequeue() *QueuedRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		return nil
	}

	// 按优先级排序
	var highest *QueuedRequest
	highestIdx := -1
	for i, req := range q.queue {
		if highest == nil || req.Priority > highest.Priority {
			highest = req
			highestIdx = i
		}
	}

	if highestIdx >= 0 {
		q.queue = append(q.queue[:highestIdx], q.queue[highestIdx+1:]...)
	}

	return highest
}

// Size 队列大小
func (q *RequestQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// Clear 清空队列
func (q *RequestQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = make([]*QueuedRequest, 0)
}
