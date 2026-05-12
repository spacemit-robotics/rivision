// Package api 实现API处理器
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/knowledge"
	"github.com/rivision/rivision-hub/pkg/models"
)

// KnowledgeHandler 知识库API处理器
type KnowledgeHandler struct {
	store  *knowledge.Store
	syncer *knowledge.Syncer
}

// NewKnowledgeHandler 创建处理器
func NewKnowledgeHandler(store *knowledge.Store, syncer *knowledge.Syncer) *KnowledgeHandler {
	return &KnowledgeHandler{
		store:  store,
		syncer: syncer,
	}
}

// SetupRoutes 设置路由
func (h *KnowledgeHandler) SetupRoutes(r *gin.RouterGroup) {
	kb := r.Group("/knowledge")
	{
		kb.GET("/rules", h.ListRules)
		kb.POST("/rules", h.CreateRule)
		kb.GET("/rules/:id", h.GetRule)
		kb.PUT("/rules/:id", h.UpdateRule)
		kb.DELETE("/rules/:id", h.DeleteRule)
		kb.GET("/targets", h.ListTargets)
		kb.POST("/targets", h.CreateTarget)
		kb.GET("/targets/:id", h.GetTarget)
		kb.DELETE("/targets/:id", h.DeleteTarget)
		kb.GET("/prompts", h.ListPrompts)
		kb.GET("/sync", h.PullSync)
		kb.POST("/sync", h.TriggerSync)
		kb.GET("/sync/status", h.SyncStatus)
		kb.GET("/version", h.GetVersion)
	}
}

// ListRules 获取规则列表
func (h *KnowledgeHandler) ListRules(c *gin.Context) {
	rules, err := h.store.GetRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rules":   rules,
		"version": h.store.GetVersion(),
	})
}

// CreateRule 创建规则
func (h *KnowledgeHandler) CreateRule(c *gin.Context) {
	var req models.CreateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.store.CreateRule(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, rule)
}

// GetRule 获取规则详情
func (h *KnowledgeHandler) GetRule(c *gin.Context) {
	id := c.Param("id")
	rule, err := h.store.GetRule(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "规则不存在"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

// UpdateRule 更新规则
func (h *KnowledgeHandler) UpdateRule(c *gin.Context) {
	id := c.Param("id")
	var req models.UpdateRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule, err := h.store.UpdateRule(id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

// DeleteRule 删除规则
func (h *KnowledgeHandler) DeleteRule(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListPrompts 获取提示词模板列表
func (h *KnowledgeHandler) ListPrompts(c *gin.Context) {
	prompts, err := h.store.GetPrompts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prompts": prompts})
}

// PullSync 供 Worker 拉取最新知识库 (GET /knowledge/sync?node_id=X&version=Y)
func (h *KnowledgeHandler) PullSync(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "知识库不可用"})
		return
	}

	// 检查 Worker 当前版本，如果已是最新则返回 304
	workerVersion, _ := strconv.Atoi(c.Query("version"))
	currentVersion := h.store.GetVersion()
	if workerVersion >= currentVersion {
		c.Status(http.StatusNotModified)
		return
	}

	// 返回完整知识库数据
	rules, err := h.store.GetRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"version": currentVersion,
		"rules":   rules,
	})
}

// TriggerSync 触发同步
func (h *KnowledgeHandler) TriggerSync(c *gin.Context) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	c.ShouldBindJSON(&req)

	// TODO: 实现立即同步到指定节点
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "同步已触发",
		"version": h.store.GetVersion(),
	})
}

// SyncStatus 同步状态
func (h *KnowledgeHandler) SyncStatus(c *gin.Context) {
	versions := h.syncer.GetNodeVersions()
	currentVersion := h.store.GetVersion()

	c.JSON(http.StatusOK, gin.H{
		"current_version": currentVersion,
		"node_versions":   versions,
	})
}

// ListTargets 获取特征目标列表
func (h *KnowledgeHandler) ListTargets(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusOK, gin.H{"targets": []interface{}{}})
		return
	}
	targets, err := h.store.GetTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if targets == nil {
		targets = []models.Target{}
	}
	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

// CreateTarget 创建特征目标
func (h *KnowledgeHandler) CreateTarget(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "知识库不可用"})
		return
	}
	var req models.CreateTargetRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	target, err := h.store.CreateTarget(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, target)
}

// GetTarget 获取特征目标详情
func (h *KnowledgeHandler) GetTarget(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标不存在"})
		return
	}
	id := c.Param("id")
	target, err := h.store.GetTarget(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "目标不存在"})
		return
	}
	c.JSON(http.StatusOK, target)
}

// DeleteTarget 删除特征目标
func (h *KnowledgeHandler) DeleteTarget(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "知识库不可用"})
		return
	}
	id := c.Param("id")
	if err := h.store.DeleteTarget(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetVersion 获取当前版本
func (h *KnowledgeHandler) GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": h.store.GetVersion(),
	})
}
