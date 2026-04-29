#!/bin/bash
# ============================================================
# RiVision 单组件安装脚-本
# 用于单独安装/升级某个组件
# ============================================================

set -e

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_BASE="/opt/rivision"
AUTO_START=true

# 获取本机IP地址 (排除 VPN/代理/虚拟网卡)
get_local_ip() {
    local ip=""
    # 优先获取物理网卡 IP (排除 docker/virbr/veth/lo/tun/Mihomo 等虚拟接口)
    ip=$(ip -4 addr show scope global | grep -v -E 'docker|virbr|veth|tun|Mihomo|br-' | grep -oP 'inet \K[0-9.]+' | grep -v '^172\.' | grep -v '^198\.18\.' | head -1)
    if [ -z "$ip" ]; then
        # 备选: hostname -I 第一个
        ip=$(hostname -I 2>/dev/null | awk '{print $1}')
    fi
    echo "${ip:-127.0.0.1}"
}

# 自动检测组件名称
detect_component() {
    local dir_name=$(basename "$SCRIPT_DIR")
    case "$dir_name" in
        rivision_cli|rivision_gateway|rivision_embed|rivision_owl|rivision_node)
            echo "$dir_name"
            ;;
        *)
            echo ""
            ;;
    esac
}

COMPONENT=$(detect_component)

usage() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -c, --component NAME    指定组件名称 (rivision_cli|rivision_gateway|rivision_embed|rivision_owl|rivision_node)"
    echo "  -d, --dest DIR          安装目录 (默认: /opt/rivision)"
    echo "  -h, --help              显示帮助"
    echo ""
    echo "示例:"
    echo "  $0                      # 自动检测组件并安装"
    echo "  $0 -c rivision_cli      # 安装 CLI 组件"
    exit 0
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -c|--component) COMPONENT="$2"; shift 2 ;;
        -d|--dest) INSTALL_BASE="$2"; shift 2 ;;
        -h|--help) usage ;;
        --auto-start) AUTO_START=true; shift ;;
        *) echo -e "${RED}未知参数: $1${NC}"; usage ;;
    esac
done

if [ -z "$COMPONENT" ]; then
    echo -e "${RED}错误: 无法检测组件名称，请使用 -c 指定${NC}"
    usage
fi

INSTALL_DIR="$INSTALL_BASE/$COMPONENT"
BACKUP_DIR="$INSTALL_BASE/backup/${COMPONENT}-$(date +%Y%m%d-%H%M%S)"

echo -e "${GREEN}=== RiVision 单组件安装 ===${NC}"
LOCAL_IP=$(get_local_ip)
echo ""
echo "  组件: $COMPONENT"
echo "  来源: $SCRIPT_DIR"
echo "  目标: $INSTALL_DIR"
echo "  本机IP: $LOCAL_IP"
echo ""

# 自动提权：非 root 时用 sudo 重新执行
if [ "$EUID" -ne 0 ]; then
    exec sudo "$0" "$@"
fi

# 检查是否升级
IS_UPGRADE=false
if [ -d "$INSTALL_DIR" ]; then
    IS_UPGRADE=true
    echo -e "${YELLOW}检测到已有安装，将执行升级...${NC}"
fi

# 停止服务
SERVICE_NAME=""
case "$COMPONENT" in
    rivision_cli) SERVICE_NAME="rivision-cli" ;;
    rivision_gateway) SERVICE_NAME="rivision-gateway" ;;
    rivision_embed) SERVICE_NAME="rivision-embed" ;;
    rivision_owl) SERVICE_NAME="rivision-owl" ;;
    rivision_node) SERVICE_NAME="rivision-node-agent" ;;
esac

# 记录服务原始状态，升级后恢复
SERVICE_WAS_RUNNING=false
if [ -n "$SERVICE_NAME" ] && systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    SERVICE_WAS_RUNNING=true
    echo "停止服务: $SERVICE_NAME"
    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
fi

# 保留配置（不备份整个目录）
PRESERVED_ENV=""
if $IS_UPGRADE && [ -f "$INSTALL_DIR/config/.env" ]; then
    PRESERVED_ENV=$(cat "$INSTALL_DIR/config/.env")
fi

# 创建目录
mkdir -p "$INSTALL_DIR"/{bin,config,logs}
mkdir -p "$INSTALL_BASE/shared"

# 初始化 shared/.env (首次安装时从模板复制)
if [ ! -f "$INSTALL_BASE/shared/.env" ]; then
    if [ -f "$SCRIPT_DIR/.env.template" ]; then
        cp "$SCRIPT_DIR/.env.template" "$INSTALL_BASE/shared/.env"
        # 自动填充 OWL_SDP_IP
        sed -i "s/^OWL_SDP_IP=$/OWL_SDP_IP=$LOCAL_IP/" "$INSTALL_BASE/shared/.env"
        echo "  已初始化: shared/.env (从模板生成)"
    fi
fi

# Gateway 需要 data 目录
if [ "$COMPONENT" = "rivision_gateway" ]; then
    mkdir -p "$INSTALL_DIR/data/cache"
fi

# 安装文件
echo "安装组件文件..."

# 复制 bin
if [ -d "$SCRIPT_DIR/bin" ]; then
    cp -r "$SCRIPT_DIR/bin/"* "$INSTALL_DIR/bin/" 2>/dev/null || true
    chmod +x "$INSTALL_DIR/bin/"* 2>/dev/null || true
    echo "  已安装: bin/"
fi

# 复制 config (包括隐藏文件如 .env)
if [ -d "$SCRIPT_DIR/config" ]; then
    # 不覆盖已有的 .env
    shopt -s dotglob nullglob  # 匹配隐藏文件，空目录不报错
    for f in "$SCRIPT_DIR/config/"*; do
        [ -e "$f" ] || continue  # 跳过不存在的文件
        fname=$(basename "$f")
        if [ "$fname" = ".env" ] && [ -f "$INSTALL_DIR/config/.env" ]; then
            echo "  跳过: config/.env (保留现有配置)"
        else
            cp -r "$f" "$INSTALL_DIR/config/"
            echo "  已安装: config/$fname"
        fi
    done
    shopt -u dotglob nullglob  # 恢复默认
fi

# 复制 lib
if [ -d "$SCRIPT_DIR/lib" ]; then
    mkdir -p "$INSTALL_DIR/lib"
    cp -r "$SCRIPT_DIR/lib/"* "$INSTALL_DIR/lib/" 2>/dev/null || true
    echo "  已安装: lib/"
    # 配置 ldconfig
    echo "$INSTALL_DIR/lib" > "/etc/ld.so.conf.d/$COMPONENT.conf"
    ldconfig 2>/dev/null || true
fi

# 复制 models
if [ -d "$SCRIPT_DIR/models" ]; then
    mkdir -p "$INSTALL_DIR/models"
    cp -r "$SCRIPT_DIR/models/"* "$INSTALL_DIR/models/" 2>/dev/null || true
    echo "  已安装: models/"
fi

# 特殊处理: OWL 额外目录
if [ "$COMPONENT" = "rivision_owl" ]; then
    for d in data recordings; do
        mkdir -p "$INSTALL_DIR/$d"
    done
    # 复制 www 目录 (web 前端)
    if [ -d "$SCRIPT_DIR/www" ]; then
        mkdir -p "$INSTALL_DIR/www"
        cp -r "$SCRIPT_DIR/www/"* "$INSTALL_DIR/www/" 2>/dev/null || true
        echo "  已安装: www/"
    fi
    # 复制 mp4path 目录
    if [ -d "$SCRIPT_DIR/mp4path" ]; then
        mkdir -p "$INSTALL_DIR/mp4path"
        cp -r "$SCRIPT_DIR/mp4path/"* "$INSTALL_DIR/mp4path/" 2>/dev/null || true
        echo "  已安装: mp4path/"
    fi
    
    # 提示用户配置 OWL_SDP_IP
    echo ""
    echo -e "${YELLOW}提示: 请配置 OWL_SDP_IP 以支持 GB28181 设备推流${NC}"
    echo "  编辑 /opt/rivision/shared/.env 添加: OWL_SDP_IP=$LOCAL_IP"
fi

# 特殊处理: CLI 额外文件
if [ "$COMPONENT" = "rivision_cli" ]; then
    # 复制根目录的 go2rtc.yaml
    if [ -f "$SCRIPT_DIR/go2rtc.yaml" ]; then
        cp "$SCRIPT_DIR/go2rtc.yaml" "$INSTALL_DIR/"
        echo "  已安装: go2rtc.yaml"
    fi
    # 复制 mp4path 目录及测试视频
    if [ -d "$SCRIPT_DIR/mp4path" ]; then
        mkdir -p "$INSTALL_DIR/mp4path"
        cp -r "$SCRIPT_DIR/mp4path/"* "$INSTALL_DIR/mp4path/" 2>/dev/null || true
        echo "  已安装: mp4path/"
    fi
    # 复制 data 目录
    if [ -d "$SCRIPT_DIR/data" ]; then
        mkdir -p "$INSTALL_DIR/data"
        cp -r "$SCRIPT_DIR/data/"* "$INSTALL_DIR/data/" 2>/dev/null || true
        echo "  已安装: data/"
    fi
fi

# 特殊处理: Node 额外目录
if [ "$COMPONENT" = "rivision_node" ]; then
    for d in engines scripts; do
        if [ -d "$SCRIPT_DIR/$d" ]; then
            mkdir -p "$INSTALL_DIR/$d"
            cp -r "$SCRIPT_DIR/$d/"* "$INSTALL_DIR/$d/" 2>/dev/null || true
            echo "  已安装: $d/"
        fi
    done
fi

# 安装 systemd service
if [ -d "$SCRIPT_DIR/services" ]; then
    # 获取安装用户 (优先使用 SUDO_USER，否则用 root)
    INSTALL_USER="${SUDO_USER:-root}"
    INSTALL_GROUP="$(id -gn "$INSTALL_USER" 2>/dev/null || echo root)"
    
    for svc in "$SCRIPT_DIR/services/"*.service; do
        if [ -f "$svc" ]; then
            svc_name=$(basename "$svc")
            # 替换占位符并安装
            sed -e "s/RIVISION_USER_PLACEHOLDER/$INSTALL_USER/g" \
                -e "s/RIVISION_GROUP_PLACEHOLDER/$INSTALL_GROUP/g" \
                "$svc" > "/etc/systemd/system/$svc_name"
            chmod 644 "/etc/systemd/system/$svc_name"
            echo "  已安装: $svc_name (User=$INSTALL_USER)"
        fi
    done
    systemctl daemon-reload 2>/dev/null || true
fi

# 恢复配置
if $IS_UPGRADE && [ -n "$PRESERVED_ENV" ]; then
    echo "$PRESERVED_ENV" > "$INSTALL_DIR/config/.env"
    echo "  已恢复: config/.env"
fi

# 设置目录所有权 (服务以 SUDO_USER 运行，需要写入权限)
if [ -n "$SUDO_USER" ]; then
    chown -R "$SUDO_USER:$(id -gn "$SUDO_USER")" "$INSTALL_DIR"
    chown -R "$SUDO_USER:$(id -gn "$SUDO_USER")" "$INSTALL_BASE/shared"
    echo "  已设置目录所有权: $SUDO_USER"
else
    echo -e "  ${YELLOW}警告: SUDO_USER 未设置，目录所有者为 root${NC}"
    echo "  如需修复: sudo chown -R <用户名> $INSTALL_DIR $INSTALL_BASE/shared"
fi

# 显示结果
echo ""
echo -e "${GREEN}=== 安装完成 ===${NC}"
echo "  组件: $COMPONENT"
echo "  位置: $INSTALL_DIR"
echo ""
ls -la "$INSTALL_DIR/bin/" 2>/dev/null || true
echo ""

# 启用开机自启 (始终执行)
if [ -n "$SERVICE_NAME" ]; then
    systemctl enable "$SERVICE_NAME" 2>/dev/null || true
    echo -e "  ${GREEN}已启用开机自启: $SERVICE_NAME${NC}"
    
    # 启动服务: AUTO_START=true 或 升级时服务原本在运行
    if $AUTO_START || $SERVICE_WAS_RUNNING; then
        echo "启动服务: $SERVICE_NAME"
        systemctl start "$SERVICE_NAME" 2>/dev/null || true
        sleep 1
        if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
            echo -e "  ${GREEN}已启动${NC}"
        else
            echo -e "  ${RED}启动失败，请检查日志: journalctl -u $SERVICE_NAME -n 50${NC}"
        fi
    else
        echo ""
        echo "启动服务:"
        echo "  systemctl start $SERVICE_NAME"
    fi
fi

