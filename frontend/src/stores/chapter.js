import { defineStore } from 'pinia'
import { api } from '../api'

export const useChapterStore = defineStore('chapter', {
  state: () => ({
    chapters: [],
    currentChapter: null,
    loading: false
  }),
  
  actions: {
    async fetchChapters(projectId) {
      this.loading = true
      try {
        this.chapters = await api.get(`/projects/${projectId}/chapters`)
      } catch (error) {
        console.error('Failed to fetch chapters:', error)
        this.chapters = []
      } finally {
        this.loading = false
      }
    },
    
    async fetchChapter(projectId, chapterNum) {
      this.loading = true
      try {
        this.currentChapter = await api.get(`/projects/${projectId}/chapters/${chapterNum}`)
      } catch (error) {
        console.error('Failed to fetch chapter:', error)
        this.currentChapter = null
      } finally {
        this.loading = false
      }
    },
    
    async createChapter(projectId, data) {
      const chapter = await api.post(`/projects/${projectId}/chapters`, data)
      this.chapters.push(chapter)
      this.chapters.sort((a, b) => a.chapter_number - b.chapter_number)
      return chapter
    },
    
    async updateChapter(projectId, chapterNum, data) {
      const chapter = await api.put(`/projects/${projectId}/chapters/${chapterNum}`, data)
      const index = this.chapters.findIndex(c => c.chapter_number === chapterNum)
      if (index !== -1) {
        this.chapters[index] = chapter
      }
      if (this.currentChapter?.chapter_number === chapterNum) {
        this.currentChapter = chapter
      }
      return chapter
    },
    
    async deleteChapter(projectId, chapterNum) {
      await api.delete(`/projects/${projectId}/chapters/${chapterNum}`)
      this.chapters = this.chapters.filter(c => c.chapter_number !== chapterNum)
      if (this.currentChapter?.chapter_number === chapterNum) {
        this.currentChapter = null
      }
    }
  }
})
