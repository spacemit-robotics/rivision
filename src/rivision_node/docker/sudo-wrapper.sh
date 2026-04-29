#!/bin/bash
# Docker sudo wrapper - 容器内已是 root，直接执行命令
exec "$@"
