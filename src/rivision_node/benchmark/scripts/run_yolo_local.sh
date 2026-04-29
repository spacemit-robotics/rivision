#!/bin/bash
# ============================================================
# RiVision YOLO 本地推理基准测试 (情况1)
# 
# 说明: 直接使用 ONNX Runtime 进行推理，无网络开销
# 适用场景: 测量纯推理性能，最大化吞吐量
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="$(cd "$BENCHMARK_DIR/.." && pwd)"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# 参数
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"
MODEL="${2:-$DEPLOY_DIR/models/yolov8n.onnx}"
RUNS="${3:-100}"
THREADS="${4:-4}"
OUTPUT_DIR="${5:-$BENCHMARK_DIR/results}"

# 加载环境
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

# 检查
if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

if [ ! -f "$MODEL" ]; then
    echo -e "${RED}❌ 模型不存在: $MODEL${NC}"
    exit 1
fi

# 检查 benchmark 工具
BENCHMARK_BIN="$BENCHMARK_DIR/build/rivision-benchmark"
if [ ! -x "$BENCHMARK_BIN" ]; then
    echo -e "${YELLOW}⚠ 编译 benchmark 工具...${NC}"
    mkdir -p "$BENCHMARK_DIR/build"
    cd "$BENCHMARK_DIR/build"
    cmake .. -DCMAKE_BUILD_TYPE=Release
    make -j$(nproc)
    cd "$SCRIPT_DIR"
fi

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║    RiVision YOLO 本地推理基准测试 (情况1)                ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "配置:"
echo "  模型:     $(basename "$MODEL")"
echo "  图片:     $(basename "$IMAGE")"
echo "  运行次数: $RUNS"
echo "  线程数:   $THREADS"
echo ""

# 设置环境变量
export OMP_NUM_THREADS=$THREADS
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:${LD_LIBRARY_PATH:-}"

# 运行测试
mkdir -p "$OUTPUT_DIR"

"$BENCHMARK_BIN" yolo-local \
    --image "$IMAGE" \
    --model "$MODEL" \
    --runs "$RUNS" \
    --threads "$THREADS" \
    --output "$OUTPUT_DIR"

echo ""
echo -e "${GREEN}✅ 测试完成${NC}"
echo "结果保存在: $OUTPUT_DIR/"
