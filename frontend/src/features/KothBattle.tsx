import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ForwardRefExoticComponent, RefAttributes } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { CvComponent } from '../cv';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { defaultKingClipsApi } from '../api/kingClips';
import type { KingClipsApi } from '../api/kingClips';
import { defaultKothApi } from '../api/koth';
import type { KothApi } from '../api/koth';

// ---------------------------------------------------------------------------
// KotH battle screen — a battle-vs-recording: the "opponent" side plays the
// current king's clip instead of a live P2P peer. There is no rtc/ wiring
// here and no arbitration WS — only the LOCAL player's face/blink is judged
// (one-sided gate); the recorded opponent is never sanity-checked.
// ---------------------------------------------------------------------------

export type HillType = 'daily' | 'monthly' | 'ranked';

/** location.state carried in by hill-select (#110): `navigate('/koth/battle', { state: { hillType } })`. */
interface KothBattleLocationState {
  hillType?: HillType;
}

const SANITY_MS_DEFAULT = 2000;
const COUNTDOWN_SECONDS_DEFAULT = 5;

// TODO: wire real MediaPipe FaceLandmarker runner (separate task, mirrors Battle.tsx). Until then
// this placeholder always reports NO face — honest scaffold, never fakes a pass.
const PLACEHOLDER_RUNNER: LandmarkRunner = {
  detectForVideo: () => ({ faceLandmarks: [] }),
};

// TODO: wire the real recording engine once #52 lands. Honest scaffold: produces a real
// (empty) WebM-typed Blob rather than pretending to have captured anything.
const PLACEHOLDER_CAPTURE: () => Promise<Blob> = () =>
  Promise.resolve(new Blob([], { type: 'video/webm' }));

type Phase = 'sanity' | 'countdown' | 'battle' | 'done';

type KingClipState =
  | { status: 'loading' }
  | { status: 'loaded'; url: string }
  | { status: 'unavailable' };

/** Structural type of CvComponent's props/ref — used for the test-injection seam below. */
type CvComponentType = ForwardRefExoticComponent<
  { runner: LandmarkRunner; callbacks?: CvCallbacks } & RefAttributes<CvHandleRef>
>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KothBattleProps {
  /** Injectable king-clips API (swap with a mock in tests). Defaults to the real client. */
  kingClipsApi?: KingClipsApi;
  /** Injectable koth API (swap with a mock in tests). Defaults to the real client. */
  kothApi?: KothApi;
  /** Injectable CV landmark runner (swap with a mock in tests). Defaults to the placeholder. */
  cvRunner?: LandmarkRunner;
  /** Sanity-check duration (ms) run before the countdown. Defaults to the criterion's 2000ms. */
  sanityMs?: number;
  /** Countdown length (seconds) run after the sanity check. Defaults to the criterion's 5s. */
  countdownSeconds?: number;
  /**
   * Test seam ONLY: overrides which component mounts the CV engine (mirrors Battle.tsx's
   * `cvComponent` prop). Production never sets this — it always defaults to the real `CvComponent`.
   */
  cvComponent?: CvComponentType;
  /**
   * Recording seam (#52 not wired yet): captures the local player's attempt as a Blob. Defaults to
   * an honest empty-Blob placeholder — see PLACEHOLDER_CAPTURE.
   */
  captureAttemptClip?: () => Promise<Blob>;
}

// ---------------------------------------------------------------------------
// KothBattle component
// ---------------------------------------------------------------------------

export function KothBattle({
  kingClipsApi = defaultKingClipsApi,
  kothApi = defaultKothApi,
  cvRunner = PLACEHOLDER_RUNNER,
  sanityMs = SANITY_MS_DEFAULT,
  countdownSeconds = COUNTDOWN_SECONDS_DEFAULT,
  cvComponent: Cv = CvComponent,
  captureAttemptClip = PLACEHOLDER_CAPTURE,
}: KothBattleProps) {
  const location = useLocation();
  const navigate = useNavigate();

  const locationState = (location.state as KothBattleLocationState | null) ?? null;
  const hillType: HillType = locationState?.hillType ?? 'daily';

  const localVideoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const cvRef = useRef<CvHandleRef>(null);

  // Refs (not state) drive decisions read from callbacks registered once at mount — state would
  // be stale inside those frozen closures (mirrors Battle.tsx).
  const facePresentRef = useRef(false);
  const phaseRef = useRef<Phase>('sanity');
  const teardownRef = useRef(false);
  const outcomeRef = useRef(false);
  const startTimeRef = useRef(0);
  const sanityTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const countdownTimerRef = useRef<ReturnType<typeof setInterval>>();

  // Drives rendering only.
  const [phase, setPhase] = useState<Phase>('sanity');
  const [countdown, setCountdown] = useState(countdownSeconds);
  const [kingClip, setKingClip] = useState<KingClipState>({ status: 'loading' });

  const teardown = useCallback(() => {
    if (teardownRef.current) return;
    teardownRef.current = true;
    if (sanityTimerRef.current) clearTimeout(sanityTimerRef.current);
    if (countdownTimerRef.current) clearInterval(countdownTimerRef.current);
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }
  }, []);

  // Criterion 3 — outcome: computes elapsedMs once (one-shot, guarded), then posts to the right
  // backend endpoint depending on hillType, then routes to the solo results screen. Any rejection
  // in the chain degrades to a neutral error result rather than crashing (mirrors Store.tsx).
  const handleOutcome = useCallback(
    (elapsedMs: number) => {
      if (outcomeRef.current) return;
      outcomeRef.current = true;
      phaseRef.current = 'done';
      setPhase('done');
      teardown();

      if (hillType === 'ranked') {
        if (elapsedMs <= 0) {
          // Sanity-check failure before any battle started — never call the ranked endpoint with
          // held_ms <= 0 (backend rejects it).
          navigate('/koth/results', { state: { hillType: 'ranked', noAttempt: true } });
          return;
        }
        kothApi
          .submitRankedAttempt({ held_ms: elapsedMs })
          .then((result) => {
            navigate('/koth/results', {
              state: {
                hillType: 'ranked',
                achievedRank: result.achieved_rank,
                currentRank: result.current_rank,
                newlyReached: result.newly_reached,
              },
            });
          })
          .catch(() => {
            navigate('/koth/results', { state: { error: true, hillType: 'ranked' } });
          });
        return;
      }

      // daily / monthly — always upload the attempt clip first (the backend requires new_clip_id
      // even on a loss), then challenge the hill with the resulting clip id.
      captureAttemptClip()
        .then((blob) => kingClipsApi.upload(hillType, elapsedMs, blob))
        .then(({ id }) =>
          kothApi.challengeHill(hillType, { survived_ms: elapsedMs, new_clip_id: String(id) }),
        )
        .then(({ won, king }) => {
          navigate('/koth/results', { state: { hillType, won, king, survivedMs: elapsedMs } });
        })
        .catch(() => {
          navigate('/koth/results', { state: { error: true, hillType } });
        });
    },
    // Intentionally stable ([]): registered once from the mount effect / cv callbacks, mirrors
    // Battle.tsx's routeToResults freezing pattern. hillType/kothApi/kingClipsApi/captureAttemptClip
    // are fixed for the lifetime of one battle attempt (location.state doesn't change mid-battle).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
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
    // Face loss AFTER the battle starts is treated as a forfeit/stop (mirrors Battle.tsx's
    // forfeit-on-face-loss idea) — compute the held duration and route to the outcome.
    if (phaseRef.current === 'battle') {
      handleOutcome(Date.now() - startTimeRef.current);
    }
    // Intentionally stable ([]): CvComponent freezes this callback on first render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onBlink = useCallback(() => {
    if (phaseRef.current === 'battle') {
      handleOutcome(Date.now() - startTimeRef.current);
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

  // Mount effect — starts the cv engine and arms the sanity-check timer. One-sided gate: this is
  // driven ONLY by the local player's face state; the king clip fetch (below) runs independently
  // and never blocks or delays this flow.
  useEffect(() => {
    teardownRef.current = false;
    outcomeRef.current = false;
    facePresentRef.current = false;
    phaseRef.current = 'sanity';

    if (localVideoRef.current && cvRef.current) {
      cvRef.current.start(localVideoRef.current);
    }

    // A sanity check runs BEFORE the countdown; if no face is present it cancels the battle
    // (elapsedMs = 0 conceptually — no battle ever started) and routes via the outcome branch.
    sanityTimerRef.current = setTimeout(() => {
      if (facePresentRef.current) {
        startCountdown();
      } else {
        handleOutcome(0);
      }
    }, sanityMs);

    return () => teardown();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Separate guarded camera-preview effect (mirrors Battle.tsx) — never blocks cv.start on the
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
        if (localVideoRef.current) {
          localVideoRef.current.srcObject = stream;
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

  // Fetches the current king clip in parallel with the sanity-check/countdown/battle flow above —
  // it must never block or delay it (one-sided gate: the opponent is a recording, no sanity-check
  // for it). 404/error degrades to a neutral "no challenger yet" placeholder, never a crash.
  useEffect(() => {
    let cancelled = false;
    kingClipsApi
      .getCurrent(hillType)
      .then((data) => {
        if (cancelled) return;
        if (data) {
          setKingClip({ status: 'loaded', url: data.download_url });
        } else {
          setKingClip({ status: 'unavailable' });
        }
      })
      .catch(() => {
        if (cancelled) return;
        setKingClip({ status: 'unavailable' });
      });
    return () => {
      cancelled = true;
    };
  }, [hillType, kingClipsApi]);

  return (
    <div data-testid="koth-battle-screen">
      <Cv ref={cvRef} runner={cvRunner} callbacks={cvCallbacks} />
      <div data-testid="battle-split">
        <video ref={localVideoRef} autoPlay muted playsInline data-testid="local-video" />
        {kingClip.status === 'loaded' && (
          <video
            src={kingClip.url}
            autoPlay
            playsInline
            data-testid="king-clip-video"
          />
        )}
        {kingClip.status === 'unavailable' && (
          <div data-testid="no-king-clip">No recorded challenger yet</div>
        )}
      </div>
      {phase === 'sanity' && <div data-testid="sanity-check">Checking for your face…</div>}
      {phase === 'countdown' && <div data-testid="countdown">{countdown}</div>}
      {phase === 'battle' && <div data-testid="battle-live">Battle!</div>}
    </div>
  );
}
