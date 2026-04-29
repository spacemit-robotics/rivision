# RiVision Benchmark v2.0

多平台、多模型的 YOLO/VLM 基准测试模块。

## 特性

- **多平台支持**: x86_64 / K3 RISC-V (NPU) / Jetson Orin (GPU)
- **多模型支持**: YOLOv5/v8/v11, Qwen3VL/FastVLM/MiniCPM
- **两种场景**: 本地最大性能 / HTTP 服务性能
- **交叉编译**: 从 x86 交叉编译到 RISC-V 和 ARM64

## 支持平台

| 平台 | 架构 | YOLO 后端 | VLM 后端 |
|------|------|-----------|----------|
| x86 | amd64 | ORT CPU | llama CPU |
| K3 | riscv64 | ORT NPU | llama smt |
| Jetson | arm64 | TensorRT | llama CUDA |

## 支持模型

**YOLO**: yolov5n/s/m, yolov8n/s/m, yolov11n/s/m  
**VLM**: qwen3vl-4b/30b, fastvlm-0.5b, minicpm-v4.5

## 目录结构

```
benchmark/
├── docs/                        # 设计文档
│   └── benchmark-v2-design.md
├── cmake/                       # 交叉编译工具链
│   ├── toolchain-riscv64.cmake
│   └── toolchain-arm64-cuda.cmake
├── profiles/                    # 平台配置
│   ├── x86.yaml
│   ├── riscv_b.yaml
│   └── arm_gpu.yaml
├── models/                      # 模型配置
│   ├── registry.yaml
│   ├── yolo/*.yaml
│   └── vlm/*.yaml
├── include/
│   ├── platform/                # 平台抽象
│   └── models/                  # 模型管理
├── src/
│   ├── platform/                # 平台检测/后端工厂
│   ├── data_source/
│   └── inference_backend/
├── scripts/
│   ├── build/                   # 构建脚本
│   └── run/                     # 运行脚本
└── bin/                         # 编译输出
```

## 测试场景

### 情况1: 本地推理 (最大性能)

直接使用本地模型进行推理，无网络开销：

```bash
# YOLO 本地测试
./scripts/run_yolo_local.sh [图片] [模型] [运行次数] [线程数]

# VLM 本地测试
./scripts/run_vlm_local.sh [图片] [运行次数] [线程数]
```

**特点:**
- 零网络延迟
- 最大化推理吞吐量
- 支持多线程并行
- 适合评估硬件极限性能

### 情况2: HTTP 服务推理

通过 HTTP 调用 yolo-server / llama-server：

```bash
# YOLO HTTP 测试 (并发扫描)
./scripts/run_yolo_http.sh [图片] [运行次数] true

# VLM HTTP 测试
./scripts/run_vlm_http.sh [图片] [运行次数]
```

**特点:**
- 端到端延迟测量
- 并发能力评估
- 模拟真实部署场景
- 连接池优化

## 编译

### 本地编译 (x86)

```bash
./scripts/build/build.sh x86
```

### 交叉编译

```bash
# K3 RISC-V
./scripts/build/build.sh riscv64

# Jetson Orin
./scripts/build/build.sh arm64

# 所有平台
./scripts/build/build.sh all
```

### 依赖

- OpenCV 4.x
- libcurl
- ONNX Runtime (x86/K3)
- TensorRT (Jetson, 可选)
- SpacemiT Toolchain (交叉编译 K3)

## 使用

### 命令行工具

```bash
# YOLO HTTP 测试 (并发扫描)
./rivision-benchmark yolo-http --image test.jpg --url http://localhost:9081 --sweep

# YOLO 本地测试
./rivision-benchmark yolo-local --image test.jpg --model models/yolov8n.onnx --threads 4

# VLM 本地测试
./rivision-benchmark vlm-local --image test.jpg --model models/minicpm.gguf --mmproj smt

# VLM HTTP 测试
./rivision-benchmark vlm-http --image test.jpg --url http://localhost:8080
```

### 选项

| 选项 | 说明 |
|------|------|
| `--image PATH` | 测试图片路径 (必需) |
| `--video PATH` | 测试视频路径 |
| `--model PATH` | 模型文件路径 |
| `--mmproj PATH` | VLM mmproj 路径 (或 "smt" 使用 NPU) |
| `--url URL` | HTTP 服务 URL |
| `--runs N` | 测试次数 (默认 100) |
| `--warmup N` | 预热次数 (默认 5) |
| `--threads N` | 线程数 (默认 4) |
| `--concurrency N` | 并发数 (默认 1) |
| `--sweep` | 并发扫描 (1,2,4,8) |
| `--output DIR` | 输出目录 |
| `--quiet` | 禁用实时显示 |

## 输出格式

### 实时输出

```
[YOLO HTTP c=4] 342/500 (0.0% err) | FPS: 85.3 | Latency: 46.8/62.4/78.9 ms (avg/p95/p99) | CPU: 78% | Mem: 512 MB
```

### 结果报告

```
╔═══════════════════════════════════════════════════════════════════╗
║  YOLO HTTP (max throughput, c=4)                                  ║
╠═══════════════════════════════════════════════════════════════════╣
║  Performance                                                      ║
║    Throughput:          85.30 fps                                 ║
║    Total Frames:          500                                     ║
║    Success Rate:       100.0%                                     ║
║    Duration:             5.86 s                                   ║
║                                                                   ║
║  Latency (ms)                                                     ║
║    Average:             46.82                                     ║
║    Min:                 32.15                                     ║
║    Max:                 98.42                                     ║
║    P50:                 44.21                                     ║
║    P95:                 62.38                                     ║
║    P99:                 78.91                                     ║
╚═══════════════════════════════════════════════════════════════════╝
```

### 多路视频支持能力

```
╔═══════════════════════════════════════════════════════════════════╗
║                    Multi-Stream Capacity                          ║
║  Max Throughput: 85.3 fps                                         ║
║    @ 30 fps/stream:   2 streams supported                         ║
║    @ 15 fps/stream:   5 streams supported                         ║
║    @ 10 fps/stream:   8 streams supported                         ║
║    @  5 fps/stream:  17 streams supported                         ║
╚═══════════════════════════════════════════════════════════════════╝
```

## 测试指标

### YOLO 指标

| 指标 | 说明 |
|------|------|
| FPS | 每秒处理帧数 |
| Latency | 端到端延迟 (ms) |
| Inference Time | 纯推理时间 (ms) |
| Preprocess Time | 预处理时间 (ms) |
| Postprocess Time | 后处理时间 (ms) |
| Network Time | 网络传输时间 (ms, HTTP 模式) |

### VLM 指标

| 指标 | 说明 |
|------|------|
| Vision Encode | 图像编码时间 (ms) |
| Prompt Eval | Prompt 处理时间 (ms) |
| Token Gen | Token 生成时间 (ms) |
| TTFT | 首 Token 延迟 (ms) |
| Tokens/s | 生成速度 (tok/s) |
| Total Time | 总处理时间 (ms) |

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        BenchmarkRunner                          │
│  ┌─────────────┐  ┌────────────────┐  ┌───────────────────────┐│
│  │ DataSource  │→ │InferenceBackend│→ │  MetricsCollector     ││
│  │ - VideoFile │  │ - LocalYOLO    │  │  - Histogram          ││
│  │ - Stream    │  │ - LocalVLM     │  │  - Statistics         ││
│  │ - Image     │  │ - HTTP YOLO    │  │  - ResourceMonitor    ││
│  │ - Folder    │  │ - HTTP VLM     │  │  - Export (json/csv)  ││
│  └─────────────┘  └────────────────┘  └───────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

## 扩展

### 添加新的数据源

继承 `DataSource` 类并实现接口：

```cpp
class MyDataSource : public DataSource {
public:
    bool open(const std::string& source) override;
    bool read(Frame& frame) override;
    void close() override;
    // ...
};
```

### 添加新的推理后端

继承 `InferenceBackend` 类并实现接口：

```cpp
class MyBackend : public InferenceBackend {
public:
    bool init(const BackendConfig& config) override;
    InferenceResult infer(const Frame& frame) override;
    void shutdown() override;
    // ...
};
```

## License

Apache 2.0
