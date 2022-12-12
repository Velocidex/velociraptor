import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

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
  ],
  server: {
    proxy: {
      "/api": {
        target: "https://0.0.0.0:8889",
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
