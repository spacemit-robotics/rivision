<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import axios from 'axios'
import CameraManagementModal from '@/components/CameraManagementModal.vue'

interface Camera {
  id: string
  name: string
  url: string
  protocol: string
  node_id: string
  status: string
  enabled: boolean
  config?: Record<string, any>
}

interface Node {
  id: string
  name: string
  host: string
  status: string
  healthy: boolean
}

const cameras = ref<Camera[]>([])
const nodes = ref<Node[]>([])
const loading = ref(false)
const selectedIds = ref<Set<string>>(new Set())
const selectAll = ref(false)

// 弹窗状态
const showCameraModal = ref(false)
const showDeleteConfirm = ref(false)
const editingCamera = ref<any>(null)
const deletingCamera = ref<Camera | null>(null)

// 测试连接
const testing = ref<string | null>(null)
const testResults = ref<Record<string, { success: boolean; message: string }>>({})

// 缩略图
const thumbnails = ref<Record<string, string>>({})

// 自动刷新
let refreshTimer: number | null = null

async function fetchCameras() {
  loading.value = true
  try {
    const [camRes, nodeRes] = await Promise.all([
      axios.get('/api/v1/cameras'),
      axios.get('/api/v1/nodes'),
    ])
    cameras.value = camRes.data.cameras || []
    nodes.value = (nodeRes.data.nodes || []).filter((n: Node) => n.healthy)
    // 获取缩略图
    cameras.value.forEach(cam => {
      if (cam.status === 'online' && cam.node_id) {
        fetchThumbnail(cam)
      }
    })
  } catch (e) {
    console.error('获取数据失败', e)
  } finally {
    loading.value = false
  }
}

async function fetchThumbnail(cam: Camera) {
  // 通过 Hub 代理获取缩略图，避免 CORS 问题
  // Hub 需要实现 /api/v1/cameras/:id/thumbnail 接口
  // 暂时跳过，因为 go2rtc frame.jpeg 可能不稳定
  // TODO: 实现 Hub 代理或使用 stream.mp4 截图
}

// P0: 添加/编辑摄像头 - 使用统一的 CameraManagementModal
function openAddModal() {
  editingCamera.value = null
  showCameraModal.value = true
}

function openEditModal(cam: any) {
  // 从 config 字段读取 AI 配置和 OWL 流模式
  const config = cam.config || {}
  editingCamera.value = {
    id: cam.id,
    name: cam.name,
    url: cam.url,
    type: cam.protocol || 'rtsp',
    nodeId: cam.node_id || '',
    group: cam.group_id || '',
    streamMode: config.stream_mode || '',
    owlChannelId: config.owl_channel_id || '',
    yoloEnabled: config.yolo_enabled ?? true,
    yoloModel: config.yolo_model || 'person',
    yoloConfidence: config.yolo_confidence ?? 0.5,
    vlmEnabled: config.vlm_enabled ?? false,
    vlmInterval: config.vlm_interval ?? 5,
  }
  showCameraModal.value = true
}

async function handleSaveCamera(camera: any) {
  try {
    // 构建 config 对象存储 AI 配置
    const config = {
      yolo_enabled: camera.yoloEnabled ?? true,
      yolo_model: camera.yoloModel || 'person',
      yolo_confidence: camera.yoloConfidence ?? 0.5,
      vlm_enabled: camera.vlmEnabled ?? false,
      vlm_interval: camera.vlmInterval ?? 5,
    }
    
    const payload = {
      name: camera.name,
      url: camera.url,
      protocol: camera.type,
      node_id: camera.nodeId || undefined,
      group_id: camera.group || undefined,
      config: config,
    }
    
    if (camera.id) {
      await axios.put(`/api/v1/cameras/${camera.id}`, payload)
    } else {
      await axios.post('/api/v1/cameras', payload)
    }
    showCameraModal.value = false
    editingCamera.value = null
    await fetchCameras()
  } catch (e: any) {
    alert('保存失败: ' + (e.response?.data?.error || e.message))
  }
}

// P0: 删除摄像头
function openDeleteConfirm(cam: Camera) {
  deletingCamera.value = cam
  showDeleteConfirm.value = true
}

async function handleDelete() {
  if (!deletingCamera.value) return
  const camId = deletingCamera.value.id
  try {
    await axios.delete(`/api/v1/cameras/${camId}`)
    showDeleteConfirm.value = false
    deletingCamera.value = null
    selectedIds.value.delete(camId)
    await fetchCameras()
  } catch (e: any) {
    alert('删除失败: ' + (e.response?.data?.error || e.message))
  }
}

// P1: 批量选择
function toggleSelect(id: string) {
  if (selectedIds.value.has(id)) {
    selectedIds.value.delete(id)
  } else {
    selectedIds.value.add(id)
  }
  selectAll.value = selectedIds.value.size === cameras.value.length
}

function toggleSelectAll() {
  if (selectAll.value) {
    selectedIds.value.clear()
  } else {
    cameras.value.forEach(c => selectedIds.value.add(c.id))
  }
  selectAll.value = !selectAll.value
}

// P1: 批量删除
async function handleBatchDelete() {
  if (selectedIds.value.size === 0) return
  if (!confirm(`确定删除选中的 ${selectedIds.value.size} 个摄像头？`)) return
  try {
    await Promise.all(
      Array.from(selectedIds.value).map(id => axios.delete(`/api/v1/cameras/${id}`))
    )
    selectedIds.value.clear()
    selectAll.value = false
    await fetchCameras()
  } catch (e: any) {
    alert('批量删除失败')
  }
}

// P1: 批量分配节点
async function handleBatchAssign(nodeId: string) {
  if (selectedIds.value.size === 0 || !nodeId) return
  try {
    await Promise.all(
      Array.from(selectedIds.value).map(id =>
        axios.post(`/api/v1/cameras/${id}/assign`, { node_id: nodeId })
      )
    )
    selectedIds.value.clear()
    selectAll.value = false
    await fetchCameras()
  } catch (e: any) {
    alert('批量分配失败')
  }
}

// P2: 测试连接 - 通过 Hub API 检查状态
async function testConnection(cam: Camera) {
  testing.value = cam.id
  testResults.value[cam.id] = { success: false, message: '测试中...' }
  try {
    if (!cam.node_id) {
      testResults.value[cam.id] = { success: false, message: '未分配节点' }
      return
    }
    // 通过 Hub API 获取摄像头状态（避免 CORS）
    const res = await axios.get(`/api/v1/cameras/${cam.id}`, { timeout: 5000 })
    const status = res.data?.status
    if (status === 'online') {
      testResults.value[cam.id] = { success: true, message: '连接正常' }
    } else if (status === 'starting') {
      testResults.value[cam.id] = { success: false, message: '启动中...' }
    } else {
      testResults.value[cam.id] = { success: false, message: '离线' }
    }
  } catch (e: any) {
    testResults.value[cam.id] = { success: false, message: '检测失败' }
  } finally {
    testing.value = null
    // 5秒后清除结果
    setTimeout(() => { delete testResults.value[cam.id] }, 5000)
  }
}

// stream_mode 短标签
function streamModeShort(mode: string): string {
  const labels: Record<string, string> = {
    hub_sip_remote: 'SIP',
    direct: '直连',
    worker_owl: 'OWL',
  }
  return labels[mode] || mode
}

// 获取节点名称
function getNodeName(nodeId: string): string {
  const node = nodes.value.find(n => n.id === nodeId)
  return node?.name || nodeId || '未分配'
}

const hasSelection = computed(() => selectedIds.value.size > 0)

onMounted(() => {
  fetchCameras()
  // 每30秒自动刷新
  refreshTimer = window.setInterval(fetchCameras, 30000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <div class="cameras-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2>摄像头管理</h2>
      <div class="header-actions">
        <button class="btn btn-outline" @click="fetchCameras" :disabled="loading">
          🔄 刷新
        </button>
        <button class="btn btn-primary" @click="openAddModal">
          ➕ 添加摄像头
        </button>
      </div>
    </div>

    <!-- 批量操作栏 -->
    <div v-if="hasSelection" class="batch-toolbar">
      <span class="selection-info">已选择 {{ selectedIds.size }} 项</span>
      <select class="batch-select" @change="(e: any) => { handleBatchAssign(e.target.value); e.target.value = '' }">
        <option value="">分配到节点...</option>
        <option v-for="node in nodes" :key="node.id" :value="node.id">
          {{ node.name || node.id }}
        </option>
      </select>
      <button class="btn btn-danger btn-sm" @click="handleBatchDelete">
        🗑️ 批量删除
      </button>
      <button class="btn btn-outline btn-sm" @click="selectedIds.clear(); selectAll = false">
        取消选择
      </button>
    </div>

    <!-- 摄像头列表 -->
    <div class="card">
      <div v-if="loading && cameras.length === 0" class="loading">
        <div class="spinner"></div>
      </div>
      <div v-else-if="cameras.length === 0" class="empty-state">
        <div class="empty-icon">📹</div>
        <div class="empty-text">暂无摄像头</div>
        <button class="btn btn-primary" @click="openAddModal">添加摄像头</button>
      </div>
      <div v-else>
        <!-- 全选 -->
        <div class="select-all-row">
          <label class="checkbox-label">
            <input type="checkbox" :checked="selectAll" @change="toggleSelectAll" />
            <span>全选</span>
          </label>
          <span class="camera-count">共 {{ cameras.length }} 个摄像头</span>
        </div>

        <!-- 摄像头网格 -->
        <div class="cameras-grid">
          <div 
            v-for="cam in cameras" 
            :key="cam.id" 
            class="camera-card"
            :class="{ selected: selectedIds.has(cam.id) }"
          >
            <!-- 选择框 -->
            <div class="card-checkbox">
              <input 
                type="checkbox" 
                :checked="selectedIds.has(cam.id)"
                @change="toggleSelect(cam.id)"
              />
            </div>

            <!-- 缩略图/预览 -->
            <div class="camera-preview">
              <img 
                v-if="thumbnails[cam.id]" 
                :src="thumbnails[cam.id]" 
                alt="预览"
                @error="delete thumbnails[cam.id]"
              />
              <div v-else class="preview-placeholder">
                <span class="preview-icon">📹</span>
              </div>
              <span :class="['status-dot', cam.status]"></span>
            </div>

            <!-- 信息 -->
            <div class="camera-info">
              <div class="camera-name">{{ cam.name || cam.id }}</div>
              <div class="camera-url" :title="cam.url || '(OWL 管理)'">
                {{ cam.url || '(由 OWL 自动管理)' }}
              </div>
              <div class="camera-meta">
                <span class="meta-item">
                  <span class="meta-icon">🖥️</span>
                  {{ getNodeName(cam.node_id) }}
                </span>
                <span v-if="cam.config?.stream_mode" :class="['stream-mode-tag', cam.config.stream_mode]">
                  {{ streamModeShort(cam.config.stream_mode) }}
                </span>
                <span v-if="cam.protocol && cam.protocol !== 'rtsp'" class="protocol-tag">
                  {{ cam.protocol.toUpperCase() }}
                </span>
                <span :class="['status-badge', cam.status]">
                  {{ cam.status === 'online' ? '在线' : cam.status === 'starting' ? '启动中' : '离线' }}
                </span>
              </div>
            </div>

            <!-- 测试结果 -->
            <div v-if="testResults[cam.id]" :class="['test-result', testResults[cam.id].success ? 'success' : 'error']">
              {{ testResults[cam.id].message }}
            </div>

            <!-- 操作按钮 -->
            <div class="camera-actions">
              <button 
                class="action-btn" 
                title="测试连接"
                @click="testConnection(cam)"
                :disabled="testing === cam.id"
              >
                {{ testing === cam.id ? '⏳' : '🔗' }}
              </button>
              <button class="action-btn" title="编辑" @click="openEditModal(cam)">✏️</button>
              <button class="action-btn danger" title="删除" @click="openDeleteConfirm(cam)">🗑️</button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 添加/编辑摄像头弹窗 - 使用统一组件 -->
    <CameraManagementModal
      v-model:visible="showCameraModal"
      :camera="editingCamera"
      :nodes="nodes"
      @save="handleSaveCamera"
      @refresh="fetchCameras"
    />

    <!-- 删除确认弹窗 -->
    <Teleport to="body">
      <div v-if="showDeleteConfirm" class="modal-overlay" @click="showDeleteConfirm = false">
        <div class="modal modal-sm" @click.stop>
          <div class="modal-header">
            <h3>确认删除</h3>
            <button class="close-btn" @click="showDeleteConfirm = false">✕</button>
          </div>
          <div class="modal-body">
            <p>确定要删除摄像头 <strong>{{ deletingCamera?.name }}</strong> 吗？</p>
            <p class="text-muted">此操作不可恢复</p>
          </div>
          <div class="modal-footer">
            <button class="btn btn-outline" @click="showDeleteConfirm = false">取消</button>
            <button class="btn btn-danger" @click="handleDelete">删除</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.cameras-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.page-header h2 {
  font-size: 20px;
  font-weight: 600;
  color: #1a1a2e;
  margin: 0;
}

.header-actions {
  display: flex;
  gap: 12px;
}

/* 批量操作栏 */
.batch-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
}

.selection-info {
  font-weight: 500;
  color: #1d4ed8;
}

.batch-select {
  padding: 6px 12px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: white;
  font-size: 14px;
}

/* 全选行 */
.select-all-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid #e5e7eb;
  margin-bottom: 16px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-size: 14px;
  color: #374151;
}

.checkbox-label input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.camera-count {
  font-size: 14px;
  color: #6b7280;
}

/* 摄像头网格 */
.cameras-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
}

.camera-card {
  position: relative;
  background: #f9fafb;
  border: 2px solid transparent;
  border-radius: 12px;
  padding: 16px;
  transition: all 0.2s ease;
}

.camera-card:hover {
  border-color: #d1d5db;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.camera-card.selected {
  border-color: #3b82f6;
  background: #eff6ff;
}

.card-checkbox {
  position: absolute;
  top: 12px;
  left: 12px;
  z-index: 2;
}

.card-checkbox input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

/* 预览 */
.camera-preview {
  position: relative;
  width: 100%;
  aspect-ratio: 16/9;
  background: #e5e7eb;
  border-radius: 8px;
  overflow: hidden;
  margin-bottom: 12px;
}

.camera-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.preview-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.preview-icon {
  font-size: 48px;
  opacity: 0.3;
}

.status-dot {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid white;
}

.status-dot.online { background: #10b981; }
.status-dot.starting { background: #f59e0b; }
.status-dot.offline { background: #ef4444; }

/* 信息 */
.camera-info {
  margin-bottom: 12px;
}

.camera-name {
  font-weight: 600;
  color: #1a1a2e;
  margin-bottom: 4px;
}

.camera-url {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 8px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.camera-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #6b7280;
}

.meta-icon {
  font-size: 14px;
}

.status-badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: 500;
}

.status-badge.online {
  background: #d1fae5;
  color: #059669;
}

.status-badge.starting {
  background: #fef3c7;
  color: #d97706;
}

.status-badge.offline {
  background: #fee2e2;
  color: #dc2626;
}

/* stream_mode & protocol tags */
.stream-mode-tag {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 11px;
  font-weight: 600;
}

.stream-mode-tag.hub_sip_remote {
  background: #dbeafe;
  color: #1d4ed8;
}

.stream-mode-tag.direct {
  background: #d1fae5;
  color: #059669;
}

.stream-mode-tag.worker_owl {
  background: #fef3c7;
  color: #d97706;
}

.protocol-tag {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 10px;
  font-weight: 600;
  background: #f3f4f6;
  color: #6b7280;
}

/* 测试结果 */
.test-result {
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12px;
  margin-bottom: 8px;
  text-align: center;
}

.test-result.success {
  background: #d1fae5;
  color: #059669;
}

.test-result.error {
  background: #fee2e2;
  color: #dc2626;
}

/* 操作按钮 */
.camera-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.action-btn {
  padding: 6px 10px;
  background: white;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
  font-size: 14px;
}

.action-btn:hover {
  background: #f3f4f6;
}

.action-btn.danger:hover {
  background: #fee2e2;
  border-color: #fecaca;
}

/* 空状态 */
.empty-state {
  text-align: center;
  padding: 60px 20px;
}

.empty-icon {
  font-size: 64px;
  margin-bottom: 16px;
  opacity: 0.3;
}

.empty-text {
  color: #9ca3af;
  margin-bottom: 20px;
}

/* 弹窗 */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  background: white;
  border-radius: 12px;
  width: 90%;
  max-width: 480px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.2);
}

.modal-sm {
  max-width: 360px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e7eb;
}

.modal-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #1a1a2e;
}

.close-btn {
  background: none;
  border: none;
  font-size: 20px;
  color: #9ca3af;
  cursor: pointer;
  padding: 4px;
}

.close-btn:hover {
  color: #374151;
}

.modal-body {
  padding: 20px;
}

.modal-body p {
  margin: 0 0 8px;
  color: #374151;
}

.text-muted {
  color: #9ca3af;
  font-size: 14px;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 20px;
  border-top: 1px solid #e5e7eb;
}

/* 表单 */
.form-group {
  margin-bottom: 16px;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  margin-bottom: 6px;
}

.required {
  color: #ef4444;
}

.form-group input,
.form-group select {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  font-size: 14px;
  transition: border-color 0.2s ease;
}

.form-group input:focus,
.form-group select:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

/* 按钮 */
.btn {
  padding: 10px 20px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-primary {
  background: #3b82f6;
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background: #2563eb;
}

.btn-outline {
  background: white;
  border: 1px solid #d1d5db;
  color: #374151;
}

.btn-outline:hover:not(:disabled) {
  background: #f3f4f6;
}

.btn-danger {
  background: #ef4444;
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background: #dc2626;
}

.btn-sm {
  padding: 6px 12px;
  font-size: 13px;
}

/* 加载状态 */
.loading {
  display: flex;
  justify-content: center;
  padding: 60px;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 3px solid #e5e7eb;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
</style>
