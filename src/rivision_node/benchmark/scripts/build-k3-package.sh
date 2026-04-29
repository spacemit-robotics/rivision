#!/bin/bash
# ============================================================
# K3 Benchmark 打包脚本
# 用于构建和打包 rivision-benchmark-k3.tar.gz
# ============================================================

set -e  # 遇到错误立即退出

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
RIVISION_NODE_DIR="$(cd "$BENCHMARK_DIR/.." && pwd)"
PROJECT_ROOT="$(cd "$RIVISION_NODE_DIR/../.." && pwd)"

# 配置
BUILD_DIR="$BENCHMARK_DIR/build-riscv64"
DIST_DIR="$BENCHMARK_DIR/dist"
PACKAGE_NAME="rivision-benchmark-k3"
PACKAGE_DIR="/tmp/${PACKAGE_NAME}-full"
TOOLCHAIN_FILE="$BENCHMARK_DIR/cmake/toolchain-riscv64.cmake"

# SpacemiT 工具链路径
SPACEMIT_TOOLCHAIN="$PROJECT_ROOT/tools/spacemit-toolchain-linux-glibc-x86_64-v1.2.2"

# SpacemiT ORT 库路径
SPACEMIT_ORT_DIR="$RIVISION_NODE_DIR/yolo-server/third_party/spacemit-ort-current"
OPENCV_DIR="$BENCHMARK_DIR/third_party/riscv64/opencv"

# llama.cpp 路径 (使用 riscv64-current 符号链接)
LLAMA_CPP_DIR="$RIVISION_NODE_DIR/third_party/llama.cpp/riscv64-current"

echo -e "${BLUE}============================================================${NC}"
echo -e "${BLUE}K3 Benchmark 打包脚本${NC}"
echo -e "${BLUE}============================================================${NC}"
echo ""

# ============================================================
# 步骤 1: 检查依赖
# ============================================================
echo -e "${YELLOW}[1/6] 检查依赖...${NC}"

# 检查交叉编译工具链
if [ ! -f "$SPACEMIT_TOOLCHAIN/bin/riscv64-unknown-linux-gnu-g++" ]; then
    echo -e "${RED}错误: 未找到 SpacemiT 交叉编译工具链${NC}"
    echo -e "${RED}期望路径: $SPACEMIT_TOOLCHAIN${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} 交叉编译工具链: $SPACEMIT_TOOLCHAIN"

# 检查 CMake
if ! command -v cmake &> /dev/null; then
    echo -e "${RED}错误: 未找到 cmake${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} CMake: $(cmake --version | head -1)"

# 检查 SpacemiT ORT
if [ ! -d "$SPACEMIT_ORT_DIR" ]; then
    echo -e "${RED}错误: 未找到 SpacemiT ORT: $SPACEMIT_ORT_DIR${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} SpacemiT ORT: $SPACEMIT_ORT_DIR"

# 检查 OpenCV
if [ ! -d "$OPENCV_DIR" ]; then
    echo -e "${RED}错误: 未找到 OpenCV: $OPENCV_DIR${NC}"
    exit 1
fi
echo -e "  ${GREEN}✓${NC} OpenCV: $OPENCV_DIR"

echo ""

# ============================================================
# 步骤 2: 配置 CMake
# ============================================================
echo -e "${YELLOW}[2/6] 配置 CMake...${NC}"

mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

cmake "$BENCHMARK_DIR" \
    -DCMAKE_TOOLCHAIN_FILE="$TOOLCHAIN_FILE" \
    -DCMAKE_BUILD_TYPE=Release \
    2>&1 | tail -20

echo -e "  ${GREEN}✓${NC} CMake 配置完成"
echo ""

# ============================================================
# 步骤 3: 编译
# ============================================================
echo -e "${YELLOW}[3/6] 编译...${NC}"

make -j$(nproc) 2>&1 | tail -10

if [ ! -f "$BUILD_DIR/rivision-benchmark" ]; then
    echo -e "${RED}错误: 编译失败，未生成 rivision-benchmark${NC}"
    exit 1
fi

echo -e "  ${GREEN}✓${NC} 编译完成: $BUILD_DIR/rivision-benchmark"
echo ""

# ============================================================
# 步骤 4: 准备打包目录
# ============================================================
echo -e "${YELLOW}[4/6] 准备打包目录...${NC}"

rm -rf "$PACKAGE_DIR"
mkdir -p "$PACKAGE_DIR"/{bin,lib,config,models/yolo,models/vlm,data,results}

# 复制二进制文件
cp "$BUILD_DIR/rivision-benchmark" "$PACKAGE_DIR/bin/rivision-benchmark.riscv64"
# 模型检查工具 (可选)
if [ -f "$BUILD_DIR/check-model-output" ]; then
    cp "$BUILD_DIR/check-model-output" "$PACKAGE_DIR/bin/check-model-output.riscv64"
fi
echo -e "  ${GREEN}✓${NC} 二进制文件"

# 复制脚本和配置
cp "$BENCHMARK_DIR/scripts/package/k3-deploy/run-benchmark.sh" "$PACKAGE_DIR/"
chmod +x "$PACKAGE_DIR/run-benchmark.sh"
cp "$BENCHMARK_DIR/scripts/package/k3-deploy/config/benchmark.yaml" "$PACKAGE_DIR/config/"
echo -e "  ${GREEN}✓${NC} 脚本和配置"

# 复制 SpacemiT ORT 库 (使用通配符匹配所有版本)
echo -e "  复制 SpacemiT ORT 库..."
for lib in libonnxruntime.so* libspacemit_ep.so* libonnxruntime_providers_shared.so*; do
    if ls "$SPACEMIT_ORT_DIR/lib/"$lib &>/dev/null 2>&1; then
        cp -P "$SPACEMIT_ORT_DIR/lib/"$lib "$PACKAGE_DIR/lib/" 2>/dev/null || true
    fi
done
# 显示已复制的 ORT 库
for lib in libonnxruntime.so libspacemit_ep.so libonnxruntime_providers_shared.so; do
    if [ -f "$PACKAGE_DIR/lib/$lib" ] || [ -L "$PACKAGE_DIR/lib/$lib" ]; then
        echo -e "    ${GREEN}✓${NC} $lib"
    fi
done

# 复制 OpenCV 库
echo -e "  复制 OpenCV 库..."
for lib in libopencv_core.so* libopencv_imgproc.so* libopencv_imgcodecs.so* \
           libopencv_dnn.so* libopencv_videoio.so*; do
    if ls "$OPENCV_DIR/lib/"$lib &>/dev/null 2>&1; then
        cp -P "$OPENCV_DIR/lib/"$lib "$PACKAGE_DIR/lib/" 2>/dev/null || true
    fi
done
echo -e "    ${GREEN}✓${NC} OpenCV 库"

# 复制 curl 库
CURL_DIR="$BENCHMARK_DIR/third_party/riscv64/curl"
echo -e "  复制 curl 库..."
if [ -d "$CURL_DIR/lib" ]; then
    cp -P "$CURL_DIR/lib/"libcurl.so* "$PACKAGE_DIR/lib/" 2>/dev/null || true
    echo -e "    ${GREEN}✓${NC} curl 库"
else
    echo -e "    ${YELLOW}⚠${NC} curl 库未找到 (HTTP 后端将不可用)"
fi

# MPP 库已废弃，使用 FFmpeg 方案替代
# 如需启用 MPP，请设置 cmake -DUSE_K3_MPP=ON
echo -e "  MPP 预处理..."
echo -e "    ${BLUE}ℹ${NC}  已废弃，使用 FFmpeg 方案"

# 复制 FFmpeg (硬件 JPEG 解码)
FFMPEG_DIR="$BENCHMARK_DIR/third_party/riscv64/ffmpeg"
echo -e "  FFmpeg 硬件 JPEG 解码 (mjpeg_stcodec)..."
if [ -d "$FFMPEG_DIR/lib" ] && ls "$FFMPEG_DIR/lib/"libavcodec.so* &>/dev/null 2>&1; then
    # 复制所有 FFmpeg 库
    for lib in libavcodec libavformat libavutil libswscale libswresample; do
        if ls "$FFMPEG_DIR/lib/"${lib}.so* &>/dev/null 2>&1; then
            cp -P "$FFMPEG_DIR/lib/"${lib}.so* "$PACKAGE_DIR/lib/" 2>/dev/null || true
        fi
    done
    echo -e "    ${GREEN}✓${NC} FFmpeg 库已打包 (libavcodec, libavformat, libavutil, libswscale)"
else
    echo -e "    ${YELLOW}⚠${NC} FFmpeg 未配置 (使用 OpenCV 软件解码)"
    echo -e "    ${BLUE}ℹ${NC}  获取 FFmpeg: ./scripts/fetch-k3-ffmpeg.sh node@k3"
fi

# 复制 llama.cpp (VLM 支持)
echo -e "  复制 llama.cpp (VLM 支持)..."
if [ -d "$LLAMA_CPP_DIR" ]; then
    # 复制库文件
    for lib in libllama.so* libggml.so* libggml-base.so* libggml-cpu.so* libmtmd.so*; do
        if ls "$LLAMA_CPP_DIR/lib/"$lib &>/dev/null 2>&1; then
            cp -P "$LLAMA_CPP_DIR/lib/"$lib "$PACKAGE_DIR/lib/" 2>/dev/null || true
        fi
    done
    echo -e "    ${GREEN}✓${NC} llama.cpp 库 (libllama, libggml, libmtmd)"
    
    # 复制 llama-mtmd-cli (VLM 推理)
    if [ -f "$LLAMA_CPP_DIR/bin/llama-mtmd-cli" ]; then
        cp "$LLAMA_CPP_DIR/bin/llama-mtmd-cli" "$PACKAGE_DIR/bin/"
        echo -e "    ${GREEN}✓${NC} llama-mtmd-cli (VLM 推理引擎)"
    else
        echo -e "    ${RED}✗${NC} llama-mtmd-cli 未找到"
    fi
    
    # 复制 llama-cli (可选，用于文本推理)
    if [ -f "$LLAMA_CPP_DIR/bin/llama-cli" ]; then
        cp "$LLAMA_CPP_DIR/bin/llama-cli" "$PACKAGE_DIR/bin/"
        echo -e "    ${GREEN}✓${NC} llama-cli (文本推理)"
    fi
else
    echo -e "    ${YELLOW}⚠${NC} llama.cpp 未找到: $LLAMA_CPP_DIR"
    echo -e "    ${YELLOW}⚠${NC} VLM benchmark 将不可用"
fi

# 复制测试数据
if [ -f "$BENCHMARK_DIR/scripts/package/k3-deploy/data/test.jpg" ]; then
    cp "$BENCHMARK_DIR/scripts/package/k3-deploy/data/test.jpg" "$PACKAGE_DIR/data/"
    echo -e "  ${GREEN}✓${NC} 测试数据"
else
    echo -e "  ${YELLOW}⚠${NC} 未找到测试图片，请手动添加 data/test.jpg"
fi

echo ""

# ============================================================
# 步骤 5: 验证库依赖
# ============================================================
echo -e "${YELLOW}[5/6] 验证库依赖...${NC}"

# 检查 rivision-benchmark 的依赖
echo -e "  rivision-benchmark 依赖:"
MISSING_LIBS=""
for lib in $(readelf -d "$PACKAGE_DIR/bin/rivision-benchmark.riscv64" 2>/dev/null | grep NEEDED | awk '{print $5}' | tr -d '[]'); do
    # 检查是否在 lib 目录中
    base_lib=$(echo "$lib" | sed 's/\.so.*/\.so/')
    if ls "$PACKAGE_DIR/lib/"${base_lib}* &>/dev/null || \
       [[ "$lib" == "libc.so"* ]] || [[ "$lib" == "libm.so"* ]] || \
       [[ "$lib" == "libpthread.so"* ]] || [[ "$lib" == "libdl.so"* ]] || \
       [[ "$lib" == "libstdc++.so"* ]] || [[ "$lib" == "libgcc_s.so"* ]] || \
       [[ "$lib" == "librt.so"* ]]; then
        echo -e "    ${GREEN}✓${NC} $lib"
    else
        echo -e "    ${RED}✗${NC} $lib (缺失)"
        MISSING_LIBS="$MISSING_LIBS $lib"
    fi
done

if [ -n "$MISSING_LIBS" ]; then
    echo -e "  ${YELLOW}警告: 部分系统库缺失，这些库应该在目标系统上存在${NC}"
fi

echo ""

# ============================================================
# 步骤 6: 创建 tar.gz 包
# ============================================================
echo -e "${YELLOW}[6/6] 创建 tar.gz 包...${NC}"

mkdir -p "$DIST_DIR"
cd /tmp
rm -f "${PACKAGE_NAME}.tar.gz"
tar -czf "${PACKAGE_NAME}.tar.gz" "${PACKAGE_NAME}-full"
mv "${PACKAGE_NAME}.tar.gz" "$DIST_DIR/"

PACKAGE_SIZE=$(ls -lh "$DIST_DIR/${PACKAGE_NAME}.tar.gz" | awk '{print $5}')
echo -e "  ${GREEN}✓${NC} 打包完成: $DIST_DIR/${PACKAGE_NAME}.tar.gz ($PACKAGE_SIZE)"

echo ""
echo -e "${GREEN}============================================================${NC}"
echo -e "${GREEN}打包成功!${NC}"
echo -e "${GREEN}============================================================${NC}"
echo ""
echo -e "部署命令:"
echo -e "  scp $DIST_DIR/${PACKAGE_NAME}.tar.gz node@node3:~/tmp/"
echo -e "  ssh node@node3 \"cd ~/tmp && tar -xzf ${PACKAGE_NAME}.tar.gz && cd ${PACKAGE_NAME}-full && ./run-benchmark.sh check\""
echo ""
