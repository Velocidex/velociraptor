import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import viteCompression from 'vite-plugin-compression';

export default defineConfig({
    root: 'src',
    base: '/app',
    build: {
      // Relative to the root
      outDir: '../build',
      emptyOutDir: true,
      copyPublicDir: true,
      sourcemap: false,
    },
    css : {
      devSourcemap: false,
    },
    plugins: [
      // …
      react({
        // Use React plugin in all *.jsx and *.tsx files
        include: '**/*.{jsx,tsx}',
      }),
      viteCompression({
          filter: x=>{
              // Dont compress the main html page because it is used
              // by Go template.
              if (/html$/.test(x)) {
                  return false;
              }
              return true;
          },
          verbose: true,
          algorithm: 'brotliCompress',
          deleteOriginFile: true,
      }),
    ],
    server: {
      port: 3000,
      strictPort: true,
      proxy: {
        "/api": {
          target: "https://127.0.0.1:8889",
          changeOrigin: true,
          secure: false,
        },
      },
    },
    preview: {
      port: 3000,
      strictPort: true,
      proxy: {
        "/api": {
          target: "https://127.0.0.1:8889",
          changeOrigin: true,
          secure: false,
        },
      },
    },
})
