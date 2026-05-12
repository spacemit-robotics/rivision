// Package rag implements Retrieval-Augmented Generation for video knowledge base Q&A (N6).
// Pipeline: IntentParser → FederatedSearch → VLM Rerank → Context Assembly → LLM → Citation.
package rag

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rivision/rivision-hub/internal/search"
)

// Config holds RAG engine configuration.
type Config struct {
	VLMURL       string `yaml:"vlm_url" json:"vlm_url"`             // LLM/VLM endpoint for generation
	MaxContext   int    `yaml:"max_context" json:"max_context"`       // max reference items (default 5)
	VLMRerank    bool   `yaml:"vlm_rerank" json:"vlm_rerank"`         // enable VLM re-ranking before generation
	CacheTTL     int    `yaml:"cache_ttl_sec" json:"cache_ttl_sec"`   // response cache TTL seconds (default 300)
	MaxTokens    int    `yaml:"max_tokens" json:"max_tokens"`         // LLM max output tokens (default 512)
	Temperature  float64 `yaml:"temperature" json:"temperature"`      // LLM temperature (default 0.3)
	StreamChunk  int    `yaml:"stream_chunk" json:"stream_chunk"`      // token chunk size for streaming (default 1)
}

func (c *Config) defaults() {
	if c.MaxContext <= 0 {
		c.MaxContext = 5
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = 300
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 512
	}
	if c.Temperature <= 0 {
		c.Temperature = 0.3
	}
	if c.StreamChunk <= 0 {
		c.StreamChunk = 1
	}
}

// RAGRequest is the API input for RAG Q&A.
type RAGRequest struct {
	Query      string `json:"query" binding:"required"`
	MaxContext int    `json:"max_context,omitempty"`  // override config default
	VLMRerank  *bool  `json:"vlm_rerank,omitempty"`   // override config default
	Stream     bool   `json:"stream,omitempty"`        // enable streaming output
}

// RAGResponse is the complete (non-streaming) API response.
type RAGResponse struct {
	Answer       string       `json:"answer"`
	References   []Reference  `json:"references"`
	Intent       interface{}  `json:"intent,omitempty"`     // parsed intent details
	SearchTimeMs int64        `json:"search_time_ms"`
	LLMTimeMs    int64        `json:"llm_time_ms"`
	TotalTimeMs  int64        `json:"total_time_ms"`
	FromCache    bool         `json:"from_cache"`
}

// Reference is a cited source in the RAG answer.
type Reference struct {
	RefID       int    `json:"ref_id"`
	DetectionID string `json:"detection_id"`
	CameraID    string `json:"camera_id"`
	Timestamp   string `json:"timestamp"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail_url,omitempty"`
	Score       float64 `json:"score"`
}

// contextBlock represents one retrieved item formatted for the LLM prompt.
type contextBlock struct {
	RefID       int
	Timestamp   string
	CameraID    string
	Description string
	DetectionID string
	Score       float64
	Thumbnail   string
}

// Engine orchestrates the RAG pipeline.
type Engine struct {
	cfg        Config
	federated  *search.FederatedSearch
	parser     *search.IntentParser
	reranker   *search.VLMReranker
	getNodes   func() []search.NodeInfo
	getCameras func() []search.CameraInfo
	client     *http.Client

	// response cache
	cacheMu sync.RWMutex
	cache   map[string]*cacheEntry
}

type cacheEntry struct {
	resp    *RAGResponse
	created time.Time
}

// NewEngine creates a RAG engine.
func NewEngine(cfg Config, federated *search.FederatedSearch, parser *search.IntentParser,
	getNodes func() []search.NodeInfo, getCameras func() []search.CameraInfo) *Engine {

	cfg.defaults()
	return &Engine{
		cfg:        cfg,
		federated:  federated,
		parser:     parser,
		getNodes:   getNodes,
		getCameras: getCameras,
		client:     &http.Client{Timeout: 60 * time.Second},
		cache:      make(map[string]*cacheEntry),
	}
}

// SetReranker enables VLM re-ranking in the pipeline (G5 integration).
func (e *Engine) SetReranker(r *search.VLMReranker) {
	e.reranker = r
}

// Query executes the full RAG pipeline and returns a complete response.
func (e *Engine) Query(ctx context.Context, req RAGRequest) (*RAGResponse, error) {
	totalStart := time.Now()

	// Check cache
	cacheKey := req.Query
	if cached := e.getCache(cacheKey); cached != nil {
		cached.FromCache = true
		return cached, nil
	}

	maxCtx := e.cfg.MaxContext
	if req.MaxContext > 0 {
		maxCtx = req.MaxContext
	}
	useRerank := e.cfg.VLMRerank
	if req.VLMRerank != nil {
		useRerank = *req.VLMRerank
	}

	// Step 1: Parse intent (G8)
	var cameras []search.CameraInfo
	if e.getCameras != nil {
		cameras = e.getCameras()
	}
	intent := e.parser.Parse(req.Query, cameras, time.Now())

	// Step 2: Federated vector search
	searchStart := time.Now()
	searchReq := search.SearchRequest{
		Query:     intent.SearchText,
		CameraIDs: intent.CameraIDs,
		TimeStart: intent.TimeStart,
		TimeEnd:   intent.TimeEnd,
		Limit:     maxCtx * 4, // over-fetch for reranking
	}

	nodes := e.getNodes()
	searchResp, err := e.federated.Search(ctx, nodes, searchReq)
	if err != nil {
		return nil, fmt.Errorf("RAG search: %w", err)
	}
	searchTimeMs := time.Since(searchStart).Milliseconds()

	// Step 3: Optional VLM re-ranking (G5)
	candidates := searchResp.Results
	if useRerank && e.reranker != nil && len(candidates) > maxCtx {
		reranked := e.reranker.Rerank(ctx, intent.SearchText, candidates, nodes)
		// Convert reranked back to SearchResult, take top maxCtx
		candidates = make([]search.SearchResult, 0, maxCtx)
		for i, rr := range reranked {
			if i >= maxCtx {
				break
			}
			candidates = append(candidates, search.SearchResult{
				DetectionID: rr.DetectionID,
				CameraID:    rr.CameraID,
				Thumbnail:   rr.Thumbnail,
				Score:       rr.FinalScore,
				Timestamp:   rr.Timestamp,
			})
		}
	} else if len(candidates) > maxCtx {
		candidates = candidates[:maxCtx]
	}

	// Step 4: Build context blocks
	blocks := make([]contextBlock, len(candidates))
	for i, c := range candidates {
		blocks[i] = contextBlock{
			RefID:       i + 1,
			Timestamp:   c.Timestamp.Format("2006-01-02T15:04:05"),
			CameraID:    c.CameraID,
			Description: c.Result,
			DetectionID: c.DetectionID,
			Score:       c.Score,
			Thumbnail:   c.Thumbnail,
		}
	}

	// Step 5: Assemble prompt and call LLM
	prompt := BuildPrompt(req.Query, blocks)
	llmStart := time.Now()
	answer, err := e.callLLM(ctx, prompt)
	if err != nil {
		// Fallback: return search results without LLM answer
		log.Printf("[rag] LLM call failed: %v, returning search-only results", err)
		answer = e.buildFallbackAnswer(blocks)
	}
	llmTimeMs := time.Since(llmStart).Milliseconds()

	// Step 6: Build references
	refs := make([]Reference, len(blocks))
	for i, b := range blocks {
		refs[i] = Reference{
			RefID:       b.RefID,
			DetectionID: b.DetectionID,
			CameraID:    b.CameraID,
			Timestamp:   b.Timestamp,
			Description: b.Description,
			Thumbnail:   b.Thumbnail,
			Score:       b.Score,
		}
	}

	// Step 7: Resolve citation markers
	answer = ResolveCitations(answer, refs)

	resp := &RAGResponse{
		Answer:       answer,
		References:   refs,
		Intent:       intent,
		SearchTimeMs: searchTimeMs,
		LLMTimeMs:    llmTimeMs,
		TotalTimeMs:  time.Since(totalStart).Milliseconds(),
	}

	// Cache the response
	e.setCache(cacheKey, resp)

	return resp, nil
}

// callLLM sends the assembled prompt to the VLM/LLM endpoint.
func (e *Engine) callLLM(ctx context.Context, prompt string) (string, error) {
	if e.cfg.VLMURL == "" {
		return "", fmt.Errorf("VLM URL not configured")
	}

	reqBody := map[string]interface{}{
		"prompt":      prompt,
		"max_tokens":  e.cfg.MaxTokens,
		"temperature": e.cfg.Temperature,
		"stream":      false,
	}
	body, _ := json.Marshal(reqBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.VLMURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("LLM request: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Response string `json:"response"`
		Text     string `json:"text"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		// Try plain text response
		return string(data), nil
	}
	if result.Response != "" {
		return result.Response, nil
	}
	return result.Text, nil
}

// StreamEvent represents a single SSE event from the LLM streaming pipeline.
type StreamEvent struct {
	Type    string      `json:"type"`              // "references", "token", "done", "error"
	Token   string      `json:"token,omitempty"`
	Refs    []Reference `json:"references,omitempty"`
	Intent  interface{} `json:"intent,omitempty"`
	Metrics *StreamDone `json:"metrics,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// StreamDone contains timing metrics sent at the end of a stream.
type StreamDone struct {
	SearchTimeMs int64 `json:"search_time_ms"`
	LLMTimeMs    int64 `json:"llm_time_ms"`
	TotalTimeMs  int64 `json:"total_time_ms"`
}

// QueryStreamPipeline executes the RAG search pipeline (steps 1-4) without calling LLM,
// returning the prepared prompt, references, and timing for the streaming handler.
func (e *Engine) QueryStreamPipeline(ctx context.Context, req RAGRequest) (prompt string, refs []Reference, intent interface{}, searchTimeMs int64, err error) {
	maxCtx := e.cfg.MaxContext
	if req.MaxContext > 0 {
		maxCtx = req.MaxContext
	}
	useRerank := e.cfg.VLMRerank
	if req.VLMRerank != nil {
		useRerank = *req.VLMRerank
	}

	// Step 1: Parse intent (G8)
	var cameras []search.CameraInfo
	if e.getCameras != nil {
		cameras = e.getCameras()
	}
	parsedIntent := e.parser.Parse(req.Query, cameras, time.Now())
	intent = parsedIntent

	// Step 2: Federated vector search
	searchStart := time.Now()
	searchReq := search.SearchRequest{
		Query:     parsedIntent.SearchText,
		CameraIDs: parsedIntent.CameraIDs,
		TimeStart: parsedIntent.TimeStart,
		TimeEnd:   parsedIntent.TimeEnd,
		Limit:     maxCtx * 4,
	}

	nodes := e.getNodes()
	searchResp, searchErr := e.federated.Search(ctx, nodes, searchReq)
	if searchErr != nil {
		return "", nil, intent, 0, fmt.Errorf("RAG search: %w", searchErr)
	}
	searchTimeMs = time.Since(searchStart).Milliseconds()

	// Step 3: Optional VLM re-ranking (G5)
	candidates := searchResp.Results
	if useRerank && e.reranker != nil && len(candidates) > maxCtx {
		reranked := e.reranker.Rerank(ctx, parsedIntent.SearchText, candidates, nodes)
		candidates = make([]search.SearchResult, 0, maxCtx)
		for i, rr := range reranked {
			if i >= maxCtx {
				break
			}
			candidates = append(candidates, search.SearchResult{
				DetectionID: rr.DetectionID,
				CameraID:    rr.CameraID,
				Thumbnail:   rr.Thumbnail,
				Score:       rr.FinalScore,
				Timestamp:   rr.Timestamp,
			})
		}
	} else if len(candidates) > maxCtx {
		candidates = candidates[:maxCtx]
	}

	// Step 4: Build context blocks + references
	blocks := make([]contextBlock, len(candidates))
	refs = make([]Reference, len(candidates))
	for i, c := range candidates {
		blocks[i] = contextBlock{
			RefID:       i + 1,
			Timestamp:   c.Timestamp.Format("2006-01-02T15:04:05"),
			CameraID:    c.CameraID,
			Description: c.Result,
			DetectionID: c.DetectionID,
			Score:       c.Score,
			Thumbnail:   c.Thumbnail,
		}
		refs[i] = Reference{
			RefID:       i + 1,
			DetectionID: c.DetectionID,
			CameraID:    c.CameraID,
			Timestamp:   blocks[i].Timestamp,
			Description: c.Result,
			Thumbnail:   c.Thumbnail,
			Score:       c.Score,
		}
	}

	prompt = BuildPrompt(req.Query, blocks)
	return prompt, refs, intent, searchTimeMs, nil
}

// CallLLMStream sends the prompt to the LLM with streaming enabled and
// emits tokens to the returned channel. The channel is closed when done.
func (e *Engine) CallLLMStream(ctx context.Context, prompt string) (<-chan string, <-chan error) {
	tokens := make(chan string, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(tokens)
		defer close(errCh)

		if e.cfg.VLMURL == "" {
			errCh <- fmt.Errorf("VLM URL not configured")
			return
		}

		reqBody := map[string]interface{}{
			"prompt":      prompt,
			"max_tokens":  e.cfg.MaxTokens,
			"temperature": e.cfg.Temperature,
			"stream":      true,
		}
		body, _ := json.Marshal(reqBody)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.VLMURL+"/api/generate", bytes.NewReader(body))
		if err != nil {
			errCh <- err
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		resp, err := e.client.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("LLM stream request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
			errCh <- fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, string(data))
			return
		}

		// Parse streaming response — supports multiple formats:
		// 1. SSE: "data: {"response":"token"}\n\n"
		// 2. NDJSON: {"response":"token"}\n
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and SSE comments
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Handle SSE "data: " prefix
			if strings.HasPrefix(line, "data: ") {
				line = strings.TrimPrefix(line, "data: ")
			}

			// Check for SSE done signal
			if line == "[DONE]" {
				return
			}

			// Parse JSON token object
			var chunk struct {
				Response string `json:"response"` // Ollama format
				Text     string `json:"text"`     // generic format
				Content  string `json:"content"`  // OpenAI-like
				Done     bool   `json:"done"`     // Ollama done flag
				Choices  []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"` // OpenAI chat format
			}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				// Not JSON — emit as raw text
				select {
				case tokens <- line:
				case <-ctx.Done():
					return
				}
				continue
			}

			if chunk.Done {
				return
			}

			// Extract token from whichever field is populated
			tok := chunk.Response
			if tok == "" {
				tok = chunk.Text
			}
			if tok == "" {
				tok = chunk.Content
			}
			if tok == "" && len(chunk.Choices) > 0 {
				tok = chunk.Choices[0].Delta.Content
			}
			if tok == "" {
				continue
			}

			select {
			case tokens <- tok:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stream read: %w", err)
		}
	}()

	return tokens, errCh
}

// buildFallbackAnswer generates a simple answer from context blocks when LLM is unavailable.
func (e *Engine) buildFallbackAnswer(blocks []contextBlock) string {
	if len(blocks) == 0 {
		return "未找到相关视频记录。"
	}
	var sb bytes.Buffer
	sb.WriteString("根据检索到的视频记录:\n\n")
	for _, b := range blocks {
		fmt.Fprintf(&sb, "- [REF-%d] %s %s: %s\n", b.RefID, b.Timestamp, b.CameraID, b.Description)
	}
	return sb.String()
}

// --- Cache ---

func (e *Engine) getCache(key string) *RAGResponse {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()
	entry, ok := e.cache[key]
	if !ok {
		return nil
	}
	if time.Since(entry.created) > time.Duration(e.cfg.CacheTTL)*time.Second {
		return nil
	}
	// Return a copy
	resp := *entry.resp
	return &resp
}

func (e *Engine) setCache(key string, resp *RAGResponse) {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()

	// Evict stale entries periodically
	if len(e.cache) > 100 {
		now := time.Now()
		ttl := time.Duration(e.cfg.CacheTTL) * time.Second
		for k, v := range e.cache {
			if now.Sub(v.created) > ttl {
				delete(e.cache, k)
			}
		}
	}

	e.cache[key] = &cacheEntry{resp: resp, created: time.Now()}
}

// ClearCache removes all cached responses.
func (e *Engine) ClearCache() {
	e.cacheMu.Lock()
	defer e.cacheMu.Unlock()
	e.cache = make(map[string]*cacheEntry)
}
