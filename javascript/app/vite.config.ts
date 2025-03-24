import react from '@vitejs/plugin-react';

import { defineConfig } from 'vite';
import path from 'path';

export default defineConfig({
  root: __dirname, // 👈 This ensures Vite serves from javascript/app
  plugins: [react()],
  resolve: {
    alias: {
      '@michelangelo/core': path.resolve(__dirname, '../packages/core/src'),
    },
  },
});
