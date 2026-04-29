#!/bin/bash
# ============================================================
# RiVision YOLO 最大性能测试
# 自动测试不同并发数，找出最大 FPS
# 用法: ./benchmark_yolo_max.sh [图片路径]
# ============================================================
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# 加载环境
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

YOLO_HOST="${YOLO_HOST:-127.0.0.1}"
YOLO_PORT="${YOLO_PORT:-9081}"
YOLO_URL="http://${YOLO_HOST}:${YOLO_PORT}"

# 测试图片
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"
if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

# 检查服务
if ! curl -s --max-time 2 "${YOLO_URL}/health" >/dev/null 2>&1; then
    echo -e "${RED}❌ YOLO 服务未运行${NC}"
    exit 1
fi

# 获取服务配置
HEALTH=$(curl -s "${YOLO_URL}/health" 2>/dev/null)
WORKERS=$(echo "$HEALTH" | sed -n 's/.*"workers":\([0-9]*\).*/\1/p')
[ -z "$WORKERS" ] && WORKERS=2

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       RiVision YOLO 最大性能测试                         ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "服务配置: Workers=$WORKERS"
echo "测试图片: $(basename "$IMAGE")"
echo ""

# 单次推理
run_inference() {
    local start_ns=$(date +%s%N)
    local response=$(curl -s -w "\n%{http_code}" \
        --connect-timeout 5 --max-time 30 \
        -X POST "${YOLO_URL}/api/detect/binary" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@${IMAGE}" 2>&1)
    local end_ns=$(date +%s%N)
    local latency=$(( (end_ns - start_ns) / 1000000 ))
    local http_code=$(echo "$response" | tail -1)
    
    if [ "$http_code" = "200" ]; then
        local inference=$(echo "$response" | sed -n 's/.*"inference_ms"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' | head -1)
        [ -z "$inference" ] && inference=$latency
        echo "OK $latency $inference"
    else
        echo "FAIL $latency 0"
    fi
}

# 并发测试
run_concurrent_test() {
    local concurrency=$1
    local runs=$2
    local temp_dir=$(mktemp -d)
    
    local total_success=0
    local total_latency=0
    local start_ns=$(date +%s%N)
    
    for ((r=1; r<=runs; r++)); do
        # 并发发送请求
        for ((i=1; i<=concurrency; i++)); do
            (run_inference > "$temp_dir/r${r}_c${i}.txt") &
        done
        wait
        
        # 收集结果
        for ((i=1; i<=concurrency; i++)); do
            result=$(cat "$temp_dir/r${r}_c${i}.txt" 2>/dev/null || echo "FAIL 0 0")
            status=$(echo "$result" | awk '{print $1}')
            latency=$(echo "$result" | awk '{print $2}')
            if [ "$status" = "OK" ]; then
                ((total_success++)) || true
                total_latency=$((total_latency + latency))
            fi
        done
    done
    
    local end_ns=$(date +%s%N)
    local total_ms=$(( (end_ns - start_ns) / 1000000 ))
    local total_sec=$(awk "BEGIN {printf \"%.2f\", $total_ms / 1000}")
    
    rm -rf "$temp_dir"
    
    local total_requests=$((runs * concurrency))
    local fps=$(awk "BEGIN {printf \"%.2f\", $total_success / $total_sec}")
    local avg_latency=0
    [ "$total_success" -gt 0 ] && avg_latency=$((total_latency / total_success))
    local success_rate=$(awk "BEGIN {printf \"%.1f\", $total_success * 100 / $total_requests}")
    
    echo "$concurrency $fps $avg_latency $success_rate $total_success $total_requests"
}

# ══════════════════════════════════════════════════════════
# 测试流程
# ══════════════════════════════════════════════════════════

echo -e "${CYAN}▶ 预热 (5次)...${NC}"
for ((i=1; i<=5; i++)); do
    run_inference > /dev/null
done
echo "  预热完成"
echo ""

# 单帧基准
echo -e "${CYAN}▶ 单帧性能测试...${NC}"
single_result=$(run_inference)
SINGLE_LATENCY=$(echo "$single_result" | awk '{print $2}')
SINGLE_INFERENCE=$(echo "$single_result" | awk '{print $3}')
echo "  单帧延迟: ${SINGLE_LATENCY} ms (推理: ${SINGLE_INFERENCE} ms)"
echo ""

# 不同并发测试
echo -e "${CYAN}▶ 并发性能测试...${NC}"
echo ""
echo "  并发数 |   FPS   | 平均延迟 | 成功率 | 成功/总数"
echo "  -------|---------|----------|--------|----------"

declare -a RESULTS=()
BEST_FPS=0
BEST_CONCURRENCY=1

# 测试 1, 2, 4, 8 并发
for c in 1 2 4 8; do
    echo -n "    $c    |"
    result=$(run_concurrent_test $c 5)
    
    fps=$(echo "$result" | awk '{print $2}')
    latency=$(echo "$result" | awk '{print $3}')
    success_rate=$(echo "$result" | awk '{print $4}')
    success=$(echo "$result" | awk '{print $5}')
    total=$(echo "$result" | awk '{print $6}')
    
    printf " %7s | %6s ms | %5s%% | %s/%s\n" "$fps" "$latency" "$success_rate" "$success" "$total"
    
    RESULTS+=("$result")
    
    # 记录最佳
    if (( $(echo "$fps > $BEST_FPS" | bc -l 2>/dev/null || echo 0) )); then
        BEST_FPS=$fps
        BEST_CONCURRENCY=$c
    fi
done

echo ""

# ══════════════════════════════════════════════════════════
# 结果分析
# ══════════════════════════════════════════════════════════

echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  最大性能测试结果                                        ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  🏆 ${GREEN}最大 FPS: ${BEST_FPS} fps${NC} (并发=${BEST_CONCURRENCY})"
echo ""
echo "  📊 性能数据"
echo "  ────────────────────────────────────────────────────────"
echo "    单帧推理时间:     ${SINGLE_INFERENCE} ms"
echo "    单帧端到端延迟:   ${SINGLE_LATENCY} ms"
echo "    服务 Workers:     ${WORKERS}"
echo "    最佳并发数:       ${BEST_CONCURRENCY}"
echo "    最大吞吐量:       ${BEST_FPS} fps"
echo ""

# 计算可支持路数
echo "  📋 多路视频支持能力"
echo "  ────────────────────────────────────────────────────────"
for target in 15 10 5 3; do
    streams=$(awk "BEGIN {printf \"%.0f\", $BEST_FPS / $target}")
    if [ "$streams" -ge 1 ]; then
        echo "    @ ${target}fps/路: 最多 ${streams} 路视频"
    else
        pct=$(awk "BEGIN {printf \"%.0f\", $BEST_FPS * 100 / $target}")
        echo "    @ ${target}fps/路: 不足 (仅能达到 ${pct}%)"
    fi
done
echo ""

# 优化建议
echo "  💡 优化建议"
echo "  ────────────────────────────────────────────────────────"
if [ "$BEST_CONCURRENCY" -ge "$WORKERS" ] && [ "$WORKERS" -lt 4 ]; then
    echo "    → 增加 YOLO_WORKERS 到 4 可能提升吞吐量"
    echo "      sudo vim /opt/rivision/rivision_node/config/.env"
    echo "      YOLO_WORKERS=4"
    echo "      sudo systemctl restart rivision-yolo"
fi
if [ "$BEST_CONCURRENCY" -lt "$WORKERS" ]; then
    echo "    → 最佳并发 < Workers，可能存在其他瓶颈"
fi
echo ""

# 保存结果
RESULT_FILE="$DEPLOY_DIR/logs/benchmark_yolo_max_$(date +%Y%m%d_%H%M%S).txt"
mkdir -p "$DEPLOY_DIR/logs"
cat > "$RESULT_FILE" << EOF
# YOLO Maximum Performance Test
# $(date '+%Y-%m-%d %H:%M:%S')
workers=$WORKERS
single_inference_ms=$SINGLE_INFERENCE
single_latency_ms=$SINGLE_LATENCY
best_concurrency=$BEST_CONCURRENCY
max_fps=$BEST_FPS
EOF

echo -e "📊 结果已保存: ${GREEN}$RESULT_FILE${NC}"
echo ""
