// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'jsdom', // Simulate a browser environment for React components
    globals: true, // Enable global Jest-like functions (describe, it, expect)
    include: ['**/__tests__/**/*.{ts,tsx}'],
    setupFiles: ['./test-setup.ts'],
    silent: 'passed-only',
    env: {
      TZ: 'UTC', // Force UTC timezone for all tests
    },
    deps: {
      optimizer: {
        web: {
          enabled: true,
          // BaseUI dnd-list appears to be bundled incorrectly according to vitest's
          // expectations. This is a workaround recommended by vite maintainers.
          //
          // Why this is only needed in test environment—vitest and vite bundle dependencies differently
          // https://github.com/vitest-dev/vitest/discussions/3221#discussioncomment-5675350

          // Proposed, and working solution:
          // https://github.com/vitest-dev/vitest/issues/4007#issuecomment-1691368010
          include: ['baseui/dnd-list'],
        },
      },
    },
  },
});
