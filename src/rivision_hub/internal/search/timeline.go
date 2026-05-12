// Package search — timeline.go implements person timeline generation (G7).
// Given a face photo or detection ID, searches face_vectors across all Workers
// and aggregates appearances into a chronological timeline.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

// TimelineRequest is the input for building a person's timeline.
type TimelineRequest struct {
	ImageData   string     `json:"image_data,omitempty"`    // base64 face photo
	DetectionID string     `json:"detection_id,omitempty"`  // OR existing detection ID
	NodeID      string     `json:"node_id,omitempty"`       // node owning the detection
	TimeStart   *time.Time `json:"time_start,omitempty"`
	TimeEnd     *time.Time `json:"time_end,omitempty"`
	Threshold   float32    `json:"threshold"`               // cosine similarity (default 0.80)
	IncludeThumb bool      `json:"include_thumbnails"`
}

// TimelineEntry is one appearance segment on a camera.
type TimelineEntry struct {
	CameraID     string  `json:"camera_id"`
	CameraName   string  `json:"camera_name,omitempty"`
	NodeID       string  `json:"node_id"`
	FirstSeen    string  `json:"first_seen"`              // RFC3339
	LastSeen     string  `json:"last_seen"`
	DurationSec  float64 `json:"duration_sec"`
	DetectionIDs []string `json:"detection_ids,omitempty"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"` // first appearance thumb
	Score        float32 `json:"score"`                   // avg match score
}

// TimelineResponse is the full timeline for a person.
type TimelineResponse struct {
	TotalAppearances int             `json:"total_appearances"`
	TotalDurationSec float64         `json:"total_duration_sec"`
	Timeline         []TimelineEntry `json:"timeline"`
	SearchTimeMs     int64           `json:"search_time_ms"`
}

// TimelineSearcher generates person timelines via face vector matching.
type TimelineSearcher struct {
	federated *FederatedSearch
	embedURL  string       // Hub embed service URL
	client    *http.Client
}

// NewTimelineSearcher creates a timeline search engine.
func NewTimelineSearcher(federated *FederatedSearch, embedURL string) *TimelineSearcher {
	return &TimelineSearcher{
		federated: federated,
		embedURL:  embedURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// BuildTimeline generates a chronological person timeline across all Workers.
func (ts *TimelineSearcher) BuildTimeline(ctx context.Context, nodes []NodeInfo, req TimelineRequest) (*TimelineResponse, error) {
	startTime := time.Now()

	if req.Threshold <= 0 {
		req.Threshold = 0.80
	}

	// Step 1: Get face vector
	var faceVec []float32
	var err error

	if req.ImageData != "" {
		faceVec, err = ts.embedFace(ctx, req.ImageData)
	} else if req.DetectionID != "" {
		faceVec, err = ts.fetchFaceVec(ctx, req.DetectionID, req.NodeID, nodes)
	} else {
		return nil, fmt.Errorf("either image_data or detection_id is required")
	}
	if err != nil {
		return nil, fmt.Errorf("get face vector: %w", err)
	}

	// Step 2: Search face_vectors across all Workers
	type nodeResult struct {
		nodeID  string
		matches []faceSearchMatch
		err     error
	}

	healthyNodes := filterHealthy(nodes)
	resultsCh := make(chan nodeResult, len(healthyNodes))
	var wg sync.WaitGroup

	for _, node := range healthyNodes {
		wg.Add(1)
		go func(n NodeInfo) {
			defer wg.Done()
			matches, err := ts.searchFacesOnNode(ctx, n, faceVec, req.Threshold, 200)
			resultsCh <- nodeResult{nodeID: n.ID, matches: matches, err: err}
		}(node)
	}

	wg.Wait()
	close(resultsCh)

	// Collect all matches
	var allMatches []faceSearchMatch
	for nr := range resultsCh {
		if nr.err != nil {
			log.Printf("[timeline] node %s search failed: %v", nr.nodeID, nr.err)
			continue
		}
		for i := range nr.matches {
			nr.matches[i].NodeID = nr.nodeID
		}
		allMatches = append(allMatches, nr.matches...)
	}

	// Step 3: Time filter
	if req.TimeStart != nil || req.TimeEnd != nil {
		allMatches = filterByTime(allMatches, req.TimeStart, req.TimeEnd)
	}

	// Step 4: Aggregate into timeline entries (group by camera, merge close appearances)
	timeline := aggregateTimeline(allMatches, nodes, req.IncludeThumb)

	totalDuration := 0.0
	for _, te := range timeline {
		totalDuration += te.DurationSec
	}

	return &TimelineResponse{
		TotalAppearances: len(allMatches),
		TotalDurationSec: totalDuration,
		Timeline:         timeline,
		SearchTimeMs:     time.Since(startTime).Milliseconds(),
	}, nil
}

// faceSearchMatch is a single face match from a Worker.
type faceSearchMatch struct {
	DetectionID string    `json:"detection_id"`
	CameraID    string    `json:"camera_id"`
	NodeID      string    `json:"node_id"`
	Timestamp   time.Time `json:"timestamp"`
	Score       float32   `json:"score"`
}

func (ts *TimelineSearcher) embedFace(ctx context.Context, imageData string) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"image": imageData,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.embedURL+"/v1/embeddings/image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed face: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed face: status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

func (ts *TimelineSearcher) fetchFaceVec(ctx context.Context, detectionID, nodeID string, nodes []NodeInfo) ([]float32, error) {
	// Ask the Worker that owns this detection for its face vector
	var host string
	var port int
	for _, n := range nodes {
		if n.ID == nodeID {
			host = n.Host
			port = n.AgentPort
			break
		}
	}
	if host == "" {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	url := fmt.Sprintf("http://%s:%d/api/v1/search/faces/vector?detection_id=%s", host, port, detectionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch face_vec: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Vector, nil
}

func (ts *TimelineSearcher) searchFacesOnNode(ctx context.Context, node NodeInfo, faceVec []float32, threshold float32, topK int) ([]faceSearchMatch, error) {
	url := fmt.Sprintf("http://%s:%d/api/v1/search/faces", node.Host, node.AgentPort)

	body, _ := json.Marshal(map[string]interface{}{
		"query_vec":  faceVec,
		"collection": "face_vectors",
		"top_k":      topK,
		"threshold":  threshold,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			DetectionID string  `json:"detection_id"`
			CameraID    string  `json:"camera_id"`
			Score       float32 `json:"score"`
			Timestamp   string  `json:"timestamp"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	matches := make([]faceSearchMatch, 0, len(result.Results))
	for _, r := range result.Results {
		if r.Score < threshold {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, r.Timestamp)
		matches = append(matches, faceSearchMatch{
			DetectionID: r.DetectionID,
			CameraID:    r.CameraID,
			Timestamp:   ts,
			Score:       r.Score,
		})
	}
	return matches, nil
}

// --- Aggregation ---

func aggregateTimeline(matches []faceSearchMatch, nodes []NodeInfo, includeThumb bool) []TimelineEntry {
	if len(matches) == 0 {
		return nil
	}

	// Sort by timestamp
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Timestamp.Before(matches[j].Timestamp)
	})

	// Group by camera, merge appearances within 5-minute gap
	const mergeGap = 5 * time.Minute

	type segKey struct {
		CameraID string
		NodeID   string
	}

	var entries []TimelineEntry
	var current *TimelineEntry

	for _, m := range matches {
		if current != nil && current.CameraID == m.CameraID && current.NodeID == m.NodeID {
			// Check if within merge gap
			lastSeen, _ := time.Parse(time.RFC3339, current.LastSeen)
			if m.Timestamp.Sub(lastSeen) <= mergeGap {
				current.LastSeen = m.Timestamp.Format(time.RFC3339)
				current.DetectionIDs = append(current.DetectionIDs, m.DetectionID)
				current.Score = (current.Score + m.Score) / 2 // running average
				continue
			}
		}

		// Finalize current and start new
		if current != nil {
			firstSeen, _ := time.Parse(time.RFC3339, current.FirstSeen)
			lastSeen, _ := time.Parse(time.RFC3339, current.LastSeen)
			current.DurationSec = lastSeen.Sub(firstSeen).Seconds()
			if current.DurationSec < 1 {
				current.DurationSec = 1 // minimum 1 second
			}
			entries = append(entries, *current)
		}

		thumbURL := ""
		if includeThumb {
			thumbURL = buildThumbnailURL(m.NodeID, m.DetectionID, nodes)
		}

		current = &TimelineEntry{
			CameraID:     m.CameraID,
			NodeID:       m.NodeID,
			FirstSeen:    m.Timestamp.Format(time.RFC3339),
			LastSeen:     m.Timestamp.Format(time.RFC3339),
			DetectionIDs: []string{m.DetectionID},
			ThumbnailURL: thumbURL,
			Score:        m.Score,
		}
	}

	// Finalize last entry
	if current != nil {
		firstSeen, _ := time.Parse(time.RFC3339, current.FirstSeen)
		lastSeen, _ := time.Parse(time.RFC3339, current.LastSeen)
		current.DurationSec = lastSeen.Sub(firstSeen).Seconds()
		if current.DurationSec < 1 {
			current.DurationSec = 1
		}
		entries = append(entries, *current)
	}

	return entries
}

func filterByTime(matches []faceSearchMatch, start, end *time.Time) []faceSearchMatch {
	var filtered []faceSearchMatch
	for _, m := range matches {
		if start != nil && m.Timestamp.Before(*start) {
			continue
		}
		if end != nil && m.Timestamp.After(*end) {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

func filterHealthy(nodes []NodeInfo) []NodeInfo {
	var healthy []NodeInfo
	for _, n := range nodes {
		if n.Healthy {
			healthy = append(healthy, n)
		}
	}
	return healthy
}
