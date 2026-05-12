package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// VideoSearcher handles video search: extract keyframes → batch embed → federated search (§5.8).
type VideoSearcher struct {
	embedURL   string
	federated  *FederatedSearch
	httpClient *http.Client
}

// NewVideoSearcher creates a video search handler.
func NewVideoSearcher(embedURL string, federated *FederatedSearch) *VideoSearcher {
	return &VideoSearcher{
		embedURL:   embedURL,
		federated:  federated,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SearchByVideo extracts keyframes from the uploaded video, embeds each frame,
// and performs a multi-vector federated search across Workers.
func (vs *VideoSearcher) SearchByVideo(videoData []byte, topK int, getNodes func() []NodeInfo) ([]AggregatedResult, error) {
	// Step 1: Extract keyframes (simplified — in production, use ffprobe + ffmpeg)
	// For now, treat the entire video as a single frame query
	frames := [][]byte{videoData}

	// Step 2: Embed each frame via Hub embed service
	var allVecs [][]float32
	for _, frame := range frames {
		vec, err := vs.embedFrame(frame)
		if err != nil {
			continue
		}
		allVecs = append(allVecs, vec)
	}

	if len(allVecs) == 0 {
		return nil, fmt.Errorf("no frames could be embedded")
	}

	// Step 3: Federated search with each vector, merge results
	aggregator := NewAggregator(topK)
	var allResults []AggregatedResult

	for _, vec := range allVecs {
		results, err := vs.federated.SearchByVector(vec, topK, getNodes())
		if err != nil {
			continue
		}
		for _, r := range results {
			allResults = append(allResults, AggregatedResult{
				DetectionID: r.DetectionID,
				CameraID:    r.CameraID,
				NodeID:      r.NodeID,
				Score:       r.Score,
				Timestamp:   r.Timestamp.Unix(),
			})
		}
	}

	return aggregator.Aggregate(allResults, nil), nil
}

// embedFrame sends a single image frame to the embed service.
func (vs *VideoSearcher) embedFrame(frameData []byte) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"image": frameData,
	})
	req, err := http.NewRequest(http.MethodPost, vs.embedURL+"/v1/embeddings/image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := vs.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed frame: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed frame: status %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}
