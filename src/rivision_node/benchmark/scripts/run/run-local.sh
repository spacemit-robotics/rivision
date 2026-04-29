#!/bin/bash
# RiVision Benchmark - 场景1: 本地最大性能测试
# Usage: ./run-local.sh [model] [source] [type]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PROJECT_ROOT="$(cd "$BENCHMARK_DIR/../../../.." && pwd)"

# 参数
MODEL="${1:-yolov8n}"
SOURCE="${2:-}"
TYPE="${3:-yolo}"  # yolo 或 vlm

# 颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

# 检测平台
detect_platform() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64) echo "x86_64" ;;
        riscv64) echo "riscv64" ;;
        aarch64) echo "arm64" ;;
        *) echo "unknown" ;;
    esac
}

PLATFORM=$(detect_platform)
info "Platform: $PLATFORM"

# 选择二进制文件
BENCHMARK_BIN="$BENCHMARK_DIR/bin/rivision-benchmark.$PLATFORM"
if [ ! -f "$BENCHMARK_BIN" ]; then
    BENCHMARK_BIN="$BENCHMARK_DIR/build/rivision-benchmark"
fi

if [ ! -f "$BENCHMARK_BIN" ]; then
    warn "Benchmark binary not found, building..."
    "$BENCHMARK_DIR/scripts/build/build.sh" auto
fi

# 默认数据源
if [ -z "$SOURCE" ]; then
    # 尝试查找测试图片
    for img in "$PROJECT_ROOT/test.jpg" \
               "$PROJECT_ROOT/models/test.jpg" \
               "$BENCHMARK_DIR/../models/test.jpg"; do
        if [ -f "$img" ]; then
            SOURCE="$img"
            break
        fi
    done
fi

if [ -z "$SOURCE" ] || [ ! -e "$SOURCE" ]; then
    echo "Error: No data source specified or found"
    echo "Usage: $0 [model] [source] [type]"
    echo "  model:  yolov8n, yolov8s, qwen3vl-4b, etc."
    echo "  source: image file, video file, or image folder"
    echo "  type:   yolo or vlm"
    exit 1
fi

# 输出目录
OUTPUT_DIR="$BENCHMARK_DIR/results/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUTPUT_DIR"

info "Model: $MODEL"
info "Source: $SOURCE"
info "Type: $TYPE"
info "Output: $OUTPUT_DIR"

# 构建命令
CMD="$BENCHMARK_BIN"

if [ "$TYPE" = "yolo" ]; then
    # YOLO 本地测试
    CMD="$CMD yolo-local"
    CMD="$CMD --image $SOURCE"
    
    # 模型路径
    MODEL_FILE="$PROJECT_ROOT/src/rivision_node/models/shared/${MODEL}.onnx"
    if [ -f "$MODEL_FILE" ]; then
        CMD="$CMD --model $MODEL_FILE"
    else
        warn "Model file not found: $MODEL_FILE"
    fi
    
    CMD="$CMD --runs 100"
    CMD="$CMD --warmup 10"
    
elif [ "$TYPE" = "vlm" ]; then
    # VLM 本地测试
    CMD="$CMD vlm-local"
    CMD="$CMD --image $SOURCE"
    
    # 模型路径
    MODEL_DIR="$PROJECT_ROOT/src/rivision_node/models/shared/Qwen3VL"
    if [ -d "$MODEL_DIR" ]; then
        CMD="$CMD --model-dir $MODEL_DIR"
    fi
    
    CMD="$CMD --prompt '简要描述图片内容'"
    CMD="$CMD --runs 10"
    CMD="$CMD --warmup 2"
fi

CMD="$CMD --output $OUTPUT_DIR"

echo ""
info "Running command:"
echo "$CMD"
echo ""

# 执行
eval "$CMD"

echo ""
info "Results saved to: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR"
