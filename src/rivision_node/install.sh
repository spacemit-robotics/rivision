#!/bin/bash
# ============================================================
# [已废弃] RiVision K3 节点一键安装脚本 (旧版)
#
# ⚠ 此脚本为旧版安装流程，不兼容新的 dist 目录结构。
# 新的部署请使用: build/templates/install.sh
# 此文件保留仅供参考，后续版本将删除。
# ============================================================
# RiVision K3 节点一键安装脚本
# 
# 功能:
#   1. 安装系统依赖
#   2. 编译 llama.cpp (K3 RVV优化)
#   3. 编译 YOLO Server (ONNX Runtime)
#   4. 验证模型文件
#   5. 安装 Node Agent (Python/Go/C++)
#   6. 配置 systemd 服务
#   7. K3 系统优化
#   8. 验证部署
#
# 用法:
#   ./install.sh <GATEWAY_URL> [OPTIONS]
#   ./install.sh http://192.168.1.100:8081
#   ./install.sh http://192.168.1.100:8081 --agent=go     # 使用 Go 版本
#   ./install.sh http://192.168.1.100:8081 --agent=cpp    # 使用 C++ 版本
#   ./install.sh http://192.168.1.100:8081 --build        # 编译 llama.cpp 和 YOLO Server
#   ./install.sh http://192.168.1.100:8081 --skip-optimize # 跳过系统优化
#   ./install.sh http://192.168.1.100:8081 --force        # 升级模式下跳过确认提示
# ============================================================
set -e

# ============ 颜色 ============
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[ OK ]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[FAIL]${NC} $1"; }
log_step()    { echo -e "\n${CYAN}════════════════════════════════════════${NC}"; echo -e "${CYAN}  $1${NC}"; echo -e "${CYAN}════════════════════════════════════════${NC}"; }

# ============ 参数解析 ============
GATEWAY_URL=""
SKIP_BUILD=true
SKIP_OPTIMIZE=false
FORCE_BUILD=false
FORCE_UPDATE=false
NODE_AGENT_TYPE="python"  # 默认 Python 版本
ARCH=$(uname -m)
ARCH_SUFFIX="$ARCH"

for arg in "$@"; do
    case "$arg" in
        --build)         FORCE_BUILD=true ;;
        --skip-build)    SKIP_BUILD=true ;;
        --skip-optimize) SKIP_OPTIMIZE=true ;;
        --force)         FORCE_UPDATE=true ;;
        --agent=*)
            NODE_AGENT_TYPE="${arg#--agent=}"
            # 规范化名称
            case "$NODE_AGENT_TYPE" in
                python|py)   NODE_AGENT_TYPE="python" ;;
                go|golang)   NODE_AGENT_TYPE="go" ;;
                cpp|c++)     NODE_AGENT_TYPE="cpp" ;;
                *)
                    echo "错误: 未知的 Agent 版本: $NODE_AGENT_TYPE"
                    echo "支持: python, go, cpp"
                    exit 1
                    ;;
            esac
            ;;
        --help|-h)
            echo "用法: ./install.sh <GATEWAY_URL> [OPTIONS]"
            echo ""
            echo "  GATEWAY_URL          Gateway 服务地址 (必须, 如 http://192.168.1.100:8081)"
            echo ""
            echo "选项:"
            echo "  --agent=TYPE         Node Agent 版本: python (默认), go, cpp"
            echo "  --build              编译 llama.cpp 和 YOLO Server (默认跳过)"
            echo "  --skip-optimize      跳过 K3 系统优化"
            echo "  --force              升级模式下跳过确认提示"
            echo "  --help, -h           显示此帮助信息"
            echo ""
            echo "示例:"
            echo "  ./install.sh http://192.168.1.100:8081"
            echo "  ./install.sh http://192.168.1.100:8081 --agent=go"
            echo "  ./install.sh http://192.168.1.100:8081 --agent=cpp --build"
            echo "  ./install.sh http://192.168.1.100:8081 --force"
            exit 0
            ;;
        http*)           GATEWAY_URL="$arg" ;;
    esac
done

# 检查 Gateway URL 是否指定
if [ -z "$GATEWAY_URL" ]; then
    # 检查已有 .env 中是否已配置
    INSTALL_DIR_CHECK="${RIVISION_HOME:-$HOME/rivision}"
    EXISTING_GW=""
    if [ -f "$INSTALL_DIR_CHECK/.env" ]; then
        EXISTING_GW=$(grep "^GATEWAY_URL=" "$INSTALL_DIR_CHECK/.env" 2>/dev/null | cut -d'=' -f2)
    fi
    if [ -z "$EXISTING_GW" ] || [ "$EXISTING_GW" = "http://192.168.122.1:8081" ]; then
        log_error "必须指定 Gateway URL"
        echo ""
        echo "用法: ./install.sh <GATEWAY_URL> [OPTIONS]"
        echo ""
        echo "示例:"
        echo "  ./install.sh http://192.168.1.100:8081"
        echo "  ./install.sh http://192.168.1.100:8081 --build"
        echo ""
        echo "使用 --help 查看更多选项"
        exit 1
    else
        GATEWAY_URL="$EXISTING_GW"
        log_info "使用已有配置的 Gateway: $GATEWAY_URL"
    fi
fi

# ============ 路径 ============
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${RIVISION_HOME:-$HOME/rivision}"
CURRENT_USER=$(whoami)
ARCH=$(uname -m)

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║   RiVision K3 节点部署                       ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
log_info "架构: $ARCH"
log_info "用户: $CURRENT_USER"
log_info "部署目录: $INSTALL_DIR"
log_info "Gateway: ${GATEWAY_URL:-未指定}"
log_info "Node Agent: $NODE_AGENT_TYPE"
echo ""

# ============================================================
# Step 1: 系统依赖
# ============================================================
log_step "Step 1/7: 安装系统依赖"

if command -v apt &>/dev/null; then
    sudo apt update -qq
    sudo apt install -y -qq build-essential cmake python3-dev python3-venv \
        python3-pip curl wget libssl-dev pkg-config ca-certificates git htop
    log_success "系统依赖安装完成"
else
    log_warn "非 apt 系统，请手动安装: build-essential cmake python3-dev python3-venv"
fi

# ============================================================
# Step 2: 检查已有部署 & 智能文件同步
# ============================================================
log_step "Step 2/7: 检查部署目录"

# ── 辅助函数 ──────────────────────────────────────────────
file_hash() { md5sum "$1" 2>/dev/null | cut -d' ' -f1; }
file_mtime() { stat -c '%Y' "$1" 2>/dev/null || echo 0; }

# 对比单个文件，返回: same / new / updated / conflict
compare_file() {
    local src="$1" dst="$2"
    if [ ! -f "$dst" ]; then echo "new"; return; fi
    if [ ! -f "$src" ]; then echo "missing"; return; fi
    local h1=$(file_hash "$src") h2=$(file_hash "$dst")
    if [ "$h1" = "$h2" ]; then echo "same"; else echo "updated"; fi
}

# 获取 node-agent 版本
get_agent_version() {
    local dir="$1"
    if [ -f "$dir/node-agent/app/__init__.py" ]; then
        grep -oP '__version__\s*=\s*"\K[^"]+' "$dir/node-agent/app/__init__.py" 2>/dev/null || echo "unknown"
    else
        echo "none"
    fi
}

# ── 检测是否为升级模式 ──────────────────────────────────────
IS_UPGRADE=false
if [ -d "$INSTALL_DIR" ] && [ "$SCRIPT_DIR" != "$INSTALL_DIR" ]; then
    # 检查目标目录是否有实质内容 (非空目录)
    if [ -f "$INSTALL_DIR/.env" ] || [ -d "$INSTALL_DIR/node-agent" ] || \
       [ -d "$INSTALL_DIR/scripts" ]; then
        IS_UPGRADE=true
    fi
fi

mkdir -p "$INSTALL_DIR"/{models,logs,config,scripts}

if $IS_UPGRADE; then
    log_info "检测到已有部署，进入升级对比模式"
    echo ""

    # ── 版本对比 ──────────────────────────────────────────
    PKG_VER=$(get_agent_version "$SCRIPT_DIR")
    CUR_VER=$(get_agent_version "$INSTALL_DIR")
    echo "  ┌─ 版本信息 ────────────────────────────────"
    echo "  │  部署包 Node Agent: v${PKG_VER}"
    echo "  │  已部署 Node Agent: v${CUR_VER}"
    if [ "$PKG_VER" != "$CUR_VER" ] && [ "$CUR_VER" != "none" ]; then
        echo -e "  │  ${YELLOW}版本不同, 将升级${NC}"
    elif [ "$CUR_VER" = "none" ]; then
        echo -e "  │  ${YELLOW}目标未安装 Node Agent${NC}"
    else
        echo -e "  │  ${GREEN}版本一致${NC}"
    fi
    echo "  └────────────────────────────────────────────"
    echo ""

    # ── 逐类文件对比 ──────────────────────────────────────
    UPDATED_FILES=()
    NEW_FILES=()
    SAME_FILES=()
    PROTECTED_FILES=()

    # 1) 脚本文件 (始终用最新版)
    echo "  ┌─ 脚本文件 (scripts/) ─────────────────────"
    for src in "$SCRIPT_DIR/scripts/"*.sh; do
        [ -f "$src" ] || continue
        fname=$(basename "$src")
        dst="$INSTALL_DIR/scripts/$fname"
        status=$(compare_file "$src" "$dst")
        case "$status" in
            new)     echo -e "  │  ${GREEN}+ $fname${NC} (新增)"; NEW_FILES+=("scripts/$fname") ;;
            updated) echo -e "  │  ${YELLOW}↑ $fname${NC} (有更新)"; UPDATED_FILES+=("scripts/$fname") ;;
            same)    echo -e "  │  · $fname (一致)"; SAME_FILES+=("scripts/$fname") ;;
        esac
    done
    echo "  └────────────────────────────────────────────"

    # 2) 配置文件 (保护用户修改)
    echo "  ┌─ 配置文件 (config/) ──────────────────────"
    for src in "$SCRIPT_DIR/config/"* "$SCRIPT_DIR/config/".*; do
        [ -f "$src" ] || continue
        fname=$(basename "$src")
        [[ "$fname" == "." || "$fname" == ".." ]] && continue
        dst="$INSTALL_DIR/config/$fname"
        status=$(compare_file "$src" "$dst")
        case "$status" in
            new)     echo -e "  │  ${GREEN}+ $fname${NC} (新增)"; NEW_FILES+=("config/$fname") ;;
            updated) echo -e "  │  ${YELLOW}! $fname${NC} (不同 → 保留已有, 新版存为 .new)"; PROTECTED_FILES+=("config/$fname") ;;
            same)    echo -e "  │  · $fname (一致)"; SAME_FILES+=("config/$fname") ;;
        esac
    done
    # .env 特殊处理
    if [ -f "$INSTALL_DIR/.env" ]; then
        echo -e "  │  ${GREEN}✓ .env${NC} (保留用户配置)"
    fi
    echo "  └────────────────────────────────────────────"

    # 3) 服务文件
    echo "  ┌─ 服务文件 (services/) ────────────────────"
    for src in "$SCRIPT_DIR/services/"*.service; do
        [ -f "$src" ] || continue
        fname=$(basename "$src")
        dst="$INSTALL_DIR/$fname"
        status=$(compare_file "$src" "$dst")
        case "$status" in
            new)     echo -e "  │  ${GREEN}+ $fname${NC} (新增)"; NEW_FILES+=("services/$fname") ;;
            updated) echo -e "  │  ${YELLOW}↑ $fname${NC} (有更新)"; UPDATED_FILES+=("services/$fname") ;;
            same)    echo -e "  │  · $fname (一致)"; SAME_FILES+=("services/$fname") ;;
        esac
    done
    echo "  └────────────────────────────────────────────"

    # 4) Node Agent 代码
    echo "  ┌─ Node Agent ──────────────────────────────"
    if [ -d "$SCRIPT_DIR/node-agent" ]; then
        agent_changes=0
        while IFS= read -r src; do
            rel="${src#$SCRIPT_DIR/}"
            dst="$INSTALL_DIR/$rel"
            status=$(compare_file "$src" "$dst")
            [ "$status" = "same" ] && continue
            agent_changes=$((agent_changes + 1))
            if [ $agent_changes -le 10 ]; then
                case "$status" in
                    new)     echo -e "  │  ${GREEN}+ $rel${NC}" ;;
                    updated) echo -e "  │  ${YELLOW}↑ $rel${NC}" ;;
                esac
            fi
        done < <(find "$SCRIPT_DIR/node-agent" -type f -not -path "*/venv/*" -not -path "*/__pycache__/*" -not -name "*.pyc" -not -name ".env")
        if [ $agent_changes -gt 10 ]; then
            echo -e "  │  ... 及另外 $((agent_changes - 10)) 个文件"
        fi
        if [ $agent_changes -eq 0 ]; then
            echo -e "  │  ${GREEN}✓ 所有文件一致${NC}"
        else
            echo -e "  │  共 ${agent_changes} 个文件需更新"
        fi
    fi
    echo "  └────────────────────────────────────────────"

    # 5) llama.cpp 编译产物
    echo "  ┌─ llama.cpp ───────────────────────────────"
    if [[ "$ARCH" == "riscv64" ]]; then
        BUILD_SUBDIR="build-k3"
    else
        BUILD_SUBDIR="build_x86"
    fi
    LLAMA_BIN="$INSTALL_DIR/llama.cpp/$BUILD_SUBDIR/bin"
    if [ -d "$LLAMA_BIN" ]; then
        # 检查二进制文件
        for bin in llama-server llama-mtmd-cli llama-quantize; do
            if [ -x "$LLAMA_BIN/$bin" ]; then
                local_mtime=$(stat -c '%Y' "$LLAMA_BIN/$bin" 2>/dev/null || echo 0)
                local_date=$(date -d "@$local_mtime" '+%Y-%m-%d %H:%M' 2>/dev/null || echo "unknown")
                echo -e "  │  ✓ $bin (编译于: $local_date)"
            else
                echo -e "  │  ${YELLOW}✗ $bin (缺失, 需重新编译)${NC}"
            fi
        done
        
        # 检查共享库
        echo -e "  │  共享库:"
        lib_count=0
        missing_libs=()
        for lib in libmtmd.so.0 libllama.so.0 libggml.so.0 libggml-base.so.0 libggml-cpu.so.0; do
            if [ -f "$LLAMA_BIN/$lib" ] || [ -L "$LLAMA_BIN/$lib" ]; then
                echo -e "  │  ✓ $lib"
                lib_count=$((lib_count + 1))
            else
                echo -e "  │  ${YELLOW}✗ $lib (缺失)${NC}"
                missing_libs+=("$lib")
            fi
        done
        
        if [ $lib_count -eq 5 ]; then
            echo -e "  │  ${GREEN}✓ 所有共享库完整${NC}"
        else
            echo -e "  │  ${YELLOW}⚠ 缺失 ${#missing_libs[@]} 个共享库，运行时可能出错${NC}"
        fi
    else
        echo -e "  │  ${YELLOW}未编译 (需运行 scripts/build.sh)${NC}"
    fi
    echo "  └────────────────────────────────────────────"

    # 6) 模型文件
    echo "  ┌─ 模型文件 ────────────────────────────────"
    model_count=0
    for f in "$INSTALL_DIR/models/"*.gguf; do
        [ -f "$f" ] || continue
        model_count=$((model_count + 1))
        echo "  │  ✓ $(basename "$f") ($(du -h "$f" | cut -f1))"
    done
    [ $model_count -eq 0 ] && echo -e "  │  ${YELLOW}无模型文件${NC}"
    echo "  └────────────────────────────────────────────"

    # ── 汇总 ──────────────────────────────────────────────
    echo ""
    echo "  ╔═ 对比汇总 ═══════════════════════════════╗"
    echo "  ║  一致: ${#SAME_FILES[@]} 文件"
    echo "  ║  新增: ${#NEW_FILES[@]} 文件"
    echo "  ║  更新: ${#UPDATED_FILES[@]} 文件"
    echo "  ║  保护: ${#PROTECTED_FILES[@]} 文件 (保留已有)"
    echo "  ╚══════════════════════════════════════════╝"
    echo ""

    # ── 执行同步 ──────────────────────────────────────────
    if [ ${#UPDATED_FILES[@]} -gt 0 ] || [ ${#NEW_FILES[@]} -gt 0 ]; then
        if ! $FORCE_UPDATE; then
            log_info "发现 $((${#UPDATED_FILES[@]} + ${#NEW_FILES[@]})) 个文件需更新"
            read -p "  是否继续? [Y/n] " -n 1 -r
            echo ""
            if [[ $REPLY =~ ^[Nn]$ ]]; then
                log_warn "用户取消更新"
                exit 0
            fi
        fi
    fi

    # 同步脚本 (直接覆盖)
    for f in "$SCRIPT_DIR/scripts/"*.sh; do
        [ -f "$f" ] || continue
        cp "$f" "$INSTALL_DIR/scripts/"
    done
    chmod +x "$INSTALL_DIR/scripts/"*.sh 2>/dev/null || true

    # 同步配置 (保护已有, 新版存为 .new)
    for rel in "${PROTECTED_FILES[@]}"; do
        cp "$SCRIPT_DIR/$rel" "$INSTALL_DIR/${rel}.new"
        log_info "$rel 有更新 → 已保存为 ${rel}.new (请手动合并)"
    done
    for rel in "${NEW_FILES[@]}"; do
        case "$rel" in
            config/*) cp "$SCRIPT_DIR/$rel" "$INSTALL_DIR/$rel" ;;
        esac
    done

    # 同步 node-agent (保留 venv 和 .env)
    if [ -d "$SCRIPT_DIR/node-agent" ]; then
        # 备份 venv 和 .env
        AGENT_VENV="$INSTALL_DIR/node-agent/venv"
        AGENT_ENV="$INSTALL_DIR/node-agent/.env"
        HAS_VENV=false; [ -d "$AGENT_VENV" ] && HAS_VENV=true
        
        rsync -a --exclude='venv/' --exclude='.env' --exclude='__pycache__/' \
            --exclude='*.pyc' "$SCRIPT_DIR/node-agent/" "$INSTALL_DIR/node-agent/" 2>/dev/null || \
        cp -r "$SCRIPT_DIR/node-agent/"* "$INSTALL_DIR/node-agent/" 2>/dev/null || true
        
        log_success "Node Agent 代码已同步 (venv 已保留)"
    fi

    # 同步服务文件
    for src in "$SCRIPT_DIR/services/"*.service; do
        [ -f "$src" ] || continue
        cp "$src" "$INSTALL_DIR/" 2>/dev/null || true
    done

    # 同步 llama.cpp 源码 (如果部署包有且目标没有/需更新)
    if [ -d "$SCRIPT_DIR/llama.cpp" ]; then
        if [ ! -d "$INSTALL_DIR/llama.cpp" ]; then
            log_info "复制 llama.cpp 源码 (目标缺失)..."
            cp -r "$SCRIPT_DIR/llama.cpp" "$INSTALL_DIR/"
        else
            # 源码更新检测（仅提示，不复制源码到安装目录）
            src_cmake="$SCRIPT_DIR/llama.cpp/CMakeLists.txt"
            dst_cmake_hash=""
            if [ -f "$SCRIPT_DIR/llama.cpp/bin/llama-server.${ARCH_SUFFIX}" ]; then
                # 有预编译缓存，检查是否需要更新
                log_info "llama.cpp 预编译缓存可用"
            elif [ -f "$src_cmake" ]; then
                log_info "llama.cpp 源码可用，如需重新编译请使用 --build"
            fi
        fi
    fi

    # 同步模型 (不覆盖已有)
    if ls "$SCRIPT_DIR/models/"*.gguf &>/dev/null; then
        cp -n "$SCRIPT_DIR/models/"*.gguf "$INSTALL_DIR/models/" 2>/dev/null || true
    fi
    if ls "$SCRIPT_DIR/models/"*.onnx &>/dev/null; then
        cp -n "$SCRIPT_DIR/models/"*.onnx "$INSTALL_DIR/models/" 2>/dev/null || true
    fi

    # 检测 .env 是否缺少新配置项
    if [ -f "$INSTALL_DIR/.env" ] && [ -f "$INSTALL_DIR/config/.env.template" ]; then
        MISSING_KEYS=()
        while IFS='=' read -r key _; do
            # 跳过注释和空行和占位符
            [[ "$key" =~ ^#.*$ || -z "$key" || "$key" =~ ^[[:space:]]*$ || "$key" =~ __.*__ ]] && continue
            key=$(echo "$key" | tr -d '[:space:]')
            if ! grep -q "^${key}=" "$INSTALL_DIR/.env" 2>/dev/null; then
                MISSING_KEYS+=("$key")
            fi
        done < "$INSTALL_DIR/config/.env.template"
        if [ ${#MISSING_KEYS[@]} -gt 0 ]; then
            log_warn ".env 缺少 ${#MISSING_KEYS[@]} 个新配置项:"
            for k in "${MISSING_KEYS[@]}"; do
                # 从模板获取默认值并追加
                default_line=$(grep "^${k}=" "$INSTALL_DIR/config/.env.template")
                echo "  + $default_line"
                echo "$default_line" >> "$INSTALL_DIR/.env"
            done
            log_success "已自动追加缺失配置项到 .env"
        fi
    fi

    log_success "文件同步完成 (升级模式)"

else
    # ── 全新安装 ──────────────────────────────────────────
    log_info "全新安装模式"

    if [ "$SCRIPT_DIR" != "$INSTALL_DIR" ]; then
        log_info "复制部署包到 $INSTALL_DIR ..."
        
        # 复制配置 (glob * 不匹配隐藏文件, 需单独复制 .env.template)
        cp -n "$SCRIPT_DIR/config/"* "$INSTALL_DIR/config/" 2>/dev/null || true
        cp -n "$SCRIPT_DIR/config/".* "$INSTALL_DIR/config/" 2>/dev/null || true
        
        # 复制脚本
        cp "$SCRIPT_DIR/scripts/"* "$INSTALL_DIR/scripts/" 2>/dev/null || true
        chmod +x "$INSTALL_DIR/scripts/"*.sh
        
        # 复制 node-agent
        if [ -d "$SCRIPT_DIR/node-agent" ]; then
            cp -r "$SCRIPT_DIR/node-agent" "$INSTALL_DIR/" 2>/dev/null || true
        fi

        # 注意：llama.cpp 和 yolo-server 不在此处全量复制
        # Step 3/4 会按需只复制运行时文件（二进制 + 库）
        
        # 复制模型 (GGUF + ONNX)
        if ls "$SCRIPT_DIR/models/"*.gguf &>/dev/null; then
            log_info "复制 VLM 模型文件..."
            cp -n "$SCRIPT_DIR/models/"*.gguf "$INSTALL_DIR/models/" 2>/dev/null || true
        fi
        if ls "$SCRIPT_DIR/models/"*.onnx &>/dev/null; then
            log_info "复制 YOLO 模型文件..."
            cp -n "$SCRIPT_DIR/models/"*.onnx "$INSTALL_DIR/models/" 2>/dev/null || true
        fi
        
        # 复制服务文件
        cp "$SCRIPT_DIR/services/"*.service "$INSTALL_DIR/" 2>/dev/null || true
    fi

    log_success "全新安装文件复制完成"
fi

# 生成 .env (如果不存在)
if [ ! -f "$INSTALL_DIR/.env" ]; then
    cp "$INSTALL_DIR/config/.env.template" "$INSTALL_DIR/.env"
    # 替换安装路径
    sed -i "s|/home/node/rivision|$INSTALL_DIR|g" "$INSTALL_DIR/.env"
    # 设置 Gateway URL
    if [ -n "$GATEWAY_URL" ]; then
        sed -i "s|GATEWAY_URL=.*|GATEWAY_URL=$GATEWAY_URL|" "$INSTALL_DIR/.env"
    fi
    # 设置 Node Agent 版本
    sed -i "s|NODE_AGENT_TYPE=.*|NODE_AGENT_TYPE=$NODE_AGENT_TYPE|" "$INSTALL_DIR/.env"
    # 根据架构设置平台相关参数
    if [[ "$ARCH" == "riscv64" ]]; then
        sed -i "s|__NODE_TAGS__|riscv,k3,compute|" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_THREADS__|6|" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_CPU_AFFINITY__|0-5|" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_BUILD_DIR__|$INSTALL_DIR/llama.cpp/build-k3|" "$INSTALL_DIR/.env"
    else
        NPROC=$(nproc 2>/dev/null || echo 4)
        sed -i "s|__NODE_TAGS__|x86,compute|" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_THREADS__|$NPROC|" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_CPU_AFFINITY__||" "$INSTALL_DIR/.env"
        sed -i "s|__LLAMA_BUILD_DIR__|$INSTALL_DIR/llama.cpp/build_x86|" "$INSTALL_DIR/.env"
    fi
    log_success ".env 配置已生成 (架构: $ARCH, Agent: $NODE_AGENT_TYPE)"
else
    log_info ".env 已存在，跳过"
    # 升级时也更新 Gateway URL（如果用户指定了新的）
    if [ -n "$GATEWAY_URL" ]; then
        sed -i "s|GATEWAY_URL=.*|GATEWAY_URL=$GATEWAY_URL|" "$INSTALL_DIR/.env"
        log_success "Gateway URL 已更新: $GATEWAY_URL"
    fi
    # 升级时更新 Node Agent 版本（如果用户指定了）
    if [ "$NODE_AGENT_TYPE" != "python" ]; then
        sed -i "s|NODE_AGENT_TYPE=.*|NODE_AGENT_TYPE=$NODE_AGENT_TYPE|" "$INSTALL_DIR/.env"
        log_success "Node Agent 版本已更新: $NODE_AGENT_TYPE"
    fi
    # 清理硬编码的 YOLO_MODEL，让 start-yolo.sh 按平台自动选择
    if grep -q "^YOLO_MODEL=" "$INSTALL_DIR/.env"; then
        sed -i "s|^YOLO_MODEL=|#YOLO_MODEL=|" "$INSTALL_DIR/.env"
        log_info "YOLO_MODEL 已改为自动选择（按平台检测）"
    fi
fi

log_success "目录结构创建完成"

# ============================================================
# Step 3: 编译 llama.cpp
# ============================================================
log_step "Step 3/8: 编译 llama.cpp"

# 路径定义（源码在包目录，运行时在安装目录）
if [[ "$ARCH" == "riscv64" ]]; then
    SRC_LLAMA_BUILD_DIR="$SCRIPT_DIR/llama.cpp/build-k3"
    DST_LLAMA_BUILD_DIR="$INSTALL_DIR/llama.cpp/build-k3"
else
    SRC_LLAMA_BUILD_DIR="$SCRIPT_DIR/llama.cpp/build_x86"
    DST_LLAMA_BUILD_DIR="$INSTALL_DIR/llama.cpp/build_x86"
fi
SRC_LLAMA_BIN="$SRC_LLAMA_BUILD_DIR/bin/llama-server"
DST_LLAMA_BIN="$DST_LLAMA_BUILD_DIR/bin/llama-server"
LLAMA_PREBUILT="$SCRIPT_DIR/llama.cpp/bin/llama-server.${ARCH_SUFFIX}"

# 部署 llama-server 二进制及共享库（运行时文件）
deploy_llama_bin() {
    local src="$1"
    local src_dir="$(dirname "$src")"
    
    # 创建目标目录
    mkdir -p "$DST_LLAMA_BUILD_DIR/bin"
    
    # 1. 复制主二进制
    cp "$src" "$DST_LLAMA_BIN"
    chmod +x "$DST_LLAMA_BIN"
    
    # 2. 复制所有共享库（-a 保留符号链接结构，避免重复复制）
    local lib_count=0
    for lib in "$src_dir"/lib*.so*; do
        [ -e "$lib" ] || continue
        lib_count=$((lib_count + 1))
    done
    if [ $lib_count -gt 0 ]; then
        cp -a "$src_dir"/lib*.so* "$DST_LLAMA_BUILD_DIR/bin/" 2>/dev/null || \
        cp -P "$src_dir"/lib*.so* "$DST_LLAMA_BUILD_DIR/bin/" 2>/dev/null || \
        cp "$src_dir"/lib*.so* "$DST_LLAMA_BUILD_DIR/bin/"
    fi
    
    # 3. 验证关键共享库存在
    local missing=0
    for lib_name in libmtmd.so libllama.so libggml.so libggml-base.so libggml-cpu.so; do
        # 检查 .so 或 .so.0 或 .so.0.x.x 任一形式存在
        if ! ls "$DST_LLAMA_BUILD_DIR/bin/${lib_name}"* &>/dev/null; then
            log_warn "共享库缺失: $lib_name"
            missing=$((missing + 1))
        fi
    done
    
    if [ $missing -gt 0 ]; then
        log_warn "缺失 $missing 个共享库，llama-server 运行时可能出错"
    fi
    
    log_success "llama-server 已部署到 $DST_LLAMA_BUILD_DIR/bin/ (含 $lib_count 个库文件)"
}

# 架构匹配检查（检查安装目录中的已有二进制 + 共享库完整性）
check_llama_arch() {
    if [ ! -x "$DST_LLAMA_BIN" ]; then return 1; fi
    local BIN_ARCH=$(file "$DST_LLAMA_BIN" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
    case "$ARCH" in
        riscv64) [ "$BIN_ARCH" = "RISC-V" ] || return 1 ;;
        aarch64) [ "$BIN_ARCH" = "aarch64" ] || return 1 ;;
        *)       [ "$BIN_ARCH" = "x86-64" ] || return 1 ;;
    esac
    # 还需验证共享库存在（旧版安装可能缺失）
    for lib_name in libmtmd.so libllama.so libggml.so libggml-base.so libggml-cpu.so; do
        if ! ls "$DST_LLAMA_BUILD_DIR/bin/${lib_name}"* &>/dev/null; then
            log_warn "共享库缺失: $lib_name，需重新部署"
            return 1
        fi
    done
}

# 检查包目录中已编译的二进制是否匹配
check_src_llama_arch() {
    if [ ! -x "$SRC_LLAMA_BIN" ]; then return 1; fi
    local BIN_ARCH=$(file "$SRC_LLAMA_BIN" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
    case "$ARCH" in
        riscv64) [ "$BIN_ARCH" = "RISC-V" ] ;;
        aarch64) [ "$BIN_ARCH" = "aarch64" ] ;;
        *)       [ "$BIN_ARCH" = "x86-64" ] ;;
    esac
}

# 优先级0：--build 强制重新编译
if $FORCE_BUILD && [ -d "$SCRIPT_DIR/llama.cpp" ]; then
    log_info "强制重新编译 llama.cpp (--build)..."
    bash "$SCRIPT_DIR/scripts/build.sh"
    if [ -x "$SRC_LLAMA_BIN" ]; then
        deploy_llama_bin "$SRC_LLAMA_BIN"
        # 缓存二进制+共享库
        mkdir -p "$SCRIPT_DIR/llama.cpp/bin"
        cp "$SRC_LLAMA_BIN" "$LLAMA_PREBUILT"
        cp -a "$SRC_LLAMA_BUILD_DIR/bin"/lib*.so* "$SCRIPT_DIR/llama.cpp/bin/" 2>/dev/null || true
    else
        log_warn "llama.cpp 编译失败"
    fi

# 优先级1：安装目录已有匹配二进制 → 跳过
elif check_llama_arch; then
    log_info "已有 $ARCH_SUFFIX llama-server，跳过"

# 优先级2：预编译缓存 → 直接部署
elif [ -f "$LLAMA_PREBUILT" ]; then
    log_info "使用预编译二进制: llama-server.$ARCH_SUFFIX"
    deploy_llama_bin "$LLAMA_PREBUILT"

# 优先级3：包目录中已编译且匹配 → 复制
elif check_src_llama_arch; then
    log_info "使用包中已编译的 llama-server"
    deploy_llama_bin "$SRC_LLAMA_BIN"
    # 缓存二进制+共享库
    mkdir -p "$SCRIPT_DIR/llama.cpp/bin"
    cp "$SRC_LLAMA_BIN" "$LLAMA_PREBUILT"
    cp -a "$SRC_LLAMA_BUILD_DIR/bin"/lib*.so* "$SCRIPT_DIR/llama.cpp/bin/" 2>/dev/null || true

# 优先级4：从源码编译（在包目录中编译）
elif [ -d "$SCRIPT_DIR/llama.cpp" ]; then
    log_info "无预编译二进制，从源码编译..."
    bash "$SCRIPT_DIR/scripts/build.sh"
    if [ -x "$SRC_LLAMA_BIN" ]; then
        deploy_llama_bin "$SRC_LLAMA_BIN"
        # 缓存二进制+共享库
        mkdir -p "$SCRIPT_DIR/llama.cpp/bin"
        cp "$SRC_LLAMA_BIN" "$LLAMA_PREBUILT"
        cp -a "$SRC_LLAMA_BUILD_DIR/bin"/lib*.so* "$SCRIPT_DIR/llama.cpp/bin/" 2>/dev/null || true
        log_info "已保存预编译缓存: llama-server.$ARCH_SUFFIX (含共享库)"
    else
        log_warn "llama.cpp 编译失败"
    fi
else
    log_warn "llama.cpp 源码不存在，跳过"
    log_info "请手动放置预编译二进制到 $SCRIPT_DIR/llama.cpp/bin/"
fi

# ============================================================
# Step 4: 验证模型文件
# ============================================================
log_step "Step 4/8: 编译 YOLO Server"

# 路径定义（源码在包目录，运行时在安装目录）
SRC_YOLO_BIN="$SCRIPT_DIR/yolo-server/build/yolo-server"
DST_YOLO_BIN="$INSTALL_DIR/yolo-server/build/yolo-server"
YOLO_PREBUILT="$SCRIPT_DIR/yolo-server/bin/yolo-server.${ARCH_SUFFIX}"

# 部署 YOLO 运行时文件（二进制 + 当前平台的 ORT 库）
deploy_yolo_runtime() {
    local src_bin="$1"

    # 1. 部署二进制
    mkdir -p "$INSTALL_DIR/yolo-server/build"
    cp "$src_bin" "$DST_YOLO_BIN"
    chmod +x "$DST_YOLO_BIN"

    # 2. 部署 ORT 运行时库到 yolo-server/lib/（与 rpath=$ORIGIN/../lib 匹配）
    local ort_src=""
    if [[ "$ARCH" == "riscv64" ]]; then
        ort_src=$(ls -d "$SCRIPT_DIR/yolo-server/third_party/onnxruntime_spacemit"* 2>/dev/null | head -1)
    else
        ort_src=$(ls -d "$SCRIPT_DIR/yolo-server/third_party/onnxruntime_x86"* 2>/dev/null | head -1)
    fi

    if [ -n "$ort_src" ] && [ -d "$ort_src" ]; then
        mkdir -p "$INSTALL_DIR/yolo-server/lib"
        # 复制所有 .so 文件（保留符号链接）
        if [ -d "$ort_src/lib" ]; then
            cp -P "$ort_src/lib/"*.so* "$INSTALL_DIR/yolo-server/lib/" 2>/dev/null || true
        else
            # x86 目录可能直接放 .so 文件
            cp -P "$ort_src/"*.so* "$INSTALL_DIR/yolo-server/lib/" 2>/dev/null || true
        fi

        # ★ 自动创建缺失的 SONAME 符号链接（修复 x86 ORT 目录缺少 .so.1 的问题）
        # yolo-server 链接的是 SONAME libonnxruntime.so.1，源目录可能只有 .so 和 .so.1.x.x
        local lib_dir="$INSTALL_DIR/yolo-server/lib"
        if [ ! -e "$lib_dir/libonnxruntime.so.1" ]; then
            # 找到 libonnxruntime.so 指向的实际文件（即编译时使用的版本）
            local target=""
            if [ -L "$lib_dir/libonnxruntime.so" ]; then
                target=$(readlink "$lib_dir/libonnxruntime.so")
            fi
            # 如果 dev link 不存在或无效，选择版本号最高的 .so.1.x.x
            if [ -z "$target" ] || [ ! -e "$lib_dir/$target" ]; then
                target=$(ls -1 "$lib_dir"/libonnxruntime.so.1.* 2>/dev/null | sort -V | tail -1)
                target=$(basename "$target" 2>/dev/null)
            fi
            if [ -n "$target" ]; then
                ln -sf "$target" "$lib_dir/libonnxruntime.so.1"
                log_info "创建 SONAME 符号链接: libonnxruntime.so.1 -> $target"
            fi
        fi

        # 验证关键符号链接
        if [ ! -e "$lib_dir/libonnxruntime.so.1" ]; then
            log_warn "libonnxruntime.so.1 符号链接缺失！YOLO Server 可能无法启动"
        fi

        log_info "ORT 运行时库已部署: $lib_dir/"
        ls -la "$lib_dir"/libonnxruntime* 2>/dev/null | while read line; do log_info "  $line"; done
    else
        log_warn "未找到当前平台的 ORT 运行时库"
    fi

    log_success "YOLO Server 已部署到 $INSTALL_DIR/yolo-server/"
}

# 架构匹配检查（检查安装目录中的已有二进制）
check_yolo_arch() {
    if [ ! -x "$DST_YOLO_BIN" ]; then return 1; fi
    local BIN_ARCH=$(file "$DST_YOLO_BIN" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
    case "$ARCH" in
        riscv64) [ "$BIN_ARCH" = "RISC-V" ] ;;
        aarch64) [ "$BIN_ARCH" = "aarch64" ] ;;
        *)       [ "$BIN_ARCH" = "x86-64" ] ;;
    esac
}

# 检查包目录中已编译的二进制是否匹配
check_src_yolo_arch() {
    if [ ! -x "$SRC_YOLO_BIN" ]; then return 1; fi
    local BIN_ARCH=$(file "$SRC_YOLO_BIN" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
    case "$ARCH" in
        riscv64) [ "$BIN_ARCH" = "RISC-V" ] ;;
        aarch64) [ "$BIN_ARCH" = "aarch64" ] ;;
        *)       [ "$BIN_ARCH" = "x86-64" ] ;;
    esac
}

# 在包目录中编译 YOLO Server
compile_yolo_in_src() {
    cd "$SCRIPT_DIR/yolo-server"
    rm -rf build && mkdir -p build && cd build

    if [[ "$ARCH" == "riscv64" ]]; then
        log_info "RISC-V 架构，启用 RVV 优化..."
        cmake .. -DCMAKE_BUILD_TYPE=Release \
                 -DCMAKE_CXX_FLAGS="-march=rv64gcv -O3 -ffast-math"
    else
        cmake .. -DCMAKE_BUILD_TYPE=Release
    fi

    make -j$(nproc)

    if [ -x "$SRC_YOLO_BIN" ]; then
        # 保存为预编译缓存
        mkdir -p "$SCRIPT_DIR/yolo-server/bin"
        cp "$SRC_YOLO_BIN" "$YOLO_PREBUILT"
        log_info "已保存预编译缓存: yolo-server.$ARCH_SUFFIX"
        return 0
    else
        return 1
    fi
}

# 优先级0：--build 强制重新编译
if $FORCE_BUILD && [ -d "$SCRIPT_DIR/yolo-server" ] && [ -f "$SCRIPT_DIR/yolo-server/CMakeLists.txt" ]; then
    log_info "强制重新编译 YOLO Server (--build)..."
    if compile_yolo_in_src; then
        deploy_yolo_runtime "$SRC_YOLO_BIN"
    else
        log_warn "YOLO Server 编译失败"
    fi

# 优先级1：安装目录已有匹配二进制 → 跳过
elif check_yolo_arch; then
    log_info "已有 $ARCH_SUFFIX yolo-server，跳过"

# 优先级2：预编译缓存 → 直接部署
elif [ -f "$YOLO_PREBUILT" ]; then
    log_info "使用预编译缓存: yolo-server.$ARCH_SUFFIX"
    deploy_yolo_runtime "$YOLO_PREBUILT"

# 优先级3：包目录中已编译且匹配 → 复制
elif check_src_yolo_arch; then
    log_info "使用包中已编译的 yolo-server"
    deploy_yolo_runtime "$SRC_YOLO_BIN"
    mkdir -p "$SCRIPT_DIR/yolo-server/bin"
    cp "$SRC_YOLO_BIN" "$YOLO_PREBUILT"

# 优先级4：从源码编译（在包目录中）
elif [ -d "$SCRIPT_DIR/yolo-server" ] && [ -f "$SCRIPT_DIR/yolo-server/CMakeLists.txt" ]; then
    log_info "无预编译二进制，从源码编译..."
    if compile_yolo_in_src; then
        deploy_yolo_runtime "$SRC_YOLO_BIN"
    else
        log_warn "YOLO Server 编译失败"
    fi
else
    log_warn "YOLO Server 源码不存在，跳过"
fi

# ============================================================
# Step 5/8: 验证模型文件
# ============================================================
log_step "Step 5/8: 验证模型文件"

MODEL_COUNT=0
for f in "$INSTALL_DIR/models/"*.gguf; do
    [ -f "$f" ] || continue
    SIZE=$(du -h "$f" | cut -f1)
    NAME=$(basename "$f")
    echo "  ✅ $NAME ($SIZE)"
    MODEL_COUNT=$((MODEL_COUNT + 1))
done

# 检查 YOLO 模型
YOLO_MODEL_COUNT=0
for f in "$INSTALL_DIR/models/"*.onnx; do
    [ -f "$f" ] || continue
    SIZE=$(du -h "$f" | cut -f1)
    NAME=$(basename "$f")
    echo "  ✅ $NAME ($SIZE)"
    YOLO_MODEL_COUNT=$((YOLO_MODEL_COUNT + 1))
done

if [ $YOLO_MODEL_COUNT -eq 0 ]; then
    log_warn "未找到 YOLO 模型 (.onnx)"
    log_info "下载: wget -O $INSTALL_DIR/models/yolov8n.onnx https://github.com/ultralytics/assets/releases/download/v0.0.0/yolov8n.onnx"
else
    # 显示推荐模型
    if [[ "$ARCH" == "riscv64" ]] && [ -f "$INSTALL_DIR/models/910byolo11n_b2.q.onnx" ]; then
        log_success "K3 推荐模型: 910byolo11n_b2.q.onnx (SpacemiT NPU 加速)"
    fi
fi

if [ $MODEL_COUNT -eq 0 ]; then
    log_warn "未找到 VLM 模型文件，请将 .gguf 文件放入 $INSTALL_DIR/models/"
else
    log_success "找到 $MODEL_COUNT 个模型文件"
    
    # 自动选择默认模型
    if [ ! -f "$INSTALL_DIR/config/active-model.sh" ]; then
        if [ -f "$INSTALL_DIR/models/fastvlm-0.5b-q8_0.gguf" ]; then
            bash "$INSTALL_DIR/scripts/switch-model.sh" fastvlm-0.5b-q8
        elif [ -f "$INSTALL_DIR/models/fastvlm-0.5b-q4_k_m.gguf" ]; then
            bash "$INSTALL_DIR/scripts/switch-model.sh" fastvlm-0.5b-q4
        fi
    fi
fi

# ============================================================
# Step 5: 安装 Node Agent
# ============================================================
log_step "Step 6/8: 安装 Node Agent ($NODE_AGENT_TYPE)"

# ── 安装 Python 版本 ──────────────────────────────────────
install_agent_python() {
    local AGENT_DIR="$INSTALL_DIR/node-agent"
    if [ ! -d "$AGENT_DIR" ]; then
        log_warn "node-agent 目录不存在: $AGENT_DIR"
        return 1
    fi
    
    # 创建 venv
    if [ ! -d "$AGENT_DIR/venv" ]; then
        log_info "创建 Python 虚拟环境..."
        python3 -m venv "$AGENT_DIR/venv"
    fi
    
    # 安装依赖
    log_info "安装 Python 依赖..."
    "$AGENT_DIR/venv/bin/pip" install --upgrade pip -q
    
    # K3 RISC-V: 配置 SpacemiT 预编译源
    if [[ "$ARCH" == "riscv64" ]]; then
        log_info "配置 SpacemiT 预编译源 (RISC-V)..."
        "$AGENT_DIR/venv/bin/pip" config set global.extra-index-url \
            https://git.spacemit.com/api/v4/projects/33/packages/pypi/simple
        "$AGENT_DIR/venv/bin/pip" config set global.trusted-host git.spacemit.com
    fi
    
    "$AGENT_DIR/venv/bin/pip" install -r "$AGENT_DIR/requirements.txt" -q
    
    # 验证
    if "$AGENT_DIR/venv/bin/python" -c "import fastapi, uvicorn, psutil" 2>/dev/null; then
        PYDANTIC_VER=$("$AGENT_DIR/venv/bin/python" -c "import pydantic; print(pydantic.VERSION)")
        log_success "Node Agent (Python) 安装完成 (pydantic $PYDANTIC_VER)"
    else
        log_error "Python 依赖安装失败"
        return 1
    fi
    
    cp -f "$INSTALL_DIR/.env" "$AGENT_DIR/.env"
}

# ── 安装 Go 版本 ──────────────────────────────────────────
install_agent_go() {
    local SRC_AGENT_DIR="$SCRIPT_DIR/node-agent-go"
    local DST_AGENT_DIR="$INSTALL_DIR/node-agent-go"
    local DST_BINARY="$DST_AGENT_DIR/node-agent-go"
    local SRC_BINARY="$SRC_AGENT_DIR/node-agent-go"
    local PREBUILT="$SRC_AGENT_DIR/bin/node-agent-go.${ARCH_SUFFIX}"
    
    if [ ! -d "$DST_AGENT_DIR" ]; then
        log_warn "node-agent-go 目录不存在: $DST_AGENT_DIR"
        return 1
    fi
    
    # 检查架构匹配
    check_go_agent_arch() {
        [ ! -x "$DST_BINARY" ] && return 1
        local BIN_ARCH=$(file "$DST_BINARY" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
        case "$ARCH" in
            riscv64) [ "$BIN_ARCH" = "RISC-V" ] ;;
            aarch64) [ "$BIN_ARCH" = "aarch64" ] ;;
            *)       [ "$BIN_ARCH" = "x86-64" ] ;;
        esac
    }
    
    # 优先级1: 已有匹配二进制 → 跳过
    if check_go_agent_arch; then
        log_info "node-agent-go 已编译 ($ARCH_SUFFIX)"
    # 优先级2: 预编译缓存 → 部署
    elif [ -f "$PREBUILT" ]; then
        log_info "使用预编译二进制: node-agent-go.$ARCH_SUFFIX"
        cp "$PREBUILT" "$DST_BINARY"
        chmod +x "$DST_BINARY"
        log_success "node-agent-go 部署完成"
    # 优先级3: 源目录已编译 → 复制
    elif [ -x "$SRC_BINARY" ]; then
        log_info "复制已编译的 node-agent-go"
        cp "$SRC_BINARY" "$DST_BINARY"
        chmod +x "$DST_BINARY"
        # 缓存到 bin/
        mkdir -p "$SRC_AGENT_DIR/bin"
        cp "$SRC_BINARY" "$PREBUILT"
        log_success "node-agent-go 部署完成 (已缓存)"
    # 优先级4: 现场编译
    else
        log_info "编译 node-agent-go..."
        if ! command -v go &>/dev/null; then
            log_error "Go 未安装，无法编译"
            log_info "请安装: sudo apt install golang-go 或从 https://go.dev 下载"
            return 1
        fi
        cd "$DST_AGENT_DIR"
        go build -ldflags "-s -w" -o "$DST_BINARY" . || { log_error "编译失败"; return 1; }
        # 缓存到源目录
        mkdir -p "$SRC_AGENT_DIR/bin"
        cp "$DST_BINARY" "$PREBUILT"
        log_success "node-agent-go 编译完成 (已缓存: $PREBUILT)"
    fi
    
    cp -f "$INSTALL_DIR/.env" "$DST_AGENT_DIR/.env"
    log_success "Node Agent (Go) 安装完成"
}

# ── 安装 C++ 版本 ──────────────────────────────────────────
install_agent_cpp() {
    local SRC_AGENT_DIR="$SCRIPT_DIR/node-agent-cpp"
    local DST_AGENT_DIR="$INSTALL_DIR/node-agent-cpp"
    local DST_BINARY="$DST_AGENT_DIR/build/node-agent-cpp"
    local SRC_BINARY="$SRC_AGENT_DIR/build/node-agent-cpp"
    local PREBUILT="$SRC_AGENT_DIR/bin/node-agent-cpp.${ARCH_SUFFIX}"
    
    if [ ! -d "$DST_AGENT_DIR" ]; then
        log_warn "node-agent-cpp 目录不存在: $DST_AGENT_DIR"
        return 1
    fi
    
    # 检查架构匹配
    check_cpp_agent_arch() {
        [ ! -x "$DST_BINARY" ] && return 1
        local BIN_ARCH=$(file "$DST_BINARY" 2>/dev/null | grep -oP '(x86-64|RISC-V|aarch64)' | head -1)
        case "$ARCH" in
            riscv64) [ "$BIN_ARCH" = "RISC-V" ] ;;
            aarch64) [ "$BIN_ARCH" = "aarch64" ] ;;
            *)       [ "$BIN_ARCH" = "x86-64" ] ;;
        esac
    }
    
    # 优先级1: 已有匹配二进制 → 跳过
    if check_cpp_agent_arch; then
        log_info "node-agent-cpp 已编译 ($ARCH_SUFFIX)"
    # 优先级2: 预编译缓存 → 部署
    elif [ -f "$PREBUILT" ]; then
        log_info "使用预编译二进制: node-agent-cpp.$ARCH_SUFFIX"
        mkdir -p "$DST_AGENT_DIR/build"
        cp "$PREBUILT" "$DST_BINARY"
        chmod +x "$DST_BINARY"
        log_success "node-agent-cpp 部署完成"
    # 优先级3: 源目录已编译 → 复制
    elif [ -x "$SRC_BINARY" ]; then
        log_info "复制已编译的 node-agent-cpp"
        mkdir -p "$DST_AGENT_DIR/build"
        cp "$SRC_BINARY" "$DST_BINARY"
        chmod +x "$DST_BINARY"
        # 缓存到 bin/
        mkdir -p "$SRC_AGENT_DIR/bin"
        cp "$SRC_BINARY" "$PREBUILT"
        log_success "node-agent-cpp 部署完成 (已缓存)"
    # 优先级4: 现场编译
    else
        log_info "编译 node-agent-cpp..."
        cd "$DST_AGENT_DIR"
        mkdir -p build && cd build
        cmake .. || { log_error "cmake 失败"; return 1; }
        make -j$(nproc) || { log_error "编译失败"; return 1; }
        # 缓存到源目录
        mkdir -p "$SRC_AGENT_DIR/bin"
        cp "$DST_BINARY" "$PREBUILT"
        log_success "node-agent-cpp 编译完成 (已缓存: $PREBUILT)"
    fi
    
    cp -f "$INSTALL_DIR/.env" "$DST_AGENT_DIR/.env"
    log_success "Node Agent (C++) 安装完成"
}

# ── 根据选择安装对应版本 ──────────────────────────────────
case "$NODE_AGENT_TYPE" in
    python)
        install_agent_python
        ;;
    go)
        install_agent_go
        ;;
    cpp)
        install_agent_cpp
        ;;
    *)
        log_error "未知的 Node Agent 版本: $NODE_AGENT_TYPE"
        ;;
esac

# ============================================================
# Step 6: 配置 systemd 服务
# ============================================================
log_step "Step 7/8: 配置 systemd 服务"

# 更新服务文件中的路径
for svc in rivision-llama.service rivision-node-agent.service rivision-yolo.service; do
    if [ -f "$INSTALL_DIR/$svc" ]; then
        # 替换默认路径为实际安装路径
        sed -i "s|/home/node/rivision|$INSTALL_DIR|g" "$INSTALL_DIR/$svc"
        sed -i "s|User=node|User=$CURRENT_USER|g" "$INSTALL_DIR/$svc"
        sed -i "s|Group=node|Group=$CURRENT_USER|g" "$INSTALL_DIR/$svc"
        
        # 复制到 systemd
        sudo cp "$INSTALL_DIR/$svc" "/etc/systemd/system/$svc"
        log_success "$svc 已安装"
    fi
done

sudo systemctl daemon-reload

# 启用服务开机自启
for svc in rivision-llama rivision-node-agent rivision-yolo; do
    if [ -f "/etc/systemd/system/${svc}.service" ]; then
        sudo systemctl enable "$svc" 2>/dev/null || true
    fi
done

# 启动/重启服务
for svc in rivision-node-agent rivision-yolo rivision-llama; do
    if [ ! -f "/etc/systemd/system/${svc}.service" ]; then
        continue
    fi
    # 检查服务依赖是否就绪
    case "$svc" in
        rivision-yolo)
            [ -x "$DST_YOLO_BIN" ] || { log_warn "$svc: 二进制不存在，跳过启动"; continue; } ;;
        rivision-llama)
            [ -x "$DST_LLAMA_BIN" ] || { log_warn "$svc: 二进制不存在，跳过启动"; continue; } ;;
    esac
    sudo systemctl restart "$svc" 2>/dev/null
    if systemctl is-active --quiet "$svc"; then
        log_success "$svc 已启动"
    else
        log_warn "$svc 启动失败，请检查: journalctl -u $svc -n 20"
    fi
done

log_success "systemd 服务已配置、启用并启动"

# ============================================================
# Step 7: K3 系统优化
# ============================================================
log_step "Step 8/8: K3 系统优化"

if $SKIP_OPTIMIZE; then
    log_info "跳过系统优化 (--skip-optimize)"
elif [[ "$ARCH" == "riscv64" ]]; then
    sudo bash "$INSTALL_DIR/scripts/optimize.sh"
    log_success "K3 系统优化完成"
else
    log_info "非 RISC-V 架构，跳过 K3 优化"
fi

# ============================================================
# 部署验证
# ============================================================
log_step "部署验证"

echo ""
echo "  部署目录: $INSTALL_DIR"

# llama.cpp
if [[ "$ARCH" == "riscv64" ]]; then
    LLAMA_BUILD="$INSTALL_DIR/llama.cpp/build-k3"
else
    LLAMA_BUILD="$INSTALL_DIR/llama.cpp/build_x86"
fi
if [ -x "$LLAMA_BUILD/bin/llama-server" ] || [ -x "$LLAMA_BUILD/bin/llama-mtmd-cli" ]; then
    log_success "llama.cpp 可执行文件就绪"
else
    log_warn "llama.cpp 未编译，请运行: scripts/build.sh"
fi

# YOLO Server
if [ -x "$INSTALL_DIR/yolo-server/build/yolo-server" ]; then
    log_success "YOLO Server 可执行文件就绪"
else
    log_warn "YOLO Server 未编译"
fi

# 模型
echo "  VLM 模型: $MODEL_COUNT 个 GGUF 文件"
if [ -f "$INSTALL_DIR/models/yolov8n.onnx" ]; then
    log_success "YOLO 模型就绪"
else
    log_warn "YOLO 模型缺失"
fi

# Node Agent
AGENT_READY=false
case "$NODE_AGENT_TYPE" in
    python)
        if [ -f "$INSTALL_DIR/node-agent/venv/bin/uvicorn" ]; then
            AGENT_READY=true
        fi
        ;;
    go)
        if [ -x "$INSTALL_DIR/node-agent-go/node-agent-go" ]; then
            AGENT_READY=true
        fi
        ;;
    cpp)
        if [ -x "$INSTALL_DIR/node-agent-cpp/build/node-agent-cpp" ]; then
            AGENT_READY=true
        fi
        ;;
esac
if $AGENT_READY; then
    log_success "Node Agent ($NODE_AGENT_TYPE) 就绪"
else
    log_warn "Node Agent ($NODE_AGENT_TYPE) 未安装"
fi

# systemd
SVC_COUNT=0
for svc in rivision-llama rivision-node-agent rivision-yolo; do
    if [ -f "/etc/systemd/system/${svc}.service" ]; then
        SVC_COUNT=$((SVC_COUNT + 1))
    fi
done
log_success "systemd 服务已配置 ($SVC_COUNT/3)"

echo ""
echo "╔══════════════════════════════════════════════╗"
echo "║   部署完成!                                  ║"
echo "╠══════════════════════════════════════════════╣"
echo "║                                              ║"
echo "║  当前 Node Agent 版本: $NODE_AGENT_TYPE"
echo "║                                              ║"
echo "║  启动服务:                                    ║"
echo "║    sudo systemctl start rivision-llama       ║"
echo "║    sudo systemctl start rivision-node-agent  ║"
echo "║    sudo systemctl start rivision-yolo        ║"
echo "║                                              ║"
echo "║  切换Agent版本:                               ║"
echo "║    scripts/switch-agent.sh --status          ║"
echo "║    scripts/switch-agent.sh go                ║"
echo "║    scripts/switch-agent.sh cpp               ║"
echo "║                                              ║"
echo "║  切换VLM模型:                                  ║"
echo "║    scripts/switch-model.sh --list            ║"
echo "║    scripts/switch-model.sh fastvlm-0.5b-q8   ║"
echo "║                                              ║"
echo "║  切换YOLO模型:                               ║"
echo "║    scripts/switch-yolo-model.sh --list       ║"
echo "║    scripts/switch-yolo-model.sh yolo11n      ║"
echo "║                                              ║"
echo "║  测试推理:                                    ║"
echo "║    scripts/test.sh test.jpg      # VLM      ║"
echo "║    scripts/test-yolo.py test.jpg # YOLO     ║"
echo "║                                              ║"
echo "║  查看日志:                                    ║"
echo "║    journalctl -u rivision-llama -f           ║"
echo "║    journalctl -u rivision-node-agent -f      ║"
echo "║    journalctl -u rivision-yolo -f            ║"
echo "║                                              ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
