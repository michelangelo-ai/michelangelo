import react from '@vitejs/plugin-react';
import path from 'path';
import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    lib: {
      entry: {
        'michelangelo-core': path.resolve(__dirname, 'index.tsx'),
        primitives: path.resolve(__dirname, 'primitives.tsx'),
      },
      formats: ['es', 'cjs'],
      fileName: (format, entryName) => `${entryName}.${format === 'es' ? 'js' : 'cjs'}`,
    },
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        'react-router-dom',
        'react-router-dom-v5-compat',
        '@bufbuild/protobuf',
        '@connectrpc/connect',
        '@connectrpc/connect-web',
        'pluralize',
        'styletron-react',
        'styletron-engine-atomic',
        '@tanstack/react-query',
        /^baseui(\/.*)?$/,
      ],
    },
    commonjsOptions: {
      include: ['styletron-react', /node_modules/],
      esmExternals: true,
    },
    outDir: 'dist',
    emptyOutDir: true,
  },
  optimizeDeps: {
    include: ['styletron-react'],
  },
  plugins: [react()],
});
