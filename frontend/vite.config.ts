import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/v1/auth': { target: 'http://localhost:8080', changeOrigin: true },
      '/v1/clips': { target: 'http://localhost:8082', changeOrigin: true },
      '/v1/king-clips': { target: 'http://localhost:8082', changeOrigin: true },
      '/v1/matches': { target: 'http://localhost:8084', changeOrigin: true },
      '/v1/ratings': { target: 'http://localhost:8084', changeOrigin: true },
      '/v1/store': { target: 'http://localhost:8083', changeOrigin: true },
      '/v1/points': { target: 'http://localhost:8083', changeOrigin: true },
      '/v1/koth': { target: 'http://localhost:8087', changeOrigin: true },
      '/v1/reports': { target: 'http://localhost:8085', changeOrigin: true },
      '/v1/health': { target: 'http://localhost:8088', changeOrigin: true },
      // Matchmaking search WS — distinct path from the signaling WS below.
      '/ws/match': {
        target: 'http://localhost:8086',
        ws: true,
        rewrite: (path) => path.replace(/^\/ws\/match/, '/ws'),
      },
      // Signaling (battle arbitration / invite-a-friend) WS.
      '/ws/signal': {
        target: 'http://localhost:8081',
        ws: true,
        rewrite: (path) => path.replace(/^\/ws\/signal/, '/ws'),
      },
    },
  },
});
