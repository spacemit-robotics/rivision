<script setup lang="ts">
import { ref, computed } from 'vue'
import axios from 'axios'

const query = ref('')
const searchType = ref<'text' | 'image' | 'hybrid'>('text')
const results = ref<any[]>([])
const loading = ref(false)

// 图片搜索
const imageFile = ref<File | null>(null)
const imagePreview = ref<string>('')
const imageInputRef = ref<HTMLInputElement | null>(null)

// 时间筛选
const timeRange = ref('all')
const timeRanges = [
  { value: 'all', label: '全部时间' },
  { value: '1h', label: '最近1小时' },
  { value: '24h', label: '最近24小时' },
  { value: '7d', label: '最近7天' },
  { value: '30d', label: '最近30天' },
]

// 摄像头筛选
const cameraFilter = ref('')
const cameras = ref<any[]>([])

// 分页
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

const hasMore = computed(() => results.value.length < total.value)

function handleImageSelect(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) {
    imageFile.value = file
    const reader = new FileReader()
    reader.onload = (ev) => {
      imagePreview.value = ev.target?.result as string
    }
    reader.readAsDataURL(file)
  }
}

function clearImage() {
  imageFile.value = null
  imagePreview.value = ''
  if (imageInputRef.value) imageInputRef.value.value = ''
}

async function handleSearch(append = false) {
  if (searchType.value === 'text' && !query.value.trim()) return
  if (searchType.value === 'image' && !imageFile.value) return
  
  loading.value = true
  if (!append) {
    page.value = 1
    results.value = []
  }
  
  try {
    let endpoint = '/api/v1/search/text'
    let payload: any = { limit: pageSize.value, offset: (page.value - 1) * pageSize.value }
    
    if (timeRange.value !== 'all') payload.time_range = timeRange.value
    if (cameraFilter.value) payload.camera_id = cameraFilter.value
    
    if (searchType.value === 'image' && imageFile.value) {
      endpoint = '/api/v1/search/image'
      const formData = new FormData()
      formData.append('image', imageFile.value)
      formData.append('limit', String(pageSize.value))
      if (timeRange.value !== 'all') formData.append('time_range', timeRange.value)
      if (cameraFilter.value) formData.append('camera_id', cameraFilter.value)
      
      const res = await axios.post(endpoint, formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      results.value = append ? [...results.value, ...(res.data.results || [])] : (res.data.results || [])
      total.value = res.data.total || results.value.length
    } else if (searchType.value === 'hybrid') {
      endpoint = '/api/v1/search/hybrid'
      payload.text_query = query.value
      if (imageFile.value) {
        const formData = new FormData()
        formData.append('image', imageFile.value)
        formData.append('text_query', query.value)
        formData.append('limit', String(pageSize.value))
        const res = await axios.post(endpoint, formData, {
          headers: { 'Content-Type': 'multipart/form-data' }
        })
        results.value = res.data.results || []
        total.value = res.data.total || results.value.length
      } else {
        const res = await axios.post('/api/v1/search/text', { query: query.value, ...payload })
        results.value = res.data.results || []
        total.value = res.data.total || results.value.length
      }
    } else {
      payload.query = query.value
      const res = await axios.post(endpoint, payload)
      results.value = append ? [...results.value, ...(res.data.results || [])] : (res.data.results || [])
      total.value = res.data.total || results.value.length
    }
  } catch (e) {
    console.error('搜索失败', e)
  } finally {
    loading.value = false
  }
}

function loadMore() {
  if (!hasMore.value || loading.value) return
  page.value++
  handleSearch(true)
}

function formatTime(ts: string) {
  if (!ts) return '-'
  return new Date(ts).toLocaleString('zh-CN')
}

function getScoreColor(score: number) {
  if (score >= 0.8) return '#10b981'
  if (score >= 0.6) return '#3b82f6'
  if (score >= 0.4) return '#f59e0b'
  return '#6b7280'
}
</script>

<template>
  <div class="search-page">
    <div class="page-header">
      <h2>智能搜索</h2>
      <p class="page-desc">支持文本、图片、混合多模态搜索</p>
    </div>

    <div class="card search-card">
      <!-- 搜索类型切换 -->
      <div class="search-type-tabs">
        <button 
          v-for="t in [{ value: 'text', label: '🔤 文本搜索' }, { value: 'image', label: '🖼️ 以图搜图' }, { value: 'hybrid', label: '🔀 混合搜索' }]"
          :key="t.value"
          :class="['tab-btn', { active: searchType === t.value }]"
          @click="searchType = t.value as 'text' | 'image' | 'hybrid'"
        >
          {{ t.label }}
        </button>
      </div>

      <!-- 筛选条件 -->
      <div class="search-filters">
        <select v-model="timeRange" class="filter-select">
          <option v-for="r in timeRanges" :key="r.value" :value="r.value">{{ r.label }}</option>
        </select>
        <select v-model="cameraFilter" class="filter-select">
          <option value="">全部摄像头</option>
          <option v-for="c in cameras" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
      </div>

      <!-- 搜索输入 -->
      <div class="search-form">
        <div v-if="searchType === 'text' || searchType === 'hybrid'" class="text-input-wrap">
          <input 
            v-model="query"
            type="text"
            class="input search-input"
            placeholder="输入搜索内容，如：穿红色衣服的人、停在门口的车辆"
            @keyup.enter="handleSearch(false)"
          />
        </div>
        
        <div v-if="searchType === 'image' || searchType === 'hybrid'" class="image-input-wrap">
          <input 
            ref="imageInputRef"
            type="file"
            accept="image/*"
            class="hidden-input"
            @change="handleImageSelect"
          />
          <div v-if="imagePreview" class="image-preview">
            <img :src="imagePreview" alt="preview" />
            <button class="clear-btn" @click="clearImage">✕</button>
          </div>
          <button v-else class="upload-btn" @click="imageInputRef?.click()">
            📤 上传图片
          </button>
        </div>

        <button class="btn btn-primary search-btn" @click="handleSearch(false)" :disabled="loading">
          {{ loading ? '搜索中...' : '🔍 搜索' }}
        </button>
      </div>
    </div>

    <!-- 搜索结果 -->
    <div class="card results-card" v-if="results.length > 0">
      <div class="results-header">
        <div class="card-title">搜索结果</div>
        <span class="results-count">共 {{ total }} 条</span>
      </div>
      <div class="results-grid">
        <div v-for="item in results" :key="item.id" class="result-item">
          <div class="result-thumbnail">
            <img v-if="item.thumbnail" :src="item.thumbnail" alt="thumbnail" />
            <span v-else>🖼️</span>
          </div>
          <div class="result-info">
            <div class="result-score" :style="{ color: getScoreColor(item.score) }">
              {{ (item.score * 100).toFixed(1) }}% 匹配
            </div>
            <div class="result-camera">📹 {{ item.camera_name || item.camera_id || '-' }}</div>
            <div class="result-desc">{{ item.description || '-' }}</div>
            <div class="result-time">🕐 {{ formatTime(item.timestamp) }}</div>
          </div>
        </div>
      </div>
      <div v-if="hasMore" class="load-more">
        <button class="btn btn-secondary" @click="loadMore" :disabled="loading">
          {{ loading ? '加载中...' : '加载更多' }}
        </button>
      </div>
    </div>

    <!-- 空状态 -->
    <div class="card" v-else-if="!loading">
      <div class="empty-state">
        <div class="empty-icon">🔍</div>
        <p>输入关键词或上传图片开始搜索</p>
        <p class="hint">支持自然语言描述，如"穿红色衣服的人"、"停在门口的车辆"等</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-page { display: flex; flex-direction: column; gap: 24px; }
.page-header h2 { font-size: 20px; color: #1a1a2e; margin: 0; }
.page-desc { font-size: 14px; color: #6b7280; margin-top: 4px; }

.search-card { padding: 20px; }

.search-type-tabs { display: flex; gap: 8px; margin-bottom: 16px; }
.tab-btn {
  padding: 10px 18px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
}
.tab-btn:hover { border-color: #3b82f6; }
.tab-btn.active { background: #3b82f6; color: #fff; border-color: #3b82f6; }

.search-filters { display: flex; gap: 12px; margin-bottom: 16px; }
.filter-select {
  padding: 10px 14px;
  border: 1px solid #d1d5db;
  border-radius: 6px;
  background: white;
  font-size: 14px;
}

.search-form { display: flex; gap: 12px; align-items: flex-start; flex-wrap: wrap; }
.text-input-wrap { flex: 1; min-width: 300px; }
.search-input { width: 100%; padding: 12px 16px; border: 1px solid #d1d5db; border-radius: 8px; font-size: 14px; }
.search-input:focus { outline: none; border-color: #3b82f6; box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1); }

.image-input-wrap { display: flex; align-items: center; }
.hidden-input { display: none; }
.upload-btn {
  padding: 12px 20px;
  border: 2px dashed #d1d5db;
  border-radius: 8px;
  background: #f9fafb;
  cursor: pointer;
  font-size: 14px;
  transition: all 0.2s;
}
.upload-btn:hover { border-color: #3b82f6; background: #eff6ff; }

.image-preview {
  position: relative;
  width: 80px;
  height: 80px;
  border-radius: 8px;
  overflow: hidden;
}
.image-preview img { width: 100%; height: 100%; object-fit: cover; }
.clear-btn {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 50%;
  background: rgba(0,0,0,0.6);
  color: #fff;
  font-size: 12px;
  cursor: pointer;
}

.search-btn { padding: 12px 24px; white-space: nowrap; }

.results-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.results-count { font-size: 14px; color: #6b7280; }

.results-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.result-item {
  background: #f9fafb;
  border-radius: 12px;
  overflow: hidden;
  transition: transform 0.2s, box-shadow 0.2s;
}
.result-item:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(0,0,0,0.1); }

.result-thumbnail {
  height: 160px;
  background: #e5e7eb;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 48px;
}
.result-thumbnail img { width: 100%; height: 100%; object-fit: cover; }

.result-info { padding: 12px 16px; }
.result-score { font-weight: 600; font-size: 15px; margin-bottom: 6px; }
.result-camera { font-size: 13px; color: #374151; margin-bottom: 4px; }
.result-desc { font-size: 13px; color: #6b7280; margin-bottom: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.result-time { font-size: 12px; color: #9ca3af; }

.load-more { text-align: center; margin-top: 20px; }
.btn-secondary { background: #f3f4f6; color: #374151; border: none; padding: 10px 24px; border-radius: 8px; cursor: pointer; }
.btn-secondary:hover { background: #e5e7eb; }

.empty-state { text-align: center; padding: 60px; color: #9ca3af; }
.empty-icon { font-size: 56px; margin-bottom: 16px; }
.hint { font-size: 13px; margin-top: 8px; }
</style>
