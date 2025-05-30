// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom', // Simulate a browser environment for React components
    globals: true, // Enable global Jest-like functions (describe, it, expect)
    include: ['**/__tests__/**/*.{ts,tsx}'],
    setupFiles: ['./test-setup.ts'],
  },
});
