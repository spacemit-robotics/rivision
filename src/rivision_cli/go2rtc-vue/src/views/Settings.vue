<template>
  <AppLayout>
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">系统设置</h1>
        <p class="page-description">配置推理网关连接参数</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="handleSaveSettings" :loading="saving">
          <el-icon><Check /></el-icon>
          保存设置
        </el-button>
      </div>
    </div>

    <div class="settings-container">
      <!-- Gateway连接设置 -->
      <div class="settings-card">
        <div class="card-header">
          <el-icon><Connection /></el-icon>
          <span>Gateway连接设置</span>
        </div>
        
        <el-form :model="settings" label-width="120px" class="setting-form">
          <el-form-item label="Gateway地址">
            <el-input v-model="settings.url" placeholder="http://localhost:8081" />
            <div class="form-help">推理网关服务的完整URL地址</div>
          </el-form-item>
          
          <el-form-item label="连接超时">
            <el-input-number v-model="settings.timeout" :min="5000" :max="60000" :step="1000" />
            <div class="form-help">连接超时时间（毫秒）</div>
          </el-form-item>
          
          <el-form-item label="重试次数">
            <el-input-number v-model="settings.retries" :min="0" :max="10" />
            <div class="form-help">连接失败时的重试次数</div>
          </el-form-item>
          
          <el-form-item label="自动重连">
            <el-switch v-model="settings.autoReconnect" />
            <div class="form-help">连接断开时自动尝试重新连接</div>
          </el-form-item>
        </el-form>
      </div>
    </div>
  </AppLayout>
</template>


<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Check, Connection } from '@element-plus/icons-vue'
import AppLayout from '@/layouts/AppLayout.vue'

const saving = ref(false)

const settings = reactive({
  url: 'http://localhost:8081',
  timeout: 10000,
  retries: 3,
  autoReconnect: true
})

const handleSaveSettings = async () => {
  saving.value = true
  try {
    localStorage.setItem('gateway-settings', JSON.stringify(settings))
    await new Promise(resolve => setTimeout(resolve, 500))
    ElMessage.success('设置保存成功')
  } catch (error) {
    ElMessage.error('设置保存失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  try {
    const saved = localStorage.getItem('gateway-settings')
    if (saved) {
      Object.assign(settings, JSON.parse(saved))
    }
  } catch (error) {
    console.error('加载设置失败:', error)
  }
})
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 8px;
}

.page-description {
  color: #909399;
  font-size: 14px;
  margin: 0;
}

.settings-container {
  max-width: 800px;
}

.settings-card {
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
  border: 1px solid #e4e7ed;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px 24px;
  border-bottom: 1px solid #e4e7ed;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.card-header .el-icon {
  color: #409eff;
  font-size: 20px;
}

.setting-form {
  padding: 24px;
}

.form-help {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.setting-form :deep(.el-form-item) {
  margin-bottom: 24px;
}

.setting-form :deep(.el-input-number) {
  width: 200px;
}
</style>
