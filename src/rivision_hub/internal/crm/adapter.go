// Package crm implements CRM system integration via webhooks (G3).
// Provides bidirectional sync: CRM → Hub (member photos → feature library)
// and Hub → CRM (VIP visit notifications via webhook).
package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// Config configures CRM integration.
type Config struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Type          string        `yaml:"type" json:"type"`                     // "generic_webhook" | "custom"
	SyncURL       string        `yaml:"sync_url" json:"sync_url"`            // GET URL to pull member list
	NotifyURL     string        `yaml:"notify_url" json:"notify_url"`        // POST URL for VIP visit events
	SyncInterval  time.Duration `yaml:"sync_interval" json:"sync_interval"`  // default 1h
	AuthHeader    string        `yaml:"auth_header" json:"auth_header"`      // e.g. "Bearer xxx"
	MaxMembers    int           `yaml:"max_members" json:"max_members"`      // limit (default 10000)
}

// DefaultConfig returns sensible CRM defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		Type:         "generic_webhook",
		SyncInterval: 1 * time.Hour,
		MaxMembers:   10000,
	}
}

// Member represents a CRM member/VIP to be synced into the feature library.
type Member struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Group    string `json:"group"`     // e.g. "diamond", "gold", "vip"
	PhotoURL string `json:"photo_url"` // URL to member photo for face embedding
}

// VisitEvent is the payload sent to CRM when a VIP is detected.
type VisitEvent struct {
	Event        string  `json:"event"`         // "vip_visit"
	PersonID     string  `json:"person_id"`
	PersonName   string  `json:"person_name"`
	Group        string  `json:"group"`
	CameraID     string  `json:"camera_id"`
	NodeID       string  `json:"node_id"`
	Timestamp    string  `json:"timestamp"`     // RFC3339
	MatchScore   float32 `json:"match_score"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
}

// EmbedFunc converts a photo (JPEG bytes) into a 512D face vector.
type EmbedFunc func(ctx context.Context, photoData []byte) ([]float32, error)

// FeatureUpdateFunc updates the feature library with synced members.
type FeatureUpdateFunc func(members []FeatureMember)

// FeatureMember is a member with computed face vector ready for the feature library.
type FeatureMember struct {
	ID     string
	Name   string
	Group  string
	Vector []float32
}

// Adapter manages CRM ↔ Hub synchronisation.
type Adapter struct {
	cfg            Config
	client         *http.Client
	embedFn        EmbedFunc
	featureUpdateFn FeatureUpdateFunc

	mu          sync.RWMutex
	lastSync    time.Time
	memberCount int
}

// NewAdapter creates a CRM adapter.
func NewAdapter(cfg Config, embedFn EmbedFunc, featureUpdateFn FeatureUpdateFunc) *Adapter {
	return &Adapter{
		cfg:             cfg,
		client:          &http.Client{Timeout: 30 * time.Second},
		embedFn:         embedFn,
		featureUpdateFn: featureUpdateFn,
	}
}

// Run starts the periodic CRM sync loop. Blocks until ctx is cancelled.
func (a *Adapter) Run(ctx context.Context) {
	if !a.cfg.Enabled || a.cfg.SyncURL == "" {
		log.Printf("[crm] CRM integration disabled")
		return
	}

	log.Printf("[crm] starting CRM sync, interval=%v, url=%s", a.cfg.SyncInterval, a.cfg.SyncURL)

	// Initial sync
	a.syncMembers(ctx)

	ticker := time.NewTicker(a.cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.syncMembers(ctx)
		}
	}
}

// NotifyVisit sends a VIP visit event to the CRM webhook.
func (a *Adapter) NotifyVisit(event VisitEvent) error {
	if !a.cfg.Enabled || a.cfg.NotifyURL == "" {
		return nil
	}

	event.Event = "vip_visit"
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal visit event: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.cfg.NotifyURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build CRM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.AuthHeader != "" {
		req.Header.Set("Authorization", a.cfg.AuthHeader)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("CRM notify: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("CRM notify: HTTP %d", resp.StatusCode)
	}

	log.Printf("[crm] notified CRM: VIP %s (%s) at camera %s", event.PersonName, event.PersonID, event.CameraID)
	return nil
}

// Stats returns CRM sync statistics.
func (a *Adapter) Stats() map[string]interface{} {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return map[string]interface{}{
		"enabled":      a.cfg.Enabled,
		"last_sync":    a.lastSync.Format(time.RFC3339),
		"member_count": a.memberCount,
		"sync_url":     a.cfg.SyncURL,
		"notify_url":   a.cfg.NotifyURL,
	}
}

func (a *Adapter) syncMembers(ctx context.Context) {
	log.Printf("[crm] syncing members from CRM...")

	members, err := a.fetchMembers(ctx)
	if err != nil {
		log.Printf("[crm] fetch members failed: %v", err)
		return
	}

	if len(members) > a.cfg.MaxMembers {
		members = members[:a.cfg.MaxMembers]
	}

	// Embed face photos
	var featureMembers []FeatureMember
	for _, m := range members {
		if m.PhotoURL == "" {
			continue
		}

		photo, err := a.fetchPhoto(ctx, m.PhotoURL)
		if err != nil {
			log.Printf("[crm] fetch photo for %s: %v", m.ID, err)
			continue
		}

		vec, err := a.embedFn(ctx, photo)
		if err != nil {
			log.Printf("[crm] embed face for %s: %v", m.ID, err)
			continue
		}

		featureMembers = append(featureMembers, FeatureMember{
			ID:     m.ID,
			Name:   m.Name,
			Group:  m.Group,
			Vector: vec,
		})
	}

	// Update feature library
	if len(featureMembers) > 0 && a.featureUpdateFn != nil {
		a.featureUpdateFn(featureMembers)
	}

	a.mu.Lock()
	a.lastSync = time.Now()
	a.memberCount = len(featureMembers)
	a.mu.Unlock()

	log.Printf("[crm] synced %d/%d members with face vectors", len(featureMembers), len(members))
}

func (a *Adapter) fetchMembers(ctx context.Context) ([]Member, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.SyncURL, nil)
	if err != nil {
		return nil, err
	}
	if a.cfg.AuthHeader != "" {
		req.Header.Set("Authorization", a.cfg.AuthHeader)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", a.cfg.SyncURL, err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data[:min(200, len(data))]))
	}

	var result struct {
		Members []Member `json:"members"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse members: %w", err)
	}
	return result.Members, nil
}

func (a *Adapter) fetchPhoto(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB limit
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
