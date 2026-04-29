#!/bin/bash
# ============================================================
# RiVision 节点卸载脚本
# 支持 K3 (RISC-V) 和 x86 架构
#
# 用法:
#   ./uninstall.sh              # 交互式卸载
#   ./uninstall.sh --keep-models # 保留模型文件
#   ./uninstall.sh --keep-config # 保留配置文件
#   ./uninstall.sh --force       # 跳过确认
#   ./uninstall.sh --help        # 显示帮助
# ============================================================
set -e

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[ OK ]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }

KEEP_MODELS=false
KEEP_CONFIG=false
FORCE=false

for arg in "$@"; do
    case "$arg" in
        --keep-models) KEEP_MODELS=true ;;
        --keep-config) KEEP_CONFIG=true ;;
        --force)       FORCE=true ;;
        --help|-h)
            echo "用法: ./uninstall.sh [OPTIONS]"
            echo ""
            echo "选项:"
            echo "  --keep-models  保留模型文件 (GGUF/ONNX)"
            echo "  --keep-config  保留配置文件 (.env)"
            echo "  --force        跳过确认提示"
            echo "  --help, -h     显示此帮助"
            exit 0
            ;;
    esac
done

INSTALL_DIR="${RIVISION_HOME:-$HOME/rivision}"
ARCH=$(uname -m)

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║   RiVision 节点卸载                          ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
log_info "架构: $ARCH"
log_info "卸载目录: $INSTALL_DIR"
log_info "保留模型: $KEEP_MODELS"
log_info "保留配置: $KEEP_CONFIG"

# 显示当前安装的组件
echo ""
log_info "已安装组件:"
[ -d "$INSTALL_DIR/node-agent" ] && echo "  • Node Agent (Python)"
[ -x "$INSTALL_DIR/node-agent-go/node-agent-go" ] && echo "  • Node Agent (Go)"
[ -x "$INSTALL_DIR/node-agent-cpp/build/node-agent-cpp" ] && echo "  • Node Agent (C++)"
[ -d "$INSTALL_DIR/llama.cpp" ] && echo "  • llama.cpp"
[ -d "$INSTALL_DIR/yolo-server" ] && echo "  • YOLO Server"
echo ""

if ! $FORCE; then
    read -p "确认卸载? [y/N] " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_warn "已取消"
        exit 0
    fi
fi

# 1. 停止服务
log_info "停止服务..."
for svc in rivision-llama rivision-node-agent rivision-yolo; do
    sudo systemctl stop "$svc" 2>/dev/null || true
    sudo systemctl disable "$svc" 2>/dev/null || true
done
log_success "服务已停止"

# 2. 移除 systemd 服务文件
log_info "移除 systemd 服务..."
sudo rm -f /etc/systemd/system/rivision-llama.service
sudo rm -f /etc/systemd/system/rivision-node-agent.service
sudo rm -f /etc/systemd/system/rivision-yolo.service
sudo systemctl daemon-reload
log_success "systemd 服务已移除"

# 3. 备份需要保留的内容
BACKUP_DIR="/tmp/rivision-backup-$(date +%Y%m%d%H%M%S)"
NEED_RESTORE=false

if [ -d "$INSTALL_DIR" ]; then
    # 备份模型
    if $KEEP_MODELS && [ -d "$INSTALL_DIR/models" ]; then
        log_info "备份模型文件..."
        mkdir -p "$BACKUP_DIR/models"
        cp "$INSTALL_DIR/models/"*.gguf "$BACKUP_DIR/models/" 2>/dev/null || true
        cp "$INSTALL_DIR/models/"*.onnx "$BACKUP_DIR/models/" 2>/dev/null || true
        NEED_RESTORE=true
        log_success "模型已备份"
    fi

    # 备份配置
    if $KEEP_CONFIG && [ -f "$INSTALL_DIR/.env" ]; then
        log_info "备份配置文件..."
        mkdir -p "$BACKUP_DIR"
        cp "$INSTALL_DIR/.env" "$BACKUP_DIR/" 2>/dev/null || true
        NEED_RESTORE=true
        log_success "配置已备份"
    fi

    # 4. 删除安装目录
    log_info "删除安装目录: $INSTALL_DIR"
    rm -rf "$INSTALL_DIR"
    log_success "安装目录已删除"

    # 5. 恢复备份内容
    if $NEED_RESTORE; then
        mkdir -p "$INSTALL_DIR"
        if [ -d "$BACKUP_DIR/models" ]; then
            mv "$BACKUP_DIR/models" "$INSTALL_DIR/"
            log_success "模型文件已恢复"
        fi
        if [ -f "$BACKUP_DIR/.env" ]; then
            mv "$BACKUP_DIR/.env" "$INSTALL_DIR/"
            log_success "配置文件已恢复"
        fi
        rm -rf "$BACKUP_DIR"
    fi
fi

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║   卸载完成                                   ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
if $KEEP_MODELS || $KEEP_CONFIG; then
    log_info "保留的文件位于: $INSTALL_DIR"
fi
log_success "RiVision 节点已卸载"
echo ""
