import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In dev the dashboard runs on its own port and proxies API calls to fluxor-server.
const serverURL = process.env.FLUXOR_SERVER_URL ?? 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': serverURL,
      '/healthz': serverURL,
    },
  },
})
