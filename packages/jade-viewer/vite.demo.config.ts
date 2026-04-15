import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

/**
 * Separate Vite config for the interactive demo page.
 *
 * vite.config.ts runs in library build mode (rollupOptions.input,
 * externals, etc.) — that config is wrong for a dev server. This
 * config is a plain Vite app rooted at demo/, with the library
 * source imported directly from ../src so HMR works on component
 * edits.
 *
 * Run with: npm run demo
 */
export default defineConfig({
  root: resolve(__dirname, 'demo'),
  plugins: [react()],
  server: {
    open: true,
    port: 5178,
  },
  resolve: {
    alias: {
      // So the demo can import the published package name if it wants,
      // and still get hot-reloading edits to the library source.
      '@enoramlabs/jade-viewer': resolve(__dirname, 'src/index.ts'),
    },
  },
});
