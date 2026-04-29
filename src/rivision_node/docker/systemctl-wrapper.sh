#!/bin/bash
# ============================================================
# Docker systemctl wrapper → supervisorctl
#
# node-agent 的 process_manager.py 调用 sudo systemctl 管理进程
# 在 Docker 中无 systemd，用 supervisorctl 替代
# ============================================================

ACTION="$1"
SERVICE="$2"

# 映射服务名到 supervisord program 名
case "$SERVICE" in
    rivision-llama*)  PROG="llama-server" ;;
    rivision-node*)   PROG="node-agent" ;;
    *)                PROG="$SERVICE" ;;
esac

case "$ACTION" in
    start|stop|restart)
        supervisorctl "$ACTION" "$PROG" 2>/dev/null
        exit $?
        ;;
    status)
        supervisorctl status "$PROG" 2>/dev/null
        exit $?
        ;;
    daemon-reload)
        # no-op in Docker
        exit 0
        ;;
    *)
        echo "systemctl (Docker wrapper): unsupported action '$ACTION'" >&2
        exit 1
        ;;
esac
