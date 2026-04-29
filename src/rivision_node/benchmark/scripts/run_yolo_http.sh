#!/bin/bash
# ============================================================
# RiVision YOLO HTTP 服务基准测试 (情况2)
# 
# 说明: 通过 HTTP 调用 yolo-server 进行推理
# 适用场景: 测量端到端性能，包含网络开销
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
RUNS="${2:-100}"
SWEEP="${3:-true}"
OUTPUT_DIR="${4:-$BENCHMARK_DIR/results}"

# 加载环境
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

YOLO_HOST="${YOLO_HOST:-127.0.0.1}"
YOLO_PORT="${YOLO_PORT:-9081}"
YOLO_URL="http://${YOLO_HOST}:${YOLO_PORT}"

# 检查图片
if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

# 检查服务
if ! curl -s --max-time 2 "${YOLO_URL}/health" >/dev/null 2>&1; then
    echo -e "${RED}❌ YOLO 服务未运行: $YOLO_URL${NC}"
    echo "请先启动服务: sudo systemctl start rivision-yolo"
    exit 1
fi

# 获取服务信息
HEALTH=$(curl -s "${YOLO_URL}/health" 2>/dev/null)
WORKERS=$(echo "$HEALTH" | sed -n 's/.*"workers":\([0-9]*\).*/\1/p')
[ -z "$WORKERS" ] && WORKERS=2

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
echo -e "${BOLD}║    RiVision YOLO HTTP 服务基准测试 (情况2)               ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "配置:"
echo "  服务:     $YOLO_URL"
echo "  Workers:  $WORKERS"
echo "  图片:     $(basename "$IMAGE")"
echo "  运行次数: $RUNS"
echo "  并发扫描: $SWEEP"
echo ""

mkdir -p "$OUTPUT_DIR"

# 运行测试
if [ "$SWEEP" = "true" ]; then
    "$BENCHMARK_BIN" yolo-http \
        --image "$IMAGE" \
        --url "$YOLO_URL" \
        --runs "$RUNS" \
        --sweep \
        --output "$OUTPUT_DIR"
else
    "$BENCHMARK_BIN" yolo-http \
        --image "$IMAGE" \
        --url "$YOLO_URL" \
        --runs "$RUNS" \
        --concurrency 1 \
        --output "$OUTPUT_DIR"
fi

echo ""
echo -e "${GREEN}✅ 测试完成${NC}"
echo "结果保存在: $OUTPUT_DIR/"
