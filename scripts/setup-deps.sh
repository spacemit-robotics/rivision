#!/bin/bash
# scripts/setup-deps.sh — 从 archive.spacemit.com 下载模型和视频文件
#
# 用法:
#   ./scripts/setup-deps.sh           # 下载全部
#   ./scripts/setup-deps.sh --models  # 仅模型
#   ./scripts/setup-deps.sh --videos  # 仅视频
#   ./scripts/setup-deps.sh --dry-run # 预览（不实际下载）
#   ./scripts/setup-deps.sh --help    # 帮助
#
# 前提: K3 上已安装 SpaceMIT AI SDK（ORT + llama.cpp 系统自带）
set -e

# === 配置 ===
BASE_URL="https://archive.spacemit.com/spacemit-ai/model_zoo"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# curl 公共参数: -k 跳过自签名证书验证, -L 跟随重定向, -f 失败时返回错误码
CURL_OPTS="-kLf --connect-timeout 15 --retry 3 --retry-delay 5"

# 默认全部下载
DO_MODELS=true
DO_VIDEOS=true
DRY_RUN=false

for arg in "$@"; do
    case "$arg" in
        --models)  DO_MODELS=true; DO_VIDEOS=false ;;
        --videos)  DO_MODELS=false; DO_VIDEOS=true ;;
        --dry-run) DRY_RUN=true ;;
        --help|-h)
            echo "用法: $0 [--models] [--videos] [--dry-run]"
            echo ""
            echo "选项:"
            echo "  --models   仅下载模型文件"
            echo "  --videos   仅下载视频文件"
            echo "  --dry-run  预览下载列表（不实际下载）"
            echo ""
            echo "下载源: $BASE_URL"
            exit 0
            ;;
    esac
done

log()  { echo -e "\033[32m[setup]\033[0m $1"; }
warn() { echo -e "\033[33m[warn]\033[0m $1"; }
err()  { echo -e "\033[31m[error]\033[0m $1"; }

# 下载单个文件: download_file <url> <dest>
download_file() {
    local url="$1" dest="$2"
    local filename
    filename=$(basename "$dest")

    if [ -f "$dest" ]; then
        # 已存在则跳过（断点续传场景可改为 -C -）
        log "  [skip] $filename (已存在)"
        return 0
    fi

    if [ "$DRY_RUN" = true ]; then
        log "  [dry-run] $url -> $dest"
        return 0
    fi

    mkdir -p "$(dirname "$dest")"
    log "  [download] $filename ..."
    if curl $CURL_OPTS --progress-bar -o "$dest" "$url"; then
        log "  [ok] $filename ($(du -h "$dest" | cut -f1))"
    else
        err "  [fail] $filename"
        rm -f "$dest"
        return 1
    fi
}

# 下载 tar.gz 并解压: download_tarball <url> <extract_dir>
download_tarball() {
    local url="$1" extract_dir="$2"
    local filename
    filename=$(basename "$url")

    if [ "$DRY_RUN" = true ]; then
        log "  [dry-run] $url -> $extract_dir/"
        return 0
    fi

    mkdir -p "$extract_dir"
    log "  [download+extract] $filename -> $extract_dir/"
    if curl $CURL_OPTS --progress-bar "$url" | tar xz -C "$extract_dir"; then
        log "  [ok] $filename"
    else
        err "  [fail] $filename"
        return 1
    fi
}

# === 连通性检查 ===
log "检查下载源: $BASE_URL"
if ! curl $CURL_OPTS -s -o /dev/null "$BASE_URL/"; then
    err "下载源不可达: $BASE_URL"
    err "请检查网络连接"
    exit 1
fi
log "下载源可达"
echo ""

# ============================================================
# 模型文件
# ============================================================
if [ "$DO_MODELS" = true ]; then
    log "=== 下载模型文件 ==="

    # --- rivision_node: YOLO + VLM ---
    log "[rivision_node] YOLO 检测模型"
    download_file "$BASE_URL/vision/yolov8/yolov8n.onnx" \
        "$ROOT/src/rivision_node/models/shared/yolov8n.onnx"
    download_file "$BASE_URL/vision/yolov11/910byolo11n_b2.q.onnx" \
        "$ROOT/src/rivision_node/models/shared/910byolo11n_b2.q.onnx"

    log "[rivision_node] VLM 推理模型 (fastvlm)"
    FASTVLM_DIR="$ROOT/src/rivision_node/models/riscv_b/fastvlm-mm-0.5b-q4_1"
    if [ -d "$FASTVLM_DIR" ] && [ -f "$FASTVLM_DIR/fastvlm-text-0.5B-Q4_1.gguf" ]; then
        log "  [skip] fastvlm-mm-0.5b-q4_1 (已存在)"
    else
        download_tarball "$BASE_URL/vlm/fastvlm-mm-0.5b-q4_1.tar.gz" \
            "$ROOT/src/rivision_node/models/riscv_b"
    fi

    log "[rivision_node] VLM 推理模型 (Qwen3VL-4B)"
    QWEN3VL_DIR="$ROOT/src/rivision_node/models/shared/Qwen3VL"
    download_file "$BASE_URL/vlm/Qwen3VL/Qwen3VL-4B-Instruct-Q4_K_M.gguf" \
        "$QWEN3VL_DIR/Qwen3VL-4B-Instruct-Q4_K_M.gguf"
    download_file "$BASE_URL/vlm/Qwen3VL/mmproj-Qwen3VL-4B-Instruct-F16.gguf" \
        "$QWEN3VL_DIR/mmproj-Qwen3VL-4B-Instruct-F16.gguf"

    # --- rivision_embed: CLIP 向量模型 ---
    log "[rivision_embed] CLIP 向量模型"
    download_file "$BASE_URL/embed/cn_clip_text.onnx" \
        "$ROOT/src/rivision_embed/models/embed/cn_clip_text.onnx"
    download_file "$BASE_URL/embed/cn_clip_vision.onnx" \
        "$ROOT/src/rivision_embed/models/embed/cn_clip_vision.onnx"

    # --- rivision_cli: YOLO 检测模型 ---
    log "[rivision_cli] YOLO 检测模型"
    download_file "$BASE_URL/vision/yolov8/yolov8n.onnx" \
        "$ROOT/src/rivision_cli/models/yolov8n.onnx"

    echo ""
fi

# ============================================================
# 视频文件
# ============================================================
if [ "$DO_VIDEOS" = true ]; then
    log "=== 下载视频文件 ==="

    log "[rivision_cli] 演示视频"
    for f in car_v1.mp4 persion_v1.mp4 road_v1.mp4 shopping_v1.mp4; do
        download_file "$BASE_URL/assets/video/rivision/rivision_cli/$f" \
            "$ROOT/src/rivision_cli/mp4path/$f"
    done

    log "[rivision_owl] 测试视频"
    for f in cam1.mp4 cam2.mp4; do
        download_file "$BASE_URL/assets/video/rivision/rivision_owl/$f" \
            "$ROOT/src/rivision_owl/mp4path/$f"
    done

    echo ""
fi

# ============================================================
# Git submodule
# ============================================================
log "=== 初始化 git submodule ==="
cd "$ROOT"
git submodule update --init --recursive 2>/dev/null || warn "git submodule 初始化失败（可能不在 git 仓库中）"

echo ""
log "完成！"
echo ""
log "下一步:"
log "  1. 确认 K3 已安装 SpaceMIT AI SDK (ORT + llama.cpp)"
log "  2. 确认 Go 1.25+ 已安装: go version"
log "  3. 构建: cd build && make release"
