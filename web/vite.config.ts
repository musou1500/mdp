import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // The Go server hosts the document root at /, so the viewer's own files live
  // under a reserved prefix where they cannot collide with the user's files.
  // Vite writes them into dist/assets/, which the server serves at /_md/assets/.
  base: '/_md/',
  build: {
    outDir: '../internal/server/dist',
    emptyOutDir: true,
  },
})
