// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

// Package camera provides go2rtc integration with OWL (GB28181) bridge.
package camera

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/rivision/rivision-cli/internal/owl"
)

// OWLStreamBridge bridges OWL GB28181 channels to go2rtc streams.
// Architecture:
//   OWL (GB28181 devices)
//     → OWL.Play(channelID) → ZLM → RTSP → go2rtc → fMP4 → Browser
//
// Responsibilities:
//   1. Dynamically register OWL channel streams with go2rtc on first access
//   2. Cache RTSP URLs to avoid repeated OWL.Play() calls
//   3. Serve /api/owl/streams in go2rtc-compatible format for CameraList.vue
//   4. Capture frames for YOLO/VLM analysis from go2rtc (not OWL directly)
type OWLStreamBridge struct {
	owl    *owl.Client
	g2r    *Go2rtcClient
	urlTTL time.Duration

	mu    sync.RWMutex
	cache map[string]*streamEntry // channelID → entry
}

type streamEntry struct {
	StreamID  string            // go2rtc stream key (same as channelID)
	ChannelID string            // OWL internal channel ID
	URLs      map[string]string // RTSP/RTMP/HTTP-FLV URLs from OWL.Play()
	AddedAt   time.Time
}

// NewOWLStreamBridge creates a new OWL→go2rtc bridge.
func NewOWLStreamBridge(owlClient *owl.Client, go2rtcClient *Go2rtcClient) *OWLStreamBridge {
	return &OWLStreamBridge{
		owl:    owlClient,
		g2r:    go2rtcClient,
		urlTTL: 5 * time.Minute, // re-fetch URLs after 5 min
		cache:  make(map[string]*streamEntry),
	}
}

// EnsureStream makes sure a channel is registered with go2rtc.
// Returns the go2rtc stream key (same as channelID) and available URLs.
func (b *OWLStreamBridge) EnsureStream(channelID string) (*streamEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	entry, ok := b.cache[channelID]
	if ok && time.Since(entry.AddedAt) < b.urlTTL {
		return entry, nil
	}

	// Play through OWL to get actual RTSP URL from ZLM
	play, err := b.owl.Play(channelID)
	if err != nil {
		return nil, fmt.Errorf("OWL.Play(%s): %w", channelID, err)
	}

	// Extract RTSP URL from play response
	var rtspURL string
	if rtsp, ok := play.URLs["rtsp"]; ok && rtsp != "" {
		rtspURL = rtsp
	} else if rtmp, ok := play.URLs["rtmp"]; ok && rtmp != "" {
		rtspURL = rtmp
	} else if flv, ok := play.URLs["http_flv"]; ok && flv != "" {
		// go2rtc can ingest HTTP-FLV via ffmpeg source
		rtspURL = fmt.Sprintf("ffmpeg:%s#live", flv)
	}

	if rtspURL == "" {
		return nil, fmt.Errorf("no RTSP/RTMP URL in OWL.Play response for %s", channelID)
	}

	// Register with go2rtc
	if err := b.g2r.AddStream(channelID, rtspURL); err != nil {
		// Log but don't fail — the stream might already exist
		log.Printf("[OWL-Bridge] go2rtc.AddStream(%s, %s) failed: %v (continuing)", channelID, rtspURL, err)
	}

	entry = &streamEntry{
		StreamID:  channelID,
		ChannelID: channelID,
		URLs:      play.URLs,
		AddedAt:   time.Now(),
	}
	b.cache[channelID] = entry

	log.Printf("[OWL-Bridge] Registered stream: %s → %s", channelID, rtspURL)
	return entry, nil
}

// GetStreamURLs returns the cached play URLs for a channel.
// If not cached, it calls OWL.Play() to fetch them.
func (b *OWLStreamBridge) GetStreamURLs(channelID string) (map[string]string, error) {
	entry, err := b.EnsureStream(channelID)
	if err != nil {
		return nil, err
	}
	return entry.URLs, nil
}

// GetAllStreams returns go2rtc-compatible stream map for CameraList.vue.
// This merges real go2rtc streams with OWL-bridged streams.
func (b *OWLStreamBridge) GetAllStreams() (map[string]Go2rtcStream, error) {
	// Get go2rtc-native streams
	native, err := b.g2r.GetStreams()
	if err != nil {
		return nil, fmt.Errorf("get go2rtc streams: %w", err)
	}

	// If OWL is enabled, get all OWL channels and update/add them
	// OWL 通道名称优先于 go2rtc 原生流名称
	if b.owl != nil && b.owl.IsEnabled() {
		channels, err := b.owl.GetAllChannels()
		if err == nil {
			for _, ch := range channels {
				streamKey := ch.ID
				if streamKey == "" {
					streamKey = ch.ChannelID
				}
				online := "offline"
				if ch.IsOnline {
					online = "online"
				}
				displayName := strings.TrimSpace(ch.Name)
				if displayName == "" {
					displayName = streamKey // 回退到 ID
				}
				// 始终更新 OWL 通道信息，确保名称正确显示
				native[streamKey] = Go2rtcStream{
					Name:      displayName,
					Sources:   []string{fmt.Sprintf("proxy://owl/%s", ch.ID)},
					Producers: []Producer{{Type: ch.Type, URL: online}},
				}
			}
		}
	}

	return native, nil
}

// GetStreamKey returns the go2rtc stream key for a given OWL channelID.
// It caches the mapping for efficiency.
func (b *OWLStreamBridge) GetStreamKey(channelID string) (string, error) {
	entry, err := b.EnsureStream(channelID)
	if err != nil {
		return "", err
	}
	return entry.StreamID, nil
}

// CaptureFrame captures a frame from go2rtc for the given channel.
// This is the unified frame capture path used by YOLO/VLM.
func (b *OWLStreamBridge) CaptureFrame(channelID string, width int) (*Frame, error) {
	streamKey, err := b.GetStreamKey(channelID)
	if err != nil {
		return nil, fmt.Errorf("get stream key: %w", err)
	}

	if width > 0 {
		return b.g2r.CaptureFrameResized(streamKey, width)
	}
	return b.g2r.CaptureFrame(streamKey)
}

// InvalidateCache removes a channel from the cache, forcing re-registration.
func (b *OWLStreamBridge) InvalidateCache(channelID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.cache, channelID)
}

// ListCached returns all cached channel IDs.
func (b *OWLStreamBridge) ListCached() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, 0, len(b.cache))
	for id := range b.cache {
		result = append(result, id)
	}
	return result
}

// StopStream stops a channel's stream and removes it from go2rtc.
func (b *OWLStreamBridge) StopStream(channelID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.cache[channelID]; !ok {
		return nil
	}

	// Remove from go2rtc
	if err := b.g2r.RemoveStream(channelID); err != nil {
		log.Printf("[OWL-Bridge] go2rtc.RemoveStream(%s) failed: %v", channelID, err)
	}

	// Stop through OWL
	if err := b.owl.Stop(channelID); err != nil {
		log.Printf("[OWL-Bridge] OWL.Stop(%s) failed: %v", channelID, err)
	}

	delete(b.cache, channelID)
	log.Printf("[OWL-Bridge] Stopped stream: %s", channelID)
	return nil
}

// Snapshot captures a JPEG snapshot directly from OWL (via ZLM snapshot API).
func (b *OWLStreamBridge) Snapshot(channelID string) ([]byte, error) {
	return b.owl.SnapshotWithAuth(channelID)
}

// ProxyStreamURL returns the go2rtc stream URL for a given channel.
// Used by the frontend to build the video src.
func (b *OWLStreamBridge) ProxyStreamURL(channelID string) (string, error) {
	streamKey, err := b.GetStreamKey(channelID)
	if err != nil {
		return "", err
	}
	// go2rtc stream.mp4 endpoint: /api/stream.mp4?src={streamKey}
	return fmt.Sprintf("/api/go2rtc/stream.mp4?src=%s", streamKey), nil
}

// NormalizeChannelID extracts a clean channel ID from various input formats.
// Accepts: channelID, deviceID/channelID, or "proxy://owl/{channelID}".
func NormalizeChannelID(id string) string {
	if strings.HasPrefix(id, "proxy://owl/") {
		return strings.TrimPrefix(id, "proxy://owl/")
	}
	if strings.Contains(id, "/") {
		parts := strings.SplitN(id, "/", 2)
		return parts[len(parts)-1]
	}
	return id
}
