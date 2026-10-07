import { api } from './index'
import { withUpdatedAt } from './concurrency'

export const projectApi = {
  // 项目列表
  list: () => api.get('/projects'),
  
  // 创建项目
  create: (data) => api.post('/projects', data),
  
  // 获取项目
  get: (id) => api.get(`/projects/${id}`),
  
  // 更新项目
  update: (id, data, updatedAt) => api.put(`/projects/${id}`, withUpdatedAt(data, updatedAt)),

  // 只读导出
  export: (id, format = 'txt') => api.get(`/projects/${id}/export`, {
    params: { format },
    responseType: 'blob'
  })
}

export default projectApi
