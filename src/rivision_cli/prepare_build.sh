#!/bin/bash
# 构建前准备：将对应平台的二进制复制到 binaries/ 目录
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EMBED_DIR="$SCRIPT_DIR/internal/embed/binaries"
PLATFORM_DIR="$EMBED_DIR/platform"

PLATFORM=${1:-riscv64}
FFMPEG_VER=${2:-}

usage() {
    echo "Usage: $0 <platform> [ffmpeg_version]"
    echo ""
    echo "Platforms:"
    echo "  riscv64              - RISC-V K3 (ffmpeg 6.2)"
    echo "  x86_64               - x86_64 (ffmpeg 6.1, default)"
    echo "  x86_64 ffmpeg44      - x86_64 (ffmpeg 4.4)"
    echo ""
    echo "Examples:"
    echo "  $0 riscv64"
    echo "  $0 x86_64"
    echo "  $0 x86_64 ffmpeg44"
}

# 确定源目录
case "$PLATFORM" in
    riscv64)
        SRC_DIR="$PLATFORM_DIR/riscv64"
        GOARCH="riscv64"
        ;;
    x86_64|amd64)
        GOARCH="amd64"
        if [ "$FFMPEG_VER" = "ffmpeg44" ]; then
            SRC_DIR="$PLATFORM_DIR/x86_64-ffmpeg44"
        else
            SRC_DIR="$PLATFORM_DIR/x86_64-ffmpeg61"
        fi
        ;;
    -h|--help)
        usage
        exit 0
        ;;
    *)
        echo "ERROR: Unknown platform: $PLATFORM"
        usage
        exit 1
        ;;
esac

if [ ! -d "$SRC_DIR" ]; then
    echo "ERROR: Platform binaries not found: $SRC_DIR"
    exit 1
fi

# 清理旧的二进制（保留 platform/ 目录和 .gitkeep）
find "$EMBED_DIR" -maxdepth 1 -type f ! -name ".gitkeep" -delete

# 复制对应平台的二进制
echo "Preparing binaries for $PLATFORM (GOARCH=$GOARCH)..."
for f in "$SRC_DIR"/*; do
    fname=$(basename "$f")
    # 重命名为 runtime.GOARCH 期望的格式
    case "$fname" in
        go2rtc-linux-*)
            dest="$EMBED_DIR/go2rtc-linux-$GOARCH"
            ;;
        rtsp_server-linux-*)
            dest="$EMBED_DIR/rtsp_server-linux-$GOARCH"
            ;;
        *)
            dest="$EMBED_DIR/$fname"
            ;;
    esac
    cp "$f" "$dest"
    echo "  $fname -> $(basename $dest)"
done

echo "Done. Ready to build for $PLATFORM."
