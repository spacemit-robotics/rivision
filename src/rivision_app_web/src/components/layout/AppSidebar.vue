<script setup lang="ts">
const menuItems = [
  { path: '/', label: '仪表盘', icon: '📊' },
  { path: '/live', label: '实时监控', icon: '📹' },
  { path: '/search', label: '智能搜索', icon: '🔍' },
  { path: '/alerts', label: '告警管理', icon: '🔔' },
  { path: '/cameras', label: '摄像头', icon: '📷' },
  { path: '/nodes', label: '节点管理', icon: '🖥' },
  { path: '/knowledge', label: '知识库', icon: '📚' },
  { path: '/analytics', label: '数据分析', icon: '📈' },
  { path: '/settings', label: '系统设置', icon: '⚙' },
]
</script>

<template>
  <aside class="app-sidebar">
    <nav>
      <router-link v-for="item in menuItems" :key="item.path" :to="item.path" class="menu-item">
        <span class="icon">{{ item.icon }}</span>
        <span class="label">{{ item.label }}</span>
      </router-link>
    </nav>
  </aside>
</template>

<style scoped>
.app-sidebar { width: 200px; background: #f8f9fa; border-right: 1px solid #eee; padding: 12px 0; }
.menu-item { display: flex; align-items: center; gap: 8px; padding: 10px 16px; color: #333; text-decoration: none; transition: background 0.2s; }
.menu-item:hover { background: #e8f0fe; }
.menu-item.router-link-exact-active { background: #e8f0fe; color: #409eff; font-weight: 500; }
.icon { font-size: 16px; }
</style>
