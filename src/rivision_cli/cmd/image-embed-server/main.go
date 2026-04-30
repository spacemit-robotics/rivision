// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net/http"
	"os"
)

type embedReq struct {
	Image string `json:"image"`
}

type embedResp struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func main() {
	host := flag.String("host", "127.0.0.1", "listen host")
	port := flag.Int("port", 18081, "listen port")
	model := flag.String("model", "", "onnx model path (reserved)")
	flag.Parse()

	if *model != "" {
		if _, err := os.Stat(*model); err != nil {
			log.Printf("[image-embed] warning: model not accessible: %v", err)
		}
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	http.HandleFunc("/v1/image/embeddings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req embedReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		b, err := base64.StdEncoding.DecodeString(req.Image)
		if err != nil || len(b) == 0 {
			http.Error(w, "invalid image base64", http.StatusBadRequest)
			return
		}
		emb := imageHashEmbedding(b, 384)
		var out embedResp
		out.Data = append(out.Data, struct {
			Embedding []float64 `json:"embedding"`
		}{Embedding: emb})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("[image-embed] serving on %s", addr)
	if *model != "" {
		log.Printf("[image-embed] model=%s", *model)
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}

func imageHashEmbedding(data []byte, dim int) []float64 {
	if dim <= 0 {
		dim = 384
	}
	v := make([]float64, dim)
	chunk := 256
	for i := 0; i < len(data); i += chunk {
		end := i + chunk
		if end > len(data) {
			end = len(data)
		}
		h := fnv.New64a()
		_, _ = h.Write(data[i:end])
		n := h.Sum64()
		idx := int(n % uint64(dim))
		val := float64((n%1000)+1) / 1000.0
		if ((n >> 63) & 1) == 1 {
			val = -val
		}
		v[idx] += val
	}
	l2Normalize(v)
	return v
}

func l2Normalize(v []float64) {
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	if norm <= 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
}
