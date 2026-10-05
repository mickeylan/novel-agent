<template>
  <router-view />
</template>

<script setup>
import { onMounted } from 'vue'
import { useAppStore } from './stores/app'

const appStore = useAppStore()

onMounted(async () => {
  // 检查后端连接
  try {
    const response = await fetch('/api/v1/health')
    const data = await response.json()
    appStore.setConnected(data.status === 'healthy')
  } catch {
    appStore.setConnected(false)
  }
})
</script>
