// Package rag — handler.go provides HTTP handlers for RAG Q&A API (N6).
package rag

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler wraps the RAG engine for HTTP serving.
type Handler struct {
	engine *Engine
}

// NewHandler creates a RAG API handler.
func NewHandler(engine *Engine) *Handler {
	return &Handler{engine: engine}
}

// SetupRoutes registers RAG routes on the given router group.
func (h *Handler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/rag")
	{
		g.POST("/query", h.Query)
		g.POST("/query/stream", h.QueryStream)
		g.DELETE("/cache", h.ClearCache)
	}
}

// Query handles POST /api/v1/rag/query — non-streaming RAG Q&A.
func (h *Handler) Query(c *gin.Context) {
	var req RAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	resp, err := h.engine.Query(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// QueryStream handles POST /api/v1/rag/query/stream — SSE streaming RAG Q&A.
// Pipeline: search+rerank synchronously → stream references → real LLM token streaming → done.
func (h *Handler) QueryStream(c *gin.Context) {
	var req RAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Stream = true

	ctx := c.Request.Context()
	totalStart := time.Now()

	// Phase 1: Execute search pipeline (non-LLM) synchronously
	prompt, refs, intent, searchTimeMs, err := h.engine.QueryStreamPipeline(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// Phase 2: Send references event
	refsData, _ := json.Marshal(StreamEvent{
		Type:   "references",
		Refs:   refs,
		Intent: intent,
	})
	c.Writer.WriteString("data: " + string(refsData) + "\n\n")
	flusher.Flush()

	// Phase 3: Stream LLM tokens
	llmStart := time.Now()
	tokenCh, errCh := h.engine.CallLLMStream(ctx, prompt)

	// If LLM streaming fails immediately, fall back to simulated chunking
	streamOK := true
	select {
	case err := <-errCh:
		if err != nil {
			log.Printf("[rag] LLM stream failed: %v, falling back to non-streaming", err)
			streamOK = false
		}
	default:
		// No immediate error, proceed with streaming
	}

	if streamOK {
		// Real LLM streaming: read tokens from channel
		for tok := range tokenCh {
			tokenData, _ := json.Marshal(StreamEvent{
				Type:  "token",
				Token: tok,
			})
			c.Writer.WriteString("data: " + string(tokenData) + "\n\n")
			flusher.Flush()
		}
		// Check for deferred error
		if err := <-errCh; err != nil {
			log.Printf("[rag] LLM stream error: %v", err)
			errData, _ := json.Marshal(StreamEvent{
				Type:  "error",
				Error: err.Error(),
			})
			c.Writer.WriteString("data: " + string(errData) + "\n\n")
			flusher.Flush()
		}
	} else {
		// Fallback: do a non-streaming LLM call and chunk the output
		resp, err := h.engine.Query(ctx, req)
		if err != nil {
			errData, _ := json.Marshal(StreamEvent{
				Type:  "error",
				Error: err.Error(),
			})
			c.Writer.WriteString("data: " + string(errData) + "\n\n")
			flusher.Flush()
		} else {
			// Chunk the pre-generated answer for visual streaming
			answer := resp.Answer
			chunkSize := h.engine.cfg.StreamChunk
			if chunkSize <= 0 {
				chunkSize = 1
			}
			runes := []rune(answer)
			for i := 0; i < len(runes); i += chunkSize {
				end := i + chunkSize
				if end > len(runes) {
					end = len(runes)
				}
				tokenData, _ := json.Marshal(StreamEvent{
					Type:  "token",
					Token: string(runes[i:end]),
				})
				c.Writer.WriteString("data: " + string(tokenData) + "\n\n")
				flusher.Flush()

				select {
				case <-ctx.Done():
					return
				case <-time.After(20 * time.Millisecond):
				}
			}
		}
	}

	llmTimeMs := time.Since(llmStart).Milliseconds()
	totalTimeMs := time.Since(totalStart).Milliseconds()

	// Phase 4: Send completion event
	doneData, _ := json.Marshal(StreamEvent{
		Type: "done",
		Metrics: &StreamDone{
			SearchTimeMs: searchTimeMs,
			LLMTimeMs:    llmTimeMs,
			TotalTimeMs:  totalTimeMs,
		},
	})
	c.Writer.WriteString("data: " + string(doneData) + "\n\n")
	flusher.Flush()

	log.Printf("[rag] stream query completed: search=%dms llm=%dms total=%dms", searchTimeMs, llmTimeMs, totalTimeMs)
}

// ClearCache handles DELETE /api/v1/rag/cache — clears the response cache.
func (h *Handler) ClearCache(c *gin.Context) {
	h.engine.ClearCache()
	c.JSON(http.StatusOK, gin.H{"status": "cache cleared"})
}
