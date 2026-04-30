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

type RemoteProvider struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewRemoteProvider(baseURL, apiKey string) *RemoteProvider {
	return &RemoteProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   "text-embedding", // 默认模型名
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *RemoteProvider) Name() string { return "remote" }

func (p *RemoteProvider) Ready(ctx context.Context) error {
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

func (p *RemoteProvider) EmbedText(ctx context.Context, text string, dim int) ([]float64, error) {
	payload := map[string]any{"model": p.model, "input": []string{text}}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(b))
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
		return nil, fmt.Errorf("embeddings status=%d", resp.StatusCode)
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
		return nil, fmt.Errorf("empty embedding")
	}
	return out.Data[0].Embedding, nil
}

func (p *RemoteProvider) Close() error { return nil }
