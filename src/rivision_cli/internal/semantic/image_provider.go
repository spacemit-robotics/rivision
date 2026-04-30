// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ImageEmbeddingProvider interface {
	Name() string
	Ready(ctx context.Context) error
	EmbedImage(ctx context.Context, imageBase64 string) ([]float64, error)
}

type LocalImageEmbeddingProvider struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewLocalImageEmbeddingProvider(baseURL, apiKey string) *LocalImageEmbeddingProvider {
	return &LocalImageEmbeddingProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (p *LocalImageEmbeddingProvider) Name() string { return "local_image_embed" }

func (p *LocalImageEmbeddingProvider) Ready(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/health", nil)
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("health status=%d", resp.StatusCode)
	}
	return nil
}

func (p *LocalImageEmbeddingProvider) EmbedImage(ctx context.Context, imageBase64 string) ([]float64, error) {
	payload := map[string]any{"image": imageBase64}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/image/embeddings", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("image embeddings status=%d", resp.StatusCode)
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty image embedding")
	}
	return out.Data[0].Embedding, nil
}
