import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  build: {
    outDir: 'dist',
  },
  server: {
    // Dev server proxies the API to the Go backend (default :8080) so the app
    // calls same-origin /api/* in both dev and production.
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
