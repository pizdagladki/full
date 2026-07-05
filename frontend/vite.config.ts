import { existsSync, mkdirSync, readdirSync, readFileSync, statSync, copyFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import path from 'node:path';
import { defineConfig, type Connect, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';

// Local, CDN-free MediaPipe WASM serving — see src/cv/FaceLandmarkerRunner.ts.
// The `@mediapipe/tasks-vision` npm package ships a prebuilt `wasm/` directory (~33MB) that we
// do NOT want to commit to the repo. Instead: resolve it from node_modules and (a) serve it at
// `/mediapipe/wasm/*` in dev via a middleware, and (b) copy it into `dist/mediapipe/wasm` on
// build — so `FilesetResolver.forVisionTasks('/mediapipe/wasm')` always resolves to a same-origin
// static asset, in dev AND in the production build, with zero runtime CDN dependency.
const MEDIAPIPE_WASM_URL_PREFIX = '/mediapipe/wasm/';

function resolveMediapipeWasmDir(): string {
  // `@mediapipe/tasks-vision`'s package.json `exports` map does NOT list `./package.json`
  // as a resolvable subpath (only the main entry + individual wasm files), so resolve the
  // main entry point instead and derive the package directory from it.
  const require = createRequire(import.meta.url);
  const mainEntryPath = require.resolve('@mediapipe/tasks-vision');
  return path.join(path.dirname(mainEntryPath), 'wasm');
}

function mediapipeWasmPlugin(): Plugin {
  const wasmDir = resolveMediapipeWasmDir();
  let outDir = 'dist';
  let root = process.cwd();

  const serveWasm: Connect.NextHandleFunction = (req, res, next) => {
    const url = req.url ?? '';
    const requestedPath = decodeURIComponent(url.split('?')[0] ?? '');
    const resolved = path.join(wasmDir, requestedPath);
    if (!resolved.startsWith(wasmDir) || !existsSync(resolved) || !statSync(resolved).isFile()) {
      next();
      return;
    }
    if (resolved.endsWith('.wasm')) res.setHeader('Content-Type', 'application/wasm');
    res.end(readFileSync(resolved));
  };

  return {
    name: 'mediapipe-wasm-assets',
    configResolved(config) {
      outDir = config.build.outDir;
      root = config.root;
    },
    configureServer(server) {
      server.middlewares.use(MEDIAPIPE_WASM_URL_PREFIX, serveWasm);
    },
    closeBundle() {
      const destDir = path.join(root, outDir, 'mediapipe', 'wasm');
      mkdirSync(destDir, { recursive: true });
      for (const file of readdirSync(wasmDir)) {
        copyFileSync(path.join(wasmDir, file), path.join(destDir, file));
      }
    },
  };
}

export default defineConfig({
  plugins: [react(), mediapipeWasmPlugin()],
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
