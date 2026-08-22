import { defineConfig } from 'vite'

// cmd/dev sets this env var to match whatever -addr it started the Go
// backend on, so overriding the backend port there doesn't silently break
// this proxy target.
const backendAddr = process.env.FASTER_WHISPER_GUI_BACKEND_ADDR ?? '127.0.0.1:8080'

export default defineConfig({
  server: {
    proxy: {
      '/api': `http://${backendAddr}`,
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
  },
})
