#!/bin/bash
# ============================================================
# RiVision Host 安装脚本
# 组件: rivision-cli + rivision_gateway + rivision-embed + rivision-owl
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
    local ip=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+')
    [ -z "$ip" ] && ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    echo "${ip:-127.0.0.1}"
}

echo ""
echo "=== RiVision Host Installation ==="
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
elif [ -d "$INSTALL_DIR" ]; then
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
HOST_SERVICES="rivision-cli rivision-gateway rivision-embed rivision-owl"
if $IS_UPGRADE; then
    echo "停止现有服务..."
    for svc in $HOST_SERVICES; do
        systemctl is-active --quiet "$svc" 2>/dev/null && run_as_root systemctl stop "$svc" 2>/dev/null && echo "  停止: $svc" || true
    done
fi

# 安装
echo "[Host] 安装组件..."
run_as_root mkdir -p "$INSTALL_DIR/shared"
# ★ 递归 chown 整个目录（确保升级安装时子目录也可写）
# shared/ 存放 .env 配置，用户需要编辑权限，设为 user 合理
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"

# rivision_cli
if [ -f "$SCRIPT_DIR/bin/rivision-cli" ]; then
    echo "  安装 rivision_cli..."
    mkdir -p "$INSTALL_DIR/rivision_cli"/{bin,config,logs,mp4path}
    cp "$SCRIPT_DIR/bin/rivision-cli" "$INSTALL_DIR/rivision_cli/bin/"
    # 复制预置的 go2rtc 和 rtsp_server（避免运行时提取需要写权限）
    cp "$SCRIPT_DIR/bin/go2rtc-"* "$INSTALL_DIR/rivision_cli/bin/" 2>/dev/null || true
    cp "$SCRIPT_DIR/bin/rtsp_server-"* "$INSTALL_DIR/rivision_cli/bin/" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/rivision_cli/bin/"*
    # 复制测试视频文件
    if [ -d "$SCRIPT_DIR/mp4path" ] && ls "$SCRIPT_DIR/mp4path/"*.mp4 >/dev/null 2>&1; then
        cp "$SCRIPT_DIR/mp4path/"*.mp4 "$INSTALL_DIR/rivision_cli/mp4path/" 2>/dev/null || true
        echo "    已复制测试视频到 mp4path/"
    fi
fi

# rivision_gateway
if [ -f "$SCRIPT_DIR/bin/rivision_gateway" ]; then
    echo "  安装 rivision_gateway..."
    mkdir -p "$INSTALL_DIR/rivision_gateway"/{bin,config,logs}
    cp "$SCRIPT_DIR/bin/rivision_gateway" "$INSTALL_DIR/rivision_gateway/bin/"
    chmod +x "$INSTALL_DIR/rivision_gateway/bin/"*
fi

# rivision_embed
if [ -f "$SCRIPT_DIR/bin/rivision-embed-server" ]; then
    echo "  安装 rivision_embed..."
    mkdir -p "$INSTALL_DIR/rivision_embed"/{bin,config,logs,models,lib}
    cp "$SCRIPT_DIR/bin/rivision-embed-server" "$INSTALL_DIR/rivision_embed/bin/"
    [ -d "$SCRIPT_DIR/models/embed" ] && cp "$SCRIPT_DIR"/models/embed/*.onnx "$INSTALL_DIR/rivision_embed/models/" 2>/dev/null || true
    if [ -d "$SCRIPT_DIR/lib" ]; then
        cp "$SCRIPT_DIR"/lib/*.so* "$INSTALL_DIR/rivision_embed/lib/" 2>/dev/null || true
        run_as_root sh -c "echo '$INSTALL_DIR/rivision_embed/lib' > /etc/ld.so.conf.d/rivision-embed.conf"
        run_as_root ldconfig 2>/dev/null || true
    fi
    chmod +x "$INSTALL_DIR/rivision_embed/bin/"*
fi

# rivision_owl
if [ -d "$SCRIPT_DIR/owl" ]; then
    echo "  安装 rivision_owl..."
    mkdir -p "$INSTALL_DIR/rivision_owl"/{bin,config,logs,data,recordings,mp4path}
    cp -r "$SCRIPT_DIR"/owl/* "$INSTALL_DIR/rivision_owl/"
    chmod +x "$INSTALL_DIR/rivision_owl/bin/"* 2>/dev/null || true
    # ★ 检查并打印 mp4path 复制日志
    if [ -d "$INSTALL_DIR/rivision_owl/mp4path" ] && ls "$INSTALL_DIR/rivision_owl/mp4path/"*.mp4 >/dev/null 2>&1; then
        echo "    已复制测试视频到 mp4path/"
    fi
fi

# systemd services
for svc in rivision-gateway.service rivision-cli.service rivision-embed.service rivision-owl.service; do
    [ -f "$SCRIPT_DIR/services/$svc" ] && \
        sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
            "$SCRIPT_DIR/services/$svc" | run_as_root tee /etc/systemd/system/"$svc" > /dev/null && \
        echo "  已安装: $svc"
done
run_as_root systemctl daemon-reload

# manifest
cp "$SCRIPT_DIR/manifest.json" "$INSTALL_DIR/" 2>/dev/null || true

echo ""
echo -e "${GREEN}=== Host 安装完成 ===${NC}"
[ -n "$NEW_VERSION" ] && echo "  版本: $NEW_VERSION"

# ============================================================
# .env 配置管理
# ★ 升级策略：保留用户配置，使用新模板
#    保留: OWL_SDP_IP, OWL_USER, OWL_PASS, ZLM_SECRET
#    更新: 其他所有配置使用新模板的值
# ============================================================
TEMPLATE_FILE="$SCRIPT_DIR/config/.env.template"
ENV_FILE="$INSTALL_DIR/shared/.env"
LOCAL_IP=$(get_local_ip)

if [ -f "$ENV_FILE" ] && $IS_UPGRADE; then
    # ===== 升级安装：保留用户配置，使用新模板 =====
    echo "升级 .env 配置..."
    
    # 1. 备份用户配置
    SAVED_OWL_SDP_IP=$(grep "^OWL_SDP_IP=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_OWL_USER=$(grep "^OWL_USER=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_OWL_PASS=$(grep "^OWL_PASS=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_ZLM_SECRET=$(grep "^ZLM_SECRET=" "$ENV_FILE" | cut -d= -f2-)
    
    # 2. 备份旧文件
    cp "$ENV_FILE" "$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
    
    # 3. 使用新模板
    if [ -f "$TEMPLATE_FILE" ]; then
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        echo "  ✅ 已更新为新模板"
    fi
    
    # 4. 恢复用户配置
    if [ -n "$SAVED_OWL_SDP_IP" ]; then
        sed -i "s/^OWL_SDP_IP=.*/OWL_SDP_IP=$SAVED_OWL_SDP_IP/" "$ENV_FILE"
        echo "  ✅ 已保留 OWL_SDP_IP=$SAVED_OWL_SDP_IP"
    fi
    if [ -n "$SAVED_OWL_USER" ] && [ "$SAVED_OWL_USER" != "admin" ]; then
        sed -i "s/^OWL_USER=.*/OWL_USER=$SAVED_OWL_USER/" "$ENV_FILE"
        echo "  ✅ 已保留 OWL_USER"
    fi
    if [ -n "$SAVED_OWL_PASS" ] && [ "$SAVED_OWL_PASS" != "admin" ]; then
        sed -i "s/^OWL_PASS=.*/OWL_PASS=$SAVED_OWL_PASS/" "$ENV_FILE"
        echo "  ✅ 已保留 OWL_PASS"
    fi
    if [ -n "$SAVED_ZLM_SECRET" ]; then
        sed -i "s/^ZLM_SECRET=.*/ZLM_SECRET=$SAVED_ZLM_SECRET/" "$ENV_FILE"
        echo "  ✅ 已保留 ZLM_SECRET"
    fi

else
    # ===== 首次安装：复制模板 =====
    if [ -f "$TEMPLATE_FILE" ]; then
        mkdir -p "$INSTALL_DIR/shared"
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        [ -n "$LOCAL_IP" ] && [ "$LOCAL_IP" != "127.0.0.1" ] && \
            sed -i "s/^OWL_SDP_IP=.*/OWL_SDP_IP=$LOCAL_IP/" "$ENV_FILE"
        echo -e "${YELLOW}已创建 shared/.env，请检查配置:${NC}"
        echo "  vi $ENV_FILE"
    fi
fi

# 启用服务
echo ""
echo "启用开机自启..."
for svc in rivision-embed rivision-gateway rivision-cli rivision-owl; do
    run_as_root systemctl enable "$svc" 2>/dev/null && echo -e "  ${GREEN}已启用: $svc${NC}" || true
done

# 启动/重启服务
echo ""
if $IS_UPGRADE; then
    echo "重启服务..."
else
    echo "启动服务..."
fi
for svc in rivision-embed rivision-gateway rivision-cli rivision-owl; do
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
run_as_root find "$INSTALL_DIR" -type d -name "lib" -exec chown -R root:root {} \; 2>/dev/null || true
for dir in logs data recordings mp4path models config shared; do
    run_as_root find "$INSTALL_DIR" -type d -name "$dir" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
done

# 创建 gw-shell 快捷命令
echo ""; echo "创建快捷命令..."
CLI_BIN="$INSTALL_DIR/rivision_cli/bin/rivision-cli"
if [ -f "$CLI_BIN" ]; then
    run_as_root ln -sf "$CLI_BIN" /usr/local/bin/gw-shell
    echo -e "  ${GREEN}已创建: gw-shell -> $CLI_BIN${NC}"
    echo "  现在可以直接使用: gw-shell <command>"
fi

echo -e "${GREEN}完成!${NC}"
