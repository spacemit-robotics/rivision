#!/bin/bash
# ============================================================
# RiVision VLM 本地推理基准测试 (情况1)
# 
# 说明: 使用 llama.cpp 进行本地 VLM 推理
# 适用场景: 测量 Vision + LLM 推理性能
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
RUNS="${2:-5}"
THREADS="${3:-6}"
OUTPUT_DIR="${4:-$BENCHMARK_DIR/results}"

# 加载环境
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

# 加载模型配置
MODEL_CONFIG="$DEPLOY_DIR/config/active-model.sh"
if [ ! -f "$MODEL_CONFIG" ]; then
    echo -e "${RED}❌ 未配置 VLM 模型${NC}"
    echo "请先运行: scripts/switch-model.sh <model>"
    exit 1
fi
source "$MODEL_CONFIG"

# 检查
if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

if [ ! -f "$ACTIVE_MODEL_FILE" ]; then
    echo -e "${RED}❌ 模型不存在: $ACTIVE_MODEL_FILE${NC}"
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
echo -e "${BOLD}║    RiVision VLM 本地推理基准测试 (情况1)                 ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "配置:"
echo "  模型:     $ACTIVE_MODEL_NAME"
echo "  模型文件: $(basename "$ACTIVE_MODEL_FILE")"
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    echo "  视觉后端: NPU (SpacemiT smt)"
else
    echo "  mmproj:   $(basename "${ACTIVE_MMPROJ_FILE:-none}")"
fi
echo "  图片:     $(basename "$IMAGE")"
echo "  运行次数: $RUNS"
echo "  线程数:   $THREADS"
echo ""

# 设置环境变量
export OMP_NUM_THREADS=$THREADS
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$DEPLOY_DIR/engines/llama.cpp:${LD_LIBRARY_PATH:-}"

mkdir -p "$OUTPUT_DIR"

# 运行测试
"$BENCHMARK_BIN" vlm-local \
    --image "$IMAGE" \
    --model "$ACTIVE_MODEL_FILE" \
    --mmproj "$ACTIVE_MMPROJ_FILE" \
    --runs "$RUNS" \
    --threads "$THREADS" \
    --output "$OUTPUT_DIR"

echo ""
echo -e "${GREEN}✅ 测试完成${NC}"
echo "结果保存在: $OUTPUT_DIR/"
