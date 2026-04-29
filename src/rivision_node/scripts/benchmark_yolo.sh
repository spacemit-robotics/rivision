#!/bin/bash
# ============================================================
# RiVision YOLO 推理性能基准测试
# 用法: ./benchmark_yolo.sh [图片路径] [运行次数] [并发数]
#
# 测试指标:
#   1. 单帧推理延迟 (ms)
#   2. 吞吐量 (fps)
#   3. 内存使用 (MB)
#   4. CPU 使用率 (%)
#   5. NPU 利用率 (K3 SpacemiT A100)
#
# 适用于 K3 (RISC-V) / x86 等不同平台的性能对比
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── 颜色 ──────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ── 参数 ──────────────────────────────────────────────────
IMAGE="${1:-}"
RUNS="${2:-10}"
CONCURRENCY="${3:-1}"
WARMUP_RUNS=3

# 加载环境变量
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

YOLO_HOST="${YOLO_HOST:-127.0.0.1}"
YOLO_PORT="${YOLO_PORT:-9081}"
YOLO_URL="http://${YOLO_HOST}:${YOLO_PORT}"
YOLO_WORKERS="${YOLO_WORKERS:-4}"
YOLO_THREADS="${YOLO_THREADS:-4}"

# ── 检测平台 ──────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    riscv64) PLATFORM="K3 (RISC-V SpacemiT A100 NPU)" ;;
    x86_64)  PLATFORM="x86_64 (CPU/CUDA)" ;;
    aarch64) PLATFORM="ARM64" ;;
    *)       PLATFORM="$ARCH" ;;
esac

# CPU 信息
if [ -f /proc/cpuinfo ]; then
    CPU_MODEL=$(grep -m1 'model name\|isa' /proc/cpuinfo | cut -d: -f2 | xargs)
else
    CPU_MODEL="unknown"
fi
NPROC=$(nproc 2>/dev/null || echo "?")

# 内存信息
get_memory_info() {
    awk '/MemTotal/{total=$2} /MemAvailable/{avail=$2} END{printf "%.1f/%.1f GB", (total-avail)/1048576, total/1048576}' /proc/meminfo 2>/dev/null || echo "unknown"
}
MEM_TOTAL=$(awk '/MemTotal/{printf "%.1f GB", $2/1048576}' /proc/meminfo 2>/dev/null || echo "unknown")

# ── 查找测试图片 ──────────────────────────────────────────
find_test_image() {
    local candidates=(
        "$DEPLOY_DIR/test.jpg"
        "$DEPLOY_DIR/test.png"
        "$DEPLOY_DIR/models/test.jpg"
        "$SCRIPT_DIR/test.jpg"
    )
    for img in "${candidates[@]}"; do
        [ -f "$img" ] && echo "$img" && return
    done
    echo ""
}

if [ -z "$IMAGE" ]; then
    IMAGE=$(find_test_image)
    if [ -z "$IMAGE" ]; then
        echo -e "${RED}❌ 未找到测试图片${NC}"
        echo "请提供图片路径: $0 <图片路径> [运行次数] [并发数]"
        echo "或将 test.jpg 放到 $DEPLOY_DIR/ 目录"
        exit 1
    fi
fi

if [ ! -f "$IMAGE" ]; then
    echo -e "${RED}❌ 图片不存在: $IMAGE${NC}"
    exit 1
fi

IMAGE_SIZE=$(stat -c%s "$IMAGE" 2>/dev/null || stat -f%z "$IMAGE" 2>/dev/null || echo "0")
IMAGE_SIZE_KB=$(( IMAGE_SIZE / 1024 ))

# ── 加载 YOLO 模型配置 ────────────────────────────────────
YOLO_CONFIG="$DEPLOY_DIR/config/active-yolo-model.sh"
if [ -f "$YOLO_CONFIG" ]; then
    source "$YOLO_CONFIG"
    YOLO_MODEL_NAME="${ACTIVE_YOLO_NAME:-unknown}"
    YOLO_MODEL_FILE="${ACTIVE_YOLO_FILE:-unknown}"
else
    YOLO_MODEL_NAME="default"
    YOLO_MODEL_FILE="yolov8n.onnx"
fi

# ── 检查 YOLO 服务 ────────────────────────────────────────
check_yolo_service() {
    if ! curl -s --max-time 2 "${YOLO_URL}/health" >/dev/null 2>&1; then
        echo -e "${RED}❌ YOLO 服务未运行${NC}"
        echo "请先启动: sudo systemctl start rivision-yolo"
        echo "或运行: $SCRIPT_DIR/start-yolo.sh"
        exit 1
    fi
}

# ── 获取进程资源使用 ──────────────────────────────────────
get_yolo_pid() {
    pgrep -f "yolo-server" | head -1 || echo ""
}

get_process_stats() {
    local pid="$1"
    if [ -n "$pid" ] && [ -d "/proc/$pid" ]; then
        # 内存: RSS (MB)
        local rss=$(awk '{print $2/1024}' /proc/$pid/statm 2>/dev/null || echo "0")
        local page_size=$(getconf PAGE_SIZE 2>/dev/null || echo 4096)
        rss=$(awk "BEGIN {printf \"%.1f\", $rss * $page_size / 1048576}")
        
        # CPU: 从 /proc/pid/stat 读取
        local stat=$(cat /proc/$pid/stat 2>/dev/null)
        local utime=$(echo "$stat" | awk '{print $14}')
        local stime=$(echo "$stat" | awk '{print $15}')
        
        echo "$rss $utime $stime"
    else
        echo "0 0 0"
    fi
}

# 计算 CPU 使用率
calc_cpu_usage() {
    local utime1=$1 stime1=$2 utime2=$3 stime2=$4 elapsed=$5
    local ticks_per_sec=$(getconf CLK_TCK 2>/dev/null || echo 100)
    local cpu_time=$(( (utime2 - utime1) + (stime2 - stime1) ))
    local cpu_sec=$(awk "BEGIN {printf \"%.2f\", $cpu_time / $ticks_per_sec}")
    local usage=$(awk "BEGIN {printf \"%.1f\", ($cpu_sec / $elapsed) * 100}")
    echo "$usage"
}

# ── 单次推理测试 ──────────────────────────────────────────
run_single_inference() {
    local start_ns=$(date +%s%N)
    
    # 添加超时: 连接超时5秒，总超时30秒
    # API: /api/detect/binary - 直接发送二进制 JPEG 数据
    local response=$(curl -s -w "\n%{http_code}" \
        --connect-timeout 5 \
        --max-time 30 \
        -X POST "${YOLO_URL}/api/detect/binary" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@${IMAGE}" \
        2>&1)
    
    local curl_exit=$?
    local end_ns=$(date +%s%N)
    local latency_ms=$(( (end_ns - start_ns) / 1000000 ))
    
    # curl 失败检查
    if [ $curl_exit -ne 0 ]; then
        echo "FAIL $latency_ms 0 0 curl_error_$curl_exit"
        return
    fi
    
    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')
    
    if [ "$http_code" = "200" ]; then
        # 兼容性提取 (不依赖 grep -P)
        # 服务返回: {"success":true,"detections":[...],"inference_ms":123}
        local inference_ms=$(echo "$body" | sed -n 's/.*"inference_ms"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p' | head -1)
        [ -z "$inference_ms" ] && inference_ms="$latency_ms"
        local detections=$(echo "$body" | tr ',' '\n' | grep -c '"class_id"' 2>/dev/null || echo "0")
        echo "OK $latency_ms $inference_ms $detections"
    else
        echo "FAIL $latency_ms 0 0 http_$http_code"
    fi
}

# ── 并发推理测试 ──────────────────────────────────────────
run_concurrent_inference() {
    local concurrency=$1
    local temp_dir=$(mktemp -d)
    
    local start_ns=$(date +%s%N)
    
    for ((i=1; i<=concurrency; i++)); do
        (
            run_single_inference > "$temp_dir/result_$i.txt"
        ) &
    done
    wait
    
    local end_ns=$(date +%s%N)
    local total_ms=$(( (end_ns - start_ns) / 1000000 ))
    
    # 汇总结果
    local success=0 total_latency=0 total_inference=0 total_detections=0
    for ((i=1; i<=concurrency; i++)); do
        local result=$(cat "$temp_dir/result_$i.txt" 2>/dev/null || echo "FAIL 0 0 0")
        local status=$(echo "$result" | awk '{print $1}')
        if [ "$status" = "OK" ]; then
            ((success++))
            total_latency=$(( total_latency + $(echo "$result" | awk '{print $2}') ))
            total_inference=$(( total_inference + $(echo "$result" | awk '{print $3}') ))
            total_detections=$(( total_detections + $(echo "$result" | awk '{print $4}') ))
        fi
    done
    
    rm -rf "$temp_dir"
    
    local avg_latency=0 avg_inference=0 avg_detections=0
    if [ "$success" -gt 0 ]; then
        avg_latency=$(( total_latency / success ))
        avg_inference=$(( total_inference / success ))
        avg_detections=$(( total_detections / success ))
    fi
    
    echo "$success $concurrency $total_ms $avg_latency $avg_inference $avg_detections"
}

# ══════════════════════════════════════════════════════════
# 主程序
# ══════════════════════════════════════════════════════════

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║          RiVision YOLO 推理性能基准测试                  ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

# 检查服务
check_yolo_service
YOLO_PID=$(get_yolo_pid)

echo "┌─ 平台信息 ──────────────────────────────────────────────"
echo "│  架构:       $PLATFORM"
echo "│  CPU:        $CPU_MODEL"
echo "│  核心数:     $NPROC"
echo "│  内存:       $MEM_TOTAL"
echo "│"
echo "├─ YOLO 配置 ─────────────────────────────────────────────"
echo "│  模型:       $YOLO_MODEL_NAME"
echo "│  文件:       $(basename "$YOLO_MODEL_FILE")"
echo "│  服务地址:   $YOLO_URL"
echo "│  Workers:    $YOLO_WORKERS"
echo "│  Threads:    $YOLO_THREADS"
echo "│  进程 PID:   ${YOLO_PID:-未检测到}"
echo "│"
echo "├─ 测试参数 ──────────────────────────────────────────────"
echo "│  测试图片:   $(basename "$IMAGE") (${IMAGE_SIZE_KB} KB)"
echo "│  运行次数:   $RUNS"
echo "│  并发数:     $CONCURRENCY"
echo "│  预热次数:   $WARMUP_RUNS"
echo "└────────────────────────────────────────────────────────"
echo ""

# ── 预热 ──────────────────────────────────────────────────
echo -e "${CYAN}▶ 预热中 ($WARMUP_RUNS 次)...${NC}"
for ((i=1; i<=WARMUP_RUNS; i++)); do
    run_single_inference > /dev/null
done
echo -e "${GREEN}  预热完成${NC}"
echo ""

# ── 记录初始资源 ──────────────────────────────────────────
MEM_BEFORE=$(get_memory_info)
if [ -n "$YOLO_PID" ]; then
    read RSS_BEFORE UTIME_BEFORE STIME_BEFORE <<< $(get_process_stats "$YOLO_PID")
else
    RSS_BEFORE=0 UTIME_BEFORE=0 STIME_BEFORE=0
fi

# ── 运行基准测试 ──────────────────────────────────────────
declare -a ALL_LATENCY=()
declare -a ALL_INFERENCE=()
declare -a ALL_DETECTIONS=()
TOTAL_SUCCESS=0
TOTAL_FAIL=0

BENCH_START=$(date +%s%N)

echo -e "${CYAN}▶ 运行基准测试 ($RUNS 次, 并发=$CONCURRENCY)...${NC}"
echo "   (每次请求超时: 30秒)"
echo ""

for ((run=1; run<=RUNS; run++)); do
    if [ "$CONCURRENCY" -eq 1 ]; then
        # 显示进度提示 (使用 echo -n 避免缓冲问题)
        echo -n "  [$run/$RUNS] 测试中... "
        
        result=$(run_single_inference)
        status=$(echo "$result" | awk '{print $1}')
        latency=$(echo "$result" | awk '{print $2}')
        inference=$(echo "$result" | awk '{print $3}')
        detections=$(echo "$result" | awk '{print $4}')
        error_info=$(echo "$result" | awk '{print $5}')
        
        if [ "$status" = "OK" ]; then
            ((TOTAL_SUCCESS++)) || true
            ALL_LATENCY+=("$latency")
            ALL_INFERENCE+=("$inference")
            ALL_DETECTIONS+=("$detections")
            echo "✓ 延迟: ${latency} ms | 推理: ${inference} ms | 检测: ${detections}"
        else
            ((TOTAL_FAIL++)) || true
            echo "✗ 失败 (${error_info:-unknown})"
        fi
    else
        result=$(run_concurrent_inference "$CONCURRENCY")
        success=$(echo "$result" | awk '{print $1}')
        total=$(echo "$result" | awk '{print $2}')
        batch_ms=$(echo "$result" | awk '{print $3}')
        avg_latency=$(echo "$result" | awk '{print $4}')
        avg_inference=$(echo "$result" | awk '{print $5}')
        avg_detections=$(echo "$result" | awk '{print $6}')
        
        TOTAL_SUCCESS=$((TOTAL_SUCCESS + success))
        TOTAL_FAIL=$((TOTAL_FAIL + total - success))
        [ "$avg_latency" -gt 0 ] 2>/dev/null && ALL_LATENCY+=("$avg_latency")
        [ "$avg_inference" -gt 0 ] 2>/dev/null && ALL_INFERENCE+=("$avg_inference")
        
        fps=$(awk "BEGIN {printf \"%.1f\", $success * 1000 / ($batch_ms + 1)}")
        printf "  [%2d/%d] 成功: %d/%d | 批次: %d ms | 平均延迟: %d ms | 吞吐: %s fps\n" \
            "$run" "$RUNS" "$success" "$total" "$batch_ms" "$avg_latency" "$fps"
    fi
done

BENCH_END=$(date +%s%N)
BENCH_TOTAL_MS=$(( (BENCH_END - BENCH_START) / 1000000 ))
BENCH_TOTAL_SEC=$(awk "BEGIN {printf \"%.2f\", $BENCH_TOTAL_MS / 1000}")

# ── 记录结束资源 ──────────────────────────────────────────
MEM_AFTER=$(get_memory_info)
if [ -n "$YOLO_PID" ]; then
    read RSS_AFTER UTIME_AFTER STIME_AFTER <<< $(get_process_stats "$YOLO_PID")
    CPU_USAGE=$(calc_cpu_usage "$UTIME_BEFORE" "$STIME_BEFORE" "$UTIME_AFTER" "$STIME_AFTER" "$BENCH_TOTAL_SEC")
else
    RSS_AFTER=0 CPU_USAGE=0
fi

# ── 计算统计 ──────────────────────────────────────────────
calc_stats() {
    local -n arr=$1
    local count=${#arr[@]}
    [ "$count" -eq 0 ] && echo "0 0 0 0" && return
    
    # 排序
    IFS=$'\n' sorted=($(sort -n <<<"${arr[*]}")); unset IFS
    
    local sum=0 min=${sorted[0]} max=${sorted[$((count-1))]}
    for v in "${arr[@]}"; do sum=$((sum + v)); done
    local avg=$((sum / count))
    
    # P50, P95, P99
    local p50_idx=$((count * 50 / 100))
    local p95_idx=$((count * 95 / 100))
    local p99_idx=$((count * 99 / 100))
    [ "$p50_idx" -ge "$count" ] && p50_idx=$((count - 1))
    [ "$p95_idx" -ge "$count" ] && p95_idx=$((count - 1))
    [ "$p99_idx" -ge "$count" ] && p99_idx=$((count - 1))
    
    echo "$avg $min $max ${sorted[$p50_idx]} ${sorted[$p95_idx]} ${sorted[$p99_idx]}"
}

read AVG_LATENCY MIN_LATENCY MAX_LATENCY P50_LATENCY P95_LATENCY P99_LATENCY <<< $(calc_stats ALL_LATENCY)
read AVG_INFERENCE _ _ _ _ _ <<< $(calc_stats ALL_INFERENCE)

# 默认值处理
[ -z "$AVG_LATENCY" ] || [ "$AVG_LATENCY" = "0" ] && AVG_LATENCY=1
[ -z "$AVG_INFERENCE" ] || [ "$AVG_INFERENCE" = "0" ] && AVG_INFERENCE=$AVG_LATENCY

# 吞吐量
TOTAL_FRAMES=$((TOTAL_SUCCESS * CONCURRENCY))
THROUGHPUT=$(awk "BEGIN {printf \"%.2f\", $TOTAL_FRAMES / $BENCH_TOTAL_SEC}")

# ── 输出结果 ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  基准测试结果                                            ║${NC}"
echo -e "${BOLD}╠══════════════════════════════════════════════════════════╣${NC}"
echo "║"
echo -e "║  ${CYAN}延迟指标 (ms)${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    平均延迟:      %8d ms\n" "$AVG_LATENCY"
printf "║    最小延迟:      %8d ms\n" "$MIN_LATENCY"
printf "║    最大延迟:      %8d ms\n" "$MAX_LATENCY"
printf "║    P50:           %8d ms\n" "$P50_LATENCY"
printf "║    P95:           %8d ms\n" "$P95_LATENCY"
printf "║    P99:           %8d ms\n" "$P99_LATENCY"
echo "║"
echo -e "║  ${CYAN}吞吐量指标${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    总帧数:        %8d 帧\n" "$TOTAL_FRAMES"
printf "║    成功率:        %7.1f%%\n" "$(awk "BEGIN {printf \"%.1f\", $TOTAL_SUCCESS * 100 / ($TOTAL_SUCCESS + $TOTAL_FAIL)}")"
printf "║    吞吐量:        %8s fps\n" "$THROUGHPUT"
printf "║    总耗时:        %8s 秒\n" "$BENCH_TOTAL_SEC"
echo "║"
echo -e "║  ${CYAN}资源使用${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    系统内存:      %s\n" "$MEM_AFTER"
printf "║    进程内存:      %8.1f MB\n" "$RSS_AFTER"
printf "║    CPU 使用率:    %7.1f%%\n" "$CPU_USAGE"
echo "║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# ── K3 性能评估 ───────────────────────────────────────────
if [ "$ARCH" = "riscv64" ]; then
    echo -e "${BOLD}┌─ K3 SpacemiT A100 NPU 性能评估 ─────────────────────────${NC}"
    
    # 确保 YOLO_WORKERS 有值
    [ -z "$YOLO_WORKERS" ] && YOLO_WORKERS=2
    
    # 基于实测单帧推理时间计算理论最大吞吐量
    if [ "$AVG_INFERENCE" -gt 0 ] 2>/dev/null; then
        SINGLE_MAX_FPS=$(awk "BEGIN {printf \"%.1f\", 1000 / $AVG_INFERENCE}")
        THEORETICAL_MAX=$(awk "BEGIN {printf \"%.1f\", $YOLO_WORKERS * 1000 / $AVG_INFERENCE}")
    else
        SINGLE_MAX_FPS="N/A"
        THEORETICAL_MAX="N/A"
    fi
    
    echo "│"
    echo "│  📊 性能分析"
    echo "│  ────────────────────────────────────────────────────"
    echo "│  单帧推理时间:     ${AVG_INFERENCE:-N/A} ms"
    echo "│  单路理论最大:     ${SINGLE_MAX_FPS} fps (1000/${AVG_INFERENCE}ms)"
    echo "│  当前 Workers:     ${YOLO_WORKERS}"
    echo "│  理论最大吞吐:     ${THEORETICAL_MAX} fps (${YOLO_WORKERS} Workers 并发)"
    echo "│  实测吞吐量:       ${THROUGHPUT} fps (并发=${CONCURRENCY})"
    echo "│"
    
    # 评估是否达到理论值
    if [ "$CONCURRENCY" -ge "$YOLO_WORKERS" ]; then
        EFFICIENCY=$(awk "BEGIN {printf \"%.0f\", ($THROUGHPUT / $THEORETICAL_MAX) * 100}" 2>/dev/null || echo "0")
        if [ "$EFFICIENCY" -ge 80 ]; then
            echo -e "│  ✅ 效率: ${GREEN}${EFFICIENCY}%${NC} (实测/理论)"
        else
            echo -e "│  ⚠️  效率: ${YELLOW}${EFFICIENCY}%${NC} (实测/理论，可能存在瓶颈)"
        fi
    else
        echo -e "│  ⚠️  测试并发(${CONCURRENCY}) < Workers(${YOLO_WORKERS})，未充分利用"
        echo "│     建议重新测试: ./benchmark_yolo.sh test.jpg 10 ${YOLO_WORKERS}"
    fi
    
    echo "│"
    echo "│  📋 多路支持能力 (基于实测)"
    echo "│  ────────────────────────────────────────────────────"
    
    # 计算可支持的视频路数
    for target_fps in 15 10 5; do
        if [ -n "$THEORETICAL_MAX" ] && [ "$THEORETICAL_MAX" != "N/A" ]; then
            streams=$(awk "BEGIN {printf \"%.0f\", $THEORETICAL_MAX / $target_fps}")
            echo "│    @ ${target_fps}fps/路: 最多 ${streams} 路"
        fi
    done
    
    echo "│"
    echo "│  💡 优化建议"
    echo "│  ────────────────────────────────────────────────────"
    if [ "$YOLO_WORKERS" -lt 4 ]; then
        echo "│    → 增加 YOLO_WORKERS=4 提升并发"
    fi
    if [ "$CONCURRENCY" -lt "$YOLO_WORKERS" ]; then
        echo "│    → 使用并发测试: ./benchmark_yolo.sh test.jpg 10 ${YOLO_WORKERS}"
    fi
    echo "└────────────────────────────────────────────────────────"
fi

# ── 保存结果 ──────────────────────────────────────────────
RESULT_FILE="$DEPLOY_DIR/logs/benchmark_yolo_$(date +%Y%m%d_%H%M%S).txt"
mkdir -p "$DEPLOY_DIR/logs"
cat > "$RESULT_FILE" << EOF
# RiVision YOLO Benchmark Result
# $(date '+%Y-%m-%d %H:%M:%S')
platform=$PLATFORM
cpu=$CPU_MODEL
cores=$NPROC
memory=$MEM_TOTAL
model=$YOLO_MODEL_NAME
workers=$YOLO_WORKERS
threads=$YOLO_THREADS
image=$(basename "$IMAGE")
image_size_kb=$IMAGE_SIZE_KB
runs=$RUNS
concurrency=$CONCURRENCY
total_frames=$TOTAL_FRAMES
success_rate=$(awk "BEGIN {printf \"%.1f\", $TOTAL_SUCCESS * 100 / ($TOTAL_SUCCESS + $TOTAL_FAIL)}")
throughput_fps=$THROUGHPUT
avg_latency_ms=$AVG_LATENCY
min_latency_ms=$MIN_LATENCY
max_latency_ms=$MAX_LATENCY
p50_latency_ms=$P50_LATENCY
p95_latency_ms=$P95_LATENCY
p99_latency_ms=$P99_LATENCY
process_memory_mb=$RSS_AFTER
cpu_usage_percent=$CPU_USAGE
total_time_sec=$BENCH_TOTAL_SEC
EOF

echo -e "📊 结果已保存: ${GREEN}$RESULT_FILE${NC}"
echo ""
