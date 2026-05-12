// Package search — reranker.go implements optional VLM-based re-ranking of search results (G5).
// After vector search returns Top-K candidates, the reranker scores each against the query
// using VLM, then blends VLM relevance with the original vector score.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

// RerankerConfig controls VLM re-ranking behavior.
type RerankerConfig struct {
	Enabled      bool    `json:"enabled" yaml:"enabled"`
	RerankTopK   int     `json:"rerank_top_k" yaml:"rerank_top_k"`     // how many candidates to re-rank (default 10)
	VLMWeight    float64 `json:"vlm_weight" yaml:"vlm_weight"`         // weight for VLM score (default 0.4)
	VectorWeight float64 `json:"vector_weight" yaml:"vector_weight"`   // weight for vector score (default 0.6)
	MaxParallel  int     `json:"max_parallel" yaml:"max_parallel"`     // parallel VLM requests (default 3)
}

// DefaultRerankerConfig returns sane defaults for re-ranking.
func DefaultRerankerConfig() RerankerConfig {
	return RerankerConfig{
		Enabled:      false, // opt-in
		RerankTopK:   10,
		VLMWeight:    0.4,
		VectorWeight: 0.6,
		MaxParallel:  3,
	}
}

// VLMReranker re-ranks search results using VLM relevance scoring.
type VLMReranker struct {
	cfg    RerankerConfig
	client *http.Client
}

// NewVLMReranker creates a VLM reranker with the given config.
func NewVLMReranker(cfg RerankerConfig) *VLMReranker {
	if cfg.RerankTopK <= 0 {
		cfg.RerankTopK = 10
	}
	if cfg.VLMWeight <= 0 {
		cfg.VLMWeight = 0.4
	}
	if cfg.VectorWeight <= 0 {
		cfg.VectorWeight = 0.6
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 3
	}
	return &VLMReranker{
		cfg:    cfg,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

// RerankResult is a search result augmented with VLM relevance score.
type RerankResult struct {
	SearchResult
	VLMRelevance float64 `json:"vlm_relevance,omitempty"` // 0.0-1.0
	VLMReason    string  `json:"vlm_reason,omitempty"`
	FinalScore   float64 `json:"final_score"`
}

// Rerank takes vector search results, fetches thumbnails, scores each with VLM,
// and returns results sorted by blended score.
func (r *VLMReranker) Rerank(ctx context.Context, query string, results []SearchResult, nodes []NodeInfo) []RerankResult {
	// Take only top candidates for VLM re-ranking
	candidates := results
	if len(candidates) > r.cfg.RerankTopK {
		candidates = candidates[:r.cfg.RerankTopK]
	}

	reranked := make([]RerankResult, len(candidates))
	var wg sync.WaitGroup
	sem := make(chan struct{}, r.cfg.MaxParallel)

	for i, sr := range candidates {
		reranked[i] = RerankResult{SearchResult: sr, FinalScore: sr.Score}

		wg.Add(1)
		go func(idx int, result SearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Fetch thumbnail
			thumbURL := buildThumbnailURL(result.NodeID, result.DetectionID, nodes)
			if thumbURL == "" {
				return
			}

			thumb, err := r.fetchThumbnail(ctx, thumbURL)
			if err != nil {
				log.Printf("[reranker] fetch thumb %s: %v", result.DetectionID, err)
				return
			}

			// Score with VLM
			relevance, reason, err := r.vlmScore(ctx, thumb, query, result, nodes)
			if err != nil {
				log.Printf("[reranker] VLM score %s: %v", result.DetectionID, err)
				return
			}

			reranked[idx].VLMRelevance = relevance
			reranked[idx].VLMReason = reason
			reranked[idx].FinalScore = r.cfg.VectorWeight*result.Score + r.cfg.VLMWeight*relevance
		}(i, sr)
	}

	wg.Wait()

	// Sort by final score
	sort.Slice(reranked, func(i, j int) bool {
		return reranked[i].FinalScore > reranked[j].FinalScore
	})

	return reranked
}

func (r *VLMReranker) fetchThumbnail(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func (r *VLMReranker) vlmScore(ctx context.Context, thumbnail []byte, query string, sr SearchResult, nodes []NodeInfo) (float64, string, error) {
	prompt := fmt.Sprintf(`用户搜索: "%s"
请评估图片内容与搜索意图的相关度。输出纯JSON:
{"relevance": 0-10, "reason": "简要说明"}`, query)

	// Route to the Worker's VLM endpoint
	vlmURL := ""
	for _, n := range nodes {
		if n.ID == sr.NodeID && n.Host != "" {
			vlmURL = fmt.Sprintf("http://%s:9081/api/vlm/analyze", n.Host)
			break
		}
	}
	if vlmURL == "" {
		return 0, "", fmt.Errorf("no VLM endpoint for node %s", sr.NodeID)
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"image":       fmt.Sprintf("data:image/jpeg;base64,%s", encodeBase64Bytes(thumbnail)),
		"prompt":      prompt,
		"max_tokens":  80,
		"temperature": 0.1,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, vlmURL, bytes.NewReader(reqBody))
	if err != nil {
		return 0, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("VLM HTTP %d", resp.StatusCode)
	}

	var vlmResp struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &vlmResp); err != nil {
		return 0, "", err
	}

	var scored struct {
		Relevance float64 `json:"relevance"`
		Reason    string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(vlmResp.Description), &scored); err != nil {
		return 0.5, vlmResp.Description, nil // fallback
	}

	return scored.Relevance / 10.0, scored.Reason, nil // normalise to 0.0-1.0
}
