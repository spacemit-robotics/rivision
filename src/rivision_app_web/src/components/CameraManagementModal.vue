<template>
  <Teleport to="body">
    <div v-if="visible" class="modal-overlay" @click="handleClose">
      <div class="modal" :class="{ 'modal-wide': !isEdit }" @click.stop>
        <!-- 头部 -->
        <div class="modal-header">
          <h3>{{ isEdit ? '编辑摄像头' : '添加摄像头' }}</h3>
          <button class="close-btn" @click="handleClose">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <line x1="18" y1="6" x2="6" y2="18"/>
              <line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
        </div>

        <!-- ═══ 新增模式: OWL Discovery Panel ═══ -->
        <div v-if="!isEdit" class="modal-body">
          <OwlDiscoveryPanel
            :nodes="nodes"
            :groups="groups"
            :existingCameraChannelIds="existingChannelIds"
            @imported="onOwlImported"
            @added="onOwlAdded"
          />
        </div>

        <!-- ═══ 编辑模式: 原有表单 ═══ -->
        <div v-else class="modal-body">
          <!-- 流模式信息 -->
          <div v-if="form.streamMode" class="stream-mode-info">
            <span class="mode-label">接入模式:</span>
            <span :class="['mode-badge', form.streamMode]">
              {{ streamModeLabel(form.streamMode) }}
            </span>
            <span v-if="form.owlChannelId" class="mode-channel">
              通道: {{ form.owlChannelId }}
            </span>
          </div>

          <!-- 基本信息 -->
          <div class="form-section">
            <h4 class="section-title">基本信息</h4>

            <div class="form-group">
              <label class="form-label">摄像头名称 <span class="required">*</span></label>
              <input
                v-model="form.name"
                type="text"
                class="form-input"
                placeholder="例如：门口摄像头"
              />
            </div>

            <div v-if="form.streamMode !== 'hub_sip_remote'" class="form-group">
              <label class="form-label">流地址</label>
              <input
                v-model="form.url"
                type="text"
                class="form-input"
                placeholder="rtsp://..."
                :disabled="form.streamMode === 'worker_owl'"
              />
              <p v-if="form.streamMode === 'worker_owl'" class="form-hint">
                worker_owl 模式: Worker 通过本地 OWL 自动解析 RTSP
              </p>
              <p v-else-if="form.streamMode === 'hub_sip_remote'" class="form-hint">
                GB28181 模式: Hub OWL 控制 SIP 信令, RTP 直达 Worker ZLM
              </p>
            </div>

            <div class="form-group">
              <label class="form-label">所属节点</label>
              <select v-model="form.nodeId" class="form-select">
                <option value="">自动分配</option>
                <option v-for="node in nodes" :key="node.id" :value="node.id">
                  {{ node.name || node.id }}
                </option>
              </select>
            </div>

            <div class="form-group">
              <label class="form-label">分组</label>
              <select v-model="form.group" class="form-select">
                <option value="">无分组</option>
                <option v-for="group in groups" :key="group" :value="group">
                  {{ group }}
                </option>
              </select>
            </div>
          </div>

          <!-- AI 分析配置 -->
          <div class="form-section">
            <h4 class="section-title">AI 分析配置</h4>
            
            <div class="toggle-group">
              <label class="toggle-label">
                <input type="checkbox" v-model="form.yoloEnabled" class="toggle-input" />
                <span class="toggle-switch"></span>
                <span class="toggle-text">
                  <strong>YOLO 目标检测</strong>
                  <small>实时检测人、车、物体等目标</small>
                </span>
              </label>
            </div>

            <div v-if="form.yoloEnabled" class="sub-options">
              <div class="form-row">
                <div class="form-group flex-1">
                  <label class="form-label">检测模型</label>
                  <select v-model="form.yoloModel" class="form-select">
                    <option value="person">人员检测</option>
                    <option value="vehicle">车辆检测</option>
                    <option value="object">通用物体</option>
                    <option value="helmet">安全帽检测</option>
                  </select>
                </div>
                <div class="form-group flex-1">
                  <label class="form-label">置信度阈值</label>
                  <div class="slider-group">
                    <input 
                      type="range" 
                      v-model.number="form.yoloConfidence" 
                      min="0.1" 
                      max="0.9" 
                      step="0.1"
                      class="form-slider"
                    />
                    <span class="slider-value">{{ form.yoloConfidence }}</span>
                  </div>
                </div>
              </div>
            </div>

            <div class="toggle-group">
              <label class="toggle-label">
                <input type="checkbox" v-model="form.vlmEnabled" class="toggle-input" />
                <span class="toggle-switch"></span>
                <span class="toggle-text">
                  <strong>VLM 场景理解</strong>
                  <small>使用视觉语言模型分析场景内容</small>
                </span>
              </label>
            </div>

            <div v-if="form.vlmEnabled" class="sub-options">
              <div class="form-group">
                <label class="form-label">分析间隔</label>
                <select v-model="form.vlmInterval" class="form-select">
                  <option :value="1">每秒 (高频)</option>
                  <option :value="5">5 秒</option>
                  <option :value="10">10 秒</option>
                  <option :value="30">30 秒</option>
                  <option :value="60">60 秒 (低频)</option>
                </select>
              </div>
            </div>
          </div>

          <!-- 高级设置 -->
          <details class="advanced-section">
            <summary class="advanced-toggle">高级设置</summary>
            <div class="advanced-content">
              <div class="form-group">
                <label class="form-label">用户名</label>
                <input v-model="form.username" type="text" class="form-input" placeholder="可选" />
              </div>
              <div class="form-group">
                <label class="form-label">密码</label>
                <input v-model="form.password" type="password" class="form-input" placeholder="可选" />
              </div>
              <div class="form-group">
                <label class="form-label">备注</label>
                <textarea v-model="form.notes" class="form-textarea" rows="2" placeholder="可选备注信息"></textarea>
              </div>
            </div>
          </details>
        </div>

        <!-- 底部操作 (编辑模式) -->
        <div v-if="isEdit" class="modal-footer">
          <button class="btn btn-secondary" @click="handleClose">取消</button>
          <button 
            class="btn btn-outline" 
            @click="handleTest"
            :disabled="!form.url || testing"
          >
            {{ testing ? '测试中...' : '测试连接' }}
          </button>
          <button 
            class="btn btn-primary" 
            @click="handleSubmit"
            :disabled="!isFormValid || saving"
          >
            {{ saving ? '保存中...' : '保存修改' }}
          </button>
        </div>

        <!-- 底部操作 (新增模式) -->
        <div v-else class="modal-footer">
          <button class="btn btn-secondary" @click="handleClose">关闭</button>
        </div>

        <!-- 测试结果 -->
        <div v-if="testResult" class="test-result" :class="testResult.success ? 'success' : 'error'">
          <span class="test-icon">{{ testResult.success ? '✅' : '❌' }}</span>
          <span class="test-message">{{ testResult.message }}</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import axios from 'axios'
import OwlDiscoveryPanel from './OwlDiscoveryPanel.vue'

// ============ 类型定义 ============
export interface Camera {
  id?: string
  name: string
  url: string
  type: 'rtsp' | 'gb28181' | 'onvif'
  nodeId?: string
  group?: string
  streamMode?: string
  owlChannelId?: string
  yoloEnabled?: boolean
  yoloModel?: string
  yoloConfidence?: number
  vlmEnabled?: boolean
  vlmInterval?: number
  username?: string
  password?: string
  notes?: string
}

export interface Node {
  id: string
  name?: string
  status?: string
}

// ============ Props ============
interface Props {
  visible: boolean
  camera?: Camera | null
  nodes?: Node[]
  groups?: string[]
  existingChannelIds?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  camera: null,
  nodes: () => [],
  groups: () => ['入口', '销售区', '收银区', '仓库', '办公区'],
  existingChannelIds: () => [],
})

// ============ Emits ============
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'save', camera: Camera): void
  (e: 'delete', cameraId: string): void
  (e: 'refresh'): void
}>()

// ============ 常量 ============
const streamModeLabels: Record<string, string> = {
  hub_sip_remote: 'GB28181 (SIP-媒体分离)',
  direct: 'RTSP 直连',
  worker_owl: 'Worker OWL (ONVIF)',
}

function streamModeLabel(mode: string): string {
  return streamModeLabels[mode] || mode
}

// ============ OWL Discovery callbacks ============
function onOwlImported(result: { count: number }) {
  if (result.count > 0) {
    emit('refresh')
  }
}

function onOwlAdded(_camera: any) {
  emit('refresh')
}

// ============ 状态 ============
const form = ref<Camera>({
  name: '',
  url: '',
  type: 'rtsp',
  nodeId: '',
  group: '',
  streamMode: '',
  owlChannelId: '',
  yoloEnabled: true,
  yoloModel: 'person',
  yoloConfidence: 0.5,
  vlmEnabled: false,
  vlmInterval: 5,
  username: '',
  password: '',
  notes: '',
})

const testing = ref(false)
const saving = ref(false)
const testResult = ref<{ success: boolean; message: string } | null>(null)

// ESC 键关闭弹窗
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.visible) {
    handleClose()
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleKeydown)
})

// ============ 计算属性 ============
const isEdit = computed(() => !!props.camera?.id)

const isFormValid = computed(() => {
  return form.value.name.trim() && form.value.url.trim()
})

const getUrlPlaceholder = computed(() => {
  switch (form.value.type) {
    case 'rtsp':
      return 'rtsp://192.168.1.100:554/stream1'
    case 'gb28181':
      return '设备ID@SIP服务器地址'
    case 'onvif':
      return 'http://192.168.1.100/onvif/device_service'
    default:
      return ''
  }
})

const getUrlHint = computed(() => {
  switch (form.value.type) {
    case 'rtsp':
      return '支持 rtsp:// 和 rtmp:// 协议'
    case 'gb28181':
      return '格式：设备ID@SIP服务地址:端口'
    case 'onvif':
      return 'ONVIF 设备服务地址'
    default:
      return ''
  }
})

// ============ 方法 ============
function resetForm() {
  form.value = {
    name: '',
    url: '',
    type: 'rtsp',
    nodeId: '',
    group: '',
    streamMode: '',
    owlChannelId: '',
    yoloEnabled: true,
    yoloModel: 'person',
    yoloConfidence: 0.5,
    vlmEnabled: false,
    vlmInterval: 5,
    username: '',
    password: '',
    notes: '',
  }
  testResult.value = null
}

function handleClose() {
  emit('update:visible', false)
}

async function handleTest() {
  if (!form.value.url) return
  
  testing.value = true
  testResult.value = null
  
  try {
    // 如果已有摄像头 ID，通过获取状态来测试
    if (props.camera?.id) {
      const res = await axios.get(`/api/v1/cameras/${props.camera.id}`)
      const status = res.data?.status
      testResult.value = {
        success: status === 'online',
        message: status === 'online' ? '连接正常' : (status === 'starting' ? '启动中...' : '离线'),
      }
    } else {
      // 新增摄像头时，只做 URL 格式验证
      const url = form.value.url.trim()
      const isValidUrl = url.startsWith('rtsp://') || url.startsWith('rtmp://') || url.startsWith('http')
      testResult.value = {
        success: isValidUrl,
        message: isValidUrl ? 'URL 格式正确，添加后可验证连接' : 'URL 格式无效',
      }
    }
  } catch (e: any) {
    testResult.value = {
      success: false,
      message: e.response?.data?.error || '测试失败',
    }
  } finally {
    testing.value = false
  }
}

async function handleSubmit() {
  if (!isFormValid.value) return
  
  saving.value = true
  
  try {
    const cameraData: Camera = {
      ...form.value,
      id: props.camera?.id,
    }
    
    emit('save', cameraData)
    handleClose()
  } finally {
    saving.value = false
  }
}

// ============ 监听 ============
watch(() => props.visible, (visible) => {
  if (visible) {
    if (props.camera) {
      form.value = { ...props.camera }
    } else {
      resetForm()
    }
  }
})
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  backdrop-filter: blur(4px);
  animation: fadeIn 0.2s ease;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.modal {
  background: #1e1e2e;
  border: 1px solid #333;
  border-radius: 16px;
  width: 90%;
  max-width: 560px;
  max-height: 90vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.4);
  animation: slideIn 0.3s ease;
  position: relative;
}

.modal.modal-wide {
  max-width: 680px;
}

/* 流模式信息条 */
.stream-mode-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  margin-bottom: 16px;
  background: rgba(59, 130, 246, 0.08);
  border: 1px solid rgba(59, 130, 246, 0.2);
  border-radius: 8px;
  font-size: 13px;
}

.mode-label {
  color: #9ca3af;
}

.mode-badge {
  padding: 2px 10px;
  border-radius: 10px;
  font-weight: 500;
  font-size: 12px;
}

.mode-badge.hub_sip_remote {
  background: #dbeafe;
  color: #1d4ed8;
}

.mode-badge.direct {
  background: #d1fae5;
  color: #059669;
}

.mode-badge.worker_owl {
  background: #fef3c7;
  color: #d97706;
}

.mode-channel {
  color: #6b7280;
  font-family: monospace;
  font-size: 12px;
}

@keyframes slideIn {
  from { opacity: 0; transform: translateY(-20px) scale(0.95); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid #333;
}

.modal-header h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: #e0e0e0;
}

.close-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: #666;
  padding: 4px;
  border-radius: 6px;
  transition: all 0.2s ease;
}

.close-btn svg {
  width: 20px;
  height: 20px;
}

.close-btn:hover {
  background: #2a2a3e;
  color: #e0e0e0;
}

.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
}

.form-section {
  margin-bottom: 24px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #9ca3af;
  margin: 0 0 16px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.form-group {
  margin-bottom: 16px;
}

.form-row {
  display: flex;
  gap: 16px;
}

.flex-1 {
  flex: 1;
}

.form-label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  color: #e0e0e0;
  margin-bottom: 8px;
}

.required {
  color: #ef4444;
}

.form-input, .form-select, .form-textarea {
  width: 100%;
  padding: 10px 14px;
  border: 1px solid #333;
  border-radius: 8px;
  background: #0f0f1a;
  color: #e0e0e0;
  font-size: 14px;
  transition: all 0.2s ease;
}

.form-input:focus, .form-select:focus, .form-textarea:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2);
}

.form-input::placeholder {
  color: #4b5563;
}

.form-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: #6b7280;
}

/* 单选按钮组 */
.radio-group {
  display: flex;
  gap: 8px;
}

.radio-label {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid #333;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.radio-label:hover {
  border-color: #4b5563;
}

.radio-label.active {
  border-color: #3b82f6;
  background: rgba(59, 130, 246, 0.1);
}

.radio-input {
  display: none;
}

.radio-icon {
  font-size: 16px;
}

.radio-text {
  font-size: 13px;
  color: #e0e0e0;
}

/* 开关组 */
.toggle-group {
  margin-bottom: 12px;
}

.toggle-label {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px;
  border: 1px solid #333;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.toggle-label:hover {
  border-color: #4b5563;
}

.toggle-input {
  display: none;
}

.toggle-switch {
  width: 40px;
  height: 22px;
  background: #333;
  border-radius: 11px;
  position: relative;
  flex-shrink: 0;
  transition: all 0.2s ease;
}

.toggle-switch::after {
  content: '';
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  background: #666;
  border-radius: 50%;
  transition: all 0.2s ease;
}

.toggle-input:checked + .toggle-switch {
  background: #3b82f6;
}

.toggle-input:checked + .toggle-switch::after {
  left: 21px;
  background: #fff;
}

.toggle-text {
  flex: 1;
}

.toggle-text strong {
  display: block;
  font-size: 14px;
  color: #e0e0e0;
  margin-bottom: 2px;
}

.toggle-text small {
  font-size: 12px;
  color: #6b7280;
}

.sub-options {
  margin: 12px 0 12px 52px;
  padding: 12px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 8px;
}

/* 滑块 */
.slider-group {
  display: flex;
  align-items: center;
  gap: 12px;
}

.form-slider {
  flex: 1;
  height: 6px;
  -webkit-appearance: none;
  background: #333;
  border-radius: 3px;
}

.form-slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 18px;
  height: 18px;
  background: #3b82f6;
  border-radius: 50%;
  cursor: pointer;
}

.slider-value {
  font-size: 14px;
  font-weight: 600;
  color: #3b82f6;
  min-width: 30px;
}

/* 高级设置 */
.advanced-section {
  border: 1px solid #333;
  border-radius: 8px;
}

.advanced-toggle {
  padding: 12px 16px;
  font-size: 14px;
  color: #9ca3af;
  cursor: pointer;
  list-style: none;
}

.advanced-toggle::-webkit-details-marker {
  display: none;
}

.advanced-toggle::before {
  content: '▶';
  margin-right: 8px;
  font-size: 10px;
  transition: transform 0.2s ease;
}

.advanced-section[open] .advanced-toggle::before {
  transform: rotate(90deg);
}

.advanced-content {
  padding: 16px;
  border-top: 1px solid #333;
}

/* 底部 */
.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 16px 24px;
  border-top: 1px solid #333;
}

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
  color: #fff;
}

.btn-primary:hover:not(:disabled) {
  background: #2563eb;
}

.btn-secondary {
  background: #333;
  color: #e0e0e0;
}

.btn-secondary:hover:not(:disabled) {
  background: #444;
}

.btn-outline {
  background: transparent;
  border: 1px solid #333;
  color: #e0e0e0;
}

.btn-outline:hover:not(:disabled) {
  border-color: #3b82f6;
  color: #3b82f6;
}

/* 测试结果 */
.test-result {
  position: absolute;
  bottom: 80px;
  left: 24px;
  right: 24px;
  padding: 12px 16px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  gap: 8px;
  animation: slideUp 0.3s ease;
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

.test-result.success {
  background: rgba(16, 185, 129, 0.2);
  border: 1px solid #10b981;
}

.test-result.error {
  background: rgba(239, 68, 68, 0.2);
  border: 1px solid #ef4444;
}

.test-icon {
  font-size: 16px;
}

.test-message {
  font-size: 14px;
  color: #e0e0e0;
}
</style>
