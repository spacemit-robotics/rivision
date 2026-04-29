#!/bin/bash
# GB28181 视频流测试脚本
# 使用方法:
#   1. 运行此脚本启动 gb28181-sim
#   2. 在 OWL Web UI (http://127.0.0.1:15123) 登录并点击播放
#   3. 脚本会自动检测 INVITE 并验证视频流
#
# 所有 ZLM 参数通过环境变量覆盖，默认值与 ZLM 二进制内置配置一致
# ZLM 默认端口: 8220, ZLM 默认密钥: eMkolBseFWMPJ8LgmRX2AVveRVtyZ0eN

SIM_DIR="$(cd "$(dirname "$0")/../gb28181-sim" && pwd)"

# ZLM 配置（环境变量优先，默认与 ZLM 二进制内置配置一致）
ZLM_HTTP_PORT="${ZLM_HTTP_PORT:-8220}"
ZLM_SECRET="${ZLM_SECRET:-eMkolBseFWMPJ8LgmRX2AVveRVtyZ0eN}"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║           GB28181 视频流测试                                  ║"
echo "╠══════════════════════════════════════════════════════════════╣"
echo "║  ZLM:  127.0.0.1:$ZLM_HTTP_PORT (HTTP), 5544 (RTSP)            ║"
echo "║  KEY:  ${ZLM_SECRET:0:8}...（前8位）                              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# Kill existing
pkill -9 gb28181-sim 2>/dev/null; sleep 1

# Start simulator
cd "$SIM_DIR"
./gb28181-sim \
  --server-ip 127.0.0.1 \
  --server-port 15060 \
  --server-id 34020000002000000001 \
  --agent-id 34020000001320000001 \
  --agent-password 12345678 \
  --channel "34020000001310000001:test" \
  --verbose 2>&1 | tee /tmp/gb28181_sim.log &
SIM_PID=$!

echo "✓ gb28181-sim 已启动 (PID: $SIM_PID)"
echo ""
echo "请在 OWL Web UI 中点击播放按钮触发视频流"
echo "OWL Web UI: http://127.0.0.1:15123"
echo ""
echo "等待 INVITE 请求..."
echo "按 Ctrl+C 退出"
echo ""

# Monitor for INVITE and stream
while true; do
    if grep -q "INVITE" /tmp/gb28181_sim.log 2>/dev/null; then
        echo "✓ 收到 INVITE 请求!"
        echo ""
        # Check for stream in ZLM（带 secret 认证）
        sleep 3
        STREAMS=$(curl -s "http://127.0.0.1:$ZLM_HTTP_PORT/index/api/getMediaList?secret=$ZLM_SECRET" 2>/dev/null)
        if echo "$STREAMS" | grep -q "stream"; then
            echo "✓ 视频流已建立!"
            STREAM_ID=$(echo "$STREAMS" | grep -o '"stream":"[^"]*"' | head -1 | cut -d'"' -f4)
            APP=$(echo "$STREAMS" | grep -o '"app":"[^"]*"' | head -1 | cut -d'"' -f4)
            echo ""
            echo "播放命令:"
            echo "  mpv rtsp://127.0.0.1:5544/$APP/$STREAM_ID"
            echo "  mpv http://127.0.0.1:$ZLM_HTTP_PORT/$APP/$STREAM_ID.live.flv"
        fi
        break
    fi
    sleep 1
done

# Keep running
wait $SIM_PID