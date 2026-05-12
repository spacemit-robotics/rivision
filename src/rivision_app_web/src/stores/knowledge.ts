import { defineStore } from 'pinia'
import { ref } from 'vue'
import { knowledgeApi } from '../api/knowledge'

export const useKnowledgeStore = defineStore('knowledge', () => {
  const rules = ref<any[]>([])
  const targets = ref<any[]>([])
  const prompts = ref<any[]>([])
  const loading = ref(false)

  async function fetchRules() {
    const res = await knowledgeApi.listRules() as any
    rules.value = res.rules || []
  }

  async function fetchTargets() {
    const res = await knowledgeApi.listTargets() as any
    targets.value = res.targets || []
  }

  async function fetchPrompts() {
    const res = await knowledgeApi.listPrompts() as any
    prompts.value = res.prompts || []
  }

  async function fetchAll() {
    loading.value = true
    try {
      await Promise.all([fetchRules(), fetchTargets(), fetchPrompts()])
    } finally {
      loading.value = false
    }
  }

  return { rules, targets, prompts, loading, fetchRules, fetchTargets, fetchPrompts, fetchAll }
})
