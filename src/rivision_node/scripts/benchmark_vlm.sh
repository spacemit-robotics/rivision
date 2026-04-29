#!/bin/bash
# ============================================================
# RiVision VLM 推理性能基准测试
# 用法: ./benchmark_vlm.sh [图片路径] [运行次数] [线程数]
#
# 测试指标:
#   1. Vision 编码时间 (图像→embedding)
#   2. Prompt eval 速度 (含 vision token 处理)
#   3. Token 生成速度 (eval tok/s)
#   4. 首 Token 延迟 (TTFT)
#   5. 内存使用 (MB)
#   6. CPU 使用率 (%)
#
# 适用于 K3 (RISC-V) / x86 等不同平台的性能对比
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-model.sh"

# ── 颜色 ──────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ── 参数 ──────────────────────────────────────────────────
IMAGE="${1:-}"
RUNS="${2:-3}"
THREADS_OVERRIDE="${3:-}"

# 加载环境变量
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

THREADS="${THREADS_OVERRIDE:-${LLAMA_THREADS:-6}}"
export OMP_NUM_THREADS="$THREADS"
if [ -n "${LLAMA_CPU_AFFINITY:-}" ]; then
    export GOMP_CPU_AFFINITY="$LLAMA_CPU_AFFINITY"
fi

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
    awk '/MemTotal/{total=$2} /MemAvailable/{avail=$2} END{printf "%.1f/%.1f GB (%.0f%% used)", (total-avail)/1048576, total/1048576, (total-avail)*100/total}' /proc/meminfo 2>/dev/null || echo "unknown"
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
        echo "请提供图片路径: $0 <图片路径> [运行次数] [线程数]"
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

# ── 加载模型配置 ──────────────────────────────────────────
if [ ! -f "$CONFIG" ]; then
    echo -e "${RED}❌ 未配置模型，请先运行: scripts/switch-model.sh <model>${NC}"
    exit 1
fi
source "$CONFIG"

# ── 确定 CLI 路径 ─────────────────────────────────────────
LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
MTMD_CLI="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
if [ ! -x "$MTMD_CLI" ]; then
    echo -e "${RED}❌ llama-mtmd-cli 不存在: $MTMD_CLI${NC}"
    echo "   请检查 engines/llama.cpp/ 目录"
    exit 1
fi

# 共享库路径
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

# ── 获取 llama 进程 PID ───────────────────────────────────
get_llama_pid() {
    pgrep -f "llama-mtmd-cli\|llama-server" | head -1 || echo ""
}

# ── 获取进程资源使用 ──────────────────────────────────────
get_process_stats() {
    local pid="$1"
    if [ -n "$pid" ] && [ -d "/proc/$pid" ]; then
        local rss=$(awk '{print $2}' /proc/$pid/statm 2>/dev/null || echo "0")
        local page_size=$(getconf PAGE_SIZE 2>/dev/null || echo 4096)
        rss=$(awk "BEGIN {printf \"%.1f\", $rss * $page_size / 1048576}")
        
        local stat=$(cat /proc/$pid/stat 2>/dev/null)
        local utime=$(echo "$stat" | awk '{print $14}')
        local stime=$(echo "$stat" | awk '{print $15}')
        
        echo "$rss $utime $stime"
    else
        echo "0 0 0"
    fi
}

# 实时监控资源
monitor_resources() {
    local output_file="$1"
    local interval=0.5
    
    while true; do
        local mem_used=$(awk '/MemTotal/{total=$2} /MemAvailable/{avail=$2} END{printf "%.1f", (total-avail)/1048576}' /proc/meminfo 2>/dev/null)
        local cpu_idle=$(awk '/^cpu / {print $5}' /proc/stat 2>/dev/null)
        echo "$(date +%s.%N) $mem_used $cpu_idle" >> "$output_file"
        sleep "$interval"
    done
}

# ══════════════════════════════════════════════════════════
# 主程序
# ══════════════════════════════════════════════════════════

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║          RiVision VLM 推理性能基准测试                   ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

echo "┌─ 平台信息 ──────────────────────────────────────────────"
echo "│  架构:       $PLATFORM"
echo "│  CPU:        $CPU_MODEL"
echo "│  核心数:     $NPROC"
echo "│  内存:       $MEM_TOTAL"
echo "│  线程数:     $THREADS"
[ -n "${LLAMA_CPU_AFFINITY:-}" ] && echo "│  CPU亲和:    $LLAMA_CPU_AFFINITY"
echo "│"
echo "├─ 模型信息 ──────────────────────────────────────────────"
echo "│  名称:       $ACTIVE_MODEL_NAME"
echo "│  模型:       $(basename "$ACTIVE_MODEL_FILE")"
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    echo "│  视觉后端:   NPU (SpacemiT smt)"
else
    echo "│  mmproj:     $(basename "${ACTIVE_MMPROJ_FILE:-none}")"
fi
echo "│"
echo "├─ 测试参数 ──────────────────────────────────────────────"
echo "│  测试图片:   $(basename "$IMAGE") (${IMAGE_SIZE_KB} KB)"
echo "│  运行次数:   $RUNS"
echo "│  max tokens: 2048"
echo "│  context:    4096"
echo "└────────────────────────────────────────────────────────"
echo ""

# ── 构建推理参数 ──────────────────────────────────────────
CMD_ARGS=(
    -m "$ACTIVE_MODEL_FILE"
    --image "$IMAGE"
    -p "${ACTIVE_PROMPT:-Describe this image in detail.}"
    -c 4096 -n 2048 -t "$THREADS"
    --temp 0.3 --seed 42
    --top-k 40 --top-p 0.9
    --no-warmup
)

# 多模态后端
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    SMT_CONFIG_DIR="$(dirname "$ACTIVE_MODEL_FILE")"
    CMD_ARGS+=(--media-backend smt --smt-config-dir "$SMT_CONFIG_DIR")
elif [ -n "$ACTIVE_MMPROJ_FILE" ] && [ -f "$ACTIVE_MMPROJ_FILE" ]; then
    CMD_ARGS+=(--mmproj "$ACTIVE_MMPROJ_FILE")
fi

# MiniCPM 特殊处理
if [[ "$ACTIVE_MODEL_NAME" == minicpm-* ]]; then
    CMD_ARGS+=(-sys "直接给出分析结果，不要输出思考过程，不要使用think标签。")
fi

# 额外参数
if [ -n "${ACTIVE_EXTRA_ARGS:-}" ]; then
    read -ra EXTRA <<< "$ACTIVE_EXTRA_ARGS"
    CMD_ARGS+=("${EXTRA[@]}")
fi

# ── 记录初始资源 ──────────────────────────────────────────
MEM_BEFORE=$(get_memory_info)
MONITOR_FILE=$(mktemp)

# 启动资源监控
monitor_resources "$MONITOR_FILE" &
MONITOR_PID=$!
trap "kill $MONITOR_PID 2>/dev/null; rm -f $MONITOR_FILE" EXIT

# ── 运行基准测试 ──────────────────────────────────────────
declare -a ALL_VISION=()
declare -a ALL_PROMPT_TPS=()
declare -a ALL_EVAL_TPS=()
declare -a ALL_TOTAL=()
declare -a ALL_PROMPT_MS=()
declare -a ALL_EVAL_MS=()
declare -a ALL_TTFT=()
declare -a ALL_TOKENS=()

echo -e "${CYAN}▶ 运行基准测试 ($RUNS 次)...${NC}"
echo ""

for ((i=1; i<=RUNS; i++)); do
    echo -e "  ${BOLD}运行 $i/$RUNS${NC}"
    
    RUN_START=$(date +%s%N)
    
    # 执行推理, 捕获全部输出
    OUTPUT=$("$MTMD_CLI" "${CMD_ARGS[@]}" 2>&1) || true
    
    RUN_END=$(date +%s%N)
    RUN_WALL_MS=$(( (RUN_END - RUN_START) / 1000000 ))
    
    # ── 提取指标 ──────────────────────────────────────────
    # Vision 编码时间 (ms)
    VISION_MS=$(echo "$OUTPUT" | grep -oP 'image slice encoded in \K\d+' | paste -sd+ | bc 2>/dev/null || echo "0")
    VISION_SLICES=$(echo "$OUTPUT" | grep -c 'image slice encoded in' || echo "0")
    
    # prompt eval
    PROMPT_TOKENS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '/\s*\K\d+' | head -1 || echo "0")
    PROMPT_MS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '=\s*\K\d+\.\d+(?= ms)' || echo "0")
    PROMPT_TPS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '\d+\.\d+(?= tokens per second)' || echo "0")
    
    # eval (生成)
    EVAL_TOKENS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '/\s*\K\d+' | head -1 || echo "0")
    EVAL_MS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '=\s*\K\d+\.\d+(?= ms)' || echo "0")
    EVAL_TPS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '\d+\.\d+(?= tokens per second)' || echo "0")
    
    # total time
    TOTAL_MS=$(echo "$OUTPUT" | grep "total time" | grep -oP '=\s*\K\d+\.\d+(?= ms)' || echo "$RUN_WALL_MS")
    
    # TTFT = Vision + Prompt eval
    TTFT="0"
    if [ -n "$VISION_MS" ] && [ "$VISION_MS" != "0" ] && [ -n "$PROMPT_MS" ] && [ "$PROMPT_MS" != "0" ]; then
        TTFT=$(echo "$VISION_MS + $PROMPT_MS" | bc 2>/dev/null || echo "0")
    fi
    
    # 记录
    ALL_VISION+=("${VISION_MS:-0}")
    ALL_PROMPT_TPS+=("${PROMPT_TPS:-0}")
    ALL_EVAL_TPS+=("${EVAL_TPS:-0}")
    ALL_TOTAL+=("${TOTAL_MS:-0}")
    ALL_PROMPT_MS+=("${PROMPT_MS:-0}")
    ALL_EVAL_MS+=("${EVAL_MS:-0}")
    ALL_TTFT+=("${TTFT:-0}")
    ALL_TOKENS+=("${EVAL_TOKENS:-0}")
    
    echo "  ┌─────────────────────────────────────────────────"
    echo "  │ Vision 编码:    ${VISION_MS:-N/A} ms (${VISION_SLICES:-?} slices)"
    echo "  │ Prompt eval:    ${PROMPT_MS:-N/A} ms (${PROMPT_TOKENS:-?} tokens, ${PROMPT_TPS:-N/A} tok/s)"
    echo "  │ Token 生成:     ${EVAL_MS:-N/A} ms (${EVAL_TOKENS:-?} tokens, ${EVAL_TPS:-N/A} tok/s)"
    echo "  │ ─────────────────────────────────────"
    echo "  │ 首token延迟:    ${TTFT} ms"
    echo "  │ 总时间:         ${TOTAL_MS:-N/A} ms"
    echo "  └─────────────────────────────────────────────────"
    echo ""
done

# 停止监控
kill $MONITOR_PID 2>/dev/null || true

# ── 分析资源使用 ──────────────────────────────────────────
MEM_AFTER=$(get_memory_info)

# 从监控数据计算峰值内存
if [ -f "$MONITOR_FILE" ] && [ -s "$MONITOR_FILE" ]; then
    PEAK_MEM=$(awk '{if($2>max) max=$2} END{printf "%.1f", max}' "$MONITOR_FILE")
    AVG_MEM=$(awk '{sum+=$2; n++} END{printf "%.1f", sum/n}' "$MONITOR_FILE")
else
    PEAK_MEM="N/A"
    AVG_MEM="N/A"
fi

# ── 计算统计 ──────────────────────────────────────────────
calc_avg() {
    local -n arr=$1
    local sum=0
    local count=${#arr[@]}
    [ "$count" -eq 0 ] && echo "0" && return
    for v in "${arr[@]}"; do
        sum=$(echo "$sum + $v" | bc 2>/dev/null || echo "$sum")
    done
    echo "scale=1; $sum / $count" | bc 2>/dev/null || echo "0"
}

AVG_VISION=$(calc_avg ALL_VISION)
AVG_PROMPT_TPS=$(calc_avg ALL_PROMPT_TPS)
AVG_EVAL_TPS=$(calc_avg ALL_EVAL_TPS)
AVG_TOTAL=$(calc_avg ALL_TOTAL)
AVG_PROMPT_MS=$(calc_avg ALL_PROMPT_MS)
AVG_EVAL_MS=$(calc_avg ALL_EVAL_MS)
AVG_TTFT=$(calc_avg ALL_TTFT)
AVG_TOKENS=$(calc_avg ALL_TOKENS)

# ── 输出结果 ──────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  VLM 基准测试结果 ($RUNS 次平均)                         ║${NC}"
echo -e "${BOLD}╠══════════════════════════════════════════════════════════╣${NC}"
echo "║"
echo -e "║  ${CYAN}推理性能${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    Vision 编码:      %10s ms\n" "$AVG_VISION"
printf "║    Prompt eval:      %10s ms\n" "$AVG_PROMPT_MS"
printf "║    Prompt eval 速度: %8s tok/s\n" "$AVG_PROMPT_TPS"
printf "║    Token 生成:       %10s ms\n" "$AVG_EVAL_MS"
printf "║    Token 生成速度:   %8s tok/s\n" "$AVG_EVAL_TPS"
printf "║    生成 Token 数:    %10s\n" "$AVG_TOKENS"
echo "║"
echo -e "║  ${CYAN}延迟指标${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    ⏱ 首token延迟(TTFT): %7s ms\n" "$AVG_TTFT"
printf "║    ⏱ 总处理时间:        %7s ms\n" "$AVG_TOTAL"
echo "║"
echo -e "║  ${CYAN}资源使用${NC}"
echo "║  ────────────────────────────────────────────────────────"
printf "║    峰值内存:        %10s GB\n" "$PEAK_MEM"
printf "║    平均内存:        %10s GB\n" "$AVG_MEM"
printf "║    系统内存:        %s\n" "$MEM_AFTER"
echo "║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# ── K3 性能评估 ───────────────────────────────────────────
if [ "$ARCH" = "riscv64" ]; then
    echo -e "${BOLD}┌─ K3 SpacemiT A100 NPU 性能评估 ─────────────────────────${NC}"
    
    # K3 VLM 预期指标 (基于 MiniCPM-V 2.5)
    if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
        echo "│  ✅ 使用 NPU 加速 (SpacemiT smt backend)"
    else
        echo -e "│  ⚠️  未使用 NPU: ${YELLOW}请切换到支持 smt 的模型${NC}"
    fi
    
    # 评估 token 生成速度
    if (( $(echo "$AVG_EVAL_TPS > 5" | bc -l 2>/dev/null || echo 0) )); then
        echo -e "│  ✅ Token生成速度: ${GREEN}${AVG_EVAL_TPS} tok/s${NC}"
    else
        echo -e "│  ⚠️  Token生成速度偏低: ${YELLOW}${AVG_EVAL_TPS} tok/s${NC}"
    fi
    
    # 评估 TTFT
    if (( $(echo "$AVG_TTFT < 5000" | bc -l 2>/dev/null || echo 0) )); then
        echo -e "│  ✅ 首Token延迟: ${GREEN}${AVG_TTFT} ms${NC}"
    else
        echo -e "│  ⚠️  首Token延迟偏高: ${YELLOW}${AVG_TTFT} ms${NC}"
    fi
    
    echo "│"
    echo "│  K3 VLM 推荐配置:"
    echo "│    - 使用 NPU 加速: --media-backend smt"
    echo "│    - 线程数: LLAMA_THREADS=6-8"
    echo "│    - 模型: MiniCPM-V 或 FastVLM (4bit/8bit量化)"
    echo "└────────────────────────────────────────────────────────"
fi

# ── 保存结果 ──────────────────────────────────────────────
RESULT_FILE="$DEPLOY_DIR/logs/benchmark_vlm_$(date +%Y%m%d_%H%M%S).txt"
mkdir -p "$DEPLOY_DIR/logs"
cat > "$RESULT_FILE" << EOF
# RiVision VLM Benchmark Result
# $(date '+%Y-%m-%d %H:%M:%S')
platform=$PLATFORM
cpu=$CPU_MODEL
cores=$NPROC
memory=$MEM_TOTAL
threads=$THREADS
model=$ACTIVE_MODEL_NAME
model_file=$(basename "$ACTIVE_MODEL_FILE")
vision_backend=$([ "$ACTIVE_MMPROJ_FILE" = "smt" ] && echo "npu_smt" || echo "cpu")
image=$(basename "$IMAGE")
image_size_kb=$IMAGE_SIZE_KB
runs=$RUNS
avg_vision_ms=$AVG_VISION
avg_prompt_ms=$AVG_PROMPT_MS
avg_prompt_tps=$AVG_PROMPT_TPS
avg_eval_ms=$AVG_EVAL_MS
avg_eval_tps=$AVG_EVAL_TPS
avg_tokens=$AVG_TOKENS
avg_ttft_ms=$AVG_TTFT
avg_total_ms=$AVG_TOTAL
peak_memory_gb=$PEAK_MEM
avg_memory_gb=$AVG_MEM
EOF

echo -e "📊 结果已保存: ${GREEN}$RESULT_FILE${NC}"
echo ""
echo "关键指标说明:"
echo "  - Vision 编码:     图像 → embedding 的处理时间 (NPU 加速)"
echo "  - Prompt eval:     prompt + vision tokens 的处理时间"
echo "  - Token 生成速度:  每秒生成的 token 数 (越高越好)"
echo "  - 首token延迟:     用户等待第一个回复字的时间 = Vision + Prompt eval"
echo "  - 总处理时间:      完整推理的端到端时间"
echo ""
