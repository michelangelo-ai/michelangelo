import react from '@vitejs/plugin-react';
import { execFileSync } from 'node:child_process';
import { defineConfig, mergeConfig } from 'vite';

import type { Plugin } from 'vite';

// Dev-only: exposes the developer's own `git config user.email` so the sandbox identity override
// (see app/dev-profile.ts) can use a real address instead of GitHub's noreply fallback, since most
// GitHub profiles don't expose a public email. `apply: 'serve'` keeps this out of `vite build`
// entirely, so it never reaches the sandbox's built/served bundle.
function devGitEmailPlugin(): Plugin {
  return {
    name: 'dev-git-email',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use('/__dev/git-email', (_req, res) => {
        try {
          const email = execFileSync('git', ['config', 'user.email'], { encoding: 'utf-8' }).trim();
          res.statusCode = email ? 200 : 404;
          res.end(email);
        } catch {
          res.statusCode = 404;
          res.end();
        }
      });
    },
  };
}

export const baseConfig = defineConfig({
  root: __dirname,
  plugins: [react(), devGitEmailPlugin()],
});

export default defineConfig(() => {
  return mergeConfig(baseConfig, {
    resolve: {
      conditions: ['workspace'],
    },
  });
});
