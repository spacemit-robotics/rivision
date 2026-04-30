#!/bin/bash
# ============================================================
# Node Agent 启动脚本 (由systemd调用)
# ============================================================

DEPLOY_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="$DEPLOY_DIR/config/.env"

# 加载环境变量
[ -f "$ENV_FILE" ] && source "$ENV_FILE"

# 参数
HOST="${AGENT_HOST:-0.0.0.0}"
PORT="${AGENT_PORT:-9090}"

# 日志目录
LOG_DIR="$DEPLOY_DIR/logs"
mkdir -p "$LOG_DIR"

# Go 二进制
BINARY="$DEPLOY_DIR/bin/rivision_node"
if [ ! -x "$BINARY" ]; then
    echo "❌ rivision_node 不存在或不可执行: $BINARY"
    echo "   请检查 bin/ 目录"
    exit 1
fi

echo "=== RiVision Node Agent ==="
echo "  地址: $HOST:$PORT"
echo "  Gateway: ${GATEWAY_URL:-未配置}"
echo "==========================="

exec "$BINARY" 2>&1 | tee -a "$LOG_DIR/node-agent-$(date +%Y%m%d).log"
