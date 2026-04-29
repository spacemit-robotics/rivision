#!/bin/bash
# ============================================================
# YOLO推理服务启动脚本 (由systemd调用)
# 读取 active-yolo-model.sh 配置，启动 YOLO Server
# ============================================================

DEPLOY_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG="$DEPLOY_DIR/config/active-yolo-model.sh"
ENV_FILE="$DEPLOY_DIR/config/.env"

# 加载环境变量
[ -f "$ENV_FILE" ] && source "$ENV_FILE"

# 加载模型配置 (如果存在)
if [ -f "$CONFIG" ]; then
    source "$CONFIG"
fi

# 路径配置
YOLO_BIN="$DEPLOY_DIR/bin/yolo-server"
MODEL_DIR="$DEPLOY_DIR/models"

# 设置运行时库路径
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:${LD_LIBRARY_PATH:-}"

# ★ K3 YOLO 配置 (根据实际测试优化)
# K3 实际能力: 4 并发帧, 总吞吐 15fps, 单帧 ~267ms
# YOLO_WORKERS=4 → 4 个并发 Session (最大化吞吐量)
# YOLO_THREADS=4 → 每 Worker 4 线程 (共享全局线程池)
# 全局线程池: SPACEMIT_EP_USE_GLOBAL_INTRA_THREAD=1
if [ "$(uname -m)" = "riscv64" ]; then
    export YOLO_WORKERS="${YOLO_WORKERS:-4}"
    export YOLO_THREADS="${YOLO_THREADS:-4}"
    echo "[YOLO] K3 SpacemiT: Workers=${YOLO_WORKERS}, Threads=${YOLO_THREADS} (4并发, 15fps吞吐)"
else
    export YOLO_WORKERS="${YOLO_WORKERS:-4}"
    export YOLO_THREADS="${YOLO_THREADS:-4}"
fi

# 配置优先级: active-yolo-model.sh > .env > 默认值
# 使用0.0.0.0允许Gateway直接访问
YOLO_HOST="${YOLO_HOST:-0.0.0.0}"
YOLO_PORT="${YOLO_PORT:-9081}"

# 模型配置 (优先使用 active-yolo-model.sh)
if [ -n "$ACTIVE_YOLO_FILE" ] && [ -f "$ACTIVE_YOLO_FILE" ]; then
    MODEL_PATH="$ACTIVE_YOLO_FILE"
    YOLO_CONF_THRESH="${ACTIVE_YOLO_CONF_THRESH:-0.5}"
    YOLO_NMS_THRESH="${ACTIVE_YOLO_NMS_THRESH:-0.45}"
    YOLO_MODEL_NAME="${ACTIVE_YOLO_NAME:-unknown}"
else
    # 回退到 .env 或默认配置
    # 默认模型：RISC-V 优先使用 K3 量化模型
    if [[ "$(uname -m)" == "riscv64" ]] && [ -f "$MODEL_DIR/910byolo11n_b2.q.onnx" ]; then
        YOLO_MODEL="${YOLO_MODEL:-910byolo11n_b2.q.onnx}"
    else
        YOLO_MODEL="${YOLO_MODEL:-yolov8n.onnx}"
    fi
    MODEL_PATH="$MODEL_DIR/$YOLO_MODEL"
    YOLO_CONF_THRESH="${YOLO_CONF_THRESH:-0.5}"
    YOLO_NMS_THRESH="${YOLO_NMS_THRESH:-0.45}"
    YOLO_MODEL_NAME="$YOLO_MODEL"
fi

# 日志目录
LOG_DIR="$DEPLOY_DIR/logs"
mkdir -p "$LOG_DIR"

# 检查可执行文件
if [ ! -x "$YOLO_BIN" ]; then
    echo "❌ YOLO Server 不存在或不可执行: $YOLO_BIN"
    echo "   请检查 bin/ 目录"
    exit 1
fi

# 检查模型文件
if [ ! -f "$MODEL_PATH" ]; then
    echo "❌ 模型文件不存在: $MODEL_PATH"
    echo "   请运行: scripts/switch-yolo-model.sh --list"
    echo "   或下载: scripts/switch-yolo-model.sh --download yolov8n.onnx"
    exit 1
fi

echo "============================================================"
echo "  RiVision YOLO Server"
echo "============================================================"
echo "  模型名称:   $YOLO_MODEL_NAME"
echo "  模型文件:   $MODEL_PATH"
echo "  监听地址:   $YOLO_HOST:$YOLO_PORT"
echo "  置信度阈值: $YOLO_CONF_THRESH"
echo "  NMS阈值:    $YOLO_NMS_THRESH"
echo "  Workers:    $YOLO_WORKERS"
echo "  Threads:    $YOLO_THREADS"
echo "============================================================"

# 启动 YOLO Server (不绑定核心，让系统自动调度)
exec "$YOLO_BIN" \
    --model "$MODEL_PATH" \
    --host "$YOLO_HOST" \
    --port "$YOLO_PORT" \
    --conf-thresh "$YOLO_CONF_THRESH" \
    --nms-thresh "$YOLO_NMS_THRESH"
