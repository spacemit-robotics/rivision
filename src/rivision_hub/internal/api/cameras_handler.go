package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-hub/internal/cameras"
	"github.com/rivision/rivision-hub/internal/scheduler"
)

// CamerasHandler handles camera CRUD and stream scheduling (§5.7).
type CamerasHandler struct {
	store     *cameras.Store
	scheduler *scheduler.Scheduler
}

// NewCamerasHandler creates a camera API handler.
func NewCamerasHandler(store *cameras.Store, sched *scheduler.Scheduler) *CamerasHandler {
	return &CamerasHandler{store: store, scheduler: sched}
}

// SetupRoutes registers camera endpoints on the v1 group.
func (h *CamerasHandler) SetupRoutes(r *gin.RouterGroup) {
	g := r.Group("/cameras")
	{
		g.GET("", h.list)
		g.POST("", h.create)
		g.GET("/:id", h.get)
		g.PUT("/:id", h.update)
		g.DELETE("/:id", h.delete)
		g.GET("/:id/stream", h.streamURL)
	}
}

func (h *CamerasHandler) list(c *gin.Context) {
	filter := cameras.CameraFilter{
		NodeID: c.Query("node_id"),
	}
	cams := h.store.ListCameras(filter)
	c.JSON(http.StatusOK, gin.H{"cameras": cams, "total": len(cams)})
}

func (h *CamerasHandler) create(c *gin.Context) {
	var req struct {
		ID       string `json:"id"`
		Name     string `json:"name" binding:"required"`
		URL      string `json:"url" binding:"required"`
		Protocol string `json:"protocol"`
		GroupID  string `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cam := cameras.Camera{
		ID:       req.ID,
		Name:     req.Name,
		URL:      req.URL,
		Protocol: req.Protocol,
		GroupID:  req.GroupID,
		Enabled:  true,
	}

	if err := h.store.CreateCamera(&cam); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	// Trigger scheduler to assign camera to a Worker
	if h.scheduler != nil {
		assignment, err := h.scheduler.Assign(c.Request.Context(), cam.ID, cam.URL)
		if err != nil {
			c.JSON(http.StatusCreated, gin.H{"camera": cam, "assignment_error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"camera": cam, "assigned_to": assignment.WorkerID})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"camera": cam})
}

func (h *CamerasHandler) get(c *gin.Context) {
	cam, ok := h.store.GetCamera(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	c.JSON(http.StatusOK, cam)
}

func (h *CamerasHandler) update(c *gin.Context) {
	id := c.Param("id")
	existing, ok := h.store.GetCamera(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Enabled  *bool  `json:"enabled"`
		GroupID  string `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.GroupID != "" {
		existing.GroupID = req.GroupID
	}
	if err := h.store.UpdateCamera(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated", "id": id})
}

func (h *CamerasHandler) delete(c *gin.Context) {
	id := c.Param("id")

	// Unassign from scheduler
	if h.scheduler != nil {
		h.scheduler.Unassign(c.Request.Context(), id)
	}

	if err := h.store.DeleteCamera(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

func (h *CamerasHandler) streamURL(c *gin.Context) {
	cam, ok := h.store.GetCamera(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}

	// Return the Worker's go2rtc direct-connect URLs
	c.JSON(http.StatusOK, gin.H{
		"camera_id": cam.ID,
		"node_id":   cam.NodeID,
		"streams": gin.H{
			"rtsp_source": cam.URL,
		},
	})
}
