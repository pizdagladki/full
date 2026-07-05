import { forwardRef, useEffect, useImperativeHandle } from 'react';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { KothBattle } from './KothBattle';
import type { KothBattleProps } from './KothBattle';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { defaultCvRunner, __resetDefaultCvRunnerForTests } from '../cv';
import type { KingClipsApi, CurrentKingClip, KingClipUploadResult } from '../api/kingClips';
import type { KothApi, ChallengeHillResult, RankedAttemptResult } from '../api/koth';

// ---------------------------------------------------------------------------
// Fake Cv mount — mirrors Battle.test.tsx's makeFakeCv seam: fires
// onFacePresent/onBlink/onFaceLost on demand instead of driving CvEngine's
// real EAR/calibration math under fake timers.
// ---------------------------------------------------------------------------

function makeFakeCv(): {
  Cv: NonNullable<KothBattleProps['cvComponent']>;
  start: ReturnType<typeof vi.fn>;
  fireFacePresent: () => void;
  fireFaceLost: () => void;
  fireBlink: () => void;
  /** The `runner` prop KothBattle actually passed down — captured for the default-wiring test below. */
  getCapturedRunner: () => LandmarkRunner | undefined;
} {
  let cb: CvCallbacks = {};
  let capturedRunner: LandmarkRunner | undefined;
  const start = vi.fn();
  const Cv = forwardRef<CvHandleRef, { runner: LandmarkRunner; callbacks?: CvCallbacks }>(
    ({ runner, callbacks }, ref) => {
      useEffect(() => {
        cb = callbacks ?? {};
        capturedRunner = runner;
      });
      useImperativeHandle(ref, () => ({
        start,
        stop: vi.fn(),
        getState: () => 'running',
      }));
      return null;
    },
  );
  return {
    Cv,
    start,
    fireFacePresent: () => cb.onFacePresent?.(),
    fireFaceLost: () => cb.onFaceLost?.(),
    fireBlink: () => cb.onBlink?.(),
    getCapturedRunner: () => capturedRunner,
  };
}

// ---------------------------------------------------------------------------
// Fake king-clips / koth API doubles
// ---------------------------------------------------------------------------

function makeKingClipsApi(options: {
  current?: CurrentKingClip | null | Error;
  upload?: KingClipUploadResult | Error;
} = {}): KingClipsApi {
  const { current = null, upload = { id: 42 } } = options;
  return {
    getCurrent:
      current instanceof Error
        ? vi.fn().mockRejectedValue(current)
        : vi.fn().mockResolvedValue(current),
    upload: upload instanceof Error ? vi.fn().mockRejectedValue(upload) : vi.fn().mockResolvedValue(upload),
  };
}

function makePendingKingClipsApi(): KingClipsApi {
  return {
    getCurrent: vi.fn().mockReturnValue(new Promise(() => {})),
    upload: vi.fn(),
  };
}

function makeKothApi(options: {
  challenge?: ChallengeHillResult | Error;
  ranked?: RankedAttemptResult | Error;
} = {}): KothApi {
  const {
    challenge = { won: true, king: { user_id: 1, clip_id: '42', blink_ts_ms: 500 } },
    ranked = { achieved_rank: 3, current_rank: 5, newly_reached: true },
  } = options;
  return {
    challengeHill:
      challenge instanceof Error
        ? vi.fn().mockRejectedValue(challenge)
        : vi.fn().mockResolvedValue(challenge),
    submitRankedAttempt:
      ranked instanceof Error ? vi.fn().mockRejectedValue(ranked) : vi.fn().mockResolvedValue(ranked),
    getKing: vi.fn().mockResolvedValue(null),
    getRankedLeaderboard: vi.fn().mockResolvedValue([]),
    getRankedMe: vi.fn().mockResolvedValue({ current_rank: 0, next_target_ms: 0 }),
  };
}

// ---------------------------------------------------------------------------
// Routing harness
// ---------------------------------------------------------------------------

interface ResultsState {
  hillType?: string;
  won?: boolean;
  survivedMs?: number;
  achievedRank?: number;
  currentRank?: number;
  newlyReached?: boolean;
  noAttempt?: boolean;
  error?: boolean;
}

function ResultsProbe() {
  const location = useLocation();
  const state = (location.state as ResultsState | null) ?? null;
  return (
    <div data-testid="results-probe">
      <span data-testid="results-hillType">{state?.hillType}</span>
      <span data-testid="results-won">{String(state?.won)}</span>
      <span data-testid="results-survivedMs">{state?.survivedMs}</span>
      <span data-testid="results-achievedRank">{state?.achievedRank}</span>
      <span data-testid="results-currentRank">{state?.currentRank}</span>
      <span data-testid="results-newlyReached">{String(state?.newlyReached)}</span>
      <span data-testid="results-noAttempt">{String(state?.noAttempt)}</span>
      <span data-testid="results-error">{String(state?.error)}</span>
    </div>
  );
}

function renderKothBattle(hillType: string, props: KothBattleProps) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/koth/battle', state: { hillType } }]}>
      <Routes>
        <Route path="/koth/battle" element={<KothBattle {...props} />} />
        <Route path="/koth/results" element={<ResultsProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

function reachBattle(fireFacePresent: () => void): void {
  act(() => {
    fireFacePresent();
  });
  act(() => {
    vi.advanceTimersByTime(2000);
  });
  act(() => {
    vi.advanceTimersByTime(5000);
  });
}

/**
 * Flushes pending microtasks (promise .then chains) under fake timers — `waitFor`'s internal
 * polling relies on real timers, which never fire once `vi.useFakeTimers()` is active, so it
 * hangs; this awaits the resolved promise chain directly instead (mirrors Battle.test.tsx's
 * `await Promise.resolve()` flushing pattern).
 */
async function flush(times = 3): Promise<void> {
  for (let i = 0; i < times; i += 1) {
    await act(async () => {
      await Promise.resolve();
    });
  }
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  __resetDefaultCvRunnerForTests();
});

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('KothBattle', () => {
  // criterion: 1 — the "opponent" side plays the current king clip fetched from
  // GET /v1/king-clips/current?hill_type=... instead of a P2P opponent.
  it('king-clip-plays-as-opponent: renders the fetched clip as the opponent video', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({
      current: { download_url: 'https://cdn.example/king.webm', blink_ts_ms: 900 },
    });
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi });

    await flush();
    const video = screen.getByTestId('king-clip-video') as HTMLVideoElement;
    expect(video.getAttribute('src')).toBe('https://cdn.example/king.webm');
    expect(kingClipsApi.getCurrent).toHaveBeenCalledWith('daily');
    expect(screen.queryByTestId('no-king-clip')).not.toBeInTheDocument();
  });

  // bug 1 — the king-clip video must be muted, same as local-video, or the browser autoplay
  // policy (Safari requires muted+playsinline; Chrome gates on Media Engagement) silently blocks
  // playback and the recorded "opponent" never plays.
  it('bug1-king-clip-video-is-muted: the king-clip video element is muted so it can autoplay', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({
      current: { download_url: 'https://cdn.example/king.webm', blink_ts_ms: 900 },
    });
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi });

    await flush();
    const video = screen.getByTestId('king-clip-video') as HTMLVideoElement;
    expect(video.muted).toBe(true);
  });

  // criterion: 1 (violation guard) — no king clip yet (404 -> null) must render the neutral
  // placeholder, not crash, and not render a video element.
  it('king-clip-plays-as-opponent violation guard: a 404/null clip renders the no-clip placeholder', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ current: null });
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi });

    await flush();
    expect(screen.getByTestId('no-king-clip')).toBeInTheDocument();
    expect(screen.queryByTestId('king-clip-video')).not.toBeInTheDocument();
  });

  // criterion: 1 — an errored clip fetch must also degrade to the neutral placeholder, never crash.
  it('king-clip-plays-as-opponent: a rejected clip fetch degrades to the no-clip placeholder', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ current: new Error('network down') });
    expect(() => renderKothBattle('daily', { cvComponent: Cv, kingClipsApi })).not.toThrow();

    await flush();
    expect(screen.getByTestId('no-king-clip')).toBeInTheDocument();
  });

  // criterion: 2 — the face gate is ONE-SIDED: sanity->countdown->battle proceeds based ONLY on
  // the local face-present mock, regardless of the king clip fetch never resolving.
  it('one-sided-gate: battle proceeds through sanity->countdown->battle even while the king clip fetch is still pending', () => {
    const { Cv, fireFacePresent } = makeFakeCv();
    const kingClipsApi = makePendingKingClipsApi();
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi });

    expect(screen.getByTestId('sanity-check')).toBeInTheDocument();

    act(() => {
      fireFacePresent();
    });
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByTestId('countdown')).toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();
    // The opponent clip never resolved — no video, no placeholder yet — and that must not have
    // blocked the local flow at all.
    expect(screen.queryByTestId('king-clip-video')).not.toBeInTheDocument();
    expect(screen.queryByTestId('no-king-clip')).not.toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — the opponent clip resolving (or failing) must never gate the
  // local sanity check: with no local face present, the sanity check still fails as normal even
  // though a king clip loaded successfully.
  it('one-sided-gate violation guard: a loaded king clip does not rescue a failed local sanity check', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({
      current: { download_url: 'https://cdn.example/king.webm', blink_ts_ms: 900 },
    });
    const kothApi = makeKothApi();
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi });

    await flush();
    expect(screen.getByTestId('king-clip-video')).toBeInTheDocument();

    // Never fire facePresent — the LOCAL player's face is never detected.
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.queryByTestId('countdown')).not.toBeInTheDocument();
  });

  // criterion: 3 — win path for daily/monthly: a mocked blink posts the upload THEN the challenge
  // with the right body shape, and routes to results with the right state.
  it('daily-win-uploads-then-challenges: a blink during battle uploads the clip then posts the challenge', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const fakeBlob = new Blob(['attempt'], { type: 'video/webm' });
    const kingClipsApi = makeKingClipsApi({ upload: { id: 77 } });
    const kothApi = makeKothApi({
      challenge: { won: true, king: { user_id: 9, clip_id: '77', blink_ts_ms: 500 } },
    });
    renderKothBattle('daily', {
      cvComponent: Cv,
      kingClipsApi,
      kothApi,
      captureAttemptClip: () => Promise.resolve(fakeBlob),
    });

    reachBattle(fireFacePresent);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    await act(async () => {
      fireBlink();
      // Flush the upload -> challenge promise chain.
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kingClipsApi.upload).toHaveBeenCalledWith('daily', 500, fakeBlob);
    expect(kothApi.challengeHill).toHaveBeenCalledWith('daily', {
      survived_ms: 500,
      new_clip_id: '77',
    });
    // Upload happens BEFORE the challenge call (call-order, not just call presence).
    const uploadOrder = vi.mocked(kingClipsApi.upload).mock.invocationCallOrder[0];
    const challengeOrder = vi.mocked(kothApi.challengeHill).mock.invocationCallOrder[0];
    expect(uploadOrder).toBeLessThan(challengeOrder);

    await flush();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-hillType').textContent).toBe('daily');
    expect(screen.getByTestId('results-won').textContent).toBe('true');
    expect(screen.getByTestId('results-survivedMs').textContent).toBe('500');
  });

  // criterion: 3 — a loss (won: false) for daily/monthly still uploads+challenges (new_clip_id is
  // required by the backend even on a loss) and routes with won: false.
  it('daily-loss-still-uploads-and-challenges: a losing outcome still posts new_clip_id and routes won=false', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ upload: { id: 11 } });
    const kothApi = makeKothApi({
      challenge: { won: false, king: { user_id: 3, clip_id: '99', blink_ts_ms: 800 } },
    });
    renderKothBattle('monthly', { cvComponent: Cv, kingClipsApi, kothApi });

    reachBattle(fireFacePresent);
    await act(async () => {
      fireBlink();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kothApi.challengeHill).toHaveBeenCalledWith(
      'monthly',
      expect.objectContaining({ new_clip_id: '11' }),
    );
    await flush();
    expect(screen.getByTestId('results-won').textContent).toBe('false');
  });

  // bug 2 — the actual win condition: outlasting the king clip's own blink_ts_ms WITHOUT ever
  // blinking. This is the previously-impossible path (a player who never blinks used to hang in
  // 'battle' forever, and survived_ms never got computed/posted). Without the fix, advancing past
  // blink_ts_ms here would never reach the results screen and this test would time out/fail.
  it('bug2-outlasts-king-clip-wins: holding past the loaded clip blink_ts_ms without blinking fires the win outcome', async () => {
    const { Cv, fireFacePresent } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({
      current: { download_url: 'https://cdn.example/king.webm', blink_ts_ms: 600 },
      upload: { id: 88 },
    });
    const kothApi = makeKothApi({
      challenge: { won: true, king: { user_id: 5, clip_id: '88', blink_ts_ms: 600 } },
    });
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi });

    // Let the king clip finish loading BEFORE the battle phase starts.
    await flush();
    expect(screen.getByTestId('king-clip-video')).toBeInTheDocument();

    reachBattle(fireFacePresent);
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();

    // Advance exactly to blink_ts_ms WITHOUT ever firing onBlink — outlasting is the win trigger,
    // not a manual blink.
    await act(async () => {
      vi.advanceTimersByTime(600);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kingClipsApi.upload).toHaveBeenCalledWith('daily', 600, expect.any(Blob));
    expect(kothApi.challengeHill).toHaveBeenCalledWith('daily', {
      survived_ms: 600,
      new_clip_id: '88',
    });

    await flush();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-hillType').textContent).toBe('daily');
    expect(screen.getByTestId('results-won').textContent).toBe('true');
    expect(screen.getByTestId('results-survivedMs').textContent).toBe('600');
  });

  // bug 2 (violation guard) — ranked's mechanic is server-side time thresholds, not tied to any
  // clip's blink moment (per the spec), so even if a clip happens to be loaded, ranked must NOT
  // arm a clip-blink timer: holding past its blink_ts_ms must not end the battle. Only the
  // player's own blink/face-loss ends a ranked attempt.
  it('bug2-ranked-has-no-king-blink-timer: holding past a loaded clip blink_ts_ms in ranked does not end the battle', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({
      current: { download_url: 'https://cdn.example/king.webm', blink_ts_ms: 300 },
    });
    const kothApi = makeKothApi({
      ranked: { achieved_rank: 1, current_rank: 2, newly_reached: false },
    });
    renderKothBattle('ranked', { cvComponent: Cv, kingClipsApi, kothApi });

    await flush();
    reachBattle(fireFacePresent);
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();

    // Advance well past the loaded clip's blink_ts_ms — ranked must have no clip-blink timer, so
    // the battle is still live and the ranked endpoint has not been called yet.
    await act(async () => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();
    expect(kothApi.submitRankedAttempt).not.toHaveBeenCalled();

    // The player still ends the attempt themselves via a blink, as usual for ranked.
    await act(async () => {
      fireBlink();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(kothApi.submitRankedAttempt).toHaveBeenCalledWith({ held_ms: 5000 });
  });

  // criterion: 3 — the ranked hill posts POST /v1/koth/ranked/attempt with held_ms and routes to
  // results with the ranked outcome shape.
  it('ranked-attempt-posts-held-ms: a blink during a ranked battle posts held_ms and routes with rank info', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const kothApi = makeKothApi({
      ranked: { achieved_rank: 2, current_rank: 4, newly_reached: true },
    });
    renderKothBattle('ranked', { cvComponent: Cv, kothApi });

    reachBattle(fireFacePresent);
    act(() => {
      vi.advanceTimersByTime(750);
    });
    await act(async () => {
      fireBlink();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kothApi.submitRankedAttempt).toHaveBeenCalledWith({ held_ms: 750 });

    await flush();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-hillType').textContent).toBe('ranked');
    expect(screen.getByTestId('results-achievedRank').textContent).toBe('2');
    expect(screen.getByTestId('results-currentRank').textContent).toBe('4');
    expect(screen.getByTestId('results-newlyReached').textContent).toBe('true');
  });

  // criterion: 3 (violation guard) — a ranked sanity-check failure (elapsedMs <= 0, no battle ever
  // started) must NOT call the ranked endpoint (backend rejects held_ms <= 0); it routes with a
  // "no attempt" state instead.
  it('ranked-sanity-fail-skips-api: a failed sanity check for ranked never calls submitRankedAttempt', async () => {
    const { Cv } = makeFakeCv();
    const kothApi = makeKothApi();
    renderKothBattle('ranked', { cvComponent: Cv, kothApi });

    // Never fire facePresent.
    await act(async () => {
      vi.advanceTimersByTime(2000);
    });

    expect(kothApi.submitRankedAttempt).not.toHaveBeenCalled();
    await flush();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-hillType').textContent).toBe('ranked');
    expect(screen.getByTestId('results-noAttempt').textContent).toBe('true');
  });

  // criterion: 3 — the sanity-check-fail path is different for daily/monthly: 0 survived_ms IS
  // valid there, so the challenge is still posted normally (unlike ranked, which is skipped).
  it('daily-sanity-fail-still-challenges: a failed sanity check for daily still uploads+challenges with survived_ms 0', async () => {
    const { Cv } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ upload: { id: 5 } });
    const kothApi = makeKothApi({
      challenge: { won: false, king: { user_id: 1, clip_id: '5', blink_ts_ms: 0 } },
    });
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi });

    // Never fire facePresent.
    await act(async () => {
      vi.advanceTimersByTime(2000);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kingClipsApi.upload).toHaveBeenCalledWith('daily', 0, expect.any(Blob));
    expect(kothApi.challengeHill).toHaveBeenCalledWith(
      'daily',
      expect.objectContaining({ survived_ms: 0 }),
    );
  });

  // criterion: 3 — any rejection in the outcome chain (network error, etc.) must not crash; it
  // degrades to a neutral error state instead.
  it('outcome-chain-error-degrades: a rejected upload/challenge routes to a neutral error state, never crashes', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ upload: new Error('storage down') });
    const kothApi = makeKothApi();
    expect(() =>
      renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi }),
    ).not.toThrow();

    reachBattle(fireFacePresent);
    await act(async () => {
      fireBlink();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    await flush();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-error').textContent).toBe('true');
    expect(kothApi.challengeHill).not.toHaveBeenCalled();
  });

  // criterion: 3 (violation guard) — face loss AFTER battle start (a forfeit) triggers the SAME
  // outcome path as a blink, and must fire only once even if both onBlink and onFaceLost occur.
  it('face-loss-during-battle-forfeits-once: losing the face mid-battle triggers the outcome exactly once', async () => {
    const { Cv, fireFacePresent, fireFaceLost, fireBlink } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ upload: { id: 1 } });
    const kothApi = makeKothApi();
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi });

    reachBattle(fireFacePresent);
    await act(async () => {
      fireFaceLost();
      fireBlink(); // must be a no-op — the outcome already fired (one-shot guard)
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kingClipsApi.upload).toHaveBeenCalledTimes(1);
  });

  // criterion: default hillType — defensive fallback to 'daily' when location.state carries none.
  it('defaults-to-daily-hillType: a missing location.state hillType falls back to daily', async () => {
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi({ upload: { id: 1 } });
    const kothApi = makeKothApi();
    render(
      <MemoryRouter initialEntries={['/koth/battle']}>
        <Routes>
          <Route
            path="/koth/battle"
            element={<KothBattle cvComponent={Cv} kingClipsApi={kingClipsApi} kothApi={kothApi} />}
          />
          <Route path="/koth/results" element={<ResultsProbe />} />
        </Routes>
      </MemoryRouter>,
    );

    reachBattle(fireFacePresent);
    await act(async () => {
      fireBlink();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(kingClipsApi.upload).toHaveBeenCalledWith('daily', expect.any(Number), expect.any(Blob));
  });

  // criteria: 1b/2b (default wiring regression guard) — with NO `cvRunner` prop supplied at all,
  // KothBattle must fall back to the real `defaultCvRunner()` singleton, NOT an inline no-face
  // placeholder. Driving a real face through CvEngine's full RAF/calibration pipeline via this
  // screen is impractical here: KothBattle.test.tsx deliberately swaps in a fake `cvComponent` (see
  // its doc comment, mirrors Battle.tsx) that never calls `runner.detectForVideo` at all, so a
  // behavioral assertion on face-present-driven UI would pass trivially regardless of which runner
  // is wired. Instead this asserts the referential-identity edge directly: seed the module
  // singleton (reset + a sentinel loader) BEFORE rendering, then confirm the exact `runner` prop
  // KothBattle passed down to its cv component IS that same singleton instance. If KothBattle.tsx's
  // default were reverted to an inline `{ detectForVideo: () => ({ faceLandmarks: [] }) }`, the
  // captured runner would be a different object and this identity check would fail.
  it('production-default-wiring: with no cvRunner prop, KothBattle wires the real defaultCvRunner() singleton', () => {
    __resetDefaultCvRunnerForTests();
    const singleton = defaultCvRunner(() => new Promise<LandmarkRunner>(() => {}));

    const { Cv, getCapturedRunner } = makeFakeCv();
    const kingClipsApi = makeKingClipsApi();
    const kothApi = makeKothApi();
    renderKothBattle('daily', { cvComponent: Cv, kingClipsApi, kothApi });

    expect(getCapturedRunner()).toBe(singleton);
  });
});
