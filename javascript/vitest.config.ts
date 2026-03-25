// vitest.config.ts - Root configuration with projects
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: true, // Enable global Jest-like functions (describe, it, expect)
    silent: 'passed-only', // Clean output - only show failures
    env: {
      TZ: 'UTC',
    },
    projects: [
      {
        extends: true,

        test: {
          name: 'core',
          environment: 'jsdom', // Simulate a browser environment for React components
          include: ['packages/core/**/__tests__/**/*.{ts,tsx}'],
          setupFiles: ['./packages/core/test-setup.ts'],
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
      },
      {
        extends: true,

        test: {
          name: 'rpc',
          environment: 'node', // Node environment for RPC logic (no React components)
          include: ['packages/rpc/**/__tests__/**/*.{ts,tsx}'],
        },
      },
      {
        extends: true,

        test: {
          name: 'lint-rules',
          environment: 'node', // Node environment for ESLint rule logic
          include: ['eslint-local-rules/**/__tests__/**/*.{js,ts}'],
        },
      },
    ],
  },
});
