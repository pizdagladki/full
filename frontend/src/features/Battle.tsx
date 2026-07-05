import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ForwardRefExoticComponent, RefAttributes } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { CvComponent } from '../cv';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { RtcComponent } from '../rtc';
import type { PcFactory, RtcHandle, WsFactory } from '../rtc';
import { WsClient } from '../api/ws';
import type { WsClientApi } from '../api/ws';
import { useAuth } from './auth';

// ---------------------------------------------------------------------------
// WS message shapes (server-side time-arbitration protocol) — kept local, no
// untyped `any`. The server routes by room_id, so outbound frames include it.
// ---------------------------------------------------------------------------

interface BlinkMsg {
  type: 'blink';
  room_id: string;
}

interface FaceLostMsg {
  type: 'face_lost';
  room_id: string;
}

/** Shape of a WS frame we don't yet know — only `type` is guaranteed. */
interface UnknownServerMsg {
  type?: string;
  winner_id?: unknown;
  loser_id?: unknown;
}

/**
 * location.state carried in by Search (`navigate('/battle', { state: { roomId, opponent } })`) or
 * by InviteRoom's private-room flow (`navigate('/battle', { state: { roomId, ranked: false } })`).
 * `ranked` defaults to `true` when absent so Search's existing ranked path — which never sets it —
 * keeps working untouched (criterion 2, #106: the invite-a-friend room is the UNRANKED branch).
 */
interface BattleLocationState {
  roomId?: string;
  opponent?: unknown;
  ranked?: boolean;
}

const ARBITRATION_WS_PATH = '/ws/signal';
const SIGNALING_URL = `${(import.meta.env?.VITE_WS_URL as string | undefined) ?? ''}/ws/signal`;

const SANITY_MS_DEFAULT = 2000;
const COUNTDOWN_SECONDS_DEFAULT = 5;

// TODO: wire real MediaPipe FaceLandmarker runner (separate task). Until then this placeholder
// always reports NO face — the honest-scaffold approach also used by Search/Home's placeholders:
// it keeps the face gate truthful (never fakes a pass) rather than pretending CV is wired up
// before it actually is.
const PLACEHOLDER_RUNNER: LandmarkRunner = {
  detectForVideo: () => ({ faceLandmarks: [] }),
};

type Phase = 'sanity' | 'countdown' | 'battle' | 'done';

/** Structural type of CvComponent's props/ref — used for the test-injection seam below. */
type CvComponentType = ForwardRefExoticComponent<
  { runner: LandmarkRunner; callbacks?: CvCallbacks } & RefAttributes<CvHandleRef>
>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface BattleProps {
  /** Injectable arbitration WS client (swap with a mock in tests). Defaults to a lazily-built WsClient. */
  wsClient?: WsClientApi;
  /** Injectable CV landmark runner (swap with a mock in tests). Defaults to the placeholder. */
  cvRunner?: LandmarkRunner;
  /** Injectable WS factory for RtcComponent's signaling socket (swap with a mock in tests). */
  rtcWsFactory?: WsFactory;
  /** Injectable RTCPeerConnection factory for RtcComponent (swap with a mock in tests). */
  rtcPcFactory?: PcFactory;
  /** Overrides useAuth().user?.id — lets tests skip mounting AuthProvider. */
  currentUserId?: string;
  /** Sanity-check duration (ms) run before the countdown. Defaults to the criterion's 2000ms. */
  sanityMs?: number;
  /** Countdown length (seconds) run after the sanity check. Defaults to the criterion's 5s. */
  countdownSeconds?: number;
  /**
   * Media-negotiation role is NOT part of this scaffold's tested criteria — a real offerer
   * election needs a deterministic tie-break using both peers' ids from the server. Fixed `true`
   * default keeps behavior deterministic without over-engineering a rule nothing here exercises.
   */
  isOfferer?: boolean;
  /**
   * Test seam ONLY: overrides which component mounts the CV engine. Production never sets this —
   * it always defaults to the real `CvComponent`. Driving a real blink through CvEngine's EAR/
   * calibration math under fake timers is fiddly; Battle.test.tsx substitutes a fake here so it can
   * fire onFacePresent/onBlink/onFaceLost on demand instead. The modules are still mounted via
   * refs in production — this only swaps the ref target in tests.
   */
  cvComponent?: CvComponentType;
}

// ---------------------------------------------------------------------------
// Battle component — split view (local | remote), sanity check, countdown,
// then live battle wiring cv/ blink+face-loss to the arbitration WS.
// ---------------------------------------------------------------------------

export function Battle({
  wsClient,
  cvRunner = PLACEHOLDER_RUNNER,
  rtcWsFactory,
  rtcPcFactory,
  currentUserId: currentUserIdProp,
  sanityMs = SANITY_MS_DEFAULT,
  countdownSeconds = COUNTDOWN_SECONDS_DEFAULT,
  isOfferer = true,
  cvComponent: Cv = CvComponent,
}: BattleProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  const locationState = (location.state as BattleLocationState | null) ?? null;
  const roomId = locationState?.roomId ?? '';
  // Criterion 2 (#106): defaults to ranked (`true`) when absent so Search's existing ranked path
  // (which never sets `ranked`) is unaffected.
  const ranked = locationState?.ranked ?? true;
  const currentUserId = currentUserIdProp ?? user?.id;

  // Lazily build the default WsClient once — never rebuilt on re-render.
  const wsRef = useRef<WsClientApi>();
  if (wsRef.current == null) {
    wsRef.current = wsClient ?? new WsClient();
  }

  const localVideoRef = useRef<HTMLVideoElement>(null);
  const remoteVideoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const cvRef = useRef<CvHandleRef>(null);
  const rtcRef = useRef<RtcHandle>(null);

  // Refs (not state) drive decisions read from callbacks registered once at mount — state would
  // be stale inside those frozen closures.
  const facePresentRef = useRef(false);
  const phaseRef = useRef<Phase>('sanity');
  const teardownRef = useRef(false);
  const startTimeRef = useRef(0);
  const currentUserIdRef = useRef(currentUserId);
  const sanityTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const countdownTimerRef = useRef<ReturnType<typeof setInterval>>();

  // Drives rendering only.
  const [phase, setPhase] = useState<Phase>('sanity');
  const [countdown, setCountdown] = useState(countdownSeconds);

  useEffect(() => {
    currentUserIdRef.current = currentUserId;
  }, [currentUserId]);

  const teardown = useCallback(() => {
    if (teardownRef.current) return;
    teardownRef.current = true;
    if (sanityTimerRef.current) clearTimeout(sanityTimerRef.current);
    if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
    wsRef.current?.close();
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
  }, []);

  const routeToResults = useCallback(
    (result: 'win' | 'loss', durationMs: number, winnerId?: number, loserId?: number) => {
      phaseRef.current = 'done';
      setPhase('done');
      teardown();
      // Criterion 2 (#106): an unranked room (e.g. InviteRoom's invite-a-friend flow) must not
      // affect rating/ELO — so winner_id/loser_id (the ids any rating/ELO update would key off of)
      // are deliberately dropped from the /results hand-off when `ranked` is false. `ranked` itself
      // is always forwarded so /results can also skip any rating UI/update for this match.
      navigate('/results', {
        state: ranked ? { result, durationMs, winnerId, loserId, ranked } : { result, durationMs, ranked },
      });
    },
    [teardown, navigate, ranked],
  );

  const startCountdown = useCallback(() => {
    phaseRef.current = 'countdown';
    setPhase('countdown');
    let remaining = countdownSeconds;
    setCountdown(remaining);
    countdownTimerRef.current = setInterval(() => {
      remaining -= 1;
      if (remaining <= 0) {
        if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
        phaseRef.current = 'battle';
        startTimeRef.current = Date.now();
        setPhase('battle');
      } else {
        setCountdown(remaining);
      }
    }, 1000);
    // Intentionally stable ([]): registered once from the mount effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onFacePresent = useCallback(() => {
    facePresentRef.current = true;
  }, []);

  const onFaceLost = useCallback(() => {
    facePresentRef.current = false;
    // Criterion 5: leaving the camera AFTER start is reported as a forfeit. Before start, losing
    // the face just fails the sanity check (handled by the sanity timer reading facePresentRef).
    if (phaseRef.current === 'battle') {
      const msg: FaceLostMsg = { type: 'face_lost', room_id: roomId };
      wsRef.current?.send(JSON.stringify(msg));
    }
    // Intentionally stable ([]): CvComponent freezes this callback on first render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onBlink = useCallback(() => {
    if (phaseRef.current === 'battle') {
      const msg: BlinkMsg = { type: 'blink', room_id: roomId };
      wsRef.current?.send(JSON.stringify(msg));
    }
    // Intentionally stable ([]): CvComponent freezes this callback on first render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const cvCallbacks = useMemo<CvCallbacks>(
    () => ({ onFacePresent, onFaceLost, onBlink }),
    // Intentionally stable ([]): passed to CvComponent, which builds its engine ONCE.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const handleMessage = useCallback(
    (data: string) => {
      try {
        const msg = JSON.parse(data) as UnknownServerMsg;
        // Authoritative outcome: winner_id/loser_id are numbers (int64 server-side); User.id is a
        // string, so the win/loss check compares String(winner_id) against the current user id.
        if (
          msg.type === 'outcome' &&
          typeof msg.winner_id === 'number' &&
          typeof msg.loser_id === 'number'
        ) {
          const result: 'win' | 'loss' =
            String(msg.winner_id) === currentUserIdRef.current ? 'win' : 'loss';
          const durationMs = Date.now() - startTimeRef.current;
          routeToResults(result, durationMs, msg.winner_id, msg.loser_id);
        }
      } catch {
        // ignore malformed WS frames
      }
    },
    [routeToResults],
  );

  // Mount effect — connects the arbitration WS, starts the cv engine, and arms the sanity-check
  // timer. In production this runs exactly once; under React.StrictMode (dev) it runs
  // mount → cleanup → mount on the same fiber, so the gate refs are reset to a clean slate HERE
  // (not just at declaration) — mirrors Search's StrictMode-safe pattern.
  useEffect(() => {
    // Reset the gate refs only — NOT the `phase`/`countdown` render state. Neither can have
    // advanced synchronously between a StrictMode mount and its synthetic cleanup (both are only
    // ever mutated from an async timer/callback), so re-setting them here would just be a
    // redundant synchronous setState inside an effect body.
    teardownRef.current = false;
    facePresentRef.current = false;
    phaseRef.current = 'sanity';

    const ws = wsRef.current!;
    ws.connect(ARBITRATION_WS_PATH);
    ws.onMessage(handleMessage);

    if (localVideoRef.current && cvRef.current) {
      cvRef.current.start(localVideoRef.current);
    }

    // Criterion 2 — a 2-second sanity check runs BEFORE the countdown; if no face is present it
    // cancels the battle as a loss (routes to results, no countdown ever renders).
    sanityTimerRef.current = setTimeout(() => {
      if (facePresentRef.current) {
        startCountdown();
      } else {
        routeToResults('loss', 0);
      }
    }, sanityMs);

    return () => teardown();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Separate guarded camera-preview effect (mirrors Search/Home) — never blocks cv.start on the
  // stream; no-crash if getUserMedia is unavailable. Once the local stream is ready, connect rtc.
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
        if (localVideoRef.current) {
          localVideoRef.current.srcObject = stream;
        }
        if (rtcRef.current) {
          // onRemoteStream must be registered AFTER connect() — connect() is what actually builds
          // the underlying RtcPeerImpl; registering it any earlier (e.g. in the mount effect,
          // before a local stream exists) would be a no-op against a peer that doesn't exist yet.
          rtcRef.current.connect({ room_id: roomId, localStream: stream });
          rtcRef.current.onRemoteStream((remoteStream) => {
            if (remoteVideoRef.current) {
              remoteVideoRef.current.srcObject = remoteStream;
            }
          });
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
  }, [roomId]);

  return (
    <div data-testid="battle-screen">
      <Cv ref={cvRef} runner={cvRunner} callbacks={cvCallbacks} />
      <RtcComponent
        ref={rtcRef}
        signalingUrl={SIGNALING_URL}
        isOfferer={isOfferer}
        wsFactory={rtcWsFactory}
        pcFactory={rtcPcFactory}
      />
      <div data-testid="battle-split">
        <video ref={localVideoRef} autoPlay muted playsInline data-testid="local-video" />
        <video ref={remoteVideoRef} autoPlay playsInline data-testid="remote-video" />
      </div>
      {phase === 'sanity' && <div data-testid="sanity-check">Checking for your face…</div>}
      {phase === 'countdown' && <div data-testid="countdown">{countdown}</div>}
      {phase === 'battle' && <div data-testid="battle-live">Battle!</div>}
    </div>
  );
}
