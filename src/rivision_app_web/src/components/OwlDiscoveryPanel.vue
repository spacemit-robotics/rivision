<template>
  <div class="owl-panel">
    <!-- 标签页切换 -->
    <div class="tab-bar">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="tab-btn"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        <span class="tab-icon">{{ tab.icon }}</span>
        {{ tab.label }}
      </button>
    </div>

    <!-- ═══ GB28181 设备/通道列表 ═══ -->
    <div v-if="activeTab === 'gb28181'" class="tab-content">
      <div class="panel-toolbar">
        <button class="btn btn-outline btn-sm" @click="loadDevicesAndChannels" :disabled="loadingChannels">
          {{ loadingChannels ? '加载中...' : '🔄 刷新设备' }}
        </button>
        <button
          class="btn btn-primary btn-sm"
          @click="importSelected"
          :disabled="selectedChannels.size === 0 || importing"
        >
          {{ importing ? '导入中...' : `📥 导入选中 (${selectedChannels.size})` }}
        </button>
        <button
          class="btn btn-outline btn-sm"
          @click="importAll"
          :disabled="channels.length === 0 || importing"
        >
          📥 全部导入
        </button>
      </div>

      <div v-if="loadingChannels" class="loading-state">
        <div class="spinner-sm"></div>
        <span>正在获取设备列表...</span>
      </div>

      <div v-else-if="channels.length === 0" class="empty-mini">
        <span>暂无 GB28181 设备注册</span>
        <p class="hint">请确保摄像头 SIP 已注册到 OWL ({{ owlSipInfo }})</p>
      </div>

      <div v-else class="channel-list">
        <div class="list-header">
          <label class="checkbox-label">
            <input type="checkbox" :checked="allGbSelected" @change="toggleAllGb" />
            <span>全选 ({{ channels.length }} 通道)</span>
          </label>
        </div>
        <div
          v-for="ch in channels"
          :key="ch.id"
          class="channel-item"
          :class="{
            selected: selectedChannels.has(ch.id),
            imported: importedIds.has(ch.id),
            disabled: importedIds.has(ch.id),
          }"
        >
          <input
            type="checkbox"
            :checked="selectedChannels.has(ch.id)"
            :disabled="importedIds.has(ch.id)"
            @change="toggleChannel(ch.id)"
          />
          <div class="channel-info">
            <div class="channel-name">
              {{ ch.name || ch.id }}
              <span v-if="ch.online" class="dot online"></span>
              <span v-else class="dot offline"></span>
            </div>
            <div class="channel-id">{{ ch.id }}</div>
            <div v-if="ch.device_id" class="channel-device">设备: {{ ch.device_id }}</div>
          </div>
          <span v-if="importedIds.has(ch.id)" class="imported-badge">已导入</span>
          <span v-else class="protocol-badge gb28181">GB28181</span>
        </div>
      </div>
    </div>

    <!-- ═══ ONVIF 发现 ═══ -->
    <div v-if="activeTab === 'onvif'" class="tab-content">
      <div class="panel-toolbar">
        <button class="btn btn-primary btn-sm" @click="startDiscover" :disabled="discovering">
          {{ discovering ? '扫描中...' : '🔍 扫描局域网' }}
        </button>
      </div>

      <div v-if="discovering" class="loading-state">
        <div class="spinner-sm"></div>
        <span>正在扫描 ONVIF 设备...</span>
      </div>

      <div v-else-if="discoveredDevices.length === 0" class="empty-mini">
        <span>点击"扫描局域网"发现 ONVIF 设备</span>
      </div>

      <div v-else class="channel-list">
        <div
          v-for="(dev, idx) in discoveredDevices"
          :key="idx"
          class="channel-item"
        >
          <div class="channel-info">
            <div class="channel-name">
              {{ dev.name || dev.ip }}
              <span class="dot online"></span>
            </div>
            <div class="channel-id">{{ dev.ip }}{{ dev.port ? ':' + dev.port : '' }}</div>
            <div v-if="dev.manufacturer" class="channel-device">
              {{ dev.manufacturer }} {{ dev.model || '' }}
            </div>
            <div v-if="dev.profiles && dev.profiles.length > 0" class="profiles-list">
              <span
                v-for="p in dev.profiles"
                :key="p.token"
                class="profile-tag"
                :title="p.stream_url"
              >
                {{ p.name }}
              </span>
            </div>
          </div>
          <button
            class="btn btn-outline btn-sm"
            @click="importOnvifDevice(dev)"
            :disabled="importing"
          >
            📥 导入
          </button>
        </div>
      </div>
    </div>

    <!-- ═══ RTSP 手动添加 ═══ -->
    <div v-if="activeTab === 'rtsp'" class="tab-content">
      <div class="form-section-mini">
        <div class="form-group">
          <label class="form-label">摄像头名称 <span class="required">*</span></label>
          <input v-model="rtspForm.name" type="text" class="form-input" placeholder="例如：门口摄像头" />
        </div>
        <div class="form-group">
          <label class="form-label">RTSP 地址 <span class="required">*</span></label>
          <input
            v-model="rtspForm.url"
            type="text"
            class="form-input"
            placeholder="rtsp://admin:pass@192.168.1.100:554/stream1"
          />
          <p class="form-hint">支持 rtsp:// 和 rtmp:// 协议</p>
        </div>
        <div class="form-group">
          <label class="form-label">所属节点</label>
          <select v-model="rtspForm.nodeId" class="form-select">
            <option value="">自动分配</option>
            <option v-for="node in nodes" :key="node.id" :value="node.id">
              {{ node.name || node.id }}
            </option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">分组</label>
          <select v-model="rtspForm.groupId" class="form-select">
            <option value="">无分组</option>
            <option v-for="group in groups" :key="group" :value="group">{{ group }}</option>
          </select>
        </div>
        <button
          class="btn btn-primary"
          @click="addRtspCamera"
          :disabled="!rtspForm.name || !rtspForm.url || saving"
        >
          {{ saving ? '添加中...' : '➕ 添加摄像头' }}
        </button>
      </div>
    </div>

    <!-- 导入结果提示 -->
    <div v-if="importMessage" :class="['import-toast', importSuccess ? 'success' : 'error']">
      {{ importMessage }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { owlApi, type OwlChannel } from '@/api/owl'
import axios from 'axios'

// ═══ Props ═══
interface Props {
  nodes?: Array<{ id: string; name?: string }>
  groups?: string[]
  existingCameraChannelIds?: string[]
}

const props = withDefaults(defineProps<Props>(), {
  nodes: () => [],
  groups: () => ['入口', '销售区', '收银区', '仓库', '办公区'],
  existingCameraChannelIds: () => [],
})

const emit = defineEmits<{
  (e: 'imported', result: { count: number }): void
  (e: 'added', camera: any): void
}>()

// ═══ Tabs ═══
const tabs = [
  { key: 'gb28181', label: 'GB28181 设备', icon: '🔗' },
  { key: 'onvif', label: 'ONVIF 发现', icon: '🌐' },
  { key: 'rtsp', label: 'RTSP 手动', icon: '📹' },
]
const activeTab = ref('gb28181')

// ═══ GB28181 state ═══
const channels = ref<OwlChannel[]>([])
const selectedChannels = ref<Set<string>>(new Set())
const loadingChannels = ref(false)
const importing = ref(false)
const importedIds = ref<Set<string>>(new Set(props.existingCameraChannelIds))
const owlSipInfo = ref('SIP 15060')

// ═══ ONVIF state ═══
const discovering = ref(false)
const discoveredDevices = ref<any[]>([])

// ═══ RTSP state ═══
const rtspForm = ref({ name: '', url: '', nodeId: '', groupId: '' })
const saving = ref(false)

// ═══ Toast ═══
const importMessage = ref('')
const importSuccess = ref(true)

function showToast(msg: string, success: boolean) {
  importMessage.value = msg
  importSuccess.value = success
  setTimeout(() => { importMessage.value = '' }, 4000)
}

// ═══ GB28181: load devices & channels ═══
async function loadDevicesAndChannels() {
  loadingChannels.value = true
  try {
    const [chRes, infoRes] = await Promise.all([
      owlApi.listChannels(),
      owlApi.serverInfo().catch(() => null),
    ])
    channels.value = (chRes as any)?.channels || (chRes as any) || []
    if (infoRes) {
      const info = infoRes as any
      owlSipInfo.value = `SIP ${info.sip_host || ''}:${info.sip_port || 15060}`
    }
  } catch (e: any) {
    showToast('获取设备失败: ' + (e.response?.data?.error || e.message), false)
  } finally {
    loadingChannels.value = false
  }
}

const allGbSelected = computed(() =>
  channels.value.length > 0 &&
  channels.value.filter(c => !importedIds.value.has(c.id)).every(c => selectedChannels.value.has(c.id))
)

function toggleAllGb() {
  if (allGbSelected.value) {
    selectedChannels.value.clear()
  } else {
    channels.value.forEach(c => {
      if (!importedIds.value.has(c.id)) {
        selectedChannels.value.add(c.id)
      }
    })
  }
}

function toggleChannel(id: string) {
  if (selectedChannels.value.has(id)) {
    selectedChannels.value.delete(id)
  } else {
    selectedChannels.value.add(id)
  }
}

// ═══ GB28181: import selected ═══
async function importSelected() {
  if (selectedChannels.value.size === 0) return
  importing.value = true
  let ok = 0, fail = 0
  try {
    for (const chId of selectedChannels.value) {
      try {
        await owlApi.importChannel(chId)
        importedIds.value.add(chId)
        ok++
      } catch {
        fail++
      }
    }
    selectedChannels.value.clear()
    showToast(`导入完成: 成功 ${ok}, 失败 ${fail}`, fail === 0)
    emit('imported', { count: ok })
  } finally {
    importing.value = false
  }
}

async function importAll() {
  importing.value = true
  try {
    const res = await owlApi.importBatch({}) as any
    const results = res?.results || []
    const ok = results.filter((r: any) => !r.error).length
    const fail = results.filter((r: any) => r.error).length
    results.forEach((r: any) => { if (r.channel_id && !r.error) importedIds.value.add(r.channel_id) })
    showToast(`批量导入: 成功 ${ok}, 失败 ${fail}`, fail === 0)
    emit('imported', { count: ok })
  } catch (e: any) {
    showToast('批量导入失败: ' + (e.response?.data?.error || e.message), false)
  } finally {
    importing.value = false
  }
}

// ═══ ONVIF: discover ═══
async function startDiscover() {
  discovering.value = true
  try {
    const res = await owlApi.discover() as any
    discoveredDevices.value = res?.devices || res || []
    if (discoveredDevices.value.length === 0) {
      showToast('未发现 ONVIF 设备', false)
    } else {
      showToast(`发现 ${discoveredDevices.value.length} 个 ONVIF 设备`, true)
    }
  } catch (e: any) {
    showToast('ONVIF 扫描失败: ' + (e.response?.data?.error || e.message), false)
  } finally {
    discovering.value = false
  }
}

async function importOnvifDevice(dev: any) {
  importing.value = true
  try {
    // ONVIF: 通过 Hub 创建摄像头，使用 profile 的 stream_url (direct mode)
    // 或无 URL 时走 worker_owl mode
    const profile = dev.profiles?.[0]
    const payload = {
      name: dev.name || dev.ip,
      url: profile?.stream_url || '',
      protocol: 'onvif',
      config: {
        onvif_host: dev.ip,
        onvif_port: dev.port,
        manufacturer: dev.manufacturer,
        model: dev.model,
      },
    }
    await axios.post('/api/v1/cameras', payload)
    showToast(`已导入 ${dev.name || dev.ip}`, true)
    emit('added', payload)
  } catch (e: any) {
    showToast('导入失败: ' + (e.response?.data?.error || e.message), false)
  } finally {
    importing.value = false
  }
}

// ═══ RTSP: add camera ═══
async function addRtspCamera() {
  if (!rtspForm.value.name || !rtspForm.value.url) return
  saving.value = true
  try {
    const payload = {
      name: rtspForm.value.name,
      url: rtspForm.value.url,
      protocol: 'rtsp',
      node_id: rtspForm.value.nodeId || undefined,
      group_id: rtspForm.value.groupId || undefined,
      config: { stream_mode: 'direct' },
    }
    await axios.post('/api/v1/cameras', payload)
    showToast(`已添加 ${rtspForm.value.name}`, true)
    emit('added', payload)
    rtspForm.value = { name: '', url: '', nodeId: '', groupId: '' }
  } catch (e: any) {
    showToast('添加失败: ' + (e.response?.data?.error || e.message), false)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadDevicesAndChannels()
})
</script>

<style scoped>
.owl-panel {
  position: relative;
}

/* ── Tab bar ── */
.tab-bar {
  display: flex;
  gap: 4px;
  margin-bottom: 16px;
  border-bottom: 2px solid #e5e7eb;
  padding-bottom: 0;
}

.tab-btn {
  padding: 10px 16px;
  border: none;
  background: transparent;
  font-size: 14px;
  font-weight: 500;
  color: #6b7280;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -2px;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
}

.tab-btn:hover {
  color: #374151;
}

.tab-btn.active {
  color: #3b82f6;
  border-bottom-color: #3b82f6;
}

.tab-icon {
  font-size: 16px;
}

/* ── Toolbar ── */
.panel-toolbar {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

/* ── Channel list ── */
.channel-list {
  max-height: 400px;
  overflow-y: auto;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.list-header {
  padding: 8px 12px;
  background: #f9fafb;
  border-bottom: 1px solid #e5e7eb;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #374151;
  cursor: pointer;
}

.checkbox-label input {
  width: 16px;
  height: 16px;
}

.channel-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-bottom: 1px solid #f3f4f6;
  transition: background 0.15s;
}

.channel-item:last-child {
  border-bottom: none;
}

.channel-item:hover:not(.disabled) {
  background: #f0f9ff;
}

.channel-item.selected {
  background: #eff6ff;
}

.channel-item.imported {
  opacity: 0.6;
}

.channel-item input[type="checkbox"] {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
}

.channel-info {
  flex: 1;
  min-width: 0;
}

.channel-name {
  font-size: 14px;
  font-weight: 500;
  color: #1f2937;
  display: flex;
  align-items: center;
  gap: 6px;
}

.channel-id {
  font-size: 12px;
  color: #9ca3af;
  font-family: monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.channel-device {
  font-size: 11px;
  color: #6b7280;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}

.dot.online {
  background: #10b981;
}

.dot.offline {
  background: #ef4444;
}

/* ── Badges ── */
.protocol-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
  white-space: nowrap;
}

.protocol-badge.gb28181 {
  background: #dbeafe;
  color: #1d4ed8;
}

.imported-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  background: #d1fae5;
  color: #059669;
  font-weight: 500;
}

/* ── Profiles ── */
.profiles-list {
  display: flex;
  gap: 4px;
  margin-top: 4px;
  flex-wrap: wrap;
}

.profile-tag {
  font-size: 10px;
  padding: 1px 6px;
  background: #f3f4f6;
  border-radius: 4px;
  color: #6b7280;
}

/* ── RTSP form ── */
.form-section-mini {
  padding: 4px 0;
}

.form-group {
  margin-bottom: 14px;
}

.form-label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  margin-bottom: 6px;
}

.required {
  color: #ef4444;
}

.form-input,
.form-select {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid #d1d5db;
  border-radius: 8px;
  font-size: 14px;
  transition: border-color 0.2s;
}

.form-input:focus,
.form-select:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.form-hint {
  margin: 4px 0 0;
  font-size: 12px;
  color: #9ca3af;
}

/* ── Loading / empty ── */
.loading-state {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 30px 0;
  justify-content: center;
  color: #6b7280;
  font-size: 14px;
}

.spinner-sm {
  width: 20px;
  height: 20px;
  border: 2px solid #e5e7eb;
  border-top-color: #3b82f6;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.empty-mini {
  text-align: center;
  padding: 30px 16px;
  color: #9ca3af;
  font-size: 14px;
}

.empty-mini .hint {
  font-size: 12px;
  margin-top: 6px;
  color: #d1d5db;
}

/* ── Toast ── */
.import-toast {
  position: absolute;
  bottom: -40px;
  left: 0;
  right: 0;
  padding: 10px 16px;
  border-radius: 8px;
  font-size: 13px;
  text-align: center;
  animation: slideUp 0.3s ease;
  z-index: 10;
}

.import-toast.success {
  background: #d1fae5;
  color: #059669;
}

.import-toast.error {
  background: #fee2e2;
  color: #dc2626;
}

@keyframes slideUp {
  from { opacity: 0; transform: translateY(8px); }
  to { opacity: 1; transform: translateY(0); }
}

/* ── Buttons (local) ── */
.btn {
  padding: 8px 16px;
  border: none;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-sm {
  padding: 6px 12px;
  font-size: 12px;
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
</style>
