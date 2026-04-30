#!/bin/bash
# ============================================================
# RiVision CLI 一键安装脚本
#
# 功能:
#   1. 检测平台架构 (x86_64/riscv64/arm64)
#   2. 编译或复制对应架构的二进制文件
#   3. 安装系统依赖 (ffmpeg)
#   4. 部署配置文件和模型
#   5. 配置 systemd 服务
#   6. 验证部署
#
# 用法:
#   ./install.sh [OPTIONS]
#   ./install.sh --build          # 强制重新编译
#   ./install.sh --skip-build     # 跳过编译，只部署
#   ./install.sh --force          # 跳过确认提示
# ============================================================
set -e

# ============ 颜色 ============
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[ OK ]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[FAIL]${NC} $1"; }
log_step()    { echo -e "\n${CYAN}════════════════════════════════════════${NC}"; echo -e "${CYAN}  $1${NC}"; echo -e "${CYAN}════════════════════════════════════════${NC}"; }

# ============ 参数解析 ============
FORCE_BUILD=false
SKIP_BUILD=false
FORCE=false
ENABLE_YOLO=true
GATEWAY_URL="http://localhost:8081"
UPGRADE_MODE=false

for arg in "$@"; do
    case "$arg" in
        --build)        FORCE_BUILD=true ;;
        --skip-build)   SKIP_BUILD=true ;;
        --force)        FORCE=true ;;
        --enable-yolo)  ENABLE_YOLO=true ;;
        --disable-yolo) ENABLE_YOLO=false ;;
        --gateway=*)    GATEWAY_URL="${arg#--gateway=}" ;;
        --upgrade)      UPGRADE_MODE=true ;;
    esac
done

# ============ 路径 ============
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${RIVISION_CLI_HOME:-/opt/rivision-cli}"
CURRENT_USER=$(whoami)
SERVER_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

# ============ 辅助函数 ============
file_hash() { md5sum "$1" 2>/dev/null | cut -d' ' -f1; }
file_mtime() { stat -c '%Y' "$1" 2>/dev/null || echo 0; }

# 文件比对：返回 same/updated/new/missing
compare_file() {
    local src="$1" dst="$2"
    if [ ! -f "$dst" ]; then echo "new"; return; fi
    if [ ! -f "$src" ]; then echo "missing"; return; fi
    local h1=$(file_hash "$src") h2=$(file_hash "$dst")
    if [ "$h1" = "$h2" ]; then echo "same"; else echo "updated"; fi
}

# 服务管理
stop_service() {
    if systemctl is-active --quiet rivision-cli 2>/dev/null; then
        log_info "停止 rivision-cli 服务..."
        sudo systemctl stop rivision-cli || true
        sleep 2
    fi
}

start_service() {
    log_info "启动 rivision-cli 服务..."
    sudo systemctl start rivision-cli || {
        log_warn "服务启动失败，尝试手动启动..."
        sudo systemctl daemon-reload
        sudo systemctl start rivision-cli || true
    }
}

# 检测升级模式
if [ -d "$INSTALL_DIR" ] && [ "$SCRIPT_DIR" != "$INSTALL_DIR" ]; then
    if [ -f "$INSTALL_DIR/rivision-cli" ] || [ -f "$INSTALL_DIR/go2rtc.yaml" ]; then
        if [ "$UPGRADE_MODE" = "false" ]; then
            UPGRADE_MODE=true
            log_info "检测到已有部署，启用升级模式"
        fi
    fi
fi

# ============ 架构检测 ============
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   echo "amd64" ;;
        aarch64|arm64)  echo "arm64" ;;
        riscv64)        echo "riscv64" ;;
        *)              echo "unknown" ;;
    esac
}

SYS_ARCH=$(detect_arch)
IS_RISCV=false
[ "$SYS_ARCH" = "riscv64" ] && IS_RISCV=true

# 二进制文件名映射
get_binary_name() {
    case "$SYS_ARCH" in
        amd64)   echo "rivision-cli-linux-amd64" ;;
        arm64)   echo "rivision-cli-linux-arm64" ;;
        riscv64) echo "rivision-cli-linux-riscv64" ;;
        *)       echo "rivision-cli" ;;
    esac
}

BINARY_NAME=$(get_binary_name)

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║   RiVision CLI 部署工具                       ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
log_info "用户: $CURRENT_USER"
log_info "架构: $SYS_ARCH"
log_info "部署目录: $INSTALL_DIR"
log_info "服务器 IP: $SERVER_IP"
echo ""

if ! $FORCE; then
    read -p "继续安装? [Y/n] " -n 1 -r
    echo ""
    [[ $REPLY =~ ^[Nn]$ ]] && exit 0
fi

# ============================================================
# Step 1: 系统依赖
# ============================================================
log_step "Step 1/5: 安装系统依赖"

if command -v apt &>/dev/null; then
    sudo apt update -qq
    
    # ffmpeg (视频帧提取)
    if ! command -v ffmpeg &>/dev/null; then
        log_info "安装 ffmpeg..."
        sudo apt install -y -qq ffmpeg
    fi
    
    # Go (仅编译时需要)
    if ! $SKIP_BUILD && ! command -v go &>/dev/null; then
        log_info "安装 Go..."
        if $IS_RISCV; then
            # RISC-V: 从系统仓库安装
            sudo apt install -y -qq golang
        else
            # 其他平台: 安装最新版 Go
            sudo apt install -y -qq golang-go || sudo apt install -y -qq golang
        fi
    fi
    
    log_success "系统依赖安装完成"
else
    log_warn "非 apt 系统，请手动安装: ffmpeg, golang"
fi

log_info "ffmpeg: $(ffmpeg -version 2>/dev/null | head -1 || echo 'missing')"
log_info "Go: $(go version 2>/dev/null || echo 'not installed (skip if --skip-build)')"

# ============================================================
# Step 2: 编译或复制二进制
# ============================================================
log_step "Step 2/5: 准备二进制文件"

# 检测运行环境：dist 目录 vs 源码目录
# dist 目录特征: bin/rivision-cli 存在
# 源码目录特征: go.mod 存在
IS_DIST_DIR=false
if [ -f "$SCRIPT_DIR/bin/rivision-cli" ] && [ ! -f "$SCRIPT_DIR/go.mod" ]; then
    IS_DIST_DIR=true
    log_info "检测到 dist 目录模式"
fi

if $IS_DIST_DIR; then
    # dist 目录模式：直接使用 bin/rivision-cli
    BINARY_PATH="$SCRIPT_DIR/bin/rivision-cli"
    BUILD_DIR="$SCRIPT_DIR/bin"
else
    # 源码目录模式：从 build 目录查找
    BUILD_DIR="$SCRIPT_DIR/build"
    BINARY_PATH="$BUILD_DIR/$BINARY_NAME"
fi
INSTALLED_BINARY="$INSTALL_DIR/rivision-cli"

# 检查是否需要编译
need_build() {
    # dist 目录模式不需要编译
    if $IS_DIST_DIR; then
        return 1
    fi
    if $FORCE_BUILD; then
        return 0  # 强制编译
    fi
    if $SKIP_BUILD; then
        return 1  # 跳过编译
    fi
    if [ ! -f "$BINARY_PATH" ]; then
        return 0  # 二进制不存在，需要编译
    fi
    # 升级模式：检查是否更新
    if $UPGRADE_MODE && [ -f "$INSTALLED_BINARY" ]; then
        local status=$(compare_file "$BINARY_PATH" "$INSTALLED_BINARY")
        if [ "$status" = "updated" ]; then
            log_info "检测到二进制更新"
            return 0
        else
            return 1
        fi
    fi
    return 1  # 二进制已存在，无需编译
}

# 升级模式下的服务管理
if $UPGRADE_MODE && need_build; then
    log_info "升级模式：需要重新编译，将停止服务"
    stop_service
fi

if need_build; then
    if ! command -v go &>/dev/null; then
        log_error "Go 未安装，无法编译"
        log_info "请安装 Go 或使用 --skip-build 选项"
        exit 1
    fi
    
    log_info "编译 $BINARY_NAME..."
    mkdir -p "$BUILD_DIR"
    
    cd "$SCRIPT_DIR"
    
    # 配置 Go 国内代理（加速依赖下载）
    log_info "配置 Go 代理 (国内加速)..."
    export GOPROXY="https://goproxy.cn,https://goproxy.io,direct"
    export GOSUMDB="sum.golang.google.cn"
    export GO111MODULE=on
    
    # 下载依赖
    log_info "下载 Go 依赖..."
    go mod download || {
        log_warn "依赖下载可能不完整，继续编译..."
    }
    
    # 根据架构设置环境变量并编译
    log_info "架构: $SYS_ARCH"
    case "$SYS_ARCH" in
        amd64)
            GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "$BINARY_PATH" .
            ;;
        arm64)
            GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o "$BINARY_PATH" .
            ;;
        riscv64)
            GOOS=linux GOARCH=riscv64 go build -ldflags "-s -w" -o "$BINARY_PATH" .
            ;;
        *)
            go build -ldflags "-s -w" -o "$BINARY_PATH" .
            ;;
    esac
    
    log_success "编译完成: $BINARY_PATH"
else
    if [ -f "$BINARY_PATH" ]; then
        log_info "使用已存在的二进制: $BINARY_PATH"
    else
        log_error "二进制文件不存在: $BINARY_PATH"
        log_info "请使用 --build 选项编译，或从其他机器复制"
        exit 1
    fi
fi

# 验证二进制架构
if command -v file &>/dev/null; then
    BINARY_ARCH=$(file "$BINARY_PATH" | grep -oE '(x86-64|aarch64|RISC-V)' || echo "unknown")
    log_info "二进制架构: $BINARY_ARCH"
fi

# ============================================================
# Step 3: 部署文件
# ============================================================
log_step "Step 3/5: 部署文件"

sudo mkdir -p "$INSTALL_DIR"/{bin,data,logs,models,models/class_names,mp4path}
sudo chown -R "$CURRENT_USER:$CURRENT_USER" "$INSTALL_DIR"
log_info "目录结构: bin/ data/ logs/ models/ mp4path/"

# ── 变更跟踪标志 ────────────────────────────────
CONFIG_CHANGED=false
SERVICE_CHANGED=false

# ── 升级模式：详细文件对比报告 ───────────────────
if $UPGRADE_MODE; then
    log_info "升级模式：文件变更对比"
    echo ""

    # 1) 二进制文件
    echo "  ┌─ 二进制文件 ─────────────────────────────"
    if [ -f "$INSTALLED_BINARY" ]; then
        status=$(compare_file "$BINARY_PATH" "$INSTALLED_BINARY")
        case "$status" in
            updated) echo -e "  │  ${YELLOW}↑ rivision-cli${NC} (有更新)" ;;
            same)    echo -e "  │  · rivision-cli (一致)" ;;
        esac
    else
        echo -e "  │  ${GREEN}+ rivision-cli${NC} (新增)"
    fi
    echo "  └────────────────────────────────────────────"

    # 2) 配置文件
    echo "  ┌─ 配置文件 ───────────────────────────────"
    for conf in rivision.yaml go2rtc.yaml; do
        src=""
        if [ "$conf" = "go2rtc.yaml" ] && [ -f "$SCRIPT_DIR/go2rtc.yaml.example" ]; then
            src="$SCRIPT_DIR/go2rtc.yaml.example"
        elif [ -f "$SCRIPT_DIR/$conf" ]; then
            src="$SCRIPT_DIR/$conf"
        fi
        if [ -n "$src" ] && [ -f "$INSTALL_DIR/$conf" ]; then
            status=$(compare_file "$src" "$INSTALL_DIR/$conf")
            case "$status" in
                updated) echo -e "  │  ${YELLOW}! $conf${NC} (不同 → 新版存为 .new)"; CONFIG_CHANGED=true ;;
                same)    echo -e "  │  · $conf (一致)" ;;
            esac
        elif [ -n "$src" ] && [ ! -f "$INSTALL_DIR/$conf" ]; then
            echo -e "  │  ${GREEN}+ $conf${NC} (新增)"; CONFIG_CHANGED=true
        fi
    done
    echo "  └────────────────────────────────────────────"

    # 3) 模型文件
    echo "  ┌─ 模型文件 ───────────────────────────────"
    model_changes=0
    for model_file in "$SCRIPT_DIR/models/"*.onnx; do
        [ -f "$model_file" ] || continue
        model_name=$(basename "$model_file")
        if [ ! -f "$INSTALL_DIR/models/$model_name" ]; then
            echo -e "  │  ${GREEN}+ $model_name${NC} (新增)"
            model_changes=$((model_changes + 1))
        else
            status=$(compare_file "$model_file" "$INSTALL_DIR/models/$model_name")
            case "$status" in
                updated) echo -e "  │  ${YELLOW}↑ $model_name${NC} (有更新)"; model_changes=$((model_changes + 1)) ;;
                same)    echo -e "  │  · $model_name (一致)" ;;
            esac
        fi
    done
    [ $model_changes -eq 0 ] && echo -e "  │  ${GREEN}✓ 所有模型一致${NC}"
    echo "  └────────────────────────────────────────────"
    echo ""
fi

# 复制二进制（带比对）
BINARY_CHANGED=false
if [ -f "$INSTALLED_BINARY" ]; then
    status=$(compare_file "$BINARY_PATH" "$INSTALLED_BINARY")
    case "$status" in
        updated)
            log_info "二进制已更新，正在替换..."
            cp "$BINARY_PATH" "$INSTALLED_BINARY"
            chmod +x "$INSTALLED_BINARY"
            BINARY_CHANGED=true
            log_success "二进制已更新: $INSTALLED_BINARY"
            ;;
        same)
            log_info "二进制未变更，跳过"
            ;;
        new)
            log_info "首次部署二进制..."
            cp "$BINARY_PATH" "$INSTALLED_BINARY"
            chmod +x "$INSTALLED_BINARY"
            BINARY_CHANGED=true
            log_success "二进制已部署: $INSTALLED_BINARY"
            ;;
    esac
else
    log_info "复制二进制文件..."
    cp "$BINARY_PATH" "$INSTALLED_BINARY"
    chmod +x "$INSTALLED_BINARY"
    BINARY_CHANGED=true
    log_success "二进制已部署: $INSTALLED_BINARY"
fi

# 创建 gw-shell 符号链接（用于和 inference-gateway 交互）
log_info "创建 gw-shell 命令..."
sudo mkdir -p /usr/local/bin
sudo ln -sf "$INSTALL_DIR/rivision-cli" /usr/local/bin/gw-shell
sudo ln -sf "$INSTALL_DIR/rivision-cli" /usr/local/bin/rivision-cli
log_success "命令已创建: gw-shell, rivision-cli"

# 配置文件路径（dist 目录在 config/ 下，源码目录在根目录）
if $IS_DIST_DIR; then
    CONFIG_SRC_DIR="$SCRIPT_DIR/config"
else
    CONFIG_SRC_DIR="$SCRIPT_DIR"
fi

# 复制配置文件
RIVISION_YAML_SRC=""
if [ -f "$CONFIG_SRC_DIR/rivision.yaml.example" ]; then
    RIVISION_YAML_SRC="$CONFIG_SRC_DIR/rivision.yaml.example"
elif [ -f "$CONFIG_SRC_DIR/rivision.yaml" ]; then
    RIVISION_YAML_SRC="$CONFIG_SRC_DIR/rivision.yaml"
fi

if [ -n "$RIVISION_YAML_SRC" ]; then
    if [ ! -f "$INSTALL_DIR/rivision.yaml" ]; then
        cp "$RIVISION_YAML_SRC" "$INSTALL_DIR/rivision.yaml"
        log_success "配置文件已部署: $INSTALL_DIR/rivision.yaml"
    else
        log_info "配置文件已存在，跳过"
    fi
fi

# 复制 go2rtc 配置（智能更新）
GO2RTC_YAML_SRC=""
if [ -f "$CONFIG_SRC_DIR/go2rtc.yaml.example" ]; then
    GO2RTC_YAML_SRC="$CONFIG_SRC_DIR/go2rtc.yaml.example"
elif [ -f "$CONFIG_SRC_DIR/go2rtc.yaml" ]; then
    GO2RTC_YAML_SRC="$CONFIG_SRC_DIR/go2rtc.yaml"
fi

if [ -n "$GO2RTC_YAML_SRC" ]; then
    if [ -f "$INSTALL_DIR/go2rtc.yaml" ]; then
        status=$(compare_file "$GO2RTC_YAML_SRC" "$INSTALL_DIR/go2rtc.yaml")
        case "$status" in
            updated)
                # 备份旧配置
                cp "$INSTALL_DIR/go2rtc.yaml" "$INSTALL_DIR/go2rtc.yaml.bak"
                log_info "已备份旧配置: go2rtc.yaml.bak"
                cp "$GO2RTC_YAML_SRC" "$INSTALL_DIR/go2rtc.yaml"
                log_success "go2rtc 配置已更新"
                ;;
            same)
                log_info "go2rtc 配置未变更，跳过"
                ;;
            *)
                cp "$GO2RTC_YAML_SRC" "$INSTALL_DIR/go2rtc.yaml"
                log_success "go2rtc 配置已部署"
                ;;
        esac
    else
        cp "$GO2RTC_YAML_SRC" "$INSTALL_DIR/go2rtc.yaml"
        log_success "go2rtc 配置已部署: $INSTALL_DIR/go2rtc.yaml"
    fi
fi

# 复制 MP4 测试视频文件
if [ -d "$SCRIPT_DIR/mp4path" ]; then
    mp4_count=$(find "$SCRIPT_DIR/mp4path" -maxdepth 1 -name "*.mp4" 2>/dev/null | wc -l)
    if [ "$mp4_count" -gt 0 ]; then
        log_info "复制 MP4 视频文件 ($mp4_count 个)..."
        cp -n "$SCRIPT_DIR/mp4path/"*.mp4 "$INSTALL_DIR/mp4path/" 2>/dev/null || true
        log_success "MP4 视频已复制到: $INSTALL_DIR/mp4path/"
    fi
fi

# 复制模型文件
if [ -d "$SCRIPT_DIR/models" ]; then
    log_info "复制模型文件..."
    for model_file in "$SCRIPT_DIR/models"/*.onnx; do
        [ -f "$model_file" ] || continue
        model_name=$(basename "$model_file")
        if [ ! -f "$INSTALL_DIR/models/$model_name" ]; then
            cp "$model_file" "$INSTALL_DIR/models/$model_name"
            log_success "模型已部署: $model_name"
        else
            log_info "模型已存在，跳过: $model_name"
        fi
    done
    
    # 复制类别名称文件
    if [ -d "$SCRIPT_DIR/models/class_names" ]; then
        cp -n "$SCRIPT_DIR/models/class_names/"*.txt "$INSTALL_DIR/models/class_names/" 2>/dev/null || true
    fi
fi

# ============================================================
# Step 4: 配置 systemd 服务
# ============================================================
log_step "Step 4/5: 配置 systemd 服务"

# 创建 systemd 服务文件
SYSTEMD_SERVICE="/etc/systemd/system/rivision-cli.service"

log_info "配置 systemd 服务..."

# 构建 ExecStart 命令
EXEC_CMD="$INSTALL_DIR/rivision-cli webui --port 8280 --with-go2rtc --with-rtsp-server --mp4-dir $INSTALL_DIR/mp4path --no-browser"
if $ENABLE_YOLO; then
    EXEC_CMD="$EXEC_CMD --enable-yolo --yolo-mode distributed --gateway-url $GATEWAY_URL"
    log_info "YOLO 检测已启用 (分布式模式)"
fi

# 生成新的服务文件到临时文件，比较后再决定是否更新
SERVICE_TMP=$(mktemp)
cat > "$SERVICE_TMP" << EOF
[Unit]
Description=RiVision CLI Web UI
After=network.target

[Service]
Type=simple
User=$CURRENT_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$EXEC_CMD
Restart=on-failure
RestartSec=5
StandardOutput=append:$INSTALL_DIR/logs/rivision-cli.log
StandardError=append:$INSTALL_DIR/logs/rivision-cli.log

Environment="HOME=$HOME"
Environment="PATH=/usr/local/bin:/usr/bin:/bin"

[Install]
WantedBy=multi-user.target
EOF

if [ -f "$SYSTEMD_SERVICE" ]; then
    if diff -q "$SERVICE_TMP" "$SYSTEMD_SERVICE" &>/dev/null; then
        log_info "systemd 服务文件未变更，跳过"
    else
        sudo cp "$SERVICE_TMP" "$SYSTEMD_SERVICE"
        sudo systemctl daemon-reload
        SERVICE_CHANGED=true
        log_success "systemd 服务文件已更新"
    fi
else
    sudo cp "$SERVICE_TMP" "$SYSTEMD_SERVICE"
    sudo systemctl daemon-reload
    SERVICE_CHANGED=true
    log_success "systemd 服务已配置"
fi
rm -f "$SERVICE_TMP"

# 配置 logrotate（日志保留1个月）
log_info "配置日志轮转..."
sudo tee "/etc/logrotate.d/rivision-cli" > /dev/null << EOF
$INSTALL_DIR/logs/rivision-cli.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 644 $CURRENT_USER $CURRENT_USER
    postrotate
        systemctl reload rivision-cli > /dev/null 2>&1 || true
    endscript
}
EOF
log_success "日志轮转已配置 (保留30天)"

# ============================================================
# Step 5: 启动服务
# ============================================================
log_step "Step 5/5: 启动服务"

DEPLOY_OK=true

# 启动/重启服务
if ! systemctl is-active --quiet rivision-cli 2>/dev/null; then
    # 服务未运行（首次安装或升级时已停止）
    start_service
elif $BINARY_CHANGED || $SERVICE_CHANGED; then
    # 服务正在运行且有变更 → 重启
    log_info "检测到变更（二进制=$BINARY_CHANGED, 服务=$SERVICE_CHANGED），重启服务..."
    sudo systemctl restart rivision-cli || {
        log_warn "服务重启失败，尝试停止后启动..."
        stop_service
        start_service
    }
elif $CONFIG_CHANGED; then
    log_warn "配置文件有变更，请检查 .new 文件并手动重启: sudo systemctl restart rivision-cli"
else
    log_info "服务已在运行，无需重启"
fi

# 等待服务就绪
if $IS_RISCV; then
    log_info "等待服务就绪 (最多 30 秒)..."
    sleep 10
else
    log_info "等待服务就绪 (最多 15 秒)..."
    sleep 5
fi

# 健康检查
check_service() {
    local name="$1"
    local url="$2"
    local max_retries=5
    local i=0

    while [ $i -lt $max_retries ]; do
        if curl -sf "$url" -o /dev/null --max-time 5 2>/dev/null; then
            log_success "$name 运行正常 ($url)"
            return 0
        fi
        i=$((i + 1))
        [ $i -lt $max_retries ] && sleep 3
    done
    log_error "$name 未响应 ($url)"
    log_warn "  排查: journalctl -u rivision-cli -n 20 --no-pager"
    DEPLOY_OK=false
    return 1
}

check_service "RiVision CLI Web UI" "http://127.0.0.1:8280/" || true

# ============================================================
# 部署结果
# ============================================================
echo ""
if $DEPLOY_OK; then
    echo "╔══════════════════════════════════════════════╗"
    echo "║   部署完成! 服务运行正常                      ║"
    echo "╠══════════════════════════════════════════════╣"
    echo "║                                              ║"
    echo "║  访问地址:                                    ║"
    echo "║    http://$SERVER_IP:8280                    ║"
    echo "║                                              ║"
    echo "║  Gateway 交互:                                ║"
    echo "║    gw-shell node list                        ║"
    echo "║    gw-shell token generate --node-id xxx    ║"
    echo "║                                              ║"
    echo "║  查看日志:                                    ║"
    echo "║    journalctl -u rivision-cli -f             ║"
    echo "║                                              ║"
    echo "╚══════════════════════════════════════════════╝"
else
    echo "╔══════════════════════════════════════════════╗"
    echo "║   部署完成，但服务可能异常!                    ║"
    echo "╠══════════════════════════════════════════════╣"
    echo "║                                              ║"
    echo "║  请检查:                                      ║"
    echo "║    sudo systemctl status rivision-cli        ║"
    echo "║    journalctl -u rivision-cli -n 50          ║"
    echo "║                                              ║"
    echo "║  手动启动测试:                                ║"
    echo "║    cd $INSTALL_DIR                           ║"
    echo "║    ./rivision-cli webui                      ║"
    echo "║                                              ║"
    echo "╚══════════════════════════════════════════════╝"
fi
echo ""
