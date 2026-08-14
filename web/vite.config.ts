import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '')
  const releaseTag = env.OPENSURGE_RELEASE_TAG?.trim() || 'v0.2.0-dev'

  return {
    plugins: [react()],
    define: {
      'import.meta.env.VITE_OPENSURGE_RELEASE_TAG': JSON.stringify(releaseTag),
    },
    build: {
      outDir: '../internal/webui/dist',
      emptyOutDir: true,
    },
    server: {
      port: 5173,
      proxy: {
        '/api': 'http://127.0.0.1:61767',
        '/bootstrap': 'http://127.0.0.1:61767',
      },
    },
  }
})
