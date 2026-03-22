import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/blackwood.v1': 'http://localhost:8080',
      '/api': 'http://localhost:8080',
    }
  }
})
