package api

import (
"encoding/json"
"log"
"net/http"
"time"

"rivision_embed/internal/clip"
)

type Handler struct {
encoder *clip.Encoder
debug   bool
}

func NewHandler(encoder *clip.Encoder, debug bool) *Handler {
if debug {
log.Println("[Embed] Debug logging enabled")
}
return &Handler{encoder: encoder, debug: debug}
}

// Health check endpoint
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}

status := "ok"
if !h.encoder.IsReady() {
status = "error"
}

resp := map[string]any{
"status":         status,
"model":          h.encoder.ModelName(),
"text_encoder":   "loaded",
"vision_encoder": "loaded",
"timestamp":      time.Now().UTC().Format(time.RFC3339),
}
writeJSON(w, http.StatusOK, resp)
}

// Models endpoint
func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodGet {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}

resp := map[string]any{
"object": "list",
"data": []map[string]any{
{
"id":         h.encoder.ModelName(),
"object":     "model",
"created":    time.Now().Unix(),
"owned_by":   "rivision",
"text_dim":   h.encoder.TextDim(),
"vision_dim": h.encoder.VisionDim(),
},
},
}
writeJSON(w, http.StatusOK, resp)
}

// Text embedding endpoint
func (h *Handler) EmbedText(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodPost {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}

var req struct {
Input string `json:"input"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
return
}

if req.Input == "" {
writeError(w, http.StatusBadRequest, "input is required")
return
}

if h.debug {
log.Printf("[Embed/Text] Input: %q (len=%d)", truncateStr(req.Input, 100), len(req.Input))
}
start := time.Now()
embedding, err := h.encoder.EmbedText(req.Input)
if err != nil {
log.Printf("Text embedding error: %v", err)
writeError(w, http.StatusInternalServerError, "Embedding failed: "+err.Error())
return
}
elapsed := time.Since(start)
if h.debug {
log.Printf("[Embed/Text] OK dim=%d latency=%dms", len(embedding), elapsed.Milliseconds())
}

resp := EmbeddingResponse{
Object: "embedding",
Model:  h.encoder.ModelName(),
Data: []EmbeddingData{
{
Index:     0,
Embedding: embedding,
Dim:       len(embedding),
},
},
Usage: Usage{
PromptTokens: len(req.Input),
TotalTokens:  len(req.Input),
LatencyMs:    elapsed.Milliseconds(),
},
}
writeJSON(w, http.StatusOK, resp)
}

// Image embedding endpoint
func (h *Handler) EmbedImage(w http.ResponseWriter, r *http.Request) {
if r.Method != http.MethodPost {
http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
return
}

var req struct {
Image string `json:"image"`
}
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
return
}

if req.Image == "" {
writeError(w, http.StatusBadRequest, "image is required (base64 encoded)")
return
}

if h.debug {
log.Printf("[Embed/Image] Input base64 len=%d", len(req.Image))
}
start := time.Now()
embedding, err := h.encoder.EmbedImage(req.Image)
if err != nil {
log.Printf("Image embedding error: %v", err)
writeError(w, http.StatusInternalServerError, "Embedding failed: "+err.Error())
return
}
elapsed := time.Since(start)
if h.debug {
log.Printf("[Embed/Image] OK dim=%d latency=%dms", len(embedding), elapsed.Milliseconds())
}

resp := EmbeddingResponse{
Object: "embedding",
Model:  h.encoder.ModelName(),
Data: []EmbeddingData{
{
Index:     0,
Embedding: embedding,
Dim:       len(embedding),
},
},
Usage: Usage{
LatencyMs: elapsed.Milliseconds(),
},
}
writeJSON(w, http.StatusOK, resp)
}

// Response types
type EmbeddingResponse struct {
Object string          `json:"object"`
Model  string          `json:"model"`
Data   []EmbeddingData `json:"data"`
Usage  Usage           `json:"usage,omitempty"`
}

type EmbeddingData struct {
Index     int       `json:"index"`
Embedding []float32 `json:"embedding"`
Dim       int       `json:"dim"`
}

type Usage struct {
PromptTokens int   `json:"prompt_tokens,omitempty"`
TotalTokens  int   `json:"total_tokens,omitempty"`
LatencyMs    int64 `json:"latency_ms,omitempty"`
}

type ErrorResponse struct {
Error struct {
Message string `json:"message"`
Type    string `json:"type"`
} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(status)
json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
resp := ErrorResponse{}
resp.Error.Message = message
resp.Error.Type = "invalid_request_error"
writeJSON(w, status, resp)
}

func truncateStr(s string, maxLen int) string {
runes := []rune(s)
if len(runes) <= maxLen {
return s
}
return string(runes[:maxLen]) + "..."
}
