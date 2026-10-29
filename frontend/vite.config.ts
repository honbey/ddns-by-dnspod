import path from 'path'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'

export default defineConfig({
  plugins: [
    // File routes are generated before the bundle is built. Code splitting is
    // off on purpose: the reverse proxy chain (nginx -> code-server -> this
    // port) answers 502 for the dynamic chunk requests issued after the first
    // paint, which left pages blank.
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: false,
    }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: 5173,
    // Vite refuses any request whose Host it does not recognise, which is every
    // workspace preview domain, so a preview has to name itself:
    // `DEV_ALLOWED_HOSTS=<host> npm run dev -- --host 0.0.0.0`.
    allowedHosts: (process.env.DEV_ALLOWED_HOSTS ?? '')
      .split(',')
      .map((host) => host.trim())
      .filter(Boolean),
    proxy: {
      // Same origin during development, so the bearer token the SPA sends is
      // accepted by the server it came from and no CORS allowlist is needed.
      '/api': {
        target: process.env.DEV_API_TARGET ?? 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      // The server answers this itself; proxying it keeps development on the
      // same document production serves, instead of the static fallback.
      '/config.json': {
        target: process.env.DEV_API_TARGET ?? 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    // Output straight into the Go embed directory; rebuild the backend to pick it up
    outDir: '../backend/internal/frontend/dist',
    emptyOutDir: true,
    target: 'es2020',
    sourcemap: false,
    assetsInlineLimit: 0,
    // No dynamic chunks to split (see autoCodeSplitting above), so the bundle
    // is one file by construction; the warning would fire on every build.
    chunkSizeWarningLimit: 1500,
  },
  test: {
    environment: 'happy-dom',
    include: ['src/**/*.test.{ts,tsx}'],
    exclude: ['e2e/**'],
  },
})
