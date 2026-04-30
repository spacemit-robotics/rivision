// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

package clip

import (
"fmt"
"math"
"strings"

ort "github.com/yalue/onnxruntime_go"
)

const (
MaxSeqLength = 52
PadTokenID   = 0
ClsTokenID   = 101
SepTokenID   = 102
)

func (e *Encoder) EmbedText(text string) ([]float32, error) {
e.mu.Lock()
defer e.mu.Unlock()

// Tokenize text
inputIDs, attentionMask := tokenize(text)

// Create input tensors
inputIDsTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(inputIDs))), inputIDs)
if err != nil {
return nil, fmt.Errorf("failed to create input_ids tensor: %w", err)
}
defer inputIDsTensor.Destroy()

attentionMaskTensor, err := ort.NewTensor(ort.NewShape(1, int64(len(attentionMask))), attentionMask)
if err != nil {
return nil, fmt.Errorf("failed to create attention_mask tensor: %w", err)
}
defer attentionMaskTensor.Destroy()

// Create output tensor
outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(e.textDim)))
if err != nil {
return nil, fmt.Errorf("failed to create output tensor: %w", err)
}
defer outputTensor.Destroy()

// Run inference with only input_ids and attention_mask
err = e.textSession.Run(
[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor},
[]ort.ArbitraryTensor{outputTensor},
)
if err != nil {
return nil, fmt.Errorf("text inference failed: %w", err)
}

embedding := outputTensor.GetData()
return l2Normalize(embedding), nil
}

func tokenize(text string) ([]int64, []int64) {
text = strings.TrimSpace(text)

inputIDs := make([]int64, MaxSeqLength)
attentionMask := make([]int64, MaxSeqLength)

// [CLS] token
inputIDs[0] = ClsTokenID
attentionMask[0] = 1

idx := 1
for _, r := range text {
if idx >= MaxSeqLength-1 {
break
}
tokenID := int64(r)
if tokenID > 127 {
tokenID = (tokenID % 20000) + 1000
}
inputIDs[idx] = tokenID
attentionMask[idx] = 1
idx++
}

// [SEP] token
if idx < MaxSeqLength {
inputIDs[idx] = SepTokenID
attentionMask[idx] = 1
idx++
}

// Padding
for i := idx; i < MaxSeqLength; i++ {
inputIDs[i] = PadTokenID
attentionMask[i] = 0
}

return inputIDs, attentionMask
}

func l2Normalize(v []float32) []float32 {
var sum float64
for _, x := range v {
sum += float64(x) * float64(x)
}
norm := math.Sqrt(sum)
if norm < 1e-12 {
norm = 1e-12
}
result := make([]float32, len(v))
for i, x := range v {
result[i] = float32(float64(x) / norm)
}
return result
}
