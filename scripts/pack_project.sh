#!/bin/bash
#
# 项目打包脚本
# 用法: ./scripts/pack_project.sh [输出文件名]
#
# 排除规则:
#   目录: .git, dist, node_modules
#   文件: *.gguf, *.onnx
#

set -e

# 获取脚本所在目录的父目录（项目根目录）
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PROJECT_NAME="$(basename "$PROJECT_ROOT")"

# 默认输出文件名
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_FILE="${1:-${PROJECT_NAME}_${TIMESTAMP}.tar.gz}"

# 如果输出路径不是绝对路径，则放在项目根目录的上一级
if [[ "$OUTPUT_FILE" != /* ]]; then
    OUTPUT_FILE="$(dirname "$PROJECT_ROOT")/$OUTPUT_FILE"
fi

echo "============================================"
echo "项目打包脚本"
echo "============================================"
echo "项目目录: $PROJECT_ROOT"
echo "项目名称: $PROJECT_NAME"
echo "输出文件: $OUTPUT_FILE"
echo ""

# 切换到项目根目录的上一级（这样打包时会包含项目文件夹名）
cd "$(dirname "$PROJECT_ROOT")"

# 排除规则
EXCLUDES=(
    # ========== 顶级目录排除 ==========
    "--exclude=${PROJECT_NAME}/.git"
    "--exclude=${PROJECT_NAME}/dist"
    "--exclude=${PROJECT_NAME}/build"
    "--exclude=${PROJECT_NAME}/tools"
    
    # ========== 通用目录排除 ==========
    "--exclude=node_modules"
    "--exclude=__pycache__"
    "--exclude=.pytest_cache"
    "--exclude=.mypy_cache"
    "--exclude=.venv"
    "--exclude=venv"
    
    # ========== 构建产物目录 ==========
    "--exclude=*/third_party"
    "--exclude=*/build-*"
    "--exclude=*/release"
    "--exclude=*/models"
    "--exclude=*/dist"
    
    # ========== 文件类型排除 ==========
    "--exclude=*.gguf"
    "--exclude=*.onnx"
    "--exclude=*.bin"
    "--exclude=*.so"
    "--exclude=*.a"
    "--exclude=*.o"
    "--exclude=*.pyc"
    "--exclude=*.log"
    "--exclude=*.tar.gz"
    "--exclude=*.zip"
    "--exclude=.DS_Store"
)

echo "排除规则:"
echo "  顶级目录: .git, dist, build, tools"
echo "  通用目录: node_modules, __pycache__, third_party, models"
echo "  构建目录: build-*, release, */dist"
echo "  文件类型: *.gguf, *.onnx, *.bin, *.so, *.a, *.o"
echo ""

# 统计排除前的大小
echo "统计项目大小..."
TOTAL_SIZE=$(du -sh "$PROJECT_NAME" 2>/dev/null | cut -f1)
echo "  原始大小: $TOTAL_SIZE"

# 执行打包
echo ""
echo "开始打包..."
tar "${EXCLUDES[@]}" -czvf "$OUTPUT_FILE" "$PROJECT_NAME" 2>&1 | tail -20

# 显示结果
if [ -f "$OUTPUT_FILE" ]; then
    PACKED_SIZE=$(du -sh "$OUTPUT_FILE" | cut -f1)
    echo ""
    echo "============================================"
    echo "✓ 打包完成!"
    echo "============================================"
    echo "  输出文件: $OUTPUT_FILE"
    echo "  压缩大小: $PACKED_SIZE"
    echo ""
    echo "解压命令:"
    echo "  tar -xzvf $(basename "$OUTPUT_FILE")"
else
    echo "✗ 打包失败!"
    exit 1
fi
