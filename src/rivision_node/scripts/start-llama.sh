#!/bin/bash
# ============================================================
# llama推理服务启动脚本 (由systemd调用)
# 读取 active-model.sh 配置，启动对应模型
# ============================================================

DEPLOY_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-model.sh"
ENV_FILE="$DEPLOY_DIR/config/.env"
MODELS_CONF="$DEPLOY_DIR/config/models.conf"
SWITCH_MODEL="$DEPLOY_DIR/scripts/switch-model.sh"

# 加载环境变量
[ -f "$ENV_FILE" ] && source "$ENV_FILE"

# 加载模型配置
if [ -f "$CONFIG" ]; then
    source "$CONFIG"
else
    MODEL_DIR="$DEPLOY_DIR/models"
    if [ -f "$MODELS_CONF" ] && [ -x "$SWITCH_MODEL" ]; then
        # 尝试通过 switch-model.sh 激活默认模型
        MODEL="${DEFAULT_MODEL:-}"
        if [ -z "$MODEL" ]; then
            # 自动检测: 优先 Qwen3VL (默认), 其次 fastvlm-mm (NPU), 最后 fastvlm-0.5b-q8 (CPU)
            if [ -d "$MODEL_DIR/Qwen3VL" ]; then
                MODEL="Qwen3VL"
            elif [ -d "$MODEL_DIR/fastvlm-mm-0.5b-q4_1" ]; then
                MODEL="fastvlm-mm-0.5b-q4_1"
            elif [ -d "$MODEL_DIR/fastvlm-0.5b-q8" ]; then
                MODEL="fastvlm-0.5b-q8"
            fi
        fi
        if [ -n "$MODEL" ]; then
            bash "$SWITCH_MODEL" "$MODEL" >/dev/null 2>&1 || true
        fi
    fi

    if [ -f "$CONFIG" ]; then
        source "$CONFIG"
        echo "[INFO] 已生成并加载 active-model.sh: ${ACTIVE_MODEL_NAME:-unknown}"
    else
        # switch-model 失败, 尝试直接检测模型文件
        if [ -f "$MODEL_DIR/fastvlm-0.5b-q8_0.gguf" ]; then
            ACTIVE_MODEL_NAME="fastvlm-0.5b-q8"
            ACTIVE_MODEL_FILE="$MODEL_DIR/fastvlm-0.5b-q8_0.gguf"
            ACTIVE_MMPROJ_FILE="$MODEL_DIR/fastvlm-0.5b-mmproj-f16.gguf"
        elif [ -f "$MODEL_DIR/fastvlm-mm-0.5b-q4_1/fastvlm-text-0.5B-Q4_1.gguf" ]; then
            ACTIVE_MODEL_NAME="fastvlm-mm-0.5b-q4_1"
            ACTIVE_MODEL_FILE="$MODEL_DIR/fastvlm-mm-0.5b-q4_1/fastvlm-text-0.5B-Q4_1.gguf"
            ACTIVE_MMPROJ_FILE="smt"
        elif [ -f "$MODEL_DIR/fastvlm-0.5b-q4_k_m.gguf" ]; then
            ACTIVE_MODEL_NAME="fastvlm-0.5b-q4"
            ACTIVE_MODEL_FILE="$MODEL_DIR/fastvlm-0.5b-q4_k_m.gguf"
            ACTIVE_MMPROJ_FILE="$MODEL_DIR/fastvlm-0.5b-mmproj-f16.gguf"
        else
            FIRST_GGUF=$(find "$MODEL_DIR" -maxdepth 2 -name "*.gguf" -type f 2>/dev/null | head -1)
            if [ -n "$FIRST_GGUF" ]; then
                ACTIVE_MODEL_NAME=$(basename "$FIRST_GGUF" .gguf)
                ACTIVE_MODEL_FILE="$FIRST_GGUF"
                ACTIVE_MMPROJ_FILE=""
            else
                echo "❌ 未找到模型文件，请先运行: scripts/switch-model.sh <model>"
                exit 1
            fi
        fi
        echo "[INFO] 自动选择模型: $ACTIVE_MODEL_NAME"
    fi
fi

# 检查模型文件
if [ ! -f "$ACTIVE_MODEL_FILE" ]; then
    echo "❌ 模型文件不存在: $ACTIVE_MODEL_FILE"
    exit 1
fi

# 确定llama-server路径
LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
LLAMA_SERVER="$LLAMA_ENGINE_DIR/llama-server"
if [ ! -f "$LLAMA_SERVER" ]; then
    # fallback: llama-mtmd-cli
    LLAMA_SERVER="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
fi

if [ ! -x "$LLAMA_SERVER" ]; then
    echo "❌ llama-server 不存在或不可执行: $LLAMA_SERVER"
    echo "   请检查 engines/llama.cpp/ 目录"
    exit 1
fi

# 设置共享库路径 (加载 lib/ 下的 libllama.so, libonnxruntime.so 等)
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

# 日志目录
LOG_DIR="$DEPLOY_DIR/logs"
mkdir -p "$LOG_DIR"

# 设置 OpenMP 环境变量 (从 systemd service 移到此处, 因为 systemd Environment= 不展开 EnvironmentFile 变量)
# K3 A100 智算核有 8 核 (索引 8-15)，默认使用 8 线程
export OMP_NUM_THREADS="${LLAMA_THREADS:-8}"

# ★ K3: SpacemiT llama.cpp 内部已实现 A100 核心绑定 (cpu_mask: ff00)
# 不需要外部设置 GOMP_CPU_AFFINITY，否则会与内部绑定冲突导致:
#   "libgomp: Thread creation failed: Invalid argument"
# 仅在明确设置 LLAMA_CPU_AFFINITY 时才覆盖
if [ "$(uname -m)" = "riscv64" ]; then
    echo "[LLAMA] K3 SpacemiT: A100 cores 8-15 (internal binding)"
    # 仅当用户明确设置时才启用外部亲和性
    if [ -n "${LLAMA_CPU_AFFINITY:-}" ]; then
        export GOMP_CPU_AFFINITY="$LLAMA_CPU_AFFINITY"
        echo "[LLAMA] Override affinity: cores ${LLAMA_CPU_AFFINITY}"
    fi
elif [ -n "${LLAMA_CPU_AFFINITY:-}" ]; then
    export GOMP_CPU_AFFINITY="$LLAMA_CPU_AFFINITY"
    echo "[LLAMA] OpenMP affinity: ${GOMP_CPU_AFFINITY}"
fi

# 构建启动参数
THREADS="${LLAMA_THREADS:-8}"
BATCH_THREADS="${LLAMA_BATCH_THREADS:-8}"
PORT="${LLAMA_PORT:-9080}"
CTX="${LLAMA_CTX_SIZE:-4096}"
PARALLEL="${LLAMA_PARALLEL:-1}"
# CPU 亲和性 (K3 不需要 taskset，SpacemiT 内部已处理)
CPU_AFFINITY="${LLAMA_CPU_AFFINITY:-}"
FLASH_ATTN="${LLAMA_FLASH_ATTN:-true}"

CMD_ARGS=(
    --host 0.0.0.0
    --port "$PORT"
    -m "$ACTIVE_MODEL_FILE"
    -t "$THREADS"
    -tb "$BATCH_THREADS"
    -c "$CTX"
    -np "$PARALLEL"
    -b 2048
    -ub 512
    --cont-batching
    --log-timestamps
)

# 多模态后端: smt 模式 (NPU) 或 mmproj 模式 (CPU)
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    # SMT/NPU 模式: --media-backend smt --smt-config-dir <模型目录>
    # 模型目录需包含 config.json + .gguf + .onnx
    SMT_CONFIG_DIR="$(dirname "$ACTIVE_MODEL_FILE")"
    CMD_ARGS+=(--media-backend smt --smt-config-dir "$SMT_CONFIG_DIR")
elif [ -n "$ACTIVE_MMPROJ_FILE" ] && [ -f "$ACTIVE_MMPROJ_FILE" ]; then
    # CPU mmproj 模式
    CMD_ARGS+=(--mmproj "$ACTIVE_MMPROJ_FILE")
fi

# Flash Attention
if [ "$FLASH_ATTN" = "true" ]; then
    CMD_ARGS+=(--flash-attn on)
fi

# 模型特定的额外参数
if [ -n "$ACTIVE_EXTRA_ARGS" ]; then
    read -ra EXTRA <<< "$ACTIVE_EXTRA_ARGS"
    CMD_ARGS+=("${EXTRA[@]}")
fi

# 注意: llama-server 不支持 --system-prompt 参数
# MiniCPM 的 system prompt 需要由客户端通过 /v1/chat/completions API 发送
# 格式: {"messages": [{"role": "system", "content": "..."}, {"role": "user", "content": "..."}]}

echo "=== RiVision Llama Service ==="
echo "  模型: $ACTIVE_MODEL_NAME"
echo "  文件: $ACTIVE_MODEL_FILE"
if [ "$ACTIVE_MMPROJ_FILE" = "smt" ]; then
    echo "  模式: NPU (smt)"
elif [ -n "$ACTIVE_MMPROJ_FILE" ]; then
    echo "  模式: CPU (mmproj)"
fi
echo "  端口: $PORT"
echo "  线程: $THREADS"
echo "================================"

# 启动 (带可选CPU绑核)
if [ -n "$CPU_AFFINITY" ]; then
    exec taskset -c "$CPU_AFFINITY" "$LLAMA_SERVER" "${CMD_ARGS[@]}" \
        2>&1 | tee -a "$LOG_DIR/llama-$(date +%Y%m%d).log"
else
    exec "$LLAMA_SERVER" "${CMD_ARGS[@]}" \
        2>&1 | tee -a "$LOG_DIR/llama-$(date +%Y%m%d).log"
fi
