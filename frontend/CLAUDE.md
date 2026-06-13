# frontend

React frontend, its own ecosystem (outside the Go module). Stack:
- MediaPipe FaceLandmarker / EAR (eye-aspect-ratio) for face & blink detection.
- WebRTC P2P + STUN (TURN added later) for peer connections.
- canvas + MediaRecorder → WebM capture; Canvas/WebGL edit templates.
- Keep canvas / CV / WebRTC in isolated components behind refs.

Backend contract — the Go services under `services/` (HTTP `net/http` + WebSocket `gorilla/websocket`).
