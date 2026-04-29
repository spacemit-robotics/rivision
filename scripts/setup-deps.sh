#!/bin/bash
# scripts/setup-deps.sh — 从 rsync 服务器下载模型和视频文件
#
# 用法:
#   ./scripts/setup-deps.sh           # 下载全部
#   ./scripts/setup-deps.sh --models  # 仅模型
#   ./scripts/setup-deps.sh --videos  # 仅视频
#   ./scripts/setup-deps.sh --dry-run # 预览（不实际下载）
#
# 前提: K3 上已安装 SpaceMIT AI SDK（ORT + llama.cpp 系统自带）
set -e

RSYNC_PASSWORD="6n0Gfv2rfoumip7y"
export RSYNC_PASSWORD
REMOTE="ai@10.0.50.55::spacemit-ai/rivision"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# 默认全部下载
DO_MODELS=true
DO_VIDEOS=true
DRY_RUN=""

for arg in "$@"; do
    case "$arg" in
        --models)  DO_MODELS=true; DO_VIDEOS=false ;;
        --videos)  DO_MODELS=false; DO_VIDEOS=true ;;
        --dry-run) DRY_RUN="-n" ;;
        --help)
            echo "用法: $0 [--models] [--videos] [--dry-run]"
            exit 0
            ;;
    esac
done

log() { echo -e "\033[32m[setup]\033[0m $1"; }
warn() { echo -e "\033[33m[warn]\033[0m $1"; }

# 检查 rsync 连通性
if ! rsync $DRY_RUN --timeout=5 "$REMOTE/" >/dev/null 2>&1; then
    warn "rsync 服务器不可达: $REMOTE"
    warn "请检查网络或 VPN 连接"
    exit 1
fi

if [ "$DO_MODELS" = true ]; then
    log "=== 下载模型文件 ==="
    mkdir -p "$ROOT/src/rivision_node/models"
    mkdir -p "$ROOT/src/rivision_embed/models"
    mkdir -p "$ROOT/src/rivision_cli/models"
    rsync -avP $DRY_RUN "$REMOTE/models/rivision_node/" "$ROOT/src/rivision_node/models/"
    rsync -avP $DRY_RUN "$REMOTE/models/rivision_embed/" "$ROOT/src/rivision_embed/models/"
    rsync -avP $DRY_RUN "$REMOTE/models/rivision_cli/" "$ROOT/src/rivision_cli/models/"
fi

if [ "$DO_VIDEOS" = true ]; then
    log "=== 下载视频文件 ==="
    mkdir -p "$ROOT/src/rivision_cli/mp4path"
    mkdir -p "$ROOT/src/rivision_owl/mp4path"
    rsync -avP $DRY_RUN "$REMOTE/mp4path/rivision_cli/" "$ROOT/src/rivision_cli/mp4path/"
    rsync -avP $DRY_RUN "$REMOTE/mp4path/rivision_owl/" "$ROOT/src/rivision_owl/mp4path/"
fi

log "=== 初始化 git submodule ==="
cd "$ROOT"
git submodule update --init --recursive 2>/dev/null || warn "git submodule 初始化失败（可能不在 git 仓库中）"

echo ""
log "完成！"
log ""
log "下一步:"
log "  1. 确认 K3 已安装 SpaceMIT AI SDK (ORT + llama.cpp)"
log "  2. 确认 Go 1.25+ 已安装: go version"
log "  3. 构建: cd build && make release-riscv64"
