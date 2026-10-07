import { api } from './index'
import { withUpdatedAt } from './concurrency'

export const chapterApi = {
  // 章节列表
  list: (projectId) => api.get(`/projects/${projectId}/chapters`),
  
  // 创建章节
  create: (projectId, data) => api.post(`/projects/${projectId}/chapters`, data),
  
  // 获取章节
  get: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}`),
  
  // 更新章节
  update: (projectId, chapterNum, data, updatedAt) => api.put(`/projects/${projectId}/chapters/${chapterNum}`, withUpdatedAt(data, updatedAt)),
  
  // 大纲
  getOutline: (projectId) => api.get(`/projects/${projectId}/outline`),
  updateOutline: (projectId, data, updatedAt) => api.put(`/projects/${projectId}/outline`, withUpdatedAt(data, updatedAt)),
  getChapterOutline: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}/outline`),
  updateChapterOutline: (projectId, chapterNum, data, updatedAt) => api.put(`/projects/${projectId}/chapters/${chapterNum}/outline`, withUpdatedAt(data, updatedAt))
}

export default chapterApi
