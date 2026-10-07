import axios from 'axios'
import { normalizeApiError } from './error'
import { attachRequestID } from './requestId'
import { STANDARD_API_TIMEOUT_MS } from './timeouts'

const api = axios.create({
  baseURL: '/api/v1',
  timeout: STANDARD_API_TIMEOUT_MS
})

// 请求拦截器
api.interceptors.request.use(
  config => attachRequestID(config),
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
    error.apiError = normalizeApiError(error)
    return Promise.reject(error)
  }
)

export { api }
