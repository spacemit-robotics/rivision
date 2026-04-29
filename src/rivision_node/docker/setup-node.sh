#!/bin/bash
# ============================================================
# RiVision Node 快速配置
#
# 用法:
#   bash docker/setup-node.sh <NODE_ID> <GATEWAY_URL> [TOKEN]
#   bash docker/setup-node.sh k3-office-01 http://192.168.1.100:8081 mytoken
#   bash docker/setup-node.sh --show
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"

# ─── 显示当前配置 ──────────────────────────────────────────
if [ "${1:-}" = "--show" ]; then
    [ -f "$ENV_FILE" ] || { echo ".env 不存在"; exit 1; }
    echo "=== 节点配置 ==="
    grep -v '^#' "$ENV_FILE" | grep -v '^$'
    exit 0
fi

# ─── 参数检查 ──────────────────────────────────────────────
NODE_ID="${1:-}"
GATEWAY_URL="${2:-}"
REGISTRATION_TOKEN="${3:-}"

if [ -z "$NODE_ID" ] || [ -z "$GATEWAY_URL" ]; then
    echo "用法: bash docker/setup-node.sh <NODE_ID> <GATEWAY_URL> [TOKEN]"
    echo ""
    echo "示例:"
    echo "  bash docker/setup-node.sh k3-office-01 http://192.168.1.100:8081"
    echo "  bash docker/setup-node.sh k3-office-01 http://192.168.1.100:8081 mytoken"
    echo ""
    echo "查看: bash docker/setup-node.sh --show"
    exit 1
fi

# ─── 生成 .env ─────────────────────────────────────────────
cat > "$ENV_FILE" << EOF
# RiVision Node: ${NODE_ID}  ($(date '+%Y-%m-%d %H:%M:%S'))

# ── 节点标识 (必填) ──
NODE_ID=${NODE_ID}
GATEWAY_URL=${GATEWAY_URL}
REGISTRATION_TOKEN=${REGISTRATION_TOKEN}

# ── 网络 ──
NODE_HOST=auto
NODE_TAGS=riscv,k3,docker
HEARTBEAT_INTERVAL=10

# ── 推理服务 ──
LLAMA_PORT=9080
LLAMA_THREADS=8
LLAMA_BATCH_THREADS=8
LLAMA_CPU_AFFINITY=
LLAMA_CTX_SIZE=4096
LLAMA_FLASH_ATTN=true

# ── Node Agent ──
AGENT_PORT=9090
LOG_LEVEL=INFO

# ── 模型 ──
DEFAULT_MODEL=fastvlm-0.5b-q8
MODELS_DIR=../models
EOF

echo "✓ 配置已生成: $ENV_FILE"
echo "  NODE_ID=$NODE_ID"
echo "  GATEWAY_URL=$GATEWAY_URL"
echo "  TOKEN=${REGISTRATION_TOKEN:-(未设置)}"
echo ""
echo "下一步:"
echo "  docker compose -f docker/docker-compose.node.k3.yml up -d"
