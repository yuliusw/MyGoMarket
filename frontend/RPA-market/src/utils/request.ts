import axios from 'axios'

const request = axios.create({
  baseURL: '/api/v1',
  withCredentials: true, // 👈 保持相对路径，不要写 http://127.0.0.1:12660
  timeout: 5000,
  headers: {
    'Content-Type': 'application/json'
  }
})
// 请求拦截器：自动注入 Token
request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('auth_token')
    if (token && config.headers) {
      // 假设后端 JWT 校验采用标准的 Authorization: Bearer <token>
      // 如果后端直接从 Header 读取 "auth_token"，则改为 config.headers['auth_token'] = token
      config.headers['Authorization'] = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

// 响应拦截器：统一处理 401 登录失效
request.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('auth_token')
      window.location.href = '/home/login'
    }
    return Promise.reject(error.response?.data || error)
  }
)

export default request