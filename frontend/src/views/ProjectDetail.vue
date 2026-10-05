<template>
  <div class="project-detail">
    <!-- Header -->
    <header class="detail-header">
      <div class="header-left">
        <button class="btn btn-secondary btn-sm" @click="$router.push('/')">
          ← 返回
        </button>
        <div class="project-title">
          <h1>{{ project?.title || '加载中...' }}</h1>
          <span v-if="project?.genre?.length" class="genre-tags">
            <span v-for="g in project.genre" :key="g" class="genre-tag">{{ g }}</span>
          </span>
        </div>
      </div>
      <div class="header-right">
        <span class="badge" :class="statusBadgeClass">
          {{ project?.status || 'draft' }}
        </span>
      </div>
    </header>
    
    <!-- Layout -->
    <div class="detail-layout">
      <!-- Sidebar -->
      <aside class="detail-sidebar">
        <nav class="sidebar-nav">
          <router-link 
            v-for="item in navItems" 
            :key="item.path"
            :to="`/projects/${projectId}/${item.path}`"
            class="nav-item"
            :class="{ active: $route.name === item.name }"
          >
            <span class="nav-icon">{{ item.icon }}</span>
            <span class="nav-label">{{ item.label }}</span>
          </router-link>
        </nav>
      </aside>
      
      <!-- Content -->
      <main class="detail-content">
        <router-view v-if="project" />
        <div v-else class="loading-container">
          <div class="spinner"></div>
          <p>加载项目...</p>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useProjectStore } from '../stores/project'

const route = useRoute()
const projectStore = useProjectStore()

const projectId = computed(() => route.params.id)
const project = computed(() => projectStore.currentProject)

const navItems = [
  { name: 'overview', path: 'overview', label: '概览', icon: '📊' },
  { name: 'chapters', path: 'chapters', label: '章节', icon: '📖' },
  { name: 'characters', path: 'characters', label: '角色', icon: '👤' },
  { name: 'outline', path: 'outline', label: '大纲', icon: '📝' },
  { name: 'writing', path: 'writing', label: '写作', icon: '✍' }
]

const statusBadgeClass = computed(() => {
  const status = project.value?.status
  if (status === 'completed') return 'badge-green'
  if (status === 'writing') return 'badge-blue'
  return 'badge-gray'
})

onMounted(() => {
  projectStore.fetchProject(projectId.value)
})

watch(projectId, (newId) => {
  if (newId) {
    projectStore.fetchProject(newId)
  }
})
</script>

<style scoped>
.project-detail {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 24px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.project-title h1 {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 4px;
}

.genre-tags {
  display: flex;
  gap: 6px;
}

.genre-tag {
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 11px;
  color: var(--text-secondary);
}

.detail-layout {
  display: flex;
  flex: 1;
}

.detail-sidebar {
  width: 200px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border);
  padding: 16px 0;
}

.sidebar-nav {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 20px;
  color: var(--text-secondary);
  text-decoration: none;
  transition: var(--transition);
}

.nav-item:hover {
  background: var(--bg-tertiary);
  color: var(--text);
}

.nav-item.active {
  background: var(--accent-soft);
  color: var(--accent);
}

.nav-icon {
  font-size: 16px;
}

.nav-label {
  font-size: 14px;
  font-weight: 500;
}

.detail-content {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}

.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 16px;
  color: var(--text-secondary);
}
</style>
