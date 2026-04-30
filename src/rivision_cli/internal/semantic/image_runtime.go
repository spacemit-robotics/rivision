package semantic

import (
	"context"
	"time"
)

func RuntimeEmbedImage(imageBase64 string) ([]float64, bool) {
	runtimeState.RLock()
	p := runtimeState.imageProvider
	runtimeState.RUnlock()
	if p == nil {
		return nil, false
	}
	// 使用超时避免长时间阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := p.EmbedImage(ctx, imageBase64)
	if err != nil || len(v) == 0 {
		runtimeState.Lock()
		runtimeState.imageLastError = "embed image failed"
		runtimeState.status.ImageProviderErr = runtimeState.imageLastError
		runtimeState.Unlock()
		return nil, false
	}
	runtimeState.Lock()
	runtimeState.imageLastError = ""
	runtimeState.status.ImageProviderErr = ""
	runtimeState.Unlock()
	return v, true
}
