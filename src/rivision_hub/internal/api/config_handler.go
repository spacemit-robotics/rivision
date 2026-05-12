package api

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ConfigHandler serves algorithm config endpoints (§5.10).
type ConfigHandler struct {
	// In production this would be backed by SQLite algorithm_config table.
	// For now we hold the latest config in memory.
	latestConfig json.RawMessage
}

// NewConfigHandler creates a config API handler.
func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// SetupRoutes registers config endpoints.
func (h *ConfigHandler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/config")
	{
		g.GET("/algorithm", h.getAlgorithm)
		g.POST("/algorithm", h.setAlgorithm)
	}
}

func (h *ConfigHandler) getAlgorithm(c *gin.Context) {
	if h.latestConfig == nil {
		c.JSON(http.StatusOK, gin.H{"config": nil, "version": 0})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": json.RawMessage(h.latestConfig), "version": 1})
}

func (h *ConfigHandler) setAlgorithm(c *gin.Context) {
	var cfg json.RawMessage
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.latestConfig = cfg

	// TODO: Push to Workers via POST worker:9181/api/v1/config/algorithm
	c.JSON(http.StatusOK, gin.H{"status": "applied", "push_to_workers": "pending"})
}
