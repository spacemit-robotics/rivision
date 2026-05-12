// Package cameras 提供摄像头 API 处理器
package cameras

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rivision/rivision-hub/internal/owl"
)

// Handler 摄像头处理器
type Handler struct {
	store      *Store
	nodeGetter func() []NodeInfo
	owlClient  *owl.Client      // default/fallback OWL client (backward compat)
	owlCfg     owl.Config        // shared OWL auth config for per-node clients
	owlClients sync.Map          // nodeID → *owl.Client (per-Worker OWL clients)
	httpClient *http.Client     // shared HTTP client for Worker notifications
}

// NodeInfo 节点信息
type NodeInfo struct {
	ID          string
	Host        string
	AgentPort   int
	OwlURL      string // OWL API URL on this Worker (e.g. http://192.168.1.10:15123)
	ZLMHost     string // Worker ZLM IP for receiving RTP (SIP-media split)
	ZLMHTTPPort int    // Worker ZLM HTTP API port (default 80)
	ZLMRTSPPort int    // Worker ZLM RTSP port (default 554)
	Healthy     bool
}

// NewHandler 创建处理器
func NewHandler(store *Store, nodeGetter func() []NodeInfo) *Handler {
	return &Handler{
		store:      store,
		nodeGetter: nodeGetter,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetOwlClient sets the default/fallback OWL client for backward compatibility.
func (h *Handler) SetOwlClient(c *owl.Client) {
	h.owlClient = c
}

// SetOwlConfig sets the shared OWL auth config used to create per-node clients.
func (h *Handler) SetOwlConfig(cfg owl.Config) {
	h.owlCfg = cfg
}

// getNodeOwlClient returns the OWL client for a specific Worker node.
// If the node has an OwlURL, creates/caches a per-node client with shared auth.
// Falls back to the default owlClient if no node-specific OWL is available.
func (h *Handler) getNodeOwlClient(nodeID string) *owl.Client {
	// Check cache
	if v, ok := h.owlClients.Load(nodeID); ok {
		return v.(*owl.Client)
	}

	// Find node's OwlURL
	if h.nodeGetter != nil {
		for _, node := range h.nodeGetter() {
			if node.ID == nodeID && node.OwlURL != "" {
				cfg := h.owlCfg
				cfg.URL = node.OwlURL
				cfg.Enabled = true
				c := owl.NewClient(cfg)
				h.owlClients.Store(nodeID, c)
				log.Printf("[CameraHandler] created OWL client for node %s: %s", nodeID, node.OwlURL)
				return c
			}
		}
	}

	// Fallback to default
	return h.owlClient
}

// SetupRoutes 设置路由
func (h *Handler) SetupRoutes(r *gin.RouterGroup) {
	cam := r.Group("/cameras")
	{
		cam.GET("", h.List)
		cam.POST("", h.Create)
		cam.GET("/:id", h.Get)
		cam.PUT("/:id", h.Update)
		cam.DELETE("/:id", h.Delete)
		cam.POST("/:id/assign", h.Assign)
		cam.PATCH("/:id/config", h.PatchConfig)
		cam.GET("/stats", h.Stats)
	}

	grp := r.Group("/camera-groups")
	{
		grp.GET("", h.ListGroups)
		grp.POST("", h.CreateGroup)
		grp.DELETE("/:id", h.DeleteGroup)
	}

	// OWL integration: GB28181/ONVIF device management
	// All endpoints accept ?node_id= to route to a specific Worker's OWL.
	// Without node_id, they use Hub's default OWL (discovery/metadata).
	owlGrp := r.Group("/owl")
	{
		// --- Discovery endpoints (typically use Hub OWL) ---
		owlGrp.GET("/server-info", h.OwlServerInfo)
		owlGrp.GET("/capabilities", h.OwlCapabilities)
		owlGrp.GET("/devices", h.OwlListDevices)
		owlGrp.GET("/channels", h.OwlListChannels)
		owlGrp.POST("/devices", h.OwlAddDevice)
		owlGrp.DELETE("/devices/:id", h.OwlDeleteDevice)
		owlGrp.POST("/devices/:id/catalog", h.OwlRefreshCatalog)
		owlGrp.POST("/import-batch", h.OwlImportBatch)
		owlGrp.POST("/channels/:id/import", h.OwlImportChannel)
		owlGrp.GET("/discover", h.OwlDiscover)

		// --- Stream control endpoints (should use Worker OWL via ?node_id=) ---
		// These trigger video operations — Worker's OWL+ZLM handles the actual stream.
		owlGrp.POST("/channels/:id/play", h.OwlPlayChannel)
		owlGrp.POST("/channels/:id/stop", h.OwlStopChannel)
		owlGrp.POST("/channels/:id/snapshot", h.OwlSnapshot)
		owlGrp.POST("/channels/:id/ptz", h.OwlPTZControl)
	}
}

// List 列出摄像头
func (h *Handler) List(c *gin.Context) {
	filter := CameraFilter{
		GroupID:     c.Query("group_id"),
		NodeID:      c.Query("node_id"),
		Status:      c.Query("status"),
		EnabledOnly: c.Query("enabled") == "true",
	}

	cameras := h.store.ListCameras(filter)

	c.JSON(http.StatusOK, gin.H{
		"cameras": cameras,
		"total":   len(cameras),
	})
}

// CreateRequest 创建请求
type CreateRequest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name" binding:"required"`
	URL         string                 `json:"url" binding:"required"`
	Protocol    string                 `json:"protocol"`
	GroupID     string                 `json:"group_id"`
	NodeID      string                 `json:"node_id"`
	Location    string                 `json:"location"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
	Enabled     *bool                  `json:"enabled"`
}

// Create 创建摄像头
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ID == "" {
		req.ID = "cam_" + uuid.New().String()[:8]
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 自动分配节点：如果未指定 node_id，选择第一个健康的节点
	nodeID := req.NodeID
	if nodeID == "" && h.nodeGetter != nil {
		nodes := h.nodeGetter()
		for _, node := range nodes {
			if node.Healthy {
				nodeID = node.ID
				log.Printf("[CameraHandler] 自动分配摄像头 %s 到节点 %s", req.ID, nodeID)
				break
			}
		}
	}

	// Hub stores metadata only — Worker resolves RTSP via its own OWL.
	// For GB28181/ONVIF: URL field is the OWL channel ID (metadata).
	// For RTSP: URL field is the direct camera RTSP address.
	protocol := strings.ToLower(req.Protocol)
	if req.Config == nil {
		req.Config = make(map[string]interface{})
	}
	if protocol == "gb28181" {
		req.Config["owl_channel_id"] = req.URL
		req.Config["stream_mode"] = "hub_sip_remote" // Hub OWL SIP signaling, Worker ZLM receives RTP
	} else if protocol == "onvif" {
		req.Config["owl_channel_id"] = req.URL
		req.Config["stream_mode"] = "direct" // Worker pulls camera RTSP directly
	} else {
		req.Config["stream_mode"] = "direct" // Worker pulls URL directly
	}

	cam := &Camera{
		ID:          req.ID,
		Name:        req.Name,
		URL:         req.URL,
		Protocol:    req.Protocol,
		GroupID:     req.GroupID,
		NodeID:      nodeID,
		Location:    req.Location,
		Description: req.Description,
		Config:      req.Config,
		Enabled:     enabled,
	}

	if err := h.store.CreateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Activate stream (GB28181: Hub OWL SIP signaling → Worker ZLM)
	if cam.NodeID != "" {
		if rtspURL, err := h.ActivateStream(c.Request.Context(), cam); err != nil {
			log.Printf("[CameraHandler] activateStream failed: %v (Worker will retry)", err)
		} else if rtspURL != "" && rtspURL != cam.URL {
			cam.URL = rtspURL
			h.store.UpdateCamera(cam)
		}
		go h.notifyNodeAddStream(cam)
	}

	c.JSON(http.StatusCreated, cam)
}

// Get 获取摄像头
func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	cam, ok := h.store.GetCamera(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	c.JSON(http.StatusOK, cam)
}

// Update 更新摄像头
func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	cam, ok := h.store.GetCamera(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	oldNodeID := cam.NodeID

	cam.Name = req.Name
	cam.URL = req.URL
	if req.Protocol != "" {
		cam.Protocol = req.Protocol
	}
	cam.GroupID = req.GroupID
	cam.NodeID = req.NodeID
	cam.Location = req.Location
	cam.Description = req.Description
	cam.Config = req.Config
	if req.Enabled != nil {
		cam.Enabled = *req.Enabled
	}

	if err := h.store.UpdateCamera(cam); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 如果节点变更，处理流迁移
	if oldNodeID != cam.NodeID {
		if oldNodeID != "" {
			go h.notifyNodeRemoveStream(oldNodeID, cam.ID)
		}
		// For hub_sip_remote: stop old SIP session (sync), then re-activate to new Worker's ZLM
		h.deactivateStreamSync(cam)
		if cam.NodeID != "" {
			if rtspURL, err := h.ActivateStream(c.Request.Context(), cam); err != nil {
				log.Printf("[CameraHandler] Update re-activateStream failed for %s: %v", cam.ID, err)
			} else if rtspURL != "" && rtspURL != cam.URL {
				cam.URL = rtspURL
				h.store.UpdateCamera(cam)
			}
			go h.notifyNodeAddStream(cam)
		}
	}

	c.JSON(http.StatusOK, cam)
}

// Delete 删除摄像头
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	cam, ok := h.store.GetCamera(id)
	if ok {
		// Notify Worker to remove stream
		if cam.NodeID != "" {
			go h.notifyNodeRemoveStream(cam.NodeID, id)
		}
		// For hub_sip_remote: Hub OWL sends SIP BYE to stop camera RTP
		// For direct/worker_owl: Worker handles its own cleanup
		h.deactivateStream(cam)
	}

	if err := h.store.DeleteCamera(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AssignRequest 分配请求
type AssignRequest struct {
	NodeID string `json:"node_id" binding:"required"`
}

// Assign 分配到节点
func (h *Handler) Assign(c *gin.Context) {
	id := c.Param("id")

	var req AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cam, ok := h.store.GetCamera(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	oldNodeID := cam.NodeID

	if err := h.store.AssignToNode(id, req.NodeID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify old Worker to stop stream
	if oldNodeID != "" && oldNodeID != req.NodeID {
		go h.notifyNodeRemoveStream(oldNodeID, id)
	}

	// For hub_sip_remote: stop old SIP session (sync), then re-activate to new Worker's ZLM
	h.deactivateStreamSync(cam)

	// Refresh cam from store (AssignToNode updated it)
	cam, _ = h.store.GetCamera(id)
	if rtspURL, err := h.ActivateStream(c.Request.Context(), cam); err != nil {
		log.Printf("[CameraHandler] re-activateStream failed for %s → %s: %v", id, req.NodeID, err)
	} else if rtspURL != "" && rtspURL != cam.URL {
		cam.URL = rtspURL
		h.store.UpdateCamera(cam)
	}
	go h.notifyNodeAddStream(cam)

	c.JSON(http.StatusOK, gin.H{"success": true, "node_id": req.NodeID})
}

// ReassignCamera implements failover.CameraReassigner interface.
// Performs the complete camera reassignment lifecycle:
//  1. Notify old Worker to remove stream (best-effort, Worker may be dead)
//  2. Deactivate SIP session for hub_sip_remote (SIP BYE + cache invalidation)
//  3. Store-level node reassignment
//  4. Activate stream on new Worker (hub_sip_remote: PlayRemote to new ZLM)
//  5. Notify new Worker to add stream
func (h *Handler) ReassignCamera(ctx context.Context, cameraID, newNodeID string) error {
	cam, ok := h.store.GetCamera(cameraID)
	if !ok {
		return fmt.Errorf("camera %s not found", cameraID)
	}

	oldNodeID := cam.NodeID

	// Step 1: Notify old Worker to remove stream (best-effort, may timeout if Worker is dead)
	if oldNodeID != "" && oldNodeID != newNodeID {
		done := make(chan struct{})
		go func() {
			defer close(done)
			h.notifyNodeRemoveStream(oldNodeID, cameraID)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Printf("[CameraHandler] ReassignCamera: notifyNodeRemoveStream timeout for %s (old node %s may be dead)", cameraID, oldNodeID)
		}
	}

	// Step 2: Deactivate old SIP session (hub_sip_remote only: SIP BYE + cache invalidation)
	h.deactivateStreamSync(cam)

	// Step 3: Store-level reassignment
	if err := h.store.AssignToNode(cameraID, newNodeID); err != nil {
		return fmt.Errorf("store AssignToNode: %w", err)
	}

	// Step 4: Activate stream on new Worker
	cam, _ = h.store.GetCamera(cameraID)
	if rtspURL, err := h.ActivateStream(ctx, cam); err != nil {
		log.Printf("[CameraHandler] ReassignCamera: activateStream failed for %s → %s: %v", cameraID, newNodeID, err)
		// Don't return error — store assignment succeeded, Worker will retry on next heartbeat
	} else if rtspURL != "" && rtspURL != cam.URL {
		cam.URL = rtspURL
		h.store.UpdateCamera(cam)
	}

	// Step 5: Notify new Worker to add stream
	go h.notifyNodeAddStream(cam)

	log.Printf("[CameraHandler] ReassignCamera: %s migrated %s → %s", cameraID, oldNodeID, newNodeID)
	return nil
}

// Stats 统计
func (h *Handler) Stats(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.GetStats())
}

// ListGroups 列出分组
func (h *Handler) ListGroups(c *gin.Context) {
	groups := h.store.ListGroups()
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// CreateGroupRequest 创建分组请求
type CreateGroupRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name" binding:"required"`
	ParentID    string `json:"parent_id"`
	Description string `json:"description"`
}

// CreateGroup 创建分组
func (h *Handler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ID == "" {
		req.ID = "grp_" + uuid.New().String()[:8]
	}

	g := &Group{
		ID:          req.ID,
		Name:        req.Name,
		ParentID:    req.ParentID,
		Description: req.Description,
	}

	if err := h.store.CreateGroup(g); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, g)
}

// DeleteGroup 删除分组
func (h *Handler) DeleteGroup(c *gin.Context) {
	id := c.Param("id")

	if err := h.store.DeleteGroup(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// notifyNodeAddStream 通知节点添加流
func (h *Handler) notifyNodeAddStream(cam *Camera) {
	if h.nodeGetter == nil {
		return
	}

	nodes := h.nodeGetter()
	for _, node := range nodes {
		if node.ID == cam.NodeID && node.Healthy {
			// Include protocol + owl_channel_id so Worker knows how to acquire stream:
			//   stream_mode=hub_sip_remote → cam.URL is localhost RTSP (RTP already flowing to Worker ZLM)
			//   stream_mode=direct         → Worker pulls cam.URL (RTSP) directly from camera
			//   stream_mode=worker_owl     → Worker uses its OWL to PlayChannel(owl_channel_id) → local RTSP
			body, _ := json.Marshal(map[string]interface{}{
				"camera_id": cam.ID,
				"url":       cam.URL,
				"name":      cam.Name,
				"protocol":  cam.Protocol,
				"config":    cam.Config,
			})
			url := fmt.Sprintf("http://%s:%d/api/v1/streams", node.Host, node.AgentPort)
			resp, err := h.httpClient.Post(url, "application/json", bytes.NewReader(body))
			if err != nil {
				log.Printf("[CameraHandler] 下发流到节点失败 camera=%s node=%s err=%v", cam.ID, node.ID, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Printf("[CameraHandler] 下发流到节点失败 camera=%s node=%s status=%d", cam.ID, node.ID, resp.StatusCode)
				return
			}
			h.store.UpdateStatus(cam.ID, "online")
			break
		}
	}
}

// notifyNodeRemoveStream 通知节点移除流
func (h *Handler) notifyNodeRemoveStream(nodeID, cameraID string) {
	if h.nodeGetter == nil {
		return
	}

	nodes := h.nodeGetter()
	for _, node := range nodes {
		if node.ID == nodeID && node.Healthy {
			url := fmt.Sprintf("http://%s:%d/api/v1/streams/%s", node.Host, node.AgentPort, cameraID)
			req, _ := http.NewRequest(http.MethodDelete, url, nil)
			resp, err := h.httpClient.Do(req)
			if err != nil {
				log.Printf("[CameraHandler] 从节点移除流失败 camera=%s node=%s err=%v", cameraID, nodeID, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				log.Printf("[CameraHandler] 从节点移除流失败 camera=%s node=%s status=%d", cameraID, nodeID, resp.StatusCode)
				return
			}
			h.store.UpdateStatus(cameraID, "offline")
			break
		}
	}
}

// getNode returns the NodeInfo for a given node ID.
func (h *Handler) getNode(nodeID string) *NodeInfo {
	if h.nodeGetter == nil {
		return nil
	}
	for _, n := range h.nodeGetter() {
		if n.ID == nodeID {
			return &n
		}
	}
	return nil
}

// ActivateStream sets up the video stream for a camera on its assigned Worker.
// For GB28181 (hub_sip_remote): Hub OWL sends SIP INVITE with Worker ZLM as media target.
// For direct/worker_owl: no Hub-side action needed — Worker handles everything.
// Returns the resolved stream URL for the Worker (localhost RTSP or direct camera URL).
// Exported so the registration handler can re-activate streams after Worker restart.
func (h *Handler) ActivateStream(ctx context.Context, cam *Camera) (string, error) {
	mode, _ := cam.Config["stream_mode"].(string)
	if mode != "hub_sip_remote" {
		return cam.URL, nil // direct or worker_owl: Worker handles stream setup
	}

	// GB28181: Hub OWL does SIP INVITE, directing media to Worker's ZLM
	chID, _ := cam.Config["owl_channel_id"].(string)
	if chID == "" {
		return "", fmt.Errorf("hub_sip_remote requires owl_channel_id")
	}

	node := h.getNode(cam.NodeID)
	if node == nil {
		return "", fmt.Errorf("node %s not found", cam.NodeID)
	}

	zlmHost := node.ZLMHost
	if zlmHost == "" {
		zlmHost = node.Host // fallback to agent host
	}
	zlmHTTP := node.ZLMHTTPPort
	if zlmHTTP == 0 {
		zlmHTTP = 80
	}

	// Call Hub OWL PlayRemote → Camera sends RTP to Worker ZLM directly
	owlC := h.owlClient
	if owlC == nil {
		return "", fmt.Errorf("Hub OWL not configured")
	}

	result, err := owlC.PlayRemote(ctx, chID, owl.PlayRemoteRequest{
		ZLMHost:     zlmHost,
		ZLMHTTPPort: zlmHTTP,
	})
	if err != nil {
		return "", fmt.Errorf("PlayRemote for %s → ZLM %s: %w", chID, zlmHost, err)
	}

	// Extract RTSP URL and normalize for Worker (localhost)
	rtspURL := owlC.ExtractBestURL(result)
	if rtspURL == "" {
		return "", fmt.Errorf("no usable stream URL from PlayRemote for %s", chID)
	}

	// Convert rtsp://worker-ip:554/... → rtsp://localhost:554/... for Worker
	zlmRTSP := node.ZLMRTSPPort
	if zlmRTSP == 0 {
		zlmRTSP = 554
	}
	rtspURL = normalizeToLocalhost(rtspURL, zlmHost, zlmRTSP)

	log.Printf("[CameraHandler] activateStream: %s → ZLM %s, Worker RTSP: %s", chID, zlmHost, rtspURL)
	return rtspURL, nil
}

// deactivateStreamSync stops the video stream synchronously (blocks until done).
// Use this when activateStream will be called immediately after (Assign/Update).
func (h *Handler) deactivateStreamSync(cam *Camera) {
	mode, _ := cam.Config["stream_mode"].(string)
	if mode != "hub_sip_remote" {
		return
	}
	chID, _ := cam.Config["owl_channel_id"].(string)
	if chID == "" || h.owlClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.owlClient.StopChannel(ctx, chID); err != nil {
		log.Printf("[CameraHandler] deactivateStreamSync StopChannel %s: %v", chID, err)
	}
	h.owlClient.InvalidateRTSPCache(chID)
	log.Printf("[CameraHandler] deactivateStreamSync: stopped %s via Hub OWL", chID)
}

// deactivateStream stops the video stream asynchronously (fire-and-forget).
// Use this for Delete where no subsequent activateStream is needed.
func (h *Handler) deactivateStream(cam *Camera) {
	mode, _ := cam.Config["stream_mode"].(string)
	if mode != "hub_sip_remote" {
		return
	}
	chID, _ := cam.Config["owl_channel_id"].(string)
	if chID == "" || h.owlClient == nil {
		return
	}
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.owlClient.StopChannel(bgCtx, chID); err != nil {
			log.Printf("[CameraHandler] deactivateStream StopChannel %s: %v", chID, err)
		}
		h.owlClient.InvalidateRTSPCache(chID)
		log.Printf("[CameraHandler] deactivateStream: stopped %s via Hub OWL", chID)
	}()
}

// normalizeToLocalhost converts rtsp://worker-ip:port/... → rtsp://localhost:port/...
// because Worker pulls from its own ZLM on localhost.
func normalizeToLocalhost(rawURL, workerHost string, rtspPort int) string {
	// Simple string replacement for the host portion
	// Full URL parsing would be more robust but this covers the common case
	for _, prefix := range []string{
		fmt.Sprintf("rtsp://%s:%d/", workerHost, rtspPort),
		fmt.Sprintf("rtsp://%s/", workerHost),
	} {
		if strings.HasPrefix(rawURL, prefix) {
			return fmt.Sprintf("rtsp://localhost:%d/", rtspPort) + rawURL[len(prefix):]
		}
	}
	return rawURL
}

// PatchConfig 更新摄像头配置（用于快速切换 AI 功能）
func (h *Handler) PatchConfig(c *gin.Context) {
	id := c.Param("id")

	cam, ok := h.store.GetCamera(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 合并配置
	if cam.Config == nil {
		cam.Config = make(map[string]interface{})
	}
	for k, v := range req {
		cam.Config[k] = v
	}

	// 保存配置
	if err := h.store.UpdateConfig(id, cam.Config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[CameraHandler] 更新配置 camera=%s config=%v", id, req)

	// 通知 Worker 启动/停止 YOLO pipeline
	if yoloVal, hasYolo := req["yolo_enabled"]; hasYolo && cam.NodeID != "" {
		yoloEnabled, _ := yoloVal.(bool)
		go h.notifyNodePipelineToggle(cam, yoloEnabled)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "updated",
		"id":      id,
		"config":  cam.Config,
	})
}

// notifyNodePipelineToggle tells the Worker to start/stop the YOLO pipeline for a camera.
func (h *Handler) notifyNodePipelineToggle(cam *Camera, yoloEnabled bool) {
	if h.nodeGetter == nil {
		return
	}
	nodes := h.nodeGetter()
	for _, node := range nodes {
		if node.ID == cam.NodeID && node.Healthy {
			body, _ := json.Marshal(map[string]interface{}{
				"yolo_enabled": yoloEnabled,
			})
			url := fmt.Sprintf("http://%s:%d/api/v1/streams/%s/pipeline", node.Host, node.AgentPort, cam.ID)
			req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
			if err != nil {
				log.Printf("[CameraHandler] pipeline toggle request build failed: %v", err)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := h.httpClient.Do(req)
			if err != nil {
				log.Printf("[CameraHandler] pipeline toggle failed camera=%s node=%s: %v", cam.ID, node.ID, err)
				return
			}
			resp.Body.Close()
			log.Printf("[CameraHandler] pipeline toggle camera=%s yolo=%v node=%s status=%d",
				cam.ID, yoloEnabled, node.ID, resp.StatusCode)
			return
		}
	}
}

// --- OWL integration endpoints (GB28181 / ONVIF) ---

// resolveOWL returns the OWL client for the request.
// If ?node_id is set, returns that Worker's OWL client; otherwise falls back to default.
func (h *Handler) resolveOWL(c *gin.Context) (*owl.Client, bool) {
	nodeID := c.Query("node_id")
	if nodeID != "" {
		client := h.getNodeOwlClient(nodeID)
		if client != nil {
			return client, true
		}
	}
	if h.owlClient != nil {
		return h.owlClient, true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "OWL integration not configured (set node_id or default owl)"})
	return nil, false
}

// OwlServerInfo returns OWL server status
func (h *Handler) OwlServerInfo(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	info, err := client.GetServerInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

// OwlCapabilities returns which OWL API features are available
func (h *Handler) OwlCapabilities(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	caps := client.ProbeCapabilities(c.Request.Context())
	c.JSON(http.StatusOK, caps)
}

// OwlListDevices proxies device list from OWL
func (h *Handler) OwlListDevices(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	devices, err := client.ListDevicesWithChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": devices, "total": len(devices)})
}

// OwlListChannels proxies channel list from OWL
func (h *Handler) OwlListChannels(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	channels, err := client.GetAllChannels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": channels, "total": len(channels)})
}

// OwlAddDevice registers a GB28181 or ONVIF device in OWL
func (h *Handler) OwlAddDevice(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	var req owl.AddDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dev, err := client.AddDevice(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, dev)
}

// OwlDeleteDevice removes a device from OWL
func (h *Handler) OwlDeleteDevice(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := client.DeleteDevice(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// OwlRefreshCatalog triggers a GB28181 catalog refresh for a device
func (h *Handler) OwlRefreshCatalog(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if err := client.RefreshCatalog(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// OwlPlayChannel triggers playback of a channel in OWL and returns streaming URLs
func (h *Handler) OwlPlayChannel(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	channelID := c.Param("id")
	result, err := client.PlayChannel(c.Request.Context(), channelID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// OwlStopChannel stops playback of a channel in OWL
func (h *Handler) OwlStopChannel(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	channelID := c.Param("id")
	if err := client.StopChannel(c.Request.Context(), channelID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	client.InvalidateRTSPCache(channelID)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// OwlSnapshot captures a JPEG snapshot from a channel
func (h *Handler) OwlSnapshot(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	channelID := c.Param("id")
	data, err := client.Snapshot(c.Request.Context(), channelID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "image/jpeg", data)
}

// OwlPTZControl sends PTZ command to a channel
func (h *Handler) OwlPTZControl(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	channelID := c.Param("id")
	var req struct {
		Command string `json:"command" binding:"required"` // left/right/up/down/zoom_in/zoom_out/stop
		Speed   int    `json:"speed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := client.PTZControl(c.Request.Context(), channelID, req.Command, req.Speed); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// OwlImportChannel imports an OWL channel as a Hub camera and assigns to a Worker.
// Key workflow: call Worker's OWL → resolve RTSP → create Hub camera → assign to Worker.
func (h *Handler) OwlImportChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req struct {
		Name    string `json:"name"`
		GroupID string `json:"group_id"`
		NodeID  string `json:"node_id"` // target Worker (whose OWL manages this camera)
	}
	c.ShouldBindJSON(&req)

	// Resolve OWL client for target node
	owlClient := h.getNodeOwlClient(req.NodeID)
	if owlClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no OWL client for node " + req.NodeID})
		return
	}

	cam, err := h.importSingleChannel(c.Request.Context(), owlClient, channelID, req.Name, req.GroupID, req.NodeID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cam)
}

// OwlImportBatch imports all online channels from a device (or all devices).
// Uses the target Worker's OWL to list and import channels.
func (h *Handler) OwlImportBatch(c *gin.Context) {
	var req struct {
		DeviceID string `json:"device_id"` // optional: import from specific device
		GroupID  string `json:"group_id"`
		NodeID   string `json:"node_id"` // target Worker (whose OWL manages these cameras)
	}
	c.ShouldBindJSON(&req)

	// Resolve OWL client for target node
	owlClient := h.getNodeOwlClient(req.NodeID)
	if owlClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no OWL client available"})
		return
	}

	ctx := c.Request.Context()

	// Fetch channels from the target Worker's OWL
	var channels []owl.Channel
	var err error
	if req.DeviceID != "" {
		channels, err = owlClient.ListChannelsByDevice(ctx, req.DeviceID)
	} else {
		channels, err = owlClient.GetAllChannels(ctx)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// Build set of already-imported owl_channel_ids to avoid duplicates
	alreadyImported := h.getImportedChannelIDs()

	type importResult struct {
		ChannelID string `json:"channel_id"`
		CameraID  string `json:"camera_id,omitempty"`
		Skipped   bool   `json:"skipped,omitempty"`
		Error     string `json:"error,omitempty"`
	}

	var results []importResult
	for _, ch := range channels {
		if !ch.IsOnline {
			continue
		}
		chID := ch.ID
		if chID == "" {
			chID = ch.ChannelID
		}
		if chID == "" {
			continue
		}

		// Skip already-imported channels
		if _, exists := alreadyImported[chID]; exists {
			results = append(results, importResult{ChannelID: chID, Skipped: true})
			continue
		}

		// Use importFromChannel with the target Worker's OWL
		cam, importErr := h.importFromChannel(ctx, owlClient, ch, req.GroupID, req.NodeID)
		if importErr != nil {
			results = append(results, importResult{ChannelID: chID, Error: importErr.Error()})
		} else {
			results = append(results, importResult{ChannelID: chID, CameraID: cam.ID})
		}
	}

	c.JSON(http.StatusOK, gin.H{"imported": results, "total": len(results)})
}

// getImportedChannelIDs returns a set of owl_channel_ids already present in the store.
func (h *Handler) getImportedChannelIDs() map[string]struct{} {
	result := make(map[string]struct{})
	all := h.store.ListCameras(CameraFilter{})
	for _, cam := range all {
		if chID, ok := cam.Config["owl_channel_id"].(string); ok && chID != "" {
			result[chID] = struct{}{}
		}
	}
	return result
}

// importSingleChannel imports one OWL channel by ID (fetches channel info from OWL).
// The owlC parameter specifies which Worker's OWL to use.
func (h *Handler) importSingleChannel(ctx context.Context, owlC *owl.Client, channelID, name, groupID, nodeID string) (*Camera, error) {
	// Fetch channel info to get Type for protocol detection
	ch, err := owlC.GetChannel(ctx, channelID)
	if err != nil {
		// Fallback: import without channel metadata
		return h.importFromChannel(ctx, owlC, owl.Channel{ID: channelID, Name: name}, groupID, nodeID)
	}
	if name != "" {
		ch.Name = name
	}
	return h.importFromChannel(ctx, owlC, *ch, groupID, nodeID)
}

// importFromChannel imports an OWL channel as a Hub camera (metadata only).
// Hub does NOT call PlayChannel/ResolveRTSP — that is Worker's responsibility.
// Hub stores: channel ID, protocol, device info, optional direct URL.
// Worker will use its own OWL to resolve RTSP when it receives the assignment.
func (h *Handler) importFromChannel(ctx context.Context, owlC *owl.Client, ch owl.Channel, groupID, nodeID string) (*Camera, error) {
	channelID := ch.ID
	if channelID == "" {
		channelID = ch.ChannelID
	}
	if channelID == "" {
		return nil, fmt.Errorf("channel has no ID")
	}

	// Determine protocol type from OWL channel type
	protocol := "rtsp"
	switch strings.ToUpper(ch.Type) {
	case "GB28181":
		protocol = "gb28181"
	case "ONVIF":
		protocol = "onvif"
	case "RTMP":
		protocol = "rtmp"
	}

	name := ch.Name
	if name == "" {
		name = "OWL-" + channelID
	}

	// Build config with metadata for Worker to use
	config := map[string]interface{}{
		"owl_channel_id": channelID,
		"source":         "owl_import",
		"protocol_type":  protocol,
	}
	if ch.DeviceID != "" {
		config["owl_device_id"] = ch.DeviceID
	}
	if ch.DeviceIP != "" {
		config["device_ip"] = ch.DeviceIP
	}

	// URL strategy by protocol:
	//   GB28181 → hub_sip_remote: Hub OWL SIP signaling, Worker ZLM receives RTP
	//   ONVIF with StreamURL → direct: Worker pulls camera RTSP directly
	//   ONVIF no StreamURL → worker_owl: Worker OWL resolves RTSP via ONVIF discovery
	//   RTSP/RTMP → direct: always use URL as-is, never proxy through OWL
	cameraURL := ""
	switch protocol {
	case "gb28181":
		config["stream_mode"] = "hub_sip_remote"
	case "onvif":
		if ch.StreamURL != "" {
			cameraURL = ch.StreamURL
			config["camera_direct_url"] = ch.StreamURL
			config["stream_mode"] = "direct"
		} else {
			// ONVIF without StreamURL: Worker OWL discovers RTSP via ONVIF GetStreamUri
			config["stream_mode"] = "worker_owl"
		}
	case "rtsp", "rtmp":
		// Direct protocols — URL is the stream itself, no OWL needed
		if ch.StreamURL != "" {
			cameraURL = ch.StreamURL
		}
		config["stream_mode"] = "direct"
	default:
		if ch.StreamURL != "" {
			cameraURL = ch.StreamURL
			config["stream_mode"] = "direct"
		} else {
			config["stream_mode"] = "worker_owl"
		}
	}

	cameraID := "cam_" + uuid.New().String()[:8]

	cam := &Camera{
		ID:       cameraID,
		Name:     name,
		URL:      cameraURL,
		Protocol: protocol,
		GroupID:  groupID,
		NodeID:   nodeID,
		Enabled:  true,
		Config:   config,
	}

	// Auto-assign to healthy node if not specified
	if cam.NodeID == "" && h.nodeGetter != nil {
		for _, node := range h.nodeGetter() {
			if node.Healthy {
				cam.NodeID = node.ID
				break
			}
		}
	}

	if err := h.store.CreateCamera(cam); err != nil {
		return nil, err
	}

	// Activate stream (GB28181: Hub OWL SIP → Worker ZLM, others: no-op)
	if cam.NodeID != "" {
		if rtspURL, err := h.ActivateStream(ctx, cam); err != nil {
			log.Printf("[CameraHandler] activateStream failed for %s: %v", cameraID, err)
		} else if rtspURL != "" && rtspURL != cam.URL {
			cam.URL = rtspURL
			h.store.UpdateCamera(cam)
		}
		go h.notifyNodeAddStream(cam)
	}

	log.Printf("[CameraHandler] imported OWL channel %s → camera %s (protocol=%s, stream_mode=%s)",
		channelID, cameraID, protocol, config["stream_mode"])
	return cam, nil
}

// OwlDiscover triggers ONVIF device discovery via OWL
func (h *Handler) OwlDiscover(c *gin.Context) {
	client, ok := h.resolveOWL(c)
	if !ok {
		return
	}
	results, err := client.DiscoverONVIF(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"discovered": results, "total": len(results)})
}
