// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

import { createRouter, createWebHistory } from 'vue-router';
import { useAuth } from '../composables/useAuth';
import Login from '../views/Login.vue';
import MonitorDashboard from '../views/MonitorDashboard.vue';

const routes = [
  {
    path: '/',
    redirect: '/monitor'
  },
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { requiresGuest: true }
  },
  {
    path: '/monitor',
    name: 'MonitorDashboard',
    component: MonitorDashboard,
    meta: { requiresAuth: false, title: '视频监控', icon: 'VideoCamera' }
  },
  {
    path: '/dashboard',
    name: 'SystemDashboard',
    component: () => import('../views/SystemDashboard.vue'),
    meta: { requiresAuth: false, title: '系统仪表盘', icon: 'Dashboard' }
  },
  {
    path: '/nodes',
    name: 'NodeManagement',
    component: () => import('../views/NodeManagement.vue'),
    meta: { requiresAuth: false, title: '节点管理', icon: 'Monitor' }
  },
  {
    path: '/tasks',
    name: 'TaskMonitor',
    component: () => import('../views/TaskMonitor.vue'),
    meta: { requiresAuth: false, title: '任务监控', icon: 'DataLine' }
  },
  {
    path: '/analysis',
    name: 'VideoAnalysis',
    component: () => import('../views/VideoAnalysis.vue'),
    meta: { requiresAuth: false, title: '视频分析', icon: 'Film' }
  },
  {
    path: '/settings',
    name: 'Settings',
    component: () => import('../views/Settings.vue'),
    meta: { requiresAuth: false, title: '系统设置', icon: 'Setting' }
  }
];

const router = createRouter({
  history: createWebHistory(),
  routes
});

router.beforeEach((to) => {
  const { isAuthenticated } = useAuth();

  if (to.meta.requiresAuth && !isAuthenticated.value) {
    return '/login';
  }

  if (to.meta.requiresGuest && isAuthenticated.value) {
    return '/monitor';
  }
});

export default router;