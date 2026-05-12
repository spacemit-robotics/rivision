<script setup lang="ts">
import { useAlertsStore } from '../../stores/alerts'
const alertsStore = useAlertsStore()
</script>

<template>
  <header class="app-header">
    <div class="logo">
      <img src="/favicon.ico" alt="RiVision" width="24" height="24" />
      <span>RiVision</span>
    </div>
    <nav class="nav-links">
      <router-link to="/">仪表盘</router-link>
      <router-link to="/live">实时监控</router-link>
      <router-link to="/search">搜索</router-link>
      <router-link to="/alerts">
        告警
        <span class="badge" v-if="alertsStore.unreadCount > 0">{{ alertsStore.unreadCount }}</span>
      </router-link>
    </nav>
    <div class="user-menu">
      <router-link to="/settings">设置</router-link>
    </div>
  </header>
</template>

<style scoped>
.app-header { display: flex; align-items: center; padding: 0 16px; height: 56px; background: #fff; border-bottom: 1px solid #eee; }
.logo { display: flex; align-items: center; gap: 8px; font-weight: bold; font-size: 18px; }
.nav-links { display: flex; gap: 16px; margin-left: 32px; }
.nav-links a { color: #666; text-decoration: none; padding: 4px 8px; border-radius: 4px; position: relative; }
.nav-links a.router-link-active { color: #409eff; font-weight: 500; }
.badge { position: absolute; top: -4px; right: -8px; background: #f56c6c; color: #fff; font-size: 10px; padding: 1px 5px; border-radius: 10px; }
.user-menu { margin-left: auto; }
</style>
