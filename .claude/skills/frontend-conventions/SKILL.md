---
name: frontend-conventions
description: Canonical structure and conventions for this monorepo's React frontend, so the fleet can implement and fix frontend tasks.
user-invocable: false
---

> STARTER canon — the operator will refine it against the real `frontend/` structure as it grows. The CI `frontend` job runs `npm run lint` / `typecheck` / `test` in `frontend/` on Node 24; a frontend task is green only when all three pass.

# Frontend — canon for this monorepo

The frontend is a **React** app (TypeScript throughout) under `frontend/`, its own ecosystem (outside the Go module). It pairs with the backend services over HTTP (Echo) + WebSocket (`coder/websocket`).

## Stack
- **React** — app shell, routing, and ordinary feature UI (store, profile, results, landing) is plain, declarative React.
- **Computer vision**: MediaPipe **FaceLandmarker** / FaceMesh — local, per-frame, in-browser CV (e.g. EAR / eye-aspect-ratio for blink detection). Runs on every frame — never inside React render.
- **Realtime**: **WebRTC** P2P with **STUN** (TURN added later) for peer connections; signaling goes through the backend WS service.
- **Recording**: `canvas` + **MediaRecorder** (WebM) + `captureStream` for edit recording.
- **Edit templates**: HTML / Canvas / WebGL overlays rendered over the video.

## HARD RULE — isolate the imperative parts
Keep the heavy, imperative, per-frame machinery — **canvas, CV (MediaPipe), WebRTC, MediaRecorder** — in ISOLATED components accessed via **refs**, OUT of React's normal render / reconciliation path. React must NOT re-render every frame. These modules own their own `requestAnimationFrame` / media loops and expose imperative handles (start/stop/getResult) via `useImperativeHandle` + `ref`; the surrounding app stays declarative.

## Layout under `frontend/`
```
frontend/
├── src/
│   ├── features/        # ordinary React feature folders (landing, store, profile, results, battle, …)
│   ├── cv/              # MediaPipe FaceLandmarker / EAR — isolated, ref-driven, per-frame loop
│   ├── rtc/             # WebRTC peer connection + STUN/TURN + signaling client — isolated, ref-driven
│   ├── recording/       # canvas + MediaRecorder (WebM) + captureStream — isolated, ref-driven
│   ├── canvas/          # Canvas/WebGL edit templates rendered over the video — isolated, ref-driven
│   ├── ui/              # shared presentational components (declarative React)
│   ├── api/             # typed client for the backend HTTP/WS API
│   └── main.tsx · App.tsx
├── package.json         # MUST define `lint`, `typecheck`, `test` scripts (CI runs them)
├── eslint.config.js · .prettierrc · tsconfig.json · vitest.config.ts
└── package-lock.json    # committed; CI runs `npm ci`
```
The `cv/`, `rtc/`, `recording/`, `canvas/` modules are the ONLY place imperative/media code lives; everything else is declarative React.

## Tooling & quality gates
- **TypeScript throughout**; avoid untyped `any` in new code.
- A frontend task is **green only when** `npm run lint` (eslint), `npm run typecheck` (`tsc --noEmit`), and `npm run test` (vitest) all pass locally — exactly what the CI `frontend` job runs (Node 24, `npm ci`).
- `package.json` MUST keep the `lint`, `typecheck`, `test` scripts; run `make -C frontend lint typecheck test` (or the npm scripts) before opening/updating a PR.
- New `cv/` / `rtc/` / `recording/` / `canvas/` modules ship with at least a smoke/unit test of their imperative API.
