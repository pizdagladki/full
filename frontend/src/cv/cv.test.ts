import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CvEngine } from './CvEngine';
import { computeEAR, LEFT_EYE_INDICES, RIGHT_EYE_INDICES } from './ear';
import type { CvCallbacks, FaceLandmarkResult, LandmarkRunner, NormalizedLandmark } from './types';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a 468-element landmark array where eye indices produce the given EAR.
 *
 * Geometry for a single eye (indices p1..p6):
 *   p1=(0,0), p4=(1,0)  → horizontal width = 1
 *   p2=(0.25, h), p6=(0.75, h)  → ||p2-p6|| = 0.5  (horizontal only, same y)
 *     Wait — to get non-zero vertical, we use:
 *   p2=(0.25, h), p6=(0.25, -h) … actually simpler:
 *
 * For EAR = e with ||p1-p4||=1:
 *   We need (||p2-p6|| + ||p3-p5||) / 2 = e
 *   Set p2=(0.25, e), p6=(0.25, -e) → ||p2-p6|| = 2e
 *   Set p3=(0.75, e), p5=(0.75, -e) → ||p3-p5|| = 2e
 *   EAR = (2e + 2e) / (2*1) = 2e (not e) — adjust: use h=e/2:
 *
 *   p2=(0.25, e/2), p6=(0.25, -e/2) → ||p2-p6|| = e
 *   p3=(0.75, e/2), p5=(0.75, -e/2) → ||p3-p5|| = e
 *   EAR = (e + e) / 2 = e  ✓
 */
function makeLandmarks(leftEar: number, rightEar: number): NormalizedLandmark[] {
  const lms: NormalizedLandmark[] = Array.from({ length: 468 }, () => ({ x: 0, y: 0, z: 0 }));

  function setEye(indices: [number, number, number, number, number, number], ear: number): void {
    const [i1, i2, i3, i4, i5, i6] = indices;
    const h = ear / 2;
    lms[i1] = { x: 0, y: 0, z: 0 };
    lms[i2] = { x: 0.25, y: h, z: 0 };
    lms[i3] = { x: 0.75, y: h, z: 0 };
    lms[i4] = { x: 1, y: 0, z: 0 };
    lms[i5] = { x: 0.75, y: -h, z: 0 };
    lms[i6] = { x: 0.25, y: -h, z: 0 };
  }

  setEye(LEFT_EYE_INDICES, leftEar);
  setEye(RIGHT_EYE_INDICES, rightEar);
  return lms;
}

/** Fake HTMLVideoElement (jsdom does not provide a real one for MediaPipe) */
function makeFakeVideo(): HTMLVideoElement {
  return document.createElement('video');
}

/** Build a mock LandmarkRunner that returns the provided result. */
function makeRunner(result: FaceLandmarkResult): LandmarkRunner {
  return { detectForVideo: vi.fn().mockReturnValue(result) };
}

/** Build a result with one face whose landmarks yield given EARs. */
function faceResult(leftEar: number, rightEar: number): FaceLandmarkResult {
  return { faceLandmarks: [makeLandmarks(leftEar, rightEar)] };
}

/** Build a result with no faces. */
const noFaceResult: FaceLandmarkResult = { faceLandmarks: [] };

/** Run N calibration frames against the engine. */
function runCalibrationFrames(engine: CvEngine, n: number, leftEar: number, rightEar: number): void {
  const result = faceResult(leftEar, rightEar);
  for (let i = 0; i < n; i++) {
    // Temporarily swap the runner result inside the engine by calling processFrame
    // via the runner the engine already holds.
    engine.processFrame(i * 33);
    // The engine's internal runner was set at construction — we rely on the mock returning
    // the same value every call; see individual tests.
    void result; // suppress unused
  }
}

// ---------------------------------------------------------------------------
// Stub RAF globally so the engine never auto-runs frames in tests
// ---------------------------------------------------------------------------
beforeEach(() => {
  vi.stubGlobal('requestAnimationFrame', vi.fn());
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

// ---------------------------------------------------------------------------
// EAR unit tests
// ---------------------------------------------------------------------------
describe('computeEAR', () => {
  it('returns the expected EAR for open-eye landmarks', () => {
    // criterion: 2 — EAR is computed per eye from landmarks
    const lms = makeLandmarks(0.4, 0.35);
    const leftEar = computeEAR(lms, LEFT_EYE_INDICES);
    expect(leftEar).toBeCloseTo(0.4, 5);
    const rightEar = computeEAR(lms, RIGHT_EYE_INDICES);
    expect(rightEar).toBeCloseTo(0.35, 5);
  });

  it('returns 0 when landmarks array is empty (missing landmark guard)', () => {
    // criterion: 2 — returns 0 if any landmark is missing; fails if guard is removed
    const lms: NormalizedLandmark[] = [];
    expect(computeEAR(lms, LEFT_EYE_INDICES)).toBe(0);
  });

  it('fails-on-violation: if EAR formula ignores vertical, blink detection breaks', () => {
    // criterion: 2 — EAR must be < open-eye EAR for a closed eye
    const openLms = makeLandmarks(0.4, 0.4);
    const closedLms = makeLandmarks(0.1, 0.1);
    const openEar = computeEAR(openLms, LEFT_EYE_INDICES);
    const closedEar = computeEAR(closedLms, LEFT_EYE_INDICES);
    // A correct formula must produce lower EAR for a closed eye
    expect(closedEar).toBeLessThan(openEar);
  });
});

// ---------------------------------------------------------------------------
// CvEngine — state machine
// ---------------------------------------------------------------------------
describe('CvEngine.getState / start / stop', () => {
  it('starts in idle state', () => {
    // criterion: 1 — getState() is part of the imperative API
    const engine = new CvEngine(makeRunner(faceResult(0.4, 0.4)));
    expect(engine.getState()).toBe('idle');
  });

  it('transitions to calibrating after start()', () => {
    // criterion: 1 — start(videoEl) is part of the imperative API
    const engine = new CvEngine(makeRunner(faceResult(0.4, 0.4)));
    engine.start(makeFakeVideo());
    expect(engine.getState()).toBe('calibrating');
  });

  it('returns to idle after stop()', () => {
    // criterion: 1 — stop() is part of the imperative API
    const engine = new CvEngine(makeRunner(faceResult(0.4, 0.4)));
    engine.start(makeFakeVideo());
    engine.stop();
    expect(engine.getState()).toBe('idle');
  });

  it('stop() cancels the pending RAF', () => {
    // criterion: 1 — per-frame loop runs via requestAnimationFrame, stop cancels it
    const engine = new CvEngine(makeRunner(faceResult(0.4, 0.4)));
    engine.start(makeFakeVideo());
    engine.stop();
    expect(cancelAnimationFrame).toHaveBeenCalled();
  });

  it('fails-on-violation: processFrame is a no-op when state is idle', () => {
    // criterion: 1 — RAF loop must not run after stop(); guard: processFrame returns early
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner);
    engine.processFrame(0); // idle — runner must NOT be called
    expect(runner.detectForVideo).not.toHaveBeenCalled();
  });

  it('re-start while running stops the previous loop first', () => {
    // criterion: 1 — start() called while running must not leave stale RAF
    const engine = new CvEngine(makeRunner(faceResult(0.4, 0.4)));
    engine.start(makeFakeVideo());
    engine.start(makeFakeVideo()); // should call stop() internally
    expect(engine.getState()).toBe('calibrating');
  });
});

// ---------------------------------------------------------------------------
// Calibration (criterion 3)
// ---------------------------------------------------------------------------
describe('calibration', () => {
  it('transitions to running after 30 calibration frames with face', () => {
    // criterion: 3 — calibration measures open-eye EAR over initial frames
    const openEar = 0.4;
    const engine = new CvEngine(makeRunner(faceResult(openEar, openEar)));
    engine.start(makeFakeVideo());
    expect(engine.getState()).toBe('calibrating');
    runCalibrationFrames(engine, 30, openEar, openEar);
    expect(engine.getState()).toBe('running');
  });

  it('uses calibrated threshold: blink fires with landmark EAR below calibrated level', () => {
    // criterion: 3 — threshold is set as mean * 0.75; verified via blink detection
    const openEar = 0.4; // calibrated threshold = 0.4 * 0.75 = 0.30
    const closedEar = 0.1; // well below 0.30
    const onBlink = vi.fn();
    const runner = makeRunner(faceResult(openEar, openEar));
    const engine = new CvEngine(runner, { onBlink });
    engine.start(makeFakeVideo());
    // Drive 30 calibration frames
    for (let i = 0; i < 30; i++) engine.processFrame(i * 33);
    expect(engine.getState()).toBe('running');

    // Now send 2 closed-eye frames
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(closedEar, closedEar));
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33);
    expect(onBlink).toHaveBeenCalled();
  });

  it('calibration fallback: uses DEFAULT_THRESHOLD when insufficient face samples', () => {
    // criterion: 3 — if calibration cannot complete, falls back to default threshold (0.25)
    const onBlink = vi.fn();
    // During calibration: no face → no samples → fallback to DEFAULT_THRESHOLD = 0.25
    const runner = makeRunner(noFaceResult);
    const engine = new CvEngine(runner, { onBlink });
    engine.start(makeFakeVideo());
    // Drive 30 frames with no face (calibration frames count only when face present,
    // but noFaceCount drives calibrationFrame—let's drive via face frames with 0-EAR
    // Actually: calibration increments calibrationFrame only when face is present.
    // With no face, we never reach CALIBRATION_FRAMES. So switch to face after:
    // Drive 10 no-face frames (noFaceCount rises but calibrationFrame stays 0)
    for (let i = 0; i < 10; i++) engine.processFrame(i * 33);

    // Now provide face with very few valid EAR samples (zero-EAR landmarks → samples not added)
    // Use landmarks where all coords are 0 → dist(p1,p4)=0 → EAR=0 → not added to samples
    const zeroEarResult: FaceLandmarkResult = { faceLandmarks: [Array.from({ length: 468 }, () => ({ x: 0, y: 0, z: 0 }))] };
    vi.mocked(runner.detectForVideo).mockReturnValue(zeroEarResult);
    // Drive 30 calibration frames with zero EAR — samples won't accumulate past MIN_CALIBRATION_SAMPLES
    for (let i = 10; i < 40; i++) engine.processFrame(i * 33);
    // Should now be in running state with DEFAULT_THRESHOLD = 0.25
    expect(engine.getState()).toBe('running');

    // Blink detection with EAR = 0.1 < DEFAULT_THRESHOLD 0.25 should fire
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.1, 0.1));
    engine.processFrame(41 * 33);
    engine.processFrame(42 * 33);
    expect(onBlink).toHaveBeenCalled();
  });

  it('fails-on-violation: blink must NOT fire when EAR is above calibrated threshold', () => {
    // criterion: 3 — calibrated threshold must gate out non-blinks
    const openEar = 0.4; // threshold = 0.30
    const onBlink = vi.fn();
    const runner = makeRunner(faceResult(openEar, openEar));
    const engine = new CvEngine(runner, { onBlink });
    engine.start(makeFakeVideo());
    for (let i = 0; i < 30; i++) engine.processFrame(i * 33);
    // EAR 0.35 > threshold 0.30 — NOT a blink
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.35, 0.35));
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33);
    expect(onBlink).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Blink detection (criterion 2)
// ---------------------------------------------------------------------------
describe('blink detection', () => {
  /** Helper: get engine past calibration, return it with the blink mock. */
  function makeRunningEngine(callbacks: CvCallbacks, openEar = 0.4): { engine: CvEngine; runner: { detectForVideo: ReturnType<typeof vi.fn> } } {
    const runner = makeRunner(faceResult(openEar, openEar));
    const engine = new CvEngine(runner, callbacks);
    engine.start(makeFakeVideo());
    for (let i = 0; i < 30; i++) engine.processFrame(i * 33);
    return { engine, runner: runner as unknown as { detectForVideo: ReturnType<typeof vi.fn> } };
  }

  it('onBlink fires when both eyes below threshold for 2 consecutive frames', () => {
    // criterion: 2 — blink fires when EAR stays below threshold for 2 consecutive frames
    const onBlink = vi.fn();
    const { engine, runner } = makeRunningEngine({ onBlink });

    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.1, 0.1));
    engine.processFrame(31 * 33);
    expect(onBlink).not.toHaveBeenCalled(); // only 1 frame so far
    engine.processFrame(32 * 33);
    expect(onBlink).toHaveBeenCalledTimes(2); // left blink + right blink
  });

  it('single-eye blink (left only) triggers onBlink — one-eye rule', () => {
    // criterion: 2 — a single-eye blink counts (one-eye rule)
    const onBlink = vi.fn();
    const { engine, runner } = makeRunningEngine({ onBlink });

    // Left eye closed (EAR 0.1), right eye open (EAR 0.35 > threshold 0.30)
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.1, 0.35));
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33);
    // Only left eye blinked
    expect(onBlink).toHaveBeenCalledTimes(1);
  });

  it('single-eye blink (right only) triggers onBlink — one-eye rule', () => {
    // criterion: 2 — single right-eye blink counts
    const onBlink = vi.fn();
    const { engine, runner } = makeRunningEngine({ onBlink });

    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.35, 0.1));
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33);
    expect(onBlink).toHaveBeenCalledTimes(1);
  });

  it('fails-on-violation: onBlink must NOT fire on only 1 frame below threshold', () => {
    // criterion: 2 — requires 2 consecutive frames (not 1) to avoid noise
    const onBlink = vi.fn();
    const { engine, runner } = makeRunningEngine({ onBlink });

    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.1, 0.1));
    engine.processFrame(31 * 33);
    expect(onBlink).not.toHaveBeenCalled();

    // Eye opens again — counter resets, no blink
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.4, 0.4));
    engine.processFrame(32 * 33);
    expect(onBlink).not.toHaveBeenCalled();
  });

  it('blink counter resets after blink fires (no double-count on 3rd frame)', () => {
    // criterion: 2 — counter resets to 0 after blink fires
    const onBlink = vi.fn();
    const { engine, runner } = makeRunningEngine({ onBlink });

    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.1, 0.4)); // left only
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33); // blink fires, counter resets
    expect(onBlink).toHaveBeenCalledTimes(1);

    engine.processFrame(33 * 33); // 1st frame after reset — no new blink yet
    expect(onBlink).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// Face gating (criterion 4)
// ---------------------------------------------------------------------------
describe('face gating', () => {
  it('onFaceLost fires after NO_FACE_WINDOW consecutive frames with no face', () => {
    // criterion: 4 — onFaceLost fires when no face detected for the short window
    const onFaceLost = vi.fn();
    const onFacePresent = vi.fn();
    // Start with a face so facePresent=true
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner, { onFaceLost, onFacePresent });
    engine.start(makeFakeVideo());

    // 1 frame with face → facePresent = true
    engine.processFrame(0);
    expect(onFacePresent).toHaveBeenCalledTimes(1);

    // Switch to no face
    vi.mocked(runner.detectForVideo).mockReturnValue(noFaceResult);
    engine.processFrame(33);
    engine.processFrame(66);
    expect(onFaceLost).not.toHaveBeenCalled(); // only 2 frames (window = 3)
    engine.processFrame(99);
    expect(onFaceLost).toHaveBeenCalledTimes(1);
  });

  it('fails-on-violation: onFaceLost must NOT fire before NO_FACE_WINDOW frames', () => {
    // criterion: 4 — the window must be reached; fails if window is 1
    const onFaceLost = vi.fn();
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner, { onFaceLost });
    engine.start(makeFakeVideo());
    engine.processFrame(0); // face present → facePresent=true

    vi.mocked(runner.detectForVideo).mockReturnValue(noFaceResult);
    engine.processFrame(33); // noFaceCount = 1
    expect(onFaceLost).not.toHaveBeenCalled();
    engine.processFrame(66); // noFaceCount = 2
    expect(onFaceLost).not.toHaveBeenCalled();
  });

  it('onFacePresent fires when face returns after being lost', () => {
    // criterion: 4 — onFacePresent fires when a face returns
    const onFaceLost = vi.fn();
    const onFacePresent = vi.fn();
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner, { onFaceLost, onFacePresent });
    engine.start(makeFakeVideo());

    // Face appears
    engine.processFrame(0);
    expect(onFacePresent).toHaveBeenCalledTimes(1);

    // Face disappears for 3 frames
    vi.mocked(runner.detectForVideo).mockReturnValue(noFaceResult);
    engine.processFrame(33);
    engine.processFrame(66);
    engine.processFrame(99);
    expect(onFaceLost).toHaveBeenCalledTimes(1);

    // Face returns
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.4, 0.4));
    engine.processFrame(132);
    expect(onFacePresent).toHaveBeenCalledTimes(2);
  });

  it('fails-on-violation: onFacePresent must fire when face first detected', () => {
    // criterion: 4 — onFacePresent must be called when face appears
    const onFacePresent = vi.fn();
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner, { onFacePresent });
    engine.start(makeFakeVideo());
    engine.processFrame(0);
    // If onFacePresent never fires, something is wrong with face-present logic
    expect(onFacePresent).toHaveBeenCalledTimes(1);
  });

  it('onFaceLost does NOT fire if face was never present', () => {
    // criterion: 4 — guard: no spurious onFaceLost on startup
    const onFaceLost = vi.fn();
    const runner = makeRunner(noFaceResult);
    const engine = new CvEngine(runner, { onFaceLost });
    engine.start(makeFakeVideo());
    // 10 frames with no face, facePresent was never true
    for (let i = 0; i < 10; i++) engine.processFrame(i * 33);
    expect(onFaceLost).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Imperative API smoke test (criterion 1 + 5)
// ---------------------------------------------------------------------------
describe('imperative API smoke test', () => {
  it('full lifecycle: start → calibrate → detect blink → stop', () => {
    // criterion: 1, 5 — exercises the full imperative API
    const onBlink = vi.fn();
    const onFacePresent = vi.fn();
    const openEar = 0.4;
    const runner = makeRunner(faceResult(openEar, openEar));
    const engine = new CvEngine(runner, { onBlink, onFacePresent });

    expect(engine.getState()).toBe('idle');
    engine.start(makeFakeVideo());
    expect(engine.getState()).toBe('calibrating');

    // Calibrate
    for (let i = 0; i < 30; i++) engine.processFrame(i * 33);
    expect(engine.getState()).toBe('running');
    expect(onFacePresent).toHaveBeenCalled();

    // Blink
    vi.mocked(runner.detectForVideo).mockReturnValue(faceResult(0.05, 0.05));
    engine.processFrame(31 * 33);
    engine.processFrame(32 * 33);
    expect(onBlink).toHaveBeenCalled();

    // Stop
    engine.stop();
    expect(engine.getState()).toBe('idle');
  });

  it('processFrame is a no-op after stop — RAF loop is truly stopped', () => {
    // criterion: 1 — per-frame loop must not run after stop()
    const onBlink = vi.fn();
    const runner = makeRunner(faceResult(0.05, 0.05));
    const engine = new CvEngine(runner, { onBlink });
    engine.start(makeFakeVideo());
    engine.stop();
    // processFrame after stop → early return, runner NOT called
    engine.processFrame(999);
    expect(runner.detectForVideo).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// No real camera / MediaPipe in tests (criterion 5)
// ---------------------------------------------------------------------------
describe('no real MediaPipe in tests', () => {
  it('mock runner is called, not real MediaPipe', () => {
    // criterion: 5 — landmark source is injected/mockable; no WASM loading
    const runner = makeRunner(faceResult(0.4, 0.4));
    const engine = new CvEngine(runner);
    engine.start(makeFakeVideo());
    engine.processFrame(0);
    // The mock was called, confirming injection works
    expect(runner.detectForVideo).toHaveBeenCalledOnce();
  });
});
