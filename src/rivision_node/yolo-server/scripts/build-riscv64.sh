#!/bin/bash
# yolo-server RISC-V  cross-compilation script
# Based on ZLMediaKit Makefile.rivision pattern

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
YOLO_SERVER_DIR="$SCRIPT_DIR/.."
# 项目根目录: yolo-server -> rivision_node -> src -> rivision-k3-deploy
PROJECT_ROOT="$(cd "$YOLO_SERVER_DIR/../../.." && pwd)"

BUILD_TYPE="Release"
JOBS=$(nproc)
BUILD_DIR="build-riscv64"
ARCH="$(uname -m)"

# 进入 yolo-server 目录（脚本可从任意位置调用）
cd "$YOLO_SERVER_DIR"

# Create build directory
mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

if [ "$ARCH" = "riscv64" ]; then
    # 原生编译 (K3 上直接编译)
    echo "=== Building yolo-server for RISC-V 64 (Native) ==="
    echo "    Compiler: $(gcc --version | head -1)"
    echo "    Build Dir: $BUILD_DIR"
    echo ""

    echo "[1/2] Configuring (native)..."
    cmake .. \
        -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
        -DCMAKE_C_FLAGS="-march=rv64gcv -O3 -ffast-math" \
        -DCMAKE_CXX_FLAGS="-march=rv64gcv -O3 -ffast-math"
else
    # 交叉编译 (x86 上编译 RISC-V)
    SPACEMIT_TOOLCHAIN="${PROJECT_ROOT}/tools/spacemit-toolchain-linux-glibc-x86_64-v1.2.2"
    RISCV_CC="${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-gcc"
    RISCV_CXX="${SPACEMIT_TOOLCHAIN}/bin/riscv64-unknown-linux-gnu-g++"
    RISCV_SYSROOT="${SPACEMIT_TOOLCHAIN}/sysroot"

    echo "=== Building yolo-server for RISC-V 64 (Cross-compile) ==="
    echo "    Toolchain: $SPACEMIT_TOOLCHAIN"
    echo "    Build Dir: $BUILD_DIR"
    echo ""

    if [ ! -x "$RISCV_CC" ]; then
        echo "ERROR: $RISCV_CC not found or not executable"
        echo "  Please ensure spacemit toolchain is extracted to: $SPACEMIT_TOOLCHAIN"
        exit 1
    fi

    # Create toolchain file
    echo 'set(CMAKE_SYSTEM_NAME Linux)' > riscv64-toolchain.cmake
    echo 'set(CMAKE_SYSTEM_PROCESSOR riscv64)' >> riscv64-toolchain.cmake
    echo "set(CMAKE_C_COMPILER \"$RISCV_CC\")" >> riscv64-toolchain.cmake
    echo "set(CMAKE_CXX_COMPILER \"$RISCV_CXX\")" >> riscv64-toolchain.cmake
    echo "set(CMAKE_SYSROOT \"$RISCV_SYSROOT\")" >> riscv64-toolchain.cmake
    echo "set(CMAKE_FIND_ROOT_PATH \"$RISCV_SYSROOT\")" >> riscv64-toolchain.cmake
    echo 'set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)' >> riscv64-toolchain.cmake
    echo 'set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)' >> riscv64-toolchain.cmake
    echo 'set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)' >> riscv64-toolchain.cmake
    echo 'set(CMAKE_FIND_ROOT_PATH_MODE_PACKAGE ONLY)' >> riscv64-toolchain.cmake
    echo 'set(CMAKE_C_FLAGS "-march=rv64gcv -O3 -ffast-math" CACHE STRING "")' >> riscv64-toolchain.cmake
    echo 'set(CMAKE_CXX_FLAGS "-march=rv64gcv -O3 -ffast-math" CACHE STRING "")' >> riscv64-toolchain.cmake

    echo "[1/2] Configuring (cross-compile)..."
    cmake .. \
        -DCMAKE_TOOLCHAIN_FILE=riscv64-toolchain.cmake \
        -DCMAKE_BUILD_TYPE="$BUILD_TYPE"
fi

# Build
echo "[2/2] Building..."
make -j"$JOBS"

# Copy to prebuilt location (consistent with current structure)
OUTPUT_DIR="$YOLO_SERVER_DIR/bin"
mkdir -p "$OUTPUT_DIR"
cp yolo-server "$OUTPUT_DIR/yolo-server.riscv64"

echo ""
echo "=== Build Complete ==="
echo "Output: $OUTPUT_DIR/yolo-server.riscv64"
echo "Size: $(du -sh $OUTPUT_DIR/yolo-server.riscv64 | cut -f1)"
echo "Architecture: $(file $OUTPUT_DIR/yolo-server.riscv64 | grep -oE "x86-64|RISC-V" || echo "Unknown")"
echo ""
echo "Next step: make package-riscv_b"
