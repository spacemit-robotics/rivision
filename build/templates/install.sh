#!/bin/bash
# ============================================================
# RiVision 统一安装脚本
#
# 用法:
#   ./install.sh --host             # 主机节点: cli + gateway + embed + owl
#   ./install.sh --node             # 算力节点: node-agent + yolo + llama
#   ./install.sh                    # 自动检测 (有 node/ 目录装 node，有 bin/rivision_gateway 装 host)
#
# 说明: 脚本以普通用户运行，仅在需要时提示输入 sudo 密码
# ============================================================
set -e

INSTALL_DIR="/opt/rivision"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 当前用户 (不使用 root)
RIVISION_USER="$(whoami)"
RIVISION_GROUP="$(id -gn)"

# 颜色
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'

# === 按需 sudo 封装函数 ===
# 需要 root 权限时调用，会提示输入密码
run_as_root() {
    if [ "$EUID" -eq 0 ]; then
        "$@"
    else
        sudo "$@"
    fi
}

# 检查是否能获取 sudo 权限 (首次调用时提示密码)
check_sudo() {
    if [ "$EUID" -ne 0 ]; then
        echo -e "${YELLOW}部分操作需要管理员权限，请输入密码:${NC}"
        sudo -v || { echo -e "${RED}无法获取 sudo 权限${NC}"; exit 1; }
    fi
}

# 获取本机IP地址
get_local_ip() {
    local ip=$(ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K[0-9.]+')
    if [ -z "$ip" ]; then
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi
    echo "${ip:-127.0.0.1}"
}

# --- 参数解析 ---
MODE=""
case "${1:-}" in
    --host)    MODE="host" ;;
    --node)    MODE="node" ;;
    --help|-h|"")
        echo "用法: $0 --host|--node"
        echo "  --host     主机节点: rivision-cli + rivision_gateway + rivision-embed + rivision-owl"
        echo "  --node     算力节点: node-agent + yolo-server + llama.cpp + 模型"
        exit 0
        ;;
    *)
        echo "未知参数: $1 (使用 --help 查看用法)"
        exit 1
        ;;
esac

echo ""
echo "=== RiVision Installation ==="
echo "  模式: $MODE"
echo "  目标: $INSTALL_DIR"

# 预先验证 sudo 权限 (首次提示密码)
check_sudo

# --- 升级检测 ---
IS_UPGRADE=false
PREV_VERSION=""
NEW_VERSION=""

if [ -f "$SCRIPT_DIR/manifest.json" ]; then
    NEW_VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$SCRIPT_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')
fi

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

# 验证校验和
if [ -f "$SCRIPT_DIR/checksums.txt" ]; then
    echo "验证校验和..."
    cd "$SCRIPT_DIR"
    sha256sum -c checksums.txt --quiet 2>/dev/null || echo -e "${YELLOW}WARNING: 部分校验失败${NC}"
fi

# --- 升级: 停止服务 + 备份 ---
HOST_SERVICES="rivision-cli rivision-gateway rivision-embed rivision-owl"
NODE_SERVICES="rivision-node-agent rivision-llama rivision-yolo"

stop_services() {
    local services="$1"
    for svc in $services; do
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            echo "  停止服务: $svc"
            run_as_root systemctl stop "$svc" 2>/dev/null || true
        fi
    done
}

if $IS_UPGRADE; then
    echo "停止现有服务..."
    case "$MODE" in
        host) stop_services "$HOST_SERVICES" ;;
        node) stop_services "$NODE_SERVICES" ;;
    esac
fi

# ============================================================
# 主机节点安装: rivision_cli + rivision_gateway + rivision_embed + rivision_owl
# 每个组件独立目录: /opt/rivision/rivision_xxx/
# ============================================================
install_host() {
    echo "[主机节点] 安装 rivision_cli + rivision_gateway + rivision_embed + rivision_owl..."
    
    # 创建共享目录 (需要 root 创建 /opt/rivision)
    run_as_root mkdir -p "$INSTALL_DIR/shared"
    run_as_root chown "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR" "$INSTALL_DIR/shared"
    
    # === 安装 rivision_cli ===
    if [ -f "$SCRIPT_DIR/bin/rivision-cli" ] || [ -d "$SCRIPT_DIR/rivision_cli" ]; then
        echo "  安装 rivision_cli..."
        mkdir -p "$INSTALL_DIR/rivision_cli"/{bin,config,logs,mp4path}
        if [ -f "$SCRIPT_DIR/bin/rivision-cli" ]; then
            cp "$SCRIPT_DIR/bin/rivision-cli" "$INSTALL_DIR/rivision_cli/bin/"
        elif [ -f "$SCRIPT_DIR/rivision_cli/bin/rivision-cli" ]; then
            cp -r "$SCRIPT_DIR/rivision_cli/"* "$INSTALL_DIR/rivision_cli/"
        fi
        chmod +x "$INSTALL_DIR/rivision_cli/bin/"* 2>/dev/null || true
        echo "    已安装: rivision_cli/"
    fi
    
    # === 安装 rivision_gateway ===
    if [ -f "$SCRIPT_DIR/bin/rivision_gateway" ] || [ -d "$SCRIPT_DIR/rivision_gateway" ]; then
        echo "  安装 rivision_gateway..."
        mkdir -p "$INSTALL_DIR/rivision_gateway"/{bin,config,logs}
        if [ -f "$SCRIPT_DIR/bin/rivision_gateway" ]; then
            cp "$SCRIPT_DIR/bin/rivision_gateway" "$INSTALL_DIR/rivision_gateway/bin/"
        elif [ -f "$SCRIPT_DIR/rivision_gateway/bin/rivision_gateway" ]; then
            cp -r "$SCRIPT_DIR/rivision_gateway/"* "$INSTALL_DIR/rivision_gateway/"
        fi
        chmod +x "$INSTALL_DIR/rivision_gateway/bin/"* 2>/dev/null || true
        echo "    已安装: rivision_gateway/"
    fi
    
    # === 安装 rivision_embed ===
    if [ -f "$SCRIPT_DIR/bin/rivision-embed-server" ] || [ -d "$SCRIPT_DIR/rivision_embed" ]; then
        echo "  安装 rivision_embed..."
        mkdir -p "$INSTALL_DIR/rivision_embed"/{bin,config,logs,models}
        if [ -f "$SCRIPT_DIR/bin/rivision-embed-server" ]; then
            cp "$SCRIPT_DIR/bin/rivision-embed-server" "$INSTALL_DIR/rivision_embed/bin/"
        elif [ -f "$SCRIPT_DIR/rivision_embed/bin/rivision-embed-server" ]; then
            cp -r "$SCRIPT_DIR/rivision_embed/"* "$INSTALL_DIR/rivision_embed/"
        fi
        # 复制模型
        if [ -d "$SCRIPT_DIR/models/embed" ]; then
            cp "$SCRIPT_DIR"/models/embed/*.onnx "$INSTALL_DIR/rivision_embed/models/" 2>/dev/null || true
        fi
        # 复制运行时库
        if [ -d "$SCRIPT_DIR/lib" ]; then
            mkdir -p "$INSTALL_DIR/rivision_embed/lib"
            cp "$SCRIPT_DIR"/lib/*.so* "$INSTALL_DIR/rivision_embed/lib/" 2>/dev/null || true
            run_as_root sh -c "echo '$INSTALL_DIR/rivision_embed/lib' > /etc/ld.so.conf.d/rivision-embed.conf"
            run_as_root ldconfig 2>/dev/null || true
        fi
        chmod +x "$INSTALL_DIR/rivision_embed/bin/"* 2>/dev/null || true
        local model_count=$(ls "$INSTALL_DIR/rivision_embed/models/"*.onnx 2>/dev/null | wc -l)
        echo "    已安装: rivision_embed/ ($model_count 个模型)"
    fi
    
    # === 安装 rivision_owl ===
    if [ -d "$SCRIPT_DIR/owl" ] || [ -d "$SCRIPT_DIR/rivision_owl" ]; then
        echo "  安装 rivision_owl..."
        mkdir -p "$INSTALL_DIR/rivision_owl"/{bin,config,logs,data,recordings,mp4path}
        if [ -d "$SCRIPT_DIR/owl" ]; then
            cp -r "$SCRIPT_DIR"/owl/* "$INSTALL_DIR/rivision_owl/" 2>/dev/null || true
        elif [ -d "$SCRIPT_DIR/rivision_owl" ]; then
            cp -r "$SCRIPT_DIR"/rivision_owl/* "$INSTALL_DIR/rivision_owl/" 2>/dev/null || true
        fi
        chmod +x "$INSTALL_DIR/rivision_owl/bin/"* 2>/dev/null || true
        # ★ 检查并打印 mp4path 复制日志
        if [ -d "$INSTALL_DIR/rivision_owl/mp4path" ] && ls "$INSTALL_DIR/rivision_owl/mp4path/"*.mp4 >/dev/null 2>&1; then
            echo "    已复制测试视频到 mp4path/"
        fi
        echo "    已安装: rivision_owl/"
    fi

    # 安装 systemd services (需要 root)
    # 替换占位符为实际安装用户
    for svc in rivision-gateway.service rivision-cli.service rivision-embed.service rivision-owl.service; do
        SVC_FILE=""
        if [ -f "$SCRIPT_DIR/services/$svc" ]; then
            SVC_FILE="$SCRIPT_DIR/services/$svc"
        elif [ -f "$SCRIPT_DIR/$svc" ]; then
            SVC_FILE="$SCRIPT_DIR/$svc"
        fi
        if [ -n "$SVC_FILE" ]; then
            # 复制并替换用户占位符
            sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" \
                -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
                "$SVC_FILE" | run_as_root tee /etc/systemd/system/"$svc" > /dev/null
            echo "  已安装: $svc"
        fi
    done
    run_as_root systemctl daemon-reload
    echo "  Host 服务用户: $RIVISION_USER"
}

# ============================================================
# 算力节点安装: node-agent + yolo + llama + 模型
# 独立目录: /opt/rivision/rivision_node/
# ============================================================
install_node() {
    echo "[算力节点] 安装 rivision_node 组件..."
    
    # 创建共享目录 (需要 root 创建 /opt/rivision)
    run_as_root mkdir -p "$INSTALL_DIR/shared"
    run_as_root chown "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR" "$INSTALL_DIR/shared"
    
    # 创建 node 独立目录结构
    mkdir -p "$INSTALL_DIR/rivision_node"/{bin,lib,engines/llama.cpp,models,config,scripts,logs}

    # node 二进制
    if [ -d "$SCRIPT_DIR/node/bin" ]; then
        cp "$SCRIPT_DIR"/node/bin/* "$INSTALL_DIR/rivision_node/bin/" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/rivision_node/bin/"* 2>/dev/null || true
        echo "  已安装: rivision_node/bin/ ($(ls "$SCRIPT_DIR/node/bin/" | tr '\n' ' '))"
    fi

    # 运行时库
    if [ -d "$SCRIPT_DIR/node/lib" ]; then
        cp "$SCRIPT_DIR"/node/lib/* "$INSTALL_DIR/rivision_node/lib/" 2>/dev/null || true
        local lib_count=$(ls "$INSTALL_DIR/rivision_node/lib/"*.so* 2>/dev/null | wc -l)
        echo "  已安装: rivision_node/lib/ ($lib_count 个 .so)"
        # 配置 ldconfig (需要 root)
        run_as_root sh -c "echo '$INSTALL_DIR/rivision_node/lib' > /etc/ld.so.conf.d/rivision-node.conf"
        run_as_root ldconfig 2>/dev/null || true
    fi

    # llama.cpp 引擎
    if [ -d "$SCRIPT_DIR/node/engines/llama.cpp" ]; then
        cp "$SCRIPT_DIR"/node/engines/llama.cpp/* "$INSTALL_DIR/rivision_node/engines/llama.cpp/" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/rivision_node/engines/llama.cpp/"* 2>/dev/null || true
        echo "  已安装: rivision_node/engines/llama.cpp/"
    fi

    # 模型
    if [ -d "$SCRIPT_DIR/node/models" ]; then
        cp -r "$SCRIPT_DIR"/node/models/* "$INSTALL_DIR/rivision_node/models/" 2>/dev/null || true
        echo "  已安装: rivision_node/models/ ($(ls "$INSTALL_DIR/rivision_node/models/" | tr '\n' ' '))"
    fi

    # 配置
    if [ -d "$SCRIPT_DIR/node/config" ]; then
        cp -r "$SCRIPT_DIR"/node/config/* "$INSTALL_DIR/rivision_node/config/" 2>/dev/null || true
        echo "  已安装: rivision_node/config/"
    fi

    # 脚本
    if [ -d "$SCRIPT_DIR/node/scripts" ]; then
        cp "$SCRIPT_DIR"/node/scripts/*.sh "$INSTALL_DIR/rivision_node/scripts/" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/rivision_node/scripts/"*.sh 2>/dev/null || true
        echo "  已安装: rivision_node/scripts/"
    fi

    # systemd 服务 (需要 root)
    # 替换占位符为实际安装用户
    if [ -d "$SCRIPT_DIR/node/services" ]; then
        for svc in "$SCRIPT_DIR"/node/services/*.service; do
            if [ -f "$svc" ]; then
                SVC_NAME=$(basename "$svc")
                # 复制并替换用户占位符
                sed -e "s/RIVISION_USER_PLACEHOLDER/$RIVISION_USER/g" \
                    -e "s/RIVISION_GROUP_PLACEHOLDER/$RIVISION_GROUP/g" \
                    "$svc" | run_as_root tee /etc/systemd/system/"$SVC_NAME" > /dev/null
            fi
        done
        run_as_root systemctl daemon-reload
        echo "  已安装: systemd services (User=$RIVISION_USER)"
    fi

}

# ============================================================
# 执行安装
# ============================================================
case "$MODE" in
    host)
        install_host
        ;;
    node)
        install_node
        ;;
esac

# manifest
cp "$SCRIPT_DIR/manifest.json" "$INSTALL_DIR/" 2>/dev/null || true

echo ""
echo -e "${GREEN}=== 安装完成: $INSTALL_DIR ===${NC}"
if [ -n "$NEW_VERSION" ]; then
    echo "  版本: $NEW_VERSION"
fi
echo ""
echo "已安装组件:"
ls -d "$INSTALL_DIR"/rivision_* 2>/dev/null | while read d; do echo "  $(basename $d)/"; done
echo ""

# --- 初始化 .env 配置 (全新安装时从模板生成) ---
LOCAL_IP=$(get_local_ip)

# Host 模式: shared/.env (CLI, Gateway, Embed, OWL 共用)
if [ "$MODE" = "host" ]; then
    if [ ! -f "$INSTALL_DIR/shared/.env" ]; then
        if [ -f "$SCRIPT_DIR/config/.env.template" ]; then
            cp "$SCRIPT_DIR/config/.env.template" "$INSTALL_DIR/shared/.env"
            # 自动设置 OWL_SDP_IP
            if [ -n "$LOCAL_IP" ] && [ "$LOCAL_IP" != "127.0.0.1" ]; then
                sed -i "s/^OWL_SDP_IP=.*/OWL_SDP_IP=$LOCAL_IP/" "$INSTALL_DIR/shared/.env"
                echo -e "  已自动设置: OWL_SDP_IP=$LOCAL_IP"
            fi
            echo -e "${YELLOW}已从模板创建 shared/.env (Host配置)，请检查:${NC}"
            echo "  vi $INSTALL_DIR/shared/.env"
            echo ""
        fi
    fi
fi

# Node 模式: rivision_node/config/.env (Node Agent, LLaMA, YOLO 配置)
if [ "$MODE" = "node" ]; then
    if [ ! -f "$INSTALL_DIR/rivision_node/config/.env" ]; then
        if [ -f "$SCRIPT_DIR/config/.env.node.template" ]; then
            cp "$SCRIPT_DIR/config/.env.node.template" "$INSTALL_DIR/rivision_node/config/.env"
            # 自动设置 NODE_ID
            NODE_HOSTNAME=$(hostname)
            sed -i "s/^NODE_ID=.*/NODE_ID=node-${NODE_HOSTNAME}/" "$INSTALL_DIR/rivision_node/config/.env"
            echo -e "  已自动设置: NODE_ID=node-${NODE_HOSTNAME}"
            echo -e "${YELLOW}已从模板创建 rivision_node/config/.env (Node配置)，请检查:${NC}"
            echo -e "${RED}  ⚠️  必须修改 GATEWAY_URL 为实际的 Gateway 地址${NC}"
            echo "  vi $INSTALL_DIR/rivision_node/config/.env"
            echo ""
        fi
    fi
fi

# 启用开机自启 (需要 root)
echo "启用开机自启..."
case "$MODE" in
    host)
        for svc in rivision-embed rivision-gateway rivision-cli; do
            run_as_root systemctl enable "$svc" 2>/dev/null && echo -e "  ${GREEN}已启用: $svc${NC}" || true
        done
        # OWL 可选启用
        run_as_root systemctl enable rivision-owl 2>/dev/null && echo -e "  ${GREEN}已启用: rivision-owl${NC}" || true
        ;;
    node)
        for svc in rivision-node-agent rivision-llama rivision-yolo; do
            run_as_root systemctl enable "$svc" 2>/dev/null && echo -e "  ${GREEN}已启用: $svc${NC}" || true
        done
        ;;
esac
echo ""

# --- 升级时重启服务 ---
if $IS_UPGRADE; then
    echo "重启服务..."
    case "$MODE" in
        host)
            for svc in rivision-embed rivision-gateway rivision-cli; do
                run_as_root systemctl restart "$svc" 2>/dev/null && echo -e "  ${GREEN}已重启: $svc${NC}" || true
            done
            # OWL 可选重启
            run_as_root systemctl restart rivision-owl 2>/dev/null && echo -e "  ${GREEN}已重启: rivision-owl${NC}" || true
            ;;
        node)
            for svc in rivision-node-agent rivision-llama rivision-yolo; do
                run_as_root systemctl restart "$svc" 2>/dev/null && echo -e "  ${GREEN}已重启: $svc${NC}" || true
            done
            ;;
    esac
else
    # 全新安装：显示启动提示
    echo "启动服务:"
    case "$MODE" in
        host)
            echo "  systemctl start rivision-owl      # GB28181 服务 (可选)"
            echo "  systemctl start rivision-embed"
            echo "  systemctl start rivision-gateway"
            echo "  systemctl start rivision-cli"
            ;;
        node)
            echo "  systemctl start rivision-node-agent"
            echo "  systemctl start rivision-llama"
            echo "  systemctl start rivision-yolo"
            ;;
    esac
fi

# --- 设置目录权限 (需要 root) ---
echo "设置目录权限 (用户: $RIVISION_USER)..."

# 整体目录归属用户
run_as_root chown -R "$RIVISION_USER:$RIVISION_GROUP" "$INSTALL_DIR"

# bin 目录和二进制文件保持 root 所有 (安全)，但允许执行
run_as_root find "$INSTALL_DIR" -type d -name "bin" -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chown root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/bin/*" -type f -exec chmod 755 {} \; 2>/dev/null || true

# engines 目录 (llama.cpp) 同样保持 root
run_as_root find "$INSTALL_DIR" -type d -name "engines" -exec chown -R root:root {} \; 2>/dev/null || true
run_as_root find "$INSTALL_DIR" -path "*/engines/*" -type f -exec chmod 755 {} \; 2>/dev/null || true

# lib 目录保持 root (共享库)
run_as_root find "$INSTALL_DIR" -type d -name "lib" -exec chown -R root:root {} \; 2>/dev/null || true

# 数据目录确保用户可写
for dir in logs data recordings mp4path models config shared; do
    run_as_root find "$INSTALL_DIR" -type d -name "$dir" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
done

# scripts 目录用户可执行
run_as_root find "$INSTALL_DIR" -type d -name "scripts" -exec chown -R "$RIVISION_USER:$RIVISION_GROUP" {} \; 2>/dev/null || true
find "$INSTALL_DIR" -path "*/scripts/*.sh" -exec chmod 755 {} \; 2>/dev/null || true

echo -e "  ${GREEN}权限设置完成${NC}"
