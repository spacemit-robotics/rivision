// Package webserver provides HTTP routes for OWL (GB28181) integration.
package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rivision/rivision-cli/internal/camera"
	"github.com/rivision/rivision-cli/internal/owl"
)

// OWLRoutes sets up OWL API routes on the given gin.Engine.
func OWLRoutes(e *gin.Engine, owlClient *owl.Client, go2rtcURL string, bridge *camera.OWLStreamBridge) {
	if owlClient == nil || !owlClient.IsEnabled() {
		return
	}

	grp := e.Group("/api/owl")
	{
		// Auth
		grp.POST("/login", handleOWLLogin(owlClient))

		// Server info
		grp.GET("/server/info", handleOWLServerInfo(owlClient))

		// Devices
		grp.GET("/devices", handleOWLDevices(owlClient))
		grp.GET("/devices/:id", handleOWLDevice(owlClient))
		grp.GET("/devices/:id/channels", handleOWLDeviceChannels(owlClient))

		// Channels — primary interface for the frontend
		grp.GET("/channels", handleOWLChannels(owlClient))

		// Streams — go2rtc-compatible format for CameraList.vue
		grp.GET("/streams", handleOWLStreams(owlClient, bridge))

		// Playback
		grp.POST("/channels/:id/play", handleOWLPlay(owlClient, bridge))
		grp.POST("/channels/:id/stop", handleOWLStop(owlClient, bridge))

		// Snapshot
		grp.POST("/channels/:id/snapshot", handleOWLSnapshot(owlClient))

		// AI
		grp.POST("/channels/:id/ai/enable", handleOWLAIEnable(owlClient))
		grp.POST("/channels/:id/ai/disable", handleOWLAIDisable(owlClient))

		// PTZ
		grp.POST("/ptz/control", handleOWLPTZ(owlClient))
	}
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

func handleOWLLogin(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		// For OWL, login is handled by the client itself.
		// The frontend uses /api/owl/login to trigger client-side login.
		if err := client.Login(); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"token":   "stored-in-client", // token is internal to the client
		})
	}
}

// ─── Server ───────────────────────────────────────────────────────────────────

func handleOWLServerInfo(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		info, err := client.GetServerInfo()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, info)
	}
}

// ─── Devices ──────────────────────────────────────────────────────────────────

func handleOWLDevices(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := 1
		pageSize := 100
		if p := c.Query("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if ps := c.Query("page_size"); ps != "" {
			fmt.Sscanf(ps, "%d", &pageSize)
		}

		items, total, err := client.GetDevices(page, pageSize)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
	}
}

func handleOWLDevice(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceID := c.Param("id")
		device, err := client.GetDevice(deviceID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, device)
	}
}

func handleOWLDeviceChannels(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		deviceID := c.Param("id")
		page := 1
		pageSize := 100
		if p := c.Query("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if ps := c.Query("page_size"); ps != "" {
			fmt.Sscanf(ps, "%d", &pageSize)
		}

		items, total, err := client.GetChannels(page, pageSize, deviceID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
	}
}

// ─── Channels ─────────────────────────────────────────────────────────────────

func handleOWLChannels(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		page := 1
		pageSize := 100
		if p := c.Query("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if ps := c.Query("page_size"); ps != "" {
			fmt.Sscanf(ps, "%d", &pageSize)
		}
		deviceID := c.Query("device_id")

		items, total, err := client.GetChannels(page, pageSize, deviceID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
	}
}

// ─── Streams — go2rtc-compatible format for CameraList.vue ───────────────────

// handleOWLStreams returns all OWL channels in go2rtc-compatible stream format:
//   { "ch3vw2l": { "name": "ch01", "producers": [{ "type": "rtsp", "url": "..." }] } }
func handleOWLStreams(client *owl.Client, bridge *camera.OWLStreamBridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		channels, err := client.GetAllChannels()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}

		// Build go2rtc-compatible map: map[string]camera.Go2rtcStream
		streams := make(map[string]camera.Go2rtcStream, len(channels))
		for _, ch := range channels {
			// Use channel internal ID as the stream name (go2rtc key)
			name := ch.ID
			if name == "" {
				name = ch.ChannelID
			}

			onlineStatus := "offline"
			if ch.IsOnline {
				onlineStatus = "online"
			}

			streams[name] = camera.Go2rtcStream{
				Name:    ch.Name,
				Sources: []string{fmt.Sprintf("proxy://owl/%s", ch.ID)},
				Producers: []camera.Producer{
					{Type: ch.Type, URL: onlineStatus},
				},
			}
		}

		// If bridge is available, merge with go2rtc-native streams
		if bridge != nil {
			native, err := bridge.GetAllStreams()
			if err == nil {
				for k, v := range native {
					streams[k] = v
				}
			}
		}

		c.JSON(http.StatusOK, streams)
	}
}

// ─── Playback ─────────────────────────────────────────────────────────────────

// handleOWLPlay initiates streaming for a channel and returns multi-protocol URLs.
// It also registers the RTSP stream with go2rtc via the bridge.
func handleOWLPlay(client *owl.Client, bridge *camera.OWLStreamBridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("id")
		if channelID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing channel id"})
			return
		}

		output, err := client.Play(channelID)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		// If bridge is available, pre-register the stream with go2rtc
		if bridge != nil {
			if _, err := bridge.EnsureStream(channelID); err != nil {
				// Log but don't fail — play succeeded, bridge is optional
				fmt.Printf("[OWL] Bridge.EnsureStream(%s) warning: %v\n", channelID, err)
			}
		}

		c.JSON(http.StatusOK, output)
	}
}

func handleOWLStop(client *owl.Client, bridge *camera.OWLStreamBridge) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("id")

		// Stop in OWL
		if err := client.Stop(channelID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		// Remove from go2rtc via bridge
		if bridge != nil {
			bridge.StopStream(channelID)
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// ─── Snapshot ─────────────────────────────────────────────────────────────────

func handleOWLSnapshot(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("id")
		data, err := client.SnapshotWithAuth(channelID)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}

		c.Header("Content-Type", "image/jpeg")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "image/jpeg", data)
	}
}

// ─── AI ─────────────────────────────────────────────────────────────────────

func handleOWLAIEnable(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("id")
		if err := client.EnableAI(channelID); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

func handleOWLAIDisable(client *owl.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("id")
		if err := client.DisableAI(channelID); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// ─── PTZ ─────────────────────────────────────────────────────────────────────

func handleOWLPTZ(client *owl.Client) gin.HandlerFunc {
	type PTZRequest struct {
		ChannelID string `json:"channel_id" binding:"required"`
		Command   string `json:"command" binding:"required"`
		Speed     int    `json:"speed"`
	}

	return func(c *gin.Context) {
		var req PTZRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Speed == 0 {
			req.Speed = 4
		}
		if err := client.PTZControl(req.ChannelID, req.Command, req.Speed); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(c *gin.Context, status int, v interface{}) {
	c.Header("Content-Type", "application/json")
	c.JSON(status, v)
}

func writeError(c *gin.Context, status int, format string, args ...interface{}) {
	body, _ := json.Marshal(gin.H{"error": fmt.Sprintf(format, args...)})
	c.Header("Content-Type", "application/json")
	c.Data(status, "application/json", body)
}
