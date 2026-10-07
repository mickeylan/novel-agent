<template>
  <div class="characters">
    <header class="page-header">
      <h2 class="page-title">角色管理</h2>
      <button class="btn btn-primary btn-sm" @click="showModal = true">
        + 添加角色
      </button>
    </header>
    
    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
    </div>
    
    <div v-else-if="characters.length === 0" class="empty-state">
      <div class="icon">👤</div>
      <div class="title">暂无角色</div>
      <div class="desc">添加故事中的主要角色</div>
      <button class="btn btn-primary" @click="showModal = true">
        添加角色
      </button>
    </div>
    
    <div v-else class="characters-grid">
      <div 
        v-for="char in characters" 
        :key="char.id"
        class="character-card card"
      >
        <div class="char-header">
          <div class="char-avatar">{{ char.name?.charAt(0) || '?' }}</div>
          <div class="char-info">
            <div class="char-name">{{ char.name }}</div>
            <div class="char-traits">
              <span v-for="t in (char.traits || []).slice(0, 2)" :key="t" class="trait-tag">
                {{ t }}
              </span>
            </div>
          </div>
        </div>
        <p class="char-arc">{{ char.arc_summary || '暂无角色弧光' }}</p>
        <div class="char-footer">
          <span class="char-state">{{ char.current_state || '未设置当前状态' }}</span>
        </div>
      </div>
    </div>
    
    <!-- Create Modal -->
    <div v-if="showModal" class="modal-overlay" @click.self="closeModal">
      <div class="modal card">
        <div class="modal-header">
          <h3>添加角色</h3>
          <button class="modal-close" @click="closeModal">×</button>
        </div>
        
        <form @submit.prevent="handleCreate" class="modal-body">
          <div class="form-group">
            <label>角色名称 *</label>
            <input v-model="form.name" type="text" class="input" required />
          </div>
          
          <div class="form-group">
            <label>性格特点</label>
            <input 
              v-model="form.traitsText" 
              type="text" 
              class="input" 
              placeholder="如：勇敢, 聪明, 善良（用逗号分隔）"
            />
          </div>
          
          <div class="form-group">
            <label>角色弧光</label>
            <textarea 
              v-model="form.arc_summary" 
              class="textarea" 
              placeholder="描述角色的成长和变化..."
              rows="3"
            ></textarea>
          </div>
          
          <div class="form-group">
            <label>当前状态</label>
            <input 
              v-model="form.current_state" 
              type="text" 
              class="input" 
              placeholder="角色当前的状态或处境"
            />
          </div>
          
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="closeModal">
              取消
            </button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? '添加中...' : '添加' }}
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
import { characterApi } from '../api/character'

const route = useRoute()

const projectId = computed(() => route.params.id)

const characters = ref([])
const loading = ref(false)
const showModal = ref(false)
const saving = ref(false)

const form = ref({
  name: '',
  traitsText: '',
  arc_summary: '',
  current_state: ''
})

onMounted(() => {
  fetchCharacters()
})

async function fetchCharacters() {
  loading.value = true
  try {
    characters.value = await characterApi.list(projectId.value)
  } catch (error) {
    console.error('Failed to fetch characters:', error)
    characters.value = []
  } finally {
    loading.value = false
  }
}

function closeModal() {
  showModal.value = false
  form.value = { name: '', traitsText: '', arc_summary: '', current_state: '' }
}

async function handleCreate() {
  saving.value = true
  try {
    const traits = form.value.traitsText
      .split(/[,，]/)
      .map(s => s.trim())
      .filter(Boolean)
    
    await characterApi.create(projectId.value, {
      name: form.value.name,
      traits: traits,
      arc_summary: form.value.arc_summary || null,
      current_state: form.value.current_state || null,
      relationships: {}
    })
    
    closeModal()
    fetchCharacters()
  } catch (error) {
    console.error('Failed to create character:', error)
  } finally {
    saving.value = false
  }
}

</script>

<style scoped>
.characters {
  max-width: 900px;
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

.characters-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}

.character-card {
  padding: 20px;
}

.char-header {
  display: flex;
  gap: 12px;
  margin-bottom: 12px;
}

.char-avatar {
  width: 48px;
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, var(--accent), #5856d6);
  border-radius: 50%;
  font-size: 20px;
  font-weight: 600;
  color: #fff;
}

.char-name {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 4px;
}

.char-traits {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.trait-tag {
  padding: 2px 6px;
  background: var(--bg-tertiary);
  border-radius: 4px;
  font-size: 11px;
  color: var(--text-secondary);
}

.char-arc {
  font-size: 13px;
  color: var(--text-secondary);
  margin-bottom: 12px;
  line-height: 1.5;
}

.char-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.char-state {
  font-size: 12px;
  color: var(--text-tertiary);
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
