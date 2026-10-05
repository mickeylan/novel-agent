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
      const project = await api.put(`/projects/${id}`, data)
      const index = this.projects.findIndex(p => p.id === id)
      if (index !== -1) {
        this.projects[index] = project
      }
      if (this.currentProject?.id === id) {
        this.currentProject = project
      }
      return project
    },
    
    async deleteProject(id) {
      await api.delete(`/projects/${id}`)
      this.projects = this.projects.filter(p => p.id !== id)
      if (this.currentProject?.id === id) {
        this.currentProject = null
      }
    }
  }
})
