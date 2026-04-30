#!/bin/bash
# yolo-server 本地编译脚本 (在 Jetson Orin 上运行)
# 解决 ORT 1.22.0 需要 GLIBCXX_3.4.30 的问题
#
# 用法: 在 Jetson 上执行
#   1. 复制 yolo-server 源码到 Jetson
#   2. 运行 ./scripts/build-native.sh
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$SCRIPT_DIR/.."

# 检测 ORT 目录 (支持多种命名)
ORT_DIR=""
for dir in "$PROJECT_ROOT/third_party/onnxruntime_arm64_gpu" \
           "$PROJECT_ROOT/third_party/onnxruntime-linux-aarch64"*; do
    if [ -d "$dir" ] && [ -f "$dir/lib/libonnxruntime.so" ]; then
        ORT_DIR="$dir"
        break
    fi
done

if [ -z "$ORT_DIR" ]; then
    echo "ERROR: ONNX Runtime not found in third_party/"
    echo "  请确保 onnxruntime_arm64_gpu 目录存在且包含 lib/libonnxruntime.so"
    exit 1
fi

BUILD_TYPE="Release"
JOBS=$(nproc)
BUILD_DIR="build-native"

echo "=== Building yolo-server (Native on Jetson) ==="
echo "    Compiler: $(g++ --version | head -1)"
echo "    ORT Dir: $ORT_DIR"
echo "    Build Dir: $BUILD_DIR"
echo ""

# 进入项目目录
cd "$PROJECT_ROOT"

# 清理并创建构建目录
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

# CMake 配置
echo "[1/2] Configuring..."
cmake .. \
    -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
    -DSPACEMIT_ORT_DIR="$ORT_DIR" \
    -DCMAKE_CXX_FLAGS="-O3 -ffast-math"

# 编译
echo "[2/2] Building..."
make -j"$JOBS"

# 复制到 bin 目录
OUTPUT_DIR="$PROJECT_ROOT/bin"
mkdir -p "$OUTPUT_DIR"
cp yolo-server "$OUTPUT_DIR/yolo-server"

echo ""
echo "=== Build Complete ==="
echo "Output: $OUTPUT_DIR/yolo-server"
echo "Size: $(du -sh $OUTPUT_DIR/yolo-server | cut -f1)"
echo ""
echo "验证 CUDA EP:"
echo "  ./bin/yolo-server -model /path/to/yolov8n.onnx"
echo "  应显示: [YOLODetector] ✓ CUDA EP enabled"
