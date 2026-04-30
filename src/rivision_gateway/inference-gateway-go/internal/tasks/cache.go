// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"sync"
	"time"
)

// ResultCache 结果缓存 - 支持TTL自动清理
type ResultCache struct {
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	expiry     map[string]time.Time
	defaultTTL time.Duration
	maxSize    int
}

type cacheEntry struct {
	Value     interface{}
	CreatedAt time.Time
}

// NewResultCache 创建结果缓存
func NewResultCache(defaultTTL time.Duration, maxSize int) *ResultCache {
	if defaultTTL == 0 {
		defaultTTL = 5 * time.Minute
	}
	if maxSize == 0 {
		maxSize = 1000
	}
	return &ResultCache{
		cache:      make(map[string]*cacheEntry),
		expiry:     make(map[string]time.Time),
		defaultTTL: defaultTTL,
		maxSize:    maxSize,
	}
}

func (c *ResultCache) Set(key string, value interface{}, ttl ...time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cache) >= c.maxSize {
		c.evictOldest()
	}

	d := c.defaultTTL
	if len(ttl) > 0 && ttl[0] > 0 {
		d = ttl[0]
	}

	c.cache[key] = &cacheEntry{Value: value, CreatedAt: time.Now()}
	c.expiry[key] = time.Now().Add(d)
}

func (c *ResultCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(c.expiry[key]) {
		// 过期，延迟删除
		go func() {
			c.mu.Lock()
			delete(c.cache, key)
			delete(c.expiry, key)
			c.mu.Unlock()
		}()
		return nil, false
	}
	return entry.Value, true
}

func (c *ResultCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
	delete(c.expiry, key)
}

func (c *ResultCache) CleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, exp := range c.expiry {
		if now.After(exp) {
			delete(c.cache, k)
			delete(c.expiry, k)
		}
	}
}

func (c *ResultCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

func (c *ResultCache) evictOldest() {
	if len(c.cache) == 0 {
		return
	}
	evictCount := len(c.cache) / 10
	if evictCount < 1 {
		evictCount = 1
	}

	type kv struct {
		key string
		t   time.Time
	}
	entries := make([]kv, 0, len(c.cache))
	for k, v := range c.cache {
		entries = append(entries, kv{k, v.CreatedAt})
	}
	// 简单排序找最旧的
	for i := 0; i < evictCount && i < len(entries); i++ {
		oldest := i
		for j := i + 1; j < len(entries); j++ {
			if entries[j].t.Before(entries[oldest].t) {
				oldest = j
			}
		}
		entries[i], entries[oldest] = entries[oldest], entries[i]
		delete(c.cache, entries[i].key)
		delete(c.expiry, entries[i].key)
	}
}

// 全局实例
var (
	globalResultCache     *ResultCache
	globalResultCacheOnce sync.Once
)

// GetResultCache 获取全局结果缓存实例
func GetResultCache() *ResultCache {
	globalResultCacheOnce.Do(func() {
		globalResultCache = NewResultCache(5*time.Minute, 1000)
	})
	return globalResultCache
}
