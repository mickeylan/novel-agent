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
        const chapter = await api.get(`/projects/${projectId}/chapters/${chapterNum}`)
        this.currentChapter = chapter
        const index = this.chapters.findIndex(item => item.chapter_number === chapterNum)
        if (index !== -1) this.chapters[index] = chapter
        return chapter
      } catch (error) {
        console.error('Failed to fetch chapter:', error)
        throw error
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
    
    acceptChapter(chapter) {
      const index = this.chapters.findIndex(item => item.chapter_number === chapter.chapter_number)
      if (index !== -1) this.chapters[index] = chapter
      if (this.currentChapter?.chapter_number === chapter.chapter_number) this.currentChapter = chapter
    },

    async updateChapter(projectId, chapterNum, data) {
      const current = this.chapters.find(c => c.chapter_number === chapterNum) || this.currentChapter
      if (!current?.updated_at) throw new Error('章节版本缺失，请刷新后重试')
      const chapter = await api.put(`/projects/${projectId}/chapters/${chapterNum}`, { ...data, updated_at: current.updated_at })
      const index = this.chapters.findIndex(c => c.chapter_number === chapterNum)
      if (index !== -1) {
        this.chapters[index] = chapter
      }
      if (this.currentChapter?.chapter_number === chapterNum) {
        this.currentChapter = chapter
      }
      return chapter
    }
  }
})
