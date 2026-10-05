import { api } from './index'

export const characterApi = {
  // 角色列表
  list: (projectId) => api.get(`/projects/${projectId}/characters`),
  
  // 创建角色
  create: (projectId, data) => api.post(`/projects/${projectId}/characters`, data),
  
  // 获取角色
  get: (projectId, characterId) => api.get(`/projects/${projectId}/characters/${characterId}`),
  
  // 更新角色
  update: (projectId, characterId, data) => api.put(`/projects/${projectId}/characters/${characterId}`, data),
  
  // 删除角色
  delete: (projectId, characterId) => api.delete(`/projects/${projectId}/characters/${characterId}`)
}

export default characterApi
