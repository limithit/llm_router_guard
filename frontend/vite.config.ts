import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // 开发环境将 /api 与 /v1 代理到后端 Go 服务（与 api-contract.md 的 Base URL 一致）
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    chunkSizeWarningLimit: 1500,
    // vendor 分层缓存：业务页(路由懒加载)频繁迭代，antd/react/i18n 稳定不变，
    // 独立 chunk 后发版只使业务 chunk 失效，vendor 命中浏览器长缓存
    rollupOptions: {
      output: {
        manualChunks: {
          react: ['react', 'react-dom', 'react-router-dom', '@tanstack/react-query', 'zustand'],
          antd: ['antd', '@ant-design/icons', 'dayjs'],
          i18n: ['i18next', 'react-i18next'],
        },
      },
    },
  },
});
