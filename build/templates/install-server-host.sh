#!/bin/bash
# ============================================================
# RiVision Server 安装脚本
# 组件: rivision_hub + rivision_gateway + rivision_owl + rivision_app_web
#
# 用法: ./install.sh
# ============================================================
set -e

INSTALL_DIR="/opt/rivision"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RIVISION_USER="$(whoami)"
RIVISION_GROUP="$(id -gn)"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'

run_as_root() {
    if [ "$EUID" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

check_sudo() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${YELLOW}部分操作需要管理员权限，请输入密码:${NC}"
        sudo -v || { echo -e "${RED}无法获取 sudo 权限${NC}"; exit 1; }
    fi
}

get_local_ip() {
    # Prefer physical LAN IP; skip docker/libvirt/tunnel/VPN interfaces
    local ip=""
    # Method 1: hostname -I first word (usually the primary LAN address)
    ip=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v '^$' | \
         grep -vE '^(172\.(1[6-9]|2[0-9]|3[01])\.|198\.18\.|10\.255\.)' | head -1)
    # Method 2: ip addr on non-virtual interfaces
    if [ -z "$ip" ]; then
        ip=$(ip -4 addr show scope global 2>/dev/null | \
             grep -v -E '(docker|br-|veth|virbr|tun|wg|tailscale)' | \
             grep -oP 'inet \K[0-9.]+' | head -1)
    fi
    # Method 3: fallback to ip route
    if [ -z "$ip" ]; then
        ip=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+')
    fi
    echo "${ip:-127.0.0.1}"
}

echo ""
echo "=== RiVision Server Installation ==="
echo "  组件: rivision_hub + rivision_gateway + rivision_owl"
echo "  目标: $INSTALL_DIR"

check_sudo

# 升级检测
IS_UPGRADE=false
NEW_VERSION=""
[ -f "$SCRIPT_DIR/manifest.json" ] && NEW_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$SCRIPT_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')

if [ -d "$INSTALL_DIR" ] && [ -f "$INSTALL_DIR/manifest.json" ]; then
    IS_UPGRADE=true
    PREV_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$INSTALL_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')
    echo -e "  类型: ${YELLOW}升级${NC} ($PREV_VERSION -> $NEW_VERSION)"
elif [ -d "$INSTALL_DIR/rivision_hub" ] || [ -d "$INSTALL_DIR/rivision_gateway" ]; then
    IS_UPGRADE=true
    echo -e "  类型: ${YELLOW}覆盖安装${NC}"
else
    echo "  类型: 全新安装"
fi
echo ""

# 校验
if [ -f "$SCRIPT_DIR/checksums.txt" ]; then
    echo "验证校验和..."
    cd "$SCRIPT_DIR"
    sha256sum -c checksums.txt --quiet 2>/dev/null || echo -e "${YELLOW}WARNING: 部分校验失败${NC}"
fi

# 停止服务
SERVER_SERVICES="rivision-hub rivision-gateway rivision-owl"
if $IS_UPGRADE; then
    echo "停止现有服务..."
    for svc in $SERVER_SERVICES; do
        systemctl is-active --quiet "$svc" 2>/dev/null && run_as_root systemctl stop "$svc" 2>/dev/null && echo "  停止: $svc" || true
    done
fi

# 安装
echo "[Server] 安装组件..."
run_as_root mkdir -p "$INSTALL_DIR/shared"
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"

LOCAL_IP=$(get_local_ip)

# ============================================================
# rivision_hub (含 rivision_app_web)
# ============================================================
if [ -f "$SCRIPT_DIR/bin/rivision-hub" ]; then
    echo "  安装 rivision_hub..."
    mkdir -p "$INSTALL_DIR/rivision_hub"/{bin,config,web,logs,data}
    cp "$SCRIPT_DIR/bin/rivision-hub" "$INSTALL_DIR/rivision_hub/bin/"
    chmod +x "$INSTALL_DIR/rivision_hub/bin/"*
    # 安装 Web UI (rivision_app_web)
    if [ -d "$SCRIPT_DIR/web" ]; then
        # 清理旧版前端残留 chunk
        rm -rf "$INSTALL_DIR/rivision_hub/web/assets" 2>/dev/null || true
        cp -r "$SCRIPT_DIR/web/"* "$INSTALL_DIR/rivision_hub/web/" 2>/dev/null || true
        echo "    已安装: web/ (rivision_app_web)"
    fi
    # 安装配置 (仅首次)
    if [ -f "$SCRIPT_DIR/config/hub.yaml" ] && [ ! -f "$INSTALL_DIR/rivision_hub/config/hub.yaml" ]; then
        cp "$SCRIPT_DIR/config/hub.yaml" "$INSTALL_DIR/rivision_hub/config/"
        echo "    已安装: config/hub.yaml"
    elif [ -f "$SCRIPT_DIR/config/hub.yaml" ]; then
        echo "    跳过: config/hub.yaml (保留现有配置)"
    fi
fi

# ============================================================
# rivision_gateway
# ============================================================
if [ -f "$SCRIPT_DIR/bin/rivision_gateway" ]; then
    echo "  安装 rivision_gateway..."
    mkdir -p "$INSTALL_DIR/rivision_gateway"/{bin,config,logs,data/cache}
    cp "$SCRIPT_DIR/bin/rivision_gateway" "$INSTALL_DIR/rivision_gateway/bin/"
    chmod +x "$INSTALL_DIR/rivision_gateway/bin/"*
fi

# ============================================================
# rivision_owl (OWL + ZLMediaKit)
# ============================================================
if [ -d "$SCRIPT_DIR/owl/bin" ]; then
    echo "  安装 rivision_owl..."
    mkdir -p "$INSTALL_DIR/rivision_owl"/{bin,config,data,logs,recordings}
    # 核心二进制
    cp "$SCRIPT_DIR/owl/bin/owl" "$INSTALL_DIR/rivision_owl/bin/"
    cp "$SCRIPT_DIR/owl/bin/ZLMediaKit" "$INSTALL_DIR/rivision_owl/bin/"
    chmod +x "$INSTALL_DIR/rivision_owl/bin/"*
    echo "    已安装: owl + ZLMediaKit"
    # OWL Web UI
    if [ -d "$SCRIPT_DIR/owl/www" ]; then
        mkdir -p "$INSTALL_DIR/rivision_owl/www"
        cp -r "$SCRIPT_DIR/owl/www/"* "$INSTALL_DIR/rivision_owl/www/" 2>/dev/null || true
        echo "    已安装: owl/www/ (OWL Web UI)"
    fi
    # 配置: 从模板渲染 (仅首次安装)
    if [ -f "$SCRIPT_DIR/owl/config/owl.toml.template" ] && [ ! -f "$INSTALL_DIR/rivision_owl/config/config.toml" ]; then
        # 设置渲染所需的环境变量默认值
        export OWL_SDP_IP="${OWL_SDP_IP:-$LOCAL_IP}"
        export OWL_MEDIA_IP="${OWL_MEDIA_IP:-127.0.0.1}"
        export OWL_WEBHOOK_IP="${OWL_WEBHOOK_IP:-127.0.0.1}"
        export OWL_LOG_DIR="${OWL_LOG_DIR:-/opt/rivision/rivision_owl/logs}"
        export OWL_DATA_DIR="${OWL_DATA_DIR:-/opt/rivision/rivision_owl/data}"
        export OWL_STORAGE_DIR="${OWL_STORAGE_DIR:-/opt/rivision/rivision_owl/recordings}"
        # envsubst 渲染模板
        if command -v envsubst &>/dev/null; then
            envsubst < "$SCRIPT_DIR/owl/config/owl.toml.template" > "$INSTALL_DIR/rivision_owl/config/config.toml"
            echo "    已渲染: config.toml (OWL_SDP_IP=$OWL_SDP_IP)"
        else
            # envsubst 不可用时用 sed 替换关键变量
            cp "$SCRIPT_DIR/owl/config/owl.toml.template" "$INSTALL_DIR/rivision_owl/config/config.toml"
            sed -i "s|\${OWL_SDP_IP:-127.0.0.1}|$LOCAL_IP|g" "$INSTALL_DIR/rivision_owl/config/config.toml"
            sed -i "s|\${OWL_LOG_DIR:-/opt/rivision-owl/logs}|/opt/rivision/rivision_owl/logs|g" "$INSTALL_DIR/rivision_owl/config/config.toml"
            sed -i "s|\${OWL_DATA_DIR:-/opt/rivision-owl/data}|/opt/rivision/rivision_owl/data|g" "$INSTALL_DIR/rivision_owl/config/config.toml"
            sed -i "s|\${OWL_STORAGE_DIR:-/opt/rivision-owl/recordings}|/opt/rivision/rivision_owl/recordings|g" "$INSTALL_DIR/rivision_owl/config/config.toml"
            echo "    已安装: config.toml (sed 模式, OWL_SDP_IP=$LOCAL_IP)"
        fi
    elif [ -f "$INSTALL_DIR/rivision_owl/config/config.toml" ]; then
        echo "    跳过: config.toml (保留现有配置)"
    fi
fi

# ============================================================
# systemd services
# ============================================================
for svc in rivision-hub.service rivision-gateway.service rivision-owl.service; do
    [ -f "$SCRIPT_DIR/services/$svc" ] && \
        sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" \
            -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
            -e "s/OWL_SDP_IP=192.168.1.107/OWL_SDP_IP=$LOCAL_IP/" \
            "$SCRIPT_DIR/services/$svc" | run_as_root tee /etc/systemd/system/"$svc" > /dev/null && \
        echo "  已安装: $svc"
done
run_as_root systemctl daemon-reload

# manifest
cp "$SCRIPT_DIR/manifest.json" "$INSTALL_DIR/" 2>/dev/null || true

echo ""
echo -e "${GREEN}=== Server 安装完成 ===${NC}"
[ -n "$NEW_VERSION" ] && echo "  版本: $NEW_VERSION"

# ============================================================
# .env 配置管理
# ============================================================
TEMPLATE_FILE="$SCRIPT_DIR/config/.env.server.template"
ENV_FILE="$INSTALL_DIR/shared/.env"

if [ -f "$ENV_FILE" ] && $IS_UPGRADE; then
    echo "升级 .env 配置..."
    # 备份
    cp "$ENV_FILE" "$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
    # 保留用户自定义值
    SAVED_GATEWAY_PORT=$(grep "^GATEWAY_PORT=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_HUB_PORT=$(grep "^HUB_PORT=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_OWL_SDP_IP=$(grep "^OWL_SDP_IP=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_ZLM_SECRET=$(grep "^ZLM_SECRET=" "$ENV_FILE" | cut -d= -f2-)
    # 使用新模板
    if [ -f "$TEMPLATE_FILE" ]; then
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        echo "  已更新为新模板"
    fi
    # 恢复自定义值
    [ -n "$SAVED_GATEWAY_PORT" ] && [ "$SAVED_GATEWAY_PORT" != "9090" ] && \
        sed -i "s/^GATEWAY_PORT=.*/GATEWAY_PORT=$SAVED_GATEWAY_PORT/" "$ENV_FILE" && \
        echo "  已保留 GATEWAY_PORT=$SAVED_GATEWAY_PORT"
    [ -n "$SAVED_HUB_PORT" ] && [ "$SAVED_HUB_PORT" != "9280" ] && \
        sed -i "s/^HUB_PORT=.*/HUB_PORT=$SAVED_HUB_PORT/" "$ENV_FILE" && \
        echo "  已保留 HUB_PORT=$SAVED_HUB_PORT"
    [ -n "$SAVED_OWL_SDP_IP" ] && \
        sed -i "s/^OWL_SDP_IP=.*/OWL_SDP_IP=$SAVED_OWL_SDP_IP/" "$ENV_FILE" && \
        echo "  已保留 OWL_SDP_IP=$SAVED_OWL_SDP_IP"
    [ -n "$SAVED_ZLM_SECRET" ] && [ "$SAVED_ZLM_SECRET" != "eMkolBseFWMPJ8LgmRX2AVveRVtyZ0eN" ] && \
        sed -i "s/^ZLM_SECRET=.*/ZLM_SECRET=$SAVED_ZLM_SECRET/" "$ENV_FILE" && \
        echo "  已保留 ZLM_SECRET"
else
    if [ -f "$TEMPLATE_FILE" ]; then
        mkdir -p "$INSTALL_DIR/shared"
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        # 自动填入探测到的本机 IP 作为 OWL_SDP_IP
        if [ -n "$LOCAL_IP" ] && [ "$LOCAL_IP" != "127.0.0.1" ]; then
            sed -i "s/^OWL_SDP_IP=$/OWL_SDP_IP=$LOCAL_IP/" "$ENV_FILE"
        fi
        echo -e "${YELLOW}已创建 shared/.env，请检查配置:${NC}"
        echo "  vi $ENV_FILE"
    fi
fi

# 启用服务
echo ""
echo "启用开机自启..."
for svc in rivision-gateway rivision-owl rivision-hub; do
    run_as_root systemctl enable "$svc" 2>/dev/null && echo -e "  ${GREEN}已启用: $svc${NC}" || true
done

# 启动服务 (顺序: gateway → owl → hub)
echo ""
if $IS_UPGRADE; then
    echo "重启服务..."
else
    echo "启动服务..."
fi
for svc in rivision-gateway rivision-owl rivision-hub; do
    if run_as_root systemctl restart "$svc" 2>/dev/null; then
        echo -e "  ${GREEN}已启动: $svc${NC}"
    else
        echo -e "  ${YELLOW}跳过: $svc (服务未安装或启动失败)${NC}"
    fi
done

# 权限
echo ""; echo "设置权限..."
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"
run_as_root find "$INSTALL_DIR" -type d -name "bin" -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chmod 755 {} \; 2>/dev/null || true
for dir in logs data config shared web www recordings; do
    run_as_root find "$INSTALL_DIR" -type d -name "$dir" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
done

echo ""
echo -e "${GREEN}=== 部署信息 ===${NC}"
echo "  Hub:     http://$LOCAL_IP:9280"
echo "  Gateway: http://$LOCAL_IP:9090"
echo "  OWL:     http://$LOCAL_IP:15123"
echo "  Web UI:  http://$LOCAL_IP:9280 (内嵌于 Hub)"
echo ""
echo "  管理:"
echo "    systemctl status rivision-hub"
echo "    systemctl status rivision-gateway"
echo "    systemctl status rivision-owl"
echo "    sudo journalctl -u rivision-hub -f"
echo "    sudo journalctl -u rivision-owl -f"
echo ""

echo -e "${GREEN}完成!${NC}"
