import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { CvComponent, defaultCvRunner } from '../cv';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { WsClient } from '../api/ws';
import type { WsClientApi } from '../api/ws';

// ---------------------------------------------------------------------------
// WS message shapes (matchmaking protocol) — kept local, no untyped `any`.
// ---------------------------------------------------------------------------

interface JoinMsg {
  type: 'join';
  mode: string;
  level: number;
}

interface LeaveMsg {
  type: 'leave';
}

interface MatchedMsg {
  type: 'matched';
  room_id: string;
  opponent: unknown;
}

/** Shape of a WS frame we don't yet know — only `type` is guaranteed. */
interface UnknownServerMsg {
  type?: string;
  room_id?: string;
  opponent?: unknown;
}

/** location.state carried in by ModeSelect (#95, #159). */
interface SearchLocationState {
  mode?: string;
  level?: number;
  /** The TikTok-style track chosen on Home — threaded through to Battle as the selected edit audio. */
  trackId?: string;
}

const MATCHMAKING_WS_PATH = '/ws/match';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SearchProps {
  /** Injectable WS client (swap with a mock in tests). Defaults to a lazily-built WsClient. */
  wsClient?: WsClientApi;
  /** Injectable CV landmark runner (swap with a mock in tests). Defaults to the real
   * MediaPipe FaceLandmarker runner (`defaultCvRunner()`). */
  cvRunner?: LandmarkRunner;
  /** Fallback mode when none is carried via location.state. Defaults to 'ranked'. */
  mode?: string;
  /** Fallback level when none is carried via location.state. Defaults to 1. */
  level?: number;
  /** Fallback trackId when none is carried via location.state. Defaults to undefined. */
  trackId?: string;
}

// ---------------------------------------------------------------------------
// Search component — matchmaking search screen + face gate
// ---------------------------------------------------------------------------

export function Search({
  wsClient,
  cvRunner = defaultCvRunner(),
  mode: modeProp,
  level: levelProp,
  trackId: trackIdProp,
}: SearchProps) {
  const location = useLocation();
  const navigate = useNavigate();

  const locationState = (location.state as SearchLocationState | null) ?? null;
  const mode = locationState?.mode ?? modeProp ?? 'ranked';
  const level = locationState?.level ?? levelProp ?? 1;
  const trackId = locationState?.trackId ?? trackIdProp;

  // Lazily build the default WsClient once — never rebuilt on re-render.
  const wsRef = useRef<WsClientApi>();
  if (wsRef.current == null) {
    wsRef.current = wsClient ?? new WsClient();
  }

  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const cvRef = useRef<CvHandleRef>(null);

  // Refs (not state) gate the join — the cv/ws callbacks below are registered ONCE (CvComponent
  // builds its engine on first render; the mount effect below runs once) and would otherwise see
  // stale closures over state values.
  const facePresentRef = useRef(false);
  const wsOpenRef = useRef(false);
  const joinedRef = useRef(false);
  const teardownRef = useRef(false);

  // Drives rendering only (prompt vs. search-animation placeholder).
  const [facePresent, setFacePresent] = useState(false);

  const maybeJoin = useCallback(() => {
    if (joinedRef.current) return;
    if (!wsOpenRef.current || !facePresentRef.current) return;
    const msg: JoinMsg = { type: 'join', mode, level };
    wsRef.current?.send(JSON.stringify(msg));
    joinedRef.current = true;
    // facePresent is already true (that's how the gate opened) — reaffirmed for clarity, this is
    // also the flag that flips the UI into the "searching" (search-animation) state.
    setFacePresent(true);
  }, [mode, level]);

  const teardown = useCallback((sendLeave: boolean) => {
    if (teardownRef.current) return;
    teardownRef.current = true;
    if (sendLeave && joinedRef.current) {
      try {
        const msg: LeaveMsg = { type: 'leave' };
        wsRef.current?.send(JSON.stringify(msg));
      } catch {
        // ignore — the WS may already be closed/unavailable
      }
    }
    wsRef.current?.close();
  }, []);

  const onFacePresent = useCallback(() => {
    facePresentRef.current = true;
    setFacePresent(true);
    maybeJoin();
    // Intentionally stable ([]): CvComponent freezes this callback on first render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onFaceLost = useCallback(() => {
    teardown(true);
    navigate('/home');
    // Intentionally stable ([]): CvComponent freezes this callback on first render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const cvCallbacks = useMemo<CvCallbacks>(
    () => ({ onFacePresent, onFaceLost }),
    // Intentionally stable ([]): passed to CvComponent, which builds its engine ONCE.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const handleMessage = useCallback(
    (data: string) => {
      try {
        const msg = JSON.parse(data) as UnknownServerMsg;
        // Require a real room id before navigating — a malformed {type:'matched'} frame with no
        // (or empty) room_id must NOT send the player to a battle screen with roomId: undefined;
        // it's ignored and the player stays on the search-animation placeholder.
        if (msg.type === 'matched' && typeof msg.room_id === 'string' && msg.room_id) {
          const matched = msg as MatchedMsg;
          teardown(false);
          // Criterion 4 (#159): trackId (the edit-audio track chosen on Home) is forwarded through
          // to Battle unchanged — Search has no use for it itself.
          navigate('/battle', {
            state: { roomId: matched.room_id, opponent: matched.opponent, trackId },
          });
        }
      } catch {
        // ignore malformed WS frames
      }
    },
    [teardown, navigate, trackId],
  );

  // Mount effect — open the matchmaking WS and start the CV loop. In production this runs exactly
  // once; under React.StrictMode (dev) it runs mount → cleanup → mount on the same fiber, so the
  // per-connection gate refs are reset to a clean slate HERE (not just at declaration) — otherwise
  // the StrictMode-only cleanup's teardown(true) latches teardownRef=true permanently, and the
  // REAL later unmount's teardown() early-returns: the second connection's `leave` is never sent
  // and its socket is never closed (a ghost queue entry — violates criterion 4).
  useEffect(() => {
    // Reset the gate refs only — NOT the `facePresent` render state. `facePresent` can only ever
    // flip true via the async, RAF-driven onFacePresent callback, which cannot fire inside this
    // synchronous mount→cleanup→mount window, so it is still (and can only be) `false` here; no
    // setState call is needed (or safe to make synchronously inside an effect body).
    teardownRef.current = false;
    wsOpenRef.current = false;
    joinedRef.current = false;
    facePresentRef.current = false;

    const ws = wsRef.current!;
    ws.connect(MATCHMAKING_WS_PATH);
    ws.onOpen(() => {
      wsOpenRef.current = true;
      maybeJoin();
    });
    ws.onMessage(handleMessage);
    if (videoRef.current && cvRef.current) {
      cvRef.current.start(videoRef.current);
    }
    return () => teardown(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Separate guarded camera-preview effect (mirrors Home.tsx) — never blocks cv.start on the
  // stream; no-crash if getUserMedia is unavailable.
  useEffect(() => {
    if (!navigator.mediaDevices?.getUserMedia) return;
    let cancelled = false;

    navigator.mediaDevices
      .getUserMedia({ video: true })
      .then((stream) => {
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }
        streamRef.current = stream;
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
        }
      })
      .catch(() => {
        // getUserMedia failed — preview stays blank, no crash
      });

    return () => {
      cancelled = true;
      if (streamRef.current) {
        streamRef.current.getTracks().forEach((t) => t.stop());
        streamRef.current = null;
      }
    };
  }, []);

  return (
    <div data-testid="search-screen">
      <CvComponent ref={cvRef} runner={cvRunner} callbacks={cvCallbacks} />
      <video ref={videoRef} autoPlay muted playsInline data-testid="search-preview" />
      {!facePresent ? (
        <div data-testid="face-prompt">Show your face to start searching</div>
      ) : (
        <div data-testid="search-animation">Searching for an opponent…</div>
      )}
    </div>
  );
}
