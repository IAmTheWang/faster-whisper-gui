import { defineConfig } from 'vite'

// cmd/dev sets this env var to match whatever -addr it started the Go
// backend on, so overriding the backend port there doesn't silently break
// this proxy target.
const backendAddr = process.env.FASTER_WHISPER_GUI_BACKEND_ADDR ?? '127.0.0.1:8080'

// cmd/dev generates the API token (shared with the Go backend) and passes it
// here. In production the Go server substitutes the placeholder itself; in dev
// the page comes from Vite, so do the same substitution while serving.
const apiToken = process.env.FASTER_WHISPER_GUI_TOKEN ?? ''

export default defineConfig({
  plugins: [
    {
      name: 'inject-api-token',
      apply: 'serve',
      transformIndexHtml: (html) => html.replace('__API_TOKEN__', apiToken),
    },
  ],
  server: {
    proxy: {
      '/api': `http://${backendAddr}`,
    },
  },
  build: {
    outDir: '../internal/webui/dist',
    emptyOutDir: true,
    // The bundle is embedded in the Go binary: skip source maps (they would
    // only bloat the exe) and the gzip-size report (it slows every build).
    sourcemap: false,
    reportCompressedSize: false,
  },
})
