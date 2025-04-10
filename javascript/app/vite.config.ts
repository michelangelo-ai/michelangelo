import react from '@vitejs/plugin-react';

import { defineConfig } from 'vite';
import path from 'path';

export default defineConfig({
  root: __dirname, // 👈 This ensures Vite serves from javascript/app
  plugins: [react()],
  resolve: {
    alias: {
      // Multiple aliases are required for the core package, such that
      // the core package can import from itself using "@" and app can
      // import from the core package using "@michelangelo/core"
      '@': path.resolve(__dirname, '../packages/core/src'),
      '@michelangelo/core': path.resolve(__dirname, '../packages/core/src'),

      '@ma/gen-k8s': path.resolve(__dirname, '../gen/grpc/k8s.io/'),
      '@ma/gen-api': path.resolve(__dirname, '../gen/grpc/michelangelo/api/'),
    },
  },
});
