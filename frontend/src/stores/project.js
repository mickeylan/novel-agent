import { defineStore } from 'pinia'
import { api } from '../api'

export const useProjectStore = defineStore('project', {
  state: () => ({
    projects: [],
    currentProject: null,
    loading: false
  }),
  
  actions: {
    async fetchProjects() {
      this.loading = true
      try {
        this.projects = await api.get('/projects')
      } catch (error) {
        console.error('Failed to fetch projects:', error)
        this.projects = []
      } finally {
        this.loading = false
      }
    },
    
    async fetchProject(id) {
      this.loading = true
      try {
        this.currentProject = await api.get(`/projects/${id}`)
      } catch (error) {
        console.error('Failed to fetch project:', error)
        this.currentProject = null
      } finally {
        this.loading = false
      }
    },
    
    async createProject(data) {
      const project = await api.post('/projects', data)
      this.projects.unshift(project)
      return project
    },
    
    async updateProject(id, data) {
      const current = this.projects.find(p => p.id === id) || this.currentProject
      if (!current?.updated_at) throw new Error('项目版本缺失，请刷新后重试')
      const project = await api.put(`/projects/${id}`, { ...data, updated_at: current.updated_at })
      const index = this.projects.findIndex(p => p.id === id)
      if (index !== -1) {
        this.projects[index] = project
      }
      if (this.currentProject?.id === id) {
        this.currentProject = project
      }
      return project
    }
  }
})
