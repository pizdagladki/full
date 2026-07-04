import { createRef } from 'react';
import { render } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RecordingComponent } from './index';
import type {
  RecordingHandle,
  CaptureStreamFactory,
  MediaRecorderFactory,
  AudioContextFactory,
  MediaRecorderLike,
  AudioContextLike,
  AudioNodeLike,
  MediaStreamAudioDestinationNodeLike,
} from './index';
import { EDIT_SLOT_DURATION_MS_DEFAULT } from '../canvas';

// ---------------------------------------------------------------------------
// Minimal test doubles — mirror rtc/rtc-component.test.tsx's Mock* pattern;
// never touch real browser media APIs.
// ---------------------------------------------------------------------------

class MockMediaStreamTrack {
  stop = vi.fn();
}

class MockMediaStream {
  private tracks: MockMediaStreamTrack[];

  constructor(tracks?: MockMediaStreamTrack[]) {
    this.tracks = tracks ?? [];
  }

  getTracks(): MockMediaStreamTrack[] {
    return this.tracks;
  }

  addTrack(track: MockMediaStreamTrack): void {
    this.tracks.push(track);
  }
}

function makeStream(tracks?: MockMediaStreamTrack[]): MediaStream {
  return new MockMediaStream(tracks) as unknown as MediaStream;
}

class MockMediaRecorder implements MediaRecorderLike {
  state: 'inactive' | 'recording' | 'paused' = 'inactive';
  private _ondataavailable: ((ev: { data: Blob }) => void) | null = null;
  private _onstop: (() => void) | null = null;

  start = vi.fn(() => {
    this.state = 'recording';
  });

  stop = vi.fn(() => {
    this.state = 'inactive';
    this._ondataavailable?.({ data: new Blob(['chunk'], { type: 'video/webm' }) });
    this._onstop?.();
  });

  set ondataavailable(cb: ((ev: { data: Blob }) => void) | null) {
    this._ondataavailable = cb;
  }
  set onstop(cb: (() => void) | null) {
    this._onstop = cb;
  }
}

class MockAudioNode implements AudioNodeLike {
  connect = vi.fn();
  start = vi.fn();
  stop = vi.fn();
}

class MockAudioContext implements AudioContextLike {
  close = vi.fn();
  destinationTrack = new MockMediaStreamTrack();
  oscillator = new MockAudioNode();

  createOscillator = vi.fn((): AudioNodeLike => this.oscillator);
  createMediaStreamDestination = vi.fn(
    (): MediaStreamAudioDestinationNodeLike => ({
      stream: makeStream([this.destinationTrack]),
    }),
  );
}

function makeFactories() {
  const recorder = new MockMediaRecorder();
  const audioCtx = new MockAudioContext();
  const videoStream = makeStream([new MockMediaStreamTrack()]);

  const captureStreamFactory: CaptureStreamFactory = vi.fn(() => videoStream);
  const mediaRecorderFactory: MediaRecorderFactory = vi.fn(() => recorder);
  const audioContextFactory: AudioContextFactory = vi.fn(() => audioCtx);

  return { recorder, audioCtx, videoStream, captureStreamFactory, mediaRecorderFactory, audioContextFactory };
}

function makeCanvas(): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = 320;
  canvas.height = 240;
  const ctx = { fillStyle: '', fillRect: vi.fn() } as unknown as CanvasRenderingContext2D;
  vi.spyOn(canvas, 'getContext').mockReturnValue(ctx);
  return canvas;
}

describe('RecordingComponent', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // criterion: 1 — RecordingComponent exposes an imperative handle (startRingBuffer/captureWin/stop)
  // via ref, wired through to the underlying engine.
  it('exposes startRingBuffer/captureWin/stop via ref, producing a WebM Blob', async () => {
    const { captureStreamFactory, mediaRecorderFactory, audioContextFactory } = makeFactories();
    const ref = createRef<RecordingHandle>();
    render(
      <RecordingComponent
        ref={ref}
        captureStreamFactory={captureStreamFactory}
        mediaRecorderFactory={mediaRecorderFactory}
        audioContextFactory={audioContextFactory}
      />,
    );

    ref.current!.startRingBuffer(makeCanvas());
    const capturePromise = ref.current!.captureWin();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    const blob = await capturePromise;

    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe('video/webm');
  });

  // criterion: 1 — the component renders null (heavy media work stays outside React's render
  // path) and exposes a ref handle.
  it('renders null and exposes a ref handle', () => {
    const { captureStreamFactory, mediaRecorderFactory, audioContextFactory } = makeFactories();
    const ref = createRef<RecordingHandle>();
    const { container } = render(
      <RecordingComponent
        ref={ref}
        captureStreamFactory={captureStreamFactory}
        mediaRecorderFactory={mediaRecorderFactory}
        audioContextFactory={audioContextFactory}
      />,
    );

    expect(container.firstChild).toBeNull();
    expect(ref.current).not.toBeNull();
  });

  // criterion: 1 (violation guard) — calling captureWin() via the ref before startRingBuffer()
  // must reject rather than silently resolve.
  it('captureWin via ref rejects if called before startRingBuffer', async () => {
    const ref = createRef<RecordingHandle>();
    render(<RecordingComponent ref={ref} />);

    await expect(ref.current!.captureWin()).rejects.toThrow(
      'captureWin() called before startRingBuffer()',
    );
  });

  // criterion: 1 — unmounting the component tears down any live recorder/stream/audio context
  // (stop() is invoked on cleanup), mirroring RtcComponent's unmount teardown.
  it('unmounting tears down a live recording (stop is invoked)', async () => {
    const { recorder, videoStream, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const ref = createRef<RecordingHandle>();
    const { unmount } = render(
      <RecordingComponent
        ref={ref}
        captureStreamFactory={captureStreamFactory}
        mediaRecorderFactory={mediaRecorderFactory}
        audioContextFactory={audioContextFactory}
      />,
    );

    ref.current!.startRingBuffer(makeCanvas());
    const capturePromise = ref.current!.captureWin();
    recorder.state = 'recording';

    unmount();

    expect(recorder.stop).toHaveBeenCalled();
    for (const track of videoStream.getTracks()) {
      expect(track.stop).toHaveBeenCalled();
    }

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise.catch(() => undefined);
  });

  // criterion: 1 (violation guard) — stop() via the ref does not throw when nothing was captured.
  it('stop via ref is a no-op when nothing was ever captured', () => {
    const ref = createRef<RecordingHandle>();
    render(<RecordingComponent ref={ref} />);

    expect(() => ref.current!.stop()).not.toThrow();
  });
});
