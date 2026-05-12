// Package search 实现联邦搜索
package search

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Cache 搜索结果缓存 (内联实现)
type Cache struct {
	mu      sync.RWMutex
	items   map[string]*cacheItem
	maxSize int
	ttl     time.Duration
	hits    int64
	misses  int64
}

type cacheItem struct {
	results   []SearchResult
	createdAt time.Time
}

// NewCache 创建缓存
func NewCache(maxSize int, ttl time.Duration) *Cache {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &Cache{
		items:   make(map[string]*cacheItem),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *Cache) cacheKey(req SearchRequest) string {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:8])
}

// Get 获取缓存
func (c *Cache) Get(key string) ([]SearchResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[key]
	if !ok || time.Since(item.createdAt) > c.ttl {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}
	atomic.AddInt64(&c.hits, 1)
	return item.results, true
}

// Set 设置缓存
func (c *Cache) Set(key string, results []SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.maxSize {
		// 简单淘汰: 删除最旧的一项
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.items {
			if oldestKey == "" || v.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.createdAt
			}
		}
		delete(c.items, oldestKey)
	}
	c.items[key] = &cacheItem{results: results, createdAt: time.Now()}
}

// Hits 缓存命中
func (c *Cache) Hits() int64 { return atomic.LoadInt64(&c.hits) }

// Misses 缓存未命中
func (c *Cache) Misses() int64 { return atomic.LoadInt64(&c.misses) }

// FederatedSearch 联邦搜索引擎
type FederatedSearch struct {
	client      *http.Client
	cache       *Cache
	maxParallel int
	timeout     time.Duration
}

// NodeInfo 节点信息
type NodeInfo struct {
	ID        string
	Host      string
	Port      int
	AgentPort int
	Healthy   bool
	NodeType  string // G6: "worker" (default) or "regional_hub" for hierarchical search
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query       string     `json:"query"`
	ImageData   string     `json:"image_data,omitempty"`
	QueryVec    []float32  `json:"query_vec,omitempty"`
	CameraIDs   []string   `json:"camera_ids,omitempty"`
	TimeStart   *time.Time `json:"time_start,omitempty"`
	TimeEnd     *time.Time `json:"time_end,omitempty"`
	Limit       int        `json:"limit"`
	TextWeight  float64    `json:"text_weight,omitempty"`
	ImageWeight float64    `json:"image_weight,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	TaskID         string    `json:"task_id"`
	DetectionID    string    `json:"detection_id,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	CameraID       string    `json:"camera_id"`
	StreamName     string    `json:"stream_name,omitempty"`
	Thumbnail      string    `json:"thumbnail,omitempty"`
	Result         string    `json:"result"`
	NodeID         string    `json:"node_id"`
	Score          float64   `json:"score"`
	TextScore      float64   `json:"text_score,omitempty"`
	ImageScore     float64   `json:"image_score,omitempty"`
}

// FederatedSearchResponse 联邦搜索响应
type FederatedSearchResponse struct {
	Results      []SearchResult `json:"results"`
	TotalNodes   int            `json:"total_nodes"`
	SuccessNodes int            `json:"success_nodes"`
	TotalResults int            `json:"total_results"`
	SearchTimeMs int64          `json:"search_time_ms"`
}

// NewFederatedSearch 创建联邦搜索引擎
func NewFederatedSearch(cacheSize int, cacheTTL time.Duration) *FederatedSearch {
	return &FederatedSearch{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:       NewCache(cacheSize, cacheTTL),
		maxParallel: 32,
		timeout:     30 * time.Second,
	}
}

// Search 执行联邦搜索
func (f *FederatedSearch) Search(ctx context.Context, nodes []NodeInfo, req SearchRequest) (*FederatedSearchResponse, error) {
	startTime := time.Now()

	if req.Limit <= 0 {
		req.Limit = 50
	}

	// 过滤健康节点
	healthyNodes := make([]NodeInfo, 0)
	for _, n := range nodes {
		if n.Healthy {
			healthyNodes = append(healthyNodes, n)
		}
	}

	if len(healthyNodes) == 0 {
		return &FederatedSearchResponse{
			Results:      []SearchResult{},
			TotalNodes:   len(nodes),
			SuccessNodes: 0,
			SearchTimeMs: time.Since(startTime).Milliseconds(),
		}, nil
	}

	// 并行查询所有节点
	var wg sync.WaitGroup
	resultsChan := make(chan nodeSearchResult, len(healthyNodes))

	// 限制并行数
	sem := make(chan struct{}, f.maxParallel)

	for _, node := range healthyNodes {
		wg.Add(1)
		go func(n NodeInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results, err := f.searchNode(ctx, n, req)
			resultsChan <- nodeSearchResult{
				nodeID:  n.ID,
				results: results,
				err:     err,
			}
		}(node)
	}

	wg.Wait()
	close(resultsChan)

	// 聚合结果
	var allResults []SearchResult
	successCount := 0

	for nr := range resultsChan {
		if nr.err != nil {
			log.Printf("[FederatedSearch] 节点 %s 查询失败: %v", nr.nodeID, nr.err)
			continue
		}
		successCount++
		allResults = append(allResults, nr.results...)
	}

	// 排序和去重
	finalResults := f.rankAndDedupe(allResults, req.Limit)

	return &FederatedSearchResponse{
		Results:      finalResults,
		TotalNodes:   len(nodes),
		SuccessNodes: successCount,
		TotalResults: len(allResults),
		SearchTimeMs: time.Since(startTime).Milliseconds(),
	}, nil
}

type nodeSearchResult struct {
	nodeID  string
	results []SearchResult
	err     error
}

// searchNode 查询单个节点
func (f *FederatedSearch) searchNode(ctx context.Context, node NodeInfo, req SearchRequest) ([]SearchResult, error) {
	url := fmt.Sprintf("http://%s:%d/api/v1/search", node.Host, node.AgentPort)

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var response struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	// 标记结果来源节点
	for i := range response.Results {
		response.Results[i].NodeID = node.ID
	}

	return response.Results, nil
}

// rankAndDedupe 排序和去重
func (f *FederatedSearch) rankAndDedupe(results []SearchResult, limit int) []SearchResult {
	if len(results) == 0 {
		return results
	}

	// 去重（基于TaskID）
	seen := make(map[string]bool)
	unique := make([]SearchResult, 0)
	for _, r := range results {
		if !seen[r.TaskID] {
			seen[r.TaskID] = true
			unique = append(unique, r)
		}
	}

	// 按分数排序
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Score > unique[j].Score
	})

	// 限制数量
	if len(unique) > limit {
		unique = unique[:limit]
	}

	return unique
}

// SearchByImage 以图搜图
func (f *FederatedSearch) SearchByImage(ctx context.Context, nodes []NodeInfo, imageData string, limit int) (*FederatedSearchResponse, error) {
	req := SearchRequest{
		ImageData:   imageData,
		Limit:       limit,
		ImageWeight: 1.0,
		TextWeight:  0,
	}
	return f.Search(ctx, nodes, req)
}

// SearchByVector performs a vector-based federated search across Workers (§5.8).
// Used by video search and image search after embedding.
func (f *FederatedSearch) SearchByVector(queryVec []float32, topK int, nodes []NodeInfo) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	req := SearchRequest{
		QueryVec: queryVec,
		Limit:    topK,
	}
	resp, err := f.Search(ctx, nodes, req)
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// SearchStats 搜索统计
type SearchStats struct {
	TotalSearches    int64          `json:"total_searches"`
	CacheHits        int64          `json:"cache_hits"`
	CacheMisses      int64          `json:"cache_misses"`
	AvgSearchTimeMs  float64        `json:"avg_search_time_ms"`
	NodeStats        map[string]int `json:"node_stats"`
}

// GetStats 获取统计信息
func (f *FederatedSearch) GetStats() *SearchStats {
	return &SearchStats{
		CacheHits:   f.cache.Hits(),
		CacheMisses: f.cache.Misses(),
		NodeStats:   make(map[string]int),
	}
}
