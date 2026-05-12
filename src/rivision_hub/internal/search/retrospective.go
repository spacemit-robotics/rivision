// Package search — retrospective.go implements post-hoc VLM-verified search (G4).
// Workflow: vector search → fetch thumbnails → VLM verify each → filter + rank.
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
	"strings"
	"sync"
	"time"
)

// RetrospectiveRequest is the input for a VLM-verified retrospective search.
type RetrospectiveRequest struct {
	Query     string     `json:"query" binding:"required"`
	VLMVerify bool       `json:"vlm_verify"`                 // default true
	TimeStart *time.Time `json:"time_start,omitempty"`
	TimeEnd   *time.Time `json:"time_end,omitempty"`
	CameraIDs []string   `json:"camera_ids,omitempty"`
	TopK      int        `json:"top_k"`                      // vector search candidates (default 20)
	VLMTopK   int        `json:"vlm_top_k"`                  // max VLM-verified results (default 5)
}

// RetrospectiveResult is a single VLM-verified search result.
type RetrospectiveResult struct {
	DetectionID    string  `json:"detection_id"`
	CameraID       string  `json:"camera_id"`
	NodeID         string  `json:"node_id"`
	Timestamp      string  `json:"timestamp,omitempty"`
	ThumbnailURL   string  `json:"thumbnail_url,omitempty"`
	VectorScore    float64 `json:"vector_score"`
	VLMVerified    bool    `json:"vlm_verified"`
	VLMDescription string  `json:"vlm_description,omitempty"`
	VLMConfidence  float64 `json:"vlm_confidence,omitempty"`
}

// RetrospectiveResponse is the full response for a retrospective search.
type RetrospectiveResponse struct {
	Results        []RetrospectiveResult `json:"results"`
	TotalCandidates int                  `json:"total_candidates"`
	VerifiedCount   int                  `json:"verified_count"`
	SearchTimeMs    int64                `json:"search_time_ms"`
}

// RetrospectiveSearcher performs vector search + VLM verification.
type RetrospectiveSearcher struct {
	federated  *FederatedSearch
	vlmURL     string       // Worker VLM endpoint pattern or Hub VLM URL
	client     *http.Client
	maxParallel int
}

// NewRetrospectiveSearcher creates a retrospective search engine.
func NewRetrospectiveSearcher(federated *FederatedSearch, vlmURL string) *RetrospectiveSearcher {
	return &RetrospectiveSearcher{
		federated:   federated,
		vlmURL:      vlmURL,
		client:      &http.Client{Timeout: 30 * time.Second},
		maxParallel: 3, // limit parallel VLM calls to avoid overloading Workers
	}
}

// Search executes a retrospective search with optional VLM verification.
func (rs *RetrospectiveSearcher) Search(ctx context.Context, nodes []NodeInfo, req RetrospectiveRequest) (*RetrospectiveResponse, error) {
	startTime := time.Now()

	if req.TopK <= 0 {
		req.TopK = 20
	}
	if req.VLMTopK <= 0 {
		req.VLMTopK = 5
	}

	// Step 1: Vector search for candidates
	searchReq := SearchRequest{
		Query:     req.Query,
		TimeStart: req.TimeStart,
		TimeEnd:   req.TimeEnd,
		CameraIDs: req.CameraIDs,
		Limit:     req.TopK,
	}
	fedResp, err := rs.federated.Search(ctx, nodes, searchReq)
	if err != nil {
		return nil, fmt.Errorf("federated search: %w", err)
	}

	candidates := make([]RetrospectiveResult, 0, len(fedResp.Results))
	for _, r := range fedResp.Results {
		candidates = append(candidates, RetrospectiveResult{
			DetectionID:  r.DetectionID,
			CameraID:     r.CameraID,
			NodeID:       r.NodeID,
			Timestamp:    r.Timestamp.Format(time.RFC3339),
			ThumbnailURL: buildThumbnailURL(r.NodeID, r.DetectionID, nodes),
			VectorScore:  r.Score,
		})
	}

	totalCandidates := len(candidates)

	// Step 2: VLM verification (if enabled)
	if req.VLMVerify && rs.vlmURL != "" && len(candidates) > 0 {
		rs.vlmVerifyCandidates(ctx, req.Query, candidates, nodes)
	}

	// Step 3: Filter and rank
	var verified []RetrospectiveResult
	for _, c := range candidates {
		if req.VLMVerify && c.VLMVerified {
			verified = append(verified, c)
		} else if !req.VLMVerify {
			verified = append(verified, c)
		}
	}

	// Sort by VLM confidence (if verified) or vector score
	sort.Slice(verified, func(i, j int) bool {
		if verified[i].VLMConfidence != verified[j].VLMConfidence {
			return verified[i].VLMConfidence > verified[j].VLMConfidence
		}
		return verified[i].VectorScore > verified[j].VectorScore
	})

	if len(verified) > req.VLMTopK {
		verified = verified[:req.VLMTopK]
	}

	// If VLM was enabled but found nothing, fall back to vector results
	if len(verified) == 0 && len(candidates) > 0 {
		limit := req.VLMTopK
		if limit > len(candidates) {
			limit = len(candidates)
		}
		verified = candidates[:limit]
	}

	return &RetrospectiveResponse{
		Results:         verified,
		TotalCandidates: totalCandidates,
		VerifiedCount:   countVerified(verified),
		SearchTimeMs:    time.Since(startTime).Milliseconds(),
	}, nil
}

func (rs *RetrospectiveSearcher) vlmVerifyCandidates(ctx context.Context, query string, candidates []RetrospectiveResult, nodes []NodeInfo) {
	sem := make(chan struct{}, rs.maxParallel)
	var wg sync.WaitGroup

	for i := range candidates {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c := &candidates[idx]

			// Fetch thumbnail from Worker
			thumb, err := rs.fetchThumbnail(ctx, c.ThumbnailURL)
			if err != nil {
				log.Printf("[retrospective] fetch thumbnail %s: %v", c.DetectionID, err)
				return
			}

			// VLM verify
			verified, desc, conf, err := rs.vlmVerify(ctx, thumb, query, nodes, c.NodeID)
			if err != nil {
				log.Printf("[retrospective] VLM verify %s: %v", c.DetectionID, err)
				return
			}

			c.VLMVerified = verified
			c.VLMDescription = desc
			c.VLMConfidence = conf
		}(i)
	}

	wg.Wait()
}

func (rs *RetrospectiveSearcher) fetchThumbnail(ctx context.Context, url string) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("empty thumbnail URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := rs.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func (rs *RetrospectiveSearcher) vlmVerify(ctx context.Context, thumbnail []byte, query string, nodes []NodeInfo, nodeID string) (bool, string, float64, error) {
	prompt := fmt.Sprintf(`用户搜索: "%s"
请判断图片内容是否与搜索意图匹配。输出纯JSON:
{"match": true/false, "description": "图片描述", "confidence": 0.0-1.0}`, query)

	// Find the node's infer endpoint for VLM
	vlmURL := rs.vlmURL
	for _, n := range nodes {
		if n.ID == nodeID && n.Host != "" {
			vlmURL = fmt.Sprintf("http://%s:9081/api/vlm/analyze", n.Host)
			break
		}
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"image":       fmt.Sprintf("data:image/jpeg;base64,%s", encodeBase64Bytes(thumbnail)),
		"prompt":      prompt,
		"max_tokens":  100,
		"temperature": 0.1,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, vlmURL, bytes.NewReader(reqBody))
	if err != nil {
		return false, "", 0, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rs.client.Do(httpReq)
	if err != nil {
		return false, "", 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return false, "", 0, fmt.Errorf("VLM HTTP %d", resp.StatusCode)
	}

	var vlmResp struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &vlmResp); err != nil {
		return false, "", 0, err
	}

	// Parse VLM JSON output
	var result struct {
		Match       bool    `json:"match"`
		Description string  `json:"description"`
		Confidence  float64 `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(vlmResp.Description), &result); err != nil {
		// If JSON parsing fails, do simple keyword matching
		desc := strings.ToLower(vlmResp.Description)
		isMatch := strings.Contains(desc, "是") || strings.Contains(desc, "yes") || strings.Contains(desc, "match")
		return isMatch, vlmResp.Description, 0.5, nil
	}

	return result.Match, result.Description, result.Confidence, nil
}

// --- Helpers ---

func buildThumbnailURL(nodeID, detectionID string, nodes []NodeInfo) string {
	for _, n := range nodes {
		if n.ID == nodeID && n.Host != "" {
			return fmt.Sprintf("http://%s:%d/api/v1/thumbnails/%s.jpg", n.Host, n.AgentPort, detectionID)
		}
	}
	return ""
}

func encodeBase64Bytes(data []byte) string {
	const base64chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf bytes.Buffer
	for i := 0; i < len(data); i += 3 {
		var b0, b1, b2 byte
		b0 = data[i]
		if i+1 < len(data) { b1 = data[i+1] }
		if i+2 < len(data) { b2 = data[i+2] }
		buf.WriteByte(base64chars[(b0>>2)&0x3F])
		buf.WriteByte(base64chars[((b0<<4)|(b1>>4))&0x3F])
		if i+1 < len(data) {
			buf.WriteByte(base64chars[((b1<<2)|(b2>>6))&0x3F])
		} else {
			buf.WriteByte('=')
		}
		if i+2 < len(data) {
			buf.WriteByte(base64chars[b2&0x3F])
		} else {
			buf.WriteByte('=')
		}
	}
	return buf.String()
}

func countVerified(results []RetrospectiveResult) int {
	count := 0
	for _, r := range results {
		if r.VLMVerified {
			count++
		}
	}
	return count
}
