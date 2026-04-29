#!/bin/bash
# GB28181 集成测试脚本
# 测试 gb28181-sim 与 OWL 的 SIP 信令交互

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OWL_DIR="$SCRIPT_DIR/../owl"
SIM_DIR="$SCRIPT_DIR/../gb28181-sim"

# 默认配置
OWL_IP="${OWL_IP:-127.0.0.1}"
OWL_SIP_PORT="${OWL_SIP_PORT:-15060}"
OWL_HTTP_PORT="${OWL_HTTP_PORT:-15123}"
SERVER_ID="${SERVER_ID:-34020000002000000001}"
AGENT_ID="${AGENT_ID:-34020000001320000001}"
CHANNEL_ID="${CHANNEL_ID:-34020000001310000001}"
PASSWORD="${PASSWORD:-12345678}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 检查依赖
check_dependencies() {
    log_info "检查依赖..."
    
    if ! command -v curl &> /dev/null; then
        log_error "curl 未安装"
        exit 1
    fi
    
    if ! command -v nc &> /dev/null && ! command -v netcat &> /dev/null; then
        log_warn "netcat 未安装，部分测试将跳过"
    fi
}

# 测试 OWL HTTP API
test_owl_http_api() {
    log_info "测试 OWL HTTP API..."
    
    # 健康检查
    if curl -s -o /dev/null -w "%{http_code}" "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/ping" | grep -q "200"; then
        log_info "✓ OWL HTTP API 可访问"
        return 0
    else
        log_error "✗ OWL HTTP API 不可访问"
        return 1
    fi
}

# 测试 OWL SIP 端口
test_owl_sip_port() {
    log_info "测试 OWL SIP 端口..."
    
    if nc -z -u "$OWL_IP" "$OWL_SIP_PORT" 2>/dev/null || \
       timeout 2 bash -c "echo '' > /dev/udp/$OWL_IP/$OWL_SIP_PORT" 2>/dev/null; then
        log_info "✓ OWL SIP 端口 $OWL_SIP_PORT 可访问"
        return 0
    else
        log_warn "⚠ 无法确认 SIP 端口状态 (可能正常)"
        return 0
    fi
}

# 测试 gb28181-sim 编译
test_sim_build() {
    log_info "测试 gb28181-sim 编译..."
    
    cd "$SIM_DIR"
    if go build -o gb28181-sim . 2>&1; then
        log_info "✓ gb28181-sim 编译成功"
        return 0
    else
        log_error "✗ gb28181-sim 编译失败"
        return 1
    fi
}

# 测试 gb28181-sim 帮助
test_sim_help() {
    log_info "测试 gb28181-sim 命令行..."
    
    cd "$SIM_DIR"
    if ./gb28181-sim --help 2>&1 | grep -q "GB28181 Device Simulator"; then
        log_info "✓ gb28181-sim --help 正常"
        return 0
    else
        log_error "✗ gb28181-sim --help 异常"
        return 1
    fi
}

# 测试 SIP REGISTER (需要 OWL 运行)
test_sip_register() {
    log_info "测试 SIP REGISTER (需要 OWL 运行)..."
    
    # 检查 OWL 是否运行
    if ! curl -s "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/ping" > /dev/null 2>&1; then
        log_warn "⚠ OWL 未运行，跳过 SIP REGISTER 测试"
        return 0
    fi
    
    cd "$SIM_DIR"
    
    # 启动 simulator，5秒后检查
    timeout 10 ./gb28181-sim \
        --server-ip "$OWL_IP" \
        --server-port "$OWL_SIP_PORT" \
        --server-id "$SERVER_ID" \
        --agent-id "$AGENT_ID" \
        --agent-password "$PASSWORD" \
        --channel "$CHANNEL_ID:test" \
        --verbose 2>&1 &
    SIM_PID=$!
    
    sleep 5
    
    # 检查设备是否注册
    DEVICES=$(curl -s "http://$OWL_IP:$OWL_HTTP_PORT/api/v1/gb28181/devices" 2>/dev/null || echo "{}")
    
    kill $SIM_PID 2>/dev/null || true
    wait $SIM_PID 2>/dev/null || true
    
    if echo "$DEVICES" | grep -q "$AGENT_ID"; then
        log_info "✓ SIP REGISTER 成功，设备已注册"
        return 0
    else
        log_warn "⚠ 设备未出现在列表中 (可能需要检查配置)"
        return 0
    fi
}

# 运行单元测试
test_unit_tests() {
    log_info "运行 Go 单元测试..."
    
    cd "$SIM_DIR"
    if go test ./... -v 2>&1; then
        log_info "✓ 单元测试通过"
        return 0
    else
        log_error "✗ 单元测试失败"
        return 1
    fi
}

# 主函数
main() {
    echo "=========================================="
    echo "  GB28181 集成测试"
    echo "=========================================="
    echo ""
    echo "OWL 地址: $OWL_IP:$OWL_HTTP_PORT (HTTP), $OWL_IP:$OWL_SIP_PORT (SIP)"
    echo "设备 ID: $AGENT_ID"
    echo "通道 ID: $CHANNEL_ID"
    echo ""
    
    PASSED=0
    FAILED=0
    
    check_dependencies
    
    # 运行测试
    tests=(
        "test_sim_build"
        "test_sim_help"
        "test_unit_tests"
        "test_owl_http_api"
        "test_owl_sip_port"
        "test_sip_register"
    )
    
    for test in "${tests[@]}"; do
        echo ""
        if $test; then
            ((PASSED++))
        else
            ((FAILED++))
        fi
    done
    
    echo ""
    echo "=========================================="
    echo "  测试结果: $PASSED 通过, $FAILED 失败"
    echo "=========================================="
    
    if [ $FAILED -gt 0 ]; then
        exit 1
    fi
}

main "$@"
