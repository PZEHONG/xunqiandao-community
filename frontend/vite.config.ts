import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  base: '/next/', plugins: [react()],
  build: {
    outDir: fileURLToPath(new URL('../web/static/berry/', import.meta.url)),
    emptyOutDir: true,
  },
  server: { proxy: Object.fromEntries(['/api', '/_AMapService', '/terms', '/privacy'].map(path => [path, { target: 'http://127.0.0.1:8080' }])) },
});
