// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
"flag"
"fmt"
"log"
"net/http"
"os"
"os/signal"
"syscall"

"rivision_embed/internal/api"
"rivision_embed/internal/clip"
)

var (
version   = "dev"
buildTime = "unknown"
)

func main() {
var (
host        string
port        int
textModel   string
visionModel string
threads     int
showVersion bool
debug       bool
)

flag.StringVar(&host, "host", "127.0.0.1", "Server host")
flag.IntVar(&port, "port", 18081, "Server port")
flag.StringVar(&textModel, "text-model", "", "Path to Chinese-CLIP text encoder ONNX model")
flag.StringVar(&visionModel, "vision-model", "", "Path to Chinese-CLIP vision encoder ONNX model")
flag.IntVar(&threads, "threads", 8, "Number of inference threads (K3 A100 NPU: 8)")
flag.BoolVar(&showVersion, "version", false, "Show version")
flag.BoolVar(&debug, "debug", false, "Enable debug logging for embedding requests")
flag.Parse()

if showVersion {
fmt.Printf("rivision-embed-server %s (built %s)\n", version, buildTime)
os.Exit(0)
}

if textModel == "" || visionModel == "" {
log.Fatal("Both --text-model and --vision-model are required")
}

// Initialize CLIP encoder
encoder, err := clip.NewEncoder(clip.EncoderConfig{
TextModelPath:   textModel,
VisionModelPath: visionModel,
Threads:         threads,
})
if err != nil {
log.Fatalf("Failed to initialize CLIP encoder: %v", err)
}
defer encoder.Close()

// Create HTTP server
handler := api.NewHandler(encoder, debug)
mux := http.NewServeMux()

// API routes
mux.HandleFunc("/health", handler.Health)
mux.HandleFunc("/v1/models", handler.Models)
mux.HandleFunc("/v1/embeddings/text", handler.EmbedText)
mux.HandleFunc("/v1/embeddings/image", handler.EmbedImage)

addr := fmt.Sprintf("%s:%d", host, port)
server := &http.Server{
Addr:    addr,
Handler: mux,
}

// Graceful shutdown
go func() {
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh
log.Println("Shutting down server...")
server.Close()
}()

log.Printf("RiVision Embed Server starting on %s", addr)
log.Printf("  Text model: %s", textModel)
log.Printf("  Vision model: %s", visionModel)
log.Printf("  Threads: %d", threads)

if err := server.ListenAndServe(); err != http.ErrServerClosed {
log.Fatalf("Server error: %v", err)
}
}
