#!/bin/bash
# ============================================================
# RiVision 完整基准测试
# 
# 运行所有 YOLO/VLM 的本地和 HTTP 测试
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BENCHMARK_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="$(cd "$BENCHMARK_DIR/.." && pwd)"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

# 参数
IMAGE="${1:-$DEPLOY_DIR/test.jpg}"
OUTPUT_DIR="${2:-$BENCHMARK_DIR/results/$(date +%Y%m%d_%H%M%S)}"

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║         RiVision 完整基准测试套件                        ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "测试图片: $(basename "$IMAGE")"
echo "输出目录: $OUTPUT_DIR"
echo ""

mkdir -p "$OUTPUT_DIR"

# 记录开始时间
echo "开始时间: $(date '+%Y-%m-%d %H:%M:%S')" > "$OUTPUT_DIR/summary.txt"
echo "" >> "$OUTPUT_DIR/summary.txt"

TESTS_RUN=0
TESTS_PASSED=0

# ── YOLO HTTP 测试 ────────────────────────────────────────
echo -e "${CYAN}▶ [1/4] YOLO HTTP 测试${NC}"
if "$SCRIPT_DIR/run_yolo_http.sh" "$IMAGE" 50 true "$OUTPUT_DIR" 2>&1 | tee "$OUTPUT_DIR/yolo_http.log"; then
    echo -e "${GREEN}✓ YOLO HTTP 测试完成${NC}"
    echo "YOLO HTTP: PASSED" >> "$OUTPUT_DIR/summary.txt"
    ((TESTS_PASSED++)) || true
else
    echo -e "${YELLOW}⚠ YOLO HTTP 测试失败或服务未运行${NC}"
    echo "YOLO HTTP: SKIPPED" >> "$OUTPUT_DIR/summary.txt"
fi
((TESTS_RUN++)) || true
echo ""

# ── YOLO 本地测试 ────────────────────────────────────────
echo -e "${CYAN}▶ [2/4] YOLO 本地测试${NC}"
YOLO_MODEL="$DEPLOY_DIR/models/yolov8n.onnx"
if [ -f "$YOLO_MODEL" ]; then
    if "$SCRIPT_DIR/run_yolo_local.sh" "$IMAGE" "$YOLO_MODEL" 50 4 "$OUTPUT_DIR" 2>&1 | tee "$OUTPUT_DIR/yolo_local.log"; then
        echo -e "${GREEN}✓ YOLO 本地测试完成${NC}"
        echo "YOLO Local: PASSED" >> "$OUTPUT_DIR/summary.txt"
        ((TESTS_PASSED++)) || true
    else
        echo -e "${YELLOW}⚠ YOLO 本地测试失败${NC}"
        echo "YOLO Local: FAILED" >> "$OUTPUT_DIR/summary.txt"
    fi
else
    echo -e "${YELLOW}⚠ YOLO 模型不存在，跳过本地测试${NC}"
    echo "YOLO Local: SKIPPED (model not found)" >> "$OUTPUT_DIR/summary.txt"
fi
((TESTS_RUN++)) || true
echo ""

# ── VLM HTTP 测试 ────────────────────────────────────────
echo -e "${CYAN}▶ [3/4] VLM HTTP 测试${NC}"
if "$SCRIPT_DIR/run_vlm_http.sh" "$IMAGE" 3 "$OUTPUT_DIR" 2>&1 | tee "$OUTPUT_DIR/vlm_http.log"; then
    echo -e "${GREEN}✓ VLM HTTP 测试完成${NC}"
    echo "VLM HTTP: PASSED" >> "$OUTPUT_DIR/summary.txt"
    ((TESTS_PASSED++)) || true
else
    echo -e "${YELLOW}⚠ VLM HTTP 测试失败或服务未运行${NC}"
    echo "VLM HTTP: SKIPPED" >> "$OUTPUT_DIR/summary.txt"
fi
((TESTS_RUN++)) || true
echo ""

# ── VLM 本地测试 ────────────────────────────────────────
echo -e "${CYAN}▶ [4/4] VLM 本地测试${NC}"
if [ -f "$DEPLOY_DIR/config/active-model.sh" ]; then
    if "$SCRIPT_DIR/run_vlm_local.sh" "$IMAGE" 3 6 "$OUTPUT_DIR" 2>&1 | tee "$OUTPUT_DIR/vlm_local.log"; then
        echo -e "${GREEN}✓ VLM 本地测试完成${NC}"
        echo "VLM Local: PASSED" >> "$OUTPUT_DIR/summary.txt"
        ((TESTS_PASSED++)) || true
    else
        echo -e "${YELLOW}⚠ VLM 本地测试失败${NC}"
        echo "VLM Local: FAILED" >> "$OUTPUT_DIR/summary.txt"
    fi
else
    echo -e "${YELLOW}⚠ VLM 模型未配置，跳过本地测试${NC}"
    echo "VLM Local: SKIPPED (model not configured)" >> "$OUTPUT_DIR/summary.txt"
fi
((TESTS_RUN++)) || true
echo ""

# ── 汇总 ────────────────────────────────────────────────
echo "" >> "$OUTPUT_DIR/summary.txt"
echo "结束时间: $(date '+%Y-%m-%d %H:%M:%S')" >> "$OUTPUT_DIR/summary.txt"
echo "测试结果: $TESTS_PASSED/$TESTS_RUN 通过" >> "$OUTPUT_DIR/summary.txt"

echo ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║                    测试完成                              ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "测试结果: $TESTS_PASSED/$TESTS_RUN 通过"
echo ""
echo "结果目录: $OUTPUT_DIR/"
echo "  - summary.txt          汇总"
echo "  - yolo_http.log        YOLO HTTP 日志"
echo "  - yolo_local.log       YOLO 本地日志"
echo "  - vlm_http.log         VLM HTTP 日志"
echo "  - vlm_local.log        VLM 本地日志"
echo "  - benchmark_*.json     详细结果"
echo ""

# 显示汇总
echo -e "${CYAN}── 汇总 ──${NC}"
cat "$OUTPUT_DIR/summary.txt"
