#!/bin/bash
# scripts/build-rtsp-server.sh — 在 K3 上从源码编译 simple-rtsp-server
#
# 用法:
#   ./scripts/build-rtsp-server.sh              # 编译并安装到 embed/binaries
#   ./scripts/build-rtsp-server.sh --clean      # 清理构建缓存
#
# 源码: https://github.com/BreakingY/simple-rtsp-server
# 前提: K3 上已安装 cmake, gcc/g++
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_CACHE="$ROOT/.cache/simple-rtsp-server"
TARGET_DIR="$ROOT/src/rivision_cli/internal/embed/binaries"
ARCH="$(uname -m)"
REPO_URL="https://github.com/BreakingY/simple-rtsp-server.git"
BRANCH="master"

BINARY_NAME="rtsp_server-linux-${ARCH}"

# 参数解析
CLEAN=false
for arg in "$@"; do
    case "$arg" in
        --branch=*) BRANCH="${arg#*=}" ;;
        --clean)    CLEAN=true ;;
        --help)
            echo "用法: $0 [--branch BRANCH] [--clean]"
            exit 0
            ;;
    esac
done

log() { echo -e "\033[32m[build-rtsp]\033[0m $1"; }
warn() { echo -e "\033[33m[warn]\033[0m $1"; }

if [ "$CLEAN" = true ]; then
    log "清理构建缓存: $BUILD_CACHE"
    rm -rf "$BUILD_CACHE"
    exit 0
fi

# 检查构建工具
for cmd in cmake gcc g++; do
    if ! command -v $cmd >/dev/null 2>&1; then
        echo "错误: 未找到 $cmd"
        exit 1
    fi
done

log "架构: $ARCH"
log "目标: $BINARY_NAME"

# 克隆或更新源码
mkdir -p "$BUILD_CACHE"
if [ -d "$BUILD_CACHE/src/.git" ]; then
    log "更新源码..."
    cd "$BUILD_CACHE/src"
    git pull
else
    log "克隆 simple-rtsp-server..."
    git clone "$REPO_URL" "$BUILD_CACHE/src"
    cd "$BUILD_CACHE/src"
fi

git checkout "$BRANCH"

# 构建
log "编译中..."
mkdir -p "$BUILD_CACHE/src/build-static"
cd "$BUILD_CACHE/src/build-static"
cmake .. -DCMAKE_BUILD_TYPE=Release
make -j$(nproc)

# 查找生成的二进制
BUILT_BIN=""
for candidate in rtsp_server_file rtsp_server; do
    if [ -f "$candidate" ]; then
        BUILT_BIN="$candidate"
        break
    fi
done

if [ -z "$BUILT_BIN" ]; then
    # 搜索 example 目录
    BUILT_BIN=$(find . -name "rtsp_server_file" -type f -executable 2>/dev/null | head -1)
fi

if [ -z "$BUILT_BIN" ]; then
    warn "未找到编译产物，列出可执行文件:"
    find . -type f -executable | head -20
    exit 1
fi

# 安装
mkdir -p "$TARGET_DIR"
cp "$BUILT_BIN" "$TARGET_DIR/$BINARY_NAME"
chmod +x "$TARGET_DIR/$BINARY_NAME"

log "完成: $TARGET_DIR/$BINARY_NAME"
ls -lh "$TARGET_DIR/$BINARY_NAME"
