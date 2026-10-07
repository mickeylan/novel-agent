<template>
  <div class="project-new">
    <header class="page-header">
      <button class="btn btn-secondary btn-sm" @click="$router.back()">
        ← 返回
      </button>
      <h1>新建项目</h1>
    </header>
    
    <form @submit.prevent="handleCreate" class="project-form card">
      <div class="form-group">
        <label>项目标题 *</label>
        <input 
          v-model="form.title" 
          type="text" 
          class="input" 
          placeholder="输入小说标题"
          required
        />
      </div>
      
      <div class="form-group">
        <label>题材类型</label>
        <input 
          v-model="form.genreText" 
          type="text" 
          class="input" 
          placeholder="如：都市, 玄幻, 科幻（用逗号分隔）"
        />
      </div>
      
      <div class="form-group">
        <label>故事前提</label>
        <textarea 
          v-model="form.premise" 
          class="textarea" 
          placeholder="简要描述故事的核心设定和情节走向"
          rows="4"
        ></textarea>
      </div>
      
      <div class="form-group">
        <label>世界观设定</label>
        <textarea 
          v-model="form.world_setting" 
          class="textarea" 
          placeholder="描述故事的世界观、背景等"
          rows="3"
        ></textarea>
      </div>
      
      <div class="form-actions">
        <button type="button" class="btn btn-secondary" @click="$router.back()">
          取消
        </button>
        <button type="submit" class="btn btn-primary" :disabled="creating">
          {{ creating ? '创建中...' : '创建项目' }}
        </button>
      </div>
    </form>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useProjectStore } from '../stores/project'

const router = useRouter()
const projectStore = useProjectStore()

const creating = ref(false)

const form = ref({
  title: '',
  genreText: '',
  premise: '',
  world_setting: ''
})

async function handleCreate() {
  if (!form.value.title.trim()) return
  
  creating.value = true
  try {
    const genres = form.value.genreText
      .split(/[,，]/)
      .map(s => s.trim())
      .filter(Boolean)
    
    const project = await projectStore.createProject({
      title: form.value.title,
      genres,
      premise: form.value.premise || null,
      world_setting: form.value.world_setting || null
    })
    
    router.push(`/projects/${project.id}`)
  } catch (error) {
    console.error('Failed to create project:', error)
  } finally {
    creating.value = false
  }
}
</script>

<style scoped>
.project-new {
  max-width: 640px;
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

.project-form {
  padding: 32px;
}

.form-group {
  margin-bottom: 24px;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--text-secondary);
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
</style>
