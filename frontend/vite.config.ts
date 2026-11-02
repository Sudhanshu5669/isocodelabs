import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // In development, forward /api calls to the Go server so the browser
    // sees one origin (no CORS setup needed).
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
