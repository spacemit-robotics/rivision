#!/bin/bash
# RiVision Benchmark K3 打包脚本
# 打包编译结果和模型文件，用于在 K3 目标平台上运行
#
# Usage: ./package-k3.sh [output_name]
# Output: benchmark-k3-<date>.tar.gz

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PROJECT_ROOT="$(cd "$BENCHMARK_DIR/../../.." && pwd)"
RIVISION_NODE="$(cd "$BENCHMARK_DIR/.." && pwd)"

OUTPUT_NAME="${1:-benchmark-k3-$(date +%Y%m%d)}"
PACKAGE_DIR="/tmp/$OUTPUT_NAME"
OUTPUT_FILE="$BENCHMARK_DIR/dist/${OUTPUT_NAME}.tar.gz"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ============================================================
# 检查构建结果
# ============================================================
check_build() {
    info "Checking build artifacts..."
    
    if [ ! -f "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" ]; then
        warn "rivision-benchmark.riscv64 not found, building..."
        "$BENCHMARK_DIR/scripts/build/build.sh" riscv64
    fi
    
    # 检查 yolo-server
    local yolo_bin="$RIVISION_NODE/yolo-server/bin/yolo-server.riscv64"
    if [ ! -f "$yolo_bin" ]; then
        warn "yolo-server.riscv64 not found"
        YOLO_SERVER_AVAILABLE=false
    else
        YOLO_SERVER_AVAILABLE=true
    fi
    
    # 检查 llama-mtmd-cli
    local llama_bin="$RIVISION_NODE/third_party/llama.cpp/riscv64/build/bin/llama-mtmd-cli"
    if [ ! -f "$llama_bin" ]; then
        warn "llama-mtmd-cli not found"
        LLAMA_CLI_AVAILABLE=false
    else
        LLAMA_CLI_AVAILABLE=true
    fi
}

# ============================================================
# 创建包目录结构
# ============================================================
create_package_structure() {
    info "Creating package structure..."
    
    rm -rf "$PACKAGE_DIR"
    mkdir -p "$PACKAGE_DIR"/{bin,lib,models/{yolo,vlm},scripts,config}
}

# ============================================================
# 复制二进制文件
# ============================================================
copy_binaries() {
    info "Copying binaries..."
    
    # rivision-benchmark
    cp "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" "$PACKAGE_DIR/bin/"
    
    # yolo-server
    if [ "$YOLO_SERVER_AVAILABLE" = "true" ]; then
        cp "$RIVISION_NODE/yolo-server/bin/yolo-server.riscv64" "$PACKAGE_DIR/bin/"
    fi
    
    # llama 工具
    if [ "$LLAMA_CLI_AVAILABLE" = "true" ]; then
        cp "$RIVISION_NODE/third_party/llama.cpp/riscv64/build/bin/llama-mtmd-cli" \
           "$PACKAGE_DIR/bin/"
        # llama-server (如果有)
        local llama_server="$RIVISION_NODE/third_party/llama.cpp/riscv64/build/bin/llama-server"
        [ -f "$llama_server" ] && cp "$llama_server" "$PACKAGE_DIR/bin/"
    fi
}

# ============================================================
# 复制库文件
# ============================================================
copy_libraries() {
    info "Copying libraries..."
    
    # ONNX Runtime (SpacemiT NPU) - 使用符号链接
    local ort_dir="$RIVISION_NODE/yolo-server/third_party/spacemit-ort-current"
    if [ -d "$ort_dir/lib" ]; then
        cp -a "$ort_dir/lib/"*.so* "$PACKAGE_DIR/lib/" 2>/dev/null || true
    fi
    
    # OpenCV (如果是静态链接则不需要)
    # cp -a /usr/lib/riscv64-linux-gnu/libopencv*.so* "$PACKAGE_DIR/lib/" 2>/dev/null || true
}

# ============================================================
# 复制模型文件
# ============================================================
copy_models() {
    info "Copying model files..."
    
    local models_src="$RIVISION_NODE/models"
    
    # YOLO 模型
    info "  - YOLO models..."
    [ -f "$models_src/shared/yolov8n.onnx" ] && \
        cp "$models_src/shared/yolov8n.onnx" "$PACKAGE_DIR/models/yolo/"
    [ -f "$models_src/shared/910byolo11n_b2.q.onnx" ] && \
        cp "$models_src/shared/910byolo11n_b2.q.onnx" "$PACKAGE_DIR/models/yolo/"
    
    # VLM 模型 - 可选 (文件较大)
    if [ "${INCLUDE_VLM_MODELS:-false}" = "true" ]; then
        info "  - VLM models (large files)..."
        
        # Qwen3VL
        if [ -d "$models_src/shared/Qwen3VL" ]; then
            mkdir -p "$PACKAGE_DIR/models/vlm/Qwen3VL"
            cp "$models_src/shared/Qwen3VL/"*.gguf "$PACKAGE_DIR/models/vlm/Qwen3VL/"
        fi
        
        # FastVLM (K3 优化)
        if [ -d "$models_src/riscv_b/fastvlm-mm-0.5b-q4_1" ]; then
            cp -r "$models_src/riscv_b/fastvlm-mm-0.5b-q4_1" "$PACKAGE_DIR/models/vlm/"
        fi
        
        # MiniCPM
        if [ -d "$models_src/shared/miniCPM_v4.5" ]; then
            mkdir -p "$PACKAGE_DIR/models/vlm/miniCPM_v4.5"
            cp "$models_src/shared/miniCPM_v4.5/"*.gguf "$PACKAGE_DIR/models/vlm/miniCPM_v4.5/"
        fi
    else
        info "  - VLM models skipped (set INCLUDE_VLM_MODELS=true to include)"
    fi
}

# ============================================================
# 复制脚本和配置
# ============================================================
copy_scripts() {
    info "Copying scripts and configs..."
    
    # 运行脚本
    cp "$BENCHMARK_DIR/scripts/run/run-k3-benchmark.sh" "$PACKAGE_DIR/scripts/"
    cp "$BENCHMARK_DIR/scripts/run/run-local.sh" "$PACKAGE_DIR/scripts/" 2>/dev/null || true
    
    # 配置文件
    cp "$BENCHMARK_DIR/profiles/riscv_b.yaml" "$PACKAGE_DIR/config/"
    cp -r "$BENCHMARK_DIR/models/"*.yaml "$PACKAGE_DIR/config/" 2>/dev/null || true
    
    # 测试图片
    [ -f "$PROJECT_ROOT/test.jpg" ] && cp "$PROJECT_ROOT/test.jpg" "$PACKAGE_DIR/"
}

# ============================================================
# 创建启动脚本
# ============================================================
create_run_script() {
    info "Creating run script..."
    
    cat > "$PACKAGE_DIR/run.sh" << 'EOF'
#!/bin/bash
# RiVision Benchmark K3 运行脚本
# Usage: ./run.sh [yolo|vlm|all] [preset]

set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 设置环境
export LD_LIBRARY_PATH="$SCRIPT_DIR/lib:$LD_LIBRARY_PATH"
export PATH="$SCRIPT_DIR/bin:$PATH"

MODE="${1:-all}"
PRESET="${2:-quick}"

echo "=== RiVision Benchmark for K3 ==="
echo "Mode: $MODE, Preset: $PRESET"
echo ""

# 检查测试图片
TEST_IMAGE="$SCRIPT_DIR/test.jpg"
if [ ! -f "$TEST_IMAGE" ]; then
    echo "[WARN] test.jpg not found, creating placeholder..."
    python3 -c "import cv2; import numpy as np; cv2.imwrite('$TEST_IMAGE', np.random.randint(0,255,(640,640,3),dtype=np.uint8))" 2>/dev/null || \
    echo "[ERROR] Cannot create test image"
fi

case "$MODE" in
    yolo)
        echo "=== YOLO Benchmark ==="
        if [ -f "$SCRIPT_DIR/bin/rivision-benchmark.riscv64" ]; then
            "$SCRIPT_DIR/bin/rivision-benchmark.riscv64" yolo-local \
                --model "$SCRIPT_DIR/models/yolo/yolov8n.onnx" \
                --image "$TEST_IMAGE" \
                --runs 100
        else
            echo "[ERROR] rivision-benchmark.riscv64 not found"
        fi
        ;;
    vlm)
        echo "=== VLM Benchmark ==="
        if [ -f "$SCRIPT_DIR/bin/llama-mtmd-cli" ]; then
            MODEL=$(find "$SCRIPT_DIR/models/vlm" -name "*.gguf" | head -1)
            if [ -n "$MODEL" ]; then
                "$SCRIPT_DIR/bin/llama-mtmd-cli" -m "$MODEL" --image "$TEST_IMAGE" \
                    -p "描述图片" -c 2048 -n 256 -t 6 --no-warmup
            else
                echo "[WARN] No VLM model found"
            fi
        fi
        ;;
    all)
        "$0" yolo "$PRESET"
        echo ""
        "$0" vlm "$PRESET"
        ;;
    *)
        echo "Usage: $0 {yolo|vlm|all} [quick|standard]"
        ;;
esac
EOF
    chmod +x "$PACKAGE_DIR/run.sh"
}

# ============================================================
# 创建 README
# ============================================================
create_readme() {
    info "Creating README..."
    
    cat > "$PACKAGE_DIR/README.md" << EOF
# RiVision Benchmark for K3

## 目录结构

\`\`\`
├── bin/                    # 可执行文件
│   ├── rivision-benchmark.riscv64
│   ├── yolo-server.riscv64
│   └── llama-mtmd-cli
├── lib/                    # 动态库
│   └── libonnxruntime.so.*
├── models/                 # 模型文件
│   ├── yolo/
│   │   ├── yolov8n.onnx
│   │   └── 910byolo11n_b2.q.onnx
│   └── vlm/
├── scripts/                # 脚本
├── config/                 # 配置文件
├── run.sh                  # 快速启动脚本
└── test.jpg                # 测试图片
\`\`\`

## 使用方法

### 快速测试
\`\`\`bash
./run.sh yolo      # YOLO 测试
./run.sh vlm       # VLM 测试
./run.sh all       # 全部测试
\`\`\`

### 完整测试
\`\`\`bash
./scripts/run-k3-benchmark.sh standard
\`\`\`

## 环境要求

- SpacemiT K3 RISC-V 平台
- 内存 >= 8GB (VLM 推荐 16GB)
- OpenCV 4.x (系统已安装)

## 注意事项

1. VLM 模型文件较大，默认不包含。使用以下命令重新打包:
   \`\`\`bash
   INCLUDE_VLM_MODELS=true ./package-k3.sh
   \`\`\`

2. 首次运行可能需要设置库路径:
   \`\`\`bash
   export LD_LIBRARY_PATH=\$PWD/lib:\$LD_LIBRARY_PATH
   \`\`\`

打包日期: $(date '+%Y-%m-%d %H:%M:%S')
EOF
}

# ============================================================
# 打包
# ============================================================
create_tarball() {
    info "Creating tarball..."
    
    mkdir -p "$BENCHMARK_DIR/dist"
    
    cd "$(dirname "$PACKAGE_DIR")"
    tar -czf "$OUTPUT_FILE" "$(basename "$PACKAGE_DIR")"
    
    info "Package created: $OUTPUT_FILE"
    info "Size: $(du -h "$OUTPUT_FILE" | cut -f1)"
}

# ============================================================
# 清理
# ============================================================
cleanup() {
    rm -rf "$PACKAGE_DIR"
}

# ============================================================
# 主流程
# ============================================================
main() {
    info "Packaging RiVision Benchmark for K3..."
    info "Output: $OUTPUT_NAME"
    
    check_build
    create_package_structure
    copy_binaries
    copy_libraries
    copy_models
    copy_scripts
    create_run_script
    create_readme
    create_tarball
    cleanup
    
    echo ""
    info "=== Package Complete ==="
    echo "  File: $OUTPUT_FILE"
    echo "  Size: $(du -h "$OUTPUT_FILE" | cut -f1)"
    echo ""
    echo "Deploy to K3:"
    echo "  scp $OUTPUT_FILE user@k3-host:~/"
    echo "  ssh user@k3-host 'tar -xzf $(basename $OUTPUT_FILE) && cd $OUTPUT_NAME && ./run.sh'"
}

main "$@"
