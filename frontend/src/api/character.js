import { api } from './index'
import { withUpdatedAt } from './concurrency'

export const characterApi = {
  // 角色列表
  list: (projectId) => api.get(`/projects/${projectId}/characters`),
  
  // 创建角色
  create: (projectId, data) => api.post(`/projects/${projectId}/characters`, data),
  
  // 获取角色
  get: (projectId, characterId) => api.get(`/projects/${projectId}/characters/${characterId}`),
  
  // 更新角色
  update: (projectId, characterId, data, updatedAt) => api.put(`/projects/${projectId}/characters/${characterId}`, withUpdatedAt(data, updatedAt))
}

export default characterApi
