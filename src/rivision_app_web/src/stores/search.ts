import { defineStore } from 'pinia'
import { ref } from 'vue'
import { searchApi } from '../api/search'

export const useSearchStore = defineStore('search', () => {
  const results = ref<any[]>([])
  const loading = ref(false)
  const query = ref('')
  const mode = ref<'text' | 'image' | 'hybrid' | 'video'>('text')
  const history = ref<string[]>([])

  async function searchText(q: string) {
    query.value = q
    mode.value = 'text'
    loading.value = true
    try {
      const res = await searchApi.text(q) as any
      results.value = res.results || []
      addHistory(q)
    } finally {
      loading.value = false
    }
  }

  async function searchImage(imageData: string) {
    mode.value = 'image'
    loading.value = true
    try {
      const res = await searchApi.image(imageData) as any
      results.value = res.results || []
    } finally {
      loading.value = false
    }
  }

  function addHistory(q: string) {
    if (!history.value.includes(q)) {
      history.value.unshift(q)
      if (history.value.length > 20) history.value.pop()
    }
  }

  function clearResults() {
    results.value = []
    query.value = ''
  }

  return { results, loading, query, mode, history, searchText, searchImage, clearResults }
})
