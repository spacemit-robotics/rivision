#!/bin/bash
# check-llama-spacemit.sh - 快速检查 llama.cpp 二进制是否包含 SpacemiT K3 A100 支持
# 无需模型文件，仅检查二进制

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo ""
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${CYAN}  SpacemiT K3 A100 Support Check${NC}"
echo -e "${CYAN}═══════════════════════════════════════════════════════════════${NC}"
echo ""

# 默认路径
LLAMA_DIR="${1:-/opt/rivision/rivision_node/engines/llama.cpp}"
LIB_DIR="${2:-/opt/rivision/rivision_node/lib}"

# 如果部署目录不存在，尝试开发目录
if [ ! -d "$LLAMA_DIR" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    for ver in riscv64-0.0.7a0 riscv64-0.0.7+a2 riscv64-0.0.7+a4 riscv64-current; do
        if [ -d "$SCRIPT_DIR/../third_party/llama.cpp/$ver" ]; then
            LLAMA_DIR="$SCRIPT_DIR/../third_party/llama.cpp/$ver"
            LIB_DIR="$LLAMA_DIR/lib"
            break
        fi
    done
fi

# 如果只传了 LLAMA_DIR，自动设置 LIB_DIR
if [ -n "$1" ] && [ -z "$2" ]; then
    LIB_DIR="$LLAMA_DIR/lib"
fi

echo "检查路径: $LLAMA_DIR"
echo "库路径: $LIB_DIR"
echo ""

# 架构检测
ARCH=$(uname -m)
echo "0. 系统环境"
echo "────────────────────────────────────────────"
echo -e "  架构: $ARCH"
if [ "$ARCH" = "riscv64" ]; then
    # K3 CPU 拓扑检测
    VISIBLE_CORES=$(nproc)
    TOTAL_CORES=$(grep -c ^processor /proc/cpuinfo 2>/dev/null || echo $VISIBLE_CORES)
    echo -e "  系统核心数: $TOTAL_CORES"
    echo -e "  进程可见核心: $VISIBLE_CORES"
    if [ "$TOTAL_CORES" -eq 16 ]; then
        echo -e "  ${GREEN}✓${NC} K3 SoC 检测到 (X100: 0-7, A100: 8-15)"
    fi
else
    echo -e "  ${YELLOW}⚠${NC} 非 K3 平台，仅检查二进制文件"
fi
echo ""

PASS=0
FAIL=0

check() {
    local name="$1"
    local result="$2"
    local expected="$3"
    
    if [ "$result" -ge "$expected" ]; then
        echo -e "  ${GREEN}✓${NC} $name: $result"
        ((PASS++))
    else
        echo -e "  ${RED}✗${NC} $name: $result (需要 >= $expected)"
        ((FAIL++))
    fi
}

echo "1. llama-server 检查"
echo "────────────────────────────────────────────"
# 部署路径直接在 LLAMA_DIR 下，开发路径在 bin/ 下
LLAMA_SERVER="$LLAMA_DIR/llama-server"
[ ! -f "$LLAMA_SERVER" ] && LLAMA_SERVER="$LLAMA_DIR/bin/llama-server"
if [ -f "$LLAMA_SERVER" ]; then
    echo -e "  ${GREEN}✓${NC} llama-server 存在"
    ((PASS++))
    
    # SpacemiT 符号计数
    SPACEMIT=$(strings "$LLAMA_SERVER" 2>/dev/null | grep -c "spacemit" || echo 0)
    check "spacemit 符号" "$SPACEMIT" 1
    
    # NUMA 支持
    NUMA=$(strings "$LLAMA_SERVER" 2>/dev/null | grep -c "llama_numa_init" || echo 0)
    check "numa 支持" "$NUMA" 1
else
    echo -e "  ${RED}✗${NC} llama-server 不存在"
    ((FAIL++))
fi

echo ""
echo "2. libggml-cpu.so 检查"
echo "────────────────────────────────────────────"
LIBGGML="$LIB_DIR/libggml-cpu.so"
if [ -f "$LIBGGML" ] || [ -L "$LIBGGML" ]; then
    LIBGGML_REAL=$(readlink -f "$LIBGGML" 2>/dev/null || echo "$LIBGGML")
    echo -e "  ${GREEN}✓${NC} libggml-cpu.so 存在"
    ((PASS++))
    
    # 检查关键符号
    BIND_AI=$(nm -D "$LIBGGML_REAL" 2>/dev/null | grep -c "bind_ai_thread" || echo 0)
    check "bind_ai_thread (A100绑核)" "$BIND_AI" 1
    
    NUMA_SET=$(nm -D "$LIBGGML_REAL" 2>/dev/null | grep -c "spacemit_set_numa" || echo 0)
    check "spacemit_set_numa_thread_affinity" "$NUMA_SET" 1
    
    SPACEMIT_SYM=$(nm -D "$LIBGGML_REAL" 2>/dev/null | grep -c "spacemit" || echo 0)
    check "spacemit 总符号数" "$SPACEMIT_SYM" 100
else
    echo -e "  ${RED}✗${NC} libggml-cpu.so 不存在"
    ((FAIL++))
fi

echo ""
echo "3. libllama.so 检查"
echo "────────────────────────────────────────────"
LIBLLAMA="$LIB_DIR/libllama.so"
if [ -f "$LIBLLAMA" ] || [ -L "$LIBLLAMA" ]; then
    LIBLLAMA_REAL=$(readlink -f "$LIBLLAMA" 2>/dev/null || echo "$LIBLLAMA")
    VERSION=$(basename "$LIBLLAMA_REAL" | sed 's/libllama.so.//')
    echo -e "  ${GREEN}✓${NC} libllama.so 存在 (版本: $VERSION)"
    ((PASS++))
else
    echo -e "  ${RED}✗${NC} libllama.so 不存在"
    ((FAIL++))
fi

echo ""
echo "════════════════════════════════════════════"
echo "结果: ${GREEN}$PASS 通过${NC}, ${RED}$FAIL 失败${NC}"
echo ""

if [ "$FAIL" -eq 0 ]; then
    echo -e "${GREEN}✓ llama.cpp 包含完整的 SpacemiT K3 A100 支持${NC}"
    echo ""
    echo "  ★ SpacemiT 内部自动绑定 A100 核心 (cpu_mask: ff00)"
    echo "  ★ 不需要外部设置 GOMP_CPU_AFFINITY 或 taskset"
    echo "  ★ 外部强制绑定会导致: libgomp: Thread creation failed"
    exit 0
else
    echo -e "${RED}✗ llama.cpp 缺少 SpacemiT K3 支持${NC}"
    echo ""
    echo "  请确认使用的是 SpacemiT 专用编译版本:"
    echo "    - riscv64-0.0.7a0 (v6 稳定版)"
    echo "    - riscv64-0.0.7+a4 (当前版)"
    exit 1
fi
