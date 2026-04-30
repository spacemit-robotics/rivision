<template>
  <div v-if="visible" class="overlay" @click.self="$emit('update:visible', false)">
    <div class="modal">
      <div class="header">
        <h3>摄像头管理</h3>
        <button class="close-btn" @click="$emit('update:visible', false)">×</button>
      </div>

      <div class="add-form">
        <input v-model="newName" placeholder="流名称(唯一ID)" />
        <input v-model="newSource" placeholder="RTSP/RTMP/HTTP 源地址" />
        <button @click="addStream" :disabled="submitting">新增</button>
      </div>

      <div class="hint">
        提示：新增后会自动出现在监控页摄像头列表。
        <span class="status-pill" :class="owlStatus.ok ? 'ok' : 'warn'">
          OWL: {{ owlStatus.ok ? `已连接(${owlStatus.count}路${owlStatus.message ? `, ${owlStatus.message}` : ''})` : `未连接(${owlStatus.message || '接口不可用'})` }}
        </span>
      </div>

      <div class="summary-row">
        <div class="summary-card">
          <div class="label">总流数</div>
          <div class="value">{{ streams.length }}</div>
        </div>
        <div class="summary-card ok">
          <div class="label">在线</div>
          <div class="value">{{ onlineCount }}</div>
        </div>
        <div class="summary-card warn">
          <div class="label">离线</div>
          <div class="value">{{ offlineCount }}</div>
        </div>
        <div class="summary-card">
          <div class="label">OWL来源</div>
          <div class="value">{{ owlSourceCount }}</div>
        </div>
      </div>

      <div class="filter-row">
        <input v-model="keyword" class="filter-input" placeholder="搜索名称/ID/来源" />
        <select v-model="sourceFilter" class="filter-select">
          <option value="all">全部来源</option>
          <option value="owl">仅OWL</option>
          <option value="go2rtc">仅go2rtc</option>
          <option value="mixed">OWL+go2rtc</option>
        </select>
        <select v-model="statusFilter" class="filter-select">
          <option value="all">全部状态</option>
          <option value="online">仅在线</option>
          <option value="offline">仅离线</option>
        </select>
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>名称</th>
              <th>在线</th>
              <th>来源</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="s in filteredStreams" :key="s.id">
              <td>{{ s.name || s.id }}</td>
              <td>{{ s.online ? '在线' : '离线' }}</td>
              <td class="source">{{ s.source || '-' }}</td>
              <td>
                <button v-if="s.removable" class="danger" @click="removeStream(s.id)">删除</button>
                <span v-else class="readonly-op">只读</span>
              </td>
            </tr>
            <tr v-if="streams.length === 0">
              <td colspan="4" class="empty">暂无流</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="footer">
        <span class="msg" :class="{ error: !!errorMsg }">{{ errorMsg || successMsg }}</span>
        <button @click="refresh">刷新</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

interface StreamRow {
  id: string
  name: string
  online: boolean
  source: string
  removable: boolean
}

interface OwlChannel {
  id?: string
  channel_id?: string
  name?: string
  is_online?: boolean
  type?: string
}

interface Go2rtcStreamInfo {
  name?: string
  sources?: string[]
  producers?: Array<{ type?: string; url?: string }>
}

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'updated'): void }>()

const streams = ref<StreamRow[]>([])
const newName = ref('')
const newSource = ref('')
const submitting = ref(false)
const errorMsg = ref('')
const successMsg = ref('')
const owlStatus = ref<{ ok: boolean; count: number; message: string }>({ ok: false, count: 0, message: '' })
const keyword = ref('')
const sourceFilter = ref<'all' | 'owl' | 'go2rtc' | 'mixed'>('all')
const statusFilter = ref<'all' | 'online' | 'offline'>('all')

const onlineCount = computed(() => streams.value.filter(s => s.online).length)
const offlineCount = computed(() => streams.value.length - onlineCount.value)
const owlSourceCount = computed(() => streams.value.filter(s => s.source.toLowerCase().includes('owl')).length)

const filteredStreams = computed(() => {
  let rows = [...streams.value]
  const kw = keyword.value.trim().toLowerCase()
  if (kw) {
    rows = rows.filter((r) =>
      r.id.toLowerCase().includes(kw) ||
      r.name.toLowerCase().includes(kw) ||
      r.source.toLowerCase().includes(kw)
    )
  }

  if (sourceFilter.value !== 'all') {
    rows = rows.filter((r) => {
      const src = r.source.toLowerCase()
      if (sourceFilter.value === 'owl') return src.includes('owl') && !src.includes('+')
      if (sourceFilter.value === 'go2rtc') return !src.includes('owl')
      if (sourceFilter.value === 'mixed') return src.includes('+')
      return true
    })
  }

  if (statusFilter.value === 'online') rows = rows.filter(r => r.online)
  if (statusFilter.value === 'offline') rows = rows.filter(r => !r.online)

  return rows
})

function mergeRows(owlRows: StreamRow[], gRows: StreamRow[]): StreamRow[] {
  const merged = new Map<string, StreamRow>()

  for (const r of gRows) {
    merged.set(r.id, { ...r })
  }

  for (const r of owlRows) {
    const old = merged.get(r.id)
    if (!old) {
      merged.set(r.id, { ...r })
      continue
    }
    merged.set(r.id, {
      id: r.id,
      name: r.name || old.name,
      online: r.online || old.online,
      source: old.source.includes('OWL') ? old.source : `${old.source}+OWL`,
      removable: old.removable,
    })
  }

  return Array.from(merged.values()).sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name, 'zh-CN'))
}

async function loadOwlRows(): Promise<StreamRow[]> {
  try {
    const infoResp = await fetch('/api/owl/server/info')
    if (!infoResp.ok) {
      owlStatus.value = { ok: false, count: 0, message: `HTTP ${infoResp.status}` }
      return []
    }

    const resp = await fetch('/api/owl/channels?page=1&page_size=500')
    if (!resp.ok) {
      // 连接成功但通道接口不可用
      owlStatus.value = { ok: true, count: 0, message: `channels HTTP ${resp.status}` }
      return []
    }
    const data = await resp.json()
    // 优先使用 data.data.channels (rivision_cli 格式), 其次 data.items (OWL 直连格式)
    const items = ((data?.data?.channels || data?.data?.items || data?.items || []) as OwlChannel[])
    owlStatus.value = { ok: true, count: items.length, message: '' }

    return items.map((ch) => {
      const id = (ch.id || ch.channel_id || '').trim()
      const name = (ch.name || ch.channel_id || ch.id || '未命名摄像头').trim()
      return {
        id,
        name,
        online: ch.is_online !== false,
        source: `OWL${ch.type ? `(${ch.type})` : ''}`,
        removable: false,
      }
    }).filter((r) => !!r.id)
  } catch (e: any) {
    owlStatus.value = { ok: false, count: 0, message: e?.message || '连接失败' }
    return []
  }
}

async function loadGo2rtcRows(): Promise<StreamRow[]> {
  const resp = await fetch('/api/go2rtc/streams')
  if (!resp.ok) return []

  const data: Record<string, Go2rtcStreamInfo> = await resp.json()
  return Object.entries(data)
    .filter(([name]) => !['status', 'payload', 'error'].includes(name))
    .map(([id, info]) => {
      const producers = info.producers ?? []
      return {
        id,
        name: (info.name && info.name.trim()) || id,
        online: Array.isArray(producers) ? producers.length > 0 : false,
        source: info.sources?.[0] || 'go2rtc',
        removable: true,
      }
    })
}

async function refresh() {
  errorMsg.value = ''
  successMsg.value = ''

  try {
    const [owlRows, go2rtcRows] = await Promise.all([loadOwlRows(), loadGo2rtcRows()])
    streams.value = mergeRows(owlRows, go2rtcRows)

    if (streams.value.length === 0) {
      if (!owlStatus.value.ok) {
        errorMsg.value = '当前未获取到 OWL 通道，且 go2rtc 也没有可用流。请检查 OWL 接口与 go2rtc。'
      } else {
        errorMsg.value = 'OWL 已连接，但暂无可用通道。请检查设备/通道在线状态。'
      }
    }
  } catch (e: any) {
    errorMsg.value = e?.message || '加载流列表失败'
  }
}

async function addStream() {
  const name = newName.value.trim()
  const src = newSource.value.trim()
  if (!name || !src) {
    errorMsg.value = '请填写流名称和源地址'
    return
  }

  submitting.value = true
  errorMsg.value = ''
  successMsg.value = ''
  try {
    const url = `/api/go2rtc/streams?name=${encodeURIComponent(name)}&src=${encodeURIComponent(src)}`
    const resp = await fetch(url, { method: 'PUT' })
    if (!resp.ok) throw new Error(`新增失败 HTTP ${resp.status}`)
    successMsg.value = '新增成功'
    newName.value = ''
    newSource.value = ''
    await refresh()
    emit('updated')
  } catch (e: any) {
    errorMsg.value = e?.message || '新增失败'
  } finally {
    submitting.value = false
  }
}

async function removeStream(name: string) {
  errorMsg.value = ''
  successMsg.value = ''
  try {
    const url = `/api/go2rtc/streams?src=${encodeURIComponent(name)}`
    const resp = await fetch(url, { method: 'DELETE' })
    if (!resp.ok) throw new Error(`删除失败 HTTP ${resp.status}`)
    successMsg.value = `已删除: ${name}`
    await refresh()
    emit('updated')
  } catch (e: any) {
    errorMsg.value = e?.message || '删除失败'
  }
}

watch(() => props.visible, (v) => {
  if (v) refresh()
})
</script>

<style scoped>
.overlay { position: fixed; inset: 0; background: rgba(0,0,0,.45); display: flex; align-items: center; justify-content: center; z-index: 1200; }
.modal { width: min(980px, 92vw); max-height: 86vh; overflow: auto; background: #1e1e2e; color: #e6e6e6; border: 1px solid #333; border-radius: 12px; padding: 14px; }
.header { display:flex; justify-content: space-between; align-items:center; }
.close-btn { background: transparent; border: none; color: #ddd; font-size: 22px; cursor: pointer; }
.add-form { margin-top: 10px; display:grid; grid-template-columns: 180px 1fr 80px; gap: 8px; }
.add-form input { height: 34px; border: 1px solid #444; border-radius: 8px; background:#252535; color:#e6e6e6; padding: 0 10px; }
.add-form button { border: 1px solid #4a9eff; background:#4a9eff; color: #fff; border-radius: 8px; }
.hint { margin-top: 8px; font-size: 12px; color: #999; display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.status-pill { border: 1px solid #3f3f52; background: #24283a; color: #c8cfde; border-radius: 999px; padding: 2px 8px; font-size: 12px; }
.status-pill.ok { border-color: #2f8f61; background: #1f3328; color: #a6efc8; }
.status-pill.warn { border-color: #8f6a31; background: #332919; color: #f0d39b; }
.summary-row { margin-top: 10px; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 8px; }
.summary-card { border: 1px solid #3a3a50; background: #24283a; border-radius: 10px; padding: 8px 10px; }
.summary-card .label { font-size: 12px; color: #9aa3bd; }
.summary-card .value { margin-top: 2px; font-size: 18px; font-weight: 700; color: #d8def0; }
.summary-card.ok { border-color: #2f8f61; background: #1f3328; }
.summary-card.ok .value { color: #a6efc8; }
.summary-card.warn { border-color: #8f6a31; background: #332919; }
.summary-card.warn .value { color: #f0d39b; }

.filter-row { margin-top: 10px; display: grid; grid-template-columns: 1fr 160px 160px; gap: 8px; }
.filter-input { height: 32px; border: 1px solid #444; border-radius: 8px; background:#252535; color:#e6e6e6; padding: 0 10px; }
.filter-select { height: 32px; border: 1px solid #444; border-radius: 8px; background:#252535; color:#ddd; padding: 0 8px; }

.table-wrap { margin-top: 10px; border: 1px solid #333; border-radius: 8px; overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
th, td { border-bottom: 1px solid #2f2f3d; padding: 8px; font-size: 13px; text-align: left; }
.source { max-width: 520px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.danger { background:#2b1f22; border:1px solid #804050; color:#ffb3c0; border-radius:6px; padding:4px 8px; }
.readonly-op { color: #8d98b3; font-size: 12px; }
.empty { text-align: center; color: #888; }
.footer { margin-top: 10px; display:flex; justify-content: space-between; align-items:center; }
.msg { color:#9ad17d; font-size: 12px; }
.msg.error { color:#ff8a8a; }
</style>
