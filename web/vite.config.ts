import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import Inspector from 'unplugin-vue-dev-locator/vite'
import traeBadgePlugin from 'vite-plugin-trae-solo-badge'

// https://vite.dev/config/
export default defineConfig({
  build: {
    sourcemap: 'hidden',
  },
  plugins: [
    vue(),
    Inspector(),
    traeBadgePlugin({
      variant: 'dark',
      position: 'bottom-right',
      prodOnly: true,
      clickable: true,
      clickUrl: 'https://www.trae.ai/solo?showJoin=1',
      autoTheme: true,
      autoThemeTarget: '#app',
    }),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // 用户服务
      '/api/v1/user': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      // 视频服务
      '/api/v1/video': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
      // 互动服务
      '/api/v1/interaction': {
        target: 'http://localhost:8083',
        changeOrigin: true,
      },
    },
  },
})
