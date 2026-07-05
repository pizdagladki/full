import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { FaceLandmarkResult, LandmarkRunner } from './types';

// Mock the whole `@mediapipe/tasks-vision` module (mirrors FaceLandmarkerRunner.test.ts) so that
// `defaultCvRunner`'s no-arg production default (`createFaceLandmarkerRunner`) never touches a
// real model/WASM/network/camera even in the one test that exercises it.
vi.mock('@mediapipe/tasks-vision', () => ({
  FaceLandmarker: { createFromOptions: vi.fn() },
  FilesetResolver: { forVisionTasks: vi.fn() },
}));

import { defaultCvRunner, __resetDefaultCvRunnerForTests } from './defaultCvRunner';
import type { CvRunnerLoader } from './defaultCvRunner';
import { createFaceLandmarkerRunner } from './FaceLandmarkerRunner';

function makeFakeVideo(): HTMLVideoElement {
  return document.createElement('video');
}

const REAL_FACE: FaceLandmarkResult = { faceLandmarks: [[{ x: 0.1, y: 0.2, z: 0.3 }]] };

function makeLoadedRunner(result: FaceLandmarkResult): LandmarkRunner {
  return { detectForVideo: vi.fn(() => result) };
}

/** A loader whose resolution/rejection is controlled by the test. */
function makeControllableLoader(): {
  loader: CvRunnerLoader;
  resolve: (runner: LandmarkRunner) => void;
  reject: (err: unknown) => void;
} {
  let resolveFn!: (runner: LandmarkRunner) => void;
  let rejectFn!: (err: unknown) => void;
  const loader: CvRunnerLoader = () =>
    new Promise<LandmarkRunner>((resolve, reject) => {
      resolveFn = resolve;
      rejectFn = reject;
    });
  return {
    loader,
    resolve: (runner: LandmarkRunner) => resolveFn(runner),
    reject: (err: unknown) => rejectFn(err),
  };
}

beforeEach(() => {
  __resetDefaultCvRunnerForTests();
});

describe('defaultCvRunner', () => {
  // criterion: a — before the async load resolves, detectForVideo is honest: no face.
  it('criterion: a — before load resolves, detectForVideo returns { faceLandmarks: [] }', () => {
    const { loader } = makeControllableLoader();
    const runner = defaultCvRunner(loader);

    const result = runner.detectForVideo(makeFakeVideo(), 0);

    expect(result).toEqual({ faceLandmarks: [] });
  });

  // criterion: b — once the mocked loader resolves, subsequent frames delegate to the real runner.
  it('criterion: b — after the loader resolves, detectForVideo delegates to the loaded runner', async () => {
    const { loader, resolve } = makeControllableLoader();
    const runner = defaultCvRunner(loader);
    const loaded = makeLoadedRunner(REAL_FACE);

    resolve(loaded);
    await Promise.resolve();
    await Promise.resolve();

    const result = runner.detectForVideo(makeFakeVideo(), 42);

    expect(result).toEqual(REAL_FACE);
    expect(loaded.detectForVideo).toHaveBeenCalledWith(expect.any(HTMLVideoElement), 42);
  });

  // criterion: c — a stable singleton: the SAME instance is returned across calls, and a second
  // loader passed on a later call is never even invoked (the singleton is already built).
  it('criterion: c — returns a stable singleton instance across calls; a later loader is ignored', () => {
    const firstLoader = vi.fn(() => new Promise<LandmarkRunner>(() => {}));
    const secondLoader = vi.fn(() => new Promise<LandmarkRunner>(() => {}));

    const first = defaultCvRunner(firstLoader);
    const second = defaultCvRunner(secondLoader);

    expect(second).toBe(first);
    expect(firstLoader).toHaveBeenCalledTimes(1);
    expect(secondLoader).not.toHaveBeenCalled();
  });

  // criterion: d — a rejected load must never throw (synchronously OR as an unhandled rejection
  // surfacing through detectForVideo), and must leave the runner permanently at no-face.
  it('criterion: d — a rejected load leaves the runner at no-face without throwing', async () => {
    const { loader, reject } = makeControllableLoader();
    const runner = defaultCvRunner(loader);

    expect(() => reject(new Error('model load failed'))).not.toThrow();
    await Promise.resolve();
    await Promise.resolve();

    expect(() => runner.detectForVideo(makeFakeVideo(), 0)).not.toThrow();
    expect(runner.detectForVideo(makeFakeVideo(), 0)).toEqual({ faceLandmarks: [] });
  });

  // fails-on-violation: a runner that "always" returns no-face (the removed PLACEHOLDER_RUNNER
  // pattern) would fail this — this deferred adapter must eventually report a real face once ready.
  it('fails-on-violation: once loaded, the adapter reports a real face (not a permanent placeholder)', async () => {
    const { loader, resolve } = makeControllableLoader();
    const runner = defaultCvRunner(loader);
    resolve(makeLoadedRunner(REAL_FACE));
    await Promise.resolve();
    await Promise.resolve();

    expect(runner.detectForVideo(makeFakeVideo(), 0).faceLandmarks).not.toEqual([]);
  });

  // Production wiring: with no loader argument, the real `createFaceLandmarkerRunner` factory is
  // used (mocked `@mediapipe/tasks-vision` above keeps this offline).
  it('wires the real createFaceLandmarkerRunner as the default loader when none is injected', () => {
    const runner = defaultCvRunner();

    expect(runner).toBeDefined();
    expect(typeof runner.detectForVideo).toBe('function');
    // No face yet (the mocked FilesetResolver/FaceLandmarker never resolve) — still honest.
    expect(runner.detectForVideo(makeFakeVideo(), 0)).toEqual({ faceLandmarks: [] });
    expect(createFaceLandmarkerRunner).toBeDefined();
  });
});
