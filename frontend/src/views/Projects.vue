<template>
  <div class="projects-page">
    <!-- Hero Section -->
    <header class="hero">
      <div class="hero-content">
        <h1 class="hero-title">
          <span class="icon">✍️</span>
          Novel Agent
        </h1>
        <p class="hero-desc">AI 驱动的智能小说创作助手</p>
        
        <div class="hero-actions">
          <button class="btn btn-primary" @click="showCreateModal = true">
            <span>+</span> 新建项目
          </button>
          <button class="btn btn-secondary" @click="$router.push('/search')">
            🔍 知识库搜索
          </button>
        </div>
      </div>
      
      <div class="hero-status">
        <div class="status-item" :class="{ connected: appStore.connected }">
          <span class="status-dot"></span>
          {{ appStore.connected ? '后端已连接' : '后端未连接' }}
        </div>
      </div>
    </header>
    
    <!-- Stats -->
    <section class="stats">
      <div class="stat-card">
        <div class="stat-value">{{ projects.length }}</div>
        <div class="stat-label">项目总数</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ draftCount }}</div>
        <div class="stat-label">进行中</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{{ totalChapters }}</div>
        <div class="stat-label">章节数</div>
      </div>
    </section>
    
    <!-- Projects Grid -->
    <section class="projects-section">
      <h2 class="section-title">我的项目</h2>
      
      <div v-if="projectStore.loading" class="loading-container">
        <div class="spinner"></div>
        <p>加载中...</p>
      </div>
      
      <div v-else-if="projects.length === 0" class="empty-state">
        <div class="icon">📚</div>
        <div class="title">暂无项目</div>
        <div class="desc">创建你的第一个小说项目，开始 AI 创作之旅</div>
        <button class="btn btn-primary" @click="showCreateModal = true">
          创建项目
        </button>
      </div>
      
      <div v-else class="projects-grid">
        <article 
          v-for="project in projects" 
          :key="project.id"
          class="project-card card"
          @click="$router.push(`/projects/${project.id}`)"
        >
          <div class="project-head">
            <span class="project-icon">✍</span>
            <div class="project-info">
              <div class="project-title">{{ project.title }}</div>
              <div class="project-time">{{ formatTime(project.updated_at) }}</div>
            </div>
            <span class="badge badge-gray">{{ project.status || 'draft' }}</span>
          </div>
          
          <p class="project-synopsis">{{ project.premise || project.world_setting || '暂无描述' }}</p>
          
          <div class="project-genres" v-if="project.genres?.length">
            <span v-for="g in project.genres.slice(0, 3)" :key="g" class="genre-tag">
              {{ g }}
            </span>
          </div>
          
          <div class="project-foot">
            <span class="data-safety">数据保护已开启</span>
            <span class="go">→</span>
          </div>
        </article>
      </div>
    </section>
    
    <!-- Create Modal -->
    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal card">
        <div class="modal-header">
          <h3>新建项目</h3>
          <button class="modal-close" @click="showCreateModal = false">×</button>
        </div>
        
        <form @submit.prevent="handleCreate" class="modal-body">
          <div class="form-group">
            <label>项目标题 *</label>
            <input 
              v-model="createForm.title" 
              type="text" 
              class="input" 
              placeholder="输入小说标题"
              required
            />
          </div>
          
          <div class="form-group">
            <label>题材类型</label>
            <input 
              v-model="createForm.genreText" 
              type="text" 
              class="input" 
              placeholder="如：都市, 玄幻, 科幻（用逗号分隔）"
            />
          </div>
          
          <div class="form-group">
            <label>故事前提</label>
            <textarea 
              v-model="createForm.premise" 
              class="textarea" 
              placeholder="简要描述故事的核心设定和情节走向"
              rows="4"
            ></textarea>
          </div>
          
          <div class="form-group">
            <label>世界观设定</label>
            <textarea 
              v-model="createForm.world_setting" 
              class="textarea" 
              placeholder="描述故事的世界观、背景等"
              rows="3"
            ></textarea>
          </div>
          
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showCreateModal = false">
              取消
            </button>
            <button type="submit" class="btn btn-primary" :disabled="creating">
              {{ creating ? '创建中...' : '创建项目' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useProjectStore } from '../stores/project'
import { useAppStore } from '../stores/app'

const router = useRouter()
const projectStore = useProjectStore()
const appStore = useAppStore()

const showCreateModal = ref(false)
const creating = ref(false)

const createForm = ref({
  title: '',
  genreText: '',
  premise: '',
  world_setting: ''
})

const projects = computed(() => projectStore.projects)

const draftCount = computed(() => projects.value.filter(p => p.status === 'draft').length)

const totalChapters = computed(() => {
  // TODO: 从 API 获取实际章节数
  return 0
})

onMounted(() => {
  projectStore.fetchProjects()
})

function formatTime(value) {
  if (!value) return '刚刚'
  const date = new Date(value)
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

async function handleCreate() {
  if (!createForm.value.title.trim()) return
  
  creating.value = true
  try {
    const genres = createForm.value.genreText
      .split(/[,，]/)
      .map(s => s.trim())
      .filter(Boolean)
    
    const project = await projectStore.createProject({
      title: createForm.value.title,
      genres,
      premise: createForm.value.premise || null,
      world_setting: createForm.value.world_setting || null
    })
    
    showCreateModal.value = false
    createForm.value = { title: '', genreText: '', premise: '', world_setting: '' }
    router.push(`/projects/${project.id}`)
  } catch (error) {
    console.error('Failed to create project:', error)
  } finally {
    creating.value = false
  }
}

</script>

<style scoped>
.projects-page {
  min-height: 100vh;
  padding: 0 24px 60px;
}

/* Hero */
.hero {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  padding: 60px 0 40px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 40px;
}

.hero-content {
  max-width: 600px;
}

.hero-title {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 36px;
  font-weight: 700;
  margin-bottom: 12px;
}

.hero-title .icon {
  font-size: 40px;
}

.hero-desc {
  color: var(--text-secondary);
  font-size: 18px;
  margin-bottom: 24px;
}

.hero-actions {
  display: flex;
  gap: 12px;
}

.hero-status {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 8px;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--red);
}

.status-item.connected .status-dot {
  background: var(--green);
}

/* Stats */
.stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px;
  margin-bottom: 48px;
}

.stat-card {
  background: var(--card);
  border: 1px solid var(--card-border);
  border-radius: var(--radius-lg);
  padding: 24px;
  text-align: center;
}

.stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--accent);
  margin-bottom: 4px;
}

.stat-label {
  color: var(--text-secondary);
  font-size: 14px;
}

/* Section */
.section-title {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 24px;
}

/* Projects Grid */
.projects-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 20px;
}

.project-card {
  padding: 22px;
  cursor: pointer;
  transition: transform 0.25s, box-shadow 0.25s;
}

.project-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--shadow-hover);
}

.project-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 12px;
}

.project-icon {
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-soft);
  border-radius: var(--radius-md);
  font-size: 20px;
}

.project-info {
  flex: 1;
  min-width: 0;
}

.project-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.project-time {
  font-size: 12px;
  color: var(--text-tertiary);
}

.project-synopsis {
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.5;
  margin-bottom: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.project-genres {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 16px;
}

.genre-tag {
  padding: 3px 8px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 11px;
  color: var(--text-secondary);
}

.project-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.go {
  font-size: 20px;
  color: var(--text-tertiary);
  transition: transform 0.2s, color 0.2s;
}

.project-card:hover .go {
  transform: translateX(4px);
  color: var(--accent);
}

/* Loading */
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px;
  gap: 16px;
  color: var(--text-secondary);
}

/* Modal */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  width: 100%;
  max-width: 520px;
  max-height: 90vh;
  overflow-y: auto;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.modal-close {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  font-size: 24px;
  color: var(--text-secondary);
  cursor: pointer;
  border-radius: 50%;
  transition: var(--transition);
}

.modal-close:hover {
  background: var(--bg-tertiary);
  color: var(--text);
}

.modal-body {
  padding: 24px;
}

.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--text-secondary);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}
</style>
