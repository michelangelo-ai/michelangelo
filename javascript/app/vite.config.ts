import react from '@vitejs/plugin-react';

import { defineConfig } from 'vite';
import path from 'path';

export default defineConfig({
  root: __dirname, // 👈 This ensures Vite serves from javascript/app
  plugins: [react()],
  resolve: {
    alias: {
      '@michelangelo/core': path.resolve(__dirname, '../packages/core/src'),
      '@michelangelo/gen-k8s': path.resolve(__dirname, '../gen/grpc/k8s.io/'),
      '@michelangelo/gen-api': path.resolve(__dirname, '../gen/grpc/michelangelo/api/'),
    },
  },
});
