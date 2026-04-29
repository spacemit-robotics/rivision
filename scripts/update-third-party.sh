#!/bin/bash
# 第三方依赖版本更新脚本
# 用法: ./scripts/update-third-party.sh [--dry-run] [--no-rebuild-yolo]
#
# 功能:
#   1. 解压指定版本的 tar.gz
#   2. 创建 "current" 符号链接
#   3. 同步到其他模块 (rivision_embed)
#   4. 更新配置文件中的版本引用

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
NODE_DIR="$ROOT_DIR/src/rivision_node"
EMBED_DIR="$ROOT_DIR/src/rivision_embed"
VERSION_FILE="$NODE_DIR/third_party.versions"

DRY_RUN=false
REBUILD_YOLO=true

while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run)
            DRY_RUN=true
            ;;
        --no-rebuild-yolo)
            REBUILD_YOLO=false
            ;;
        --rebuild-yolo)
            REBUILD_YOLO=true
            ;;
        *)
            echo "ERROR: Unknown argument: $1"
            echo "Usage: $0 [--dry-run] [--no-rebuild-yolo]"
            exit 1
            ;;
    esac
    shift
done

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[update]${NC} $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }

# 加载版本配置
if [ ! -f "$VERSION_FILE" ]; then
    echo "ERROR: Version file not found: $VERSION_FILE"
    exit 1
fi
source "$VERSION_FILE"

log "版本配置:"
log "  SPACEMIT_ORT: $SPACEMIT_ORT_VERSION"
log "  LLAMA_CPP:    $LLAMA_CPP_VERSION"
log "  REBUILD_YOLO: $REBUILD_YOLO"
echo ""

# === 1. SpaceMIT ONNX Runtime ===
ORT_ARCHIVE_PATH="$NODE_DIR/yolo-server/third_party/spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}.tar.gz"
ORT_DIR="$NODE_DIR/yolo-server/third_party/spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}"
ORT_CURRENT="$NODE_DIR/yolo-server/third_party/spacemit-ort-current"
ORT_PREV_TARGET=""
ORT_LINK_CHANGED=false

if [ -L "$ORT_CURRENT" ]; then
    ORT_PREV_TARGET="$(readlink "$ORT_CURRENT")"
fi

if [ -f "$ORT_ARCHIVE_PATH" ]; then
    log "处理 SpaceMIT ONNX Runtime ${SPACEMIT_ORT_VERSION}..."
    
    # 解压 (如果目录不存在)
    if [ ! -d "$ORT_DIR" ]; then
        log "  解压: $ORT_ARCHIVE_PATH"
        $DRY_RUN || tar -xzf "$ORT_ARCHIVE_PATH" -C "$(dirname "$ORT_ARCHIVE_PATH")"
    else
        log "  目录已存在: $ORT_DIR"
    fi
    
    # 创建符号链接
    log "  创建符号链接: spacemit-ort-current -> spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}"
    if [ "$ORT_PREV_TARGET" != "spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}" ]; then
        ORT_LINK_CHANGED=true
    fi
    $DRY_RUN || (rm -f "$ORT_CURRENT" && ln -s "spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}" "$ORT_CURRENT")
    
    # 同步到 rivision_embed
    EMBED_ORT_DIR="$EMBED_DIR/third_party/spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}"
    EMBED_ORT_CURRENT="$EMBED_DIR/third_party/spacemit-ort-current"
    if [ ! -d "$EMBED_ORT_DIR" ]; then
        log "  同步到 rivision_embed..."
        $DRY_RUN || cp -r "$ORT_DIR" "$EMBED_ORT_DIR"
    fi
    $DRY_RUN || (rm -f "$EMBED_ORT_CURRENT" && ln -s "spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}" "$EMBED_ORT_CURRENT")
else
    warn "SpaceMIT ORT 压缩包未找到: $ORT_ARCHIVE_PATH"
fi

# === 2. llama.cpp ===
LLAMA_ARCHIVE_PATH="$NODE_DIR/third_party/llama.cpp/llama.cpp.${LLAMA_CPP_VERSION}.tar.gz"
LLAMA_EXTRACTED="$NODE_DIR/third_party/llama.cpp/llama.cpp.${LLAMA_CPP_VERSION}"
LLAMA_DIR="$NODE_DIR/third_party/llama.cpp/riscv64-${LLAMA_CPP_VERSION}"
LLAMA_CURRENT="$NODE_DIR/third_party/llama.cpp/riscv64-current"

if [ -f "$LLAMA_ARCHIVE_PATH" ]; then
    log "处理 llama.cpp ${LLAMA_CPP_VERSION}..."
    
    # 解压 (如果目录不存在)
    if [ ! -d "$LLAMA_DIR" ]; then
        log "  解压: $LLAMA_ARCHIVE_PATH"
        $DRY_RUN || tar -xzf "$LLAMA_ARCHIVE_PATH" -C "$(dirname "$LLAMA_ARCHIVE_PATH")"
        # 重命名为标准格式
        if [ -d "$LLAMA_EXTRACTED" ] && [ ! -d "$LLAMA_DIR" ]; then
            log "  重命名: llama.cpp.${LLAMA_CPP_VERSION} -> riscv64-${LLAMA_CPP_VERSION}"
            $DRY_RUN || mv "$LLAMA_EXTRACTED" "$LLAMA_DIR"
        fi
    else
        log "  目录已存在: $LLAMA_DIR"
    fi
    
    # 创建符号链接
    log "  创建符号链接: riscv64-current -> riscv64-${LLAMA_CPP_VERSION}"
    $DRY_RUN || (rm -f "$LLAMA_CURRENT" && ln -s "riscv64-${LLAMA_CPP_VERSION}" "$LLAMA_CURRENT")
else
    warn "llama.cpp 压缩包未找到: $LLAMA_ARCHIVE_PATH"
fi

if [ "$REBUILD_YOLO" = true ]; then
    if [ "$ORT_LINK_CHANGED" = true ]; then
        log "检测到 ORT current 版本切换: ${ORT_PREV_TARGET:-<none>} -> spacemit-ort.riscv64.${SPACEMIT_ORT_VERSION}"
        if [ "$DRY_RUN" = true ]; then
            log "[dry-run] 将执行: make -C $NODE_DIR build-yolo-riscv64"
        else
            log "触发 yolo-server 重新编译..."
            make -C "$NODE_DIR" build-yolo-riscv64
        fi
    else
        log "ORT current 未变化，跳过 yolo-server 重编译"
    fi
else
    log "已禁用 yolo-server 重编译（--no-rebuild-yolo）"
fi

echo ""
log "完成! 符号链接状态:"
ls -la "$NODE_DIR/yolo-server/third_party/" 2>/dev/null | grep "current" || true
ls -la "$NODE_DIR/third_party/llama.cpp/" 2>/dev/null | grep "current" || true
ls -la "$EMBED_DIR/third_party/" 2>/dev/null | grep "current" || true
