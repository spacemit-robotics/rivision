#!/bin/bash
# test-llama-a100.sh - 验证 llama.cpp 是否正确运行在 K3 A100 核心上
# 用法: ./test-llama-a100.sh [llama.cpp目录]

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║  K3 A100 Core Verification Test                              ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo ""

# 确定 llama.cpp 目录
LLAMA_DIR="${1:-/opt/rivision/rivision_node/engines/llama.cpp}"
LIB_DIR="${2:-/opt/rivision/rivision_node/lib}"

if [ ! -d "$LLAMA_DIR" ]; then
    # 尝试开发环境路径
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    LLAMA_DIR="$SCRIPT_DIR/../third_party/llama.cpp/riscv64-0.0.7a0"
    LIB_DIR="$LLAMA_DIR/lib"
fi

echo "1. 环境检测"
echo "───────────────────────────────────────────────────────────────"
echo "   架构: $(uname -m)"
echo "   CPU 核心数: $(nproc)"
echo "   llama.cpp 目录: $LLAMA_DIR"
echo ""

# 检查是否是 K3 (RISC-V)
if [ "$(uname -m)" != "riscv64" ]; then
    echo -e "   ${YELLOW}⚠ 非 RISC-V 平台，跳过 A100 测试${NC}"
    echo ""
    echo "   在 K3 节点上运行此脚本以验证 A100 核心"
    exit 0
fi

echo "2. SpacemiT K3 支持检测"
echo "───────────────────────────────────────────────────────────────"

# 检查 llama-server
LLAMA_SERVER="$LLAMA_DIR/llama-server"
if [ ! -f "$LLAMA_SERVER" ]; then
    echo -e "   ${RED}✗ llama-server 不存在: $LLAMA_SERVER${NC}"
    exit 1
fi

# 检查 SpacemiT 函数
SPACEMIT_COUNT=$(strings "$LLAMA_SERVER" 2>/dev/null | grep -c spacemit || echo 0)
echo "   llama-server spacemit 符号: $SPACEMIT_COUNT"
if [ "$SPACEMIT_COUNT" -gt 0 ]; then
    echo -e "   ${GREEN}✓ SpacemiT 支持已编译${NC}"
else
    echo -e "   ${RED}✗ 未检测到 SpacemiT 支持！${NC}"
    echo "   这是通用 RISC-V 版本，不会自动使用 A100 核心"
    exit 1
fi

# 检查 libggml-cpu.so
LIBGGML="$LIB_DIR/libggml-cpu.so"
if [ -f "$LIBGGML" ] || [ -L "$LIBGGML" ]; then
    # 找到实际文件
    LIBGGML_REAL=$(readlink -f "$LIBGGML" 2>/dev/null || echo "$LIBGGML")
    BIND_AI=$(nm -D "$LIBGGML_REAL" 2>/dev/null | grep -c bind_ai_thread || echo 0)
    NUMA_AFFINITY=$(nm -D "$LIBGGML_REAL" 2>/dev/null | grep -c spacemit_set_numa || echo 0)
    echo "   libggml-cpu bind_ai_thread: $BIND_AI"
    echo "   libggml-cpu numa_affinity: $NUMA_AFFINITY"
    if [ "$BIND_AI" -gt 0 ]; then
        echo -e "   ${GREEN}✓ A100 绑核函数存在${NC}"
    else
        echo -e "   ${YELLOW}⚠ 未找到 bind_ai_thread${NC}"
    fi
else
    echo -e "   ${YELLOW}⚠ libggml-cpu.so 不存在，跳过检查${NC}"
fi

echo ""
echo "3. CPU 拓扑检测"
echo "───────────────────────────────────────────────────────────────"

# K3 CPU 拓扑: 0-7 = X100 (通用), 8-15 = A100 (AI)
# 注意: nproc 返回的是当前进程可用的核心数，可能受 cgroup 限制
VISIBLE_CORES=$(nproc)
# 尝试获取系统实际核心数
TOTAL_CORES=$(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo $VISIBLE_CORES)
AI_CORES=$((TOTAL_CORES / 2))

echo "   系统核心数: $TOTAL_CORES"
echo "   进程可见核心: $VISIBLE_CORES"
if [ "$VISIBLE_CORES" -lt "$TOTAL_CORES" ]; then
    echo -e "   ${YELLOW}⚠ 进程受 cgroup 限制，可能无法访问所有核心${NC}"
fi
echo "   X100 通用核: 0-$((AI_CORES - 1))"
echo "   A100 智算核: $AI_CORES-$((TOTAL_CORES - 1))"

# 检查当前进程的 CPU 亲和性
CURRENT_AFFINITY=$(taskset -p $$ 2>/dev/null | awk '{print $NF}' || echo "unknown")
echo "   当前进程亲和性: $CURRENT_AFFINITY"
echo ""

echo "4. 实时 CPU 使用率测试"
echo "───────────────────────────────────────────────────────────────"
echo "   启动 llama-server 并监控 CPU 使用..."
echo ""

# 检查是否有模型文件 (排除 mmproj 视觉编码器)
MODEL_DIR="/opt/rivision/rivision_node/models"
MODEL_FILE=""
for dir in "$MODEL_DIR"/*/; do
    for f in "$dir"/*.gguf; do
        if [ -f "$f" ]; then
            # 跳过 mmproj (CLIP) 文件，它不是主模型
            case "$(basename "$f")" in
                mmproj*|*mmproj*) continue ;;
            esac
            MODEL_FILE="$f"
            break 2
        fi
    done
done

if [ -z "$MODEL_FILE" ]; then
    echo -e "   ${YELLOW}⚠ 未找到模型文件，跳过运行时测试${NC}"
    echo "   请手动运行 llama-server 并使用以下命令监控:"
    echo ""
    echo "   # 监控各核心 CPU 使用率"
    echo "   mpstat -P ALL 1"
    echo ""
    echo "   # 或查看进程的 CPU 亲和性"
    echo "   taskset -p \$(pidof llama-server)"
    echo ""
    exit 0
fi

echo "   模型: $MODEL_FILE"
echo ""

# 后台启动 llama-server
export LD_LIBRARY_PATH="$LIB_DIR:${LD_LIBRARY_PATH:-}"
"$LLAMA_SERVER" -m "$MODEL_FILE" -t 8 --port 19999 > /tmp/llama-test.log 2>&1 &
LLAMA_PID=$!
echo "   llama-server PID: $LLAMA_PID"

# 等待启动
sleep 3

# 检查是否运行
if ! kill -0 $LLAMA_PID 2>/dev/null; then
    echo -e "   ${RED}✗ llama-server 启动失败${NC}"
    cat /tmp/llama-test.log
    exit 1
fi

# 检查 CPU 亲和性
echo ""
echo "   CPU 亲和性检查:"
AFFINITY=$(taskset -p $LLAMA_PID 2>/dev/null | awk '{print $NF}')
echo "   affinity mask: $AFFINITY"

# 解析亲和性掩码
# A100 核心 8-15 对应的掩码: 0xff00 (二进制 1111111100000000)
# X100 核心 0-7 对应的掩码: 0x00ff (二进制 0000000011111111)
if [[ "$AFFINITY" == *"ff00"* ]] || [[ "$AFFINITY" == *"FF00"* ]]; then
    echo -e "   ${GREEN}✓ 进程绑定到 A100 核心 (8-15)${NC}"
elif [[ "$AFFINITY" == *"ffff"* ]] || [[ "$AFFINITY" == *"FFFF"* ]]; then
    echo -e "   ${YELLOW}⚠ 进程可使用所有核心 (0-15)${NC}"
else
    echo -e "   ${YELLOW}⚠ 亲和性掩码: $AFFINITY${NC}"
fi

# 发送测试请求
echo ""
echo "   发送测试请求..."
curl -s -X POST http://127.0.0.1:19999/completion \
    -H "Content-Type: application/json" \
    -d '{"prompt":"Hello","n_predict":5}' > /dev/null 2>&1 &

sleep 2

# 检查线程的 CPU 使用
echo ""
echo "   线程 CPU 分布:"
ps -T -p $LLAMA_PID -o tid,psr,pcpu 2>/dev/null | head -20

# 统计使用的核心
CORES_USED=$(ps -T -p $LLAMA_PID -o psr 2>/dev/null | tail -n +2 | sort -u)
echo ""
echo "   使用的核心: $CORES_USED"

# 分析核心类型
X100_COUNT=0
A100_COUNT=0
for core in $CORES_USED; do
    if [ "$core" -lt 8 ]; then
        ((X100_COUNT++))
    else
        ((A100_COUNT++))
    fi
done

echo ""
echo "5. 测试结果"
echo "───────────────────────────────────────────────────────────────"
echo "   X100 核心使用数: $X100_COUNT"
echo "   A100 核心使用数: $A100_COUNT"

if [ "$A100_COUNT" -gt "$X100_COUNT" ]; then
    echo -e "   ${GREEN}✓ 主要运行在 A100 核心上${NC}"
else
    echo -e "   ${RED}✗ 未正确使用 A100 核心${NC}"
fi

# 清理
kill $LLAMA_PID 2>/dev/null || true
rm -f /tmp/llama-test.log

echo ""
echo "测试完成"
