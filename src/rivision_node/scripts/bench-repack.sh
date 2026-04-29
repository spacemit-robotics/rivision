#!/bin/bash
# ============================================================
# P4 Repack 隔离测试脚本
# 测试 KV cache (F16 vs Q8_0) × 线程数 (6 vs 8) 的组合
# 每种配置运行 3 次取平均
#
# 用法: ./scripts/bench-repack.sh
# 需要在 K3 上运行 (repack build)
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
IMAGE="$DEPLOY_DIR/test.jpg"
RUNS=3

# 模型路径
MODEL="$DEPLOY_DIR/models/fastvlm-0.5b-q8_0.gguf"
MMPROJ="$DEPLOY_DIR/models/fastvlm-0.5b-mmproj-f16.gguf"

# CLI 路径
LLAMA_ENGINE_DIR="${LLAMA_ENGINE_DIR:-$DEPLOY_DIR/engines/llama.cpp}"
MTMD_CLI="$LLAMA_ENGINE_DIR/llama-mtmd-cli"
if [ ! -x "$MTMD_CLI" ]; then
    echo "❌ llama-mtmd-cli 不存在: $MTMD_CLI"
    exit 1
fi

# 共享库路径
export LD_LIBRARY_PATH="$DEPLOY_DIR/lib:$LLAMA_ENGINE_DIR:${LD_LIBRARY_PATH:-}"

# 测试配置: NAME THREADS KV_ARGS
CONFIGS=(
    "repack+F16_KV_t6|6|"
    "repack+F16_KV_t8|8|"
    "repack+Q8_KV_t6|6|--cache-type-k q8_0 --cache-type-v q8_0"
    "repack+Q8_KV_t8|8|--cache-type-k q8_0 --cache-type-v q8_0"
)

echo "=============================================="
echo "  P4 Repack 隔离测试"
echo "  模型: fastvlm-0.5b-q8"
echo "  图片: $IMAGE"
echo "  每种配置运行 ${RUNS} 次"
echo "=============================================="
echo ""

# 结果表头
printf "%-22s | %6s | %10s | %10s | %10s | %10s | %10s\n" \
    "配置" "线程" "Vision(ms)" "ImgDec(ms)" "PE(tok/s)" "Eval(tok/s)" "Total(ms)"
printf "%s\n" "----------------------|--------|------------|------------|------------|------------|----------"

for config_str in "${CONFIGS[@]}"; do
    IFS='|' read -r NAME THREADS KV_ARGS <<< "$config_str"

    export OMP_NUM_THREADS=$THREADS

    sum_vision=0
    sum_imgdec=0
    sum_pe=0
    sum_eval=0
    sum_total=0
    valid_runs=0

    for run in $(seq 1 $RUNS); do
        # 构建命令
        CMD_ARGS=(
            -m "$MODEL"
            --mmproj "$MMPROJ"
            --image "$IMAGE"
            -p "简要描述图片内容"
            -c 4096 -n 2048 -t "$THREADS"
            --temp 0.3 --seed 42
            --top-k 40 --top-p 0.9
            --jinja
        )

        if [ -n "$KV_ARGS" ]; then
            read -ra KV_EXTRA <<< "$KV_ARGS"
            CMD_ARGS+=("${KV_EXTRA[@]}")
        fi

        # 运行并捕获输出
        OUTPUT=$("$MTMD_CLI" "${CMD_ARGS[@]}" 2>&1) || true

        # 提取指标
        vision=$(echo "$OUTPUT" | grep -oP 'image slice encoded in \K[0-9]+' | head -1)
        imgdec=$(echo "$OUTPUT" | grep -oP 'image decoded.*in \K[0-9]+' | head -1)
        pe=$(echo "$OUTPUT" | grep 'prompt eval time' | grep -oP '(\d+\.\d+) tokens per second' | grep -oP '[\d.]+')
        ev=$(echo "$OUTPUT" | grep '  eval time' | grep -oP '(\d+\.\d+) tokens per second' | grep -oP '[\d.]+')
        total=$(echo "$OUTPUT" | grep 'total time' | grep -oP '(\d+\.\d+) ms' | head -1 | grep -oP '[\d.]+')

        if [ -n "$pe" ] && [ -n "$ev" ]; then
            sum_vision=$(echo "$sum_vision + ${vision:-0}" | bc)
            sum_imgdec=$(echo "$sum_imgdec + ${imgdec:-0}" | bc)
            sum_pe=$(echo "$sum_pe + $pe" | bc)
            sum_eval=$(echo "$sum_eval + $ev" | bc)
            sum_total=$(echo "$sum_total + ${total:-0}" | bc)
            valid_runs=$((valid_runs + 1))
        fi

        # 进度指示
        echo -ne "\r  [$NAME] run $run/$RUNS ...   " >&2
    done
    echo -ne "\r                                          \r" >&2

    if [ "$valid_runs" -gt 0 ]; then
        avg_vision=$(echo "scale=0; $sum_vision / $valid_runs" | bc)
        avg_imgdec=$(echo "scale=0; $sum_imgdec / $valid_runs" | bc)
        avg_pe=$(echo "scale=2; $sum_pe / $valid_runs" | bc)
        avg_eval=$(echo "scale=2; $sum_eval / $valid_runs" | bc)
        avg_total=$(echo "scale=0; $sum_total / $valid_runs" | bc)

        printf "%-22s | %6s | %10s | %10s | %10s | %10s | %10s\n" \
            "$NAME" "$THREADS" "$avg_vision" "$avg_imgdec" "$avg_pe" "$avg_eval" "$avg_total"
    else
        printf "%-22s | %6s | %10s | %10s | %10s | %10s | %10s\n" \
            "$NAME" "$THREADS" "FAIL" "FAIL" "FAIL" "FAIL" "FAIL"
    fi
done

echo ""
echo "=== 测试完成 ==="
echo ""
echo "参考基线 (q8_2.log, 无repack, F16 KV, 8线程):"
echo "  Vision=1607ms, ImgDec=838ms, PE=19.43tok/s, Eval=20.98tok/s, Total=7309ms"
