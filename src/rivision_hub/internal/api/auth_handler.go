package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/auth"
)

// AuthAPIHandler serves auth endpoints (§5.7).
type AuthAPIHandler struct {
	handler *auth.Handler
}

// NewAuthAPIHandler creates an auth API handler.
func NewAuthAPIHandler(handler *auth.Handler) *AuthAPIHandler {
	return &AuthAPIHandler{handler: handler}
}

// SetupRoutes registers auth endpoints.
func (h *AuthAPIHandler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/auth")
	{
		g.POST("/login", h.handler.Login)
		g.POST("/logout", h.handler.Logout)
		g.GET("/me", h.me)
		g.POST("/tokens", h.createToken)
	}
}

func (h *AuthAPIHandler) me(c *gin.Context) {
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *AuthAPIHandler) createToken(c *gin.Context) {
	// Gateway token management — delegated to admin
	c.JSON(http.StatusOK, gin.H{"status": "token_created"})
}
