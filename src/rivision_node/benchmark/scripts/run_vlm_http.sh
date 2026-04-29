#!/bin/bash
# ============================================================
# RiVision VLM HTTP 服务基准测试 (情况2)
# 
# 说明: 通过 HTTP 调用 llama-server 进行 VLM 推理
# 适用场景: 测量端到端 VLM 性能
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
OUTPUT_DIR="${3:-$BENCHMARK_DIR/results}"

# 加载环境
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

LLAMA_HOST="${LLAMA_HOST:-127.0.0.1}"
LLAMA_PORT="${LLAMA_PORT:-8080}"
LLAMA_URL="http://${LLAMA_HOST}:${LLAMA_PORT}"

# 检查图片
if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

# 检查服务
if ! curl -s --max-time 5 "${LLAMA_URL}/health" >/dev/null 2>&1; then
    echo -e "${RED}❌ VLM 服务未运行: $LLAMA_URL${NC}"
    echo "请先启动服务: scripts/start-llama.sh"
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
echo -e "${BOLD}║    RiVision VLM HTTP 服务基准测试 (情况2)                ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "配置:"
echo "  服务:     $LLAMA_URL"
echo "  图片:     $(basename "$IMAGE")"
echo "  运行次数: $RUNS"
echo ""

mkdir -p "$OUTPUT_DIR"

# 运行测试
"$BENCHMARK_BIN" vlm-http \
    --image "$IMAGE" \
    --url "$LLAMA_URL" \
    --runs "$RUNS" \
    --output "$OUTPUT_DIR"

echo ""
echo -e "${GREEN}✅ 测试完成${NC}"
echo "结果保存在: $OUTPUT_DIR/"
