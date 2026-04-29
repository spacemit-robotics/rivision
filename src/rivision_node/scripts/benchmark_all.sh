#!/bin/bash
# ============================================================
# RiVision 完整推理性能基准测试套件
# 用法: ./benchmark_all.sh [--yolo-only|--vlm-only] [--quick|--full]
#
# 自动运行:
#   1. YOLO 目标检测基准测试
#   2. VLM 视觉语言模型基准测试
#   3. 生成综合性能报告
#
# 适用于 K3 (RISC-V SpacemiT A100 NPU) 性能评估
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── 颜色 ──────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# ── 参数解析 ──────────────────────────────────────────────
RUN_YOLO=true
RUN_VLM=true
QUICK_MODE=false
YOLO_RUNS=10
YOLO_CONCURRENCY=4
VLM_RUNS=3

while [[ $# -gt 0 ]]; do
    case $1 in
        --yolo-only) RUN_VLM=false ;;
        --vlm-only)  RUN_YOLO=false ;;
        --quick)     QUICK_MODE=true; YOLO_RUNS=5; VLM_RUNS=1 ;;
        --full)      YOLO_RUNS=20; VLM_RUNS=5 ;;
        -h|--help)
            echo "用法: $0 [选项]"
            echo ""
            echo "选项:"
            echo "  --yolo-only    只运行 YOLO 测试"
            echo "  --vlm-only     只运行 VLM 测试"
            echo "  --quick        快速测试 (YOLO: 5次, VLM: 1次)"
            echo "  --full         完整测试 (YOLO: 20次, VLM: 5次)"
            echo "  -h, --help     显示帮助"
            exit 0
            ;;
        *) echo "未知参数: $1"; exit 1 ;;
    esac
    shift
done

# ── 检测平台 ──────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    riscv64) PLATFORM="K3-SpacemiT-A100" ;;
    x86_64)  PLATFORM="x86_64" ;;
    aarch64) PLATFORM="ARM64" ;;
    *)       PLATFORM="$ARCH" ;;
esac

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_DIR="$DEPLOY_DIR/logs/benchmark_$TIMESTAMP"
mkdir -p "$REPORT_DIR"

# ── 查找测试图片 ──────────────────────────────────────────
find_test_image() {
    local candidates=(
        "$DEPLOY_DIR/test.jpg"
        "$DEPLOY_DIR/test.png"
        "$SCRIPT_DIR/test.jpg"
    )
    for img in "${candidates[@]}"; do
        [ -f "$img" ] && echo "$img" && return
    done
    echo ""
}

TEST_IMAGE=$(find_test_image)
if [ -z "$TEST_IMAGE" ]; then
    echo -e "${YELLOW}⚠️  未找到测试图片，创建默认测试图片...${NC}"
    # 使用 ImageMagick 创建测试图片
    if command -v convert &>/dev/null; then
        TEST_IMAGE="$DEPLOY_DIR/test.jpg"
        convert -size 640x480 xc:skyblue \
            -fill white -pointsize 40 -gravity center \
            -annotate 0 "RiVision Test" \
            "$TEST_IMAGE"
        echo -e "${GREEN}✅ 已创建测试图片: $TEST_IMAGE${NC}"
    else
        echo -e "${RED}❌ 需要测试图片，请放置 test.jpg 到 $DEPLOY_DIR/${NC}"
        exit 1
    fi
fi

# ══════════════════════════════════════════════════════════
# 主程序
# ══════════════════════════════════════════════════════════

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║     RiVision 完整推理性能基准测试套件                    ║${NC}"
echo -e "${BOLD}║                  K3 SpacemiT A100 NPU                    ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "┌─ 测试配置 ──────────────────────────────────────────────"
echo "│  平台:         $PLATFORM"
echo "│  测试图片:     $(basename "$TEST_IMAGE")"
echo "│  报告目录:     $REPORT_DIR"
echo "│"
echo "│  测试项目:"
$RUN_YOLO && echo "│    ✓ YOLO 目标检测 ($YOLO_RUNS 次, 并发=$YOLO_CONCURRENCY)"
$RUN_VLM  && echo "│    ✓ VLM 视觉语言 ($VLM_RUNS 次)"
echo "│"
$QUICK_MODE && echo "│  模式:         快速测试"
echo "└────────────────────────────────────────────────────────"
echo ""

# ── 系统信息收集 ──────────────────────────────────────────
echo -e "${CYAN}▶ 收集系统信息...${NC}"

SYSINFO_FILE="$REPORT_DIR/system_info.txt"
cat > "$SYSINFO_FILE" << EOF
# RiVision 系统信息
# $(date '+%Y-%m-%d %H:%M:%S')
# ============================================================

## 平台
platform=$PLATFORM
arch=$(uname -m)
kernel=$(uname -r)
hostname=$(hostname)

## CPU
$(cat /proc/cpuinfo | grep -E '^(model name|isa|cpu MHz|processor)' | head -20)

## 内存
$(cat /proc/meminfo | grep -E '^(MemTotal|MemFree|MemAvailable|Buffers|Cached)')

## 存储
$(df -h / 2>/dev/null | tail -1)

## NPU/加速器
EOF

# K3 SpacemiT 特定信息
if [ "$ARCH" = "riscv64" ]; then
    echo "spacemit_a100=true" >> "$SYSINFO_FILE"
    # 尝试获取 NPU 状态
    if [ -d /sys/class/spacemit ]; then
        ls -la /sys/class/spacemit >> "$SYSINFO_FILE" 2>/dev/null || true
    fi
fi

echo -e "${GREEN}  系统信息已保存${NC}"

# ── YOLO 基准测试 ─────────────────────────────────────────
YOLO_RESULT=""
if $RUN_YOLO; then
    echo ""
    echo -e "${BOLD}════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  YOLO 目标检测基准测试${NC}"
    echo -e "${BOLD}════════════════════════════════════════════════════════════${NC}"
    echo ""
    
    # 检查 YOLO 服务
    [ -f "$DEPLOY_DIR/config/.env" ] && source "$DEPLOY_DIR/config/.env"
    YOLO_HOST="${YOLO_HOST:-127.0.0.1}"
    YOLO_PORT="${YOLO_PORT:-9081}"
    
    if ! curl -s --max-time 2 "http://${YOLO_HOST}:${YOLO_PORT}/health" >/dev/null 2>&1; then
        echo -e "${YELLOW}⚠️  YOLO 服务未运行，尝试启动...${NC}"
        "$SCRIPT_DIR/start-yolo.sh" &
        YOLO_PID=$!
        sleep 5
        
        if ! curl -s --max-time 2 "http://${YOLO_HOST}:${YOLO_PORT}/health" >/dev/null 2>&1; then
            echo -e "${RED}❌ YOLO 服务启动失败，跳过测试${NC}"
            RUN_YOLO=false
        fi
    fi
    
    if $RUN_YOLO; then
        YOLO_LOG="$REPORT_DIR/yolo_benchmark.log"
        "$SCRIPT_DIR/benchmark_yolo.sh" "$TEST_IMAGE" "$YOLO_RUNS" "$YOLO_CONCURRENCY" 2>&1 | tee "$YOLO_LOG"
        
        # 提取关键指标
        YOLO_RESULT=$(ls -t "$DEPLOY_DIR/logs/benchmark_yolo_"*.txt 2>/dev/null | head -1)
        if [ -n "$YOLO_RESULT" ]; then
            cp "$YOLO_RESULT" "$REPORT_DIR/"
        fi
    fi
fi

# ── VLM 基准测试 ──────────────────────────────────────────
VLM_RESULT=""
if $RUN_VLM; then
    echo ""
    echo -e "${BOLD}════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}  VLM 视觉语言模型基准测试${NC}"
    echo -e "${BOLD}════════════════════════════════════════════════════════════${NC}"
    echo ""
    
    # 检查模型配置
    if [ ! -f "$DEPLOY_DIR/config/active-model.sh" ]; then
        echo -e "${RED}❌ VLM 模型未配置，跳过测试${NC}"
        echo "   请先运行: $SCRIPT_DIR/switch-model.sh <model>"
        RUN_VLM=false
    fi
    
    if $RUN_VLM; then
        VLM_LOG="$REPORT_DIR/vlm_benchmark.log"
        "$SCRIPT_DIR/benchmark_vlm.sh" "$TEST_IMAGE" "$VLM_RUNS" 2>&1 | tee "$VLM_LOG"
        
        VLM_RESULT=$(ls -t "$DEPLOY_DIR/logs/benchmark_vlm_"*.txt 2>/dev/null | head -1)
        if [ -n "$VLM_RESULT" ]; then
            cp "$VLM_RESULT" "$REPORT_DIR/"
        fi
    fi
fi

# ── 生成综合报告 ──────────────────────────────────────────
echo ""
echo -e "${CYAN}▶ 生成综合报告...${NC}"

REPORT_FILE="$REPORT_DIR/benchmark_report.md"

cat > "$REPORT_FILE" << EOF
# RiVision 推理性能基准测试报告

**测试时间:** $(date '+%Y-%m-%d %H:%M:%S')  
**平台:** $PLATFORM  
**架构:** $(uname -m)  

---

## 📊 测试概览

| 项目 | 状态 |
|------|------|
EOF

if [ -n "$YOLO_RESULT" ]; then
    echo "| YOLO 目标检测 | ✅ 完成 |" >> "$REPORT_FILE"
else
    echo "| YOLO 目标检测 | ❌ 跳过 |" >> "$REPORT_FILE"
fi

if [ -n "$VLM_RESULT" ]; then
    echo "| VLM 视觉语言 | ✅ 完成 |" >> "$REPORT_FILE"
else
    echo "| VLM 视觉语言 | ❌ 跳过 |" >> "$REPORT_FILE"
fi

# YOLO 结果
if [ -n "$YOLO_RESULT" ]; then
    cat >> "$REPORT_FILE" << EOF

---

## 🎯 YOLO 目标检测

EOF
    source "$YOLO_RESULT"
    cat >> "$REPORT_FILE" << EOF
| 指标 | 值 |
|------|------|
| 模型 | $model |
| Workers | $workers |
| 吞吐量 | **${throughput_fps} fps** |
| 平均延迟 | ${avg_latency_ms} ms |
| P95 延迟 | ${p95_latency_ms} ms |
| 成功率 | ${success_rate}% |
| 进程内存 | ${process_memory_mb} MB |
| CPU 使用率 | ${cpu_usage_percent}% |
EOF
fi

# VLM 结果
if [ -n "$VLM_RESULT" ]; then
    cat >> "$REPORT_FILE" << EOF

---

## 🧠 VLM 视觉语言模型

EOF
    source "$VLM_RESULT"
    cat >> "$REPORT_FILE" << EOF
| 指标 | 值 |
|------|------|
| 模型 | $model |
| 视觉后端 | $vision_backend |
| Token生成速度 | **${avg_eval_tps} tok/s** |
| 首Token延迟 | ${avg_ttft_ms} ms |
| 总处理时间 | ${avg_total_ms} ms |
| Vision编码 | ${avg_vision_ms} ms |
| 峰值内存 | ${peak_memory_gb} GB |
EOF
fi

# K3 优化建议
if [ "$ARCH" = "riscv64" ]; then
    cat >> "$REPORT_FILE" << EOF

---

## 🔧 K3 SpacemiT A100 优化建议

### YOLO 优化
1. **Workers配置**: 设置 \`YOLO_WORKERS=4\` 实现 4 并发帧
2. **线程配置**: 设置 \`YOLO_THREADS=4\` 共享全局线程池
3. **目标吞吐量**: 15 fps (4并发 × ~267ms/帧)

### VLM 优化
1. **启用NPU**: 使用 \`--media-backend smt\` 加速 Vision 编码
2. **模型选择**: 推荐 MiniCPM-V 4bit 量化版本
3. **线程配置**: \`LLAMA_THREADS=6-8\`

### 内存优化
1. 使用量化模型 (4bit/8bit) 减少内存占用
2. 避免同时运行 YOLO 和 VLM 大模型
3. 监控 \`/proc/meminfo\` 防止 OOM

EOF
fi

cat >> "$REPORT_FILE" << EOF

---

## 📁 测试文件

- 系统信息: \`system_info.txt\`
EOF

[ -n "$YOLO_RESULT" ] && echo "- YOLO 详细结果: \`$(basename "$YOLO_RESULT")\`" >> "$REPORT_FILE"
[ -n "$VLM_RESULT" ] && echo "- VLM 详细结果: \`$(basename "$VLM_RESULT")\`" >> "$REPORT_FILE"

echo "" >> "$REPORT_FILE"
echo "*Generated by RiVision Benchmark Suite*" >> "$REPORT_FILE"

# ── 完成 ──────────────────────────────────────────────────
echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║  基准测试完成                                            ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "📊 报告目录: ${GREEN}$REPORT_DIR${NC}"
echo ""
echo "文件列表:"
ls -la "$REPORT_DIR"
echo ""

# 如果有 YOLO 服务是我们启动的，停止它
if [ -n "${YOLO_PID:-}" ]; then
    echo "停止临时 YOLO 服务..."
    kill "$YOLO_PID" 2>/dev/null || true
fi

echo -e "${GREEN}✅ 完成！${NC}"
