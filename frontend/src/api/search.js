import { api } from './index'

export const searchApi = {
  // 向量搜索
  vector: (data) => api.post('/search/vector', data),
  
  // 关键词搜索
  keyword: (data) => api.post('/search/keyword', data),

  // 向量与关键词联合检索
  hybrid: (data) => api.post('/search/hybrid', data),
  ragContextPreview: (projectId, data) => api.post(`/projects/${projectId}/rag-context-preview`, data)
}

export default searchApi
