// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
	"hash/fnv"
	"math"
	"strings"
)

// EmbedText 统一文本向量化入口。
// 优先使用 runtime provider (rivision-embed/remote)，回退到 hash embedding。
func EmbedText(text string, dim int) []float64 {
	if dim <= 0 {
		dim = 384
	}
	if v, ok := runtimeEmbedText(text, dim); ok && len(v) > 0 {
		return v
	}
	return hashEmbed(text, dim)
}

func hashEmbed(text string, dim int) []float64 {
	v := make([]float64, dim)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return v
	}
	for _, tk := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(tk))
		n := h.Sum64()
		idx := int(n % uint64(dim))
		sign := 1.0
		if ((n >> 63) & 1) == 1 {
			sign = -1
		}
		v[idx] += sign
	}
	l2Normalize(v)
	return v
}

func l2Normalize(v []float64) {
	var norm float64
	for i := range v {
		norm += v[i] * v[i]
	}
	if norm <= 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
}

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts)*2)
	for _, p := range parts {
		r := []rune(p)
		if len(r) <= 2 {
			out = append(out, p)
			continue
		}
		for i := 0; i < len(r)-1; i++ {
			out = append(out, string(r[i:i+2]))
		}
	}
	if len(out) == 0 {
		out = append(out, s)
	}
	return out
}
