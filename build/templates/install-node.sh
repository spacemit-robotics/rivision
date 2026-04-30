#!/bin/bash
# ============================================================
# RiVision Node 安装脚本
# 组件: node-agent + yolo-server + llama.cpp + 模型
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

echo ""
echo "=== RiVision Node Installation ==="
echo "  目标: $INSTALL_DIR/rivision_node"

check_sudo

# 升级检测
IS_UPGRADE=false
NEW_VERSION=""
[ -f "$SCRIPT_DIR/manifest.json" ] && NEW_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$SCRIPT_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')

if [ -d "$INSTALL_DIR/rivision_node" ] && [ -f "$INSTALL_DIR/rivision_node/manifest.json" ]; then
    IS_UPGRADE=true
    PREV_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$INSTALL_DIR/rivision_node/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')
    echo -e "  类型: ${YELLOW}升级${NC} ($PREV_VERSION -> $NEW_VERSION)"
elif [ -d "$INSTALL_DIR/rivision_node" ]; then
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
# ★ 停止顺序：先停 node-agent，再停推理服务
NODE_SERVICES="rivision-node-agent rivision-llama rivision-yolo"
if $IS_UPGRADE; then
    echo "停止现有服务..."
    for svc in $NODE_SERVICES; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            run_as_root systemctl stop "$svc" 2>/dev/null
            echo -e "  ${GREEN}已停止: $svc${NC}"
        elif systemctl list-unit-files "$svc.service" 2>/dev/null | grep -q "$svc"; then
            echo -e "  ${YELLOW}已停止: $svc (未运行)${NC}"
        fi
    done
fi

# 安装
echo "[Node] 安装组件..."
run_as_root mkdir -p "$INSTALL_DIR"
run_as_root chown "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"

mkdir -p "$INSTALL_DIR/rivision_node"/{bin,lib,engines/llama.cpp,models,config,scripts,logs}

# bin
if [ -d "$SCRIPT_DIR/bin" ]; then
    cp "$SCRIPT_DIR"/bin/* "$INSTALL_DIR/rivision_node/bin/" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/rivision_node/bin/"* 2>/dev/null || true
    echo "  已安装: bin/ ($(ls "$SCRIPT_DIR/bin/" 2>/dev/null | tr '\n' ' '))"
fi

# lib
if [ -d "$SCRIPT_DIR/lib" ]; then
    cp "$SCRIPT_DIR"/lib/* "$INSTALL_DIR/rivision_node/lib/" 2>/dev/null || true
    run_as_root sh -c "echo '$INSTALL_DIR/rivision_node/lib' > /etc/ld.so.conf.d/rivision-node.conf"
    run_as_root ldconfig 2>/dev/null || true
    echo "  已安装: lib/ ($(ls "$INSTALL_DIR/rivision_node/lib/"*.so* 2>/dev/null | wc -l) 个 .so)"
fi

# engines
if [ -d "$SCRIPT_DIR/engines/llama.cpp" ]; then
    cp "$SCRIPT_DIR"/engines/llama.cpp/* "$INSTALL_DIR/rivision_node/engines/llama.cpp/" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/rivision_node/engines/llama.cpp/"* 2>/dev/null || true
    echo "  已安装: engines/llama.cpp/"
fi

# models
if [ -d "$SCRIPT_DIR/models" ]; then
    cp -r "$SCRIPT_DIR"/models/* "$INSTALL_DIR/rivision_node/models/" 2>/dev/null || true
    echo "  已安装: models/ ($(ls "$INSTALL_DIR/rivision_node/models/" 2>/dev/null | tr '\n' ' '))"
fi

# config
if [ -d "$SCRIPT_DIR/config" ]; then
    cp -r "$SCRIPT_DIR"/config/* "$INSTALL_DIR/rivision_node/config/" 2>/dev/null || true
    echo "  已安装: config/"
fi

# scripts
if [ -d "$SCRIPT_DIR/scripts" ]; then
    cp "$SCRIPT_DIR"/scripts/*.sh "$INSTALL_DIR/rivision_node/scripts/" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/rivision_node/scripts/"*.sh 2>/dev/null || true
    echo "  已安装: scripts/"
fi

# systemd services
if [ -d "$SCRIPT_DIR/services" ]; then
    for svc in "$SCRIPT_DIR"/services/*.service; do
        [ -f "$svc" ] || continue
        SVC_NAME=$(basename "$svc")
        sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
            "$svc" | run_as_root tee /etc/systemd/system/"$SVC_NAME" > /dev/null
        echo "  已安装: $SVC_NAME"
    done
    run_as_root systemctl daemon-reload
fi

# manifest (保存到 node 目录)
cp "$SCRIPT_DIR/manifest.json" "$INSTALL_DIR/rivision_node/" 2>/dev/null || true

echo ""
echo -e "${GREEN}=== Node 安装完成 ===${NC}"
[ -n "$NEW_VERSION" ] && echo "  版本: $NEW_VERSION"

# ============================================================
# .env 配置管理
# ★ 升级策略：保留用户配置，使用新模板
#    保留: NODE_ID, GATEWAY_URL, REGISTRATION_TOKEN
#    更新: 其他所有配置使用新模板的值
# ============================================================
TEMPLATE_FILE="$SCRIPT_DIR/config/.env.node.template"
ENV_FILE="$INSTALL_DIR/rivision_node/config/.env"

if [ -f "$ENV_FILE" ] && $IS_UPGRADE; then
    # ===== 升级安装：保留用户配置，使用新模板 =====
    echo "升级 .env 配置..."
    
    # 1. 备份用户配置
    SAVED_NODE_ID=$(grep "^NODE_ID=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_GATEWAY_URL=$(grep "^GATEWAY_URL=" "$ENV_FILE" | cut -d= -f2-)
    SAVED_REGISTRATION_TOKEN=$(grep "^REGISTRATION_TOKEN=" "$ENV_FILE" | cut -d= -f2-)
    
    # 2. 备份旧文件
    cp "$ENV_FILE" "$ENV_FILE.bak.$(date +%Y%m%d%H%M%S)"
    
    # 3. 使用新模板
    if [ -f "$TEMPLATE_FILE" ]; then
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        echo "  ✅ 已更新为新模板"
    fi
    
    # 4. 恢复用户配置
    if [ -n "$SAVED_NODE_ID" ]; then
        sed -i "s/^NODE_ID=.*/NODE_ID=$SAVED_NODE_ID/" "$ENV_FILE"
        echo "  ✅ 已保留 NODE_ID=$SAVED_NODE_ID"
    fi
    if [ -n "$SAVED_GATEWAY_URL" ]; then
        sed -i "s|^GATEWAY_URL=.*|GATEWAY_URL=$SAVED_GATEWAY_URL|" "$ENV_FILE"
        echo "  ✅ 已保留 GATEWAY_URL=$SAVED_GATEWAY_URL"
    fi
    if [ -n "$SAVED_REGISTRATION_TOKEN" ]; then
        sed -i "s/^REGISTRATION_TOKEN=.*/REGISTRATION_TOKEN=$SAVED_REGISTRATION_TOKEN/" "$ENV_FILE"
        echo "  ✅ 已保留 REGISTRATION_TOKEN"
    fi
    
else
    # ===== 首次安装：复制模板 =====
    if [ -f "$TEMPLATE_FILE" ]; then
        cp "$TEMPLATE_FILE" "$ENV_FILE"
        NODE_HOSTNAME=$(hostname)
        sed -i "s/^NODE_ID=.*/NODE_ID=node-${NODE_HOSTNAME}/" "$ENV_FILE"
        echo -e "  已自动设置: NODE_ID=node-${NODE_HOSTNAME}"
        echo ""
        echo -e "${YELLOW}已创建 config/.env，请检查配置:${NC}"
        echo -e "${RED}  ⚠️  必须修改 GATEWAY_URL 为实际的 Gateway 地址${NC}"
        echo "  vi $INSTALL_DIR/rivision_node/config/.env"
    fi
fi

# 检测节点类型并决定启用哪些服务
# 从模板读取配置 (确保 node_llama/node_yolo 正确识别)
LLAMA_ENABLED=true
YOLO_ENABLED=true

if [ -f "$TEMPLATE_FILE" ]; then
    grep -q "^LLAMA_ENABLED=false" "$TEMPLATE_FILE" && LLAMA_ENABLED=false
    grep -q "^YOLO_ENABLED=false" "$TEMPLATE_FILE" && YOLO_ENABLED=false
fi

# 3. 根据目录内容检测 (兜底: 没有对应组件则强制禁用)
if [ ! -d "$INSTALL_DIR/rivision_node/engines/llama.cpp" ] || [ -z "$(ls -A $INSTALL_DIR/rivision_node/engines/llama.cpp 2>/dev/null)" ]; then
    LLAMA_ENABLED=false
    echo "  [检测] llama.cpp 目录不存在或为空，禁用 LLAMA"
fi
if [ ! -f "$INSTALL_DIR/rivision_node/bin/yolo-server" ]; then
    YOLO_ENABLED=false
    echo "  [检测] yolo-server 不存在，禁用 YOLO"
fi

echo ""
echo "节点类型检测:"
echo "  LLAMA_ENABLED=$LLAMA_ENABLED"
echo "  YOLO_ENABLED=$YOLO_ENABLED"

# 停止并清理不需要的服务 (先 stop，后 disable，最后删除服务文件)
echo ""
echo "配置服务..."

# 先停止不需要的服务
if ! $LLAMA_ENABLED; then
    echo -e "  ${YELLOW}停止并移除: rivision-llama (node_yolo 模式)${NC}"
    run_as_root systemctl stop rivision-llama 2>/dev/null || true
    run_as_root systemctl disable rivision-llama 2>/dev/null || true
    run_as_root rm -f /etc/systemd/system/rivision-llama.service 2>/dev/null || true
fi

if ! $YOLO_ENABLED; then
    echo -e "  ${YELLOW}停止并移除: rivision-yolo (node_llama 模式)${NC}"
    run_as_root systemctl stop rivision-yolo 2>/dev/null || true
    run_as_root systemctl disable rivision-yolo 2>/dev/null || true
    run_as_root rm -f /etc/systemd/system/rivision-yolo.service 2>/dev/null || true
fi

# 重新加载 systemd 配置 (清理后)
run_as_root systemctl daemon-reload

# 启用需要的服务
run_as_root systemctl enable rivision-node-agent 2>/dev/null && echo -e "  ${GREEN}已启用: rivision-node-agent${NC}" || true

if $LLAMA_ENABLED; then
    run_as_root systemctl enable rivision-llama 2>/dev/null && echo -e "  ${GREEN}已启用: rivision-llama${NC}" || true
fi

if $YOLO_ENABLED; then
    run_as_root systemctl enable rivision-yolo 2>/dev/null && echo -e "  ${GREEN}已启用: rivision-yolo${NC}" || true
fi

# 启动/重启服务
# ★ 重要：先启动推理服务 (llama/yolo)，再启动 node-agent
#    否则 node-agent 会在推理服务启动前报告健康检查失败
echo ""
if $IS_UPGRADE; then
    echo "重启服务..."
else
    echo "启动服务..."
fi

# 1. 先启动 llama (如果启用)
if $LLAMA_ENABLED; then
    echo "  启动 rivision-llama..."
    if run_as_root systemctl restart rivision-llama; then
        echo -e "  ${GREEN}已启动: rivision-llama${NC}"
        # 等待 llama 服务就绪
        sleep 2
    else
        echo -e "  ${RED}启动失败: rivision-llama${NC}"
        echo "  诊断: systemctl status rivision-llama"
        run_as_root systemctl status rivision-llama --no-pager -l 2>&1 | head -20 || true
    fi
fi

# 2. 再启动 yolo (如果启用)
if $YOLO_ENABLED; then
    echo "  启动 rivision-yolo..."
    if run_as_root systemctl restart rivision-yolo; then
        echo -e "  ${GREEN}已启动: rivision-yolo${NC}"
        sleep 1
    else
        echo -e "  ${RED}启动失败: rivision-yolo${NC}"
        echo "  诊断: systemctl status rivision-yolo"
        run_as_root systemctl status rivision-yolo --no-pager -l 2>&1 | head -20 || true
    fi
fi

# 3. 最后启动 node-agent (此时推理服务应该已经就绪)
echo "  启动 rivision-node-agent..."
if run_as_root systemctl restart rivision-node-agent 2>/dev/null; then
    echo -e "  ${GREEN}已启动: rivision-node-agent${NC}"
else
    echo -e "  ${YELLOW}跳过: rivision-node-agent (启动失败)${NC}"
fi

# 权限
echo ""; echo "设置权限..."
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR/rivision_node"
run_as_root find "$INSTALL_DIR/rivision_node" -type d -name "bin" -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR/rivision_node" -path "*/bin/*" -type f -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR/rivision_node" -path "*/bin/*" -type f -exec chmod 755 {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR/rivision_node" -type d -name "engines" -exec chown -R root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR/rivision_node" -path "*/engines/*" -type f -exec chmod 755 {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR/rivision_node" -type d -name "lib" -exec chown -R root:root {} \; 2>/dev/null || true
for dir in logs models config scripts; do
    run_as_root find "$INSTALL_DIR/rivision_node" -type d -name "$dir" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
done

echo -e "${GREEN}完成!${NC}"
