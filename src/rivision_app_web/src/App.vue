<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { useNodesStore } from './stores/nodes'

const router = useRouter()
const authStore = useAuthStore()
const nodesStore = useNodesStore()

const sidebarOpen = ref(true)
const currentTime = ref(new Date().toLocaleString('zh-CN'))

let timeInterval: number

onMounted(() => {
  timeInterval = setInterval(() => {
    currentTime.value = new Date().toLocaleString('zh-CN')
  }, 1000)
  
  // 如果已登录，获取节点列表
  if (authStore.isAuthenticated) {
    nodesStore.fetchNodes()
  }
})

onUnmounted(() => {
  clearInterval(timeInterval)
})

const navItems = [
  { name: '仪表盘', path: '/', icon: '📊' },
  { name: '节点管理', path: '/nodes', icon: '🖥️' },
  { name: '摄像头', path: '/cameras', icon: '📹' },
  { name: '智能搜索', path: '/search', icon: '🔍' },
  { name: '事件告警', path: '/events', icon: '🔔' },
  { name: '规则配置', path: '/rules', icon: '⚙️' },
  { name: '知识库', path: '/knowledge', icon: '📚' },
  { name: '统计分析', path: '/analytics', icon: '📈' },
]

const handleLogout = () => {
  authStore.logout()
  router.push('/login')
}
</script>

<template>
  <div class="app-container" v-if="authStore.isAuthenticated">
    <!-- 侧边栏 -->
    <aside :class="['sidebar', { collapsed: !sidebarOpen }]">
      <div class="logo">
        <span class="logo-icon">🎯</span>
        <span v-if="sidebarOpen" class="logo-text">RiVision</span>
      </div>
      
      <nav class="nav-menu">
        <router-link 
          v-for="item in navItems" 
          :key="item.path"
          :to="item.path"
          class="nav-item"
        >
          <span class="nav-icon">{{ item.icon }}</span>
          <span v-if="sidebarOpen" class="nav-text">{{ item.name }}</span>
        </router-link>
      </nav>
      
      <div class="sidebar-footer">
        <button @click="sidebarOpen = !sidebarOpen" class="toggle-btn">
          {{ sidebarOpen ? '◀' : '▶' }}
        </button>
      </div>
    </aside>

    <!-- 主内容区 -->
    <main class="main-content">
      <!-- 顶部栏 -->
      <header class="top-bar">
        <div class="top-left">
          <h1 class="page-title">边缘AI视频分析平台</h1>
        </div>
        <div class="top-right">
          <span class="time">{{ currentTime }}</span>
          <span class="user-info">
            <span class="user-avatar">👤</span>
            <span>{{ authStore.user?.username || 'Admin' }}</span>
          </span>
          <button @click="handleLogout" class="logout-btn">退出</button>
        </div>
      </header>

      <!-- 页面内容 -->
      <div class="page-content">
        <router-view />
      </div>
    </main>
  </div>

  <!-- 未登录显示登录页 -->
  <router-view v-else />
</template>

<style scoped>
.app-container {
  display: flex;
  min-height: 100vh;
  background: #f0f2f5;
}

.sidebar {
  width: 240px;
  background: linear-gradient(180deg, #1a1a2e 0%, #16213e 100%);
  color: white;
  display: flex;
  flex-direction: column;
  transition: width 0.3s ease;
}

.sidebar.collapsed {
  width: 64px;
}

.logo {
  display: flex;
  align-items: center;
  padding: 20px;
  border-bottom: 1px solid rgba(255,255,255,0.1);
}

.logo-icon {
  font-size: 28px;
}

.logo-text {
  margin-left: 12px;
  font-size: 20px;
  font-weight: bold;
}

.nav-menu {
  flex: 1;
  padding: 16px 8px;
}

.nav-item {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  color: rgba(255,255,255,0.7);
  text-decoration: none;
  border-radius: 8px;
  margin-bottom: 4px;
  transition: all 0.2s;
}

.nav-item:hover {
  background: rgba(255,255,255,0.1);
  color: white;
}

.nav-item.router-link-active {
  background: #3b82f6;
  color: white;
}

.nav-icon {
  font-size: 20px;
}

.nav-text {
  margin-left: 12px;
}

.sidebar-footer {
  padding: 16px;
  border-top: 1px solid rgba(255,255,255,0.1);
}

.toggle-btn {
  width: 100%;
  padding: 8px;
  background: rgba(255,255,255,0.1);
  border: none;
  border-radius: 4px;
  color: white;
  cursor: pointer;
}

.main-content {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.top-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 24px;
  background: white;
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: #1a1a2e;
}

.top-right {
  display: flex;
  align-items: center;
  gap: 20px;
}

.time {
  color: #666;
  font-size: 14px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
}

.user-avatar {
  font-size: 20px;
}

.logout-btn {
  padding: 6px 16px;
  background: #ef4444;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.logout-btn:hover {
  background: #dc2626;
}

.page-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}
</style>
