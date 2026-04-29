#!/bin/bash
# ============================================================
# 线程数调优测试
# 用法: ./bench-threads.sh [图片路径]
# 在 K3 上运行，分别测试 2/4/6/8 线程
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-model.sh"
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"

[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

if [ ! -f "$CONFIG" ]; then
    echo "❌ 未配置模型，请先运行: scripts/switch-model.sh <model>"
    exit 1
fi
source "$CONFIG"

LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
MTMD_CLI="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
if [ ! -x "$MTMD_CLI" ]; then
    echo "❌ llama-mtmd-cli 不存在: $MTMD_CLI"
    exit 1
fi

export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

echo "=== 线程调优测试 ==="
echo "  模型: $ACTIVE_MODEL_NAME"
echo "  图片: $IMAGE"
echo ""

CMD_BASE=(
    -m "$ACTIVE_MODEL_FILE"
    --image "$IMAGE"
    -p "${ACTIVE_PROMPT:-描述照片里面的内容}"
    -c 4096 -n 2048
    --temp 0.3 --seed 42
    --top-k 40 --top-p 0.9
)

# 多模态后端
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    SMT_CONFIG_DIR="$(dirname "$ACTIVE_MODEL_FILE")"
    CMD_BASE+=(--media-backend smt --smt-config-dir "$SMT_CONFIG_DIR")
elif [ -n "$ACTIVE_MMPROJ_FILE" ] && [ -f "$ACTIVE_MMPROJ_FILE" ]; then
    CMD_BASE+=(--mmproj "$ACTIVE_MMPROJ_FILE")
fi

if [[ "$ACTIVE_MODEL_NAME" == minicpm-* ]]; then
    CMD_BASE+=(-sys "直接给出分析结果，不要输出思考过程，不要使用think标签。")
fi

if [ -n "$ACTIVE_EXTRA_ARGS" ]; then
    read -ra EXTRA <<< "$ACTIVE_EXTRA_ARGS"
    CMD_BASE+=("${EXTRA[@]}")
fi

echo "线程数 | Prompt eval (tok/s) | Eval (tok/s) | 总时间 (ms)"
echo "-------|--------------------|--------------|-----------"

for THREADS in 2 4 6 8; do
    export OMP_NUM_THREADS=$THREADS

    OUTPUT=$("$MTMD_CLI" "${CMD_BASE[@]}" -t "$THREADS" 2>&1)

    PROMPT_TOKS=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '[\d.]+(?= tokens per second)')
    EVAL_TOKS=$(echo "$OUTPUT" | grep "eval time" | grep -oP '[\d.]+(?= tokens per second)')
    TOTAL_MS=$(echo "$OUTPUT" | grep "total time" | grep -oP '[\d.]+(?= ms /)')

    printf "  %d    | %18s  | %12s | %s\n" "$THREADS" "${PROMPT_TOKS:-N/A}" "${EVAL_TOKS:-N/A}" "${TOTAL_MS:-N/A}"
done

echo ""
echo "=== 测试完成 ==="
