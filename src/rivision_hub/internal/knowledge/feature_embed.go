package knowledge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FeatureEmbedder encodes feature library photos into vectors via rivision_embed (§5.11).
// Flow: photo → embed_service → feature_vec → store
type FeatureEmbedder struct {
	embedURL   string
	httpClient *http.Client
}

// NewFeatureEmbedder creates a feature embedder targeting the Hub-side embed service.
func NewFeatureEmbedder(embedURL string) *FeatureEmbedder {
	return &FeatureEmbedder{
		embedURL:   embedURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// EmbedPhoto sends a JPEG photo to the embed service and returns a 512-D vector.
func (fe *FeatureEmbedder) EmbedPhoto(jpegData []byte) ([]float32, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"image": jpegData,
	})
	req, err := http.NewRequest(http.MethodPost, fe.embedURL+"/v1/embeddings/image", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := fe.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed photo: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed photo: status %d: %s", resp.StatusCode, data)
	}

	var result struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	return result.Embedding, nil
}
