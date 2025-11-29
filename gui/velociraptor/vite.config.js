import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import viteCompression from 'vite-plugin-compression';
import eslint from 'vite-plugin-eslint';

export default defineConfig({
    root: 'src',
    base: '/app',
    build: {
        // Relative to the root
        outDir: '../build',
        emptyOutDir: true,
        copyPublicDir: true,
        // We dont really care about this.
        chunkSizeWarningLimit: 10000000,
        sourcemap: false,
        rollupOptions: {
            onwarn(warning, warn) {
                // Pointless warning we cant do anything about.
                if (warning.code === "MODULE_LEVEL_DIRECTIVE") {
                    return;
                }
                warn(warning);
            },
        },
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

              // We implement brotli compression in the Go server
              // because this needs to be mutated - so we build it uncompressed
              // in the bundle.
              if (/css$/.test(x)) {
                  return false;
              }
              return true;
          },
          verbose: true,
          algorithm: 'brotliCompress',
          deleteOriginFile: true,
      }),
        // This adds significant time to build. Only enable sometimes.
        // eslint(),
    ],
    server: {
      port: 3000,
      // By default only bind to localhost for development.
      host: "127.0.0.1",
      // Sometimes it is useful to bind on all interfaces.
      // host: "0.0.0.0",
      strictPort: true,
      proxy: {
        "/api": {
          target: "https://127.0.0.1:8889",
          changeOrigin: true,
          secure: false,
        },
        "/debug": {
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
