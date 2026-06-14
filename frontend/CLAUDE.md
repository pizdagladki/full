# frontend

React frontend, its own ecosystem (outside the Go module). Stack:
- MediaPipe FaceLandmarker / EAR (eye-aspect-ratio) for face & blink detection.
- WebRTC P2P + STUN (TURN added later) for peer connections.
- canvas + MediaRecorder → WebM capture; Canvas/WebGL edit templates.
- Keep canvas / CV / WebRTC in isolated components behind refs.

## Commands (from this folder)
- `npm ci` (install from the committed lockfile), then `make lint` / `make typecheck` / `make test`
  (eslint + `tsc --noEmit` + vitest), `make format` (prettier). `make help` lists targets.

Backend contract — the Go services under `services/` (HTTP `Echo` + WebSocket `coder/websocket`).
