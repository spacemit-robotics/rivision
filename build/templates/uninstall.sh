#!/bin/bash
# ============================================================
# RiVision 统一卸载脚本 (与 install.sh 对应)
#
# 用法:
#   ./uninstall.sh --host             # 卸载主机节点
#   ./uninstall.sh --node             # 卸载算力节点
#   ./uninstall.sh --all              # 全部卸载
#   ./uninstall.sh                    # 自动检测已安装组件并卸载
#
# 选项:
#   --keep-data    保留 .env、models/、mp4path/、logs/
#   --purge        删除所有数据 + 备份 (默认保留)
#   --yes          跳过确认提示
# ============================================================
set -e

INSTALL_DIR="/opt/rivision"

# 颜色
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'

# --- 参数解析 ---
MODE=""
KEEP_DATA=false
PURGE=false
YES=false

for arg in "$@"; do
    case "$arg" in
        --host)       MODE="host" ;;
        --node)       MODE="node" ;;
        --all)        MODE="all" ;;
        --keep-data)  KEEP_DATA=true ;;
        --purge)      PURGE=true ;;
        --yes|-y)     YES=true ;;
        --help|-h)
            echo "用法: $0 [--host|--node|--all] [--keep-data|--purge] [--yes]"
            echo ""
            echo "模式:"
            echo "  --host       卸载主机节点: rivision-cli + rivision_gateway"
            echo "  --node       卸载算力节点: node-agent + yolo-server + llama.cpp"
            echo "  --all        全部卸载"
            echo "  (无参数)     自动检测已安装组件"
            echo ""
            echo "选项:"
            echo "  --keep-data  保留用户数据 (.env, models/, mp4path/, logs/)"
            echo "  --purge      删除所有数据 + /opt/rivision.backup.* 备份"
            echo "  --yes        跳过确认提示"
            exit 0
            ;;
        *)
            echo "未知参数: $arg (使用 --help 查看用法)"
            exit 1
            ;;
    esac
done

# 检查 root
if [ "$EUID" -ne 0 ]; then
    echo "请使用 root 或 sudo 运行"
    exit 1
fi

# 检查安装目录
if [ ! -d "$INSTALL_DIR" ]; then
    echo "未检测到 RiVision 安装 ($INSTALL_DIR 不存在)"
    exit 0
fi

# 自动检测模式
if [ -z "$MODE" ]; then
    HAS_HOST=false
    HAS_NODE=false
    [ -f "$INSTALL_DIR/bin/rivision_gateway" ] || [ -f "$INSTALL_DIR/bin/rivision-cli" ] && HAS_HOST=true
    [ -f "$INSTALL_DIR/bin/rivision_node" ] || [ -f "$INSTALL_DIR/bin/yolo-server" ] && HAS_NODE=true

    if $HAS_HOST && $HAS_NODE; then
        MODE="all"
    elif $HAS_NODE; then
        MODE="node"
    elif $HAS_HOST; then
        MODE="host"
    else
        MODE="all"
    fi
    echo -e "自动检测模式: ${GREEN}$MODE${NC}"
fi

# 版本信息
VERSION=""
if [ -f "$INSTALL_DIR/manifest.json" ]; then
    VERSION=$(grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' "$INSTALL_DIR/manifest.json" | head -1 | sed 's/.*"version"[[:space:]]*:[[:space:]]*"//;s/"//')
fi

echo ""
echo "=== RiVision Uninstall ==="
echo "  模式: $MODE"
echo "  目标: $INSTALL_DIR"
[ -n "$VERSION" ] && echo "  版本: $VERSION"
echo ""

# 确认
if ! $YES; then
    echo -e "${RED}警告: 此操作将卸载 RiVision 并停止相关服务${NC}"
    read -p "确认卸载? [y/N] " confirm
    case "$confirm" in
        [yY][eE][sS]|[yY]) ;;
        *) echo "已取消"; exit 0 ;;
    esac
    echo ""
fi

# ============================================================
# 服务定义 (与 install.sh 对应)
# ============================================================
HOST_SERVICES="rivision-cli rivision-gateway"
NODE_SERVICES="rivision-node-agent rivision-llama rivision-yolo"

# ============================================================
# 停止 + 禁用 + 删除 systemd 服务
# ============================================================
remove_services() {
    local services="$1"
    for svc in $services; do
        if systemctl is-enabled --quiet "$svc" 2>/dev/null; then
            echo "  禁用服务: $svc"
            systemctl disable "$svc" 2>/dev/null || true
        fi
        if systemctl is-active --quiet "$svc" 2>/dev/null; then
            echo "  停止服务: $svc"
            systemctl stop "$svc" 2>/dev/null || true
        fi
        if [ -f "/etc/systemd/system/$svc.service" ]; then
            rm -f "/etc/systemd/system/$svc.service"
            echo "  已删除: /etc/systemd/system/$svc.service"
        fi
    done
}

# ============================================================
# 卸载主机节点
# ============================================================
uninstall_host() {
    echo "[主机节点] 卸载 rivision-cli + rivision_gateway..."

    # 停止 + 删除 systemd 服务
    remove_services "$HOST_SERVICES"

    # 删除二进制
    for bin in rivision-cli rivision_gateway; do
        if [ -f "$INSTALL_DIR/bin/$bin" ]; then
            rm -f "$INSTALL_DIR/bin/$bin"
            echo "  已删除: bin/$bin"
        fi
    done

    # 删除 rivision-cli 运行时提取的二进制
    for bin in go2rtc-riscv64 rtsp_server-riscv64; do
        if [ -f "$INSTALL_DIR/bin/$bin" ]; then
            rm -f "$INSTALL_DIR/bin/$bin"
            echo "  已删除: bin/$bin (运行时提取)"
        fi
    done

    # 删除 mp4path (除非 --keep-data)
    if ! $KEEP_DATA && [ -d "$INSTALL_DIR/mp4path" ]; then
        rm -rf "${INSTALL_DIR:?}/mp4path"
        echo "  已删除: mp4path/"
    fi
}

# ============================================================
# 卸载算力节点
# ============================================================
uninstall_node() {
    echo "[算力节点] 卸载 rivision_node 组件..."

    # 停止 + 删除 systemd 服务
    remove_services "$NODE_SERVICES"

    # 删除 node 二进制
    for bin in rivision_node yolo-server; do
        if [ -f "$INSTALL_DIR/bin/$bin" ]; then
            rm -f "$INSTALL_DIR/bin/$bin"
            echo "  已删除: bin/$bin"
        fi
    done

    # 删除运行时库
    if [ -d "$INSTALL_DIR/lib" ]; then
        rm -rf "${INSTALL_DIR:?}/lib"
        echo "  已删除: lib/"
    fi

    # 删除 llama.cpp 引擎
    if [ -d "$INSTALL_DIR/engines" ]; then
        rm -rf "${INSTALL_DIR:?}/engines"
        echo "  已删除: engines/"
    fi

    # 删除脚本
    if [ -d "$INSTALL_DIR/scripts" ]; then
        rm -rf "${INSTALL_DIR:?}/scripts"
        echo "  已删除: scripts/"
    fi

    # 删除 services 目录
    if [ -d "$INSTALL_DIR/services" ]; then
        rm -rf "${INSTALL_DIR:?}/services"
        echo "  已删除: services/"
    fi

    # 删除模型 (除非 --keep-data)
    if ! $KEEP_DATA && [ -d "$INSTALL_DIR/models" ]; then
        rm -rf "${INSTALL_DIR:?}/models"
        echo "  已删除: models/"
    elif $KEEP_DATA && [ -d "$INSTALL_DIR/models" ]; then
        echo -e "  ${YELLOW}保留: models/${NC}"
    fi

    # 删除配置 (除非 --keep-data)
    if ! $KEEP_DATA && [ -d "$INSTALL_DIR/config" ]; then
        rm -rf "${INSTALL_DIR:?}/config"
        echo "  已删除: config/"
    elif $KEEP_DATA && [ -d "$INSTALL_DIR/config" ]; then
        echo -e "  ${YELLOW}保留: config/${NC}"
    fi

    # 清理 ldconfig
    if [ -f /etc/ld.so.conf.d/rivision.conf ]; then
        rm -f /etc/ld.so.conf.d/rivision.conf
        ldconfig 2>/dev/null || true
        echo "  已清理: ldconfig"
    fi
}

# ============================================================
# 执行卸载
# ============================================================
case "$MODE" in
    host) uninstall_host ;;
    node) uninstall_node ;;
    all)  uninstall_host; uninstall_node ;;
esac

# systemctl daemon-reload
systemctl daemon-reload 2>/dev/null || true

# 删除公共文件
rm -f "$INSTALL_DIR/manifest.json"

# 删除 .env (除非 --keep-data)
if ! $KEEP_DATA && [ -f "$INSTALL_DIR/.env" ]; then
    rm -f "$INSTALL_DIR/.env"
    echo "  已删除: .env"
elif $KEEP_DATA && [ -f "$INSTALL_DIR/.env" ]; then
    echo -e "  ${YELLOW}保留: .env${NC}"
fi

# 删除各组件 logs (除非 --keep-data)
# 新目录结构: 各组件有独立的 logs/ 目录
if ! $KEEP_DATA; then
    for comp_dir in rivision_cli rivision_gateway rivision_embed rivision_owl rivision_node; do
        if [ -d "$INSTALL_DIR/$comp_dir/logs" ]; then
            rm -rf "${INSTALL_DIR:?}/$comp_dir/logs"
            echo "  已删除: $comp_dir/logs/"
        fi
    done
elif $KEEP_DATA; then
    for comp_dir in rivision_cli rivision_gateway rivision_embed rivision_owl rivision_node; do
        if [ -d "$INSTALL_DIR/$comp_dir/logs" ]; then
            echo -e "  ${YELLOW}保留: $comp_dir/logs/${NC}"
        fi
    done
fi

# 清理空的 bin/ 目录
if [ -d "$INSTALL_DIR/bin" ]; then
    rmdir "$INSTALL_DIR/bin" 2>/dev/null || true
fi

# 清理空的安装目录
if [ -d "$INSTALL_DIR" ]; then
    # 检查是否还有文件残留
    REMAINING=$(find "$INSTALL_DIR" -type f 2>/dev/null | wc -l)
    if [ "$REMAINING" -eq 0 ]; then
        rm -rf "${INSTALL_DIR:?}"
        echo "  已删除: $INSTALL_DIR/"
    else
        echo -e "  ${YELLOW}$INSTALL_DIR/ 保留 ($REMAINING 个文件残留)${NC}"
    fi
fi

# --purge: 删除备份
if $PURGE; then
    echo ""
    echo "清理备份..."
    for bak in /opt/rivision.backup.*; do
        if [ -d "$bak" ]; then
            rm -rf "$bak"
            echo "  已删除: $bak"
        fi
    done
fi

echo ""
echo -e "${GREEN}=== 卸载完成 ===${NC}"
if $KEEP_DATA; then
    echo -e "${YELLOW}用户数据已保留在 $INSTALL_DIR/${NC}"
fi
