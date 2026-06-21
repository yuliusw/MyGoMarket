import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    vueJsx(),
    vueDevTools(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    },
  },
server: {
    port: 5173,
    proxy: {
      // 拦截所有以 /api 开头的请求
      '/api': {
        target: 'http://localhost:12660', // 转发到你的后端真实地址
        changeOrigin: true, // 必须开启，用来突破跨域限制
        // 注意：因为你的实际请求地址里本身就包含 /api/v1，所以这里不需要 rewrite 重写路径
      }
    }
  }
})
