#!/bin/bash
# ============================================================
# SpacemiT K3 系统层优化脚本
# 需要 root 权限执行: sudo bash optimize.sh
# ============================================================
set -e

echo "=== SpacemiT K3 系统层优化 ==="

# 1. CPU 调速器设置为 performance
echo ">>> 设置 CPU 调速器为 performance 模式"
for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    if [ -f "$cpu" ]; then
        echo performance > "$cpu" 2>/dev/null || true
    fi
done

# 2. 禁用 CPU 深度空闲状态（降低延迟）
echo ">>> 禁用深度 CPU 空闲状态"
for cpu in /sys/devices/system/cpu/cpu*/cpuidle/state*/disable; do
    if [ -f "$cpu" ]; then
        echo 1 > "$cpu" 2>/dev/null || true
    fi
done

# 3. 内核参数优化
echo ">>> 优化内核参数"
sysctl -w vm.swappiness=10 2>/dev/null || true
sysctl -w vm.dirty_ratio=20 2>/dev/null || true
sysctl -w vm.dirty_background_ratio=5 2>/dev/null || true

# 4. 透明大页
echo ">>> 配置透明大页"
echo madvise > /sys/kernel/mm/transparent_hugepage/enabled 2>/dev/null || true

# 5. 内存锁定限制
echo ">>> 设置内存锁定限制"
ulimit -l unlimited 2>/dev/null || true

echo ""
echo "=== K3 系统优化完成 ==="
echo ""
echo "推荐的 grub/bootargs 参数（需重启生效）:"
echo "  isolcpus=8-15 nohz_full=8-15 rcu_nocbs=8-15"
echo ""
echo "验证:"
echo "  cat /sys/devices/system/cpu/cpu8/cpufreq/scaling_governor"
