// Copyright 2026 SpacemiT (Hangzhou) Technology Co. Ltd.
//
// SPDX-License-Identifier: Apache-2.0

<template>
  <Transition name="drawer">
    <div class="gateway-drawer" v-if="visible">
      <div class="panel-header">
        <div class="panel-title">
          <el-icon><Connection /></el-icon>
          <span>网关状态</span>
        </div>
        <button class="close-btn" @click="$emit('close')">
          <el-icon><Close /></el-icon>
        </button>
      </div>

      <!-- 连接状态 -->
      <div class="connection-status" :class="{ connected: isGatewayOnline }">
        <span class="status-dot"></span>
        <span>{{ isGatewayOnline ? '已连接' : '未连接' }}</span>
      </div>

      <!-- 节点变化通知栏（建议4） -->
      <Transition name="notification">
        <div class="node-change-notification" v-if="gatewayStore.nodeChangeNotification">
          <span>&#9889; {{ gatewayStore.nodeChangeNotification }}</span>
        </div>
      </Transition>

      <!-- 可滚动内容区 -->
      <div class="drawer-content">
        <!-- 统计卡片 -->
        <div class="stats-row">
        <div class="stat-item">
          <div class="stat-icon primary">
            <el-icon><Monitor /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ gatewayStore.nodeStatusSummary.healthy }}</div>
            <div class="stat-label">健康节点</div>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon info">
            <el-icon><DataLine /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ gatewayStore.currentTasks.length }}</div>
            <div class="stat-label">运行任务</div>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon success">
            <el-icon><SuccessFilled /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ formatPercentage(gatewayStore.systemStats.successRate) }}</div>
            <div class="stat-label">成功率</div>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon warning">
            <el-icon><Timer /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-value">{{ formatResponseTime(gatewayStore.systemStats.averageResponseTime) }}</div>
            <div class="stat-label">响应时间</div>
          </div>
        </div>
        </div>

        <!-- 节点列表 -->
        <div class="section">
        <div class="section-header">
          <span class="section-title">
            <el-icon><Monitor /></el-icon>
            节点列表
          </span>
          <el-button size="small" type="primary" link @click="refreshNodes">
            <el-icon><RefreshRight /></el-icon>
            刷新
          </el-button>
        </div>
        <div class="section-content">
          <el-table :data="nodeList" size="small" max-height="240" v-loading="nodesLoading">
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="host" label="地址" min-width="130">
              <template #default="{ row }">
                <span class="address-cell">{{ row.host }}:{{ row.port }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="healthy" label="状态" width="60">
              <template #default="{ row }">
                <el-tag :type="row.healthy ? 'success' : 'danger'" size="small">
                  {{ row.healthy ? '在线' : '离线' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="CPU" width="55">
              <template #default="{ row }">
                <span class="resource-value" :class="{ 'high-usage': (row.cpu_percent || 0) > 80 }">
                  {{ Math.round(row.cpu_percent || 0) }}%
                </span>
              </template>
            </el-table-column>
            <el-table-column label="内存" width="55">
              <template #default="{ row }">
                <span class="resource-value" :class="{ 'high-usage': (row.memory_percent || 0) > 80 }">
                  {{ Math.round(row.memory_percent || 0) }}%
                </span>
              </template>
            </el-table-column>
            <el-table-column label="心跳" width="60">
              <template #default="{ row }">
                <span class="heartbeat-cell">{{ formatHeartbeat(row.last_heartbeat) }}</span>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="nodeList.length === 0 && !nodesLoading" description="暂无节点" :image-size="60" />
          </div>
        </div>

        <!-- 运行中任务 -->
        <div class="section">
        <div class="section-header">
          <span class="section-title">
            <el-icon><DataLine /></el-icon>
            运行中任务
          </span>
          <el-button size="small" type="primary" link @click="refreshTasks">
            <el-icon><RefreshRight /></el-icon>
            刷新
          </el-button>
        </div>
        <div class="section-content">
          <el-table :data="runningTasks" size="small" max-height="150" v-loading="tasksLoading">
            <el-table-column prop="id" label="任务ID" width="100">
              <template #default="{ row }">
                <el-tooltip :content="row.task_id || row.id || ''" placement="top">
                  <span class="task-id-cell">{{ (row.task_id || row.id || '').slice(0, 8) }}...</span>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="类型" width="80">
              <template #default="{ row }">
                {{ row.type || row.task_type || 'VLM' }}
              </template>
            </el-table-column>
            <el-table-column label="进度" min-width="120">
              <template #default="{ row }">
                <span v-if="!row.progress || row.progress <= 0" class="analyzing-badge">分析中...</span>
                <el-progress v-else :percentage="row.progress" :stroke-width="6" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="60">
              <template #default="{ row }">
                <el-button size="small" type="danger" link @click="cancelTask(row)">
                  取消
                </el-button>
              </template>
            </el-table-column>
          </el-table>
          <el-empty v-if="runningTasks.length === 0 && !tasksLoading" description="暂无 VLM 分析任务" :image-size="60">
            <template #description>
              <span style="font-size: 12px; color: #909399;">暂无 VLM 分析任务<br/>YOLO 检测在视频流开启时自动运行</span>
            </template>
          </el-empty>
          </div>
        </div>

        <!-- ★ YOLO 实时检测状态（前端 DirectDetector 真实数据） -->
        <div class="section" v-if="gatewayStore.yoloPipelineStats">
          <div class="section-header">
            <span class="section-title">
              <el-icon><DataLine /></el-icon>
              YOLO 实时检测
            </span>
          </div>
          <div class="yolo-stats-grid">
            <div class="yolo-stat">
              <span class="yolo-stat-value">{{ gatewayStore.yoloPipelineStats.healthy_nodes }}</span>
              <span class="yolo-stat-label">健康节点</span>
            </div>
            <div class="yolo-stat">
              <span class="yolo-stat-value">{{ yoloDetectorStats.avgLatencyMs }}ms</span>
              <span class="yolo-stat-label">平均延迟</span>
            </div>
            <div class="yolo-stat">
              <span class="yolo-stat-value">{{ yoloDetectorStats.targetFps.toFixed(1) }}</span>
              <span class="yolo-stat-label">帧率(fps)</span>
            </div>
            <div class="yolo-stat">
              <span class="yolo-stat-value">{{ formatFrameCount(yoloDetectorStats.totalFrames) }}</span>
              <span class="yolo-stat-label">总帧数</span>
            </div>
            <div class="yolo-stat">
              <span class="yolo-stat-value">{{ yoloDetectorStats.activeCameras }}</span>
              <span class="yolo-stat-label">摄像头</span>
            </div>
            <div class="yolo-stat">
              <span class="yolo-stat-value" :class="{ 'error-value': yoloDetectorStats.totalErrors > 0 }">{{ yoloDetectorStats.totalErrors }}</span>
              <span class="yolo-stat-label">错误</span>
            </div>
          </div>
        </div>

        <!-- 分布式网关设置 -->
        <div class="section" style="margin-top: 24px;">
        <div class="section-header" @click="settingsExpanded = !settingsExpanded">
          <span class="section-title">
            <el-icon><Setting /></el-icon>
            分布式网关设置
          </span>
          <el-icon class="expand-icon" :class="{ expanded: settingsExpanded }">
            <ArrowDown />
          </el-icon>
        </div>
        <div class="section-content settings-form" v-show="settingsExpanded">
          <el-form :model="settings" label-position="top" size="small">
            <el-form-item label="地址">
              <el-input v-model="settings.url" placeholder="http://localhost:8081" />
            </el-form-item>
            <el-form-item label="超时(ms)">
              <el-input-number v-model="settings.timeout" :min="1000" :max="60000" :step="1000" style="width: 100%" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" size="small" @click="saveSettings" :loading="saving">
                保存
              </el-button>
              <el-button size="small" @click="testConnection" :loading="testing">
                测试
              </el-button>
            </el-form-item>
            </el-form>
          </div>
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import {
  Connection,
  Close,
  Monitor,
  DataLine,
  SuccessFilled,
  Timer,
  RefreshRight,
  Setting,
  ArrowDown
} from '@element-plus/icons-vue'
import { useGatewayStore } from '@/stores/gateway'
import { yoloDirectDetector } from '@/services/yoloDirectDetector'

const props = defineProps<{
  visible: boolean
}>()

// ★ 前端 DirectDetector 实时统计（替代已禁用的 Go 后端 pipeline 统计）
const yoloDetectorStats = ref({
  totalFrames: 0,
  totalErrors: 0,
  totalSkipped: 0,
  avgLatencyMs: 0,
  activeCameras: 0,
  targetFps: 5,
  perCameraFps: {} as Record<string, number>,
})
let _detectorStatsTimer: number | null = null

function refreshDetectorStats() {
  if (yoloDirectDetector.enabled) {
    yoloDetectorStats.value = yoloDirectDetector.getStats()
  }
}

const emit = defineEmits<{
  (e: 'close'): void
}>()

const gatewayStore = useGatewayStore()

const nodesLoading = ref(false)
const tasksLoading = ref(false)
const isGatewayOnline = ref(false)

// 通过 /api/status 检查网关状态（与 AnalysisStatus 保持一致）
async function checkGatewayStatus() {
  try {
    const resp = await fetch('/api/status')
    if (resp.ok) {
      const data = await resp.json()
      isGatewayOnline.value = data.gateway?.status === 'healthy'
    } else {
      isGatewayOnline.value = false
    }
  } catch (e) {
    isGatewayOnline.value = false
  }
}
const saving = ref(false)
const testing = ref(false)
const settingsExpanded = ref(false)

const settings = reactive({
  url: 'http://localhost:8081',
  timeout: 10000
})

const nodeList = computed(() => {
  return [...gatewayStore.nodes]
})

const runningTasks = computed(() => {
  return gatewayStore.currentTasks.filter(t => t.status === 'running')
})

const formatPercentage = (value: number | undefined) => {
  if (value === undefined || value === null) return '0%'
  return `${Math.round(value * 100)}%`
}

const formatResponseTime = (value: number | undefined) => {
  if (value === undefined || value === null) return '0ms'
  return `${Math.round(value)}ms`
}

// ★ 格式化心跳时间（如 "5s前"、"2m前"）
const formatHeartbeat = (isoStr: string | undefined) => {
  if (!isoStr) return '无'
  try {
    const elapsed = (Date.now() - new Date(isoStr).getTime()) / 1000
    if (elapsed < 0 || elapsed > 86400) return '无'
    if (elapsed < 60) return `${Math.round(elapsed)}s`
    if (elapsed < 3600) return `${Math.round(elapsed / 60)}m`
    return `${Math.round(elapsed / 3600)}h`
  } catch {
    return '无'
  }
}

// ★ 格式化帧数（如 1234 → "1.2K"）
const formatFrameCount = (count: number) => {
  if (count < 1000) return String(count)
  if (count < 1000000) return `${(count / 1000).toFixed(1)}K`
  return `${(count / 1000000).toFixed(1)}M`
}

// ★ 手动刷新（显示 loading）vs 后台静默刷新（不显示 loading）
const refreshNodes = async () => {
  nodesLoading.value = true
  try {
    await gatewayStore.fetchNodeStatus()
  } finally {
    nodesLoading.value = false
  }
}

const refreshTasks = async () => {
  tasksLoading.value = true
  try {
    await gatewayStore.fetchCurrentTasks()
  } finally {
    tasksLoading.value = false
  }
}

// ★ 后台静默刷新（不触发 loading 遮罩，避免界面闪烁）
const silentRefreshNodes = async () => {
  try {
    await gatewayStore.fetchNodeStatus()
  } catch (e) {
    // 静默失败
  }
}

const silentRefreshTasks = async () => {
  try {
    await gatewayStore.fetchCurrentTasks()
  } catch (e) {
    // 静默失败
  }
}

const cancelTask = async (task: any) => {
  const taskId = task.task_id || task.id
  if (!taskId) return
  
  try {
    await gatewayStore.forceCancelTask(taskId, '用户取消')
    ElMessage.success('任务已取消')
    await refreshTasks()
  } catch (error) {
    ElMessage.error('取消任务失败')
  }
}

const saveSettings = async () => {
  saving.value = true
  try {
    localStorage.setItem('gateway-settings', JSON.stringify(settings))
    // 重新初始化连接
    await gatewayStore.initialize(settings.url)
    ElMessage.success('设置已保存')
  } catch (error) {
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

const testConnection = async () => {
  testing.value = true
  try {
    await gatewayStore.initialize(settings.url)
    ElMessage.success('连接成功')
  } catch (error) {
    ElMessage.error('连接失败')
  } finally {
    testing.value = false
  }
}

const loadSettings = () => {
  try {
    const saved = localStorage.getItem('gateway-settings')
    if (saved) {
      const parsed = JSON.parse(saved)
      settings.url = parsed.url || settings.url
      settings.timeout = parsed.timeout || settings.timeout
    }
  } catch (e) {
    console.error('加载设置失败', e)
  }
}

watch(() => props.visible, (val) => {
  if (val) {
    checkGatewayStatus()
    refreshNodes()
    refreshTasks()
    gatewayStore.fetchYoloPipelineStats()
  }
})

onMounted(() => {
  loadSettings()
  checkGatewayStatus()
  refreshNodes()    // 首次加载显示 loading
  refreshTasks()    // 首次加载显示 loading
  gatewayStore.fetchYoloPipelineStats()
  // ★ 后台定时刷新使用静默模式（不显示 loading 遮罩，避免界面闪烁）
  // ★ 降低轮询频率: 避免与 YOLO 请求竞争浏览器连接 (Chrome 限制 6 并发/域)
  setInterval(checkGatewayStatus, 10000)   // 5s → 10s
  setInterval(silentRefreshNodes, 15000)   // 10s → 15s
  setInterval(silentRefreshTasks, 10000)   // 3s → 10s (VLM任务不需要秒级更新)
  setInterval(() => gatewayStore.fetchYoloPipelineStats(), 15000)  // 5s → 15s
  // ★ 刷新前端 DirectDetector 统计
  refreshDetectorStats()
  _detectorStatsTimer = window.setInterval(refreshDetectorStats, 10000)  // 5s → 10s
})
</script>

<style scoped>
/* 侧边抽屉样式 */
.gateway-drawer {
  position: fixed;
  top: 56px;
  right: 0;
  bottom: 0;
  width: 420px;
  background: #fff;
  box-shadow: -4px 0 16px rgba(0, 0, 0, 0.1);
  z-index: 1000;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* 抽屉动画 */
.drawer-enter-active,
.drawer-leave-active {
  transition: transform 0.3s ease;
}

.drawer-enter-from,
.drawer-leave-to {
  transform: translateX(100%);
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid #ebeef5;
  background: #fff;
  flex-shrink: 0;
}

.panel-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.panel-title .el-icon {
  color: #409eff;
  font-size: 18px;
}

.close-btn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #909399;
  transition: all 0.2s;
}

.close-btn:hover {
  background: #f5f7fa;
  color: #303133;
}

.connection-status {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 16px;
  background: #fef0f0;
  color: #f56c6c;
  font-size: 13px;
  flex-shrink: 0;
}

.connection-status.connected {
  background: #f0f9eb;
  color: #67c23a;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
}

/* 统计卡片 - 2x2 网格 */
.stats-row {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid #ebeef5;
  flex-shrink: 0;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px;
  background: #f5f7fa;
  border-radius: 6px;
}

.stat-icon {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 16px;
  flex-shrink: 0;
}

.stat-icon.primary { background: #409eff; }
.stat-icon.info { background: #e6a23c; }
.stat-icon.success { background: #67c23a; }
.stat-icon.warning { background: #f56c6c; }

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 18px;
  font-weight: 700;
  color: #303133;
  line-height: 1.2;
}

.stat-label {
  font-size: 11px;
  color: #909399;
  margin-top: 2px;
}

/* 可滚动内容区 */
.drawer-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
}

.section {
  padding: 12px 16px;
  border-bottom: 1px solid #ebeef5;
}

.section:last-child {
  border-bottom: none;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  cursor: pointer;
  user-select: none;
}

.section-header:hover {
  color: #409eff;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.section-title .el-icon {
  color: #409eff;
  font-size: 14px;
}

.expand-icon {
  color: #909399;
  font-size: 12px;
  transition: transform 0.2s;
}

.expand-icon.expanded {
  transform: rotate(180deg);
}

.section-content {
  background: #fafafa;
  border-radius: 6px;
  overflow: hidden;
  margin-top: 8px;
}

/* 表格样式优化 */
.section-content :deep(.el-table) {
  font-size: 12px;
}

.section-content :deep(.el-table th) {
  padding: 6px 0;
  font-size: 12px;
}

.section-content :deep(.el-table td) {
  padding: 6px 0;
}

.section-content :deep(.el-empty__description) {
  font-size: 12px;
}

.address-cell {
  white-space: nowrap;
  font-family: monospace;
  font-size: 11px;
}

.settings-form {
  padding: 12px;
}

.settings-form :deep(.el-form-item) {
  margin-bottom: 10px;
}

.settings-form :deep(.el-form-item:last-child) {
  margin-bottom: 0;
}

.settings-form :deep(.el-form-item__label) {
  font-size: 12px;
  padding-bottom: 4px;
}

/* ★ 节点变化通知条（建议4） */
.node-change-notification {
  padding: 6px 16px;
  background: linear-gradient(90deg, #e6f7ff, #f0f9eb);
  color: #1890ff;
  font-size: 13px;
  font-weight: 500;
  text-align: center;
  flex-shrink: 0;
  border-bottom: 1px solid #91d5ff;
}

.notification-enter-active,
.notification-leave-active {
  transition: all 0.3s ease;
}

.notification-enter-from,
.notification-leave-to {
  opacity: 0;
  max-height: 0;
  padding: 0 16px;
}

/* ★ 节点资源值（建议1） */
.resource-value {
  font-size: 11px;
  font-family: monospace;
  color: #606266;
}

.resource-value.high-usage {
  color: #f56c6c;
  font-weight: 600;
}

.heartbeat-cell {
  font-size: 11px;
  color: #909399;
  font-family: monospace;
}

/* ★ YOLO 实时检测统计网格（建议2） */
.yolo-stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-top: 8px;
}

.yolo-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 4px;
  background: #f5f7fa;
  border-radius: 6px;
}

.yolo-stat-value {
  font-size: 16px;
  font-weight: 700;
  color: #303133;
  font-family: monospace;
}

.yolo-stat-value.error-value {
  color: #f56c6c;
}

.yolo-stat-label {
  font-size: 10px;
  color: #909399;
  margin-top: 2px;
}

/* 任务相关 */
.task-id-cell {
  font-family: monospace;
  font-size: 11px;
  color: #606266;
}

.analyzing-badge {
  display: inline-block;
  padding: 2px 8px;
  background: #e6f7ff;
  color: #1890ff;
  border-radius: 4px;
  font-size: 11px;
}

@media (max-width: 768px) {
  .gateway-drawer {
    width: 100%;
    top: 0;
  }
}
</style>
