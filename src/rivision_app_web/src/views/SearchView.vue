<template>
  <div class="search-view">
    <!-- 搜索头部 -->
    <div class="search-header">
      <h1>智能搜索</h1>
      <p class="subtitle">自然语言搜索 · 以图搜图 · 混合检索</p>
    </div>

    <!-- 搜索模式切换 -->
    <div class="search-modes">
      <button 
        v-for="mode in searchModes" 
        :key="mode.value"
        :class="['mode-btn', { active: currentMode === mode.value }]"
        @click="currentMode = mode.value"
      >
        <component :is="mode.icon" class="icon" />
        {{ mode.label }}
      </button>
    </div>

    <!-- 搜索输入区 -->
    <div class="search-input-area">
      <!-- 文本搜索 -->
      <div v-if="currentMode === 'text' || currentMode === 'hybrid'" class="text-input">
        <input 
          v-model="textQuery"
          type="text" 
          placeholder="输入搜索内容，如：红色衣服的人、正在打电话..."
          @keyup.enter="handleSearch"
        />
      </div>

      <!-- 图像上传 -->
      <div v-if="currentMode === 'image' || currentMode === 'hybrid'" class="image-input">
        <div 
          class="upload-area"
          :class="{ 'has-image': imagePreview }"
          @click="triggerUpload"
          @drop.prevent="handleDrop"
          @dragover.prevent
        >
          <img v-if="imagePreview" :src="imagePreview" alt="preview" />
          <div v-else class="upload-placeholder">
            <svg class="upload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="17 8 12 3 7 8" />
              <line x1="12" y1="3" x2="12" y2="15" />
            </svg>
            <span>点击或拖拽上传图片</span>
          </div>
        </div>
        <input 
          ref="fileInput"
          type="file" 
          accept="image/*" 
          hidden 
          @change="handleFileChange"
        />
      </div>

      <!-- 搜索按钮 -->
      <button class="search-btn" @click="handleSearch" :disabled="isSearching">
        <span v-if="isSearching" class="loading-spinner"></span>
        <span v-else>搜索</span>
      </button>
    </div>

    <!-- 高级选项 -->
    <div class="advanced-options" v-if="showAdvanced">
      <div class="option-group">
        <label>时间范围</label>
        <select v-model="timeRange">
          <option value="">全部时间</option>
          <option value="1h">最近1小时</option>
          <option value="24h">最近24小时</option>
          <option value="7d">最近7天</option>
          <option value="30d">最近30天</option>
        </select>
      </div>
      <div class="option-group">
        <label>摄像头</label>
        <select v-model="selectedCamera">
          <option value="">全部摄像头</option>
          <option v-for="cam in cameras" :key="cam.id" :value="cam.id">
            {{ cam.name }}
          </option>
        </select>
      </div>
      <div class="option-group" v-if="currentMode === 'hybrid'">
        <label>文本权重: {{ textWeight }}</label>
        <input type="range" v-model="textWeight" min="0" max="1" step="0.1" />
      </div>
    </div>
    <button class="toggle-advanced" @click="showAdvanced = !showAdvanced">
      {{ showAdvanced ? '收起高级选项' : '展开高级选项' }}
    </button>

    <!-- 搜索结果 -->
    <div class="search-results" v-if="results.length > 0">
      <div class="results-header">
        <span>找到 {{ totalResults }} 个结果</span>
        <span class="search-time">耗时 {{ searchTime }}ms</span>
      </div>

      <div class="results-grid">
        <div 
          v-for="item in results" 
          :key="item.id"
          class="result-card"
          @click="openDetail(item)"
        >
          <div class="thumbnail">
            <img :src="item.thumbnail_url" :alt="item.description" />
            <div class="score-badge">{{ (item.score * 100).toFixed(0) }}%</div>
          </div>
          <div class="info">
            <p class="description">{{ item.description }}</p>
            <div class="meta">
              <span class="camera">{{ item.camera_name || item.camera_id }}</span>
              <span class="time">{{ formatTime(item.timestamp) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- 分页 -->
      <div class="pagination" v-if="totalResults > pageSize">
        <button @click="prevPage" :disabled="currentPage === 1">上一页</button>
        <span>{{ currentPage }} / {{ totalPages }}</span>
        <button @click="nextPage" :disabled="currentPage === totalPages">下一页</button>
      </div>
    </div>

    <!-- 空结果 -->
    <div class="no-results" v-else-if="hasSearched && !isSearching">
      <svg viewBox="0 0 24 24" class="empty-icon">
        <circle cx="11" cy="11" r="8" fill="none" stroke="currentColor" stroke-width="2"/>
        <line x1="21" y1="21" x2="16.65" y2="16.65" stroke="currentColor" stroke-width="2"/>
      </svg>
      <p>未找到相关结果</p>
      <p class="hint">尝试使用不同的关键词或调整搜索条件</p>
    </div>

    <!-- 详情弹窗 -->
    <div class="modal" v-if="selectedItem" @click.self="selectedItem = null">
      <div class="modal-content">
        <button class="close-btn" @click="selectedItem = null">&times;</button>
        <img :src="selectedItem.thumbnail_url" alt="detail" />
        <div class="detail-info">
          <h3>{{ selectedItem.description }}</h3>
          <p><strong>摄像头:</strong> {{ selectedItem.camera_name || selectedItem.camera_id }}</p>
          <p><strong>时间:</strong> {{ formatTime(selectedItem.timestamp) }}</p>
          <p><strong>相似度:</strong> {{ (selectedItem.score * 100).toFixed(1) }}%</p>
          <p v-if="selectedItem.event_type"><strong>事件类型:</strong> {{ selectedItem.event_type }}</p>
        </div>
        <div class="detail-actions">
          <button @click="playVideo(selectedItem)">播放录像</button>
          <button @click="downloadClip(selectedItem)">下载片段</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'

// 搜索模式
const searchModes = [
  { value: 'text', label: '文字搜索', icon: 'SearchIcon' },
  { value: 'image', label: '以图搜图', icon: 'ImageIcon' },
  { value: 'hybrid', label: '混合搜索', icon: 'MixIcon' },
]

const currentMode = ref('text')
const textQuery = ref('')
const imageBase64 = ref('')
const imagePreview = ref('')
const isSearching = ref(false)
const hasSearched = ref(false)
const showAdvanced = ref(false)

// 高级选项
const timeRange = ref('')
const selectedCamera = ref('')
const textWeight = ref(0.6)

// 结果
const results = ref([])
const totalResults = ref(0)
const searchTime = ref(0)
const currentPage = ref(1)
const pageSize = 20
const selectedItem = ref(null)

// 摄像头列表
const cameras = ref([])

const totalPages = computed(() => Math.ceil(totalResults.value / pageSize))

// 文件上传
const fileInput = ref(null)

const triggerUpload = () => {
  fileInput.value?.click()
}

const handleFileChange = (e) => {
  const file = e.target.files[0]
  if (file) {
    processImage(file)
  }
}

const handleDrop = (e) => {
  const file = e.dataTransfer.files[0]
  if (file && file.type.startsWith('image/')) {
    processImage(file)
  }
}

const processImage = (file) => {
  const reader = new FileReader()
  reader.onload = (e) => {
    imagePreview.value = e.target.result
    imageBase64.value = e.target.result.split(',')[1]
  }
  reader.readAsDataURL(file)
}

// 搜索
const handleSearch = async () => {
  if (currentMode.value === 'text' && !textQuery.value.trim()) return
  if (currentMode.value === 'image' && !imageBase64.value) return

  isSearching.value = true
  hasSearched.value = true

  try {
    const endpoint = `/api/v1/search/${currentMode.value}`
    const body = buildSearchBody()

    const response = await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body)
    })

    const data = await response.json()
    results.value = data.results || []
    totalResults.value = data.total || 0
    searchTime.value = data.stats?.search_time_ms || 0
    currentPage.value = 1
  } catch (err) {
    console.error('Search failed:', err)
  } finally {
    isSearching.value = false
  }
}

const buildSearchBody = () => {
  const body = {
    limit: pageSize,
    offset: (currentPage.value - 1) * pageSize
  }

  if (selectedCamera.value) {
    body.cameras = [selectedCamera.value]
  }

  if (timeRange.value) {
    const now = new Date()
    const ranges = {
      '1h': 60 * 60 * 1000,
      '24h': 24 * 60 * 60 * 1000,
      '7d': 7 * 24 * 60 * 60 * 1000,
      '30d': 30 * 24 * 60 * 60 * 1000
    }
    body.time_range = {
      start: new Date(now - ranges[timeRange.value]).toISOString(),
      end: now.toISOString()
    }
  }

  switch (currentMode.value) {
    case 'text':
      body.query = textQuery.value
      break
    case 'image':
      body.image = imageBase64.value
      break
    case 'hybrid':
      body.text_query = textQuery.value
      body.image = imageBase64.value
      body.text_weight = parseFloat(textWeight.value)
      body.image_weight = 1 - parseFloat(textWeight.value)
      break
  }

  return body
}

// 分页
const prevPage = () => {
  if (currentPage.value > 1) {
    currentPage.value--
    handleSearch()
  }
}

const nextPage = () => {
  if (currentPage.value < totalPages.value) {
    currentPage.value++
    handleSearch()
  }
}

// 详情
const openDetail = (item) => {
  selectedItem.value = item
}

const playVideo = (item) => {
  // 跳转到录像回放
  window.open(`/playback?camera=${item.camera_id}&time=${item.timestamp}`, '_blank')
}

const downloadClip = (item) => {
  // 下载视频片段
  window.open(`/api/v1/clips/${item.id}/download`, '_blank')
}

// 格式化时间
const formatTime = (timestamp) => {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return date.toLocaleString('zh-CN')
}

// 加载摄像头列表
onMounted(async () => {
  try {
    const res = await fetch('/api/v1/cameras')
    const data = await res.json()
    cameras.value = data.cameras || []
  } catch (err) {
    console.error('Failed to load cameras:', err)
  }
})
</script>

<style scoped>
.search-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 24px;
}

.search-header {
  text-align: center;
  margin-bottom: 32px;
}

.search-header h1 {
  font-size: 2rem;
  font-weight: 600;
  color: #1f2937;
  margin-bottom: 8px;
}

.subtitle {
  color: #6b7280;
  font-size: 0.95rem;
}

.search-modes {
  display: flex;
  justify-content: center;
  gap: 12px;
  margin-bottom: 24px;
}

.mode-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 20px;
  border: 2px solid #e5e7eb;
  border-radius: 8px;
  background: white;
  color: #374151;
  font-size: 0.95rem;
  cursor: pointer;
  transition: all 0.2s;
}

.mode-btn:hover {
  border-color: #3b82f6;
  color: #3b82f6;
}

.mode-btn.active {
  background: #3b82f6;
  border-color: #3b82f6;
  color: white;
}

.search-input-area {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 16px;
}

.text-input input {
  width: 100%;
  padding: 14px 20px;
  border: 2px solid #e5e7eb;
  border-radius: 12px;
  font-size: 1rem;
  outline: none;
  transition: border-color 0.2s;
}

.text-input input:focus {
  border-color: #3b82f6;
}

.upload-area {
  border: 2px dashed #d1d5db;
  border-radius: 12px;
  padding: 40px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
  background: #f9fafb;
}

.upload-area:hover {
  border-color: #3b82f6;
  background: #eff6ff;
}

.upload-area.has-image {
  padding: 8px;
}

.upload-area img {
  max-width: 100%;
  max-height: 200px;
  border-radius: 8px;
}

.upload-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: #6b7280;
}

.upload-icon {
  width: 48px;
  height: 48px;
  stroke-width: 1.5;
}

.search-btn {
  padding: 14px 32px;
  background: linear-gradient(135deg, #3b82f6, #2563eb);
  color: white;
  border: none;
  border-radius: 12px;
  font-size: 1rem;
  font-weight: 500;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}

.search-btn:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.4);
}

.search-btn:disabled {
  opacity: 0.7;
  cursor: not-allowed;
}

.loading-spinner {
  display: inline-block;
  width: 20px;
  height: 20px;
  border: 2px solid rgba(255,255,255,0.3);
  border-radius: 50%;
  border-top-color: white;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.advanced-options {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  padding: 16px;
  background: #f9fafb;
  border-radius: 12px;
  margin-bottom: 8px;
}

.option-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 150px;
}

.option-group label {
  font-size: 0.85rem;
  color: #6b7280;
}

.option-group select,
.option-group input[type="range"] {
  padding: 8px 12px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
}

.toggle-advanced {
  background: none;
  border: none;
  color: #3b82f6;
  cursor: pointer;
  font-size: 0.9rem;
  margin-bottom: 24px;
}

.search-results {
  margin-top: 32px;
}

.results-header {
  display: flex;
  justify-content: space-between;
  margin-bottom: 16px;
  color: #6b7280;
}

.results-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 20px;
}

.result-card {
  background: white;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
}

.result-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0,0,0,0.15);
}

.thumbnail {
  position: relative;
  aspect-ratio: 16/9;
  overflow: hidden;
}

.thumbnail img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.score-badge {
  position: absolute;
  top: 8px;
  right: 8px;
  background: rgba(59, 130, 246, 0.9);
  color: white;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 0.8rem;
  font-weight: 500;
}

.info {
  padding: 12px;
}

.description {
  font-size: 0.95rem;
  color: #1f2937;
  margin-bottom: 8px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.8rem;
  color: #9ca3af;
}

.pagination {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 16px;
  margin-top: 24px;
}

.pagination button {
  padding: 8px 16px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: white;
  cursor: pointer;
}

.pagination button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.no-results {
  text-align: center;
  padding: 60px 20px;
  color: #9ca3af;
}

.empty-icon {
  width: 64px;
  height: 64px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.hint {
  font-size: 0.9rem;
  margin-top: 8px;
}

.modal {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
}

.modal-content {
  background: white;
  border-radius: 16px;
  max-width: 600px;
  width: 90%;
  max-height: 90vh;
  overflow: auto;
  position: relative;
}

.modal-content img {
  width: 100%;
  border-radius: 16px 16px 0 0;
}

.close-btn {
  position: absolute;
  top: 12px;
  right: 12px;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 50%;
  background: rgba(0,0,0,0.5);
  color: white;
  font-size: 1.5rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.detail-info {
  padding: 20px;
}

.detail-info h3 {
  margin-bottom: 12px;
}

.detail-info p {
  margin: 8px 0;
  color: #4b5563;
}

.detail-actions {
  display: flex;
  gap: 12px;
  padding: 0 20px 20px;
}

.detail-actions button {
  flex: 1;
  padding: 12px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 500;
}

.detail-actions button:first-child {
  background: #3b82f6;
  color: white;
}

.detail-actions button:last-child {
  background: #f3f4f6;
  color: #374151;
}
</style>
