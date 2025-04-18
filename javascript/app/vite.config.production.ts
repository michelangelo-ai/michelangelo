import { defineConfig, mergeConfig } from 'vite';

import { baseConfig } from './vite.config';

export default defineConfig(() => {
  return mergeConfig(baseConfig, {
    mode: 'production',
    resolve: {
      conditions: ['production'],
    },
  });
});
