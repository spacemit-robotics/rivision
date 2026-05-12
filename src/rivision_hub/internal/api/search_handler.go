// Package api 实现API处理器
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/search"
)

// SearchHandler 搜索API处理器
type SearchHandler struct {
	federated      *search.FederatedSearch
	getNodes       func() []search.NodeInfo
	intentParser   *search.IntentParser          // G8
	retroSearcher  *search.RetrospectiveSearcher // G4
	timelineSearch *search.TimelineSearcher      // G7
	reranker       *search.VLMReranker           // G5
	getCameras     func() []search.CameraInfo    // G8: for intent camera matching
}

// NewSearchHandler 创建处理器
func NewSearchHandler(federated *search.FederatedSearch, getNodes func() []search.NodeInfo) *SearchHandler {
	return &SearchHandler{
		federated:    federated,
		getNodes:     getNodes,
		intentParser: search.NewIntentParser("", false),
	}
}

// SetIntentParser sets the intent parser for natural language query parsing (G8).
func (h *SearchHandler) SetIntentParser(ip *search.IntentParser, getCameras func() []search.CameraInfo) {
	h.intentParser = ip
	h.getCameras = getCameras
}

// SetRetrospectiveSearcher sets the retrospective searcher (G4).
func (h *SearchHandler) SetRetrospectiveSearcher(rs *search.RetrospectiveSearcher) {
	h.retroSearcher = rs
}

// SetTimelineSearcher sets the timeline searcher (G7).
func (h *SearchHandler) SetTimelineSearcher(ts *search.TimelineSearcher) {
	h.timelineSearch = ts
}

// SetReranker sets the VLM reranker (G5).
func (h *SearchHandler) SetReranker(r *search.VLMReranker) {
	h.reranker = r
}

// SetupRoutes 设置路由
func (h *SearchHandler) SetupRoutes(r *gin.RouterGroup) {
	s := r.Group("/search")
	{
		s.POST("/federated", h.FederatedSearch)
		s.POST("/federated/image", h.FederatedImageSearch)
		s.POST("/smart", h.SmartSearch)            // G8: intent-parsed search
		s.POST("/retrospective", h.Retrospective)  // G4: VLM-verified search
		s.POST("/timeline", h.Timeline)             // G7: person timeline
		s.GET("/stats", h.Stats)
	}
}

// FederatedSearch 联邦搜索
func (h *SearchHandler) FederatedSearch(c *gin.Context) {
	var req search.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	nodes := h.getNodes()
	resp, err := h.federated.Search(ctx, nodes, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// FederatedImageSearch 联邦以图搜图
func (h *SearchHandler) FederatedImageSearch(c *gin.Context) {
	var req struct {
		ImageData string `json:"image_data" binding:"required"`
		Limit     int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 50
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	nodes := h.getNodes()
	resp, err := h.federated.SearchByImage(ctx, nodes, req.ImageData, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SmartSearch performs intent-parsed search: extracts time/camera/behavior from natural language (G8).
func (h *SearchHandler) SmartSearch(c *gin.Context) {
	var req struct {
		Query string `json:"query" binding:"required"`
		Limit int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	// Parse intent
	var cameras []search.CameraInfo
	if h.getCameras != nil {
		cameras = h.getCameras()
	}
	intent := h.intentParser.Parse(req.Query, cameras, time.Now())

	// Build federated search request from parsed intent
	searchReq := search.SearchRequest{
		Query:     intent.SearchText,
		CameraIDs: intent.CameraIDs,
		TimeStart: intent.TimeStart,
		TimeEnd:   intent.TimeEnd,
		Limit:     req.Limit,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	nodes := h.getNodes()
	resp, err := h.federated.Search(ctx, nodes, searchReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Optionally apply VLM re-ranking (G5)
	var reranked []search.RerankResult
	if h.reranker != nil && len(resp.Results) > 0 {
		reranked = h.reranker.Rerank(ctx, intent.SearchText, resp.Results, nodes)
	}

	c.JSON(http.StatusOK, gin.H{
		"intent":   intent,
		"results":  resp.Results,
		"reranked": reranked,
		"total":    resp.TotalResults,
		"time_ms":  resp.SearchTimeMs,
	})
}

// Retrospective performs VLM-verified retrospective search (G4).
func (h *SearchHandler) Retrospective(c *gin.Context) {
	if h.retroSearcher == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "retrospective search not configured"})
		return
	}

	var req search.RetrospectiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()

	nodes := h.getNodes()
	resp, err := h.retroSearcher.Search(ctx, nodes, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Timeline builds a person's chronological appearance timeline (G7).
func (h *SearchHandler) Timeline(c *gin.Context) {
	if h.timelineSearch == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "timeline search not configured"})
		return
	}

	var req search.TimelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()

	nodes := h.getNodes()
	resp, err := h.timelineSearch.BuildTimeline(ctx, nodes, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Stats 搜索统计
func (h *SearchHandler) Stats(c *gin.Context) {
	stats := h.federated.GetStats()
	c.JSON(http.StatusOK, stats)
}
