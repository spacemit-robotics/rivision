// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
"bytes"
"context"
"encoding/json"
"fmt"
"io"
"net/http"
"sync"
"time"
)

// EmbedProvider 调用 rivision-embed 服务进行文本和图像向量化
type EmbedProvider struct {
baseURL string
apiKey  string
http    *http.Client
mu      sync.RWMutex
ready   bool
dim     int
}

// NewEmbedProvider 创建 Embed Provider
func NewEmbedProvider(baseURL, apiKey string) *EmbedProvider {
return &EmbedProvider{
baseURL: baseURL,
apiKey:  apiKey,
http: &http.Client{
Timeout: 30 * time.Second,
},
dim: 512, // Chinese-CLIP 默认 512 维
}
}

func (p *EmbedProvider) Name() string {
return "embed"
}

// Ready 检查服务是否可用
func (p *EmbedProvider) Ready(ctx context.Context) error {
req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/health", nil)
if err != nil {
return err
}

resp, err := p.http.Do(req)
if err != nil {
p.mu.Lock()
p.ready = false
p.mu.Unlock()
return fmt.Errorf("embed service unavailable: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
p.mu.Lock()
p.ready = false
p.mu.Unlock()
return fmt.Errorf("embed service returned status %d", resp.StatusCode)
}

p.mu.Lock()
p.ready = true
p.mu.Unlock()
return nil
}

// IsReady 返回服务是否就绪
func (p *EmbedProvider) IsReady() bool {
p.mu.RLock()
defer p.mu.RUnlock()
return p.ready
}

// Dim 返回向量维度
func (p *EmbedProvider) Dim() int {
return p.dim
}

// EmbedText 文本向量化
func (p *EmbedProvider) EmbedText(ctx context.Context, text string) ([]float64, error) {
payload := map[string]any{"input": text}
body, err := json.Marshal(payload)
if err != nil {
return nil, err
}

req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/embeddings/text", bytes.NewReader(body))
if err != nil {
return nil, err
}
req.Header.Set("Content-Type", "application/json")
if p.apiKey != "" {
req.Header.Set("Authorization", "Bearer "+p.apiKey)
}

resp, err := p.http.Do(req)
if err != nil {
return nil, fmt.Errorf("embed text request failed: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
respBody, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("embed text failed (%d): %s", resp.StatusCode, string(respBody))
}

var result embeddingResponse
if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
return nil, fmt.Errorf("decode response failed: %w", err)
}

if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
return nil, fmt.Errorf("empty embedding response")
}

// 转换 float32 到 float64
embedding := make([]float64, len(result.Data[0].Embedding))
for i, v := range result.Data[0].Embedding {
embedding[i] = float64(v)
}

// 更新维度
p.mu.Lock()
p.dim = len(embedding)
p.mu.Unlock()

return embedding, nil
}

// EmbedImage 图像向量化
func (p *EmbedProvider) EmbedImage(ctx context.Context, imageBase64 string) ([]float64, error) {
payload := map[string]any{"image": imageBase64}
body, err := json.Marshal(payload)
if err != nil {
return nil, err
}

req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/embeddings/image", bytes.NewReader(body))
if err != nil {
return nil, err
}
req.Header.Set("Content-Type", "application/json")
if p.apiKey != "" {
req.Header.Set("Authorization", "Bearer "+p.apiKey)
}

resp, err := p.http.Do(req)
if err != nil {
return nil, fmt.Errorf("embed image request failed: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode >= 400 {
respBody, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("embed image failed (%d): %s", resp.StatusCode, string(respBody))
}

var result embeddingResponse
if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
return nil, fmt.Errorf("decode response failed: %w", err)
}

if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
return nil, fmt.Errorf("empty embedding response")
}

// 转换 float32 到 float64
embedding := make([]float64, len(result.Data[0].Embedding))
for i, v := range result.Data[0].Embedding {
embedding[i] = float64(v)
}

return embedding, nil
}

func (p *EmbedProvider) Close() error {
return nil
}

// embeddingResponse API 响应结构
type embeddingResponse struct {
Object string `json:"object"`
Model  string `json:"model"`
Data   []struct {
Index     int       `json:"index"`
Embedding []float32 `json:"embedding"`
Dim       int       `json:"dim"`
} `json:"data"`
Usage struct {
PromptTokens int   `json:"prompt_tokens,omitempty"`
TotalTokens  int   `json:"total_tokens,omitempty"`
LatencyMs    int64 `json:"latency_ms,omitempty"`
} `json:"usage,omitempty"`
}

// embedProviderAdapter 适配 EmbedProvider 到 embeddingProvider 接口
type embedProviderAdapter struct {
*EmbedProvider
}

func (a *embedProviderAdapter) EmbedText(ctx context.Context, text string, dim int) ([]float64, error) {
return a.EmbedProvider.EmbedText(ctx, text)
}
