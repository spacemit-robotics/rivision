#!/bin/bash
# scripts/build-go2rtc.sh — 在 K3 上从源码编译 go2rtc
#
# 用法:
#   ./scripts/build-go2rtc.sh              # 编译并安装到 embed/binaries
#   ./scripts/build-go2rtc.sh --version v1.9.8  # 指定版本
#   ./scripts/build-go2rtc.sh --clean      # 清理构建缓存
#
# 前提: K3 上已安装 Go 1.25+
set -e

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_CACHE="$ROOT/.cache/go2rtc"
TARGET_DIR="$ROOT/src/rivision_cli/internal/embed/binaries"
ARCH="$(uname -m)"
GO2RTC_REPO="https://github.com/AlexxIT/go2rtc.git"
GO2RTC_VERSION="v1.9.8"

# 架构映射
case "$ARCH" in
    riscv64) GO_ARCH="riscv64" ;;
    x86_64)  GO_ARCH="amd64" ;;
    aarch64) GO_ARCH="arm64" ;;
    *)       echo "不支持的架构: $ARCH"; exit 1 ;;
esac

BINARY_NAME="go2rtc-linux-${ARCH}"

# 参数解析
CLEAN=false
for arg in "$@"; do
    case "$arg" in
        --version=*) GO2RTC_VERSION="${arg#*=}" ;;
        --version)   shift; GO2RTC_VERSION="$2" ;;
        --clean)     CLEAN=true ;;
        --help)
            echo "用法: $0 [--version TAG] [--clean]"
            exit 0
            ;;
    esac
done

log() { echo -e "\033[32m[build-go2rtc]\033[0m $1"; }
warn() { echo -e "\033[33m[warn]\033[0m $1"; }

if [ "$CLEAN" = true ]; then
    log "清理构建缓存: $BUILD_CACHE"
    rm -rf "$BUILD_CACHE"
    exit 0
fi

# 检查 Go
if ! command -v go >/dev/null 2>&1; then
    echo "错误: 未找到 go 命令"
    exit 1
fi
log "Go: $(go version)"
log "目标: $BINARY_NAME ($GO2RTC_VERSION)"

# 克隆或更新源码
mkdir -p "$BUILD_CACHE"
if [ -d "$BUILD_CACHE/go2rtc/.git" ]; then
    log "更新 go2rtc 源码..."
    cd "$BUILD_CACHE/go2rtc"
    git fetch --tags
else
    log "克隆 go2rtc..."
    git clone "$GO2RTC_REPO" "$BUILD_CACHE/go2rtc"
    cd "$BUILD_CACHE/go2rtc"
fi

# 切换到指定版本
git checkout "$GO2RTC_VERSION"
log "版本: $(git describe --tags --always 2>/dev/null || echo $GO2RTC_VERSION)"

# 编译
log "编译中..."
CGO_ENABLED=0 GOOS=linux GOARCH="$GO_ARCH" \
    go build -trimpath -ldflags "-s -w" -o "$BINARY_NAME" .

# 安装
mkdir -p "$TARGET_DIR"
mv "$BINARY_NAME" "$TARGET_DIR/$BINARY_NAME"
chmod +x "$TARGET_DIR/$BINARY_NAME"

log "完成: $TARGET_DIR/$BINARY_NAME"
ls -lh "$TARGET_DIR/$BINARY_NAME"
