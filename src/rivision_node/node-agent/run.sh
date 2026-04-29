#!/bin/bash
# ============================================================
# RiVision Node Agent (Go) 启动脚本
# 支持预编译二进制缓存 (K3/x86)
# ============================================================

cd "$(dirname "$0")"

# 加载 .env
[ -f ".env" ] && source ".env"

# 环境变量
export AGENT_HOST=${AGENT_HOST:-"0.0.0.0"}
export AGENT_PORT=${AGENT_PORT:-"9090"}
export NODE_ID=${NODE_ID:-$(hostname)}
export LLAMA_SERVICE_NAME=${LLAMA_SERVICE_NAME:-"rivision-llama"}
export LLAMA_PORT=${LLAMA_PORT:-"9080"}
export GATEWAY_URL=${GATEWAY_URL:-"http://192.168.122.1:8081"}
export LOG_LEVEL=${LOG_LEVEL:-"INFO"}

BINARY="./node-agent-go"
ARCH=$(uname -m)
PREBUILT="./bin/node-agent-go.${ARCH}"

echo "启动 RiVision Node Agent (Go)..."
echo "  架构: $ARCH"
echo "  节点ID: $NODE_ID"
echo "  监听地址: $AGENT_HOST:$AGENT_PORT"
echo "  Gateway: $GATEWAY_URL"

# 检查二进制文件
if [ ! -x "$BINARY" ]; then
    # 优先使用预编译缓存
    if [ -f "$PREBUILT" ]; then
        echo "使用预编译二进制: $PREBUILT"
        cp "$PREBUILT" "$BINARY"
        chmod +x "$BINARY"
    else
        # 现场编译
        echo "编译 node-agent-go..."
        if command -v go &>/dev/null; then
            go build -ldflags "-s -w" -o "$BINARY" . || { echo "编译失败！"; exit 1; }
        elif command -v make &>/dev/null; then
            make build || { echo "编译失败！"; exit 1; }
        else
            echo "错误: 未找到 go 或 make，无法编译"
            exit 1
        fi
    fi
fi

exec "$BINARY"
