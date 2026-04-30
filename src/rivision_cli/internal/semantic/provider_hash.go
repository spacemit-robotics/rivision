// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
	"context"
)

type HashProvider struct{}

func NewHashProvider() *HashProvider { return &HashProvider{} }

func (p *HashProvider) Name() string { return "hash_hybrid" }

func (p *HashProvider) Ready(ctx context.Context) error { return nil }

func (p *HashProvider) EmbedText(ctx context.Context, text string, dim int) ([]float64, error) {
	return hashEmbed(text, dim), nil
}

func (p *HashProvider) Close() error { return nil }
