// vitest.config.ts
import path from 'path';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom', // Simulate a browser environment for React components
    globals: true, // Enable global Jest-like functions (describe, it, expect)
    include: ['**/__tests__/**/*.{ts,tsx}'],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),

      '@ma/gen-k8s': path.resolve(__dirname, '../../gen/grpc/k8s.io/'),
      '@ma/gen-api': path.resolve(__dirname, '../../gen/grpc/michelangelo/api/'),
    },
  },
});
