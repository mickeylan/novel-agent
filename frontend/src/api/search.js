import { api } from './index'

export const searchApi = {
  // 向量搜索
  vector: (data) => api.post('/search/vector', data),
  
  // 关键词搜索
  keyword: (data) => api.post('/search/keyword', data)
}

export default searchApi
