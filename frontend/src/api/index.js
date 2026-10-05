import axios from 'axios'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: 300000 // 5分钟，用于 AI 生成
})

// 请求拦截器
api.interceptors.request.use(
  config => {
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// 响应拦截器
api.interceptors.response.use(
  response => {
    return response.data
  },
  error => {
    const message = error.response?.data?.error || error.message || '请求失败'
    console.error('API Error:', message)
    return Promise.reject(error)
  }
)

export { api }
