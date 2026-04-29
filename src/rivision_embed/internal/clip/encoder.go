package clip

import (
"fmt"
"log"
"os"
"sync"

ort "github.com/yalue/onnxruntime_go"
)

type EncoderConfig struct {
TextModelPath   string
VisionModelPath string
Threads         int
OrtLibPath      string
}

type Encoder struct {
mu            sync.Mutex
textSession   *ort.DynamicAdvancedSession
visionSession *ort.DynamicAdvancedSession
textDim       int
visionDim     int
initialized   bool
}

func NewEncoder(cfg EncoderConfig) (*Encoder, error) {
libPath := cfg.OrtLibPath
if libPath == "" {
libPath = getOrtLibPath()
}

// 检查是否在 K3 平台上运行（支持 A100 NPU 加速）
if SpaceMITEPAvailable() {
log.Printf("[CLIP] 检测到 K3 平台, 尝试初始化 SpaceMIT A100 NPU...")
if InitSpaceMITNPU() {
log.Printf("[CLIP] ✓ SpaceMIT A100 NPU 加速已启用")
} else {
log.Printf("[CLIP] SpaceMIT NPU 初始化失败, 将使用 CPU 推理")
}
} else {
log.Printf("[CLIP] 运行在非 K3 平台, 使用 CPU 推理")
}

ort.SetSharedLibraryPath(libPath)
if err := ort.InitializeEnvironment(); err != nil {
return nil, fmt.Errorf("failed to initialize ONNX Runtime: %w", err)
}
log.Printf("[CLIP] ONNX Runtime 已初始化, 库路径: %s", libPath)

opts, err := ort.NewSessionOptions()
if err != nil {
return nil, fmt.Errorf("failed to create session options: %w", err)
}
defer opts.Destroy()

// ★ 关键: 在 K3 平台上附加 SpaceMIT A100 NPU EP
if IsSpaceMITReady() {
	// 根据官方文档: ORT 线程设为 1，完全由 EP 控制线程
	if err := opts.SetIntraOpNumThreads(1); err != nil {
		return nil, fmt.Errorf("failed to set thread count: %w", err)
	}
	optsPtr := GetSessionOptionsPtr(opts)
	if optsPtr != nil {
		if err := AttachSpaceMITEP(optsPtr, cfg.Threads); err != nil {
			log.Printf("[CLIP] ⚠️ SpaceMIT EP attach failed: %v, using CPU", err)
			// 回退到 CPU 模式，恢复线程数
			opts.SetIntraOpNumThreads(cfg.Threads)
		} else {
			log.Printf("[CLIP] ✓ SpaceMIT A100 NPU EP attached (target=%d cores)", cfg.Threads)
		}
	}
} else {
	// 非 K3 平台，使用 CPU 推理
	if err := opts.SetIntraOpNumThreads(cfg.Threads); err != nil {
		return nil, fmt.Errorf("failed to set thread count: %w", err)
	}
}

// Text model: input_ids, attention_mask -> text_features
textSession, err := ort.NewDynamicAdvancedSession(cfg.TextModelPath,
[]string{"input_ids", "attention_mask"},
[]string{"text_features"},
opts)
if err != nil {
return nil, fmt.Errorf("failed to load text model: %w", err)
}

// Vision model: pixel_values -> image_features
visionSession, err := ort.NewDynamicAdvancedSession(cfg.VisionModelPath,
[]string{"pixel_values"},
[]string{"image_features"},
opts)
if err != nil {
textSession.Destroy()
return nil, fmt.Errorf("failed to load vision model: %w", err)
}

return &Encoder{
textSession:   textSession,
visionSession: visionSession,
textDim:       512,
visionDim:     512,
initialized:   true,
}, nil
}

func (e *Encoder) Close() error {
e.mu.Lock()
defer e.mu.Unlock()

if e.textSession != nil {
e.textSession.Destroy()
}
if e.visionSession != nil {
e.visionSession.Destroy()
}
ort.DestroyEnvironment()
return nil
}

func (e *Encoder) TextDim() int       { return e.textDim }
func (e *Encoder) VisionDim() int     { return e.visionDim }
func (e *Encoder) IsReady() bool      { return e.initialized }
func (e *Encoder) ModelName() string  { return "chinese-clip-vit-b-16" }

func getOrtLibPath() string {
if envPath := os.Getenv("ORT_LIB_PATH"); envPath != "" {
return envPath
}
paths := []string{
"./lib/libonnxruntime.so",
"/opt/rivision/lib/libonnxruntime.so",
"/usr/local/lib/libonnxruntime.so",
"/usr/lib/libonnxruntime.so",
}
for _, p := range paths {
if _, err := os.Stat(p); err == nil {
return p
}
}
return "libonnxruntime.so"
}
