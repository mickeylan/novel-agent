# Novel Agent 前端规划

> 文档版本：v1.0  
> 创建日期：2026-10-05  
> 参考架构：new-aigc-comfyui-minimax-h3

---

## 一、技术选型

**完全参考 minimax-h3 架构：**

| 类别 | 技术 | 说明 |
|------|------|------|
| **框架** | Vue 3 + Vite | Composition API |
| **状态管理** | Pinia | 轻量级状态管理 |
| **HTTP** | Axios | API 调用 |
| **路由** | Vue Router 4 | SPA 路由 |
| **样式** | 原生 CSS | 变量 + scoped styles |
| **组件** | 单文件组件 (.vue) | SFC 模式 |

**不使用的技术：**
- ~~Ant Design~~ - 改用原生 CSS + 自定义组件
- ~~Tailwind~~ - 改用 CSS 变量
- ~~React~~ - 使用 Vue 3

---

## 二、项目结构

```
frontend/
├── index.html
├── package.json
├── vite.config.js
├── public/
├── src/
│   ├── main.js
│   ├── App.vue
│   ├── api/                 # API 调用层
│   │   ├── index.js         # Axios 配置 + 统一拦截器
│   │   ├── project.js       # 项目 API
│   │   ├── chapter.js       # 章节 API
│   │   ├── character.js     # 角色 API
│   │   ├── search.js        # 搜索 API
│   │   └── creation.js      # 创作 API
│   ├── components/          # 通用组件
│   │   ├── PageHero.vue     # Hero 区域
│   │   ├── ProjectCard.vue  # 项目卡片
│   │   ├── ChapterItem.vue  # 章节项
│   │   ├── LoadingSpinner.vue
│   │   ├── Modal.vue
│   │   └── StatusBadge.vue
│   ├── views/               # 页面视图
│   │   ├── Projects.vue     # 项目列表（首页）
│   │   ├── ProjectNew.vue   # 新建项目
│   │   ├── ProjectDetail.vue # 项目详情
│   │   ├── Characters.vue   # 角色管理
│   │   ├── Chapters.vue     # 章节列表
│   │   ├── WritingStudio.vue # 写作工作室
│   │   ├── Outline.vue      # 大纲编辑
│   │   ├── Search.vue       # 知识库搜索
│   │   ├── Evaluation.vue   # 质量评估
│   │   └── Settings.vue     # 设置
│   ├── router/
│   │   └── index.js        # 路由配置
│   ├── stores/              # Pinia 状态
│   │   ├── app.js          # 全局状态
│   │   ├── project.js       # 项目状态
│   │   ├── chapter.js       # 章节状态
│   │   └── writing.js       # 写作状态
│   ├── utils/
│   │   └── format.js        # 工具函数
│   └── styles/
│       ├── variables.css    # CSS 变量
│       └── global.css      # 全局样式
```

---

## 三、API 层设计

### 3.1 Axios 配置 (api/index.js)

```javascript
import axios from 'axios'
import { ElMessage } from 'element-plus'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 300000, // 5分钟，用于 AI 生成
})

// 请求拦截器
api.interceptors.request.use(config => {
  return config
})

// 响应拦截器
api.interceptors.response.use(
  response => response.data,
  error => {
    const msg = error.response?.data?.error || error.message || '请求失败'
    ElMessage.error(msg)
    return Promise.reject(error)
  }
)

export { api }

// 各模块 API
export const projects = () => api.get('/projects')
export const createProject = (data) => api.post('/projects', data)
export const getProject = (id) => api.get(`/projects/${id}`)
export const deleteProject = (id) => api.delete(`/projects/${id}`)

// ... 其他 API
```

---

## 四、路由设计

```javascript
// router/index.js
import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/',
    name: 'projects',
    component: () => import('../views/Projects.vue')
  },
  {
    path: '/projects/new',
    name: 'project-new',
    component: () => import('../views/ProjectNew.vue')
  },
  {
    path: '/projects/:id',
    name: 'project-detail',
    component: () => import('../views/ProjectDetail.vue'),
    children: [
      { path: '', redirect: 'overview' },
      { path: 'overview', name: 'overview', component: () => import('../views/Overview.vue') },
      { path: 'chapters', name: 'chapters', component: () => import('../views/Chapters.vue') },
      { path: 'characters', name: 'characters', component: () => import('../views/Characters.vue') },
      { path: 'outline', name: 'outline', component: () => import('../views/Outline.vue') },
      { path: 'writing', name: 'writing', component: () => import('../views/WritingStudio.vue') },
    ]
  },
  {
    path: '/search',
    name: 'search',
    component: () => import('../views/Search.vue')
  },
]

export default createRouter({
  history: createWebHistory(),
  routes
})
```

---

## 五、页面设计

### 5.1 项目列表 (Projects.vue) - 首页

**功能：**
- Hero 区域（参考 minimax-h3 风格）
- 工作台统计（项目总数、制作中、已完成）
- 项目卡片网格
- 新建项目按钮

**组件结构：**
```
Projects.vue
├── Hero Section (PageHero)
├── Workspace Summary
├── Project Grid
│   └── ProjectCard (v-for)
├── Create Modal
└── Video Complete Alert
```

### 5.2 项目详情 (ProjectDetail.vue)

**功能：**
- 项目信息头部
- 侧边导航
- 内容区域（嵌套路由）

**布局：**
```
ProjectDetail.vue
├── Header (项目信息)
├── Layout
│   ├── Sidebar (导航)
│   └── Content (子路由)
│       ├── Overview (概览)
│       ├── Chapters (章节)
│       ├── Characters (角色)
│       ├── Outline (大纲)
│       └── Writing (写作)
```

### 5.3 章节编辑 (ChapterEditor)

**功能：**
- 工具栏（保存、AI写作、AI润色、AI审核）
- Markdown 编辑器
- 章节大纲侧边栏
- 状态栏

### 5.4 写作工作室 (WritingStudio)

**功能：**
- AI 生成面板
- 生成进度
- 预览区域
- 保存/重新生成

---

## 六、状态管理 (Pinia Stores)

### 6.1 App Store (app.js)

```javascript
import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    connected: false,
    loading: false,
  }),
  actions: {
    setConnected(v) {
      this.connected = v
    },
    setLoading(v) {
      this.loading = v
    }
  }
})
```

### 6.2 Project Store (project.js)

```javascript
import { defineStore } from 'pinia'
import { api } from '../api'

export const useProjectStore = defineStore('project', {
  state: () => ({
    projects: [],
    currentProject: null,
    loading: false,
  }),
  actions: {
    async fetchProjects() {
      this.loading = true
      try {
        this.projects = await api.projects()
      } finally {
        this.loading = false
      }
    },
    async createProject(data) {
      const project = await api.createProject(data)
      this.projects.unshift(project)
      return project
    },
    async deleteProject(id) {
      await api.deleteProject(id)
      this.projects = this.projects.filter(p => p.id !== id)
    },
    setCurrentProject(project) {
      this.currentProject = project
    }
  }
})
```

### 6.3 Writing Store (writing.js)

```javascript
import { defineStore } from 'pinia'
import { api } from '../api'

export const useWritingStore = defineStore('writing', {
  state: () => ({
    isGenerating: false,
    progress: 0,
    generatedContent: '',
  }),
  actions: {
    async generateChapter(projectId, chapterNum, config) {
      this.isGenerating = true
      this.progress = 0
      try {
        const result = await api.writeChapter(projectId, chapterNum, config)
        this.generatedContent = result.content
        return result
      } finally {
        this.isGenerating = false
      }
    }
  }
})
```

---

## 七、样式设计

### 7.1 CSS 变量 (styles/variables.css)

```css
:root {
  /* 颜色 */
  --bg: #08090c;
  --bg-secondary: #111522;
  --card: #16181e;
  --card-hover: #1e2028;
  
  --accent: #0a84ff;
  --accent-soft: rgba(10, 132, 255, 0.15);
  --orange: #d96c00;
  --green: #16883b;
  --red: #ff3b30;
  
  --text: #f5f5f7;
  --text-secondary: #86868b;
  --text-tertiary: #6e6e73;
  
  --border: rgba(255, 255, 255, 0.1);
  
  /* 圆角 */
  --radius-sm: 9px;
  --radius-md: 13px;
  --radius-lg: 18px;
  --radius-xl: 28px;
  
  /* 阴影 */
  --shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
  --shadow-hover: 0 8px 30px rgba(0, 0, 0, 0.25);
}
```

### 7.2 全局样式 (styles/global.css)

```css
@import './variables.css';

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: var(--bg);
  color: var(--text);
  line-height: 1.5;
}

a {
  color: var(--accent);
  text-decoration: none;
}

.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  border-radius: 980px;
  font-weight: 600;
  cursor: pointer;
  border: none;
  transition: all 0.2s;
}

.btn-primary {
  background: var(--accent);
  color: #fff;
}

.btn-primary:hover {
  filter: brightness(1.1);
}

.card {
  background: var(--card);
  border-radius: var(--radius-lg);
  border: 1px solid var(--border);
}
```

---

## 八、组件设计

### 8.1 ProjectCard.vue

```vue
<template>
  <article 
    class="card project-card" 
    :style="{ '--project-progress': progress + '%' }"
    @click="$router.push(`/projects/${project.id}`)"
  >
    <div class="card-accent"></div>
    <div class="project-head">
      <span class="project-icon">{{ icon }}</span>
      <div class="project-info">
        <div class="project-title">{{ project.title }}</div>
        <div class="project-time">{{ formatTime(project.updated_at) }}</div>
      </div>
      <span class="badge" :class="statusClass">{{ statusText }}</span>
    </div>
    <p class="project-synopsis">{{ project.description || '暂无描述' }}</p>
    <div class="project-stats">
      <span class="stat"><b>{{ chapterCount }}</b><small>章节</small></span>
      <span class="stat"><b>{{ wordCount }}</b><small>字数</small></span>
    </div>
    <div class="project-foot">
      <button class="btn btn-sm btn-danger" @click.stop="deleteProject">
        删除
      </button>
      <span class="go">→</span>
    </div>
  </article>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  project: { type: Object, required: true }
})

const emit = defineEmits(['delete'])

const progress = computed(() => 0) // TODO: 计算进度

const icon = computed(() => '✍')
const statusClass = computed(() => 'badge-gray')
const statusText = computed(() => '草稿')

function formatTime(value) {
  if (!value) return '刚刚'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'
  }).format(new Date(value))
}

function deleteProject() {
  if (confirm(`确定删除项目「${props.project.title}」？`)) {
    emit('delete', props.project.id)
  }
}
</script>

<style scoped>
.project-card {
  padding: 22px;
  cursor: pointer;
  transition: transform 0.25s, box-shadow 0.25s;
}

.project-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--shadow-hover);
}

/* ... 更多样式 */
</style>
```

---

## 九、开发阶段

### Phase 1: 基础框架 (1天)
- [ ] 项目初始化 (Vite + Vue 3)
- [ ] 路由配置
- [ ] 样式变量定义
- [ ] API 客户端封装
- [ ] Pinia Store 基础

### Phase 2: 首页 (1天)
- [ ] Projects.vue 主页面
- [ ] PageHero 组件
- [ ] ProjectCard 组件
- [ ] 新建项目弹窗

### Phase 3: 项目详情 (2天)
- [ ] ProjectDetail.vue 布局
- [ ] Overview 概览
- [ ] 侧边导航

### Phase 4: 章节管理 (1天)
- [ ] Chapters.vue 章节列表
- [ ] ChapterCard 组件
- [ ] 章节创建/编辑

### Phase 5: 角色管理 (1天)
- [ ] Characters.vue 角色表
- [ ] 角色表单

### Phase 6: 写作功能 (2天)
- [ ] WritingStudio.vue 写作工作室
- [ ] AI 生成面板
- [ ] 进度展示

### Phase 7: 大纲编辑 (1天)
- [ ] Outline.vue 大纲视图
- [ ] 三幕式结构编辑

### Phase 8: 搜索 (1天)
- [ ] Search.vue 搜索页面
- [ ] 结果展示

---

## 十、依赖清单

```json
{
  "name": "novel-agent-frontend",
  "private": true,
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "axios": "^1.7.9",
    "pinia": "^2.2.6",
    "vue": "^3.5.13",
    "vue-router": "^4.5.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.2.1",
    "vite": "^6.0.7"
  }
}
```

---

## 附录：API 端点

### 项目
```
GET    /api/v1/projects           # 列表
POST   /api/v1/projects           # 创建
GET    /api/v1/projects/:id       # 详情
DELETE /api/v1/projects/:id       # 删除
```

### 章节
```
GET    /api/v1/projects/:id/chapters           # 列表
POST   /api/v1/projects/:id/chapters           # 创建
GET    /api/v1/projects/:id/chapters/:num       # 详情
PUT    /api/v1/projects/:id/chapters/:num       # 更新
DELETE /api/v1/projects/:id/chapters/:num       # 删除
```

### 角色
```
GET    /api/v1/projects/:id/characters           # 列表
POST   /api/v1/projects/:id/characters           # 创建
GET    /api/v1/projects/:id/characters/:cid      # 详情
PUT    /api/v1/projects/:id/characters/:cid      # 更新
DELETE /api/v1/projects/:id/characters/:cid      # 删除
```

### 创作
```
POST /api/v1/projects/:id/generate-outline     # 生成大纲
POST /api/v1/projects/:id/chapters/:num/write  # AI 写作
POST /api/v1/projects/:id/chapters/:num/polish # AI 润色
POST /api/v1/projects/:id/evaluate             # AI 评估
```

### 搜索
```
POST /api/v1/search/vector   # 向量搜索
POST /api/v1/search/keyword  # 关键词搜索
```

---

*文档结束*
