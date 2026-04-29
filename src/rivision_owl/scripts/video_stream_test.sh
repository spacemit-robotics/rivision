#!/bin/bash
# GB28181 视频流测试脚本 (Go 版本)
# 测试流程: gb28181-sim -> OWL (SIP) -> ZLMediaKit (RTP) -> mpv 播放
#
# 所有 ZLM 参数通过环境变量覆盖，默认值与 ZLM 二进制内置配置一致
# ZLM 默认端口: 8220, ZLM 默认密钥: eMkolBseFWMPJ8LgmRX2AVveRVtyZ0eN

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SIM_DIR="$SCRIPT_DIR/../gb28181-sim"
OWL_DIR="$SCRIPT_DIR/../owl"
ZLM_DIR="$SCRIPT_DIR/../zlmediakit/prebuilt/x86_64"

# 配置（所有值均可通过环境变量覆盖，默认与 ZLM 二进制内置配置一致）
OWL_IP="${OWL_IP:-127.0.0.1}"
OWL_SIP_PORT="${OWL_SIP_PORT:-15060}"
OWL_HTTP_PORT="${OWL_HTTP_PORT:-15123}"
# ZLM HTTP API 端口（默认 8220，与 ZLM 预编译二进制内置值一致）
ZLM_HTTP_PORT="${ZLM_HTTP_PORT:-8220}"
# ZLM HTTP API 密钥（必须与 ZLM 二进制内置配置一致）
ZLM_SECRET="${ZLM_SECRET:-eMkolBseFWMPJ8LgmRX2AVveRVtyZ0eN}"

SERVER_ID="${SERVER_ID:-34020000002000000001}"
AGENT_ID="${AGENT_ID:-34020000001320000001}"
CHANNEL_ID="${CHANNEL_ID:-34020000001310000001}"
PASSWORD="${PASSWORD:-12345678}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }

cleanup() {
    log_info "清理测试进程..."
    [ -n "$SIM_PID" ] && kill $SIM_PID 2>/dev/null || true
    [ -n "$MPV_PID" ] && kill $MPV_PID 2>/dev/null || true
}
trap cleanup EXIT

# 检查服务
check_services() {
    log_step "检查服务状态..."

    if ! curl -s "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/ping" > /dev/null 2>&1; then
        log_error "OWL 服务未运行 (http://$OWL_IP:$OWL_HTTP_PORT)"
        return 1
    fi
    log_info "✓ OWL 服务正常"

    if ! curl -s "http://$OWL_IP:$ZLM_HTTP_PORT/index/api/getServerConfig?secret=$ZLM_SECRET" 2>/dev/null | grep -q "code"; then
        log_error "ZLMediaKit 服务未运行 (http://$OWL_IP:$ZLM_HTTP_PORT)"
        return 1
    fi
    log_info "✓ ZLMediaKit 服务正常"

    if ! command -v ffmpeg &> /dev/null; then
        log_error "FFmpeg 未安装"
        return 1
    fi
    log_info "✓ FFmpeg 已安装"

    if ! command -v mpv &> /dev/null; then
        log_warn "mpv 未安装，将跳过播放测试"
    else
        log_info "✓ mpv 已安装"
    fi

    return 0
}

# 启动模拟器
start_simulator() {
    local source="${1:-test}"
    local channels="${2:-1}"

    log_step "启动 gb28181-sim (源: $source, 通道数: $channels)..."

    cd "$SIM_DIR"

    # 构建通道参数
    local channel_args=""
    for i in $(seq 1 $channels); do
        local ch_id=$(printf "340200000013100000%02d" $i)
        channel_args="$channel_args --channel $ch_id:$source"
    done

    ./gb28181-sim \
        --server-ip "$OWL_IP" \
        --server-port "$OWL_SIP_PORT" \
        --server-id "$SERVER_ID" \
        --agent-id "$AGENT_ID" \
        --agent-password "$PASSWORD" \
        $channel_args \
        --verbose &
    SIM_PID=$!

    sleep 3

    if ! kill -0 $SIM_PID 2>/dev/null; then
        log_error "模拟器启动失败"
        return 1
    fi

    log_info "✓ 模拟器已启动 (PID: $SIM_PID)"
    return 0
}

# 触发视频点播
trigger_play() {
    local channel_id="${1:-$CHANNEL_ID}"

    log_step "触发视频点播 (通道: $channel_id)..."

    # 登录获取 token
    local login_resp=$(curl -s -X POST "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/login" \
        -H "Content-Type: application/json" \
        -d '{"username":"admin","password":"admin"}' 2>/dev/null)

    local token=$(echo "$login_resp" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

    if [ -z "$token" ]; then
        log_warn "无法获取 token，尝试无认证请求..."
    fi

    # 请求点播
    local play_resp=$(curl -s -X POST "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/gb28181/play" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $token" \
        -d "{\"device_id\":\"$AGENT_ID\",\"channel_id\":\"$channel_id\"}" 2>/dev/null)

    log_info "点播响应: $play_resp"

    # 等待流建立
    sleep 2

    return 0
}

# 获取流地址
get_stream_url() {
    log_step "获取流媒体地址..."

    # 从 ZLMediaKit 获取流列表（带 secret 认证）
    local streams=$(curl -s "http://$OWL_IP:$ZLM_HTTP_PORT/index/api/getMediaList?secret=$ZLM_SECRET" 2>/dev/null)

    # 解析流地址
    local stream_id=$(echo "$streams" | grep -o '"stream":"[^"]*"' | head -1 | cut -d'"' -f4)
    local app=$(echo "$streams" | grep -o '"app":"[^"]*"' | head -1 | cut -d'"' -f4)

    if [ -n "$stream_id" ] && [ -n "$app" ]; then
        # 构建 RTSP URL
        RTSP_URL="rtsp://$OWL_IP:5544/$app/$stream_id"
        # 构建 HTTP-FLV URL
        FLV_URL="http://$OWL_IP:$ZLM_HTTP_PORT/$app/$stream_id.live.flv"
        # 构建 HLS URL
        HLS_URL="http://$OWL_IP:$ZLM_HTTP_PORT/$app/$stream_id/hls.m3u8"

        log_info "RTSP: $RTSP_URL"
        log_info "FLV:  $FLV_URL"
        log_info "HLS:  $HLS_URL"
        return 0
    else
        log_warn "未找到活跃流，使用默认地址..."
        # 使用 GB28181 默认流格式
        RTSP_URL="rtsp://$OWL_IP:5544/rtp/$AGENT_ID_$CHANNEL_ID"
        FLV_URL="http://$OWL_IP:$ZLM_HTTP_PORT/rtp/${AGENT_ID}_${CHANNEL_ID}.live.flv"
        return 1
    fi
}

# 播放视频
play_video() {
    local url="$1"
    local duration="${2:-10}"

    if ! command -v mpv &> /dev/null; then
        log_warn "mpv 未安装，跳过播放"
        return 0
    fi

    log_step "使用 mpv 播放视频 (${duration}秒)..."
    log_info "URL: $url"

    timeout $duration mpv --no-video --really-quiet "$url" 2>/dev/null &
    MPV_PID=$!

    sleep $duration

    if kill -0 $MPV_PID 2>/dev/null; then
        log_info "✓ 视频播放正常"
        kill $MPV_PID 2>/dev/null
        return 0
    else
        log_warn "⚠ 视频播放结束"
        return 0
    fi
}

# 单路测试
test_single_channel() {
    log_info ""
    log_info "═══════════════════════════════════════════════════════════════"
    log_info "  测试 1: 单路视频流 (测试图案)"
    log_info "═══════════════════════════════════════════════════════════════"

    start_simulator "test" 1
    sleep 3
    trigger_play "$CHANNEL_ID"
    get_stream_url

    # 检查流是否存在（带 secret 认证）
    local streams=$(curl -s "http://$OWL_IP:$ZLM_HTTP_PORT/index/api/getMediaList?secret=$ZLM_SECRET" 2>/dev/null)
    if echo "$streams" | grep -q "rtp"; then
        log_info "✓ 单路视频流测试成功"
    else
        log_warn "⚠ 未检测到视频流 (可能 INVITE 未触发)"
    fi

    kill $SIM_PID 2>/dev/null || true
    sleep 2
}

# 多路测试
test_multi_channel() {
    log_info ""
    log_info "═══════════════════════════════════════════════════════════════"
    log_info "  测试 2: 多路视频流 (3 通道)"
    log_info "═══════════════════════════════════════════════════════════════"

    start_simulator "test" 3
    sleep 3

    # 触发多路点播
    for i in 1 2 3; do
        local ch_id=$(printf "340200000013100000%02d" $i)
        log_info "请求通道 $ch_id..."
        trigger_play "$ch_id" &
    done
    wait

    sleep 3

    # 检查流数量（带 secret 认证）
    local streams=$(curl -s "http://$OWL_IP:$ZLM_HTTP_PORT/index/api/getMediaList?secret=$ZLM_SECRET" 2>/dev/null)
    local count=$(echo "$streams" | grep -o '"stream"' | wc -l)

    log_info "活跃流数量: $count"

    if [ "$count" -ge 1 ]; then
        log_info "✓ 多路视频流测试成功"
    else
        log_warn "⚠ 未检测到视频流"
    fi

    kill $SIM_PID 2>/dev/null || true
    sleep 2
}

# 主函数
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║          GB28181 视频流测试 (Go 版本)                             ║"
    echo "╠══════════════════════════════════════════════════════════════════╣"
    echo "║  OWL:  $OWL_IP:$OWL_HTTP_PORT (HTTP), $OWL_SIP_PORT (SIP)          ║"
    echo "║  ZLM:  $OWL_IP:$ZLM_HTTP_PORT (HTTP), 5544 (RTSP)                  ║"
    echo "║  KEY:  ${ZLM_SECRET:0:8}...（前8位）                                  ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo ""

    # 检查服务
    if ! check_services; then
        log_error "服务检查失败，退出"
        exit 1
    fi

    # 编译检查
    cd "$SIM_DIR"
    if ! go build -o gb28181-sim . 2>&1; then
        log_error "gb28181-sim 编译失败"
        exit 1
    fi
    log_info "✓ gb28181-sim 编译成功"

    # 运行测试
    test_single_channel
    test_multi_channel

    echo ""
    echo "╔══════════════════════════════════════════════════════════════════╗"
    echo "║                      测试完成                                     ║"
    echo "╚══════════════════════════════════════════════════════════════════╝"
    echo ""
    echo "手动播放命令:"
    echo "  mpv rtsp://$OWL_IP:5544/rtp/\${STREAM_ID}"
    echo "  mpv http://$OWL_IP:$ZLM_HTTP_PORT/rtp/\${STREAM_ID}.live.flv"
    echo ""
}

main "$@"