import { api } from './index'

export const creationApi = {
  // 生成大纲
  generateOutline: (projectId, data) => api.post(`/projects/${projectId}/generate-outline`, data),
  
  // AI 写作
  writeChapter: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/write`, data),
  
  // AI 润色
  polishChapter: (projectId, data) => api.post(`/projects/${projectId}/polish`, data),
  
  // AI 评估
  auditChapter: (projectId, data) => api.post(`/projects/${projectId}/audit`, data)
}

export default creationApi
