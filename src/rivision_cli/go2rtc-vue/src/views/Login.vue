// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <div class="login-container">
    <div class="login-form">
      <div class="login-header">
        <h1>监控系统登录</h1>
        <p>请使用您的账号登录</p>
      </div>

      <form @submit.prevent="handleLogin" class="login-form-content">
        <div class="form-group">
          <label class="form-label">用户名</label>
          <input
              v-model="credentials.username"
              type="text"
              required
              class="form-input"
              placeholder="请输入用户名"
          />
        </div>

        <div class="form-group">
          <label class="form-label">密码</label>
          <div class="password-container">
            <input
                v-model="credentials.password"
                :type="isPasswordVisible ? 'text' : 'password'"
                required
                class="form-input"
                placeholder="请输入密码"
            />
            <button
                type="button"
                class="toggle-password"
                @click="togglePassword"
            >
              {{ isPasswordVisible ? '👁️' : '🙈' }}
            </button>
          </div>
        </div>

        <div v-if="error" class="error-message">
          {{ error }}
        </div>

        <div v-if="loading" class="loading-spinner">
          <span class="spinner"></span>
        </div>

        <button
            type="submit"
            class="login-btn btn-primary"
            :disabled="loading"
        >
          {{ loading ? '登录中...' : '登录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuth } from '../composables/useAuth';
import type { LoginCredentials } from '../types/auth';

const router = useRouter();
const { login } = useAuth();

const credentials = ref<LoginCredentials>({
  username: '',
  password: '',
});

const error = ref('');
const loading = ref(false);
const isPasswordVisible = ref(false);

const togglePassword = () => {
  isPasswordVisible.value = !isPasswordVisible.value;
};

const handleLogin = async (): Promise<void> => {
  error.value = '';
  loading.value = true;

  try {
    const success = await login(credentials.value);
    if (success) {
      await router.push('/dashboard');
    } else {
      error.value = '用户名或密码错误，请确认输入是否正确。';
    }
  } catch (err: any) {
    error.value = '登录失败，请检查网络或联系管理员。';
  } finally {
    loading.value = false;
  }
};
</script>

<style scoped>
/* ========== 背景粒子动画 ========== */
.login-container {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: var(--bg-primary, #0a0a0a);
  overflow: hidden;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

/* 背景粒子层 */
.login-container::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: radial-gradient(circle at center, rgba(0, 255, 255, 0.03) 0%, rgba(0, 0, 0, 0.4) 70%);
  pointer-events: none;
  z-index: 0;
  animation: glowWave 15s ease-in-out infinite alternate;
}

/* 粒子点（浮空光点） */
.login-container::after {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 0;
  background: radial-gradient(circle at center, transparent 50%, rgba(0, 200, 255, 0.1) 100%);
  backdrop-filter: blur(20px);
  opacity: 0.4;
  animation: floatParticles 20s linear infinite;
}

/* 粒子点 - 用多个伪元素模拟 */
.login-container::after {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 0;
  background-image:
      radial-gradient(circle at 20% 20%, rgba(50, 200, 255, 0.1) 1px, transparent 1px),
      radial-gradient(circle at 80% 40%, rgba(100, 255, 200, 0.1) 1px, transparent 1px),
      radial-gradient(circle at 30% 80%, rgba(255, 150, 200, 0.1) 1px, transparent 1px),
      radial-gradient(circle at 70% 10%, rgba(255, 200, 100, 0.1) 1px, transparent 1px),
      radial-gradient(circle at 50% 50%, rgba(150, 100, 255, 0.1) 1px, transparent 1px);
  background-size: 40px 40px, 30px 30px, 45px 45px, 35px 35px, 50px 50px;
  background-position: 0 0, 10px 10px, 20px 30px, 30px 5px, 40px 40px;
  animation: floatParticles 20s linear infinite;
}

/* 动画：波浪光晕 */
@keyframes glowWave {
  from {
    background-position: center;
    transform: scale(1);
  }
  to {
    background-position: 20% 80%, 80% 20%, 40% 40%;
    transform: scale(1.02);
  }
}

/* 动画：粒子浮动（轻微移动 + 渐变） */
@keyframes floatParticles {
  0% {
    background-position: 0 0, 10px 10px, 20px 30px, 30px 5px, 40px 40px;
    opacity: 0.3;
  }
  50% {
    background-position: 40px 10px, 20px 60px, 80px 50px, 10px 80px, 60px 20px;
    opacity: 0.4;
  }
  100% {
    background-position: 100px 0, 70px 40px, 30px 80px, 90px 10px, 20px 90px;
    opacity: 0.3;
  }
}

/* 登录表单（保持在最前层，z-index=1） */
.login-form {
  position: relative;
  z-index: 10;
  backdrop-filter: blur(20px);
  border: 2px solid var(--border-primary, #333);
  border-radius: 16px;
  padding: 40px;
  width: 100%;
  max-width: 450px;
  box-shadow:
      0 10px 30px rgba(0, 0, 0, 0.4),
      inset 0 1px 0 rgba(255, 255, 255, 0.05);
  animation: slideInFromRight 0.8s cubic-bezier(0.25, 0.8, 0.25, 1);
}

@keyframes slideInFromRight {
  from {
    opacity: 0;
    transform: translateX(30px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* 头部样式 */
.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-header h1 {
  font-size: 1.8rem;
  font-weight: 600;
  margin: 0 0 8px;
  color: #000000;
}

.login-header p {
  font-size: 0.95rem;
  color: #aaa;
  margin: 0;
}

/* 表单组 */
.form-group {
  margin-bottom: 24px;
}

.form-label {
  display: block;
  margin-bottom: 8px;
  font-size: 0.95rem;
  color: #0a0a0a;
  font-weight: 500;
}

.form-input {
  width: 100%;
  padding: 14px 16px;
  font-size: 1rem;
  border: 1px solid #444;
  background: rgba(255, 255, 255, 0.08);
  color: #0a0a0a;
  border-radius: 10px;
  transition: border-color 0.2s ease;
}

.form-input:focus {
  outline: none;
  border-color: #3498db;
  box-shadow: 0 0 0 2px rgba(52, 152, 219, 0.2);
}

/* 密码容器与眼图标 */
.password-container {
  position: relative;
}

.toggle-password {
  position: absolute;
  right: 12px;
  top: 50%;
  transform: translateY(-50%);
  background: none;
  border: none;
  color: #ccc;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 0;
  transition: color 0.2s;
}

.toggle-password:hover {
  color: #fff;
}

/* 错误提示 */
.error-message {
  margin-top: 12px;
  padding: 12px;
  background: rgba(220, 40, 40, 0.1);
  color: #e53935;
  border: 1px solid #e53935;
  border-radius: 8px;
  font-size: 0.9rem;
  text-align: center;
  font-weight: 500;
}

/* 加载动画 */
.loading-spinner {
  display: flex;
  justify-content: center;
  align-items: center;
  margin: 20px 0;
  font-size: 1rem;
  color: #3498db;
}

.spinner {
  border: 4px solid #f3f3f3;
  border-top: 4px solid #3498db;
  border-radius: 50%;
  width: 24px;
  height: 24px;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

/* 按钮样式 */
.login-btn {
  width: 100%;
  padding: 14px;
  font-size: 1rem;
  font-weight: 600;
  border: none;
  background: #3498db;
  color: white;
  border-radius: 10px;
  cursor: pointer;
  transition: background 0.2s ease, transform 0.1s ease;
  box-shadow: 0 4px 12px rgba(52, 152, 219, 0.3);
}

.login-btn:hover:not(:disabled) {
  background: #2980b9;
  transform: translateY(-1px);
}

.login-btn:active {
  transform: translateY(0);
}

.login-btn:disabled {
  background: #7f8c8d;
  cursor: not-allowed;
  opacity: 0.6;
}

/* 响应式适配 */
@media (max-width: 480px) {
  .login-form {
    padding: 32px 24px;
    max-width: 95%;
  }

  .login-header h1 {
    font-size: 1.5rem;
  }

  .login-header p {
    font-size: 0.9rem;
  }

  .form-input, .toggle-password {
    font-size: 0.95rem;
  }

  .form-label {
    font-size: 0.9rem;
  }
}
</style>
