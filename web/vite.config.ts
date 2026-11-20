// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Dev: `npm run dev` proxies /api to a running `braindb serve` (default port 7791).
// Build: output goes straight into the Go embed folder (internal/server/ui/dist).
export default defineConfig({
  base: './',
  plugins: [react()],
  server: {
    proxy: {
      '/api': { target: 'http://127.0.0.1:7791', changeOrigin: false },
    },
  },
  build: {
    outDir: '../internal/server/ui/dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
});
