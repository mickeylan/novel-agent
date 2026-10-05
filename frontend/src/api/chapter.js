import { api } from './index'

export const chapterApi = {
  // 章节列表
  list: (projectId) => api.get(`/projects/${projectId}/chapters`),
  
  // 创建章节
  create: (projectId, data) => api.post(`/projects/${projectId}/chapters`, data),
  
  // 获取章节
  get: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}`),
  
  // 更新章节
  update: (projectId, chapterNum, data) => api.put(`/projects/${projectId}/chapters/${chapterNum}`, data),
  
  // 删除章节
  delete: (projectId, chapterNum) => api.delete(`/projects/${projectId}/chapters/${chapterNum}`),
  
  // 大纲
  getOutline: (projectId) => api.get(`/projects/${projectId}/outline`),
  updateOutline: (projectId, data) => api.put(`/projects/${projectId}/outline`, data),
  getChapterOutline: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}/outline`),
  updateChapterOutline: (projectId, chapterNum, data) => api.put(`/projects/${projectId}/chapters/${chapterNum}/outline`, data)
}

export default chapterApi
