<template>
  <div class="chapters">
    <header class="page-header">
      <h2 class="page-title">章节管理</h2>
      <button class="btn btn-primary btn-sm" @click="openCreateModal">
        + 新建章节
      </button>
    </header>
    
    <div v-if="chapterStore.loading" class="loading-container">
      <div class="spinner"></div>
    </div>
    
    <div v-else-if="chapters.length === 0" class="empty-state">
      <div class="icon">📖</div>
      <div class="title">暂无章节</div>
      <div class="desc">创建第一个章节开始创作</div>
      <button class="btn btn-primary" @click="openCreateModal">
        创建章节
      </button>
    </div>
    
    <div v-else class="chapters-list">
      <div 
        v-for="chapter in chapters" 
        :key="chapter.id"
        class="chapter-item card"
        @click="editChapter(chapter)"
      >
        <div class="chapter-num">{{ chapter.chapter_number }}</div>
        <div class="chapter-info">
          <div class="chapter-title">{{ chapter.title || `第${chapter.chapter_number}章` }}</div>
          <div class="chapter-meta">
            <span>{{ chapter.word_count || 0 }} 字</span>
            <span class="badge badge-gray">{{ chapter.status || 'draft' }}</span>
          </div>
        </div>
        <div class="chapter-actions">
          <button 
            class="btn btn-sm btn-secondary" 
            @click.stop="editChapter(chapter)"
          >
            编辑
          </button>

        </div>
      </div>
    </div>
    
    <!-- Create/Edit Modal -->
    <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
      <div class="modal card">
        <div class="modal-header">
          <h3>{{ editingChapter ? '编辑章节' : '新建章节' }}</h3>
          <button class="modal-close" @click="closeModal">×</button>
        </div>
        
        <form @submit.prevent="handleSave" class="modal-body">
          <div class="form-group">
            <label>章节序号 *</label>
            <input 
              v-model.number="form.chapter_number" 
              type="number" 
              class="input" 
              min="1"
              required
              :disabled="!!editingChapter"
            />
          </div>
          
          <div class="form-group">
            <label>章节标题</label>
            <input 
              v-model="form.title" 
              type="text" 
              class="input" 
              placeholder="章节标题"
            />
          </div>
          
          <div class="form-group">
            <label>章节大纲</label>
            <textarea 
              v-model="form.outline" 
              class="textarea" 
              placeholder="本章要讲述的内容..."
              rows="4"
            ></textarea>
          </div>
          
          <div v-if="editingChapter" class="form-group">
            <label>正文内容</label>
            <textarea 
              v-model="form.content" 
              class="textarea content-editor" 
              placeholder="章节正文..."
              rows="10"
            ></textarea>
          </div>
          
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="closeModal">
              取消
            </button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? '保存中...' : '保存' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useChapterStore } from '../stores/chapter'

const route = useRoute()
const chapterStore = useChapterStore()

const projectId = computed(() => route.params.id)
const chapters = computed(() => chapterStore.chapters)

const showModal = ref(false)
const editingChapter = ref(null)
const saving = ref(false)

const form = ref({
  chapter_number: 1,
  title: '',
  outline: '',
  content: ''
})

onMounted(() => {
  chapterStore.fetchChapters(projectId.value)
})

function openCreateModal() {
  editingChapter.value = null
  const nextNumber = chapters.value.reduce((max, chapter) => Math.max(max, chapter.chapter_number), 0) + 1
  form.value = { chapter_number: nextNumber, title: '', outline: '', content: '' }
  showModal.value = true
}

function editChapter(chapter) {
  editingChapter.value = chapter
  form.value = {
    chapter_number: chapter.chapter_number,
    title: chapter.title || '',
    outline: chapter.outline || '',
    content: chapter.content || ''
  }
  showModal.value = true
}

function closeModal() {
  showModal.value = false
  editingChapter.value = null
  form.value = { chapter_number: chapters.value.length + 1, title: '', outline: '', content: '' }
}

async function handleSave() {
  saving.value = true
  try {
    const data = {
      chapter_number: form.value.chapter_number,
      title: form.value.title || null,
      outline: form.value.outline || null,
      content: form.value.content || null
    }
    
    if (editingChapter.value) {
      await chapterStore.updateChapter(projectId.value, form.value.chapter_number, data)
    } else {
      await chapterStore.createChapter(projectId.value, data)
    }
    closeModal()
  } catch (error) {
    console.error('Failed to save chapter:', error)
  } finally {
    saving.value = false
  }
}

</script>

<style scoped>
.chapters {
  max-width: 800px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
}

.chapters-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.chapter-item {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 20px;
  cursor: pointer;
  transition: var(--transition);
}

.chapter-item:hover {
  background: var(--card-hover);
}

.chapter-num {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--accent-soft);
  color: var(--accent);
  border-radius: var(--radius-md);
  font-size: 18px;
  font-weight: 600;
}

.chapter-info {
  flex: 1;
}

.chapter-title {
  font-weight: 500;
  margin-bottom: 4px;
}

.chapter-meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
  color: var(--text-secondary);
}

.chapter-actions {
  display: flex;
  gap: 8px;
}

.loading-container {
  display: flex;
  justify-content: center;
  padding: 40px;
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
  max-width: 560px;
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

.content-editor {
  font-family: 'Georgia', serif;
  line-height: 1.8;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
</style>
