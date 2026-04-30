#!/bin/bash
# ============================================================
# 快速测试脚本
# 用法: ./test.sh [图片路径]
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-model.sh"
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"

# 加载环境变量
[ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"

# 加载模型配置
if [ ! -f "$CONFIG" ]; then
    echo "❌ 未配置模型，请先运行: scripts/switch-model.sh <model>"
    exit 1
fi
source "$CONFIG"

# 确定CLI路径
LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
MTMD_CLI="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
if [ ! -x "$MTMD_CLI" ]; then
    echo "❌ llama-mtmd-cli 不存在: $MTMD_CLI"
    echo "   请检查 engines/llama.cpp/ 目录"
    exit 1
fi

# 共享库路径
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

THREADS="${LLAMA_THREADS:-6}"
export OMP_NUM_THREADS=$THREADS

echo "=== 快速推理测试 ==="
echo "  模型: $ACTIVE_MODEL_NAME"
echo "  图片: $IMAGE"
echo "  线程: $THREADS"
echo ""

# 构建参数
CMD_ARGS=(
    -m "$ACTIVE_MODEL_FILE"
    --image "$IMAGE"
    -p "${ACTIVE_PROMPT:-描述照片里面的内容}"
    -c 4096 -n 2048 -t "$THREADS"
    --temp 0.3 --seed 42
    --top-k 40 --top-p 0.9
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

# 额外参数
if [ -n "$ACTIVE_EXTRA_ARGS" ]; then
    read -ra EXTRA <<< "$ACTIVE_EXTRA_ARGS"
    CMD_ARGS+=("${EXTRA[@]}")
fi

"$MTMD_CLI" "${CMD_ARGS[@]}"
