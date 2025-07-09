import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: path.resolve(__dirname, 'index.ts'),
      name: 'MichelangeloRpc',
      formats: ['es'],
    },
    rollupOptions: {
      external: [
        'react',
        '@bufbuild/protobuf',
        '@connectrpc/connect',
        '@connectrpc/connect-web',
        '@tanstack/react-query',
      ],
    },
    outDir: 'dist',
    emptyOutDir: true,
  },
  plugins: [react()],
  resolve: {
    conditions: ['workspace'],
  },
});
