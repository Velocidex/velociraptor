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
    define: {
        'process.env.NODE_ENV': JSON.stringify(process.env.NODE_ENV),
        'process.env.MY_ENV': JSON.stringify(process.env.MY_ENV),
    },
    css : {
      // Creates inline source maps which don't work with Chrome (108)
      // dev tools Coverage analysis. Rather use build.sourcemap:true
      // if you need to do Coverage analysis.
      devSourcemap: false,
    },
    resolve: {
        preserveSymlinks: true,
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
        "/notebooks": {
          target: "https://127.0.0.1:8889",
          changeOrigin: true,
          secure: false,
        },
        "/downloads": {
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
});
