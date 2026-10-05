import { api } from './index'

export const projectApi = {
  // 项目列表
  list: () => api.get('/projects'),
  
  // 创建项目
  create: (data) => api.post('/projects', data),
  
  // 获取项目
  get: (id) => api.get(`/projects/${id}`),
  
  // 更新项目
  update: (id, data) => api.put(`/projects/${id}`, data),
  
  // 删除项目
  delete: (id) => api.delete(`/projects/${id}`)
}

export default projectApi
