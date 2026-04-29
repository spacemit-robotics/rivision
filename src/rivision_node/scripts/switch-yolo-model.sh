#!/bin/bash
# ============================================================
# YOLO 模型切换脚本
# 用法: ./switch-yolo-model.sh <model-name>
# 示例: ./switch-yolo-model.sh yolov8n
#       ./switch-yolo-model.sh yolo11s
#       ./switch-yolo-model.sh --list
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
MODELS_CONF="$DEPLOY_DIR/config/yolo-models.conf"
MODELS_DIR="$DEPLOY_DIR/models"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

# 列出可用模型
list_models() {
    echo "可用 YOLO 模型:"
    echo "─────────────────────────────────────────────────────────"
    printf "  %-15s %-20s %-10s %s\n" "名称" "文件" "状态" "描述"
    echo "─────────────────────────────────────────────────────────"
    while IFS='|' read -r name model input_size conf_thresh nms_thresh desc; do
        [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
        local status="❌ 缺失"
        if [ -f "$MODELS_DIR/$model" ]; then
            local size=$(du -h "$MODELS_DIR/$model" | cut -f1)
            status="✅ $size"
        fi
        printf "  %-15s %-20s %-10s %s\n" "$name" "$model" "$status" "$desc"
    done < "$MODELS_CONF"
    echo ""
    
    # 显示当前激活模型
    if [ -f "$DEPLOY_DIR/config/active-yolo-model.sh" ]; then
        source "$DEPLOY_DIR/config/active-yolo-model.sh"
        echo -e "当前激活: ${GREEN}${ACTIVE_YOLO_NAME}${NC} (${ACTIVE_YOLO_FILE})"
    else
        echo -e "当前激活: ${YELLOW}未配置 (使用默认 yolov8n)${NC}"
    fi
}

# 下载模型
download_model() {
    local model_file="$1"
    local model_name="${model_file%.onnx}"
    
    echo "正在下载 $model_file..."
    
    # 判断模型系列
    if [[ "$model_name" == yolov8* ]]; then
        # YOLOv8 系列
        local url="https://github.com/ultralytics/assets/releases/download/v8.3.0/${model_file}"
    elif [[ "$model_name" == yolo11* ]]; then
        # YOLO11 系列
        local url="https://github.com/ultralytics/assets/releases/download/v8.3.0/${model_file}"
    else
        echo -e "${RED}❌ 未知模型系列: $model_name${NC}"
        return 1
    fi
    
    if command -v wget &>/dev/null; then
        wget -q --show-progress -O "$MODELS_DIR/$model_file" "$url"
    elif command -v curl &>/dev/null; then
        curl -L -o "$MODELS_DIR/$model_file" "$url"
    else
        echo -e "${RED}❌ 需要 wget 或 curl${NC}"
        return 1
    fi
    
    if [ -f "$MODELS_DIR/$model_file" ]; then
        echo -e "${GREEN}✅ 下载完成: $model_file${NC}"
    else
        echo -e "${RED}❌ 下载失败${NC}"
        return 1
    fi
}

# 切换模型
switch_model() {
    local target="$1"
    local found=false
    
    while IFS='|' read -r name model input_size conf_thresh nms_thresh desc; do
        [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
        if [ "$name" = "$target" ]; then
            found=true
            
            # 检查模型文件
            if [ ! -f "$MODELS_DIR/$model" ]; then
                echo -e "${YELLOW}⚠️ 模型文件不存在: $MODELS_DIR/$model${NC}"
                read -p "是否下载? [Y/n] " -n 1 -r
                echo ""
                if [[ ! $REPLY =~ ^[Nn]$ ]]; then
                    download_model "$model" || exit 1
                else
                    exit 1
                fi
            fi
            
            # 生成启动配置
            cat > "$DEPLOY_DIR/config/active-yolo-model.sh" << EOF
# 自动生成 - 勿手动编辑
# 由 switch-yolo-model.sh 在 $(date '+%Y-%m-%d %H:%M:%S') 生成
ACTIVE_YOLO_NAME="$name"
ACTIVE_YOLO_FILE="$MODELS_DIR/$model"
ACTIVE_YOLO_INPUT_SIZE="$input_size"
ACTIVE_YOLO_CONF_THRESH="$conf_thresh"
ACTIVE_YOLO_NMS_THRESH="$nms_thresh"
ACTIVE_YOLO_DESC="$desc"
EOF
            
            echo -e "${GREEN}✅ 已切换到: $name${NC}"
            echo "   模型: $model"
            echo "   输入尺寸: ${input_size}x${input_size}"
            echo "   置信度阈值: $conf_thresh"
            echo "   NMS阈值: $nms_thresh"
            echo "   描述: $desc"
            echo ""
            
            # 提示重启服务
            echo "请重启推理服务使配置生效:"
            echo "  sudo systemctl restart rivision-yolo"
            
            break
        fi
    done < "$MODELS_CONF"
    
    if ! $found; then
        echo -e "${RED}❌ 未知模型: $target${NC}"
        echo ""
        list_models
        exit 1
    fi
}

# 主入口
case "${1:-}" in
    --list|-l|"")
        list_models
        ;;
    --download|-d)
        if [ -z "$2" ]; then
            echo "用法: $0 --download <model-file>"
            exit 1
        fi
        download_model "$2"
        ;;
    *)
        switch_model "$1"
        ;;
esac
