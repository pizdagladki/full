import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ComponentType, ForwardRefExoticComponent, MutableRefObject, RefAttributes } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { CvComponent, defaultCvRunner } from '../cv';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { RtcComponent } from '../rtc';
import type { PcFactory, RtcHandle, WsFactory } from '../rtc';
import { RecordingComponent, submitWinClip } from '../recording';
import type { RecordingComponentProps, RecordingHandle } from '../recording';
import { WsClient } from '../api/ws';
import type { WsClientApi } from '../api/ws';
import { defaultClipsApi } from '../api/clips';
import type { ClipsApi } from '../api/clips';
import { useAuth } from './auth';
import { Distraction, makeEmptyBattleMeta } from './Distraction';
import type { BattleMeta, DistractionProps } from './Distraction';
import { readLocal } from '../utils/storage';

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
 * location.state carried in by Search (`navigate('/battle', { state: { roomId, opponent, trackId } })`)
 * or by InviteRoom's private-room flow (`navigate('/battle', { state: { roomId, ranked: false } })`).
 * `ranked` defaults to `true` when absent so Search's existing ranked path — which never sets it —
 * keeps working untouched (criterion 2, #106: the invite-a-friend room is the UNRANKED branch).
 * `trackId` (#159, criterion 4) is the TikTok-style track chosen on Home — carried end-to-end
 * (Home → ModeSelect → Search → Battle) as the selected edit audio for a win clip.
 */
interface BattleLocationState {
  roomId?: string;
  opponent?: unknown;
  ranked?: boolean;
  trackId?: string;
}

const ARBITRATION_WS_PATH = '/ws/signal';
const SIGNALING_URL = `${(import.meta.env?.VITE_WS_URL as string | undefined) ?? ''}/ws/signal`;

const SANITY_MS_DEFAULT = 2000;
const COUNTDOWN_SECONDS_DEFAULT = 5;

type Phase = 'sanity' | 'countdown' | 'battle' | 'win-edit' | 'loss-edit' | 'done';

/** Structural type of CvComponent's props/ref — used for the test-injection seam below. */
type CvComponentType = ForwardRefExoticComponent<
  { runner: LandmarkRunner; callbacks?: CvCallbacks } & RefAttributes<CvHandleRef>
>;

/** Structural type of RecordingComponent's props/ref — used for the test-injection seam below. */
type RecordingComponentType = ForwardRefExoticComponent<
  RecordingComponentProps & RefAttributes<RecordingHandle>
>;

/** Structural type of Distraction's props — used for the test-injection seam below. */
type DistractionComponentType = ComponentType<DistractionProps>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface BattleProps {
  /** Injectable arbitration WS client (swap with a mock in tests). Defaults to a lazily-built WsClient. */
  wsClient?: WsClientApi;
  /** Injectable CV landmark runner (swap with a mock in tests). Defaults to the real
   * MediaPipe FaceLandmarker runner (`defaultCvRunner()`). */
  cvRunner?: LandmarkRunner;
  /** Injectable WS factory for RtcComponent's signaling socket (swap with a mock in tests). */
  rtcWsFactory?: WsFactory;
  /** Injectable RTCPeerConnection factory for RtcComponent (swap with a mock in tests). */
  rtcPcFactory?: PcFactory;
  /** Injectable win-clip upload/convert API (swap with a mock in tests). Defaults to the real
   * `ClipsApiClient` (`defaultClipsApi`). */
  clipsApi?: ClipsApi;
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
  /**
   * Test seam ONLY: overrides which component mounts the recording engine — mirrors `cvComponent`
   * above. Production never sets this — it always defaults to the real `RecordingComponent`.
   */
  recordingComponent?: RecordingComponentType;
  /**
   * Test seam (#160): overrides the battle-meta ref Battle passes to `Distraction`, so a test can
   * inject its own ref and assert `ref.current.distractions` after applying a distraction.
   * Production never sets this — Battle builds its own ref internally, once, like `wsRef`.
   */
  battleMetaRef?: MutableRefObject<BattleMeta>;
  /**
   * Test seam ONLY: overrides which component mounts the distraction control — mirrors
   * `cvComponent`/`recordingComponent` above. Production never sets this — it always defaults to
   * the real `Distraction`. Lets a test inject a spy that records the exact props (e.g.
   * `battleStartMs`) Battle rendered it with, without depending on Distraction's own timer math.
   */
  distractionComponent?: DistractionComponentType;
}

// ---------------------------------------------------------------------------
// Battle component — split view (local | remote), sanity check, countdown,
// then live battle wiring cv/ blink+face-loss to the arbitration WS.
// ---------------------------------------------------------------------------

export function Battle({
  wsClient,
  cvRunner = defaultCvRunner(),
  rtcWsFactory,
  rtcPcFactory,
  clipsApi = defaultClipsApi,
  currentUserId: currentUserIdProp,
  sanityMs = SANITY_MS_DEFAULT,
  countdownSeconds = COUNTDOWN_SECONDS_DEFAULT,
  isOfferer = true,
  cvComponent: Cv = CvComponent,
  recordingComponent: Recording = RecordingComponent,
  battleMetaRef: battleMetaRefProp,
  distractionComponent: DistractionCmp = Distraction,
}: BattleProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  const locationState = (location.state as BattleLocationState | null) ?? null;
  const roomId = locationState?.roomId ?? '';
  // Criterion 2 (#106): defaults to ranked (`true`) when absent so Search's existing ranked path
  // (which never sets `ranked`) is unaffected.
  const ranked = locationState?.ranked ?? true;
  // Criterion 4 (#159): the TikTok-style track chosen on Home, threaded through ModeSelect/Search.
  // Carried through to the win-clip flow below and forwarded to /results as the selected edit
  // audio; `undefined` when absent (e.g. a direct/invite-room entry with no track selection).
  const trackId = locationState?.trackId;
  const currentUserId = currentUserIdProp ?? user?.id;

  // Lazily build the default WsClient once — never rebuilt on re-render.
  const wsRef = useRef<WsClientApi>();
  if (wsRef.current == null) {
    wsRef.current = wsClient ?? new WsClient();
  }

  // Criterion 2 (#160): Battle owns a single battle-meta object — built once (like `wsRef` above),
  // never recreated on re-render — passed to `Distraction` so the SAME object the recording/
  // sharing layer would later consume accumulates every applied distraction. When a test injects
  // its own `battleMetaRef` prop, that ref object is used as-is instead of building an internal one.
  const ownBattleMetaRef = useRef<BattleMeta>();
  if (ownBattleMetaRef.current == null) {
    ownBattleMetaRef.current = makeEmptyBattleMeta();
  }
  const battleMetaRef: MutableRefObject<BattleMeta> =
    battleMetaRefProp ?? (ownBattleMetaRef as MutableRefObject<BattleMeta>);

  const localVideoRef = useRef<HTMLVideoElement>(null);
  const remoteVideoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const cvRef = useRef<CvHandleRef>(null);
  const rtcRef = useRef<RtcHandle>(null);
  const recordingRef = useRef<RecordingHandle>(null);

  // Refs (not state) drive decisions read from callbacks registered once at mount — state would
  // be stale inside those frozen closures.
  const facePresentRef = useRef(false);
  const phaseRef = useRef<Phase>('sanity');
  const teardownRef = useRef(false);
  const startTimeRef = useRef(0);
  const currentUserIdRef = useRef(currentUserId);
  const sanityTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const countdownTimerRef = useRef<ReturnType<typeof setInterval>>();
  // Criterion 1 (#159): guards the ring buffer so it starts exactly once — the local stream
  // resolves asynchronously (getUserMedia), so this fires from BOTH the countdown→battle
  // transition and the stream-ready callback, whichever settles last.
  const ringBufferStartedRef = useRef(false);
  // Criterion 2 (#159): captures the routeToResults closure for the loss/skip-edit Skip button —
  // set synchronously when entering 'loss-edit', read on click.
  const skipEditRef = useRef<() => void>(() => {});

  // Drives rendering only.
  const [phase, setPhase] = useState<Phase>('sanity');
  const [countdown, setCountdown] = useState(countdownSeconds);
  // Criterion 1 (#160): mirrors `startTimeRef.current` into render state — a ref's `.current` must
  // never be read during render (only inside effects/callbacks), so this is what's actually passed
  // to `Distraction` as `battleStartMs` below.
  const [battleStartMs, setBattleStartMs] = useState(0);

  useEffect(() => {
    currentUserIdRef.current = currentUserId;
  }, [currentUserId]);

  const teardown = useCallback(() => {
    if (teardownRef.current) return;
    teardownRef.current = true;
    if (sanityTimerRef.current) clearTimeout(sanityTimerRef.current);
    if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
    wsRef.current?.close();
    recordingRef.current?.stop();
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
  }, []);

  const routeToResults = useCallback(
    (result: 'win' | 'loss', durationMs: number, winnerId?: number, loserId?: number, mp4Url?: string) => {
      phaseRef.current = 'done';
      setPhase('done');
      teardown();
      // Criterion 2 (#106): an unranked room (e.g. InviteRoom's invite-a-friend flow) must not
      // affect rating/ELO — so winner_id/loser_id (the ids any rating/ELO update would key off of)
      // are deliberately dropped from the /results hand-off when `ranked` is false. `ranked` itself
      // is always forwarded so /results can also skip any rating UI/update for this match.
      // Criterion 3 (#159): a resolved win-clip `mp4Url` is shareable regardless of ranked status,
      // so it's included in BOTH branches (never gated by `ranked`).
      // Criterion 4 (#159): `trackId` (the edit audio chosen on Home) is likewise always forwarded —
      // the recording engine (#52) currently mixes in a placeholder oscillator track (no seam to
      // inject a real one yet), so `trackId` is carried through purely as the selection that WOULD
      // drive the win clip's audio once a real track-audio seam exists.
      navigate('/results', {
        state: ranked
          ? { result, durationMs, winnerId, loserId, ranked, mp4Url, trackId }
          : { result, durationMs, ranked, mp4Url, trackId },
      });
    },
    [teardown, navigate, ranked, trackId],
  );

  // Criterion 1 (#159): starts the ring buffer exactly once — only once BOTH the battle phase has
  // begun AND the local stream is available. Called from the countdown→battle transition below
  // AND from the getUserMedia resolution effect, whichever settles last.
  const maybeStartRingBuffer = useCallback(() => {
    if (ringBufferStartedRef.current) return;
    if (phaseRef.current !== 'battle') return;
    if (!streamRef.current) return;
    recordingRef.current?.startRingBuffer(streamRef.current);
    ringBufferStartedRef.current = true;
  }, []);

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
        // Criterion 1 (#160): mirrors the just-recorded battle-start timestamp into render state
        // so `Distraction`'s `battleStartMs` prop reflects it exactly.
        setBattleStartMs(startTimeRef.current);
        maybeStartRingBuffer();
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

  // Criterion 2 (#159): the single point where win/loss is known routes into either the win-clip
  // capture flow or the loss/skip-edit placeholder — never straight to /results.
  const handleOutcome = useCallback(
    (result: 'win' | 'loss', durationMs: number, winnerId: number, loserId: number) => {
      if (result === 'win') {
        phaseRef.current = 'win-edit';
        setPhase('win-edit');
        // Capture ids/durationMs BEFORE the await — teardown() (called by routeToResults) must not
        // run until AFTER captureWin() resolves, since the recorder needs the still-live stream.
        void (async () => {
          let mp4Url: string | undefined;
          try {
            if (!recordingRef.current) {
              throw new Error('recording engine not mounted');
            }
            const blob = await recordingRef.current.captureWin();
            const { id } = await submitWinClip(blob, clipsApi);
            mp4Url = clipsApi.getClipDownloadUrl(id);
          } catch {
            // Capture/upload failed — never strand the player: route to results without a clip.
            mp4Url = undefined;
          }
          routeToResults('win', durationMs, winnerId, loserId, mp4Url);
        })();
      } else {
        phaseRef.current = 'loss-edit';
        // Skip button (rendered below) routes to /results with NO clip — captured here so the
        // click handler always uses THIS outcome's ids/durationMs, not a stale render's.
        skipEditRef.current = () => routeToResults('loss', durationMs, winnerId, loserId);
        setPhase('loss-edit');
      }
    },
    [routeToResults, clipsApi],
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
          handleOutcome(result, durationMs, msg.winner_id, msg.loser_id);
        }
      } catch {
        // ignore malformed WS frames
      }
    },
    [handleOutcome],
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
    ringBufferStartedRef.current = false;

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

    // HOTFIX (#172 lands the real fix): honor the camera picked on Home; fall back to default.
    const savedCamId = readLocal('cameraDeviceId');
    navigator.mediaDevices
      .getUserMedia(savedCamId ? { video: { deviceId: { exact: savedCamId } } } : { video: true })
      .catch(() => navigator.mediaDevices.getUserMedia({ video: true }))
      .then((stream) => {
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }
        streamRef.current = stream;
        if (localVideoRef.current) {
          localVideoRef.current.srcObject = stream;
        }
        // Criterion 1 (#159): the stream may resolve AFTER the countdown→battle transition — check
        // here too so the ring buffer starts as soon as whichever of the two settles last.
        maybeStartRingBuffer();
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
  }, [roomId, maybeStartRingBuffer]);

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
      <Recording ref={recordingRef} />
      <div data-testid="battle-split">
        <video ref={localVideoRef} autoPlay muted playsInline data-testid="local-video" />
        <video ref={remoteVideoRef} autoPlay playsInline data-testid="remote-video" />
        {/* Criterion 1/3 (#160): the Distraction control is gated on `phase === 'battle'` alone —
            it mounts exactly at battle-start (never during sanity/countdown/win-edit/loss-edit/
            done), so its internal 30s unlock timer aligns with `startTimeRef.current`, and it
            unmounts (clearing its timers) the instant the phase leaves 'battle'. */}
        {phase === 'battle' && (
          <DistractionCmp battleStartMs={battleStartMs} battleMetaRef={battleMetaRef} />
        )}
      </div>
      {phase === 'sanity' && <div data-testid="sanity-check">Checking for your face…</div>}
      {phase === 'countdown' && <div data-testid="countdown">{countdown}</div>}
      {phase === 'battle' && <div data-testid="battle-live">Battle!</div>}
      {phase === 'win-edit' && (
        <div data-testid="win-edit">Preparing your win clip…</div>
      )}
      {phase === 'loss-edit' && (
        <div data-testid="loss-edit">
          <p>Watch the winner&apos;s clip.</p>
          <button
            type="button"
            data-testid="skip-edit"
            onClick={() => skipEditRef.current()}
          >
            Skip
          </button>
        </div>
      )}
    </div>
  );
}
