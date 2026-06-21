import { defineConfig } from 'vite';

export default defineConfig({
  root: 'frontend/admin-flow',
  base: '/admin-flow/',
  plugins: [{
    name: 'admin-flow-slash-redirect',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        if (req.url?.startsWith('/admin-flow?') || req.url === '/admin-flow') {
          const target = req.url.replace('/admin-flow', '/admin-flow/');
          res.statusCode = 302;
          res.setHeader('Location', target);
          res.end();
          return;
        }
        next();
      });
    },
  }],
  server: {
    proxy: {
      '/admin/v1': 'http://127.0.0.1:8080',
      '/openapi/v1': 'http://127.0.0.1:8080',
      '/v1': 'http://127.0.0.1:8080',
      '/share': 'http://127.0.0.1:8080',
      '/invite': 'http://127.0.0.1:8080',
      '/healthz': 'http://127.0.0.1:8080',
      '/metrics': 'http://127.0.0.1:8080',
    },
  },
  build: {
    outDir: '../../web/admin-flow',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/@codemirror') || id.includes('node_modules/@uiw/react-codemirror') || id.includes('node_modules/@lezer')) return 'vendor-editor';
          if (id.includes('node_modules/@xyflow')) return 'vendor-flow';
          if (id.includes('node_modules/lucide-react')) return 'vendor-icons';
          if (id.includes('node_modules/react') || id.includes('node_modules/react-dom')) return 'vendor-react';
          if (id.includes('/frontend/admin-flow/src/components/workspace-home/')) return 'workspace-home';
          if (id.includes('/frontend/admin-flow/src/components/')) return 'studio-components';
          return undefined;
        },
      },
    },
  },
});
