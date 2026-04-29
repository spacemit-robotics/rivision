#!/bin/bash
# ============================================================
# RiVision Build Utilities - 统一构建输出格式
# ============================================================
# 用法: source build/scripts/build_utils.sh
#
# 函数:
#   print_header <component> <profile> <platform> <output>
#   print_step <current> <total> <message>
#   print_substep <message>
#   print_success <message>
#   print_error <message>
#   print_complete <output_dir>
#   print_section <title>
# ============================================================

# 颜色定义
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    CYAN=''
    BOLD=''
    NC=''
fi

# 打印组件构建头部
# 参数: <component> <profile> <platform> <output>
print_header() {
    local component="$1"
    local profile="$2"
    local platform="$3"
    local output="$4"
    
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║  📦 Building ${component}"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo "  Profile:  ${profile}"
    echo "  Platform: ${platform}"
    echo "  Output:   ${output}"
    echo ""
}

# 打印发布构建头部
# 参数: <version> <platform> <package_type> <output>
print_release_header() {
    local version="$1"
    local platform="$2"
    local package="$3"
    local output="$4"
    
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║  🚀 RiVision Release Build                                   ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo "  Version:  ${version}"
    echo "  Platform: ${platform}"
    echo "  Package:  ${package}"
    echo "  Output:   ${output}"
    echo ""
}

# 打印分区标题
# 参数: <title>
print_section() {
    local title="$1"
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo " ${title}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# 打印构建步骤
# 参数: <current> <total> <message>
print_step() {
    local current="$1"
    local total="$2"
    local message="$3"
    printf "[%d/%d] %s...\n" "$current" "$total" "$message"
}

# 打印子步骤（缩进）
# 参数: <message>
print_substep() {
    echo "      $1"
}

# 打印成功消息
# 参数: <message>
print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

# 打印错误消息
# 参数: <message>
print_error() {
    echo -e "${RED}✗ Error: $1${NC}"
}

# 打印警告消息
# 参数: <message>
print_warn() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# 打印构建完成摘要
# 参数: <output_dir> [contents_description]
print_complete() {
    local output="$1"
    local contents="$2"
    local size=$(du -sh "$output" 2>/dev/null | cut -f1)
    
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║  ✅ Package Complete                                         ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo "  Output: ${output}"
    echo "  Size:   ${size}"
    
    if [ -n "$contents" ]; then
        echo ""
        echo "  Contents:"
        echo "$contents" | while read line; do
            echo "    $line"
        done
    fi
    echo ""
}

# 打印发布完成摘要
# 参数: <host_dir> <node_dir>
print_release_complete() {
    local host_dir="$1"
    local node_dir="$2"
    
    echo ""
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║  ✅ Release Complete                                         ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo ""
    echo "  Output Packages:"
    echo "  ┌─────────────────────────────────────────────────────────────┐"
    
    if [ -d "$host_dir" ]; then
        local host_size=$(du -sh "$host_dir" 2>/dev/null | cut -f1)
        printf "  │  %-8s %-6s cli, gateway, embed, owl                   │\n" "host/" "$host_size"
    fi
    
    if [ -d "$node_dir" ]; then
        local node_size=$(du -sh "$node_dir" 2>/dev/null | cut -f1)
        printf "  │  %-8s %-6s node-agent, llama.cpp, yolo-server         │\n" "node/" "$node_size"
    fi
    
    echo "  └─────────────────────────────────────────────────────────────┘"
    echo ""
    echo "  Deploy Commands:"
    [ -d "$host_dir" ] && echo "    scp -r host/ target:/tmp/ && cd /tmp/host && ./install.sh"
    [ -d "$node_dir" ] && echo "    scp -r node/ target:/tmp/ && cd /tmp/node && ./install.sh"
    echo ""
}

# 列出目录内容摘要
# 参数: <dir>
list_contents() {
    local dir="$1"
    
    if [ -d "$dir/bin" ]; then
        local bins=$(ls "$dir/bin" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
        echo "bin/     : $bins"
    fi
    
    if [ -d "$dir/config" ]; then
        local configs=$(ls "$dir/config" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
        echo "config/  : $configs"
    fi
    
    if [ -d "$dir/lib" ]; then
        local libs=$(ls "$dir/lib" 2>/dev/null | wc -l)
        echo "lib/     : $libs files"
    fi
    
    if [ -d "$dir/models" ]; then
        local models=$(ls "$dir/models" 2>/dev/null | wc -l)
        echo "models/  : $models files"
    fi
    
    if [ -d "$dir/services" ]; then
        local svcs=$(ls "$dir/services" 2>/dev/null | tr '\n' ', ' | sed 's/,$//')
        echo "services/: $svcs"
    fi
}
