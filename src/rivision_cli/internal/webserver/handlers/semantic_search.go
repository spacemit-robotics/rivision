package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/semantic"
)

type SemanticSearchHandler struct {
	store *semantic.Store
}

func NewSemanticSearchHandler(store *semantic.Store) *SemanticSearchHandler {
	return &SemanticSearchHandler{store: store}
}

// GET /api/vlm/search/stats
func (h *SemanticSearchHandler) Stats(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "semantic search disabled"})
		return
	}
	count, err := h.store.Stats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	status := semantic.ProviderStatusSnapshot()
	c.JSON(http.StatusOK, gin.H{"success": true, "total_indexed": count, "provider": status})
}

// GET /api/vlm/search/provider/status
func (h *SemanticSearchHandler) ProviderStatus(c *gin.Context) {
	status := semantic.ProviderStatusSnapshot()
	c.JSON(http.StatusOK, gin.H{"success": true, "provider": status})
}

// POST /api/vlm/search
func (h *SemanticSearchHandler) Search(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "semantic search disabled"})
		return
	}

	var req struct {
		Query    string `json:"query"`
		CameraID string `json:"camera_id"`
		Limit    int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request"})
		return
	}

	q := strings.TrimSpace(req.Query)
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "query is required"})
		return
	}

	items, err := h.store.Search(q, req.CameraID, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"items":   items,
		"total":   len(items),
		"provider": semantic.ProviderStatusSnapshot(),
	})
}

// POST /api/vlm/search/image
func (h *SemanticSearchHandler) SearchByImage(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "semantic search disabled"})
		return
	}
	var req struct {
		Caption        string    `json:"caption"`
		Tags           []string  `json:"tags"`
		CameraID       string    `json:"camera_id"`
		Limit          int       `json:"limit"`
		TaskID         string    `json:"task_id"`
		ImageEmbedding []float64 `json:"image_embedding"`
		ImageBase64    string    `json:"image_base64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request"})
		return
	}
	if req.TaskID != "" && len(req.ImageEmbedding) > 0 {
		_ = h.store.UpsertImageEmbedding(req.TaskID, req.ImageEmbedding)
	}
	if req.TaskID != "" && req.ImageBase64 != "" {
		if emb, ok := semantic.RuntimeEmbedImage(req.ImageBase64); ok {
			_ = h.store.UpsertImageEmbedding(req.TaskID, emb)
		}
	}
	query := semantic.BuildImageCaptionQuery(req.Caption, req.Tags)
	items, err := h.store.Search(query, req.CameraID, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"mode":    "image_caption_text",
		"query":   query,
		"items":   items,
		"total":   len(items),
		"provider": semantic.ProviderStatusSnapshot(),
	})
}

// POST /api/vlm/search/hybrid
func (h *SemanticSearchHandler) HybridSearch(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "semantic search disabled"})
		return
	}
	var req struct {
		semantic.HybridSearchInput
		ImageBase64 string `json:"image_base64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request"})
		return
	}
	in := req.HybridSearchInput
	if len(in.ImageEmbedding) == 0 && req.ImageBase64 != "" {
		if emb, ok := semantic.RuntimeEmbedImage(req.ImageBase64); ok {
			in.ImageEmbedding = emb
		}
	}
	items, err := h.store.HybridSearch(in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"mode":    "hybrid_fallback_text",
		"items":   items,
		"total":   len(items),
		"provider": semantic.ProviderStatusSnapshot(),
	})
}

// POST /api/vlm/search/reindex
// 从网关历史记录导入并更新本地 SQLite 语义索引
func (h *SemanticSearchHandler) ReindexFromHistory(c *gin.Context) {
	if h == nil || h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "semantic search disabled"})
		return
	}

	var req struct {
		Items []semantic.HistoryItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid request"})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true, "updated": 0})
		return
	}

	if err := h.store.UpsertHistoryItems(req.Items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	count, _ := h.store.Stats()
	c.JSON(http.StatusOK, gin.H{"success": true, "updated": len(req.Items), "total_indexed": count})
}

// POST /api/vlm/search/backfill
func (h *SemanticSearchHandler) Backfill(c *gin.Context) {
var req struct {
Limit int `json:"limit"`
}
if err := c.ShouldBindJSON(&req); err != nil || req.Limit <= 0 {
req.Limit = 100
}
count, err := h.store.BackfillImageEmbeddings(req.Limit)
if err != nil {
c.JSON(500, gin.H{"success": false, "error": err.Error()})
return
}
c.JSON(200, gin.H{"success": true, "processed": count})
}

