import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: path.resolve(__dirname, 'index.tsx'),
      name: 'MichelangeloCore',
      formats: ['es', 'cjs'],
      fileName: (format) => `michelangelo-core.${format === 'es' ? 'js' : 'cjs'}`,
    },
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        'react-router-dom',
        'react-router-dom-v5-compat',
        '@bufbuild/protobuf',
        '@connectrpc/connect',
        '@connectrpc/connect-web',
        'pluralize',
        'styletron-engine-monolithic',
        'styletron-react',
        '@tanstack/react-query',
      ],
    },
    outDir: 'dist',
    emptyOutDir: true,
  },
  plugins: [react()],
});
