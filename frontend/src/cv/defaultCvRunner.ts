import type { FaceLandmarkResult, LandmarkRunner } from './types';
import { createFaceLandmarkerRunner } from './FaceLandmarkerRunner';

/** Injectable loader seam — production defaults to the real `createFaceLandmarkerRunner`; tests
 * inject a fake so no real model/network/camera is ever touched. */
export type CvRunnerLoader = () => Promise<LandmarkRunner>;

/**
 * A synchronous `LandmarkRunner` adapter that defers to a `LandmarkRunner` which is still loading
 * asynchronously. `detectForVideo` is honest: it returns `{ faceLandmarks: [] }` ONLY until the
 * real runner has finished loading (or if loading fails) — it is NOT a placeholder that "always"
 * returns empty, it delegates to the real runner's own result once ready. Never throws — a
 * rejected load just leaves the adapter permanently at no-face.
 */
class DeferredCvRunner implements LandmarkRunner {
  private inner: LandmarkRunner | null = null;

  constructor(load: CvRunnerLoader) {
    load()
      .then((runner) => {
        this.inner = runner;
      })
      .catch(() => {
        // Load failed — stays no-face forever. Never throws, never fabricates a face.
      });
  }

  detectForVideo(videoElement: HTMLVideoElement, timestamp: number): FaceLandmarkResult {
    if (!this.inner) return { faceLandmarks: [] };
    return this.inner.detectForVideo(videoElement, timestamp);
  }
}

/** Module-level singleton — ensures the (expensive, one-time) WASM+model load is kicked off at
 * most once app-wide, and that `defaultCvRunner()` called from a default-parameter position
 * returns the SAME stable instance on every render (cheap, no per-render construction). */
let singleton: DeferredCvRunner | null = null;

/**
 * Returns the shared, stable, lazy default `LandmarkRunner` used by every game screen. Safe to
 * call from a default-parameter position (`cvRunner = defaultCvRunner()`) on every render: the
 * underlying real MediaPipe FaceLandmarker load is kicked off exactly once, the first time this
 * is called anywhere in the app; every subsequent call (with or without an explicit `loader`)
 * returns that same already-created instance.
 *
 * @param loader Only consulted the FIRST time this is ever called (module-level singleton) —
 *   defaults to the real `createFaceLandmarkerRunner`. Tests that need a fresh instance must call
 *   `__resetDefaultCvRunnerForTests` first, then pass a mock loader here.
 */
export function defaultCvRunner(loader: CvRunnerLoader = createFaceLandmarkerRunner): LandmarkRunner {
  if (!singleton) {
    singleton = new DeferredCvRunner(loader);
  }
  return singleton;
}

/** Test-only: clears the module-level singleton so the next `defaultCvRunner()` call constructs a
 * fresh instance (and consults the `loader` argument again). Never called from production code. */
export function __resetDefaultCvRunnerForTests(): void {
  singleton = null;
}
