<template>
  <div class="overview">
    <h2 class="page-title">项目概览</h2>
    
    <div class="overview-grid">
      <!-- 项目信息 -->
      <section class="info-card card">
        <h3>基本信息</h3>
        <dl class="info-list">
          <div class="info-item">
            <dt>标题</dt>
            <dd>{{ project.title }}</dd>
          </div>
          <div class="info-item">
            <dt>题材</dt>
            <dd>
              <span v-for="g in project.genre" :key="g" class="genre-tag">{{ g }}</span>
              <span v-if="!project.genre?.length" class="text-secondary">未设置</span>
            </dd>
          </div>
          <div class="info-item">
            <dt>状态</dt>
            <dd><span class="badge badge-gray">{{ project.status }}</span></dd>
          </div>
          <div class="info-item">
            <dt>创建时间</dt>
            <dd>{{ formatTime(project.created_at) }}</dd>
          </div>
          <div class="info-item">
            <dt>更新时间</dt>
            <dd>{{ formatTime(project.updated_at) }}</dd>
          </div>
        </dl>
      </section>
      
      <!-- 故事设定 -->
      <section class="premise-card card">
        <h3>故事前提</h3>
        <p v-if="project.premise" class="premise-text">{{ project.premise }}</p>
        <p v-else class="text-secondary">暂无故事前提</p>
      </section>
      
      <section class="world-card card">
        <h3>世界观设定</h3>
        <p v-if="project.world_setting" class="world-text">{{ project.world_setting }}</p>
        <p v-else class="text-secondary">暂无世界观设定</p>
      </section>
      
      <!-- 统计 -->
      <section class="stats-card card">
        <h3>创作统计</h3>
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-value">{{ chapters.length }}</div>
            <div class="stat-label">章节数</div>
          </div>
          <div class="stat-item">
            <div class="stat-value">{{ totalWords.toLocaleString() }}</div>
            <div class="stat-label">总字数</div>
          </div>
          <div class="stat-item">
            <div class="stat-value">{{ characters.length }}</div>
            <div class="stat-label">角色数</div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useProjectStore } from '../stores/project'
import { useChapterStore } from '../stores/chapter'
import { chapterApi, characterApi } from '../api'

const route = useRoute()
const projectStore = useProjectStore()
const chapterStore = useChapterStore()

const project = computed(() => projectStore.currentProject)
const chapters = computed(() => chapterStore.chapters)

const totalWords = computed(() => {
  return chapters.value.reduce((sum, ch) => {
    return sum + (ch.word_count || 0)
  }, 0)
})

const characters = computed(() => [])

const projectId = computed(() => route.params.id)

onMounted(async () => {
  try {
    await chapterStore.fetchChapters(projectId.value)
    // TODO: 获取角色列表
  } catch (error) {
    console.error('Failed to fetch overview data:', error)
  }
})

function formatTime(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(new Date(value))
}
</script>

<style scoped>
.overview {
  max-width: 900px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 24px;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}

.info-card,
.premise-card,
.world-card,
.stats-card {
  padding: 24px;
}

.info-card {
  grid-column: span 2;
}

.premise-card,
.world-card {
  grid-column: span 1;
}

.stats-card {
  grid-column: span 2;
}

h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 16px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.info-list {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.info-item dt {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-bottom: 4px;
}

.info-item dd {
  font-size: 14px;
}

.genre-tag {
  display: inline-block;
  padding: 2px 8px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 12px;
  margin-right: 6px;
}

.premise-text,
.world-text {
  font-size: 14px;
  line-height: 1.7;
  color: var(--text-secondary);
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 20px;
}

.stat-item {
  text-align: center;
  padding: 16px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md);
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--accent);
  margin-bottom: 4px;
}

.stat-label {
  font-size: 13px;
  color: var(--text-secondary);
}
</style>
