<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const rules = ref<any[]>([])
const loading = ref(false)

async function fetchRules() {
  loading.value = true
  try {
    const res = await axios.get('/api/v1/rules')
    rules.value = res.data.rules || []
  } catch (e) {
    console.error('获取规则失败', e)
  } finally {
    loading.value = false
  }
}

onMounted(fetchRules)

async function toggleRule(rule: any) {
  try {
    await axios.put(`/api/v1/rules/${rule.id}`, {
      ...rule,
      enabled: !rule.enabled
    })
    rule.enabled = !rule.enabled
  } catch (e) {
    console.error('更新规则失败', e)
  }
}
</script>

<template>
  <div class="rules-page">
    <div class="page-header">
      <h2>规则配置</h2>
      <button class="btn btn-primary">➕ 新建规则</button>
    </div>

    <div class="card">
      <div v-if="loading" class="loading"><div class="spinner"></div></div>
      <table v-else class="table">
        <thead>
          <tr>
            <th>规则名称</th>
            <th>描述</th>
            <th>触发条件</th>
            <th>优先级</th>
            <th>状态</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="rule in rules" :key="rule.id">
            <td><strong>{{ rule.name }}</strong></td>
            <td>{{ rule.description || '-' }}</td>
            <td><code>{{ rule.trigger?.type || '-' }}</code></td>
            <td>{{ rule.priority || 0 }}</td>
            <td>
              <label class="switch">
                <input type="checkbox" :checked="rule.enabled" @change="toggleRule(rule)">
                <span class="slider"></span>
              </label>
            </td>
            <td>
              <button class="btn btn-sm">编辑</button>
              <button class="btn btn-sm btn-danger">删除</button>
            </td>
          </tr>
          <tr v-if="rules.length === 0">
            <td colspan="6" class="empty-cell">暂无规则，点击上方"新建规则"创建</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.rules-page { display: flex; flex-direction: column; gap: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; }
.page-header h2 { font-size: 20px; color: #1a1a2e; }
code { background: #f3f4f6; padding: 2px 6px; border-radius: 4px; font-size: 12px; }
.btn-sm { padding: 4px 12px; font-size: 12px; margin-right: 4px; }
.empty-cell { text-align: center; padding: 40px !important; color: #9ca3af; }
.switch { position: relative; display: inline-block; width: 44px; height: 24px; }
.switch input { opacity: 0; width: 0; height: 0; }
.slider { position: absolute; cursor: pointer; inset: 0; background: #ccc; border-radius: 24px; transition: 0.3s; }
.slider:before { position: absolute; content: ""; height: 18px; width: 18px; left: 3px; bottom: 3px; background: white; border-radius: 50%; transition: 0.3s; }
input:checked + .slider { background: #3b82f6; }
input:checked + .slider:before { transform: translateX(20px); }
</style>
