import { forwardRef, useEffect, useImperativeHandle } from 'react';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Battle } from './Battle';
import type { BattleProps } from './Battle';
import type { WsClientApi } from '../api/ws';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { defaultCvRunner, __resetDefaultCvRunnerForTests } from '../cv';
import type { PcLike, WsLike, PcFactory, WsFactory } from '../rtc';

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

// ---------------------------------------------------------------------------
// Routing harness
// ---------------------------------------------------------------------------

function ResultsProbe() {
  const location = useLocation();
  const state = location.state as
    | { result?: string; durationMs?: number; winnerId?: number; loserId?: number }
    | null;
  return (
    <div data-testid="results-probe">
      <span data-testid="results-result">{state?.result}</span>
      <span data-testid="results-duration">{state?.durationMs}</span>
      <span data-testid="results-winner">{state?.winnerId}</span>
      <span data-testid="results-loser">{state?.loserId}</span>
    </div>
  );
}

function renderBattle(props: BattleProps) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/battle', state: { roomId: 'room-1' } }]}>
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
  // authoritative outcome frame the app routes to results carrying the result + match duration.
  it.each([
    { name: 'win when winner_id matches the current user', winnerId: 7, loserId: 3, want: 'win' },
    { name: 'loss when winner_id does not match the current user', winnerId: 3, loserId: 7, want: 'loss' },
  ])('blink-reported-and-outcome-routes: $name', ({ winnerId, loserId, want }) => {
    const { ws, fireMessage } = makeMockWs();
    const { Cv, fireFacePresent, fireBlink } = makeFakeCv();
    renderBattle({ wsClient: ws, cvComponent: Cv, currentUserId: '7' });

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
