#!/bin/bash
# ============================================================
# 模型切换脚本
# 用法: ./switch-model.sh <model-name>
# 示例: ./switch-model.sh fastvlm-0.5b-q4
#       ./switch-model.sh minicpm-v4.5-q4
#       ./switch-model.sh --list
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
MODELS_CONF="$DEPLOY_DIR/config/models.conf"
ENV_FILE="$DEPLOY_DIR/config/.env"
MODELS_DIR="$DEPLOY_DIR/models"

# 颜色
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

# 列出可用模型
list_models() {
    echo "可用模型:"
    echo "─────────────────────────────────────────"
    while IFS='|' read -r name model mmproj extra prompt; do
        [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
        local status="❌ 缺失"
        if [ "$mmproj" = "smt" ]; then
            # SMT/NPU 模式: 检查模型目录（含 config.json + .gguf + .onnx）
            local model_dir="$MODELS_DIR/$(dirname "$model")"
            if [ -d "$model_dir" ] && [ -f "$MODELS_DIR/$model" ]; then
                local size=$(du -h "$MODELS_DIR/$model" | cut -f1)
                status="✅ $size (NPU)"
            fi
        elif [ -f "$MODELS_DIR/$model" ] && [ -f "$MODELS_DIR/$mmproj" ]; then
            local size=$(du -h "$MODELS_DIR/$model" | cut -f1)
            status="✅ $size"
        fi
        printf "  %-25s %s\n" "$name" "$status"
    done < "$MODELS_CONF"
    echo ""
    
    # 显示当前激活模型
    if [ -f "$DEPLOY_DIR/config/active-model.sh" ]; then
        source "$DEPLOY_DIR/config/active-model.sh"
        echo -e "当前激活: ${GREEN}${ACTIVE_MODEL_NAME}${NC}"
    else
        echo -e "当前激活: ${YELLOW}未配置${NC}"
    fi
}

# 切换模型
switch_model() {
    local target="$1"
    local found=false
    
    while IFS='|' read -r name model mmproj extra prompt; do
        [[ "$name" =~ ^#.*$ || -z "$name" ]] && continue
        if [ "$name" = "$target" ]; then
            found=true
            
            # 检查模型文件
            if [ ! -f "$MODELS_DIR/$model" ]; then
                echo -e "${RED}❌ 模型文件不存在: $MODELS_DIR/$model${NC}"
                exit 1
            fi
            if [ "$mmproj" = "smt" ]; then
                # SMT/NPU 模式: 验证模型目录包含必要文件
                local model_dir="$MODELS_DIR/$(dirname "$model")"
                if [ ! -f "$model_dir/config.json" ]; then
                    echo -e "${YELLOW}⚠ SMT 模式缺少 config.json: $model_dir${NC}"
                fi
            elif [ -n "$mmproj" ] && [ ! -f "$MODELS_DIR/$mmproj" ]; then
                echo -e "${RED}❌ mmproj文件不存在: $MODELS_DIR/$mmproj${NC}"
                exit 1
            fi
            
            # 替换转义空格
            extra="${extra//\\x20/ }"
            prompt="${prompt//\\x20/ }"
            
            # 生成启动配置
            local mmproj_value
            if [ "$mmproj" = "smt" ]; then
                mmproj_value="smt"
            else
                mmproj_value="$MODELS_DIR/$mmproj"
            fi
            cat > "$DEPLOY_DIR/config/active-model.sh" << EOF
# 自动生成 - 勿手动编辑
# 由 switch-model.sh 在 $(date '+%Y-%m-%d %H:%M:%S') 生成
ACTIVE_MODEL_NAME="$name"
ACTIVE_MODEL_FILE="$MODELS_DIR/$model"
ACTIVE_MMPROJ_FILE="$mmproj_value"
ACTIVE_EXTRA_ARGS="$extra"
ACTIVE_PROMPT="$prompt"
EOF
            
            echo -e "${GREEN}✅ 已切换到: $name${NC}"
            echo "   模型: $model"
            if [ "$mmproj" = "smt" ]; then
                echo "   模式: NPU (smt)"
            else
                echo "   投影: $mmproj"
            fi
            [ -n "$extra" ] && echo "   额外参数: $extra"
            [ -n "$prompt" ] && echo "   提示词: $prompt"
            echo ""
            
            # 提示重启服务
            echo "请重启推理服务使配置生效:"
            echo "  sudo systemctl restart rivision-llama"
            
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
    *)
        switch_model "$1"
        ;;
esac
