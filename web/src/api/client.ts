import axios from 'axios'

const client = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

// Request interceptor - inject token
client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 401 跳转防抖：多个并发请求同时过期时只触发一次跳转
let redirectingToLogin = false

// Response interceptor - handle errors
client.interceptors.response.use(
  (res) => {
    // Blob 响应（头像等二进制）不包含业务 status 字段，直接透传
    if (res.config.responseType === 'blob' || res.data instanceof Blob) {
      return res
    }
    if (res.data?.status !== 0) {
      return Promise.reject(new Error(res.data?.info || 'Unknown error'))
    }
    return res
  },
  (err) => {
    // JWT 过期/无效：清理登录态并跳转登录页
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      if (!redirectingToLogin) {
        redirectingToLogin = true
        window.location.hash = '#/login'
      }
    }
    return Promise.reject(err)
  }
)

export default client

// Generic API response
export interface ApiResponse<T = any> {
  status: number
  info: string
  data: T
}
