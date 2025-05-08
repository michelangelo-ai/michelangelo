import react from '@vitejs/plugin-react';
import { defineConfig, mergeConfig } from 'vite';

export const baseConfig = defineConfig({
  root: __dirname,
  plugins: [react()],
});

export default defineConfig(() => {
  return mergeConfig(baseConfig, {
    resolve: {
      conditions: ['workspace'],
    },
  });
});
