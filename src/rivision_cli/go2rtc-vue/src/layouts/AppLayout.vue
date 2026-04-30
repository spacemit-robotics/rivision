// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="app-layout">
    <!-- 侧边栏 -->
    <aside class="sidebar">
      <div class="sidebar-header">
        <span class="logo">🎬</span>
        <span class="title">分布式算力网关</span>
      </div>
      
      <div class="sidebar-section">
        <div class="section-title">
          <el-icon><Connection /></el-icon>
          <span>Gateway</span>
        </div>
        
        <nav class="nav-menu">
          <router-link to="/dashboard" class="nav-item" :class="{ active: $route.path === '/dashboard' }">
            <el-icon><DataBoard /></el-icon>
            <span>仪表盘</span>
          </router-link>
          <router-link to="/tasks" class="nav-item" :class="{ active: $route.path === '/tasks' }">
            <el-icon><List /></el-icon>
            <span>任务监控</span>
          </router-link>
          <router-link to="/nodes" class="nav-item" :class="{ active: $route.path === '/nodes' }">
            <el-icon><Monitor /></el-icon>
            <span>节点管理</span>
            <span v-if="offlineCount > 0" class="badge error">{{ offlineCount }}</span>
          </router-link>
          <router-link to="/settings" class="nav-item" :class="{ active: $route.path === '/settings' }">
            <el-icon><Setting /></el-icon>
            <span>系统设置</span>
          </router-link>
        </nav>
      </div>
    </aside>

    <!-- 主内容区 -->
    <div class="main-container">
      <!-- 顶部栏 -->
      <header class="topbar">
        <div class="topbar-left">
          <div class="connection-status" :class="connectionClass">
            <span class="status-dot"></span>
            <span class="status-text">{{ connectionText }}</span>
          </div>
          <el-button v-if="!isConnected" size="small" type="primary" @click="reconnect">
            重新连接
          </el-button>
        </div>
        <div class="topbar-right">
          <div v-if="nodeStats" class="node-stats">
            <el-icon><Monitor /></el-icon>
            <span>{{ nodeStats.healthy }}/{{ nodeStats.total }}</span>
          </div>
        </div>
      </header>

      <!-- 页面内容 -->
      <main class="main-content">
        <slot></slot>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { Connection, DataBoard, List, Monitor, Setting } from '@element-plus/icons-vue'

const gatewayStore = useGatewayStore()

const isConnected = computed(() => gatewayStore.connectionStatus === 'connected')
const connectionClass = computed(() => ({
  connected: gatewayStore.connectionStatus === 'connected',
  connecting: gatewayStore.connectionStatus === 'connecting',
  disconnected: gatewayStore.connectionStatus === 'disconnected',
  error: gatewayStore.connectionStatus === 'error'
}))
const connectionText = computed(() => {
  switch (gatewayStore.connectionStatus) {
    case 'connected': return '已连接'
    case 'connecting': return '连接中...'
    case 'disconnected': return '未连接'
    case 'error': return '连接失败'
    default: return '未知'
  }
})

const nodeStats = computed(() => gatewayStore.nodeStatusSummary)
const offlineCount = computed(() => gatewayStore.nodeStatusSummary.offline)

const reconnect = () => {
  gatewayStore.initialize()
}
</script>

<style scoped>
.app-layout {
  display: flex;
  min-height: 100vh;
  background: #f5f7fa;
}

/* 侧边栏 */
.sidebar {
  width: 200px;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.sidebar-header {
  padding: 20px 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid #e4e7ed;
}

.sidebar-header .logo {
  font-size: 24px;
}

.sidebar-header .title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.sidebar-section {
  padding: 16px 0;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  color: #909399;
  font-size: 12px;
  text-transform: uppercase;
}

.nav-menu {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 0 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 6px;
  color: #606266;
  text-decoration: none;
  font-size: 14px;
  transition: all 0.2s;
}

.nav-item:hover {
  background: #f5f7fa;
  color: #409eff;
}

.nav-item.active {
  background: #ecf5ff;
  color: #409eff;
}

.nav-item .badge {
  margin-left: auto;
  padding: 2px 6px;
  border-radius: 10px;
  font-size: 12px;
  color: #fff;
}

.nav-item .badge.error {
  background: #f56c6c;
}

/* 主容器 */
.main-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

/* 顶部栏 */
.topbar {
  height: 56px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  flex-shrink: 0;
}

.topbar-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 16px;
  font-size: 13px;
}

.connection-status.connected {
  background: #f0f9eb;
  color: #67c23a;
}

.connection-status.connecting {
  background: #fdf6ec;
  color: #e6a23c;
}

.connection-status.disconnected,
.connection-status.error {
  background: #fef0f0;
  color: #f56c6c;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.node-stats {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #606266;
  font-size: 14px;
}

/* 主内容 */
.main-content {
  flex: 1;
  padding: 24px;
  overflow: auto;
}
</style>
