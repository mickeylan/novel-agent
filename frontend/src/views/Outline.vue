<template>
  <div class="outline">
    <header class="page-header">
      <h2 class="page-title">故事大纲</h2>
      <div class="header-actions">
        <button class="btn btn-secondary btn-sm" @click="loadOutline">
          刷新
        </button>
        <button class="btn btn-primary btn-sm" @click="generateOutline" :disabled="generating">
          {{ generating ? '生成中...' : '✨ AI 生成大纲' }}
        </button>
      </div>
    </header>
    
    <div v-if="loading" class="loading-container">
      <div class="spinner"></div>
    </div>
    
    <div v-else class="outline-content">
      <!-- 项目设定 -->
      <section class="outline-section card">
        <h3>故事前提</h3>
        <textarea 
          v-model="premise" 
          class="textarea" 
          placeholder="描述故事的核心设定..."
        ></textarea>
      </section>
      
      <section class="outline-section card">
        <h3>世界观设定</h3>
        <textarea 
          v-model="worldSetting" 
          class="textarea" 
          placeholder="描述故事的世界观..."
        ></textarea>
      </section>
      
      <!-- 三幕式结构 -->
      <section class="outline-section card">
        <h3>📖 三幕式结构</h3>
        
        <div class="act act1">
          <div class="act-header">
            <span class="act-num">第一幕</span>
            <span class="act-name">铺垫</span>
          </div>
          <textarea 
            v-model="act1Content" 
            class="textarea act-textarea" 
            placeholder="描述故事背景、主角介绍、核心冲突引入..."
          ></textarea>
        </div>
        
        <div class="act act2">
          <div class="act-header">
            <span class="act-num">第二幕</span>
            <span class="act-name">对抗</span>
          </div>
          <textarea 
            v-model="act2Content" 
            class="textarea act-textarea" 
            placeholder="描述主要冲突展开、主角成长挑战、中点转折..."
          ></textarea>
        </div>
        
        <div class="act act3">
          <div class="act-header">
            <span class="act-num">第三幕</span>
            <span class="act-name">解决</span>
          </div>
          <textarea 
            v-model="act3Content" 
            class="textarea act-textarea" 
            placeholder="描述高潮对决、冲突解决、结局收束..."
          ></textarea>
        </div>
      </section>
      
      <div class="outline-actions">
        <button class="btn btn-primary" @click="saveOutline">
          保存大纲
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { chapterApi } from '../api/chapter'
import { creationApi } from '../api/creation'

const route = useRoute()
const projectId = computed(() => route.params.id)

const loading = ref(false)
const generating = ref(false)
const saving = ref(false)

const premise = ref('')
const worldSetting = ref('')
const act1Content = ref('')
const act2Content = ref('')
const act3Content = ref('')
const outlineUpdatedAt = ref('')

onMounted(() => {
  loadOutline()
})

async function loadOutline() {
  loading.value = true
  try {
    const project = await chapterApi.getOutline(projectId.value)
    premise.value = project.premise || ''
    worldSetting.value = project.world_setting || ''
    outlineUpdatedAt.value = project.updated_at || ''
    
    // 尝试解析大纲 JSON
    if (project.outline) {
      try {
        const outline = typeof project.outline === 'string' 
          ? JSON.parse(project.outline) 
          : project.outline
        act1Content.value = outline.act1_setup || ''
        act2Content.value = outline.act2_confrontation || ''
        act3Content.value = outline.act3_resolution || ''
      } catch {
        // 如果不是 JSON，设置为 act1
        act1Content.value = project.outline
      }
    }
  } catch (error) {
    console.error('Failed to load outline:', error)
  } finally {
    loading.value = false
  }
}

async function saveOutline() {
  saving.value = true
  try {
    const outlineData = {
      premise: premise.value,
      world_setting: worldSetting.value,
      structure: {
        act1_setup: act1Content.value,
        act2_confrontation: act2Content.value,
        act3_resolution: act3Content.value
      }
    }
    
    const updated = await chapterApi.updateOutline(projectId.value, outlineData, outlineUpdatedAt.value)
    outlineUpdatedAt.value = updated.updated_at
  } catch (error) {
    console.error('Failed to save outline:', error)
  } finally {
    saving.value = false
  }
}

async function generateOutline() {
  if (!premise.value) {
    alert('请先填写故事前提')
    return
  }
  
  generating.value = true
  try {
    const result = await creationApi.generateOutline(projectId.value, {
      premise: premise.value,
      genre: ''
    })
    
    if (result.structure) {
      act1Content.value = result.structure.act1_setup || ''
      act2Content.value = result.structure.act2_confrontation || ''
      act3Content.value = result.structure.act3_resolution || ''
    }
  } catch (error) {
    console.error('Failed to generate outline:', error)
  } finally {
    generating.value = false
  }
}
</script>

<style scoped>
.outline {
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

.header-actions {
  display: flex;
  gap: 8px;
}

.outline-section {
  padding: 24px;
  margin-bottom: 20px;
}

.outline-section h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 16px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.act {
  margin-bottom: 20px;
}

.act:last-child {
  margin-bottom: 0;
}

.act-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.act-num {
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 13px;
  font-weight: 600;
}

.act1 .act-num {
  background: var(--accent-soft);
  color: var(--accent);
}

.act2 .act-num {
  background: var(--orange-soft);
  color: var(--orange);
}

.act3 .act-num {
  background: var(--green-soft);
  color: var(--green);
}

.act-name {
  font-size: 14px;
  font-weight: 500;
}

.act-textarea {
  min-height: 100px;
}

.outline-actions {
  display: flex;
  justify-content: flex-end;
}

.loading-container {
  display: flex;
  justify-content: center;
  padding: 40px;
}
</style>
