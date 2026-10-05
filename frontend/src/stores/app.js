import { defineStore } from 'pinia'

export const useAppStore = defineStore('app', {
  state: () => ({
    connected: false,
    loading: false
  }),
  actions: {
    setConnected(v) {
      this.connected = v
    },
    setLoading(v) {
      this.loading = v
    }
  }
})
