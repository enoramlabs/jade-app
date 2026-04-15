import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

/**
 * Force decode-named-character-reference to its pure-JS (Node) entry
 * point instead of its browser entry. The browser entry evaluates
 * document.createElement("i") at module init and throws in Next.js
 * static generation / any non-browser JS runtime.
 *
 * The package ships two implementations via conditional exports:
 *   "browser": "./index.dom.js"   // uses document.createElement
 *   "default": "./index.js"       // pure-JS lookup table
 * Vite's library-build resolver prefers the browser condition, so we
 * intercept the import at the top-level Vite plugin layer (enforce:
 * 'pre' runs before Vite's built-in resolver) and redirect it to the
 * absolute path of index.js, bypassing the exports map entirely.
 * Works identically in browser and Node — just a static lookup.
 */
function forceSSRSafeDecoder(): Plugin {
  return {
    name: 'force-ssr-safe-decode-named-char-ref',
    enforce: 'pre',
    resolveId(source) {
      if (source === 'decode-named-character-reference') {
        return resolve(
          __dirname,
          'node_modules/decode-named-character-reference/index.js',
        );
      }
      return null;
    },
  };
}

export default defineConfig({
  plugins: [forceSSRSafeDecoder(), react()],
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'JadeViewer',
      formats: ['es', 'cjs'],
      fileName: (format) => (format === 'es' ? 'index.js' : 'index.cjs'),
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react/jsx-runtime'],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
        },
      },
    },
    sourcemap: true,
    emptyOutDir: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./__tests__/setup.ts'],
  },
});
