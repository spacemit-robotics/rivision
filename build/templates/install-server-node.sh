#!/bin/bash
# ============================================================
# RiVision Server Node (边缘节点) 安装脚本
# 组件: rivision_worker + rivision_embed + rivision_infer [+ rivision_owl]
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
    ip=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v '^$' | \
         grep -vE '^(172\.(1[6-9]|2[0-9]|3[01])\.|198\.18\.|10\.255\.)' | head -1)
    if [ -z "$ip" ]; then
        ip=$(ip -4 addr show scope global 2>/dev/null | \
             grep -v -E '(docker|br-|veth|virbr|tun|wg|tailscale)' | \
             grep -oP 'inet \K[0-9.]+' | head -1)
    fi
    [ -z "$ip" ] && ip=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+')
    echo "${ip:-127.0.0.1}"
}

echo ""
echo "=== RiVision Server Node Installation ==="

# OWL 是必选组件 (GB28181/ONVIF 流媒体)
if [ ! -d "$SCRIPT_DIR/owl/bin" ]; then
    echo -e "${RED}错误: OWL 组件未找到 ($SCRIPT_DIR/owl/bin)${NC}"
    echo "  OWL 是必选组件，用于 GB28181/ONVIF 流媒体接入"
    echo "  请确保发布包完整或重新构建: make release-server-node"
    exit 1
fi

echo "  组件: rivision_worker + rivision_embed + rivision_owl"
echo "  目标: $INSTALL_DIR"

check_sudo

# 升级检测
IS_UPGRADE=false
NEW_VERSION=""
[ -f "$SCRIPT_DIR/manifest.json" ] && NEW_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$SCRIPT_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')

if [ -d "$INSTALL_DIR/rivision_worker" ] && [ -f "$INSTALL_DIR/manifest.json" ]; then
    IS_UPGRADE=true
    PREV_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$INSTALL_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')
    echo -e "  类型: ${YELLOW}升级${NC} ($PREV_VERSION -> $NEW_VERSION)"
elif [ -d "$INSTALL_DIR/rivision_worker" ]; then
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

# 停止服务 (当前架构: worker + embed + owl + go2rtc)
STOP_SERVICES="rivision-worker rivision-embed rivision-owl rivision-go2rtc"
if $IS_UPGRADE; then
    echo "停止现有服务..."
    for svc in $STOP_SERVICES; do
        if [ -f "/etc/systemd/system/${svc}.service" ] && systemctl is-active --quiet "$svc" 2>/dev/null; then
            run_as_root systemctl stop "$svc" 2>/dev/null && \
                echo -e "  ${GREEN}已停止: $svc${NC}" || \
                echo -e "  ${YELLOW}停止超时: $svc${NC}"
        fi
    done
    # 确保关键进程已退出 (防止 Text file busy)
    for bin in rivision-worker rivision-embed-server go2rtc; do
        for i in $(seq 1 5); do
            pgrep -x "$bin" >/dev/null 2>&1 || break
            sleep 1
        done
        # 强制终止残留进程
        run_as_root pkill -9 -x "$bin" 2>/dev/null || true
    done
fi

# 安装
echo "[Server Node] 安装组件..."
run_as_root mkdir -p "$INSTALL_DIR/shared"
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"

# rivision_worker
if [ -f "$SCRIPT_DIR/bin/rivision-worker" ]; then
    echo "  安装 rivision_worker..."
    mkdir -p "$INSTALL_DIR/rivision_worker"/{bin,config,logs,data,cache}
    cp "$SCRIPT_DIR/bin/rivision-worker" "$INSTALL_DIR/rivision_worker/bin/"
    chmod +x "$INSTALL_DIR/rivision_worker/bin/"*
    # 安装配置 (不覆盖已有)
    if [ -f "$SCRIPT_DIR/config/worker.yaml" ] && [ ! -f "$INSTALL_DIR/rivision_worker/config/worker.yaml" ]; then
        cp "$SCRIPT_DIR/config/worker.yaml" "$INSTALL_DIR/rivision_worker/config/"
        echo "    已安装: config/worker.yaml"
    elif [ -f "$SCRIPT_DIR/config/worker.yaml" ]; then
        cp "$SCRIPT_DIR/config/worker.yaml" "$INSTALL_DIR/rivision_worker/config/worker.yaml.new"
        echo "    跳过: config/worker.yaml (保留现有配置)"
        echo "    提示: 新版配置已保存为 worker.yaml.new，如遇启动错误可对比合并:"
        echo "          diff $INSTALL_DIR/rivision_worker/config/worker.yaml{,.new}"
    fi
fi

# go2rtc (RTSP → WebRTC/fMP4 转换)
GO2RTC_BIN=""
if [ -f "$SCRIPT_DIR/bin/go2rtc.riscv64" ]; then
    GO2RTC_BIN="$SCRIPT_DIR/bin/go2rtc.riscv64"
elif [ -f "$SCRIPT_DIR/bin/go2rtc" ]; then
    GO2RTC_BIN="$SCRIPT_DIR/bin/go2rtc"
fi
if [ -n "$GO2RTC_BIN" ]; then
    echo "  安装 go2rtc..."
    mkdir -p "$INSTALL_DIR/rivision_worker/bin"
    cp "$GO2RTC_BIN" "$INSTALL_DIR/rivision_worker/bin/go2rtc"
    chmod +x "$INSTALL_DIR/rivision_worker/bin/go2rtc"
    echo "    已安装: bin/go2rtc ($(basename $GO2RTC_BIN))"
    # go2rtc 配置
    if [ -f "$SCRIPT_DIR/config/go2rtc.yaml" ] && [ ! -f "$INSTALL_DIR/rivision_worker/config/go2rtc.yaml" ]; then
        cp "$SCRIPT_DIR/config/go2rtc.yaml" "$INSTALL_DIR/rivision_worker/config/"
        echo "    已安装: config/go2rtc.yaml"
    elif [ -f "$SCRIPT_DIR/config/go2rtc.yaml" ]; then
        cp "$SCRIPT_DIR/config/go2rtc.yaml" "$INSTALL_DIR/rivision_worker/config/go2rtc.yaml.new"
        echo "    跳过: config/go2rtc.yaml (保留现有配置)"
    fi
fi

# rivision_embed (嵌入服务)
if [ -f "$SCRIPT_DIR/bin/rivision-embed-server" ]; then
    echo "  安装 rivision_embed..."
    mkdir -p "$INSTALL_DIR/rivision_embed"/{bin,config,logs,lib}
    cp "$SCRIPT_DIR/bin/rivision-embed-server" "$INSTALL_DIR/rivision_embed/bin/"
    chmod +x "$INSTALL_DIR/rivision_embed/bin/"*
    # 运行时库
    if [ -d "$SCRIPT_DIR/lib" ]; then
        cp "$SCRIPT_DIR"/lib/*.so* "$INSTALL_DIR/rivision_embed/lib/" 2>/dev/null || true
        run_as_root sh -c "echo '$INSTALL_DIR/rivision_embed/lib' > /etc/ld.so.conf.d/rivision-embed.conf"
        run_as_root ldconfig 2>/dev/null || true
        echo "    已安装: lib/ ($(ls "$INSTALL_DIR/rivision_embed/lib/"*.so* 2>/dev/null | wc -l) 个 .so)"
    fi
fi

# 模型目录 (yolo + embed + vlm，统一在 rivision_worker/models/)
echo "  安装模型..."

# 升级时清理旧模型 (防止跨平台或版本残留)
if $IS_UPGRADE; then
    echo "    清理旧模型..."
    rm -rf "$INSTALL_DIR/rivision_worker/models/yolo"/* 2>/dev/null || true
    rm -rf "$INSTALL_DIR/rivision_worker/models/vlm"/* 2>/dev/null || true
    # embed 模型不清理 (升级时复用)
fi
mkdir -p "$INSTALL_DIR/rivision_worker/models"/{yolo,embed,vlm}

# YOLO 模型 (K3: yolo11)
if [ -d "$SCRIPT_DIR/models/yolo" ]; then
    cp -r "$SCRIPT_DIR"/models/yolo/* "$INSTALL_DIR/rivision_worker/models/yolo/" 2>/dev/null || true
    YOLO_COUNT=$(find "$INSTALL_DIR/rivision_worker/models/yolo" -name "*.onnx" 2>/dev/null | wc -l)
    echo "    已安装: models/yolo/ ($YOLO_COUNT 个 .onnx)"
fi

# Embed 模型 (cn_clip)
if [ -d "$SCRIPT_DIR/models/embed" ]; then
    cp "$SCRIPT_DIR"/models/embed/*.onnx "$INSTALL_DIR/rivision_worker/models/embed/" 2>/dev/null || true
    EMBED_COUNT=$(ls "$INSTALL_DIR/rivision_worker/models/embed/"*.onnx 2>/dev/null | wc -l)
    echo "    已安装: models/embed/ ($EMBED_COUNT 个 .onnx)"
fi

# VLM 模型 (K3: fastvlm-mm-q4_1)
if [ -d "$SCRIPT_DIR/models/vlm" ]; then
    for d in "$SCRIPT_DIR"/models/vlm/*/; do
        [ -d "$d" ] && cp -r "$d" "$INSTALL_DIR/rivision_worker/models/vlm/"
    done
    VLM_COUNT=$(find "$INSTALL_DIR/rivision_worker/models/vlm" -maxdepth 1 -type d 2>/dev/null | wc -l)
    echo "    已安装: models/vlm/ ($((VLM_COUNT-1)) 个模型目录)"
fi

# ============================================================
# rivision_owl (OWL + ZLMediaKit) — 必选组件
# ============================================================
echo "  安装 rivision_owl..."
LOCAL_IP=$(get_local_ip)
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
# 注意: owl.toml.template 使用 ${VAR:-default} 语法，envsubst 不支持
# 必须用 sed 先替换特定变量，再用通配符替换剩余默认值
if [ -f "$SCRIPT_DIR/owl/config/owl.toml.template" ] && [ ! -f "$INSTALL_DIR/rivision_owl/config/config.toml" ]; then
    # OWL_SDP_IP: 用于 SDP 对外宣告，必须是本机可达的 LAN IP
    OWL_SDP_IP="${OWL_SDP_IP:-$LOCAL_IP}"
    OWL_MEDIA_IP="${OWL_MEDIA_IP:-127.0.0.1}"
    OWL_WEBHOOK_IP="${OWL_WEBHOOK_IP:-127.0.0.1}"
    OWL_LOG_DIR="${OWL_LOG_DIR:-/opt/rivision/rivision_owl/logs}"
    OWL_DATA_DIR="${OWL_DATA_DIR:-/opt/rivision/rivision_owl/data}"
    OWL_STORAGE_DIR="${OWL_STORAGE_DIR:-/opt/rivision/rivision_owl/recordings}"
    
    # 先替换关键变量，再用通配符替换所有 ${VAR:-default} 为 default
    cat "$SCRIPT_DIR/owl/config/owl.toml.template" | \
        sed "s|\${OWL_SDP_IP:-[^}]*}|$OWL_SDP_IP|g" | \
        sed "s|\${OWL_MEDIA_IP:-[^}]*}|$OWL_MEDIA_IP|g" | \
        sed "s|\${OWL_WEBHOOK_IP:-[^}]*}|$OWL_WEBHOOK_IP|g" | \
        sed "s|\${OWL_LOG_DIR:-[^}]*}|$OWL_LOG_DIR|g" | \
        sed "s|\${OWL_DATA_DIR:-[^}]*}|$OWL_DATA_DIR|g" | \
        sed "s|\${OWL_STORAGE_DIR:-[^}]*}|$OWL_STORAGE_DIR|g" | \
        sed 's|\${[^:}]*:-\([^}]*\)}|\1|g' > "$INSTALL_DIR/rivision_owl/config/config.toml"
    echo -e "    已渲染: config.toml (${GREEN}SDPIP=$OWL_SDP_IP${NC})"
elif [ -f "$INSTALL_DIR/rivision_owl/config/config.toml" ]; then
    echo "    跳过: config.toml (保留现有配置)"
fi

# systemd services (新架构: worker + embed + owl + go2rtc)
for svc in rivision-embed.service rivision-worker.service rivision-owl.service rivision-go2rtc.service; do
    [ -f "$SCRIPT_DIR/services/$svc" ] && \
        sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
            "$SCRIPT_DIR/services/$svc" | run_as_root tee /etc/systemd/system/"$svc" > /dev/null && \
        echo "  已安装: $svc"
done
run_as_root systemctl daemon-reload

# 清理旧服务 (升级时)
if $IS_UPGRADE; then
    for old_svc in rivision-yolo rivision-vlm rivision-infer; do
        if [ -f "/etc/systemd/system/${old_svc}.service" ]; then
            run_as_root systemctl disable "$old_svc" 2>/dev/null || true
            run_as_root rm -f "/etc/systemd/system/${old_svc}.service"
            echo "  已清理旧服务: $old_svc"
        fi
    done
    run_as_root systemctl daemon-reload
fi

# manifest
cp "$SCRIPT_DIR/manifest.json" "$INSTALL_DIR/" 2>/dev/null || true

echo ""
echo -e "${GREEN}=== Server Node 安装完成 ===${NC}"
[ -n "$NEW_VERSION" ] && echo "  版本: $NEW_VERSION"

# ============================================================
# .env 配置管理
# ============================================================
TEMPLATE_FILE="$SCRIPT_DIR/config/.env.server-node.template"
ENV_FILE="$INSTALL_DIR/shared/.env"

if [ -f "$ENV_FILE" ] && $IS_UPGRADE; then
    echo "升级 .env 配置..."
    # 备份用户配置
    SAVED_NODE_ID=$(grep "^NODE_ID=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_HUB_URL=$(grep "^HUB_URL=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_GATEWAY_URL=$(grep "^GATEWAY_URL=" "$ENV_FILE" | cut -d= -f2-)
    # 备份旧文件
    cp "$ENV_FILE" "$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
    # 使用新模板
    if [ -f "$TEMPLATE_FILE" ]; then
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        echo "  ✅ 已更新为新模板"
    fi
    # 恢复用户配置
    [ -n "$SAVED_NODE_ID" ] && \
        sed -i "s/^NODE_ID=.*/NODE_ID=$SAVED_NODE_ID/" "$ENV_FILE" && \
        echo "  ✅ 已保留 NODE_ID=$SAVED_NODE_ID"
    [ -n "$SAVED_HUB_URL" ] && \
        sed -i "s|^HUB_URL=.*|HUB_URL=$SAVED_HUB_URL|" "$ENV_FILE" && \
        echo "  ✅ 已保留 HUB_URL=$SAVED_HUB_URL"
    [ -n "$SAVED_GATEWAY_URL" ] && \
        sed -i "s|^GATEWAY_URL=.*|GATEWAY_URL=$SAVED_GATEWAY_URL|" "$ENV_FILE" && \
        echo "  ✅ 已保留 GATEWAY_URL=$SAVED_GATEWAY_URL"
else
    if [ -f "$TEMPLATE_FILE" ]; then
        mkdir -p "$INSTALL_DIR/shared"
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        NODE_HOSTNAME=$(hostname)
        sed -i "s/^NODE_ID=.*/NODE_ID=node-${NODE_HOSTNAME}/" "$ENV_FILE"
        echo -e "  已自动设置: NODE_ID=node-${NODE_HOSTNAME}"
        echo ""
        echo -e "${YELLOW}已创建 shared/.env，请检查配置:${NC}"
        echo -e "${RED}  ⚠️  必须修改 HUB_URL 和 GATEWAY_URL 为实际的 Server 地址${NC}"
        echo "  sudo vi $ENV_FILE"
        echo ""
        echo -e "${YELLOW}修改 .env 后，必须重启所有服务使配置生效:${NC}"
        echo "  sudo systemctl restart rivision-embed rivision-worker"
    fi
fi

# 启用服务 (新架构: owl → go2rtc → embed → worker)
echo ""
echo "启用开机自启..."
for svc in rivision-owl rivision-go2rtc rivision-embed rivision-worker; do
    run_as_root systemctl enable "$svc" 2>/dev/null && echo -e "  ${GREEN}已启用: $svc${NC}" || true
done

# 启动服务 (顺序: owl → embed → worker)
echo ""
if $IS_UPGRADE; then
    echo "重启服务..."
else
    echo "启动服务..."
fi

# 1. OWL (流媒体服务，必须先启动)
echo "  启动 rivision-owl..."
if run_as_root systemctl restart rivision-owl 2>/dev/null; then
    echo -e "  ${GREEN}已启动: rivision-owl${NC}"
    sleep 1
else
    echo -e "  ${RED}启动失败: rivision-owl${NC}"
    echo "  诊断: sudo journalctl -u rivision-owl -n 20"
fi

# 2. go2rtc (RTSP → WebRTC/fMP4 转换)
if [ -f /etc/systemd/system/rivision-go2rtc.service ]; then
    echo "  启动 rivision-go2rtc..."
    if run_as_root systemctl restart rivision-go2rtc 2>/dev/null; then
        echo -e "  ${GREEN}已启动: rivision-go2rtc${NC}"
        sleep 1
    else
        echo -e "  ${YELLOW}启动失败: rivision-go2rtc${NC}"
        echo "  诊断: sudo journalctl -u rivision-go2rtc -n 20"
    fi
fi

# 3. embed
echo "  启动 rivision-embed..."
if run_as_root systemctl restart rivision-embed 2>/dev/null; then
    echo -e "  ${GREEN}已启动: rivision-embed${NC}"
    sleep 1
else
    echo -e "  ${RED}启动失败: rivision-embed${NC}"
    echo "  诊断: sudo journalctl -u rivision-embed -n 20"
fi

# 4. worker (内置 YOLO + VLM 推理)
echo "  启动 rivision-worker..."
if run_as_root systemctl restart rivision-worker 2>/dev/null; then
    echo -e "  ${GREEN}已启动: rivision-worker${NC}"
else
    echo -e "  ${YELLOW}跳过: rivision-worker (启动失败)${NC}"
    echo "  诊断: sudo journalctl -u rivision-worker -n 20"
fi

# 权限
echo ""; echo "设置权限..."
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"
run_as_root find "$INSTALL_DIR" -type d -name "bin" -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chmod 755 {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -type d -name "lib" -exec chown -R root:root {} \; 2>/dev/null || true
for dir in logs data cache config shared models recordings www; do
    run_as_root find "$INSTALL_DIR" -type d -name "$dir" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
done

LOCAL_IP=$(get_local_ip)
echo ""
echo -e "${GREEN}=== 部署信息 ===${NC}"
echo "  Worker:  http://$LOCAL_IP:8080 (HTTP API, 内置 YOLO+VLM)"
echo "  go2rtc:  http://$LOCAL_IP:1984 (RTSP → WebRTC/fMP4)"
echo "  Embed:   http://$LOCAL_IP:8091 (CLIP 嵌入服务)"
echo "  OWL:     http://$LOCAL_IP:15123 (GB28181/ONVIF 流媒体)"
echo ""
echo "  模型目录: $INSTALL_DIR/rivision_worker/models/"
echo "    - yolo/   ($(find "$INSTALL_DIR/rivision_worker/models/yolo" -name "*.onnx" 2>/dev/null | wc -l) 个 .onnx)"
echo "    - embed/  ($(ls "$INSTALL_DIR/rivision_worker/models/embed/"*.onnx 2>/dev/null | wc -l) 个 .onnx)"
echo "    - vlm/    ($(find "$INSTALL_DIR/rivision_worker/models/vlm" -maxdepth 1 -type d 2>/dev/null | wc -l) 个目录)"
echo ""
echo "  管理:"
echo "    systemctl status rivision-owl"
echo "    systemctl status rivision-go2rtc"
echo "    systemctl status rivision-embed"
echo "    systemctl status rivision-worker"
echo "    sudo journalctl -u rivision-worker -f"
echo ""

echo -e "${GREEN}完成!${NC}"
