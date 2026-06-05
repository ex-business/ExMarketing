import { defineConfig } from 'vite';

export default defineConfig({
  root: 'frontend/admin-flow',
  base: '/admin-flow/',
  server: {
    proxy: {
      '/admin/v1': 'http://127.0.0.1:8080',
      '/openapi/v1': 'http://127.0.0.1:8080',
      '/v1': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
      '/metrics': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: '../../web/admin-flow',
    emptyOutDir: true,
  },
});
