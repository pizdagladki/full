import { forwardRef, useEffect, useImperativeHandle } from 'react';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Battle } from './Battle';
import type { BattleProps } from './Battle';
import type { WsClientApi } from '../api/ws';
import type { ClipsApi } from '../api/clips';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { defaultCvRunner, __resetDefaultCvRunnerForTests } from '../cv';
import type { PcLike, WsLike, PcFactory, WsFactory } from '../rtc';
import { RecordingComponent } from '../recording';
import type {
  RecordingHandle,
  RecordingComponentProps,
  MediaRecorderLike,
  AudioContextLike,
  AudioNodeLike,
  MediaStreamAudioDestinationNodeLike,
  MediaRecorderFactory,
  AudioContextFactory,
  MediaStreamFactory,
} from '../recording';
import { EDIT_SLOT_DURATION_MS_DEFAULT } from '../canvas';

// ---------------------------------------------------------------------------
// Mock arbitration WS client — captures onMessage cb so tests can fire server
// frames directly (mirrors Search.test.tsx's makeMockWs).
// ---------------------------------------------------------------------------

function makeMockWs(): { ws: WsClientApi; fireMessage: (data: string) => void } {
  let msgCb: ((data: string) => void) | undefined;
  const ws: WsClientApi = {
    connect: vi.fn(),
    send: vi.fn(),
    close: vi.fn(),
    onMessage: vi.fn((cb: (data: string) => void) => {
      msgCb = cb;
    }),
    onOpen: vi.fn(),
    onClose: vi.fn(),
  };
  return { ws, fireMessage: (data: string) => msgCb?.(data) };
}

/** Extracts every parsed JSON payload sent via ws.send. */
function sentMessages(ws: WsClientApi): unknown[] {
  return vi.mocked(ws.send).mock.calls.map((call) => JSON.parse(call[0] as string) as unknown);
}

// ---------------------------------------------------------------------------
// Fake Cv mount — a stand-in for the real `CvComponent`. Battle mounts the
// real CvComponent in production; this test seam (`cvComponent` prop) lets us
// fire onFacePresent/onBlink/onFaceLost on demand instead of driving CvEngine's
// EAR/calibration math under fake timers (see Battle.tsx's doc comment on the
// prop for why — this is the officially sanctioned lighter-weight approach).
// ---------------------------------------------------------------------------

function makeFakeCv(): {
  Cv: NonNullable<BattleProps['cvComponent']>;
  start: ReturnType<typeof vi.fn>;
  fireFacePresent: () => void;
  fireFaceLost: () => void;
  fireBlink: () => void;
  /** The `runner` prop Battle actually passed down — captured for the default-wiring test below. */
  getCapturedRunner: () => LandmarkRunner | undefined;
} {
  let cb: CvCallbacks = {};
  let capturedRunner: LandmarkRunner | undefined;
  const start = vi.fn();
  const Cv = forwardRef<CvHandleRef, { runner: LandmarkRunner; callbacks?: CvCallbacks }>(
    ({ runner, callbacks }, ref) => {
      // Capture the (stable, useMemo'd []) callbacks prop as a side effect rather than during
      // render, so the fake component still obeys the "render must be pure" rule.
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
// Mock rtc wsFactory/pcFactory (mirrors rtc/rtc-component.test.tsx) — used
// only by the split/remote-stream test, which drives the real RtcComponent.
// ---------------------------------------------------------------------------

class MockWebSocket implements WsLike {
  send = vi.fn();
  close = vi.fn();
  set onopen(_cb: (() => void) | null) {
    // no-op — not exercised here
  }
  set onmessage(_cb: ((ev: { data: string }) => void) | null) {
    // no-op — not exercised here
  }
}

class MockRTCPeerConnection implements PcLike {
  addTrack = vi.fn();
  close = vi.fn();
  createOffer = vi.fn().mockResolvedValue({ type: 'offer', sdp: '' } as RTCSessionDescriptionInit);
  createAnswer = vi.fn().mockResolvedValue({ type: 'answer', sdp: '' } as RTCSessionDescriptionInit);
  setLocalDescription = vi.fn().mockResolvedValue(undefined);
  setRemoteDescription = vi.fn().mockResolvedValue(undefined);
  addIceCandidate = vi.fn().mockResolvedValue(undefined);
  private trackCb: ((ev: RTCTrackEvent) => void) | null = null;
  set onnegotiationneeded(_cb: (() => void) | null) {
    // no-op — not exercised here
  }
  set onicecandidate(_cb: ((ev: { candidate: RTCIceCandidate | null }) => void) | null) {
    // no-op — not exercised here
  }
  set ontrack(cb: ((ev: RTCTrackEvent) => void) | null) {
    this.trackCb = cb;
  }
  fireTrack(stream: MediaStream): void {
    this.trackCb?.({ streams: [stream] } as unknown as RTCTrackEvent);
  }
}

class MockMediaStreamTrack {
  kind = 'video';
  stop = vi.fn();
}

class MockMediaStream {
  private tracks: MockMediaStreamTrack[];
  constructor() {
    this.tracks = [new MockMediaStreamTrack()];
  }
  getTracks(): MockMediaStreamTrack[] {
    return this.tracks;
  }
}

/**
 * RtcComponent/wsFactory/pcFactory overrides so tests that resolve a real local stream (needed to
 * exercise the ring buffer / win-clip recording flow below) never construct a real browser
 * WebSocket/RTCPeerConnection — mirrors the "remote-stream-attached" test above.
 */
function rtcMockFactories(): { rtcWsFactory: WsFactory; rtcPcFactory: PcFactory } {
  return {
    rtcWsFactory: (() => new MockWebSocket()) as WsFactory,
    rtcPcFactory: () => new MockRTCPeerConnection(),
  };
}

// ---------------------------------------------------------------------------
// Mock recording component (mirrors makeFakeCv) — a stand-in for the real
// `RecordingComponent`. Battle mounts the real one in production; this seam
// (`recordingComponent` prop) lets tests drive/assert startRingBuffer/
// captureWin/stop without touching real MediaRecorder/AudioContext APIs.
// ---------------------------------------------------------------------------

function makeFakeRecording(): {
  Recording: NonNullable<BattleProps['recordingComponent']>;
  startRingBuffer: ReturnType<typeof vi.fn>;
  captureWin: ReturnType<typeof vi.fn>;
  stop: ReturnType<typeof vi.fn>;
} {
  const startRingBuffer = vi.fn();
  const captureWin = vi.fn();
  const stop = vi.fn();
  const Recording = forwardRef<RecordingHandle, RecordingComponentProps>(
    function FakeRecording(_props, ref) {
      useImperativeHandle(ref, () => ({ startRingBuffer, captureWin, stop }));
      return null;
    },
  );
  return { Recording, startRingBuffer, captureWin, stop };
}

// ---------------------------------------------------------------------------
// Real recording engine, wired with injected factories — used by the "genuine
// wiring" test that exercises the actual ~10s edit-slot wait via fake timers
// (jsdom has no MediaRecorder/AudioContext/MediaStream, so all three engine
// factories must be injected; mirrors recording/index.test.ts's pattern).
// ---------------------------------------------------------------------------

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
  oscillator = new MockAudioNode();
  createOscillator = vi.fn((): AudioNodeLike => this.oscillator);
  createMediaStreamDestination = vi.fn(
    (): MediaStreamAudioDestinationNodeLike => ({
      stream: new MockMediaStream() as unknown as MediaStream,
    }),
  );
}

/** Mirrors the real `new MediaStream(tracks)` default — jsdom doesn't implement it. */
function makeMediaStreamFactory(): MediaStreamFactory {
  return vi.fn((tracks) => ({ getTracks: () => tracks }) as unknown as MediaStream);
}

function makeRealRecording(): {
  Recording: NonNullable<BattleProps['recordingComponent']>;
  recorder: MockMediaRecorder;
} {
  const recorder = new MockMediaRecorder();
  const audioCtx = new MockAudioContext();
  const mediaRecorderFactory: MediaRecorderFactory = () => recorder;
  const audioContextFactory: AudioContextFactory = () => audioCtx;
  const mediaStreamFactory = makeMediaStreamFactory();
  const Recording = forwardRef<RecordingHandle, RecordingComponentProps>(
    function RealRecording(_props, ref) {
      return (
        <RecordingComponent
          ref={ref}
          mediaRecorderFactory={mediaRecorderFactory}
          audioContextFactory={audioContextFactory}
          mediaStreamFactory={mediaStreamFactory}
        />
      );
    },
  );
  return { Recording, recorder };
}

// ---------------------------------------------------------------------------
// Mock ClipsApi — captures upload/convert calls (mirrors the real ClipsApi
// shape); tests assert on these instead of touching real fetch/network.
// ---------------------------------------------------------------------------

function makeMockClipsApi(clipId = 'clip-1'): ClipsApi {
  return {
    getClips: vi.fn().mockResolvedValue([]),
    getClipDownloadUrl: vi.fn((id: string) => `https://clips.example/v1/clips/${id}/download`),
    uploadClip: vi.fn().mockResolvedValue({ id: clipId }),
    convertClip: vi.fn().mockResolvedValue(undefined),
  };
}

// ---------------------------------------------------------------------------
// Guarded getUserMedia stub — mirrors Search/Home's pattern so the local
// stream resolves and streamRef.current is set (needed to start the ring
// buffer and to feed the win-clip recording flow).
// ---------------------------------------------------------------------------

function stubGetUserMedia(stream: MediaStream = new MockMediaStream() as unknown as MediaStream) {
  Object.defineProperty(globalThis.navigator, 'mediaDevices', {
    value: { getUserMedia: vi.fn().mockResolvedValue(stream) },
    writable: true,
    configurable: true,
  });
}

/** Flushes the getUserMedia().then(...) microtask so streamRef.current settles. */
async function flushGetUserMedia(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

// ---------------------------------------------------------------------------
// Routing harness
// ---------------------------------------------------------------------------

function ResultsProbe() {
  const location = useLocation();
  const state = location.state as
    | {
        result?: string;
        durationMs?: number;
        winnerId?: number;
        loserId?: number;
        mp4Url?: string;
        trackId?: string;
      }
    | null;
  return (
    <div data-testid="results-probe">
      <span data-testid="results-result">{state?.result}</span>
      <span data-testid="results-duration">{state?.durationMs}</span>
      <span data-testid="results-winner">{state?.winnerId}</span>
      <span data-testid="results-loser">{state?.loserId}</span>
      <span data-testid="results-mp4-url">{state?.mp4Url}</span>
      <span data-testid="results-track-id">{state?.trackId}</span>
    </div>
  );
}

function renderBattle(
  props: BattleProps,
  locationState: Record<string, unknown> = { roomId: 'room-1' },
) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/battle', state: locationState }]}>
      <Routes>
        <Route path="/battle" element={<Battle {...props} />} />
        <Route path="/results" element={<ResultsProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

/**
 * Fires face-present, then advances the sanity check (2000ms default) and the countdown (5000ms
 * default) to reach the battle phase.
 */
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

describe('Battle', () => {
  // criterion: 1 — mounts cv/rtc via refs (cv.start called against the local video element) and
  // renders a split layout with both a local and a remote video.
  it('mounts-cv-rtc-via-refs: renders the split layout and starts the cv engine against local-video', () => {
    const { ws } = makeMockWs();
    const { Cv, start } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: 'u1' });

    expect(screen.getByTestId('battle-screen')).toBeInTheDocument();
    expect(screen.getByTestId('local-video')).toBeInTheDocument();
    expect(screen.getByTestId('remote-video')).toBeInTheDocument();
    expect(start).toHaveBeenCalledWith(screen.getByTestId('local-video'));
    expect(ws.connect).toHaveBeenCalledWith('/ws/signal');
  });

  // criterion: 1 — the rtc remote stream is attached to the right-hand video once it arrives.
  it('remote-stream-attached: a stream from rtc.onRemoteStream is attached to remote-video', async () => {
    Object.defineProperty(globalThis.navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockResolvedValue(new MockMediaStream()) },
      writable: true,
      configurable: true,
    });

    const { ws } = makeMockWs();
    const { Cv } = makeFakeCv();
    const pc = new MockRTCPeerConnection();
    const wsFactory: WsFactory = (() => new MockWebSocket()) as WsFactory;
    const pcFactory: PcFactory = () => pc;

    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      currentUserId: 'u1',
      rtcWsFactory: wsFactory,
      rtcPcFactory: pcFactory,
    });

    // Flush the getUserMedia().then(...) microtask so rtc.connect() runs.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(pc.addTrack).toHaveBeenCalled();

    const remoteStream = new MockMediaStream() as unknown as MediaStream;
    act(() => {
      pc.fireTrack(remoteStream);
    });

    const remoteVideo = screen.getByTestId('remote-video') as HTMLVideoElement;
    expect(remoteVideo.srcObject).toBe(remoteStream);
  });

  // criterion: 2 — a 2-second sanity check runs BEFORE the countdown; with no face present it
  // cancels the battle as a loss and routes to results — the countdown must never appear.
  it('sanity-check-fail-loss: no face present after 2s routes to /results as a loss, no countdown', () => {
    const { ws } = makeMockWs();
    const { Cv } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: 'u1' });

    // Never fire facePresent — no face is ever detected.
    act(() => {
      vi.advanceTimersByTime(2000);
    });

    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-result').textContent).toBe('loss');
    expect(screen.queryByTestId('countdown')).not.toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — with a face present at the 2s mark, the sanity check must
  // NOT cancel the battle: no navigation to /results, and the countdown appears instead.
  it('sanity-check-fail-loss violation guard: a face present at 2s does not route to a loss', () => {
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: 'u1' });

    act(() => {
      fireFacePresent();
    });
    act(() => {
      vi.advanceTimersByTime(2000);
    });

    expect(screen.queryByTestId('results-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('countdown')).toBeInTheDocument();
  });

  // criterion: 3 — a 5-second countdown placeholder runs AFTER the sanity check; the battle
  // starts when it reaches zero.
  it('countdown-then-start: after the sanity check passes, a 5s countdown runs then battle starts', () => {
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: 'u1' });

    act(() => {
      fireFacePresent();
    });
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByTestId('countdown').textContent).toBe('5');
    expect(screen.queryByTestId('battle-live')).not.toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(4000);
    });
    // Not yet zero — still counting down, battle has not started.
    expect(screen.getByTestId('countdown')).toBeInTheDocument();
    expect(screen.queryByTestId('battle-live')).not.toBeInTheDocument();

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();
    expect(screen.queryByTestId('countdown')).not.toBeInTheDocument();
  });

  // criterion: 4 — a local blink sends {type:"blink", room_id} over the signaling WS; on the
  // authoritative outcome frame the app eventually routes to results carrying the result + match
  // duration (#159: a win goes through the win-edit/capture flow first; a loss requires Skip).
  it.each([
    { name: 'win when winner_id matches the current user', winnerId: 7, loserId: 3, want: 'win' as const },
    {
      name: 'loss when winner_id does not match the current user',
      winnerId: 3,
      loserId: 7,
      want: 'loss' as const,
    },
  ])('blink-reported-and-outcome-routes: $name', async ({ winnerId, loserId, want }) => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    captureWin.mockResolvedValue(new Blob(['clip'], { type: 'video/webm' }));
    const clipsApi = makeMockClipsApi();
    renderBattle({ wsClient: ws, cvComponent: Cv, recordingComponent: Recording, clipsApi, currentUserId: '7' });

    reachBattle(fireFacePresent);

    act(() => {
      fireBlink();
    });
    expect(sentMessages(ws)).toContainEqual({ type: 'blink', room_id: 'room-1' });

    act(() => {
      vi.advanceTimersByTime(1234);
    });
    act(() => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: winnerId, loser_id: loserId }));
    });

    if (want === 'win') {
      // #159: a win first captures/uploads the clip — flush that microtask chain.
      await act(async () => {
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
    } else {
      // #159: a loss shows loss-edit with a Skip button — click it to reach /results.
      act(() => {
        screen.getByTestId('skip-edit').click();
      });
    }

    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-result').textContent).toBe(want);
    expect(screen.getByTestId('results-duration').textContent).toBe('1234');
  });

  // criterion: 4 (violation guard) — a malformed/non-outcome frame must NOT navigate away from
  // the battle screen.
  it('blink-reported-and-outcome-routes violation guard: a non-outcome frame does not navigate', () => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

    reachBattle(fireFacePresent);

    act(() => {
      fireMessage(JSON.stringify({ type: 'queued' }));
    });
    act(() => {
      fireMessage('not json');
    });

    expect(screen.queryByTestId('results-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('battle-live')).toBeInTheDocument();
  });

  // criterion: 5 — leaving the camera AFTER start is reported as a forfeit (face_lost).
  it('forfeit-on-face-loss: losing the face during battle sends {type:"face_lost", room_id}', () => {
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent, fireFaceLost } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

    reachBattle(fireFacePresent);

    act(() => {
      fireFaceLost();
    });

    expect(sentMessages(ws)).toContainEqual({ type: 'face_lost', room_id: 'room-1' });
  });

  // criterion: 5 (violation guard) — losing the face BEFORE the battle starts (during the
  // countdown) must NOT be reported as a forfeit — only a loss AFTER start counts.
  it('forfeit-on-face-loss violation guard: losing the face during the countdown does not forfeit', () => {
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent, fireFaceLost } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

    act(() => {
      fireFacePresent();
    });
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(screen.getByTestId('countdown')).toBeInTheDocument();

    act(() => {
      fireFaceLost();
    });

    expect(sentMessages(ws)).not.toContainEqual({ type: 'face_lost', room_id: 'room-1' });
  });

  // criteria: 1b/2b (default wiring regression guard) — with NO `cvRunner` prop supplied at all,
  // Battle must fall back to the real `defaultCvRunner()` singleton, NOT an inline no-face
  // placeholder. Driving a real face through CvEngine's full RAF/calibration pipeline via this
  // screen is impractical here: Battle.test.tsx deliberately swaps in a fake `cvComponent` (see its
  // doc comment) that never calls `runner.detectForVideo` at all, so a behavioral assertion on
  // face-present-driven UI would pass trivially regardless of which runner is wired. Instead this
  // asserts the referential-identity edge directly: seed the module singleton (reset + a sentinel
  // loader) BEFORE rendering, then confirm the exact `runner` prop Battle passed down to its cv
  // component IS that same singleton instance. If Battle.tsx's default were reverted to an inline
  // `{ detectForVideo: () => ({ faceLandmarks: [] }) }`, the captured runner would be a different
  // object and this identity check would fail.
  it('production-default-wiring: with no cvRunner prop, Battle wires the real defaultCvRunner() singleton', () => {
    __resetDefaultCvRunnerForTests();
    const singleton = defaultCvRunner(() => new Promise<LandmarkRunner>(() => {}));

    const { ws } = makeMockWs();
    const { Cv, getCapturedRunner } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: 'u1' });

    expect(getCapturedRunner()).toBe(singleton);
  });
});

// ---------------------------------------------------------------------------
// Issue #159 — wire win-clip recording + ~10s edit slot into battle→results
// ---------------------------------------------------------------------------

describe('Battle — #159 win-clip recording + edit slot', () => {
  // criterion: 1 — the ring buffer starts exactly once, once BOTH the battle phase has begun AND
  // the local stream is available (stream resolves first here; battle-start triggers the start).
  it('ring-buffer-start: starts the ring buffer against the local stream once battle begins', async () => {
    stubGetUserMedia();
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, startRingBuffer } = makeFakeRecording();
    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      currentUserId: 'u1',
      ...rtcMockFactories(),
    });

    await flushGetUserMedia();
    expect(startRingBuffer).not.toHaveBeenCalled();

    reachBattle(fireFacePresent);

    expect(startRingBuffer).toHaveBeenCalledTimes(1);
    expect(startRingBuffer.mock.calls[0][0]).toBeInstanceOf(MockMediaStream);
  });

  // criterion: 1 (violation guard) — order independence + "at most once": when the stream resolves
  // AFTER the battle phase has already begun, the ring buffer still starts (from the stream-ready
  // callback) — and starts EXACTLY once, not twice, even though both call sites run.
  it('ring-buffer-start violation guard: stream resolving AFTER battle-start still starts it exactly once', async () => {
    const { ws } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, startRingBuffer } = makeFakeRecording();
    // getUserMedia is stubbed but its promise is only resolved manually below, AFTER battle starts.
    let resolveStream: (s: MediaStream) => void = () => {};
    const streamPromise = new Promise<MediaStream>((resolve) => {
      resolveStream = resolve;
    });
    Object.defineProperty(globalThis.navigator, 'mediaDevices', {
      value: { getUserMedia: vi.fn().mockReturnValue(streamPromise) },
      writable: true,
      configurable: true,
    });

    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      currentUserId: 'u1',
      ...rtcMockFactories(),
    });

    reachBattle(fireFacePresent);
    expect(startRingBuffer).not.toHaveBeenCalled();

    const stream = new MockMediaStream() as unknown as MediaStream;
    await act(async () => {
      resolveStream(stream);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(startRingBuffer).toHaveBeenCalledTimes(1);
    expect(startRingBuffer.mock.calls[0][0]).toBe(stream);
  });

  // criterion: 1 — teardown (unmount / routing away) stops the recording engine.
  it('ring-buffer-stop-on-teardown: teardown() (e.g. a sanity-check failure) stops the recording engine', () => {
    const { ws } = makeMockWs();
    const { Cv } = makeFakeCv();
    const { Recording, stop } = makeFakeRecording();
    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      currentUserId: 'u1',
    });

    // Never fire facePresent — the 2s sanity check fails, calling teardown() (which stops any live
    // recording) synchronously before routing away.
    act(() => {
      vi.advanceTimersByTime(2000);
    });

    expect(stop).toHaveBeenCalled();
  });

  // criterion: 2/3/4 — the full win flow: outcome → win-edit phase → captureWin() → submitWinClip
  // (upload+convert) → /results carrying mp4Url (+ trackId, the selected edit audio).
  it('win-clip-flow: win outcome captures, uploads+converts, and routes to /results with mp4Url', async () => {
    stubGetUserMedia();
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    const blob = new Blob(['clip'], { type: 'video/webm' });
    captureWin.mockResolvedValue(blob);
    const clipsApi = makeMockClipsApi('clip-42');

    renderBattle(
      {
        wsClient: ws,
        cvComponent: Cv,
        recordingComponent: Recording,
        clipsApi,
        currentUserId: '7',
        ...rtcMockFactories(),
      },
      { roomId: 'room-1', trackId: 'track-9' },
    );

    await flushGetUserMedia();
    reachBattle(fireFacePresent);

    act(() => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
    });

    // Immediately after the outcome fires (before any capture/upload microtask has flushed), the
    // win-edit placeholder is showing — the flow does NOT jump straight to /results.
    expect(screen.getByTestId('win-edit')).toBeInTheDocument();
    expect(screen.queryByTestId('results-probe')).not.toBeInTheDocument();

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(captureWin).toHaveBeenCalledTimes(1);
    expect(clipsApi.uploadClip).toHaveBeenCalledWith(blob);
    expect(clipsApi.convertClip).toHaveBeenCalledWith('clip-42');
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-result').textContent).toBe('win');
    expect(screen.getByTestId('results-mp4-url').textContent).toBe(
      'https://clips.example/v1/clips/clip-42/download',
    );
    // criterion: 4 — trackId (the chosen edit audio) is forwarded into the /results state.
    expect(screen.getByTestId('results-track-id').textContent).toBe('track-9');
  });

  // criterion: 2 (violation guard) — teardown() must NOT run before captureWin() resolves (the
  // recorder needs the still-live stream): the arbitration WS must still be open while win-edit is
  // showing, and only closes once capture/upload has actually completed.
  it('win-clip-flow violation guard: teardown is deferred until AFTER captureWin resolves', async () => {
    stubGetUserMedia();
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    let resolveCapture: (b: Blob) => void = () => {};
    captureWin.mockReturnValue(
      new Promise<Blob>((resolve) => {
        resolveCapture = resolve;
      }),
    );
    const clipsApi = makeMockClipsApi();

    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      clipsApi,
      currentUserId: '7',
      ...rtcMockFactories(),
    });

    await flushGetUserMedia();
    reachBattle(fireFacePresent);

    await act(async () => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
      await Promise.resolve();
    });

    // Still mid-capture — teardown() (which closes the WS) must NOT have run yet.
    expect(ws.close).not.toHaveBeenCalled();

    await act(async () => {
      resolveCapture(new Blob(['clip'], { type: 'video/webm' }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(ws.close).toHaveBeenCalled();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — a captureWin()/upload failure must never strand the player:
  // it still routes to /results, just without a clip.
  it('win-clip-flow violation guard: a capture/upload failure still routes to /results, without a clip', async () => {
    stubGetUserMedia();
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    captureWin.mockRejectedValue(new Error('capture failed'));
    const clipsApi = makeMockClipsApi();

    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      clipsApi,
      currentUserId: '7',
      ...rtcMockFactories(),
    });

    await flushGetUserMedia();
    reachBattle(fireFacePresent);

    await act(async () => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(clipsApi.uploadClip).not.toHaveBeenCalled();
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-result').textContent).toBe('win');
    expect(screen.getByTestId('results-mp4-url').textContent).toBe('');
  });

  // criterion: 2 — the loss path shows the loss-edit placeholder with a WORKING Skip button that
  // routes to /results with NO clip.
  it('loss-then-skip: loss outcome shows loss-edit with a Skip button routing to /results without a clip', () => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    renderBattle({ wsClient: ws, cvComponent: Cv, recordingComponent: Recording, currentUserId: '7' });

    reachBattle(fireFacePresent);

    act(() => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 3, loser_id: 7 }));
    });

    expect(screen.getByTestId('loss-edit')).toBeInTheDocument();
    expect(screen.queryByTestId('results-probe')).not.toBeInTheDocument();
    // The recording engine is never invoked on the loss path — only the winner captures a clip.
    expect(captureWin).not.toHaveBeenCalled();

    act(() => {
      screen.getByTestId('skip-edit').click();
    });

    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-result').textContent).toBe('loss');
    expect(screen.getByTestId('results-mp4-url').textContent).toBe('');
  });

  // criterion: 2 (violation guard) — without clicking Skip, the loss/skip-edit screen must NOT
  // auto-navigate away on its own; a Skip button is the key requirement.
  it('loss-then-skip violation guard: without clicking Skip, the player stays on loss-edit', () => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

    reachBattle(fireFacePresent);

    act(() => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 3, loser_id: 7 }));
    });

    expect(screen.getByTestId('loss-edit')).toBeInTheDocument();
    expect(screen.queryByTestId('results-probe')).not.toBeInTheDocument();
  });

  // criterion: 4 (violation guard) — with no trackId ever carried in location.state, /results
  // receives none (not some hard-coded stand-in) on a win.
  it('trackId-threaded violation guard: with no trackId in location.state, /results receives none', async () => {
    stubGetUserMedia();
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, captureWin } = makeFakeRecording();
    captureWin.mockResolvedValue(new Blob(['clip'], { type: 'video/webm' }));
    const clipsApi = makeMockClipsApi();

    renderBattle(
      {
        wsClient: ws,
        cvComponent: Cv,
        recordingComponent: Recording,
        clipsApi,
        currentUserId: '7',
        ...rtcMockFactories(),
      },
      { roomId: 'room-1' },
    );

    await flushGetUserMedia();
    reachBattle(fireFacePresent);

    await act(async () => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId('results-track-id').textContent).toBe('');
  });

  // criterion: 2/5 — genuine end-to-end wiring: the REAL recording engine (RecordingComponent),
  // not a fake, actually plays the ~10s edit slot (via fake timers) before the win clip resolves
  // and the flow reaches /results with a clip. This proves the engine is genuinely wired, not just
  // mocked away — a Battle.tsx regression that stopped calling captureWin()/submitWinClip() at all
  // would leave the flow stuck on 'win-edit' forever and this test would time out/fail.
  it('win-clip-flow: the real recording engine plays the ~10s edit slot before /results carries a clip', async () => {
    const stream = new MockMediaStream() as unknown as MediaStream;
    stubGetUserMedia(stream);
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    const { Recording, recorder } = makeRealRecording();
    const clipsApi = makeMockClipsApi('clip-real');

    renderBattle({
      wsClient: ws,
      cvComponent: Cv,
      recordingComponent: Recording,
      clipsApi,
      currentUserId: '7',
      ...rtcMockFactories(),
    });

    await flushGetUserMedia();
    reachBattle(fireFacePresent);

    await act(async () => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
      await Promise.resolve();
    });

    expect(screen.getByTestId('win-edit')).toBeInTheDocument();
    expect(recorder.start).toHaveBeenCalledTimes(1);
    expect(recorder.stop).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    });
    // Flush submitWinClip's upload+convert microtasks.
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(recorder.stop).toHaveBeenCalledTimes(1);
    expect(clipsApi.uploadClip).toHaveBeenCalled();
    expect(clipsApi.convertClip).toHaveBeenCalledWith('clip-real');
    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-mp4-url').textContent).toBe(
      'https://clips.example/v1/clips/clip-real/download',
    );
  });

  // criterion: 5 — with no `recordingComponent` prop, Battle mounts the real `RecordingComponent`
  // (default wiring). Proven indirectly: a real engine that never had startRingBuffer() called
  // rejects captureWin() with its own specific error, which handleOutcome's catch block turns into
  // a graceful "route to results without a clip" — a fake/no-op default would never throw that.
  it('production-default-wiring: with no recordingComponent prop, the real RecordingComponent is mounted', async () => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent } = makeFakeCv();
    // No getUserMedia stub — streamRef.current stays null, so the ring buffer is never started,
    // and the real engine's captureWin() will throw "called before startRingBuffer()".
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

    reachBattle(fireFacePresent);

    await act(async () => {
      fireMessage(JSON.stringify({ type: 'outcome', winner_id: 7, loser_id: 3 }));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(screen.getByTestId('results-probe')).toBeInTheDocument();
    expect(screen.getByTestId('results-mp4-url').textContent).toBe('');
  });
});
