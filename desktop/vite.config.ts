import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  // Electron 打包后以 file:// 加载页面，静态资源必须用相对路径
  base: './',
  plugins: [vue()],
  server: {
    // 显式监听 IPv4：wait-on 与 Electron 都用 127.0.0.1，避免 localhost 解析到 ::1 而探活失败
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1024,
  },
})
