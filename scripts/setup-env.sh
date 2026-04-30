#!/bin/bash
# scripts/setup-env.sh — K3 RISC-V 构建环境一键安装
#
# 用法:
#   ./scripts/setup-env.sh           # 交互式安装（检测缺失项，确认后安装）
#   ./scripts/setup-env.sh --auto    # 自动安装（不询问，直接安装全部缺失项）
#   ./scripts/setup-env.sh --check   # 仅检查（等同 make check-env）
#   ./scripts/setup-env.sh --help    # 帮助
#
# 支持的依赖:
#   [必需] Go 1.25+          — 所有 Go 组件编译
#   [必需] cmake             — ZLMediaKit / yolo-server 编译
#   [必需] gcc/g++           — C/C++ 编译
#   [可选] Node.js 18+       — Web 前端构建
#   [运行] spacemit-onnxruntime — ONNX Runtime + SpacemiT EP (embed/yolo-server)
#   [运行] llama.cpp-tools-spacemit — VLM 推理 (llama-server, SpaceMIT 硬件加速)
set -e

# === 配置 ===
GO_MIN_VERSION="1.25.0"
GO_INSTALL_VERSION="1.26.2"
GO_MIRROR="https://mirrors.aliyun.com/golang"
NODE_MAJOR=22

AUTO=false
CHECK_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --auto)  AUTO=true ;;
        --check) CHECK_ONLY=true ;;
        --help|-h)
            sed -n '2,/^set -e/p' "$0" | grep '^#' | sed 's/^# \?//'
            exit 0
            ;;
    esac
done

# === 工具函数 ===
log()  { echo -e "\033[32m[setup]\033[0m $1"; }
warn() { echo -e "\033[33m[warn]\033[0m $1"; }
err()  { echo -e "\033[31m[error]\033[0m $1"; }
ask()  {
    if [ "$AUTO" = true ]; then return 0; fi
    read -rp "  是否安装? [Y/n] " ans
    case "$ans" in
        [nN]*) return 1 ;;
        *) return 0 ;;
    esac
}

# 版本比较: ver_ge "1.26.2" "1.25.0" → true
ver_ge() {
    [ "$(printf '%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}

MISSING_REQUIRED=()
MISSING_OPTIONAL=()
INSTALLED=()

# === 检测函数 ===

check_go() {
    # 检查 PATH 中和 /usr/local/go 中
    local go_bin=""
    if command -v go >/dev/null 2>&1; then
        go_bin="go"
    elif [ -x /usr/local/go/bin/go ]; then
        go_bin="/usr/local/go/bin/go"
    fi

    if [ -n "$go_bin" ]; then
        local ver
        ver=$($go_bin version | grep -oP 'go\K[0-9]+\.[0-9]+(\.[0-9]+)?')
        if ver_ge "$ver" "$GO_MIN_VERSION"; then
            log "Go $ver (>= $GO_MIN_VERSION)"
            # 确保在 PATH 中
            if ! command -v go >/dev/null 2>&1; then
                warn "Go 已安装在 /usr/local/go 但不在 PATH 中"
                warn "  添加到 ~/.bashrc: export PATH=\$PATH:/usr/local/go/bin"
            fi
            return 0
        else
            warn "Go $ver 版本过低 (需要 >= $GO_MIN_VERSION)"
            MISSING_REQUIRED+=("go")
            return 1
        fi
    else
        warn "Go 未安装"
        MISSING_REQUIRED+=("go")
        return 1
    fi
}

check_cmake() {
    if command -v cmake >/dev/null 2>&1; then
        local ver
        ver=$(cmake --version | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+')
        log "cmake $ver"
        return 0
    else
        warn "cmake 未安装"
        MISSING_REQUIRED+=("cmake")
        return 1
    fi
}

check_gcc() {
    if command -v gcc >/dev/null 2>&1; then
        local ver
        ver=$(gcc -dumpversion)
        log "gcc $ver"
        return 0
    else
        warn "gcc 未安装"
        MISSING_REQUIRED+=("gcc")
        return 1
    fi
}

check_nodejs() {
    if command -v node >/dev/null 2>&1; then
        local ver
        ver=$(node --version)
        log "Node.js $ver"
        return 0
    else
        warn "Node.js 未安装 (Web 前端构建需要)"
        MISSING_OPTIONAL+=("nodejs")
        return 1
    fi
}

check_ort() {
    # 必须是 spacemit-onnxruntime (含 SpacemiT EP 硬件加速)
    if dpkg -s spacemit-onnxruntime >/dev/null 2>&1; then
        local ver
        ver=$(dpkg -s spacemit-onnxruntime | grep '^Version:' | awk '{print $2}')
        log "spacemit-onnxruntime $ver (SpacemiT EP 硬件加速)"
        return 0
    fi
    # 检查上游 libonnxruntime (无 SpacemiT 加速，不满足要求)
    if dpkg -s libonnxruntime1.23 >/dev/null 2>&1 || ls /usr/lib/libonnxruntime.so* >/dev/null 2>&1; then
        warn "检测到上游 ONNX Runtime (无 SpacemiT 硬件加速)"
        warn "  必须替换为 spacemit-onnxruntime 以获得 K3 硬件加速"
        MISSING_REQUIRED+=("ort")
        return 1
    fi
    warn "ONNX Runtime 未安装 (embed/yolo-server 运行需要)"
    warn "  安装: sudo apt install spacemit-onnxruntime"
    MISSING_REQUIRED+=("ort")
    return 1
}

check_llama() {
    # 必须是 spacemit 版本 (含 K3 硬件加速，依赖 spacemit-onnxruntime)
    if dpkg -s llama.cpp-tools-spacemit >/dev/null 2>&1; then
        local ver
        ver=$(dpkg -s llama.cpp-tools-spacemit | grep '^Version:' | awk '{print $2}')
        log "llama.cpp-tools-spacemit $ver (SpacemiT 硬件加速)"
        return 0
    fi
    # 检查上游 llama.cpp-tools (无 SpacemiT 加速，不满足要求)
    if dpkg -s llama.cpp-tools >/dev/null 2>&1; then
        local ver
        ver=$(dpkg -s llama.cpp-tools | grep '^Version:' | awk '{print $2}')
        warn "检测到上游 llama.cpp-tools $ver (无 SpacemiT 硬件加速)"
        warn "  必须替换为 llama.cpp-tools-spacemit 以获得 K3 硬件加速"
        MISSING_REQUIRED+=("llama")
        return 1
    fi
    # 检查手动安装的 llama-server (不满足要求)
    if command -v llama-server >/dev/null 2>&1 || [ -x /usr/local/bin/llama-server ]; then
        local path
        path=$(command -v llama-server 2>/dev/null || echo "/usr/local/bin/llama-server")
        warn "检测到手动安装的 llama-server ($path)"
        warn "  必须使用 llama.cpp-tools-spacemit 包以获得 K3 硬件加速"
        MISSING_REQUIRED+=("llama")
        return 1
    fi
    warn "llama-server 未安装 (VLM 推理节点需要)"
    warn "  安装: sudo apt install llama.cpp-tools-spacemit"
    MISSING_REQUIRED+=("llama")
    return 1
}

# === 安装函数 ===

install_go() {
    local arch
    arch=$(uname -m)
    local goarch="riscv64"
    if [ "$arch" = "aarch64" ]; then goarch="arm64"; fi
    if [ "$arch" = "x86_64" ]; then goarch="amd64"; fi

    local tarball="go${GO_INSTALL_VERSION}.linux-${goarch}.tar.gz"
    local url="${GO_MIRROR}/${tarball}"

    log "安装 Go ${GO_INSTALL_VERSION} (${goarch})..."
    log "  下载: $url"

    # 备份旧版本
    if [ -d /usr/local/go ]; then
        log "  备份旧版本到 /usr/local/go.bak"
        sudo mv /usr/local/go /usr/local/go.bak
    fi

    curl -kLf --progress-bar -o "/tmp/${tarball}" "$url"
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"

    # 确保 PATH 包含 go
    if ! echo "$PATH" | grep -q "/usr/local/go/bin"; then
        export PATH=$PATH:/usr/local/go/bin
        # 写入 .bashrc（如果还没有）
        if ! grep -q '/usr/local/go/bin' ~/.bashrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
            log "  已添加 /usr/local/go/bin 到 ~/.bashrc"
        fi
    fi

    log "  Go $(/usr/local/go/bin/go version | grep -oP 'go[0-9]+\.[0-9]+\.[0-9]+') 安装完成"
    INSTALLED+=("go")
}

install_cmake() {
    log "安装 cmake..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq cmake
    log "  cmake $(cmake --version | head -1 | grep -oP '[0-9]+\.[0-9]+\.[0-9]+') 安装完成"
    INSTALLED+=("cmake")
}

install_gcc() {
    log "安装 build-essential (gcc/g++/make)..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq build-essential
    log "  gcc $(gcc -dumpversion) 安装完成"
    INSTALLED+=("gcc")
}

install_nodejs() {
    log "安装 Node.js ${NODE_MAJOR}..."
    if command -v apt-get >/dev/null 2>&1; then
        # 尝试 nodesource
        if curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" -o /tmp/nodesource_setup.sh 2>/dev/null; then
            sudo bash /tmp/nodesource_setup.sh
            sudo apt-get install -y -qq nodejs
            rm -f /tmp/nodesource_setup.sh
            log "  Node.js $(node --version) 安装完成"
            INSTALLED+=("nodejs")
        else
            err "  无法下载 nodesource 安装脚本"
            err "  请手动安装: https://nodejs.org/"
            return 1
        fi
    fi
}

install_ort() {
    log "安装 spacemit-onnxruntime..."
    if apt-cache show spacemit-onnxruntime >/dev/null 2>&1; then
        sudo apt-get install -y -qq spacemit-onnxruntime
        log "  ONNX Runtime 安装完成"
        INSTALLED+=("ort")
    else
        err "  spacemit-onnxruntime 包不在 apt 源中"
        err "  请确认已配置 SpaceMIT 软件源"
        return 1
    fi
}

install_llama() {
    log "安装 llama.cpp-tools-spacemit (SpacemiT 硬件加速版)..."
    if ! apt-cache show llama.cpp-tools-spacemit >/dev/null 2>&1; then
        err "  llama.cpp-tools-spacemit 包不在 apt 源中"
        err "  请确认已配置 SpaceMIT 软件源 (archive.spacemit.com)"
        return 1
    fi
    # spacemit 版本与上游 llama.cpp 系列包文件冲突，需先卸载
    local upstream_pkgs=""
    for pkg in llama.cpp llama.cpp-tools llama.cpp-tools-extra libllama-dev libllama0 libggml-dev libggml0; do
        if dpkg -s "$pkg" >/dev/null 2>&1; then
            upstream_pkgs="$upstream_pkgs $pkg"
        fi
    done
    if [ -n "$upstream_pkgs" ]; then
        log "  卸载冲突的上游包:$upstream_pkgs"
        sudo apt-get remove -y -qq $upstream_pkgs
    fi
    sudo apt-get install -y -qq llama.cpp-tools-spacemit
    local ver
    ver=$(dpkg -s llama.cpp-tools-spacemit | grep '^Version:' | awk '{print $2}')
    log "  llama.cpp-tools-spacemit $ver 安装完成"
    INSTALLED+=("llama")
}

# === 主流程 ===

echo ""
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  RiVision K3 环境检查与安装                                    ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

log "--- 构建工具 (必需) ---"
check_go || true
check_cmake || true
check_gcc || true

echo ""
log "--- 构建工具 (可选) ---"
check_nodejs || true

echo ""
log "--- 运行时依赖 (必需，须为 SpaceMIT 定制版) ---"
check_ort || true
check_llama || true

echo ""

# 仅检查模式
if [ "$CHECK_ONLY" = true ]; then
    if [ ${#MISSING_REQUIRED[@]} -gt 0 ]; then
        err "缺失必需依赖: ${MISSING_REQUIRED[*]}"
        err "运行 ./scripts/setup-env.sh 安装"
        exit 1
    fi
    log "环境检查通过"
    exit 0
fi

# 无缺失
if [ ${#MISSING_REQUIRED[@]} -eq 0 ] && [ ${#MISSING_OPTIONAL[@]} -eq 0 ]; then
    log "所有依赖已就绪，无需安装"
    exit 0
fi

# 显示安装计划
if [ ${#MISSING_REQUIRED[@]} -gt 0 ]; then
    echo "  必需安装: ${MISSING_REQUIRED[*]}"
fi
if [ ${#MISSING_OPTIONAL[@]} -gt 0 ]; then
    echo "  可选安装: ${MISSING_OPTIONAL[*]}"
fi
echo ""

# 安装必需依赖
for dep in "${MISSING_REQUIRED[@]}"; do
    log "--- 安装 $dep ---"
    if ask; then
        install_"$dep" || warn "$dep 安装失败，请手动安装"
    else
        warn "跳过 $dep"
    fi
    echo ""
done

# 安装可选依赖
for dep in "${MISSING_OPTIONAL[@]}"; do
    log "--- 安装 $dep (可选) ---"
    if ask; then
        install_"$dep" || warn "$dep 安装失败，请手动安装"
    else
        warn "跳过 $dep"
    fi
    echo ""
done

# 总结
echo ""
if [ ${#INSTALLED[@]} -gt 0 ]; then
    log "已安装: ${INSTALLED[*]}"
fi
log "完成"
echo ""
