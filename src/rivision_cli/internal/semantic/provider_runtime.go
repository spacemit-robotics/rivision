package semantic

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

type RuntimeOptions struct {
	RemoteURL    string
	RemoteAPIKey string
	EnableRemote bool
	EmbedURL     string
	EmbedAPIKey  string
}

type ProviderStatus struct {
	ActiveProvider   string   `json:"active_provider"`
	ProviderChain    []string `json:"provider_chain"`
	Degraded         bool     `json:"degraded"`
	LastError        string   `json:"last_error,omitempty"`
	Architecture     string   `json:"architecture"`
	ModelPath        string   `json:"model_path,omitempty"`
	ImageProvider    string   `json:"image_provider,omitempty"`
	ImageProviderErr string   `json:"image_provider_error,omitempty"`
}

type embeddingProvider interface {
	Name() string
	Ready(ctx context.Context) error
	EmbedText(ctx context.Context, text string, dim int) ([]float64, error)
	Close() error
}

var runtimeState struct {
	sync.RWMutex
	providers         []embeddingProvider
	activeIdx         int
	status            ProviderStatus
	imageProvider     ImageEmbeddingProvider
	imageProviderName string
	imageLastError    string
}

func ConfigureProviderRuntime(opts RuntimeOptions) {
	runtimeState.Lock()
	defer runtimeState.Unlock()

	for _, p := range runtimeState.providers {
		_ = p.Close()
	}
	runtimeState.providers = nil
	runtimeState.activeIdx = 0
	runtimeState.status = ProviderStatus{
		Architecture: runtime.GOARCH,
	}

	// EmbedProvider (rivision-embed / Chinese-CLIP) - 优先使用
	if strings.TrimSpace(opts.EmbedURL) != "" {
		embedProvider := NewEmbedProvider(opts.EmbedURL, opts.EmbedAPIKey)
		runtimeState.providers = append(runtimeState.providers, &embedProviderAdapter{embedProvider})
		// 同时设置为 image provider
		runtimeState.imageProvider = embedProvider
		runtimeState.imageProviderName = embedProvider.Name()
		if err := embedProvider.Ready(context.Background()); err != nil {
			runtimeState.imageLastError = err.Error()
		}
		runtimeState.status.ImageProvider = runtimeState.imageProviderName
		runtimeState.status.ImageProviderErr = runtimeState.imageLastError
	}

	if opts.EnableRemote && opts.RemoteURL != "" {
		runtimeState.providers = append(runtimeState.providers, NewRemoteProvider(opts.RemoteURL, opts.RemoteAPIKey))
	}

	runtimeState.providers = append(runtimeState.providers, NewHashProvider())
	runtimeState.status.ProviderChain = make([]string, 0, len(runtimeState.providers))
	for _, p := range runtimeState.providers {
		runtimeState.status.ProviderChain = append(runtimeState.status.ProviderChain, p.Name())
	}

	activateFirstReadyLocked()
}

func activateFirstReadyLocked() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for i, p := range runtimeState.providers {
		if err := p.Ready(ctx); err == nil {
			runtimeState.activeIdx = i
			runtimeState.status.ActiveProvider = p.Name()
			runtimeState.status.Degraded = i > 0
			runtimeState.status.LastError = ""
			return
		} else {
			runtimeState.status.LastError = err.Error()
		}
	}
	if len(runtimeState.providers) > 0 {
		runtimeState.activeIdx = len(runtimeState.providers) - 1
		runtimeState.status.ActiveProvider = runtimeState.providers[runtimeState.activeIdx].Name()
		runtimeState.status.Degraded = true
	}
}

func ProviderStatusSnapshot() ProviderStatus {
	runtimeState.RLock()
	defer runtimeState.RUnlock()
	return runtimeState.status
}

func runtimeEmbedText(text string, dim int) ([]float64, bool) {
	// 先获取 providers 快照，避免长时间持有锁
	runtimeState.RLock()
	if len(runtimeState.providers) == 0 {
		runtimeState.RUnlock()
		return nil, false
	}
	providers := make([]embeddingProvider, len(runtimeState.providers))
	copy(providers, runtimeState.providers)
	runtimeState.RUnlock()

	// 使用较短的超时，避免长时间阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 在锁外执行 HTTP 调用，避免阻塞其他操作
	for i := 0; i < len(providers); i++ {
		v, err := providers[i].EmbedText(ctx, text, dim)
		if err == nil {
			// 成功后更新状态
			runtimeState.Lock()
			runtimeState.activeIdx = i
			runtimeState.status.ActiveProvider = providers[i].Name()
			runtimeState.status.Degraded = i > 0
			runtimeState.status.LastError = ""
			runtimeState.Unlock()
			return v, true
		}
		// 记录错误但不阻塞
		runtimeState.Lock()
		runtimeState.status.LastError = fmt.Sprintf("%s: %v", providers[i].Name(), err)
		runtimeState.Unlock()
	}

	return nil, false
}
