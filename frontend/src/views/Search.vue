<template>
  <div class="search-page">
    <header class="page-header">
      <button class="btn btn-secondary btn-sm" @click="$router.push('/')">
        ← 返回
      </button>
      <h1>知识库搜索</h1>
    </header>
    
    <div class="search-box card">
      <div class="search-types">
        <button 
          v-for="type in searchTypes" 
          :key="type.value"
          class="type-btn"
          :class="{ active: currentType === type.value }"
          @click="currentType = type.value"
        >
          {{ type.label }}
        </button>
      </div>
      
      <div class="search-input-group">
        <input 
          v-model="query" 
          type="text" 
          class="search-input" 
          placeholder="输入搜索内容..."
          @keyup.enter="doSearch"
        />
        <button class="btn btn-primary" @click="doSearch" :disabled="searching">
          {{ searching ? '搜索中...' : '🔍 搜索' }}
        </button>
      </div>
    </div>
    
    <div v-if="searching" class="loading-container">
      <div class="spinner"></div>
      <p>搜索中...</p>
    </div>
    
    <div v-else-if="results.length === 0 && hasSearched" class="empty-state">
      <div class="icon">🔍</div>
      <div class="title">未找到相关结果</div>
      <div class="desc">尝试其他关键词</div>
    </div>
    
    <div v-else-if="results.length > 0" class="results">
      <div class="results-header">
        <span>找到 {{ results.length }} 个结果</span>
      </div>
      
      <div class="results-list">
        <div 
          v-for="(result, index) in results" 
          :key="index"
          class="result-item card"
        >
          <div class="result-book">{{ result.book_title }}</div>
          <p class="result-content">{{ result.content }}</p>
          <div class="result-meta">
            <span v-if="result.similarity" class="similarity">
              相似度: {{ (result.similarity * 100).toFixed(1) }}%
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { searchApi } from '../api/search'

const searchTypes = [
  { value: 'hybrid', label: '混合搜索' },
  { value: 'vector', label: '向量搜索' },
  { value: 'keyword', label: '关键词搜索' }
]

const currentType = ref('hybrid')
const query = ref('')
const searching = ref(false)
const hasSearched = ref(false)
const results = ref([])

async function doSearch() {
  if (!query.value.trim()) return
  
  searching.value = true
  hasSearched.value = true
  
  try {
    const request = { query: query.value.trim(), limit: 20 }
    const response = currentType.value === 'vector'
      ? await searchApi.vector(request)
      : currentType.value === 'keyword'
        ? await searchApi.keyword(request)
        : await searchApi.hybrid(request)
    results.value = response.results || []
  } catch (error) {
    console.error('Search failed:', error)
    results.value = []
  } finally {
    searching.value = false
  }
}
</script>

<style scoped>
.search-page {
  max-width: 800px;
  margin: 0 auto;
  padding: 24px;
}

.page-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 32px;
}

.page-header h1 {
  font-size: 24px;
  font-weight: 600;
}

.search-box {
  padding: 24px;
  margin-bottom: 24px;
}

.search-types {
  display: flex;
  gap: 8px;
  margin-bottom: 16px;
}

.type-btn {
  padding: 8px 16px;
  border-radius: 980px;
  background: var(--bg-tertiary);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: var(--transition);
}

.type-btn:hover {
  background: var(--card-hover);
}

.type-btn.active {
  background: var(--accent-soft);
  border-color: var(--accent);
  color: var(--accent);
}

.search-input-group {
  display: flex;
  gap: 12px;
}

.search-input {
  flex: 1;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--text);
  font-size: 15px;
}

.search-input:focus {
  outline: none;
  border-color: var(--accent);
}

.results-header {
  font-size: 14px;
  color: var(--text-secondary);
  margin-bottom: 16px;
}

.results-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.result-item {
  padding: 20px;
}

.result-book {
  font-size: 13px;
  font-weight: 600;
  color: var(--accent);
  margin-bottom: 8px;
}

.result-content {
  font-size: 14px;
  line-height: 1.6;
  margin-bottom: 12px;
}

.result-meta {
  font-size: 12px;
  color: var(--text-tertiary);
}

.similarity {
  padding: 2px 8px;
  background: var(--accent-soft);
  border-radius: 4px;
  color: var(--accent);
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px;
  gap: 16px;
  color: var(--text-secondary);
}
</style>
