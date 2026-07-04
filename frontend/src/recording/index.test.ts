import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { RecordingEngineImpl, submitWinClip } from './index';
import type {
  MediaRecorderLike,
  CaptureStreamFactory,
  MediaRecorderFactory,
  AudioContextFactory,
  AudioContextLike,
  AudioNodeLike,
  MediaStreamAudioDestinationNodeLike,
  MediaStreamFactory,
} from './index';
import type { ClipsApi } from '../api/clips';
import { EDIT_SLOT_DURATION_MS_DEFAULT } from '../canvas';

// ---------------------------------------------------------------------------
// Test doubles — mirror rtc/rtc.test.ts's MockMediaStream/MockMediaStreamTrack
// pattern; never touch real browser media APIs.
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

  return {
    recorder,
    audioCtx,
    videoStream,
    captureStreamFactory,
    mediaRecorderFactory,
    audioContextFactory,
  };
}

// Mirrors the real `new MediaStream(tracks)` default — jsdom doesn't implement the MediaStream
// constructor, so tests must inject this like the other factories.
function makeMediaStreamFactory(): MediaStreamFactory {
  return vi.fn((tracks) => makeStream(tracks as unknown as MockMediaStreamTrack[]));
}

function makeCanvas(): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = 320;
  canvas.height = 240;
  // jsdom does not implement getContext('2d') without the optional 'canvas' npm package —
  // stub it so the edit-slot placeholder (src/canvas) has something to draw onto.
  const ctx = { fillStyle: '', fillRect: vi.fn() } as unknown as CanvasRenderingContext2D;
  vi.spyOn(canvas, 'getContext').mockReturnValue(ctx);
  return canvas;
}

describe('RecordingEngineImpl', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // criterion: 1 — ring-buffer-then-capture: startRingBuffer() followed by captureWin() produces
  // a WebM Blob.
  it('ring-buffer-then-capture: startRingBuffer then captureWin resolves with a WebM Blob', async () => {
    const { captureStreamFactory, mediaRecorderFactory, audioContextFactory } = makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    const canvas = makeCanvas();

    engine.startRingBuffer(canvas);
    const capturePromise = engine.captureWin();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    const blob = await capturePromise;

    expect(blob).toBeInstanceOf(Blob);
    expect(blob.type).toBe('video/webm');
    expect(blob.size).toBeGreaterThan(0);
  });

  // criterion: 1 (violation guard) — heavy encoding must NOT happen at startRingBuffer() time; the
  // ring buffer stays light. captureStreamFactory/mediaRecorderFactory/audioContextFactory must
  // only be invoked once captureWin() runs.
  it('heavy-encoding-not-at-ring-buffer-time: startRingBuffer does not call any factory; only captureWin does', async () => {
    const { captureStreamFactory, mediaRecorderFactory, audioContextFactory } = makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    const canvas = makeCanvas();

    engine.startRingBuffer(canvas);

    expect(captureStreamFactory).not.toHaveBeenCalled();
    expect(mediaRecorderFactory).not.toHaveBeenCalled();
    expect(audioContextFactory).not.toHaveBeenCalled();

    const capturePromise = engine.captureWin();

    expect(captureStreamFactory).toHaveBeenCalledTimes(1);
    expect(mediaRecorderFactory).toHaveBeenCalledTimes(1);
    expect(audioContextFactory).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise;
  });

  // criterion: 2 — the edit-slot placeholder plays for ~10s (the default) as part of captureWin();
  // the recorder must still be running right before the duration elapses and stopped right after.
  it('~10s-edit-slot-timing: the recorder stays active until ~10s elapse, then stops', async () => {
    const { recorder, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    engine.startRingBuffer(makeCanvas());

    const capturePromise = engine.captureWin();
    expect(recorder.start).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT - 1);
    expect(recorder.stop).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    await capturePromise;
    expect(recorder.stop).toHaveBeenCalledTimes(1);
  });

  // criterion: 2 (violation guard) — an injected editSlotDurationMs overrides the ~10s default,
  // proving the duration is actually threaded through to the edit slot rather than hardcoded.
  it('~10s-edit-slot-timing violation guard: a custom editSlotDurationMs overrides the ~10s default', async () => {
    const { recorder, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
      editSlotDurationMs: 1500,
    });
    engine.startRingBuffer(makeCanvas());

    const capturePromise = engine.captureWin();

    await vi.advanceTimersByTimeAsync(1499);
    expect(recorder.stop).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    await capturePromise;
    expect(recorder.stop).toHaveBeenCalledTimes(1);
  });

  // criterion: 2 — an audio track (placeholder "TikTok track") is mixed via an AudioContext into
  // the captured stream: the factory is invoked, a destination created, a placeholder source
  // connected to it, and the destination's track added onto the recorded stream.
  it('audio-mix: mixes a placeholder audio track via AudioContext into the captured stream', async () => {
    const { audioCtx, videoStream, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    engine.startRingBuffer(makeCanvas());

    const capturePromise = engine.captureWin();

    expect(audioContextFactory).toHaveBeenCalledTimes(1);
    expect(audioCtx.createMediaStreamDestination).toHaveBeenCalledTimes(1);
    expect(audioCtx.createOscillator).toHaveBeenCalledTimes(1);
    expect(audioCtx.oscillator.connect).toHaveBeenCalledWith(
      expect.objectContaining({ stream: expect.anything() }),
    );
    // The destination's track must have been added onto the recorded video stream.
    expect(videoStream.getTracks()).toContain(audioCtx.destinationTrack);

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise;
    expect(audioCtx.close).toHaveBeenCalled();
  });

  // criterion: 2 (violation guard) — without mixing, the recorded stream would only carry the
  // video track(s); this asserts the audio destination's track is genuinely present afterward.
  it('audio-mix violation guard: the recorded stream carries both the video track and the mixed audio track', async () => {
    const { audioCtx, videoStream, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    engine.startRingBuffer(makeCanvas());

    const capturePromise = engine.captureWin();
    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise;

    const finalTracks = videoStream.getTracks();
    expect(finalTracks.length).toBeGreaterThanOrEqual(2);
    expect(finalTracks).toContain(audioCtx.destinationTrack);
  });

  // criterion: 4 — captureWin() with a MediaStream source (not a canvas) skips captureStreamFactory
  // entirely and still waits out the edit-slot duration before resolving.
  it('accepts a MediaStream source directly, without calling captureStreamFactory', async () => {
    const { mediaRecorderFactory, audioContextFactory } = makeFactories();
    const captureStreamFactory: CaptureStreamFactory = vi.fn(() => makeStream());
    const mediaStreamFactory = makeMediaStreamFactory();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
      mediaStreamFactory,
    });

    const directStream = makeStream([new MockMediaStreamTrack()]);
    engine.startRingBuffer(directStream);

    const capturePromise = engine.captureWin();
    expect(captureStreamFactory).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    const blob = await capturePromise;
    expect(blob).toBeInstanceOf(Blob);
  });

  // criterion: bug fix — a MediaStream source is the CALLER'S OWN stream object (e.g. the same
  // stream also live on an active WebRTC call via RtcPeerImpl). captureWin() must never mutate it
  // (no addTrack against the caller's stream) — the mixed audio track must land on a brand-new
  // stream built via mediaStreamFactory instead.
  it('media-stream-source-no-mutate: captureWin never calls addTrack on the caller-supplied MediaStream', async () => {
    const { mediaRecorderFactory, audioContextFactory } = makeFactories();
    const captureStreamFactory: CaptureStreamFactory = vi.fn(() => makeStream());
    const mediaStreamFactory = makeMediaStreamFactory();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
      mediaStreamFactory,
    });

    const originalStream = makeStream([new MockMediaStreamTrack()]) as unknown as MockMediaStream;
    const addTrackSpy = vi.spyOn(originalStream, 'addTrack');

    engine.startRingBuffer(originalStream as unknown as MediaStream);
    const capturePromise = engine.captureWin();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise;

    expect(addTrackSpy).not.toHaveBeenCalled();
    // The caller's stream must still only carry the track it started with — untouched.
    expect(originalStream.getTracks()).toHaveLength(1);
  });

  // criterion: bug fix (violation guard) — stop() with a MediaStream source must stop only the
  // engine's own placeholder audio track and must NOT stop the caller's original tracks (e.g. the
  // shared camera/mic tracks of a live WebRTC call) — the caller owns their lifecycle.
  it('media-stream-source-stop-scope: stop() stops the engine-owned audio track but not the caller-supplied MediaStream tracks', async () => {
    const { audioCtx, mediaRecorderFactory, audioContextFactory } = makeFactories();
    const captureStreamFactory: CaptureStreamFactory = vi.fn(() => makeStream());
    const mediaStreamFactory = makeMediaStreamFactory();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
      mediaStreamFactory,
    });

    const originalTrack = new MockMediaStreamTrack();
    const originalStream = makeStream([originalTrack]);
    engine.startRingBuffer(originalStream);

    const capturePromise = engine.captureWin();
    engine.stop();

    expect(originalTrack.stop).not.toHaveBeenCalled();
    expect(audioCtx.destinationTrack.stop).toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise.catch(() => undefined);
    // Still untouched after the in-flight capture settles.
    expect(originalTrack.stop).not.toHaveBeenCalled();
  });

  // criterion: 1 (violation guard) — captureWin() called before startRingBuffer() must fail loudly
  // rather than silently producing an empty/garbage blob.
  it('captureWin throws if called before startRingBuffer', async () => {
    const engine = new RecordingEngineImpl();
    await expect(engine.captureWin()).rejects.toThrow(
      'captureWin() called before startRingBuffer()',
    );
  });

  // criterion: stop() tears down a live recorder/stream/audio context without throwing.
  it('stop tears down the recorder, stream tracks, and audio context', async () => {
    const { recorder, videoStream, captureStreamFactory, mediaRecorderFactory, audioContextFactory } =
      makeFactories();
    const engine = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
    });
    engine.startRingBuffer(makeCanvas());

    const capturePromise = engine.captureWin();
    // Interrupt mid-flight, before the edit slot completes on its own.
    recorder.state = 'recording';
    engine.stop();

    expect(recorder.stop).toHaveBeenCalled();
    for (const track of videoStream.getTracks()) {
      expect(track.stop).toHaveBeenCalled();
    }

    // Let the in-flight captureWin settle so it doesn't leak a dangling timer/rejection.
    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await capturePromise.catch(() => undefined);
  });

  it('stop is a no-op (does not throw) when nothing was ever captured', () => {
    const engine = new RecordingEngineImpl();
    expect(() => engine.stop()).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// submitWinClip — upload-then-convert orchestration; separate from captureWin so a loser can
// skip submission entirely.
// ---------------------------------------------------------------------------

describe('captureWin / submitWinClip decoupling (loser-skip path)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // criterion: 3 — the loser path keeps its own recorded version with a skip option: both sides of a
  // match run captureWin() independently and each get their own Blob, but only the winner's Blob is
  // ever handed to submitWinClip — the loser's Blob is kept locally and never touches the ClipsApi.
  it('loser-skip: only the winner Blob is submitted; the loser Blob never reaches uploadClip/convertClip', async () => {
    const winnerFactories = makeFactories();
    const winnerEngine = new RecordingEngineImpl({
      captureStreamFactory: winnerFactories.captureStreamFactory,
      mediaRecorderFactory: winnerFactories.mediaRecorderFactory,
      audioContextFactory: winnerFactories.audioContextFactory,
    });
    const loserFactories = makeFactories();
    const loserEngine = new RecordingEngineImpl({
      captureStreamFactory: loserFactories.captureStreamFactory,
      mediaRecorderFactory: loserFactories.mediaRecorderFactory,
      audioContextFactory: loserFactories.audioContextFactory,
    });

    winnerEngine.startRingBuffer(makeCanvas());
    loserEngine.startRingBuffer(makeCanvas());
    const winnerCapture = winnerEngine.captureWin();
    const loserCapture = loserEngine.captureWin();
    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    const winnerBlob = await winnerCapture;
    const loserBlob = await loserCapture;

    const uploadClip = vi.fn().mockResolvedValue({ id: 'clip-winner' });
    const convertClip = vi.fn().mockResolvedValue(undefined);
    const clipsApi = { uploadClip, convertClip } as unknown as ClipsApi;

    // The app submits ONLY the winner's blob — the loser keeps its own recorded version locally
    // and simply never calls submitWinClip for it (its "skip option").
    const result = await submitWinClip(winnerBlob, clipsApi);

    expect(result).toEqual({ id: 'clip-winner' });
    expect(uploadClip).toHaveBeenCalledTimes(1);
    // Reference identity, not deep-equality — the mock recorders happen to produce
    // byte-identical chunks, so this proves it's genuinely the winner's Blob object that was
    // submitted (and not, say, the loser's) rather than two Blobs that merely look alike.
    expect(uploadClip.mock.calls[0][0]).toBe(winnerBlob);
    expect(uploadClip.mock.calls[0][0]).not.toBe(loserBlob);
    expect(convertClip).toHaveBeenCalledTimes(1);
  });
});

describe('submitWinClip', () => {
  // criterion: 3 — on a win, the blob is uploaded via clipsApi.uploadClip, then MP4 conversion is
  // requested via clipsApi.convertClip(id), and the id is returned.
  it('upload-then-convert: calls uploadClip then convertClip with the returned id, and returns it', async () => {
    const uploadClip = vi.fn().mockResolvedValue({ id: 'clip-abc' });
    const convertClip = vi.fn().mockResolvedValue(undefined);
    const clipsApi = { uploadClip, convertClip } as unknown as ClipsApi;
    const blob = new Blob(['x'], { type: 'video/webm' });

    const result = await submitWinClip(blob, clipsApi);

    expect(uploadClip).toHaveBeenCalledWith(blob);
    expect(convertClip).toHaveBeenCalledWith('clip-abc');
    expect(result).toEqual({ id: 'clip-abc' });
  });

  // criterion: 3 (violation guard) — convertClip must be called AFTER uploadClip resolves (using
  // the id it returned), not before/independently — proves the two calls are properly sequenced.
  it('upload-then-convert violation guard: convertClip is called with the id from uploadClip, not before it resolves', async () => {
    const callOrder: string[] = [];
    const uploadClip = vi.fn().mockImplementation(async () => {
      callOrder.push('upload');
      return { id: 'clip-xyz' };
    });
    const convertClip = vi.fn().mockImplementation(async (id: string) => {
      callOrder.push(`convert:${id}`);
    });
    const clipsApi = { uploadClip, convertClip } as unknown as ClipsApi;

    await submitWinClip(new Blob(['x']), clipsApi);

    expect(callOrder).toEqual(['upload', 'convert:clip-xyz']);
  });

  // criterion: 3 — errors from uploadClip propagate to the caller (the caller can choose not to
  // submit / retry — upload is never silently swallowed).
  it('propagates errors from uploadClip', async () => {
    const uploadClip = vi.fn().mockRejectedValue(new Error('upload failed'));
    const convertClip = vi.fn();
    const clipsApi = { uploadClip, convertClip } as unknown as ClipsApi;

    await expect(submitWinClip(new Blob(['x']), clipsApi)).rejects.toThrow('upload failed');
    expect(convertClip).not.toHaveBeenCalled();
  });

  // criterion: 3 — errors from convertClip also propagate to the caller.
  it('propagates errors from convertClip', async () => {
    const uploadClip = vi.fn().mockResolvedValue({ id: 'clip-abc' });
    const convertClip = vi.fn().mockRejectedValue(new Error('convert failed'));
    const clipsApi = { uploadClip, convertClip } as unknown as ClipsApi;

    await expect(submitWinClip(new Blob(['x']), clipsApi)).rejects.toThrow('convert failed');
  });
});
