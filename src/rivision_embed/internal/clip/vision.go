package clip

import (
"bytes"
"encoding/base64"
"fmt"
"image"
"image/jpeg"
"image/png"
"strings"

ort "github.com/yalue/onnxruntime_go"
"golang.org/x/image/draw"
)

const (
ImageSize = 224
// ImageNet normalization values
MeanR = 0.48145466
MeanG = 0.4578275
MeanB = 0.40821073
StdR  = 0.26862954
StdG  = 0.26130258
StdB  = 0.27577711
)

func (e *Encoder) EmbedImage(imageBase64 string) ([]float32, error) {
e.mu.Lock()
defer e.mu.Unlock()

// Strip data URL prefix if present (e.g., "data:image/jpeg;base64,")
if idx := strings.Index(imageBase64, ","); idx != -1 && strings.HasPrefix(imageBase64, "data:") {
imageBase64 = imageBase64[idx+1:]
}
imageBase64 = strings.TrimSpace(imageBase64)

// Decode base64
imageData, err := base64.StdEncoding.DecodeString(imageBase64)
if err != nil {
// Try with padding
imageBase64 = strings.TrimSpace(imageBase64)
switch len(imageBase64) % 4 {
case 2:
imageBase64 += "=="
case 3:
imageBase64 += "="
}
imageData, err = base64.StdEncoding.DecodeString(imageBase64)
if err != nil {
return nil, fmt.Errorf("failed to decode base64: %w", err)
}
}

// Decode image
img, err := decodeImage(imageData)
if err != nil {
return nil, fmt.Errorf("failed to decode image: %w", err)
}

// Preprocess image
pixelValues := preprocessImage(img)

// Create input tensor [1, 3, 224, 224]
inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, ImageSize, ImageSize), pixelValues)
if err != nil {
return nil, fmt.Errorf("failed to create input tensor: %w", err)
}
defer inputTensor.Destroy()

// Create output tensor
outputTensor, err := ort.NewEmptyTensor[float32](ort.NewShape(1, int64(e.visionDim)))
if err != nil {
return nil, fmt.Errorf("failed to create output tensor: %w", err)
}
defer outputTensor.Destroy()

// Run inference
err = e.visionSession.Run(
[]ort.ArbitraryTensor{inputTensor},
[]ort.ArbitraryTensor{outputTensor},
)
if err != nil {
return nil, fmt.Errorf("vision inference failed: %w", err)
}

// Get output and normalize
embedding := outputTensor.GetData()
normalized := l2Normalize(embedding)
return normalized, nil
}

func decodeImage(data []byte) (image.Image, error) {
reader := bytes.NewReader(data)

// Try JPEG
img, err := jpeg.Decode(reader)
if err == nil {
return img, nil
}

// Try PNG
reader.Reset(data)
img, err = png.Decode(reader)
if err == nil {
return img, nil
}

// Try generic decode
reader.Reset(data)
img, _, err = image.Decode(reader)
if err != nil {
return nil, fmt.Errorf("unsupported image format: %w", err)
}
return img, nil
}

func preprocessImage(img image.Image) []float32 {
// Resize to 224x224
resized := resizeImage(img, ImageSize, ImageSize)

// Convert to CHW format with normalization
pixels := make([]float32, 3*ImageSize*ImageSize)

for y := 0; y < ImageSize; y++ {
for x := 0; x < ImageSize; x++ {
r, g, b, _ := resized.At(x, y).RGBA()
// Normalize to [0, 1] then apply ImageNet normalization
rf := (float32(r>>8) / 255.0 - MeanR) / StdR
gf := (float32(g>>8) / 255.0 - MeanG) / StdG
bf := (float32(b>>8) / 255.0 - MeanB) / StdB

// CHW format
idx := y*ImageSize + x
pixels[0*ImageSize*ImageSize+idx] = rf // R channel
pixels[1*ImageSize*ImageSize+idx] = gf // G channel
pixels[2*ImageSize*ImageSize+idx] = bf // B channel
}
}
return pixels
}

func resizeImage(img image.Image, width, height int) *image.RGBA {
dst := image.NewRGBA(image.Rect(0, 0, width, height))
draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
return dst
}
