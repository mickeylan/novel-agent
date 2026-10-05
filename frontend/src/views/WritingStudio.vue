<template>
  <div class="writing-studio">
    <header class="page-header">
      <h2 class="page-title">AI 写作工作室</h2>
    </header>
    
    <div class="studio-layout">
      <!-- 左侧：章节选择 -->
      <aside class="chapter-selector card">
        <h3>选择章节</h3>
        <div class="chapter-list">
          <div 
            v-for="ch in chapters" 
            :key="ch.id"
            class="chapter-btn"
            :class="{ active: selectedChapter === ch.chapter_number }"
            @click="selectChapter(ch)"
          >
            <span class="ch-num">第{{ ch.chapter_number }}章</span>
            <span class="ch-title">{{ ch.title || '未命名' }}</span>
          </div>
        </div>
      </aside>
      
      <!-- 中间：编辑区 -->
      <main class="editor-area">
        <div v-if="!selectedChapter" class="empty-state">
          <div class="icon">✍️</div>
          <div class="title">选择章节开始写作</div>
          <div class="desc">从左侧选择或创建章节</div>
        </div>
        
        <div v-else class="editor-panel">
          <div class="editor-toolbar">
            <span class="toolbar-title">第{{ selectedChapter }}章</span>
            <div class="toolbar-actions">
              <button class="btn btn-sm btn-secondary" @click="saveContent">
                保存
              </button>
              <button 
                class="btn btn-sm btn-primary" 
                @click="showWriteModal = true"
                :disabled="isGenerating"
              >
                ✨ AI 写作
              </button>
              <button 
                class="btn btn-sm btn-secondary" 
                @click="showPolishModal = true"
                :disabled="isGenerating || !content"
              >
                🎨 AI 润色
              </button>
            </div>
          </div>
          
          <textarea 
            v-model="content" 
            class="content-editor"
            placeholder="在这里输入章节内容..."
          ></textarea>
          
          <div class="editor-status">
            <span>{{ content.length }} 字</span>
          </div>
        </div>
      </main>
      
      <!-- 右侧：生成面板 -->
      <aside v-if="isGenerating || generationResult" class="generation-panel card">
        <h3>{{ isGenerating ? '生成中...' : '生成结果' }}</h3>
        
        <div v-if="isGenerating" class="generating">
          <div class="spinner"></div>
          <p>正在生成内容，请稍候...</p>
        </div>
        
        <div v-else-if="generationResult" class="result">
          <pre class="result-content">{{ generationResult }}</pre>
          <div class="result-actions">
            <button class="btn btn-sm btn-primary" @click="applyResult">
              应用结果
            </button>
            <button class="btn btn-sm btn-secondary" @click="generationResult = null">
              关闭
            </button>
          </div>
        </div>
      </aside>
    </div>
    
    <!-- Write Modal -->
    <div v-if="showWriteModal" class="modal-overlay" @click.self="showWriteModal = false">
      <div class="modal card">
        <div class="modal-header">
          <h3>AI 写作</h3>
          <button class="modal-close" @click="showWriteModal = false">×</button>
        </div>
        
        <form @submit.prevent="startWrite" class="modal-body">
          <div class="form-group">
            <label>章节大纲</label>
            <textarea 
              v-model="writeConfig.outline" 
              class="textarea" 
              placeholder="描述本章要讲述的内容..."
              rows="4"
            ></textarea>
          </div>
          
          <div class="form-group">
            <label>目标字数</label>
            <input 
              v-model.number="writeConfig.targetWordCount" 
              type="number" 
              class="input" 
              min="1000"
              max="20000"
            />
          </div>
          
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showWriteModal = false">
              取消
            </button>
            <button type="submit" class="btn btn-primary">
              开始生成
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
import { creationApi } from '../api/creation'

const route = useRoute()
const chapterStore = useChapterStore()

const projectId = computed(() => route.params.id)
const chapters = computed(() => chapterStore.chapters)

const selectedChapter = ref(null)
const content = ref('')
const isGenerating = ref(false)
const generationResult = ref(null)
const showWriteModal = ref(false)
const showPolishModal = ref(false)

const writeConfig = ref({
  outline: '',
  targetWordCount: 5000
})

onMounted(() => {
  chapterStore.fetchChapters(projectId.value)
})

function selectChapter(ch) {
  selectedChapter.value = ch.chapter_number
  content.value = ch.content || ''
}

async function saveContent() {
  if (!selectedChapter.value) return
  
  await chapterStore.updateChapter(projectId.value, selectedChapter.value, {
    content: content.value
  })
}

async function startWrite() {
  if (!selectedChapter.value) return
  
  showWriteModal.value = false
  isGenerating.value = true
  generationResult.value = null
  
  try {
    const result = await creationApi.writeChapter(projectId.value, selectedChapter.value, {
      outline: writeConfig.value.outline,
      target_word_count: writeConfig.value.targetWordCount
    })
    
    generationResult.value = result.content
    content.value = result.content
  } catch (error) {
    console.error('Failed to write chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

function applyResult() {
  content.value = generationResult.value
  generationResult.value = null
  saveContent()
}
</script>

<style scoped>
.writing-studio {
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}

.page-header {
  margin-bottom: 16px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
}

.studio-layout {
  display: grid;
  grid-template-columns: 200px 1fr 300px;
  gap: 16px;
  flex: 1;
  min-height: 0;
}

.chapter-selector {
  padding: 16px;
  overflow-y: auto;
}

.chapter-selector h3 {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 12px;
  text-transform: uppercase;
}

.chapter-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chapter-btn {
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: var(--transition);
}

.chapter-btn:hover {
  background: var(--bg-tertiary);
}

.chapter-btn.active {
  background: var(--accent-soft);
}

.ch-num {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--accent);
  margin-bottom: 2px;
}

.ch-title {
  display: block;
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.editor-area {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.editor-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.editor-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  border: 1px solid var(--border);
  border-bottom: none;
}

.toolbar-title {
  font-weight: 600;
}

.toolbar-actions {
  display: flex;
  gap: 8px;
}

.content-editor {
  flex: 1;
  padding: 16px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  color: var(--text);
  font-size: 15px;
  line-height: 1.8;
  font-family: 'Georgia', serif;
  resize: none;
}

.content-editor:focus {
  outline: none;
}

.editor-status {
  padding: 8px 16px;
  font-size: 12px;
  color: var(--text-tertiary);
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-top: none;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
}

.generation-panel {
  padding: 16px;
  overflow-y: auto;
}

.generation-panel h3 {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 16px;
  text-transform: uppercase;
}

.generating {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 20px;
  color: var(--text-secondary);
}

.result-content {
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 400px;
  overflow-y: auto;
  padding: 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-sm);
  margin-bottom: 16px;
}

.result-actions {
  display: flex;
  gap: 8px;
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
  max-width: 480px;
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

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
</style>
