#!/bin/bash
# build_all.sh — 通用构建驱动 (K3 RISC-V 原生编译)
# 读取 components.yaml，自动构建所有组件并生成 release
#
# 用法：
#   ./build/build_all.sh                          # 默认版本 + riscv_b profile
#   ./build/build_all.sh v1.2.0                   # 指定版本
#   ./build/build_all.sh v1.0.0 riscv_a           # 指定 node profile
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
COMPONENTS_FILE="$ROOT_DIR/components.yaml"

VERSION=${1:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo "v1.0.0")}
NODE_PROFILE=${2:-riscv_b}
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

PLATFORM=riscv64
GOARCH=riscv64
PLATFORM_TARGET=riscv64

DIST_DIR="$ROOT_DIR/dist/$VERSION-$TIMESTAMP/$PLATFORM"
mkdir -p "$DIST_DIR/bin"

echo "============================================"
echo " RiVision Build System (K3 RISC-V native)"
echo " Platform:     $PLATFORM"
echo " Version:      $VERSION"
echo " Node Profile: $NODE_PROFILE"
echo " Output:       $DIST_DIR"
echo "============================================"
echo ""

# --- 解析 components.yaml ---
# 简单的 YAML 解析器（不依赖 yq/python）
parse_components() {
    local current_name=""
    local in_components=false

    while IFS= read -r line; do
        # 跳过注释和空行
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        [[ -z "${line// }" ]] && continue

        # 进入 components 块
        if [[ "$line" == "components:" ]]; then
            in_components=true
            continue
        fi

        # 离开 components 块（遇到同级 key）
        if $in_components && [[ "$line" =~ ^[a-z] ]]; then
            # 输出最后一个组件
            if [ -n "$current_name" ]; then
                echo "COMPONENT_END:$current_name"
            fi
            in_components=false
            continue
        fi

        if ! $in_components; then
            continue
        fi

        # 组件名（2空格缩进）
        if [[ "$line" =~ ^[[:space:]]{2}[a-z_]+:$ ]]; then
            # 输出上一个组件
            if [ -n "$current_name" ]; then
                echo "COMPONENT_END:$current_name"
            fi
            current_name=$(echo "$line" | sed 's/^ *//;s/:$//')
            echo "COMPONENT_START:$current_name"
            continue
        fi

        # 属性（4空格缩进）
        if [[ "$line" =~ ^[[:space:]]{4}[a-z_]+: ]]; then
            local key=$(echo "$line" | sed 's/^ *//;s/:.*//')
            local val=$(echo "$line" | sed 's/^[^:]*: *//;s/^ *"//;s/" *$//')
            echo "PROP:$key=$val"
        fi
    done < "$COMPONENTS_FILE"

    # 输出最后一个组件
    if [ -n "$current_name" ]; then
        echo "COMPONENT_END:$current_name"
    fi
}

# --- 构建所有组件 ---
STEP=0
TOTAL=$(parse_components | grep -c "COMPONENT_START" || echo 0)
FAILED=()

build_component() {
    local name=$1 type=$2 source=$3 build_cmd=$4 output=$5 install_as=$6
    local output_riscv64=$7

    STEP=$((STEP + 1))
    echo "[$STEP/$TOTAL] Building $name ($type)..."

    # 变量替换
    local resolved_cmd=$(echo "$build_cmd" | \
        sed "s|\${PLATFORM}|$PLATFORM|g" | \
        sed "s|\${PLATFORM_TARGET}|$PLATFORM_TARGET|g" | \
        sed "s|\${GOARCH}|$GOARCH|g" | \
        sed "s|\${VERSION}|$VERSION|g" | \
        sed "s|\${NODE_PROFILE}|$NODE_PROFILE|g" | \
        sed "s|\${DIST_DIR}|$DIST_DIR|g")

    # 确定输出文件
    local resolved_output="$output"
    if [ -n "$output_riscv64" ]; then
        resolved_output="$output_riscv64"
    fi

    echo "  cmd: $resolved_cmd"
    echo "  dir: $ROOT_DIR/$source"

    # 执行构建
    cd "$ROOT_DIR/$source"
    if ! eval "$resolved_cmd"; then
        echo "  FAILED: $name"
        FAILED+=("$name")
        cd "$ROOT_DIR"
        return 1
    fi

    # 复制产物到 dist（profile 类型由自己的 build_cmd 直接输出）
    if [ "$type" = "go" ] && [ -n "$resolved_output" ] && [ -n "$install_as" ]; then
        local dest_dir=$(dirname "$DIST_DIR/$install_as")
        mkdir -p "$dest_dir"
        cp "$resolved_output" "$DIST_DIR/$install_as"
        echo "  output: $install_as"
    fi

    cd "$ROOT_DIR"
    echo ""
}

# 解析并执行
COMP_NAME="" COMP_TYPE="" COMP_SOURCE="" COMP_BUILD="" COMP_OUTPUT="" COMP_INSTALL=""
COMP_OUTPUT_RISCV64=""

while IFS= read -r token; do
    case "$token" in
        COMPONENT_START:*)
            COMP_NAME="${token#COMPONENT_START:}"
            COMP_TYPE="" COMP_SOURCE="" COMP_BUILD="" COMP_OUTPUT="" COMP_INSTALL=""
            COMP_OUTPUT_RISCV64=""
            ;;
        PROP:type=*)         COMP_TYPE="${token#PROP:type=}" ;;
        PROP:source=*)       COMP_SOURCE="${token#PROP:source=}" ;;
        PROP:build_cmd=*)    COMP_BUILD="${token#PROP:build_cmd=}" ;;
        PROP:output=*)       COMP_OUTPUT="${token#PROP:output=}" ;;
        PROP:output_riscv64=*) COMP_OUTPUT_RISCV64="${token#PROP:output_riscv64=}" ;;
        PROP:install_as=*)   COMP_INSTALL="${token#PROP:install_as=}" ;;
        COMPONENT_END:*)
            if [ -n "$COMP_TYPE" ] && [ -n "$COMP_BUILD" ]; then
                build_component "$COMP_NAME" "$COMP_TYPE" "$COMP_SOURCE" \
                    "$COMP_BUILD" "$COMP_OUTPUT" "$COMP_INSTALL" \
                    "$COMP_OUTPUT_RISCV64"
            fi
            ;;
    esac
done < <(parse_components)

# --- 生成 manifest + checksum ---
echo "Generating manifest and checksums..."
"$SCRIPT_DIR/scripts/generate_manifest.sh" "$DIST_DIR" "$VERSION" "$PLATFORM"
"$SCRIPT_DIR/scripts/checksum.sh" "$DIST_DIR"
cp "$SCRIPT_DIR/templates/install.sh" "$DIST_DIR/"
cp "$SCRIPT_DIR/templates/uninstall.sh" "$DIST_DIR/"
cp "$SCRIPT_DIR/templates/rivision-gateway.service" "$DIST_DIR/" 2>/dev/null || true
cp "$SCRIPT_DIR/templates/rivision-cli.service" "$DIST_DIR/" 2>/dev/null || true
# Host 端 .env 模板
mkdir -p "$DIST_DIR/config"
cp "$SCRIPT_DIR/templates/.env.template" "$DIST_DIR/config/" 2>/dev/null || true
chmod +x "$DIST_DIR/install.sh" "$DIST_DIR/uninstall.sh"
ln -sfn "$VERSION-$TIMESTAMP" "$ROOT_DIR/dist/latest"

# --- 结果 ---
echo ""
echo "============================================"
if [ ${#FAILED[@]} -gt 0 ]; then
    echo " Build completed with errors:"
    for f in "${FAILED[@]}"; do echo "   FAILED: $f"; done
    echo ""
fi
echo " Release: $DIST_DIR"
du -sh "$DIST_DIR"
echo "============================================"
