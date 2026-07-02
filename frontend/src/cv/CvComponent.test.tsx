import { createRef } from 'react';
import { render } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CvComponent } from './CvComponent';
import type { CvHandleRef, FaceLandmarkResult, LandmarkRunner } from './types';

function makeFakeVideo(): HTMLVideoElement {
  return document.createElement('video');
}

function makeRunner(result: FaceLandmarkResult): LandmarkRunner {
  return { detectForVideo: vi.fn().mockReturnValue(result) };
}

// Stub RAF so we can drive the frame loop deterministically and inspect what gets scheduled.
let rafCallbacks: FrameRequestCallback[] = [];

beforeEach(() => {
  rafCallbacks = [];
  vi.stubGlobal(
    'requestAnimationFrame',
    vi.fn((cb: FrameRequestCallback) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length;
    }),
  );
  vi.stubGlobal('cancelAnimationFrame', vi.fn());
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('CvComponent', () => {
  it('criterion 7: unmounting stops the engine even if the consumer never called stop()', () => {
    const runner = makeRunner({ faceLandmarks: [] });
    const ref = createRef<CvHandleRef>();
    const { unmount } = render(<CvComponent ref={ref} runner={runner} />);

    ref.current!.start(makeFakeVideo());
    expect(ref.current!.getState()).toBe('calibrating');
    expect(rafCallbacks.length).toBe(1);

    // Tick one frame manually — the engine is alive and processes it, scheduling the next tick.
    rafCallbacks[0](0);
    expect(runner.detectForVideo).toHaveBeenCalledTimes(1);
    expect(rafCallbacks.length).toBe(2);

    unmount();

    // Unmount must invoke engine.stop() — the RAF loop is cancelled. (React detaches the ref on
    // unmount, so we can no longer call getState() through it — we assert via cancelAnimationFrame
    // and, below, via the fact that no further ticks reach the runner.)
    expect(cancelAnimationFrame).toHaveBeenCalled();

    // Simulate the browser firing the already-queued RAF callback anyway (a frame that was
    // in flight at unmount time) — the engine must ignore it because stop() already flipped
    // it back to idle. No further detectForVideo ticks after unmount.
    rafCallbacks[1](33);
    expect(runner.detectForVideo).toHaveBeenCalledTimes(1);
  });

  it('fails-on-violation: without the unmount cleanup, the engine would keep ticking after unmount', () => {
    // This asserts the observable contract the cleanup effect guarantees: after unmount, a
    // stray queued RAF tick must NOT reach the runner. If the `useEffect` cleanup (criterion 7)
    // were removed, `engine.stop()` would never be called on unmount, the engine would still be
    // in 'running'/'calibrating' state, and this queued tick WOULD call detectForVideo again —
    // flipping this expectation to failing.
    const runner = makeRunner({ faceLandmarks: [] });
    const ref = createRef<CvHandleRef>();
    const { unmount } = render(<CvComponent ref={ref} runner={runner} />);

    ref.current!.start(makeFakeVideo());
    rafCallbacks[0](0); // first tick — schedules the next one
    const pendingTick = rafCallbacks[rafCallbacks.length - 1];
    const callsBeforeUnmount = vi.mocked(runner.detectForVideo).mock.calls.length;

    unmount();
    pendingTick(33);

    expect(vi.mocked(runner.detectForVideo).mock.calls.length).toBe(callsBeforeUnmount);
  });

  it('unmounting without ever calling start() does not throw', () => {
    const runner = makeRunner({ faceLandmarks: [] });
    const ref = createRef<CvHandleRef>();
    const { unmount } = render(<CvComponent ref={ref} runner={runner} />);

    expect(() => unmount()).not.toThrow();
  });

  it('renders null and exposes a ref handle', () => {
    const runner = makeRunner({ faceLandmarks: [] });
    const ref = createRef<CvHandleRef>();
    const { container } = render(<CvComponent ref={ref} runner={runner} />);

    expect(container.firstChild).toBeNull();
    expect(ref.current).not.toBeNull();
  });
});
