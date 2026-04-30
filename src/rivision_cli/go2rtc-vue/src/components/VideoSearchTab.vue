<template>
  <div class="video-search-tab">
    <div class="search-toolbar">
      <div class="search-mode-tabs">
        <button 
          :class="['mode-tab', searchMode === 'text' ? 'active' : '']" 
          @click="searchMode = 'text'">文字搜索</button>
        <button 
          :class="['mode-tab', searchMode === 'image' ? 'active' : '']" 
          @click="searchMode = 'image'">以图搜索</button>
      </div>
      
      <div v-if="searchMode === 'text'" class="search-row">
        <input
          v-model="query"
          class="search-input"
          placeholder="输入自然语言描述，例如：找出现两辆车并有人停留的片段"
        />
      </div>
      
      <div v-if="searchMode === 'image'" class="search-row image-upload-row">
        <input 
          type="file" 
          ref="imageInput"
          accept="image/*" 
          @change="onImageSelect"
          style="display: none"
        />
        <button class="upload-btn" @click="($refs.imageInput as HTMLInputElement)?.click()">
          {{ selectedImage ? '重新选择图片' : '📷 选择图片' }}
        </button>
        <span v-if="selectedImage" class="selected-file">{{ selectedImageName }}</span>
        <img v-if="imagePreview" :src="imagePreview" class="image-preview" />
      </div>
      
      <div class="search-row">
        <select v-model="cameraFilter" class="filter-select">
          <option value="all">全部摄像头</option>
          <option v-for="cam in availableCameras" :key="cam.id" :value="cam.id">{{ cam.name || cam.id }}</option>
        </select>
        <button class="search-btn" :disabled="searching || (searchMode === 'image' && !selectedImage)" @click="runSearch">
          {{ searching ? '搜索中...' : '搜索' }}
        </button>
      </div>
    </div>


    <div v-if="statusHint" class="status-hint" :class="statusHintType">
      {{ statusHint }}
    </div>

    <div class="result-list">
      <div v-if="results.length === 0" class="empty-state">
        {{ emptyText }}
      </div>

      <div
        v-for="item in results"
        :key="item.id"
        class="result-item"
        @click="$emit('view-detail', item)"
      >
        <img v-if="item.thumbnail" class="thumb" :src="item.thumbnail" alt="thumbnail" />
        <div v-else class="thumb empty">无缩略图</div>

        <div class="meta">
          <div class="line1">
            <span class="camera">摄像头名称：{{ getCameraName(item) }}<template v-if="item.nodeId"> ({{ item.nodeId }})</template></span>
            <span class="time">{{ formatTs(item.timestamp) }}</span>
          </div>
          <div class="line2">{{ item.result || '（无描述）' }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'

interface SearchItem {
  id: string
  timestamp: number
  cameraId: string
  cameraName?: string
  result?: string
  thumbnail?: string
  nodeId?: string
  processingTime?: number
  resolution?: string
}

interface CameraOption {
  id: string
  name: string
}

const props = defineProps<{
  historyItems: SearchItem[]
  availableCameras: CameraOption[]
}>()

defineEmits<{
  (e: 'view-detail', item: SearchItem): void
}>()

const query = ref('')
const cameraFilter = ref('all')
const searchMode = ref<'text' | 'image'>('text')
const selectedImage = ref<File | null>(null)
const selectedImageName = ref('')
const imagePreview = ref('')
const submittedQuery = ref('')
const searching = ref(false)
const searchedResults = ref<SearchItem[] | null>(null)
const statusHint = ref('')
const statusHintType = ref<'info' | 'warn' | 'ok'>('info')

function normalize(text: string) {
  return (text || '').toLowerCase().trim()
}

function getCameraName(item: SearchItem): string {
  // 优先使用项目自带的 cameraName，否则查找 availableCameras
  if (item.cameraName) return item.cameraName
  const cam = props.availableCameras.find(c => c.id === item.cameraId)
  return cam?.name || item.cameraId
}

function onImageSelect(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    selectedImage.value = file
    selectedImageName.value = file.name
    // 生成预览
    const reader = new FileReader()
    reader.onload = () => {
      imagePreview.value = reader.result as string
    }
    reader.readAsDataURL(file)
  }
}

async function imageToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      // 移除 data:image/xxx;base64, 前缀
      const base64 = result.split(',')[1] || result
      resolve(base64)
    }
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}

async function runSearch() {
  // 不立即清空结果，等新结果返回后再更新
  statusHint.value = ''
  
  // 以图搜索模式
  if (searchMode.value === 'image' && selectedImage.value) {
    submittedQuery.value = '[以图搜索]'
    searching.value = true
    try {
      // 先用当前历史记录增量更新本地语义索引（与文字搜索一致）
      if (props.historyItems.length > 0) {
        await fetch('/api/vlm/search/reindex', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            items: props.historyItems.map((i) => ({
              task_id: i.id,
              timestamp: Math.floor(i.timestamp / 1000),
              camera_id: i.cameraId,
              stream_name: i.cameraName || i.cameraId,
              thumbnail: i.thumbnail || '',
              result: i.result || '',
              node_id: i.nodeId || '',
              processing_time: i.processingTime || 0,
              resolution: i.resolution || '',
            })),
          }),
        })
      }

      const imageBase64 = await imageToBase64(selectedImage.value)
      const resp = await fetch('/api/vlm/search/hybrid', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          text_query: '',
          image_base64: imageBase64,
          camera_id: cameraFilter.value,
          limit: 100,
        }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const data = await resp.json()
      const items = (data.items || []) as any[]
      searchedResults.value = items.map((it) => ({
        id: it.task_id,
        timestamp: new Date(it.timestamp).getTime(),
        cameraId: it.camera_id,
        cameraName: it.stream_name || it.camera_id,
        result: it.result,
        thumbnail: it.thumbnail,
        nodeId: it.node_id,
        processingTime: it.processing_time,
        resolution: it.resolution,
      }))
      if (searchedResults.value.length === 0) {
        statusHintType.value = 'info'
        statusHint.value = '以图搜索完成，但没有找到相似图片。'
      } else {
        statusHintType.value = 'ok'
        statusHint.value = `以图搜索完成：找到 ${searchedResults.value.length} 个相似结果。`
      }
    } catch (e) {
      console.error('[VideoSearchTab] image search failed:', e)
      statusHintType.value = 'warn'
      statusHint.value = '以图搜索失败，请检查网络连接。'
    } finally {
      searching.value = false
    }
    return
  }
  
  // 文字搜索模式
  submittedQuery.value = query.value
  const kw = normalize(submittedQuery.value)

  if (props.availableCameras.length === 0) {
    statusHintType.value = 'warn'
    statusHint.value = '当前没有可用摄像头，请先在“摄像头管理”中确认流已接入并在线。'
  }

  if (!kw) {
    statusHintType.value = 'info'
    statusHint.value = '请输入搜索描述（自然语言）后再执行搜索。'
    return
  }

  if (props.historyItems.length === 0) {
    statusHintType.value = 'warn'
    statusHint.value = '暂无VLM结果可检索：当前未执行VLM分析或没有可用算力节点。'
    return
  }

  searching.value = true
  try {
    // 先用当前历史记录增量更新本地语义索引
    await fetch('/api/vlm/search/reindex', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        items: props.historyItems.map((i) => ({
          task_id: i.id,
          timestamp: Math.floor(i.timestamp / 1000),
          camera_id: i.cameraId,
          stream_name: i.cameraName || i.cameraId,
          thumbnail: i.thumbnail || '',
          result: i.result || '',
          node_id: i.nodeId || '',
          processing_time: i.processingTime || 0,
          resolution: i.resolution || '',
        })),
      }),
    })

    const resp = await fetch('/api/vlm/search/hybrid', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text_query: kw,
        camera_id: cameraFilter.value,
        limit: 100,
      }),
    })

    if (!resp.ok) {
      throw new Error(`HTTP ${resp.status}`)
    }
    const data = await resp.json()
    const items = (data.items || []) as any[]
    searchedResults.value = items.map((it) => ({
      id: it.task_id,
      timestamp: new Date(it.timestamp).getTime(),
      cameraId: it.camera_id,
      cameraName: it.stream_name || it.camera_id,
      result: it.result,
      thumbnail: it.thumbnail,
      nodeId: it.node_id,
      processingTime: it.processing_time,
      resolution: it.resolution,
    }))

    if (searchedResults.value.length === 0) {
      statusHintType.value = 'info'
      statusHint.value = '语义检索已执行，但没有匹配结果。可尝试更换关键词或放宽条件。'
    } else {
      statusHintType.value = 'ok'
      statusHint.value = `检索完成：命中 ${searchedResults.value.length} 条结果。`
    }
  } catch (e) {
    console.error('[VideoSearchTab] semantic search failed, fallback to local filtering:', e)
    // 仅当本地结果非空时才回退，避免清空已有结果
    const fallback = localResults.value
    if (fallback.length > 0) {
      searchedResults.value = fallback
      statusHintType.value = 'warn'
      statusHint.value = `语义索引服务不可用，已回退到本地文本匹配（${fallback.length} 条）。`
    } else {
      statusHintType.value = 'warn'
      statusHint.value = '语义索引服务不可用，本地匹配也无结果。'
    }
  } finally {
    searching.value = false
  }
}

const localResults = computed(() => {
  const kw = normalize(submittedQuery.value)
  // 修复: 未执行搜索时不显示任何记录
  if (!kw) return []
  return props.historyItems
    .filter((item) => {
      if (cameraFilter.value !== 'all' && item.cameraId !== cameraFilter.value) return false
      return normalize(item.result || '').includes(kw) || normalize(item.cameraId).includes(kw)
    })
    .slice(0, 100)
})

const results = computed(() => searchedResults.value ?? localResults.value)

const emptyText = computed(() => {
  if (!submittedQuery.value.trim()) {
    if (props.availableCameras.length === 0) {
      return '暂无可用摄像头。请先在“摄像头管理”中确认流是否在线。'
    }
    if (props.historyItems.length === 0) {
      return '暂无可检索数据：当前未执行VLM分析，或未接入可用算力节点。'
    }
    return '请输入搜索描述后执行检索。'
  }
  return '暂无匹配结果'
})

function formatTs(ts: number) {
  const d = new Date(ts)
  return `${d.toLocaleDateString()} ${d.toLocaleTimeString()}`
}
</script>

<style scoped>
.video-search-tab {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-secondary, #1e1e2e);
  border: 1px solid var(--border-color, #333);
  border-radius: 10px;
  padding: 10px;
}

.search-toolbar {
  display: flex;
  gap: 8px;
}

.search-input {
  flex: 1;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--border-color, #444);
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
  padding: 0 10px;
}

.filter-select {
  width: 180px;
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--border-color, #444);
  background: var(--bg-tertiary, #252535);
  color: var(--text-primary, #e0e0e0);
}

.search-btn {
  height: 32px;
  border-radius: 8px;
  border: 1px solid var(--accent-primary, #10a37f);
  background: var(--accent-primary, #10a37f);
  color: #fff;
  padding: 0 12px;
  cursor: pointer;
}

.search-tip {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-secondary, #888);
}

.status-hint {
  margin-top: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 12px;
  border: 1px solid #3a3a4d;
  color: #bfc7d5;
  background: #222636;
}

.status-hint.warn {
  border-color: #8b6b2a;
  background: #2f291d;
  color: #f3d08a;
}

.status-hint.ok {
  border-color: #2e7d4f;
  background: #1f2f28;
  color: #9de3bd;
}

.result-list {
  margin-top: 10px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.result-item {
  display: flex;
  gap: 10px;
  background: var(--bg-tertiary, #252535);
  border: 1px solid var(--border-color, #333);
  border-radius: 8px;
  padding: 8px;
  cursor: pointer;
}

.result-item:hover {
  border-color: var(--accent-primary, #10a37f);
}

.thumb {
  width: 120px;
  height: 68px;
  object-fit: cover;
  border-radius: 6px;
  background: #111;
}

.thumb.empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #777;
  font-size: 12px;
}

.meta {
  min-width: 0;
  flex: 1;
}

.line1 {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--text-secondary, #aaa);
}

.line2 {
  margin-top: 6px;
  color: var(--text-primary, #e0e0e0);
  font-size: 13px;
  line-height: 1.4;
}

.empty-state {
  text-align: center;
  color: var(--text-secondary, #888);
  padding: 24px 0;
}

.search-mode-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 10px;
}

.mode-tab {
  padding: 6px 16px;
  border: 1px solid var(--border-color, #444);
  border-radius: 6px;
  background: var(--bg-secondary, #1a1a2e);
  color: var(--text-secondary, #aaa);
  cursor: pointer;
  font-size: 13px;
}

.mode-tab.active {
  background: var(--accent-primary, #10a37f);
  color: #fff;
  border-color: var(--accent-primary, #10a37f);
}

.search-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

.image-upload-row {
  flex-wrap: wrap;
}

.upload-btn {
  padding: 8px 16px;
  border: 1px dashed var(--border-color, #444);
  border-radius: 8px;
  background: var(--bg-secondary, #1a1a2e);
  color: var(--text-primary, #e0e0e0);
  cursor: pointer;
}

.upload-btn:hover {
  border-color: var(--accent-primary, #10a37f);
}

.selected-file {
  font-size: 12px;
  color: var(--text-secondary, #aaa);
}

.image-preview {
  width: 80px;
  height: 60px;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid var(--border-color, #444);
}
</style>
