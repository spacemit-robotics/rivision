#!/bin/bash
# package_profile.sh — YAML 驱动的 rivision_node 打包脚本
# 新增 profile 只需创建 profiles/xxx.yaml，无需修改此脚本
# dist 中只包含二进制、.so 库、模型、配置和脚本，不包含任何源代码
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE=${1:-riscv_b}
DIST_DIR=${2:-"$SCRIPT_DIR/dist/$PROFILE"}

# --- YAML 解析器 ---
# 从 profile YAML 中读取 key=value（忽略注释和嵌套结构）
parse_yaml() {
    local file=$1
    grep -E '^[a-z_]+:' "$file" | while IFS= read -r line; do
        local key=$(echo "$line" | sed 's/:.*//')
        local val=$(echo "$line" | sed 's/^[^:]*: *//;s/^ *//;s/ *$//;s/^"//;s/"$//')
        # 值用双引号包裹，防止空格分隔的路径被 eval 错误解析
        echo "${key}=\"${val}\""
    done
}

# --- 加载 profile ---
PROFILE_FILE="$SCRIPT_DIR/profiles/$PROFILE.yaml"
if [ ! -f "$PROFILE_FILE" ]; then
    echo "ERROR: Profile not found: $PROFILE_FILE"
    echo "Available profiles:"
    ls "$SCRIPT_DIR/profiles/"*.yaml 2>/dev/null | xargs -I{} basename {} .yaml
    exit 1
fi

# 加载所有 key=value 到变量
eval "$(parse_yaml "$PROFILE_FILE")"

echo "=== Packaging rivision_node ==="
echo "  Profile:  $name"
echo "  Platform: $platform (GOARCH=$goarch)"
echo "  Output:   $DIST_DIR"
echo ""

# --- 清理并创建目录 ---
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"/{bin,lib,models,config,scripts,services}
# 仅在需要 llama 时创建 engines/llama.cpp
if [ "$llama_type" != "skip" ]; then
    mkdir -p "$DIST_DIR/engines/llama.cpp"
fi
cd "$SCRIPT_DIR"

# --- [1/6] 构建 node-agent（Go 跨平台编译）---
echo "[1/6] Building node-agent..."
cd node-agent
GOOS=linux GOARCH=$goarch CGO_ENABLED=0 go build -ldflags "-s -w" -o "$DIST_DIR/bin/rivision_node" .
cd "$SCRIPT_DIR"

# --- [2/6] 复制 yolo-server 预编译二进制 ---
echo "[2/6] Copying yolo-server binary..."
if [ "$yolo_bin" = "skip" ]; then
    echo "  跳过 yolo-server (仅 llama 模式)"
elif [ -n "$yolo_bin" ] && [ -f "$yolo_bin" ]; then
    cp "$yolo_bin" "$DIST_DIR/bin/yolo-server"
    chmod +x "$DIST_DIR/bin/yolo-server"
elif [ -n "$yolo_bin_fallback" ] && [ -f "$yolo_bin_fallback" ]; then
    cp "$yolo_bin_fallback" "$DIST_DIR/bin/yolo-server"
    chmod +x "$DIST_DIR/bin/yolo-server"
else
    echo "  WARNING: yolo-server binary not found: $yolo_bin"
    echo "  在目标平台编译后放到: $yolo_bin"
fi

# --- [3/6] 复制 onnxruntime 运行时库（仅 .so）---
echo "[3/6] Copying onnxruntime libraries..."
if [ "$yolo_bin" = "skip" ]; then
    echo "  跳过 onnxruntime (仅 llama 模式，无需 ORT)"
elif [ -n "$ort_lib_dir" ] && [ -d "$ort_lib_dir" ]; then
    if [ -n "$ort_lib_glob" ]; then
        find "$ort_lib_dir" -maxdepth 1 -name "$ort_lib_glob" -exec cp {} "$DIST_DIR/lib/" \;
    else
        find "$ort_lib_dir" -maxdepth 1 -name "*.so*" -exec cp {} "$DIST_DIR/lib/" \;
    fi
    # 额外运行时库（如 spine-tcm）
    if [ -n "$ort_extra_lib" ] && [ -d "$ort_extra_lib" ]; then
        find "$ort_extra_lib" -maxdepth 1 -name "*.so*" -exec cp {} "$DIST_DIR/lib/" \;
    fi
fi

# --- [4/6] 复制 llama.cpp（仅 bin + lib，不复制源码）---
echo "[4/6] Copying llama.cpp binaries..."
if [ "$llama_type" = "skip" ]; then
    echo "  跳过 llama.cpp"
elif [ "$llama_type" = "prebuilt" ]; then
    # 预编译结构：直接复制 bin/ 和 lib/
    if [ -d "$llama_source/bin" ]; then
        find "$llama_source/bin" -maxdepth 1 -type f \
            ! -name "*.py" ! -name "test-*" \
            -exec cp {} "$DIST_DIR/engines/llama.cpp/" \;
    fi
    if [ -d "$llama_source/lib" ]; then
        find "$llama_source/lib" -maxdepth 1 -name "*.so*" \
            -exec cp {} "$DIST_DIR/lib/" \;
    fi
elif [ "$llama_type" = "source_repo" ]; then
    # 完整源码仓库：只提取 build 目录下的编译产物
    local_build_dir="$llama_source/$llama_build_dir"
    if [ -d "$local_build_dir/bin" ]; then
        find "$local_build_dir/bin" -maxdepth 1 -type f -name "llama-*" \
            -exec cp {} "$DIST_DIR/engines/llama.cpp/" \;
    fi
    # 也从顶层 bin/ 复制（排除 .py 和 test-*）
    if [ -d "$llama_source/bin" ]; then
        find "$llama_source/bin" -maxdepth 1 -type f \
            ! -name "*.py" ! -name "test-*" \
            -exec cp {} "$DIST_DIR/engines/llama.cpp/" \;
    fi
    # 复制 .so 库
    if [ -d "$local_build_dir" ]; then
        find "$local_build_dir" -name "*.so*" -exec cp {} "$DIST_DIR/lib/" \; 2>/dev/null || true
    fi
fi
chmod +x "$DIST_DIR/engines/llama.cpp/"* 2>/dev/null || true

# --- [5/6] 复制模型 ---
# 支持环境变量控制:
#   SKIP_MODELS=1           - 跳过所有模型
#   SKIP_OPTIONAL_MODELS=1  - 跳过可选模型 (model_optional)
#   MODEL_EXTRA="path1 path2" - 添加额外模型
echo "[5/6] Copying models..."
if [ "${SKIP_MODELS:-}" = "1" ]; then
    echo "  跳过所有模型 (SKIP_MODELS=1)"
else
    # 默认模型
    if [ -n "$model_default" ] && [ -d "$model_default" ]; then
        cp -r "$model_default" "$DIST_DIR/models/"
        echo "  + $(basename $model_default) (默认)"
    fi
    # 可选模型
    if [ "${SKIP_OPTIONAL_MODELS:-}" = "1" ]; then
        echo "  跳过可选模型 (SKIP_OPTIONAL_MODELS=1)"
    elif [ -n "$model_optional" ]; then
        for opt_model in $model_optional; do
            if [ -d "$opt_model" ]; then
                cp -r "$opt_model" "$DIST_DIR/models/"
                echo "  + $(basename $opt_model) (可选)"
            fi
        done
    fi
    # YOLO 模型（空格分隔的多个路径）
    if [ -n "$model_yolo" ]; then
        for yolo_model in $model_yolo; do
            if [ -f "$yolo_model" ]; then
                cp "$yolo_model" "$DIST_DIR/models/"
                echo "  + $(basename $yolo_model) (YOLO)"
            fi
        done
    fi
    # 额外模型 (环境变量)
    if [ -n "${MODEL_EXTRA:-}" ]; then
        for extra in $MODEL_EXTRA; do
            if [ -e "$extra" ]; then
                cp -r "$extra" "$DIST_DIR/models/"
                echo "  + $(basename $extra) (额外)"
            fi
        done
    fi
fi

# --- [6/6] 复制配置和脚本 (根据 profile 过滤) ---
echo "[6/6] Copying config and scripts..."

# 通用配置 (排除 yolo/llama 专用配置)
for f in config/*; do
    fname=$(basename "$f")
    # 跳过 yolo 专用配置 (如果 yolo_bin=skip)
    if [ "$yolo_bin" = "skip" ]; then
        case "$fname" in yolo*) continue ;; esac
    fi
    cp -r "$f" "$DIST_DIR/config/" 2>/dev/null || true
done

# 通用脚本 (根据 profile 过滤)
for f in scripts/*.sh; do
    fname=$(basename "$f")
    # 跳过 yolo 脚本 (如果 yolo_bin=skip)
    if [ "$yolo_bin" = "skip" ]; then
        case "$fname" in *yolo*|*YOLO*) continue ;; esac
    fi
    # 跳过 llama 脚本 (如果 llama_type=skip)
    if [ "$llama_type" = "skip" ]; then
        case "$fname" in *llama*|*model*) continue ;; esac
    fi
    cp "$f" "$DIST_DIR/scripts/" 2>/dev/null || true
done

# 服务文件 (根据 profile 过滤)
for f in services/*; do
    fname=$(basename "$f")
    # 跳过 yolo 服务 (如果 yolo_bin=skip)
    if [ "$yolo_bin" = "skip" ]; then
        case "$fname" in *yolo*|*YOLO*) continue ;; esac
    fi
    # 跳过 llama 服务 (如果 llama_type=skip)
    if [ "$llama_type" = "skip" ]; then
        case "$fname" in *llama*|*LLAMA*) continue ;; esac
    fi
    cp -r "$f" "$DIST_DIR/services/" 2>/dev/null || true
done

# 生成默认模型配置（Qwen3VL）
if [ -f "$DIST_DIR/models/Qwen3VL/Qwen3VL-4B-Instruct-Q4_K_M.gguf" ]; then
    cat > "$DIST_DIR/config/active-model.sh" << 'EOF'
# 自动生成 - 默认模型配置
# 可通过 scripts/switch-model.sh 切换
ACTIVE_MODEL_NAME="Qwen3VL"
ACTIVE_MODEL_FILE="$DEPLOY_DIR/models/Qwen3VL/Qwen3VL-4B-Instruct-Q4_K_M.gguf"
ACTIVE_MMPROJ_FILE="$DEPLOY_DIR/models/Qwen3VL/mmproj-Qwen3VL-4B-Instruct-F16.gguf"
ACTIVE_EXTRA_ARGS="--cache-type-k q8_0 --cache-type-v q8_0"
ACTIVE_PROMPT="简要描述图片内容"
EOF
    echo "  默认模型: Qwen3VL"
fi

# --- 最终验证：确保 dist 中无源代码 ---
echo ""
echo "Verifying no source code in dist..."
VIOLATIONS=$(find "$DIST_DIR" -type f \( \
    -name "*.cpp" -o -name "*.cc" -o -name "*.c" -o -name "*.h" \
    -o -name "*.go" -o -name "*.py" \
    -o -name "*.cmake" -o -name "CMakeLists.txt" -o -name "CMakeCache.txt" \
    -o -name "Makefile" -o -name "*.mk" \
    -o -name "go.mod" -o -name "go.sum" \
    -o -name "*.o" -o -name "*.d" \
    -o -name "README.md" -o -name "CONTRIBUTING.md" -o -name "SECURITY.md" \
    -o -name "*.lock" -o -name "*.toml" -o -name "*.nix" \
    -o -name ".git*" -o -name ".clang*" -o -name ".flake*" \
    \) 2>/dev/null)

if [ -n "$VIOLATIONS" ]; then
    echo "WARNING: Source artifacts found, cleaning..."
    echo "$VIOLATIONS" | xargs rm -f
fi

echo ""
echo "Package complete: $DIST_DIR ($name)"
du -sh "$DIST_DIR"
