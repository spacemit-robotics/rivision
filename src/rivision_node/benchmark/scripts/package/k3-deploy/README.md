# RiVision Benchmark - K3 RISC-V 部署包

## 快速开始

```bash
# 1. 解压
tar -xzf rivision-benchmark-k3.tar.gz
cd rivision-benchmark-k3

# 2. 查看平台信息
./rivision-benchmark platform

# 3. 运行 YOLO 测试
./run-benchmark.sh yolo

# 4. 运行所有测试
./run-benchmark.sh all -i data/test.jpg
```

## 目录结构

```
rivision-benchmark-k3/
├── rivision-benchmark      # 主程序启动器 (自动设置环境)
├── yolo-server             # YOLO HTTP 服务器启动器
├── run-benchmark.sh        # 多模型测试脚本
├── README.md
│
├── bin/                    # 二进制文件
│   ├── rivision-benchmark.riscv64
│   ├── yolo-server.riscv64
│   └── llama-mtmd-cli
│
├── lib/                    # 依赖库 (自动加载，无需配置)
│   ├── libonnxruntime.so*  # ONNX Runtime + SpacemiT NPU
│   ├── libspacemit_ep.so*  # SpacemiT 执行提供者
│   ├── libopencv_*.so*     # OpenCV 4.14
│   ├── libllama.so*        # llama.cpp
│   ├── libggml*.so*        # GGML
│   └── libcurl.so*         # CURL
│
├── models/                 # 模型文件
│   ├── yolo/               # YOLO 模型
│   │   ├── 910byolo11n_b2.q.onnx  # NPU 优化 (推荐)
│   │   └── yolov8n.onnx           # CPU 通用
│   └── vlm/                # VLM 模型 (需自行添加)
│       └── (放置 .gguf 文件)
│
├── data/                   # 测试数据
│   └── test.jpg            # 测试图片
│
├── config/                 # 配置文件
│   ├── benchmark.yaml      # 测试配置
│   └── riscv_b.yaml        # 平台配置
│
└── results/                # 测试结果输出
```

## 添加模型

### YOLO 模型

将 `.onnx` 文件放入 `models/yolo/` 目录：
- **NPU 优化模型**: 文件名包含 `910b` 或 `.q.` 将自动使用 NPU
- **CPU 模型**: 其他模型默认使用 CPU

### VLM 模型

将 `.gguf` 文件放入 `models/vlm/` 目录：

```bash
# 下载推荐模型 (约 2GB)
wget https://huggingface.co/Qwen/Qwen2.5-VL-3B-GGUF/resolve/main/Qwen2.5-VL-3B-Q4_K_M.gguf \
    -O models/vlm/Qwen2.5-VL-3B-Q4_K_M.gguf
```

## 命令参考

### 主程序

```bash
# 平台检测
./rivision-benchmark platform

# YOLO 本地测试
./rivision-benchmark yolo-local \
    --model models/yolo/910byolo11n_b2.q.onnx \
    --backend ort_npu \
    --image data/test.jpg \
    --runs 100

# VLM 本地测试
./rivision-benchmark vlm-local \
    --model models/vlm/model.gguf \
    --image data/test.jpg \
    --prompt "描述这张图片"

# YOLO HTTP 测试
./rivision-benchmark yolo-http \
    --url http://localhost:8080 \
    --image data/test.jpg \
    --concurrency 4
```

### 批量测试脚本

```bash
# 显示帮助
./run-benchmark.sh --help

# 列出可用模型
./run-benchmark.sh list-models

# 运行 YOLO 测试 (所有模型)
./run-benchmark.sh yolo -r 100

# 运行 VLM 测试
./run-benchmark.sh vlm -i data/test.jpg

# 运行全部测试
./run-benchmark.sh all -i data/test.jpg -r 50

# 自定义输出目录
./run-benchmark.sh all -o /path/to/output
```

### YOLO HTTP 服务器

```bash
# 启动服务器
./yolo-server --model models/yolo/910byolo11n_b2.q.onnx --port 8080

# 后台运行
./yolo-server --model models/yolo/910byolo11n_b2.q.onnx &

# 测试 API
curl -X POST http://localhost:8080/detect \
    -F "image=@data/test.jpg"
```

## 配置文件

编辑 `config/benchmark.yaml` 自定义测试参数：

```yaml
yolo:
  default_runs: 100      # 每个模型运行次数
  warmup_runs: 5         # 预热次数
  
vlm:
  default_runs: 5
  default_prompt: "描述这张图片"
  max_tokens: 256
```

## 测试报告

测试结果保存在 `results/` 目录：

```
results/
├── benchmark_report_20260417_130000.txt
├── yolo_results.json
└── vlm_results.json
```

## 故障排除

### 程序无法运行

```bash
# 检查文件权限
chmod +x rivision-benchmark yolo-server run-benchmark.sh

# 检查库加载
./rivision-benchmark platform
```

### NPU 不可用

```bash
# 检查 SpacemiT NPU 驱动
ls /dev/spacemit*
dmesg | grep spacemit

# 回退到 CPU
./rivision-benchmark yolo-local --backend ort_cpu ...
```

### 性能优化

```bash
# 设置 CPU 性能模式
echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
```

## 系统要求

- **操作系统**: Linux (RISC-V 64-bit)
- **内存**: ≥2GB (VLM 需要更多)
- **存储**: ≥500MB (不含模型)
- **依赖**: 无 (所有库已打包)
