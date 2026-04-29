#!/bin/bash
set -e

DIST_DIR=$1

if [ -z "$DIST_DIR" ]; then
    echo "Usage: $0 <dist_dir>"
    exit 1
fi

cd "$DIST_DIR"
echo "Generating checksums..."

# 生成所有可执行文件和库的校验和
# 支持独立包结构: host/ 或 node/ 或单组件
{
    # host 独立包结构
    [ -d bin ] && find bin -type f -exec sha256sum {} \; 2>/dev/null || true
    [ -d owl/bin ] && find owl/bin -type f -exec sha256sum {} \; 2>/dev/null || true
    [ -d lib ] && find lib -name "*.so*" -exec sha256sum {} \; 2>/dev/null || true
    
    # node 独立包结构 (rivision_node 打包后的结构)
    [ -d engines ] && find engines -type f -exec sha256sum {} \; 2>/dev/null || true
} > checksums.txt

echo "Checksums saved to checksums.txt ($(wc -l < checksums.txt) entries)"
