package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/nodes"
)

// NodesHandler proxies Gateway API for node management (§5.4).
type NodesHandler struct {
	client *nodes.GatewayClient
}

// NewNodesHandler creates a nodes API handler.
func NewNodesHandler(client *nodes.GatewayClient) *NodesHandler {
	return &NodesHandler{client: client}
}

// SetupRoutes registers the /nodes proxy endpoints.
func (h *NodesHandler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/nodes")
	{
		g.GET("", h.list)
		g.GET("/:id/status", h.status)
	}
}

func (h *NodesHandler) list(c *gin.Context) {
	nodeList, err := h.client.ListNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nodes": nodeList, "total": len(nodeList)})
}

func (h *NodesHandler) status(c *gin.Context) {
	node, err := h.client.GetNode(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, node)
}
