#!/bin/bash
# RiVision Benchmark - 场景2: HTTP 服务性能测试
# Usage: ./run-http.sh [type] [url] [image]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PROJECT_ROOT="$(cd "$BENCHMARK_DIR/../../../.." && pwd)"

# 参数
TYPE="${1:-yolo}"  # yolo 或 vlm
URL="${2:-}"
IMAGE="${3:-}"

# 默认 URL
YOLO_HOST="${YOLO_HOST:-localhost}"
YOLO_PORT="${YOLO_PORT:-8080}"
LLAMA_HOST="${LLAMA_HOST:-localhost}"
LLAMA_PORT="${LLAMA_PORT:-8081}"

if [ -z "$URL" ]; then
    if [ "$TYPE" = "yolo" ]; then
        URL="http://${YOLO_HOST}:${YOLO_PORT}"
    else
        URL="http://${LLAMA_HOST}:${LLAMA_PORT}"
    fi
fi

# 颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# 检测平台和二进制
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
BENCHMARK_BIN="$BENCHMARK_DIR/bin/rivision-benchmark.$PLATFORM"
if [ ! -f "$BENCHMARK_BIN" ]; then
    BENCHMARK_BIN="$BENCHMARK_DIR/build/rivision-benchmark"
fi

if [ ! -f "$BENCHMARK_BIN" ]; then
    warn "Benchmark binary not found, building..."
    "$BENCHMARK_DIR/scripts/build/build.sh" auto
fi

# 默认图片
if [ -z "$IMAGE" ]; then
    for img in "$PROJECT_ROOT/test.jpg" \
               "$PROJECT_ROOT/models/test.jpg" \
               "$BENCHMARK_DIR/../models/test.jpg"; do
        if [ -f "$img" ]; then
            IMAGE="$img"
            break
        fi
    done
fi

if [ -z "$IMAGE" ] || [ ! -f "$IMAGE" ]; then
    error "No test image found. Please specify an image file."
fi

# 检查服务
info "Checking service at $URL..."
if [ "$TYPE" = "yolo" ]; then
    CHECK_URL="${URL}/health"
else
    CHECK_URL="${URL}/health"
fi

if ! curl -s --connect-timeout 3 "$CHECK_URL" > /dev/null 2>&1; then
    error "Service not available at $URL. Please start the service first."
fi
info "Service is available"

# 输出目录
OUTPUT_DIR="$BENCHMARK_DIR/results/$(date +%Y%m%d_%H%M%S)_http"
mkdir -p "$OUTPUT_DIR"

info "Type: $TYPE"
info "URL: $URL"
info "Image: $IMAGE"
info "Output: $OUTPUT_DIR"

# 并发级别
CONCURRENCY_LEVELS="1,2,4,8"

# 构建命令
CMD="$BENCHMARK_BIN"

if [ "$TYPE" = "yolo" ]; then
    CMD="$CMD yolo-http"
    CMD="$CMD --url ${URL}/api/detect/binary"
    CMD="$CMD --image $IMAGE"
    CMD="$CMD --runs 100"
    CMD="$CMD --warmup 10"
    CMD="$CMD --concurrency $CONCURRENCY_LEVELS"
else
    CMD="$CMD vlm-http"
    CMD="$CMD --url ${URL}/v1/chat/completions"
    CMD="$CMD --image $IMAGE"
    CMD="$CMD --prompt '简要描述图片内容'"
    CMD="$CMD --runs 20"
    CMD="$CMD --warmup 2"
    CMD="$CMD --concurrency 1,2"
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

# 打印摘要
if [ -f "$OUTPUT_DIR/summary.json" ]; then
    echo ""
    info "Summary:"
    cat "$OUTPUT_DIR/summary.json" | head -50
fi
