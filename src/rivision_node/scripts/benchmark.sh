#!/bin/bash
# ============================================================
# RiVision 推理性能基准测试
# 用法: ./benchmark.sh [图片路径] [运行次数] [线程数]
#
# 输出一张照片推理的详细步骤和时间:
#   1. Vision 编码时间 (图像→embedding)
#   2. Prompt eval 速度 (含 vision token 处理)
#   3. Token 生成速度 (eval tok/s)
#   4. 总处理时间
#
# 适用于 K3 (RISC-V) / x86 等不同平台的性能对比
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-model.sh"

# ── 参数 ──────────────────────────────────────────────────
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"
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
    riscv64) PLATFORM="K3 (RISC-V rv64gcv)" ;;
    x86_64)  PLATFORM="x86_64" ;;
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
MEM_TOTAL=$(awk '/MemTotal/{printf "%.1f GB", $2/1048576}' /proc/meminfo 2>/dev/null || echo "unknown")

# ── 加载模型配置 ──────────────────────────────────────────
if [ ! -f "$CONFIG" ]; then
    echo "❌ 未配置模型，请先运行: scripts/switch-model.sh <model>"
    exit 1
fi
source "$CONFIG"

# ── 确定 CLI 路径 ─────────────────────────────────────────
LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
MTMD_CLI="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
if [ ! -x "$MTMD_CLI" ]; then
    echo "❌ llama-mtmd-cli 不存在: $MTMD_CLI"
    echo "   请检查 engines/llama.cpp/ 目录"
    exit 1
fi

# 共享库路径
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

# ── 检查图片 ──────────────────────────────────────────────
if [ ! -f "$IMAGE" ]; then
    echo "❌ 图片不存在: $IMAGE"
    echo "   用法: $0 <图片路径> [运行次数] [线程数]"
    exit 1
fi

IMAGE_SIZE=$(stat -c%s "$IMAGE" 2>/dev/null || stat -f%z "$IMAGE" 2>/dev/null || echo "?")

# ── 输出头 ────────────────────────────────────────────────
echo "╔══════════════════════════════════════════════════════════╗"
echo "║          RiVision 推理性能基准测试                       ║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""
echo "┌─ 平台信息 ──────────────────────────────────────────────"
echo "│  架构:     $PLATFORM"
echo "│  CPU:      $CPU_MODEL"
echo "│  核心数:   $NPROC"
echo "│  内存:     $MEM_TOTAL"
echo "│  线程数:   $THREADS"
[ -n "${LLAMA_CPU_AFFINITY:-}" ] && echo "│  CPU亲和:  $LLAMA_CPU_AFFINITY"
echo "│"
echo "├─ 模型信息 ──────────────────────────────────────────────"
echo "│  名称:     $ACTIVE_MODEL_NAME"
echo "│  模型:     $(basename "$ACTIVE_MODEL_FILE")"
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    echo "│  模式:     NPU (smt)"
else
    echo "│  mmproj:   $(basename "$ACTIVE_MMPROJ_FILE")"
fi
echo "│"
echo "├─ 测试参数 ──────────────────────────────────────────────"
echo "│  图片:     $IMAGE ($(( ${IMAGE_SIZE:-0} / 1024 )) KB)"
echo "│  运行次数: $RUNS"
echo "│  max tokens: 2048"
echo "│  context:  4096"
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

# 多模态后端: smt 模式 (NPU) 或 mmproj 模式 (CPU)
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    SMT_CONFIG_DIR="$(dirname "$ACTIVE_MODEL_FILE")"
    CMD_ARGS+=(--media-backend smt --smt-config-dir "$SMT_CONFIG_DIR")
elif [ -n "$ACTIVE_MMPROJ_FILE" ] && [ -f "$ACTIVE_MMPROJ_FILE" ]; then
    CMD_ARGS+=(--mmproj "$ACTIVE_MMPROJ_FILE")
fi

# MiniCPM (Qwen3) 需要 system prompt 禁用 thinking 模式
if [[ "$ACTIVE_MODEL_NAME" == minicpm-* ]]; then
    CMD_ARGS+=(-sys "直接给出分析结果，不要输出思考过程，不要使用think标签。")
fi

# 额外参数 (如 --jinja)
if [ -n "${ACTIVE_EXTRA_ARGS:-}" ]; then
    read -ra EXTRA <<< "$ACTIVE_EXTRA_ARGS"
    CMD_ARGS+=("${EXTRA[@]}")
fi

# ── 运行基准 ──────────────────────────────────────────────
declare -a ALL_VISION=()
declare -a ALL_PROMPT_TPS=()
declare -a ALL_EVAL_TPS=()
declare -a ALL_TOTAL=()
declare -a ALL_PROMPT_MS=()
declare -a ALL_EVAL_MS=()

for ((i=1; i<=RUNS; i++)); do
    echo "▶ 运行 $i/$RUNS ..."
    
    # 执行推理, 捕获全部输出
    OUTPUT=$("$MTMD_CLI" "${CMD_ARGS[@]}" 2>&1) || true
    
    # ── 提取指标 ──────────────────────────────────────────
    # Vision 编码时间 (ms) - 累加所有 image slice 编码时间
    VISION_MS=$(echo "$OUTPUT" | grep -oP 'image slice encoded in \K\d+' | paste -sd+ | bc 2>/dev/null || echo "0")
    VISION_SLICES=$(echo "$OUTPUT" | grep -c 'image slice encoded in' || echo "0")
    
    # prompt eval: tokens数, 总时间(ms), 速度(tok/s)
    PROMPT_TOKENS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '/\s*\K\d+' | head -1)
    PROMPT_MS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
    PROMPT_TPS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '\d+\.\d+(?= tokens per second)')
    
    # eval (生成): tokens数, 总时间(ms), 速度(tok/s)
    EVAL_TOKENS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '/\s*\K\d+' | head -1)
    EVAL_MS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
    EVAL_TPS=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '\d+\.\d+(?= tokens per second)')
    
    # total time (ms)
    TOTAL_MS=$(echo "$OUTPUT" | grep "total time" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
    
    # 记录
    ALL_VISION+=("${VISION_MS:-0}")
    ALL_PROMPT_TPS+=("${PROMPT_TPS:-0}")
    ALL_EVAL_TPS+=("${EVAL_TPS:-0}")
    ALL_TOTAL+=("${TOTAL_MS:-0}")
    ALL_PROMPT_MS+=("${PROMPT_MS:-0}")
    ALL_EVAL_MS+=("${EVAL_MS:-0}")
    
    # 计算首 token 延迟 (TTFT) = vision + prompt_eval
    if [ -n "$VISION_MS" ] && [ -n "$PROMPT_MS" ]; then
        TTFT=$(echo "$VISION_MS + $PROMPT_MS" | bc 2>/dev/null || echo "N/A")
    else
        TTFT="N/A"
    fi
    
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

# ── 汇总统计 ──────────────────────────────────────────────
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

# TTFT
AVG_TTFT=$(echo "$AVG_VISION + $AVG_PROMPT_MS" | bc 2>/dev/null || echo "N/A")

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  汇总结果 ($RUNS 次平均)                                  ║"
echo "╠══════════════════════════════════════════════════════════╣"
printf "║  %-20s %12s %s\n" "指标" "值" "║"
echo "║──────────────────────────────────────────────────────────║"
printf "║  %-20s %10s ms %s\n" "Vision 编码" "$AVG_VISION" "║"
printf "║  %-20s %10s ms %s\n" "Prompt eval" "$AVG_PROMPT_MS" "║"
printf "║  %-20s %8s tok/s %s\n" "Prompt eval 速度" "$AVG_PROMPT_TPS" "║"
printf "║  %-20s %10s ms %s\n" "Token 生成" "$AVG_EVAL_MS" "║"
printf "║  %-20s %8s tok/s %s\n" "Token 生成速度" "$AVG_EVAL_TPS" "║"
echo "║──────────────────────────────────────────────────────────║"
printf "║  %-20s %10s ms %s\n" "⏱ 首token延迟(TTFT)" "$AVG_TTFT" "║"
printf "║  %-20s %10s ms %s\n" "⏱ 总处理时间" "$AVG_TOTAL" "║"
echo "╚══════════════════════════════════════════════════════════╝"
echo ""

# ── 输出机器可读格式 (方便对比) ───────────────────────────
RESULT_FILE="$DEPLOY_DIR/logs/benchmark-$(date +%Y%m%d_%H%M%S).txt"
mkdir -p "$DEPLOY_DIR/logs"
cat > "$RESULT_FILE" << EOF
# RiVision Benchmark Result
# $(date '+%Y-%m-%d %H:%M:%S')
platform=$PLATFORM
cpu=$CPU_MODEL
cores=$NPROC
memory=$MEM_TOTAL
threads=$THREADS
model=$ACTIVE_MODEL_NAME
image=$(basename "$IMAGE")
runs=$RUNS
avg_vision_ms=$AVG_VISION
avg_prompt_ms=$AVG_PROMPT_MS
avg_prompt_tps=$AVG_PROMPT_TPS
avg_eval_ms=$AVG_EVAL_MS
avg_eval_tps=$AVG_EVAL_TPS
avg_ttft_ms=$AVG_TTFT
avg_total_ms=$AVG_TOTAL
EOF

echo "📊 结果已保存: $RESULT_FILE"
echo ""
echo "关键指标说明:"
echo "  - Vision 编码:     图像 → embedding 的处理时间"
echo "  - Prompt eval:     prompt + vision tokens 的处理时间"
echo "  - Token 生成速度:  每秒生成的 token 数 (越高越好)"
echo "  - 首token延迟:     用户等待第一个回复字的时间 = Vision + Prompt eval"
echo "  - 总处理时间:      完整推理的端到端时间"
echo ""
