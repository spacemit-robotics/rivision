#!/bin/bash
# ============================================================
# RiVision Benchmark 多模型测试脚本 (K3 RISC-V)
# 支持 YAML 配置驱动 + 命令行参数
# ============================================================

# 注意: 不使用 set -e，因为需要手动处理错误

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 自动设置环境
export LD_LIBRARY_PATH="$SCRIPT_DIR/lib:$LD_LIBRARY_PATH"

# ============================================================
# NPU 配置 (K3 A100 智算核)
# ============================================================
# K3 有 8 个 A100 核心，SpacemiT EP/llama.cpp 自动调度到 A100
#
# ★★★ 4 核最优配置 (最高吞吐 + 最高 NPU 利用率) ★★★
#   YOLO_THREADS=4 YOLO_DUAL_SESSION=1 CPU_THREADS=4
#   实测: 47.8 FPS, NPU利用率 95%
#
# 配置方式 (优先级从高到低):
#   1. 命令行: YOLO_THREADS=4 ./run-benchmark.sh yolo
#   2. 环境变量: export YOLO_THREADS=4 或 export VLM_THREADS=8
#   3. 配置文件: config/benchmark.yaml → npu.yolo_threads / npu.vlm_threads
#   4. 默认值: YOLO=4, VLM=8, DUAL_SESSION=1

# YOLO 线程数 (ORT SpacemiT EP)
if [ -z "$YOLO_THREADS" ]; then
    if command -v yq &> /dev/null && [ -f "$SCRIPT_DIR/config/benchmark.yaml" ]; then
        YOLO_THREADS=$(yq '.npu.yolo_threads // 4' "$SCRIPT_DIR/config/benchmark.yaml" 2>/dev/null || echo 4)
    else
        YOLO_THREADS=4  # ★ 当前可用 4 核
    fi
fi
export YOLO_THREADS

# VLM 线程数 (llama.cpp)
if [ -z "$VLM_THREADS" ]; then
    if command -v yq &> /dev/null && [ -f "$SCRIPT_DIR/config/benchmark.yaml" ]; then
        VLM_THREADS=$(yq '.npu.vlm_threads // 8' "$SCRIPT_DIR/config/benchmark.yaml" 2>/dev/null || echo 8)
    else
        VLM_THREADS=8  # llama.cpp 支持全部 8 核
    fi
fi
export VLM_THREADS

# CPU 线程数 (预处理/后处理)
# ★ 实测: 4 线程足够 (NPU 是瓶颈)
if [ -z "$CPU_THREADS" ]; then
    if command -v yq &> /dev/null && [ -f "$SCRIPT_DIR/config/benchmark.yaml" ]; then
        CPU_THREADS=$(yq '.preproc.cpu_threads // 4' "$SCRIPT_DIR/config/benchmark.yaml" 2>/dev/null || echo 4)
    else
        CPU_THREADS=4  # ★ 默认 4 线程 (NPU 是瓶颈，4 核足够)
    fi
fi
export CPU_THREADS

# 双 Session 模式 (0=最低延时, 1=高吞吐)
# ★ 默认启用双 Session (实测 FPS 提升 15%)
if [ -z "$YOLO_DUAL_SESSION" ]; then
    if command -v yq &> /dev/null && [ -f "$SCRIPT_DIR/config/benchmark.yaml" ]; then
        DUAL_SESSION_VAL=$(yq '.npu.dual_session // true' "$SCRIPT_DIR/config/benchmark.yaml" 2>/dev/null || echo "true")
        if [ "$DUAL_SESSION_VAL" = "false" ]; then
            YOLO_DUAL_SESSION=0
        else
            YOLO_DUAL_SESSION=1
        fi
    else
        YOLO_DUAL_SESSION=1  # ★ 默认双 Session (最优配置)
    fi
fi
export YOLO_DUAL_SESSION

# FFmpeg 多实例 (并行解码)
# ★ 实测: 4 实例可消除解码瓶颈
if [ -z "$FFMPEG_MULTI_INSTANCE" ]; then
    FFMPEG_MULTI_INSTANCE=4  # ★ 默认 4 实例 (匹配预处理线程数)
fi
export FFMPEG_MULTI_INSTANCE

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# 默认配置
CONFIG_FILE="$SCRIPT_DIR/config/benchmark.yaml"
PROFILE="standard"
OUTPUT_DIR="$SCRIPT_DIR/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="$OUTPUT_DIR/benchmark_report_$TIMESTAMP.txt"

# 从配置读取默认值 (如果 yq 可用)
if command -v yq &> /dev/null && [ -f "$CONFIG_FILE" ]; then
    RUNS=$(yq '.yolo.defaults.runs // 100' "$CONFIG_FILE" 2>/dev/null || echo 100)
    WARMUP=$(yq '.yolo.defaults.warmup // 5' "$CONFIG_FILE" 2>/dev/null || echo 5)
else
    RUNS=100
    WARMUP=5
fi

# 帮助信息
show_help() {
    echo -e "${CYAN}RiVision Benchmark - K3 RISC-V${NC}"
    echo ""
    echo "Usage: $0 [COMMAND] [OPTIONS]"
    echo ""
    echo -e "${GREEN}Commands:${NC}"
    echo "  check            系统环境检查 (平台/NPU/库/模型)"
    echo "  run              运行完整测试 (基于配置文件)"
    echo "  yolo             仅运行 YOLO 测试"
    echo "  vlm              仅运行 VLM 测试"
    echo "  list             列出可用模型和测试数据"
    echo ""
    echo -e "${GREEN}Options:${NC}"
    echo "  -c, --config FILE   配置文件 (默认: config/benchmark.yaml)"
    echo "  -p, --profile NAME  测试配置 (quick/standard/thorough/npu_only/cpu_only)"
    echo "  -r, --runs N        每个模型运行次数"
    echo "  -w, --warmup N      预热次数"
    echo "  -i, --image PATH    测试图片路径"
    echo "  -o, --output DIR    输出目录"
    echo "  --report            测试完成后生成汇总报告 (vlm_benchmark_summary.txt / yolo_benchmark_summary.txt)"
    echo "  -h, --help          显示帮助"
    echo ""
    echo -e "${GREEN}模型指定 (支持多种方式):${NC}"
    echo "  -m, --model PATH    指定单个模型文件或目录"
    echo "  --yolo-model NAME   指定 YOLO 模型 (文件名或路径)"
    echo "  --vlm-model NAME    指定 VLM 模型 (目录名或路径)"
    echo "  --backend TYPE      指定后端 (ort_cpu/ort_npu/llama_cpu)"
    echo ""
    echo -e "${GREEN}Profiles (预设配置):${NC}"
    echo "  quick      快速验证 (YOLO: 10次, VLM: 2次)"
    echo "  standard   标准测试 (YOLO: 100次, VLM: 5次) [默认]"
    echo "  thorough   深度测试 (YOLO: 500次, VLM: 20次)"
    echo "  npu_only   仅测试 NPU 模型 (.q.onnx)"
    echo "  cpu_only   仅测试 CPU 模型"
    echo ""
    echo -e "${GREEN}Examples:${NC}"
    echo "  $0 check                                    # 系统检查"
    echo "  $0 list                                     # 列出模型"
    echo "  $0 run                                      # 运行全部"
    echo "  $0 run -p quick                             # 快速测试"
    echo ""
    echo -e "${GREEN}YOLO 模型指定示例:${NC}"
    echo "  $0 yolo                                     # 测试所有 YOLO"
    echo "  $0 yolo -p npu_only                         # 仅测试 NPU 模型"
    echo "  $0 yolo --yolo-model yolov8n.q.onnx         # 指定单个模型"
    echo "  $0 yolo -m models/yolo/yolo11n.q.onnx       # 完整路径"
    echo ""
    echo -e "${GREEN}VLM 模型指定示例:${NC}"
    echo "  $0 vlm                                      # 测试所有 VLM"
    echo "  $0 vlm --vlm-model fastvlm-0.5b-q8          # 指定模型目录"
    echo "  $0 vlm --vlm-model Qwen3.5-4B               # SMT/NPU 模型"
    echo "  $0 vlm -m models/vlm/miniCPM-V-4_5          # 完整路径"
}

# ============================================================
# 系统检查 (check 命令)
# ============================================================
run_check() {
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║          RiVision Benchmark - 系统检查                   ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    local all_ok=true
    local host_arch=$(uname -m)
    local is_riscv=false
    
    # 检测是否在 RISC-V 平台 (支持多种返回值)
    case "$host_arch" in
        riscv64|riscv|rv64*)
            is_riscv=true
            ;;
    esac
    
    # 也检查 NPU 设备存在性作为辅助判断
    if ls /dev/spacemit* 2>/dev/null | head -1 > /dev/null; then
        is_riscv=true
    fi
    
    # 1. 平台检测
    echo -e "${BLUE}[1/5] 平台架构检测${NC}"
    echo -e "  主机架构: $host_arch"
    echo -e "  目标架构: riscv64"
    
    if [ "$is_riscv" = true ]; then
        echo -e "  ${GREEN}✓${NC} 检测到 RISC-V/K3 平台"
        # 在 RISC-V 上，尝试运行二进制
        if [ -f "bin/rivision-benchmark.riscv64" ]; then
            echo -e "  正在测试二进制..."
            local output
            local ret
            # 使用 --help 测试二进制是否可运行 (不需要额外参数)
            output=$(./bin/rivision-benchmark.riscv64 --help 2>&1) && ret=0 || ret=$?
            # --help 通常返回 0 或 1，只要有输出且不是动态库错误就认为成功
            if echo "$output" | grep -q -i "usage\|option\|command\|benchmark"; then
                echo -e "  ${GREEN}✓ 二进制可正常运行${NC}"
                # 显示版本或帮助信息的前几行
                echo "$output" | head -3 | sed 's/^/    /'
            elif [ $ret -eq 127 ] || echo "$output" | grep -q -i "not found\|cannot\|error loading"; then
                echo -e "  ${RED}✗ 二进制运行失败 - 缺少依赖库${NC}"
                echo "$output" | head -3 | sed 's/^/    /'
                all_ok=false
            else
                echo -e "  ${GREEN}✓ 二进制可执行${NC}"
            fi
        else
            echo -e "  ${RED}✗ 二进制文件不存在${NC}"
            all_ok=false
        fi
    else
        # 在非 RISC-V 上，只检查文件
        if [ -f "bin/rivision-benchmark.riscv64" ]; then
            local bin_info=$(file bin/rivision-benchmark.riscv64 2>/dev/null | grep -o "RISC-V")
            if [ -n "$bin_info" ]; then
                echo -e "  ${GREEN}✓ 二进制文件有效 (RISC-V 交叉编译)${NC}"
                echo -e "  ${YELLOW}⚠ 当前在 $host_arch 上，需部署到 K3 运行${NC}"
            else
                echo -e "  ${RED}✗ 二进制文件无效${NC}"
                all_ok=false
            fi
        else
            echo -e "  ${RED}✗ 二进制文件不存在${NC}"
            all_ok=false
        fi
    fi
    echo ""
    
    # 2. NPU 检测
    echo -e "${BLUE}[2/5] NPU 设备检测${NC}"
    if ls /dev/spacemit* 2>/dev/null | head -1 > /dev/null; then
        echo -e "  ${GREEN}✓ SpacemiT NPU 设备存在${NC}"
        ls /dev/spacemit* 2>/dev/null | while read dev; do echo "    - $dev"; done
    else
        if [ "$is_riscv" = true ]; then
            echo -e "  ${YELLOW}⚠ 未检测到 NPU 设备 (将使用 CPU 后端)${NC}"
        else
            echo -e "  ${YELLOW}⚠ 非 RISC-V 平台，跳过 NPU 检测${NC}"
        fi
    fi
    # 显示 A100 核心配置
    echo -e "  YOLO (ORT):     ${GREEN}${YOLO_THREADS}${NC} 核 (YOLO_THREADS 或 npu.yolo_threads)"
    echo -e "  VLM (llama):    ${GREEN}${VLM_THREADS}${NC} 核 (VLM_THREADS 或 npu.vlm_threads)"
    echo -e "  说明: SpacemiT EP/llama.cpp 自动调度到 A100，无需手动绑定"
    echo ""
    
    # 3. 依赖库检查
    echo -e "${BLUE}[3/5] 依赖库检查${NC}"
    local libs_ok=true
    for lib in libonnxruntime.so libopencv_core.so libllama.so libcurl.so; do
        if [ -f "lib/$lib" ]; then
            echo -e "  ${GREEN}✓${NC} $lib"
        else
            echo -e "  ${RED}✗${NC} $lib (缺失)"
            libs_ok=false
        fi
    done
    if [ "$libs_ok" = false ]; then
        all_ok=false
    fi
    echo ""
    
    # 4. 模型文件检查
    echo -e "${BLUE}[4/5] 模型文件检查${NC}"
    echo -e "  ${CYAN}YOLO 模型 (models/yolo/):${NC}"
    local yolo_count=0
    for model in models/yolo/*.onnx; do
        if [ -f "$model" ]; then
            model_name=$(basename "$model")
            size=$(du -h "$model" | cut -f1)
            # 检测后端类型: .q.onnx 或 910b 为 NPU
            if [[ "$model_name" == *".q."* ]] || [[ "$model_name" == *"910b"* ]]; then
                backend="NPU"
            else
                backend="CPU"
            fi
            echo -e "    ${GREEN}✓${NC} $model_name ($size) [$backend]"
            yolo_count=$((yolo_count + 1))
        fi
    done
    [ $yolo_count -eq 0 ] && echo -e "    ${YELLOW}⚠ 无 YOLO 模型${NC}"
    echo -e "    总计: $yolo_count 个模型"
    
    echo -e "  ${CYAN}VLM 模型 (models/vlm/):${NC}"
    local vlm_count=0
    # 检测子目录结构的 VLM 模型
    for dir in models/vlm/*/; do
        if [ -d "$dir" ]; then
            dir_name=$(basename "$dir")
            # 跳过非模型文件
            [[ "$dir_name" == *.txt ]] && continue
            
            # 检测模型类型和完整性
            local model_valid=false
            local mode=""
            local issues=""
            
            if [ -f "${dir}config.json" ]; then
                # SMT/NPU 模式检测
                mode="SMT/NPU"
                local has_gguf=$(ls "${dir}"*.gguf 2>/dev/null | head -1)
                local has_onnx=$(ls "${dir}"*.onnx 2>/dev/null | head -1)
                if [ -n "$has_gguf" ] && [ -n "$has_onnx" ]; then
                    model_valid=true
                else
                    issues="缺少 .gguf 或 .onnx"
                fi
            elif ls "${dir}"*mmproj*.gguf 2>/dev/null | head -1 > /dev/null; then
                # mmproj 模式检测
                mode="mmproj"
                local has_text_gguf=$(ls "${dir}"*.gguf 2>/dev/null | grep -v mmproj | head -1)
                local has_mmproj=$(ls "${dir}"*mmproj*.gguf 2>/dev/null | head -1)
                if [ -n "$has_text_gguf" ] && [ -n "$has_mmproj" ]; then
                    model_valid=true
                else
                    issues="缺少 text 或 mmproj 文件"
                fi
            else
                mode="unknown"
                issues="无法识别模型结构"
            fi
            
            if [ "$model_valid" = true ]; then
                echo -e "    ${GREEN}✓${NC} $dir_name [$mode]"
                vlm_count=$((vlm_count + 1))
            else
                echo -e "    ${YELLOW}⚠${NC} $dir_name [$mode] - $issues"
            fi
        fi
    done
    # 检查根目录的独立 gguf 文件
    for model in models/vlm/*.gguf; do
        if [ -f "$model" ]; then
            model_name=$(basename "$model")
            size=$(du -h "$model" | cut -f1)
            echo -e "    ${GREEN}✓${NC} $model_name ($size) [standalone]"
            vlm_count=$((vlm_count + 1))
        fi
    done
    [ $vlm_count -eq 0 ] && echo -e "    ${YELLOW}⚠ 无 VLM 模型${NC}"
    echo -e "    总计: $vlm_count 个模型"
    echo ""
    
    # 5. 测试数据检查
    echo -e "${BLUE}[5/5] 测试数据检查${NC}"
    for file in data/test.jpg data/test.mp4; do
        if [ -f "$file" ]; then
            size=$(du -h "$file" | cut -f1)
            echo -e "  ${GREEN}✓${NC} $file ($size)"
        else
            echo -e "  ${RED}✗${NC} $file (缺失)"
        fi
    done
    echo ""
    
    # 总结
    echo -e "${CYAN}══════════════════════════════════════════════════════════${NC}"
    if [ "$all_ok" = true ]; then
        if [ "$is_riscv" = true ]; then
            echo -e "${GREEN}✓ 系统检查通过，可以运行测试${NC}"
            echo ""
            echo -e "快速开始:"
            echo -e "  ./run-benchmark.sh run -p quick    # 快速测试"
            echo -e "  ./run-benchmark.sh run             # 标准测试"
        else
            echo -e "${GREEN}✓ 打包检查通过，可以部署到 K3${NC}"
            echo ""
            echo -e "部署命令:"
            echo -e "  scp ../rivision-benchmark-k3.tar.gz user@k3-host:~/"
            echo -e "  ssh user@k3-host 'tar -xzf rivision-benchmark-k3.tar.gz'"
            echo -e "  ssh user@k3-host 'cd rivision-benchmark-k3-full && ./run-benchmark.sh check'"
        fi
    else
        echo -e "${RED}✗ 系统检查发现问题，请先解决${NC}"
    fi
    echo -e "${CYAN}══════════════════════════════════════════════════════════${NC}"
}

# 列出模型和数据
list_all() {
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║          可用模型和测试数据                              ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    echo -e "${BLUE}YOLO 模型 (models/yolo/):${NC}"
    echo -e "  ┌──────────────────────────────────┬────────┬────────┐"
    echo -e "  │ 模型名称                         │ 大小   │ 后端   │"
    echo -e "  ├──────────────────────────────────┼────────┼────────┤"
    for model in models/yolo/*.onnx; do
        if [ -f "$model" ]; then
            name=$(basename "$model")
            size=$(du -h "$model" | cut -f1)
            if [[ "$name" == *"910b"* ]] || [[ "$name" == *".q."* ]]; then
                backend="NPU"
            else
                backend="CPU"
            fi
            printf "  │ %-32s │ %6s │ %-6s │\n" "$name" "$size" "$backend"
        fi
    done
    echo -e "  └──────────────────────────────────┴────────┴────────┘"
    echo ""
    
    echo -e "${BLUE}VLM 模型 (models/vlm/):${NC}"
    local vlm_found=false
    # 检测子目录结构的 VLM 模型
    for dir in models/vlm/*/; do
        if [ -d "$dir" ]; then
            dir_name=$(basename "$dir")
            # 跳过 README 等非模型目录
            [ "$dir_name" = "README.txt" ] && continue
            
            vlm_found=true
            
            # 检测模型类型
            if [ -f "${dir}config.json" ]; then
                # SMT/NPU 模式 (有 config.json)
                mode="SMT/NPU"
                # 检查 onnx 和 gguf 文件
                onnx_count=$(ls "${dir}"*.onnx 2>/dev/null | wc -l)
                gguf_count=$(ls "${dir}"*.gguf 2>/dev/null | wc -l)
                echo -e "  ${GREEN}✓${NC} $dir_name [${CYAN}$mode${NC}]"
                echo -e "      ├─ config.json"
                ls "${dir}"*.gguf 2>/dev/null | while read f; do
                    echo -e "      ├─ $(basename "$f")"
                done
                ls "${dir}"*.onnx 2>/dev/null | while read f; do
                    echo -e "      └─ $(basename "$f")"
                done
            elif ls "${dir}"*mmproj*.gguf 2>/dev/null | head -1 > /dev/null; then
                # mmproj 模式 (有 mmproj 文件)
                mode="mmproj"
                echo -e "  ${GREEN}✓${NC} $dir_name [${CYAN}$mode${NC}]"
                ls "${dir}"*.gguf 2>/dev/null | while read f; do
                    fname=$(basename "$f")
                    if [[ "$fname" == *"mmproj"* ]]; then
                        echo -e "      ├─ $fname (vision)"
                    else
                        echo -e "      └─ $fname (text)"
                    fi
                done
            else
                # 其他类型
                echo -e "  ${YELLOW}?${NC} $dir_name [未知结构]"
            fi
        fi
    done
    # 也检查根目录的 gguf 文件
    for model in models/vlm/*.gguf; do
        if [ -f "$model" ]; then
            vlm_found=true
            name=$(basename "$model")
            size=$(du -h "$model" | cut -f1)
            echo -e "  ${GREEN}✓${NC} $name ($size) [standalone]"
        fi
    done
    if [ "$vlm_found" = false ]; then
        echo -e "  ${YELLOW}(无模型 - 将模型文件夹放入此目录)${NC}"
    fi
    echo ""
    
    echo -e "${BLUE}测试数据 (data/):${NC}"
    for f in data/*; do
        if [ -f "$f" ]; then
            fname=$(basename "$f")
            # 过滤掉 .gitkeep 等隐藏文件
            [[ "$fname" == .* ]] && continue
            size=$(du -h "$f" | cut -f1)
            echo -e "  - $fname ($size)"
        fi
    done
    echo ""
    
    echo -e "${BLUE}配置文件 (config/):${NC}"
    ls config/*.yaml 2>/dev/null | while read f; do echo "  - $(basename $f)"; done
}

# 查找测试图片
find_test_image() {
    if [ -n "$TEST_IMAGE" ] && [ -f "$TEST_IMAGE" ]; then
        echo "$TEST_IMAGE"
        return
    fi
    
    # 查找默认测试图片
    for img in data/test.jpg data/sample.jpg models/test.jpg /tmp/test.jpg; do
        if [ -f "$img" ]; then
            echo "$img"
            return
        fi
    done
    
    echo ""
}

# YOLO 测试
run_yolo_tests() {
    local image=$(find_test_image)

    if [ -z "$image" ]; then
        echo -e "${YELLOW}警告: 未找到测试图片，使用内置测试${NC}"
    fi

    # 收集平台信息 (全局，供 report 使用)
    BENCH_ARCH=$(uname -m)
    BENCH_PLATFORM=""
    case "$BENCH_ARCH" in
        riscv64) BENCH_PLATFORM="K3 (RISC-V rv64gcv)" ;;
        x86_64)  BENCH_PLATFORM="x86_64" ;;
        aarch64) BENCH_PLATFORM="ARM64" ;;
        *)       BENCH_PLATFORM="$BENCH_ARCH" ;;
    esac
    BENCH_CPU_MODEL="unknown"
    if [ -f /proc/cpuinfo ]; then
        BENCH_CPU_MODEL=$(grep -m1 'model name\|isa' /proc/cpuinfo | cut -d: -f2 | xargs 2>/dev/null || echo "unknown")
    fi
    BENCH_NPROC=$(nproc 2>/dev/null || echo "?")
    BENCH_MEM_TOTAL=$(awk '/MemTotal/{printf "%.1f GB", $2/1048576}' /proc/meminfo 2>/dev/null || echo "unknown")

    # 汇总数据数组 (全局，供 generate_yolo_summary_report 使用)
    declare -ga YOLO_NAMES=()
    declare -ga YOLO_BACKENDS=()
    declare -ga YOLO_FPS=()
    declare -ga YOLO_LATENCY=()
    declare -ga YOLO_MEMORY=()
    declare -ga YOLO_CPU=()
    declare -ga YOLO_MODEL_SIZE=()

    echo -e "${BLUE}=== YOLO Benchmark ===${NC}" | tee -a "$REPORT_FILE"
    echo "时间: $(date)" | tee -a "$REPORT_FILE"
    echo "运行次数: $RUNS, 预热: $WARMUP" | tee -a "$REPORT_FILE"
    [ -n "$BACKEND_FILTER" ] && echo "后端过滤: $BACKEND_FILTER" | tee -a "$REPORT_FILE"
    [ -n "$YOLO_MODEL" ] && echo "指定模型: $YOLO_MODEL" | tee -a "$REPORT_FILE"
    [ -n "$SINGLE_MODEL" ] && echo "指定模型: $SINGLE_MODEL" | tee -a "$REPORT_FILE"
    echo "" | tee -a "$REPORT_FILE"
    
    local tested=0
    
    # 确定要测试的模型列表
    local models_to_test=()
    
    if [ -n "$SINGLE_MODEL" ]; then
        # 使用 -m 指定的模型
        if [ -f "$SINGLE_MODEL" ]; then
            models_to_test=("$SINGLE_MODEL")
        elif [ -f "models/yolo/$SINGLE_MODEL" ]; then
            models_to_test=("models/yolo/$SINGLE_MODEL")
        else
            echo -e "${RED}错误: 找不到模型 $SINGLE_MODEL${NC}"
            return 1
        fi
    elif [ -n "$YOLO_MODEL" ]; then
        # 使用 --yolo-model 指定的模型
        if [ -f "$YOLO_MODEL" ]; then
            models_to_test=("$YOLO_MODEL")
        elif [ -f "models/yolo/$YOLO_MODEL" ]; then
            models_to_test=("models/yolo/$YOLO_MODEL")
        else
            echo -e "${RED}错误: 找不到 YOLO 模型 $YOLO_MODEL${NC}"
            return 1
        fi
    else
        # 测试所有模型
        for m in models/yolo/*.onnx; do
            [ -f "$m" ] && models_to_test+=("$m")
        done
    fi
    
    # 遍历模型
    for model in "${models_to_test[@]}"; do
        if [ ! -f "$model" ]; then
            continue
        fi
        
        model_name=$(basename "$model")
        
        # 确定后端类型
        local backend="ort_cpu"
        if [[ "$model_name" == *".q."* ]] || [[ "$model_name" == *"910b"* ]]; then
            backend="ort_npu"
        fi
        
        # 强制指定后端
        [ -n "$FORCE_BACKEND" ] && backend="$FORCE_BACKEND"
        
        # 后端过滤
        if [ -n "$BACKEND_FILTER" ]; then
            if [ "$BACKEND_FILTER" = "npu" ] && [ "$backend" != "ort_npu" ]; then
                continue
            fi
            if [ "$BACKEND_FILTER" = "cpu" ] && [ "$backend" != "ort_cpu" ]; then
                continue
            fi
        fi
        
        echo -e "${GREEN}测试模型: $model_name${NC}" | tee -a "$REPORT_FILE"
        echo "  后端: $backend" | tee -a "$REPORT_FILE"

        # 运行测试
        cmd="./bin/rivision-benchmark.riscv64 yolo-local --model $model --backend $backend --runs $RUNS --warmup $WARMUP"
        if [ -n "$image" ]; then
            cmd="$cmd --image $image"
        fi

        echo "  命令: $cmd" | tee -a "$REPORT_FILE"
        echo "" | tee -a "$REPORT_FILE"

        # ★ 捕获输出并过滤 MPP/FFmpeg 硬件日志
        local OUTPUT
        OUTPUT=$($cmd 2>&1) || true
        echo "$OUTPUT" | grep -v -E '^\[MPP-|^\[mjpeg_stcodec' | tee -a "$REPORT_FILE"

        # ── 提取性能指标 ──────────────────────────────────────
        local yolo_fps=$(echo "$OUTPUT" | grep -oP 'Throughput:\s+\K[\d.]+' || true)
        local yolo_lat=$(echo "$OUTPUT" | grep 'Average:' | grep -oP '[\d.]+' | head -1 || true)
        local yolo_mem=$(echo "$OUTPUT" | grep 'Peak Memory:' | grep -oP '[\d.]+' | head -1 || true)
        local yolo_cpu=$(echo "$OUTPUT" | grep 'Avg CPU:' | grep -oP '[\d.]+' | head -1 || true)
        local yolo_model_sz=$(stat -c%s "$model" 2>/dev/null || echo "0")
        yolo_model_sz=$(echo "scale=1; $yolo_model_sz / 1048576" | bc 2>/dev/null || echo "0")

        if [ -n "$yolo_fps" ]; then
            echo -e "  ${GREEN}✓ 完成${NC}" | tee -a "$REPORT_FILE"
            YOLO_NAMES+=("$model_name")
            YOLO_BACKENDS+=("$backend")
            YOLO_FPS+=("${yolo_fps:-0}")
            YOLO_LATENCY+=("${yolo_lat:-0}")
            YOLO_MEMORY+=("${yolo_mem:-0}")
            YOLO_CPU+=("${yolo_cpu:-0}")
            YOLO_MODEL_SIZE+=("${yolo_model_sz:-0}")
        else
            echo -e "  ${RED}✗ 失败${NC}" | tee -a "$REPORT_FILE"
        fi
        echo "" | tee -a "$REPORT_FILE"
        tested=$((tested + 1))
    done
    
    if [ $tested -eq 0 ]; then
        echo -e "${YELLOW}未找到匹配的 YOLO 模型${NC}" | tee -a "$REPORT_FILE"
    else
        echo -e "${CYAN}共测试 $tested 个 YOLO 模型${NC}" | tee -a "$REPORT_FILE"
    fi
}

# VLM 测试
run_vlm_tests() {
    local image=$(find_test_image)
    
    if [ -z "$image" ]; then
        echo -e "${RED}错误: VLM 测试需要图片，请使用 -i 指定${NC}"
        return 1
    fi
    
    # ── 平台信息 ──────────────────────────────────────────────
    BENCH_ARCH=$(uname -m)
    BENCH_PLATFORM=""
    case "$BENCH_ARCH" in
        riscv64) BENCH_PLATFORM="K3 (RISC-V rv64gcv)" ;;
        x86_64)  BENCH_PLATFORM="x86_64" ;;
        aarch64) BENCH_PLATFORM="ARM64" ;;
        *)       BENCH_PLATFORM="$BENCH_ARCH" ;;
    esac

    BENCH_CPU_MODEL="unknown"
    if [ -f /proc/cpuinfo ]; then
        BENCH_CPU_MODEL=$(grep -m1 'model name\|isa' /proc/cpuinfo | cut -d: -f2 | xargs 2>/dev/null || echo "unknown")
    fi
    BENCH_NPROC=$(nproc 2>/dev/null || echo "?")
    BENCH_MEM_TOTAL=$(awk '/MemTotal/{printf "%.1f GB", $2/1048576}' /proc/meminfo 2>/dev/null || echo "unknown")
    local IMAGE_SIZE=$(stat -c%s "$image" 2>/dev/null || stat -f%z "$image" 2>/dev/null || echo "?")
    
    # ── 报告头 ────────────────────────────────────────────────
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════╗${NC}" | tee -a "$REPORT_FILE"
    echo -e "${CYAN}║          RiVision VLM 推理性能基准测试                   ║${NC}" | tee -a "$REPORT_FILE"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════╝${NC}" | tee -a "$REPORT_FILE"
    echo "" | tee -a "$REPORT_FILE"
    echo "┌─ 平台信息 ──────────────────────────────────────────────" | tee -a "$REPORT_FILE"
    echo "│  架构:     $BENCH_PLATFORM" | tee -a "$REPORT_FILE"
    echo "│  CPU:      $BENCH_CPU_MODEL" | tee -a "$REPORT_FILE"
    echo "│  核心数:   $BENCH_NPROC" | tee -a "$REPORT_FILE"
    echo "│  内存:     $BENCH_MEM_TOTAL" | tee -a "$REPORT_FILE"
    echo "│  VLM线程:  ${VLM_THREADS:-8}" | tee -a "$REPORT_FILE"
    echo "│" | tee -a "$REPORT_FILE"
    echo "├─ 测试参数 ──────────────────────────────────────────────" | tee -a "$REPORT_FILE"
    echo "│  测试图片: $image ($((IMAGE_SIZE / 1024)) KB)" | tee -a "$REPORT_FILE"
    echo "│  时间:     $(date '+%Y-%m-%d %H:%M:%S')" | tee -a "$REPORT_FILE"
    [ -n "$VLM_MODEL" ] && echo "│  指定模型: $VLM_MODEL" | tee -a "$REPORT_FILE"
    [ -n "$SINGLE_MODEL" ] && echo "│  指定模型: $SINGLE_MODEL" | tee -a "$REPORT_FILE"
    echo "└────────────────────────────────────────────────────────" | tee -a "$REPORT_FILE"
    echo "" | tee -a "$REPORT_FILE"
    
    local tested=0
    
    # ── 排除列表 (会 OOM 崩溃的模型) ────────────────────────────
    # qwen30b 已通过 -ctk f16 -ctv f16 -c 4096 控制内存，不再排除
    local VLM_EXCLUDE_LIST=""
    
    is_excluded_model() {
        local model_name="$1"
        # A3B 是 MoE 变体 (仅 3B 活跃参数)，不会 OOM，允许通过
        local name_lower=$(echo "$model_name" | tr '[:upper:]' '[:lower:]')
        if [[ "$name_lower" == *"a3b"* ]]; then
            return 1  # A3B 变体不排除
        fi
        for exclude in $VLM_EXCLUDE_LIST; do
            if [[ "$model_name" == *"$exclude"* ]]; then
                return 0  # 匹配，需要排除
            fi
        done
        return 1  # 不匹配，可以测试
    }
    
    # ── 汇总数据数组 (全局，供 generate_vlm_summary_report 使用) ──
    declare -ga VLM_NAMES=()
    declare -ga VLM_PARAMS=()
    declare -ga VLM_LOAD_MS=()
    declare -ga VLM_VISION_MS=()
    declare -ga VLM_PROMPT_MS=()
    declare -ga VLM_PROMPT_TPS=()
    declare -ga VLM_EVAL_MS=()
    declare -ga VLM_EVAL_TPS=()
    declare -ga VLM_TTFT_MS=()
    declare -ga VLM_TOTAL_MS=()
    declare -ga VLM_TOTAL_TOKENS=()
    
    # 测试单个指定的 VLM 模型
    test_vlm_model() {
        local model_dir="$1"
        local dir_name=$(basename "$model_dir")
        
        echo -e "${GREEN}▶ 测试模型: $dir_name${NC}" | tee -a "$REPORT_FILE"
        
        local mode=""
        local model_file=""
        local mmproj_arg=""
        
        # 检测模型类型
        if [ -f "${model_dir}/config.json" ]; then
            # SMT/NPU 模式
            mode="SMT/NPU"
            model_file=$(ls "${model_dir}"/*.gguf 2>/dev/null | head -1)
            mmproj_arg="--media-backend smt --smt-config-dir ${model_dir}"
        elif ls "${model_dir}"/*mmproj*.gguf 2>/dev/null | head -1 > /dev/null; then
            # mmproj 模式
            mode="mmproj"
            model_file=$(ls "${model_dir}"/*.gguf 2>/dev/null | grep -v mmproj | head -1)
            local mmproj_file=$(ls "${model_dir}"/*mmproj*.gguf 2>/dev/null | head -1)
            mmproj_arg="--mmproj $mmproj_file"
        else
            echo -e "  ${YELLOW}⚠ 无法识别模型结构，跳过${NC}" | tee -a "$REPORT_FILE"
            return 1
        fi
        
        if [ -z "$model_file" ] || [ ! -f "$model_file" ]; then
            echo -e "  ${RED}✗ 模型文件不存在${NC}" | tee -a "$REPORT_FILE"
            return 1
        fi
        
        echo "  模式: $mode" | tee -a "$REPORT_FILE"
        echo "  模型: $(basename "$model_file")" | tee -a "$REPORT_FILE"
        
        # 使用 llama-mtmd-cli 或 rivision-benchmark
        # 注意: K3 有 16 核心但只有 8 个高性能核心，必须限制线程数
        local cmd
        # Qwen3-VL 30B-A3B 需要 f16 KV cache 才能正确推理
        # 匹配: qwen30b_multimodal_files, qwen3vl-30b-text-q4_1.gguf 等
        local kv_cache_args=""
        local dir_lower=$(echo "$model_dir" | tr '[:upper:]' '[:lower:]')
        local file_lower=$(echo "$(basename "$model_file")" | tr '[:upper:]' '[:lower:]')
        if [[ "$dir_lower" == *"qwen"*"30b"* ]] || [[ "$file_lower" == *"qwen"*"30b"* ]]; then
            # -ctk/-ctv f16: 该模型需要 f16 KV cache 才能正确推理
            # -c 4096: 限制上下文长度，避免默认 262144 导致 KV cache 24GB+ OOM
            kv_cache_args="-ctk f16 -ctv f16 -c 4096"
        fi
        if [ -f "bin/llama-mtmd-cli" ]; then
            cmd="./bin/llama-mtmd-cli -t ${VLM_THREADS:-8} -m $model_file $mmproj_arg --image $image -p '描述这张图片' --no-warmup ${kv_cache_args}"
        else
            cmd="./bin/rivision-benchmark.riscv64 vlm-local --model $model_file --image $image --prompt '描述这张图片' --runs $RUNS"
        fi
        
        echo "  命令: $cmd" | tee -a "$REPORT_FILE"
        echo "" | tee -a "$REPORT_FILE"
        
        # ★ 执行推理并捕获输出
        local OUTPUT
        OUTPUT=$(eval $cmd 2>&1) || true
        
        # ★ 过滤并输出日志 (排除 MPP/FFmpeg 硬件日志)
        echo "$OUTPUT" | grep -v -E '^\[MPP-|^\[mjpeg_stcodec' | tee -a "$REPORT_FILE"
        
        # ── 提取性能指标 ──────────────────────────────────────
        local load_ms=$(echo "$OUTPUT" | grep "llama_model_load_from_file_impl" | tail -1 | grep -oP '\d+\.\d+(?= seconds)' | head -1)
        [ -n "$load_ms" ] && load_ms=$(echo "$load_ms * 1000" | bc 2>/dev/null | cut -d. -f1)
        
        # Vision 编码时间 (ms) - 累加所有 image slice 编码时间
        local vision_ms=$(echo "$OUTPUT" | grep -oP 'image slice encoded in \K\d+' | paste -sd+ | bc 2>/dev/null || echo "0")
        local vision_slices=$(echo "$OUTPUT" | grep -c 'image slice encoded in' || echo "0")
        
        local prompt_tokens=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '/\s*\K\d+' | head -1)
        local prompt_ms=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
        local prompt_tps=$(echo "$OUTPUT" | grep "prompt eval time" | grep -oP '\d+\.\d+(?= tokens per second)')
        
        local eval_tokens=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '/\s*\K\d+' | head -1)
        local eval_ms=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
        local eval_tps=$(echo "$OUTPUT" | grep "eval time" | grep -v "prompt" | grep -oP '\d+\.\d+(?= tokens per second)')
        
        local total_ms=$(echo "$OUTPUT" | grep "total time" | grep -oP '=\s*\K\d+\.\d+(?= ms)')
        local total_tokens=$(echo "$OUTPUT" | grep "total time" | grep -oP '/\s*\K\d+' | head -1)
        
        local model_params=$(echo "$OUTPUT" | grep "model params" | grep -oP '\d+\.\d+ [MBG]' | head -1)
        
        # 计算首 token 延迟 (TTFT) = vision + prompt_eval
        local ttft_ms="N/A"
        if [ -n "$vision_ms" ] && [ "$vision_ms" != "0" ] && [ -n "$prompt_ms" ]; then
            ttft_ms=$(echo "$vision_ms + $prompt_ms" | bc 2>/dev/null || echo "N/A")
        elif [ -n "$prompt_ms" ]; then
            ttft_ms="$prompt_ms"
        fi
        
        # ── 输出性能摘要 ──────────────────────────────────────
        echo "" | tee -a "$REPORT_FILE"
        echo "  ┌─────────────────────────────────────────────────" | tee -a "$REPORT_FILE"
        echo "  │ 模型参数:      ${model_params:-N/A}" | tee -a "$REPORT_FILE"
        echo "  │ 加载时间:      ${load_ms:-N/A} ms" | tee -a "$REPORT_FILE"
        [ "$vision_ms" != "0" ] && echo "  │ Vision 编码:   ${vision_ms} ms (${vision_slices} slices)" | tee -a "$REPORT_FILE"
        echo "  │ Prompt eval:   ${prompt_ms:-N/A} ms (${prompt_tokens:-?} tokens, ${prompt_tps:-N/A} tok/s)" | tee -a "$REPORT_FILE"
        echo "  │ Token 生成:    ${eval_ms:-N/A} ms (${eval_tokens:-?} tokens, ${eval_tps:-N/A} tok/s)" | tee -a "$REPORT_FILE"
        echo "  │ ─────────────────────────────────────" | tee -a "$REPORT_FILE"
        echo "  │ 首token延迟:   ${ttft_ms} ms" | tee -a "$REPORT_FILE"
        echo "  │ 总时间:        ${total_ms:-N/A} ms (${total_tokens:-?} tokens)" | tee -a "$REPORT_FILE"
        echo "  └─────────────────────────────────────────────────" | tee -a "$REPORT_FILE"
        
        # ── 保存单模型报告文件 (机器可读格式) ──────────────
        local model_report="$OUTPUT_DIR/benchmark_${dir_name}.txt"
        cat > "$model_report" << EOF
# VLM Benchmark Result
# $(date '+%Y-%m-%d %H:%M:%S')
platform=$BENCH_PLATFORM
cpu=$BENCH_CPU_MODEL
cores=$BENCH_NPROC
memory=$BENCH_MEM_TOTAL
threads=${VLM_THREADS:-8}
model=$dir_name
image=$(basename "$image")
runs=1
avg_vision_ms=${vision_ms:-0} #Vision 编码 单位=ms
avg_prompt_ms=${prompt_ms:-0} #Prompt eval 单位=ms
avg_prompt_tps=${prompt_tps:-0} #Prompt eval 单位=tok/s
avg_eval_ms=${eval_ms:-0} #Token 生成  单位=ms
avg_eval_tps=${eval_tps:-0} #Token 生成  单位=tok/s
avg_ttft_ms=${ttft_ms:-0} #首token延迟(TTFT) 单位=ms
avg_total_ms=${total_ms:-0} #总处理时间  单位=ms
EOF
        echo "  📊 报告已保存: $model_report" | tee -a "$REPORT_FILE"
        
        # ── 记录到汇总数组 ────────────────────────────────────
        VLM_NAMES+=("$dir_name")
        VLM_PARAMS+=("${model_params:-N/A}")
        VLM_LOAD_MS+=("${load_ms:-0}")
        VLM_VISION_MS+=("${vision_ms:-0}")
        VLM_PROMPT_MS+=("${prompt_ms:-0}")
        VLM_PROMPT_TPS+=("${prompt_tps:-0}")
        VLM_EVAL_MS+=("${eval_ms:-0}")
        VLM_EVAL_TPS+=("${eval_tps:-0}")
        VLM_TTFT_MS+=("${ttft_ms:-0}")
        VLM_TOTAL_MS+=("${total_ms:-0}")
        VLM_TOTAL_TOKENS+=("${total_tokens:-0}")
        
        if [ -n "$total_ms" ]; then
            echo -e "  ${GREEN}✓ 完成${NC}" | tee -a "$REPORT_FILE"
            return 0
        else
            echo -e "  ${RED}✗ 失败${NC}" | tee -a "$REPORT_FILE"
            return 1
        fi
    }
    
    # 确定要测试的模型
    if [ -n "$SINGLE_MODEL" ] || [ -n "$VLM_MODEL" ]; then
        local target_model="${SINGLE_MODEL:-$VLM_MODEL}"
        
        # 查找模型目录
        if [ -d "$target_model" ]; then
            test_vlm_model "$target_model" && tested=$((tested + 1))
        elif [ -d "models/vlm/$target_model" ]; then
            test_vlm_model "models/vlm/$target_model" && tested=$((tested + 1))
        elif [ -f "$target_model" ]; then
            # 直接指定 gguf 文件
            echo -e "${GREEN}测试模型: $(basename "$target_model")${NC}" | tee -a "$REPORT_FILE"
            if [ -f "bin/llama-mtmd-cli" ]; then
                cmd="./bin/llama-mtmd-cli -t ${VLM_THREADS:-8} -m $target_model --image $image -p '描述这张图片' --no-warmup"
            else
                cmd="./bin/rivision-benchmark.riscv64 vlm-local --model $target_model --image $image --prompt '描述这张图片' --runs $RUNS"
            fi
            echo "  命令: $cmd" | tee -a "$REPORT_FILE"
            echo "" | tee -a "$REPORT_FILE"
            # ★ 过滤 MPP/FFmpeg 硬件日志
            if eval $cmd 2>&1 | grep -v -E '^\[MPP-|^\[mjpeg_stcodec' | tee -a "$REPORT_FILE"; then
                echo -e "  ${GREEN}✓ 完成${NC}" | tee -a "$REPORT_FILE"
                tested=$((tested + 1))
            else
                echo -e "  ${RED}✗ 失败${NC}" | tee -a "$REPORT_FILE"
            fi
        else
            echo -e "${RED}错误: 找不到 VLM 模型 $target_model${NC}"
            return 1
        fi
    else
        # 测试所有 VLM 模型子目录
        for dir in models/vlm/*/; do
            if [ -d "$dir" ]; then
                dir_name=$(basename "$dir")
                [[ "$dir_name" == *.txt ]] && continue
                # 检查是否在排除列表
                if is_excluded_model "$dir_name"; then
                    echo -e "${YELLOW}⏭ 跳过模型: $dir_name (在排除列表中，会导致 OOM)${NC}" | tee -a "$REPORT_FILE"
                    echo "" | tee -a "$REPORT_FILE"
                    continue
                fi
                test_vlm_model "$dir" && tested=$((tested + 1))
                echo "" | tee -a "$REPORT_FILE"
            fi
        done
        
        # 也测试根目录的独立 gguf 文件
        for model in models/vlm/*.gguf; do
            if [ -f "$model" ]; then
                model_name=$(basename "$model")
                # 检查是否在排除列表
                if is_excluded_model "$model_name"; then
                    echo -e "${YELLOW}⏭ 跳过模型: $model_name (在排除列表中，会导致 OOM)${NC}" | tee -a "$REPORT_FILE"
                    echo "" | tee -a "$REPORT_FILE"
                    continue
                fi
                echo -e "${GREEN}测试模型: $model_name [standalone]${NC}" | tee -a "$REPORT_FILE"
                # Qwen3-VL 30B-A3B 需要 f16 KV cache
                local standalone_kv_args=""
                local name_lower=$(echo "$model_name" | tr '[:upper:]' '[:lower:]')
                if [[ "$name_lower" == *"qwen"*"30b"* ]]; then
                    standalone_kv_args="-ctk f16 -ctv f16 -c 4096"
                fi
                if [ -f "bin/llama-mtmd-cli" ]; then
                    cmd="./bin/llama-mtmd-cli -t ${VLM_THREADS:-8} -m $model --image $image -p '描述这张图片' --no-warmup ${standalone_kv_args}"
                else
                    cmd="./bin/rivision-benchmark.riscv64 vlm-local --model $model --image $image --prompt '描述这张图片' --runs $RUNS"
                fi
                echo "  命令: $cmd" | tee -a "$REPORT_FILE"
                echo "" | tee -a "$REPORT_FILE"
                # ★ 过滤 MPP/FFmpeg 硬件日志
                if eval $cmd 2>&1 | grep -v -E '^\[MPP-|^\[mjpeg_stcodec' | tee -a "$REPORT_FILE"; then
                    echo -e "  ${GREEN}✓ 完成${NC}" | tee -a "$REPORT_FILE"
                    tested=$((tested + 1))
                else
                    echo -e "  ${RED}✗ 失败${NC}" | tee -a "$REPORT_FILE"
                fi
                echo "" | tee -a "$REPORT_FILE"
            fi
        done
    fi
    
    if [ $tested -eq 0 ]; then
        echo -e "${YELLOW}未找到匹配的 VLM 模型${NC}" | tee -a "$REPORT_FILE"
    else
        echo -e "${CYAN}共测试 $tested 个 VLM 模型${NC}" | tee -a "$REPORT_FILE"
        
        # ── 汇总统计表格 ──────────────────────────────────────────
        if [ ${#VLM_NAMES[@]} -gt 0 ]; then
            echo "" | tee -a "$REPORT_FILE"
            echo -e "${CYAN}╔════════════════════════════════════════════════════════════════════════════════════════════════════╗${NC}" | tee -a "$REPORT_FILE"
            echo -e "${CYAN}║                                    VLM 性能汇总                                                    ║${NC}" | tee -a "$REPORT_FILE"
            echo -e "${CYAN}╠════════════════════════════════════════════════════════════════════════════════════════════════════╣${NC}" | tee -a "$REPORT_FILE"
            printf "║ %-18s │ %9s │ %10s │ %10s │ %10s │ %10s │ %10s ║\n" "模型" "参数量" "Vision(ms)" "TTFT(ms)" "Prompt" "Decode" "总时间(ms)" | tee -a "$REPORT_FILE"
            printf "║ %-18s │ %9s │ %10s │ %10s │ %10s │ %10s │ %10s ║\n" "" "" "" "" "(tok/s)" "(tok/s)" "" | tee -a "$REPORT_FILE"
            echo "╠════════════════════════════════════════════════════════════════════════════════════════════════════╣" | tee -a "$REPORT_FILE"
            
            for i in "${!VLM_NAMES[@]}"; do
                local name="${VLM_NAMES[$i]}"
                local params="${VLM_PARAMS[$i]}"
                local vision="${VLM_VISION_MS[$i]}"
                local ttft="${VLM_TTFT_MS[$i]}"
                local prompt="${VLM_PROMPT_TPS[$i]}"
                local eval="${VLM_EVAL_TPS[$i]}"
                local total="${VLM_TOTAL_MS[$i]}"
                # 截断长模型名
                [ ${#name} -gt 18 ] && name="${name:0:15}..."
                printf "║ %-18s │ %9s │ %10s │ %10s │ %10s │ %10s │ %10s ║\n" "$name" "$params" "$vision" "$ttft" "$prompt" "$eval" "$total" | tee -a "$REPORT_FILE"
            done
            
            echo "╚════════════════════════════════════════════════════════════════════════════════════════════════════╝" | tee -a "$REPORT_FILE"
            
            # ── 保存汇总 CSV 格式 ──────────────────────────────────
            local VLM_RESULT_FILE="$OUTPUT_DIR/vlm_benchmark_$TIMESTAMP.csv"
            echo "model,params,vision_ms,prompt_ms,prompt_tps,eval_ms,eval_tps,ttft_ms,total_ms,total_tokens" > "$VLM_RESULT_FILE"
            for i in "${!VLM_NAMES[@]}"; do
                echo "${VLM_NAMES[$i]},${VLM_PARAMS[$i]},${VLM_VISION_MS[$i]},${VLM_PROMPT_MS[$i]},${VLM_PROMPT_TPS[$i]},${VLM_EVAL_MS[$i]},${VLM_EVAL_TPS[$i]},${VLM_TTFT_MS[$i]},${VLM_TOTAL_MS[$i]},${VLM_TOTAL_TOKENS[$i]}" >> "$VLM_RESULT_FILE"
            done
            echo "" | tee -a "$REPORT_FILE"
            echo "📊 汇总数据已保存: $VLM_RESULT_FILE" | tee -a "$REPORT_FILE"
            
            # ── 关键指标说明 ──────────────────────────────────────
            echo "" | tee -a "$REPORT_FILE"
            echo "关键指标说明:" | tee -a "$REPORT_FILE"
            echo "  - Vision:       图像编码时间 (图像→embedding)" | tee -a "$REPORT_FILE"
            echo "  - TTFT:         首token延迟 = Vision + Prompt eval (用户等待首字时间)" | tee -a "$REPORT_FILE"
            echo "  - Prompt:       prompt + vision tokens 处理速度" | tee -a "$REPORT_FILE"
            echo "  - Decode:       文本生成速度 (越高越好，流畅度关键)" | tee -a "$REPORT_FILE"
            echo "  - 总时间:       完整推理的端到端时间" | tee -a "$REPORT_FILE"
            echo "" | tee -a "$REPORT_FILE"
            echo "📁 单模型报告: $OUTPUT_DIR/benchmark_<model>.txt" | tee -a "$REPORT_FILE"
        fi
    fi
}

# ══════════════════════════════════════════════════════════════════════════════
# VLM 汇总报告生成
# ══════════════════════════════════════════════════════════════════════════════
generate_vlm_summary_report() {
    if [ ${#VLM_NAMES[@]} -eq 0 ]; then
        echo -e "${YELLOW}无 VLM 测试数据，跳过报告生成${NC}"
        return
    fi

    local SUMMARY_FILE="$OUTPUT_DIR/vlm_benchmark_summary.txt"
    local TS=$(date '+%Y-%m-%d %H:%M:%S')

    cat > "$SUMMARY_FILE" << EOF
# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
# RiVision VLM Benchmark Summary Report
# Generated: $TS
# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════

# ── 测试环境 ───────────────────────────────────────────────────────────────────────────────────────────────────────────
platform=$BENCH_PLATFORM
cpu=$BENCH_CPU_MODEL
cores=$BENCH_NPROC
memory=$BENCH_MEM_TOTAL
threads=${VLM_THREADS:-8}

# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
# VLM 性能汇总表
# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
EOF

    echo "# ┌──────────────────────┬──────────┬─────────┬───────────┬───────────┬────────────┬────────────┬────────────┬───────────┐" >> "$SUMMARY_FILE"
    echo "# │ 模型                 │ 参数量   │ 后端    │ Vision    │ Prompt    │ Prompt     │ Decode     │ TTFT       │ 总时间    │" >> "$SUMMARY_FILE"
    echo "# │                      │          │         │ (ms)      │ (ms)      │ (tok/s)    │ (tok/s)    │ (ms)       │ (ms)      │" >> "$SUMMARY_FILE"
    echo "# ├──────────────────────┼──────────┼─────────┼───────────┼───────────┼────────────┼────────────┼────────────┼───────────┤" >> "$SUMMARY_FILE"

    for i in "${!VLM_NAMES[@]}"; do
        local name="${VLM_NAMES[$i]}"
        local params="${VLM_PARAMS[$i]}"
        local vision="${VLM_VISION_MS[$i]}"
        local prompt_ms="${VLM_PROMPT_MS[$i]}"
        local prompt_tps="${VLM_PROMPT_TPS[$i]}"
        local eval_tps="${VLM_EVAL_TPS[$i]}"
        local ttft="${VLM_TTFT_MS[$i]}"
        local total="${VLM_TOTAL_MS[$i]}"
        [ ${#name} -gt 20 ] && name="${name:0:17}..."
        printf "# │ %-20s │ %8s │ SMT/NPU │ %-9s │ %-9s │ %-10s │ %-10s │ %-10s │ %-9s │\n" \
            "$name" "$params" "$vision" "$prompt_ms" "$prompt_tps" "$eval_tps" "$ttft" "$total" >> "$SUMMARY_FILE"
    done

    echo "# └──────────────────────┴──────────┴─────────┴───────────┴───────────┴────────────┴────────────┴────────────┴───────────┘" >> "$SUMMARY_FILE"

    # 详细数据表
    cat >> "$SUMMARY_FILE" << 'EOF'

# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
# 详细数据表 (含生成 token 数)
# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
EOF

    echo "# ┌──────────────────────┬──────────┬────────────┬───────────┬───────────┬────────────┬──────────┬────────────┐" >> "$SUMMARY_FILE"
    echo "# │ 模型                 │ 参数量   │ Eval(ms)   │ Eval      │ TTFT(s)   │ Total(s)   │ Tokens   │ Latency/tok│" >> "$SUMMARY_FILE"
    echo "# │                      │          │            │ (tok/s)   │           │            │          │            │" >> "$SUMMARY_FILE"
    echo "# ├──────────────────────┼──────────┼────────────┼───────────┼───────────┼────────────┼──────────┼────────────┤" >> "$SUMMARY_FILE"

    for i in "${!VLM_NAMES[@]}"; do
        local model="${VLM_NAMES[$i]}"
        local params="${VLM_PARAMS[$i]}"
        local eval_ms="${VLM_EVAL_MS[$i]}"
        local eval_tps="${VLM_EVAL_TPS[$i]}"
        local ttft="${VLM_TTFT_MS[$i]}"
        local total="${VLM_TOTAL_MS[$i]}"
        local tokens="${VLM_TOTAL_TOKENS[$i]}"

        local ttft_s="N/A"
        [ -n "$ttft" ] && [ "$ttft" != "0" ] && [ "$ttft" != "N/A" ] && ttft_s=$(echo "scale=2; $ttft / 1000" | bc 2>/dev/null || echo "N/A")
        local total_s="N/A"
        [ -n "$total" ] && [ "$total" != "0" ] && [ "$total" != "N/A" ] && total_s=$(echo "scale=1; $total / 1000" | bc 2>/dev/null || echo "N/A")
        local latency="N/A"
        [ -n "$tokens" ] && [ "$tokens" != "0" ] && [ -n "$eval_ms" ] && [ "$eval_ms" != "0" ] && \
            latency=$(echo "scale=1; $eval_ms / $tokens" | bc 2>/dev/null || echo "N/A")

        [ ${#model} -gt 20 ] && model="${model:0:17}..."
        printf "# │ %-20s │ %8s │ %-10s │ %-9s │ %-9s │ %-10s │ %-8s │ %-10s │\n" \
            "$model" "$params" "$eval_ms" "$eval_tps" "$ttft_s" "$total_s" "$tokens" "$latency" >> "$SUMMARY_FILE"
    done

    echo "# └──────────────────────┴──────────┴────────────┴───────────┴───────────┴────────────┴──────────┴────────────┘" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" << 'EOF'

# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
# 指标说明
# ═══════════════════════════════════════════════════════════════════════════════════════════════════════════════════════
# Vision(ms)       = 图像编码时间 (SMT/NPU 模式下集成在 Prompt 中，显示为 0)
# Prompt(ms)       = Prompt + Vision tokens 处理时间
# Prompt(tok/s)    = Prompt 处理速度
# Decode(tok/s)    = 文本生成速度，越高用户体验越流畅
# TTFT(ms)         = 首 Token 延迟 (Time To First Token)，用户等待首字响应时间
# Total(ms)        = 完整推理端到端时间
# Latency/tok      = 每个 token 平均生成延迟 = Eval(ms) / Tokens
EOF

    echo "" >> "$SUMMARY_FILE"
    echo -e "${GREEN}✅ VLM 汇总报告已生成: $SUMMARY_FILE${NC}"
    cat "$SUMMARY_FILE"
}

# ══════════════════════════════════════════════════════════════════════════════
# YOLO 汇总报告生成
# ══════════════════════════════════════════════════════════════════════════════
generate_yolo_summary_report() {
    if [ ${#YOLO_NAMES[@]} -eq 0 ]; then
        echo -e "${YELLOW}无 YOLO 测试数据，跳过报告生成${NC}"
        return
    fi

    local SUMMARY_FILE="$OUTPUT_DIR/yolo_benchmark_summary.txt"
    local TS=$(date '+%Y-%m-%d %H:%M:%S')

    cat > "$SUMMARY_FILE" << EOF

╔════════════════════════════════════════════════════════════════════════════════════════╗
║                              YOLO 模型资源需求评估报告                                           ║
║                            $BENCH_PLATFORM                                        ║
╠════════════════════════════════════════════════════════════════════════════════════════╣
║  测试平台: $BENCH_CPU_MODEL | 核心: $BENCH_NPROC | 内存: $BENCH_MEM_TOTAL                ║
║  测试条件: ${RUNS}次推理, ${WARMUP}次预热 | 输入: 640×640                                    ║
║  生成时间: $TS                                                                       ║
╚════════════════════════════════════════════════════════════════════════════════════════╝

┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                             一、综合性能指标                                         ┃
┣━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫
EOF

    printf "┃ %-14s│%8s│%8s│%8s│%7s│%8s│%8s ┃\n" "模型" "后端" "FPS" "延迟" "CPU" "内存" "模型" >> "$SUMMARY_FILE"
    printf "┃ %-14s│%8s│%8s│%8s│%7s│%8s│%8s ┃\n" "" "" "(帧/秒)" "(ms)" "(%)" "(MB)" "(MB)" >> "$SUMMARY_FILE"
    echo "┣━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┫" >> "$SUMMARY_FILE"

    for i in "${!YOLO_NAMES[@]}"; do
        local name="${YOLO_NAMES[$i]}"
        local be="${YOLO_BACKENDS[$i]}"
        local fps="${YOLO_FPS[$i]}"
        local lat="${YOLO_LATENCY[$i]}"
        local cpu="${YOLO_CPU[$i]}"
        local mem="${YOLO_MEMORY[$i]}"
        local sz="${YOLO_MODEL_SIZE[$i]}"
        [ ${#name} -gt 14 ] && name="${name:0:11}..."
        printf "┃ %-14s│%8s│%8s│%8s│%6s%%│%8s│%8s ┃\n" "$name" "$be" "$fps" "$lat" "$cpu" "$mem" "$sz" >> "$SUMMARY_FILE"
    done

    echo "┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛" >> "$SUMMARY_FILE"

    cat >> "$SUMMARY_FILE" << 'EOF'

┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃                              二、指标说明                                          ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛

  ┌──────────────────────────────────────────────────────────────────────────┐
  │ FPS (帧/秒)    = 每秒处理帧数，越高越好                                    │
  │ 延迟 (ms)      = 单帧推理延迟 (预处理 + NPU推理 + 后处理)                   │
  │ CPU (%)        = 平均 CPU 使用率                                          │
  │ 内存 (MB)      = 峰值内存占用                                              │
  │ 模型 (MB)      = 模型文件大小                                              │
  ├──────────────────────────────────────────────────────────────────────────┤
  │ 延迟对实时视频流的影响 (@30fps 视频源):                                     │
  │   <50ms  = 1-2帧延迟，几乎无感知，适合实时监控/交互                         │
  │   50-100ms = 2-3帧延迟，略有滞后，适合视频分析/录像                         │
  │   >200ms = 5-6帧延迟，明显延迟，仅适合离线批处理                            │
  └──────────────────────────────────────────────────────────────────────────┘
EOF

    echo "" >> "$SUMMARY_FILE"
    echo -e "${GREEN}✅ YOLO 汇总报告已生成: $SUMMARY_FILE${NC}"
    cat "$SUMMARY_FILE"
}

# 解析参数
COMMAND=""
TEST_IMAGE=""
SINGLE_MODEL=""
YOLO_MODEL=""
VLM_MODEL=""
BACKEND_FILTER=""
FORCE_BACKEND=""
GENERATE_REPORT=0

while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        -p|--profile)
            PROFILE="$2"
            # 根据 profile 设置参数
            case $PROFILE in
                quick)
                    RUNS=10; WARMUP=2
                    ;;
                standard)
                    RUNS=100; WARMUP=5
                    ;;
                thorough)
                    RUNS=500; WARMUP=20
                    ;;
                npu_only)
                    RUNS=100; BACKEND_FILTER="npu"
                    ;;
                cpu_only)
                    RUNS=100; BACKEND_FILTER="cpu"
                    ;;
            esac
            shift 2
            ;;
        -r|--runs)
            RUNS="$2"
            shift 2
            ;;
        -w|--warmup)
            WARMUP="$2"
            shift 2
            ;;
        -i|--image)
            TEST_IMAGE="$2"
            shift 2
            ;;
        -m|--model)
            SINGLE_MODEL="$2"
            shift 2
            ;;
        --yolo-model)
            YOLO_MODEL="$2"
            shift 2
            ;;
        --vlm-model)
            VLM_MODEL="$2"
            shift 2
            ;;
        --backend)
            FORCE_BACKEND="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --report)
            GENERATE_REPORT=1
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        check|run|yolo|vlm|list|all)
            COMMAND="$1"
            shift
            ;;
        *)
            echo -e "${RED}未知参数: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 执行命令
case "$COMMAND" in
    check)
        run_check
        ;;
    list)
        list_all
        ;;
    yolo)
        run_yolo_tests
        [ "$GENERATE_REPORT" = "1" ] && generate_yolo_summary_report
        echo -e "\n${GREEN}报告已保存: $REPORT_FILE${NC}"
        ;;
    vlm)
        run_vlm_tests
        [ "$GENERATE_REPORT" = "1" ] && generate_vlm_summary_report
        echo -e "\n${GREEN}报告已保存: $REPORT_FILE${NC}"
        ;;
    run|all)
        echo -e "${CYAN}使用配置: $CONFIG_FILE${NC}"
        echo -e "${CYAN}Profile: $PROFILE (YOLO: $RUNS runs, warmup: $WARMUP)${NC}"
        echo ""
        run_yolo_tests
        echo ""
        run_vlm_tests
        if [ "$GENERATE_REPORT" = "1" ]; then
            generate_yolo_summary_report
            generate_vlm_summary_report
        fi
        echo -e "\n${GREEN}报告已保存: $REPORT_FILE${NC}"
        ;;
    "")
        show_help
        ;;
    *)
        echo -e "${RED}未知命令: $COMMAND${NC}"
        show_help
        exit 1
        ;;
esac
