#!/bin/bash
# RiVision Benchmark - K3 RISC-V 专项测试脚本
# 针对 SpacemiT K3 平台的完整 YOLO/VLM 性能测试
#
# 使用方式:
#   ./run-k3-benchmark.sh [preset] [output_dir]
#   preset: quick | standard | comprehensive
#
# 要求:
#   - 在 K3 (riscv64) 上运行
#   - yolo-server 已启动 (HTTP 测试)
#   - llama-server 已启动 (HTTP VLM 测试)
#   - 模型文件已部署

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PROJECT_ROOT="$(cd "$BENCHMARK_DIR/../../../.." && pwd)"
RIVISION_NODE="$PROJECT_ROOT/src/rivision_node"

# 参数
PRESET="${1:-standard}"
OUTPUT_DIR="${2:-$BENCHMARK_DIR/results/k3_$(date +%Y%m%d_%H%M%S)}"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
section() { echo -e "\n${BLUE}========== $1 ==========${NC}\n"; }

# ============================================================
# 平台检查
# ============================================================
check_platform() {
    local arch=$(uname -m)
    if [ "$arch" != "riscv64" ]; then
        warn "Not running on RISC-V ($arch). Some tests may fail."
    else
        info "Platform: K3 RISC-V 64-bit"
    fi
    
    # 检查内存
    local mem_gb=$(free -g | awk '/^Mem:/{print $2}')
    info "Memory: ${mem_gb}GB"
    
    # 检查 NPU
    if [ -e "/dev/spacemit_npu" ] || [ -d "/sys/class/misc/spacemit-npu" ]; then
        info "SpacemiT NPU: Available"
    else
        warn "SpacemiT NPU: Not detected"
    fi
}

# ============================================================
# 服务检查
# ============================================================
check_services() {
    local yolo_url="${YOLO_URL:-http://localhost:8080}"
    local llama_url="${LLAMA_URL:-http://localhost:8081}"
    
    info "Checking yolo-server at $yolo_url..."
    if curl -s --connect-timeout 3 "$yolo_url/health" > /dev/null 2>&1; then
        info "yolo-server: Running"
        YOLO_HTTP_AVAILABLE=true
    else
        warn "yolo-server: Not available (HTTP tests will be skipped)"
        YOLO_HTTP_AVAILABLE=false
    fi
    
    info "Checking llama-server at $llama_url..."
    if curl -s --connect-timeout 3 "$llama_url/health" > /dev/null 2>&1; then
        info "llama-server: Running"
        VLM_HTTP_AVAILABLE=true
    else
        warn "llama-server: Not available (HTTP VLM tests will be skipped)"
        VLM_HTTP_AVAILABLE=false
    fi
}

# ============================================================
# 模型检查
# ============================================================
check_models() {
    local models_dir="$RIVISION_NODE/models"
    
    info "Checking model files..."
    
    # YOLO 模型
    YOLO_MODELS=()
    for model in yolov8n.onnx 910byolo11n_b2.q.onnx; do
        if [ -f "$models_dir/shared/$model" ]; then
            YOLO_MODELS+=("$models_dir/shared/$model")
            info "  Found: $model"
        fi
    done
    
    if [ ${#YOLO_MODELS[@]} -eq 0 ]; then
        warn "No YOLO models found in $models_dir/shared"
    fi
    
    # VLM 模型
    VLM_MODELS=()
    
    # Qwen3VL
    if [ -f "$models_dir/shared/Qwen3VL/Qwen3VL-4B-Instruct-Q4_K_M.gguf" ]; then
        VLM_MODELS+=("qwen3vl-4b")
        info "  Found: Qwen3VL-4B"
    fi
    
    # FastVLM
    if [ -d "$models_dir/riscv_b/fastvlm-mm-0.5b-q4_1" ]; then
        VLM_MODELS+=("fastvlm-0.5b")
        info "  Found: FastVLM-0.5B"
    fi
    
    # MiniCPM
    if [ -f "$models_dir/shared/miniCPM_v4.5/MiniCPM-V-4_5-Q4_K_M.gguf" ]; then
        VLM_MODELS+=("minicpm-v4.5")
        info "  Found: MiniCPM-V-4.5"
    fi
    
    if [ ${#VLM_MODELS[@]} -eq 0 ]; then
        warn "No VLM models found"
    fi
}

# ============================================================
# 测试图片
# ============================================================
find_test_image() {
    for img in \
        "$PROJECT_ROOT/test.jpg" \
        "$RIVISION_NODE/models/test.jpg" \
        "$BENCHMARK_DIR/test.jpg" \
        "/tmp/test.jpg"; do
        if [ -f "$img" ]; then
            TEST_IMAGE="$img"
            info "Test image: $TEST_IMAGE"
            return 0
        fi
    done
    
    # 创建测试图片
    warn "No test image found, creating placeholder..."
    TEST_IMAGE="/tmp/benchmark_test.jpg"
    convert -size 640x640 xc:gray "$TEST_IMAGE" 2>/dev/null || \
    python3 -c "import cv2; import numpy as np; cv2.imwrite('$TEST_IMAGE', np.random.randint(0,255,(640,640,3),dtype=np.uint8))" 2>/dev/null || \
    error "Cannot create test image"
}

# ============================================================
# YOLO 本地测试
# ============================================================
run_yolo_local_tests() {
    section "YOLO Local Benchmark (ORT NPU)"
    
    if [ ${#YOLO_MODELS[@]} -eq 0 ]; then
        warn "Skipping: No YOLO models"
        return
    fi
    
    local runs=100
    local warmup=10
    
    case "$PRESET" in
        quick) runs=50; warmup=5 ;;
        comprehensive) runs=200; warmup=20 ;;
    esac
    
    for model in "${YOLO_MODELS[@]}"; do
        local model_name=$(basename "$model" .onnx)
        info "Testing: $model_name"
        
        local out="$OUTPUT_DIR/yolo_local_${model_name}.json"
        
        # 使用现有的 benchmark 脚本或二进制
        if [ -x "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" ]; then
            "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" yolo-local \
                --model "$model" \
                --image "$TEST_IMAGE" \
                --runs "$runs" \
                --warmup "$warmup" \
                --output "$out" 2>&1 | tee -a "$OUTPUT_DIR/yolo_local.log"
        else
            # 使用 shell 脚本备选
            "$RIVISION_NODE/scripts/benchmark_yolo_max.sh" "$TEST_IMAGE" "$runs" 2>&1 | \
                tee -a "$OUTPUT_DIR/yolo_local.log"
        fi
    done
}

# ============================================================
# YOLO HTTP 测试
# ============================================================
run_yolo_http_tests() {
    section "YOLO HTTP Benchmark"
    
    if [ "$YOLO_HTTP_AVAILABLE" != "true" ]; then
        warn "Skipping: yolo-server not available"
        return
    fi
    
    local yolo_url="${YOLO_URL:-http://localhost:8080}"
    local runs=100
    local concurrency="1,2,4,8"
    
    case "$PRESET" in
        quick) runs=50; concurrency="1,2,4" ;;
        comprehensive) runs=200; concurrency="1,2,4,8,16" ;;
    esac
    
    info "URL: $yolo_url"
    info "Runs: $runs, Concurrency: $concurrency"
    
    local out="$OUTPUT_DIR/yolo_http.json"
    
    if [ -x "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" ]; then
        "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" yolo-http \
            --url "$yolo_url/api/detect/binary" \
            --image "$TEST_IMAGE" \
            --runs "$runs" \
            --concurrency "$concurrency" \
            --output "$out" 2>&1 | tee -a "$OUTPUT_DIR/yolo_http.log"
    else
        # 备选: 使用 curl 进行简单测试
        info "Using fallback curl-based test..."
        for c in $(echo "$concurrency" | tr ',' ' '); do
            info "  Concurrency: $c"
            for i in $(seq 1 $runs); do
                curl -s -X POST "$yolo_url/api/detect/binary" \
                    -H "Content-Type: application/octet-stream" \
                    --data-binary "@$TEST_IMAGE" > /dev/null &
                [ $((i % c)) -eq 0 ] && wait
            done
            wait
        done
    fi
}

# ============================================================
# VLM 本地测试
# ============================================================
run_vlm_local_tests() {
    section "VLM Local Benchmark (llama.cpp smt)"
    
    if [ ${#VLM_MODELS[@]} -eq 0 ]; then
        warn "Skipping: No VLM models"
        return
    fi
    
    local llama_bin="$RIVISION_NODE/third_party/llama.cpp/riscv64/build/bin/llama-mtmd-cli"
    if [ ! -x "$llama_bin" ]; then
        llama_bin=$(which llama-mtmd-cli 2>/dev/null || echo "")
    fi
    
    if [ -z "$llama_bin" ]; then
        warn "llama-mtmd-cli not found, using benchmark script"
        llama_bin=""
    fi
    
    local runs=10
    local threads=6
    
    case "$PRESET" in
        quick) runs=5 ;;
        comprehensive) runs=20 ;;
    esac
    
    for vlm in "${VLM_MODELS[@]}"; do
        info "Testing VLM: $vlm"
        
        local out="$OUTPUT_DIR/vlm_local_${vlm}.log"
        
        # 使用现有的 VLM benchmark 脚本
        MODEL="$vlm" RUNS="$runs" THREADS="$threads" \
            "$RIVISION_NODE/scripts/benchmark_vlm.sh" "$TEST_IMAGE" 2>&1 | \
            tee "$out"
    done
}

# ============================================================
# VLM HTTP 测试
# ============================================================
run_vlm_http_tests() {
    section "VLM HTTP Benchmark"
    
    if [ "$VLM_HTTP_AVAILABLE" != "true" ]; then
        warn "Skipping: llama-server not available"
        return
    fi
    
    local llama_url="${LLAMA_URL:-http://localhost:8081}"
    local runs=10
    
    case "$PRESET" in
        quick) runs=5 ;;
        comprehensive) runs=20 ;;
    esac
    
    info "URL: $llama_url"
    info "Runs: $runs"
    
    local out="$OUTPUT_DIR/vlm_http.json"
    
    if [ -x "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" ]; then
        "$BENCHMARK_DIR/bin/rivision-benchmark.riscv64" vlm-http \
            --url "$llama_url/v1/chat/completions" \
            --image "$TEST_IMAGE" \
            --prompt "简要描述图片内容" \
            --runs "$runs" \
            --output "$out" 2>&1 | tee -a "$OUTPUT_DIR/vlm_http.log"
    fi
}

# ============================================================
# 生成报告
# ============================================================
generate_report() {
    section "Generating Report"
    
    local report="$OUTPUT_DIR/report.md"
    
    cat > "$report" << EOF
# RiVision Benchmark Report - K3 RISC-V

**Date:** $(date '+%Y-%m-%d %H:%M:%S')
**Platform:** $(uname -m) / $(uname -r)
**Preset:** $PRESET

## System Info

- **CPU:** $(cat /proc/cpuinfo | grep 'model name' | head -1 | cut -d: -f2 | xargs)
- **Cores:** $(nproc)
- **Memory:** $(free -h | awk '/^Mem:/{print $2}')
- **NPU:** $([ -e "/dev/spacemit_npu" ] && echo "SpacemiT NPU Available" || echo "Not detected")

## Test Configuration

| Test | Status |
|------|--------|
| YOLO Local | $([ ${#YOLO_MODELS[@]} -gt 0 ] && echo "✅" || echo "⏭️ Skipped") |
| YOLO HTTP | $([ "$YOLO_HTTP_AVAILABLE" = "true" ] && echo "✅" || echo "⏭️ Skipped") |
| VLM Local | $([ ${#VLM_MODELS[@]} -gt 0 ] && echo "✅" || echo "⏭️ Skipped") |
| VLM HTTP | $([ "$VLM_HTTP_AVAILABLE" = "true" ] && echo "✅" || echo "⏭️ Skipped") |

## Results

EOF

    # 添加测试结果摘要
    for log in "$OUTPUT_DIR"/*.log; do
        if [ -f "$log" ]; then
            echo "### $(basename "$log" .log)" >> "$report"
            echo '```' >> "$report"
            tail -20 "$log" >> "$report"
            echo '```' >> "$report"
            echo "" >> "$report"
        fi
    done
    
    info "Report saved: $report"
}

# ============================================================
# 主流程
# ============================================================
main() {
    section "RiVision K3 Benchmark"
    
    info "Preset: $PRESET"
    info "Output: $OUTPUT_DIR"
    
    mkdir -p "$OUTPUT_DIR"
    
    # 检查
    check_platform
    check_services
    check_models
    find_test_image
    
    # 运行测试
    run_yolo_local_tests
    run_yolo_http_tests
    run_vlm_local_tests
    run_vlm_http_tests
    
    # 报告
    generate_report
    
    section "Complete"
    info "Results saved to: $OUTPUT_DIR"
    ls -la "$OUTPUT_DIR"
}

main "$@"
