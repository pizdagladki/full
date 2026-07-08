import { StrictMode } from 'react';
import { act, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Search } from './Search';
import type { WsClientApi } from '../api/ws';
import type { FaceLandmarkResult, LandmarkRunner } from '../cv';
import { defaultCvRunner, __resetDefaultCvRunnerForTests } from '../cv';

// ---------------------------------------------------------------------------
// RAF stub — same pattern as CvComponent.test.tsx: collect scheduled callbacks
// and tick them manually so frames are driven deterministically.
// ---------------------------------------------------------------------------

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

  // Search's camera-preview effect is guarded (like Home) — stub getUserMedia so it resolves
  // cleanly and never blocks/crashes; cv.start() is NOT gated on this (per design).
  Object.defineProperty(globalThis.navigator, 'mediaDevices', {
    value: {
      getUserMedia: vi.fn().mockResolvedValue({ getTracks: () => [] }),
    },
    writable: true,
    configurable: true,
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  __resetDefaultCvRunnerForTests();
});

// ---------------------------------------------------------------------------
// Mock WS client — captures the onOpen/onMessage callbacks so tests can fire
// WS-open and server messages directly.
// ---------------------------------------------------------------------------

function makeMockWs(): {
  ws: WsClientApi;
  fireOpen: () => void;
  fireMessage: (data: string) => void;
} {
  let openCb: (() => void) | undefined;
  let msgCb: ((data: string) => void) | undefined;

  const ws: WsClientApi = {
    connect: vi.fn(),
    send: vi.fn(),
    close: vi.fn(),
    onMessage: vi.fn((cb: (data: string) => void) => {
      msgCb = cb;
    }),
    onOpen: vi.fn((cb: () => void) => {
      openCb = cb;
    }),
    onClose: vi.fn(),
  };

  return {
    ws,
    fireOpen: () => openCb?.(),
    fireMessage: (data: string) => msgCb?.(data),
  };
}

function makeCvRunner(): {
  runner: LandmarkRunner;
  setResult: (r: FaceLandmarkResult) => void;
} {
  let nextResult: FaceLandmarkResult = { faceLandmarks: [] };
  const runner: LandmarkRunner = {
    detectForVideo: vi.fn(() => nextResult),
  };
  return {
    runner,
    setResult: (r: FaceLandmarkResult) => {
      nextResult = r;
    },
  };
}

const FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [[{ x: 0, y: 0, z: 0 }]] };
const NO_FACE_FRAME: FaceLandmarkResult = { faceLandmarks: [] };

/** Sets the next detection result, then ticks the latest pending RAF callback. */
function tickFrame(
  setResult: (r: FaceLandmarkResult) => void,
  result: FaceLandmarkResult,
  ts = 0,
): void {
  setResult(result);
  const cb = rafCallbacks[rafCallbacks.length - 1];
  act(() => {
    cb(ts);
  });
}

/** Extracts every parsed JSON payload sent via ws.send. */
function sentMessages(ws: WsClientApi): unknown[] {
  return vi.mocked(ws.send).mock.calls.map((call) => JSON.parse(call[0] as string) as unknown);
}

function BattleProbe() {
  const location = useLocation();
  const state = location.state as
    | { roomId?: string; opponent?: unknown; trackId?: string }
    | null;
  return (
    <div data-testid="battle-probe">
      <span data-testid="battle-room-id">{state?.roomId}</span>
      <span data-testid="battle-opponent">{JSON.stringify(state?.opponent)}</span>
      <span data-testid="battle-track-id">{state?.trackId}</span>
    </div>
  );
}

function renderSearch(wsClient: WsClientApi, cvRunner: LandmarkRunner, trackId?: string) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/search', state: trackId ? { trackId } : undefined }]}>
      <Routes>
        <Route path="/search" element={<Search wsClient={wsClient} cvRunner={cvRunner} />} />
        <Route path="/home" element={<div>HOME</div>} />
        <Route path="/battle" element={<BattleProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

/** Same as renderSearch, but wrapped in React.StrictMode (mount→cleanup→mount in dev). */
function renderSearchStrict(wsClient: WsClientApi, cvRunner: LandmarkRunner) {
  return render(
    <StrictMode>
      <MemoryRouter initialEntries={['/search']}>
        <Routes>
          <Route path="/search" element={<Search wsClient={wsClient} cvRunner={cvRunner} />} />
          <Route path="/home" element={<div>HOME</div>} />
          <Route path="/battle" element={<BattleProbe />} />
        </Routes>
      </MemoryRouter>
    </StrictMode>,
  );
}

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('Search', () => {
  // criterion: 1 — join is gated on face presence; with no face, join is never sent and the
  // "show your face" prompt is visible.
  it('no-face-blocks-join: with no face present the join is NOT sent and the face prompt is shown', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, NO_FACE_FRAME);

    expect(ws.send).not.toHaveBeenCalled();
    expect(screen.getByTestId('face-prompt')).toBeInTheDocument();
    expect(screen.queryByTestId('search-animation')).not.toBeInTheDocument();
  });

  // criterion: 1 — once a face is present AND the WS is open, join is sent with mode/level.
  it('face-present-sends-join: ws-open + a face frame sends {type:"join", mode, level}', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    const sent = sentMessages(ws);
    expect(sent).toEqual([{ type: 'join', mode: 'ranked', level: 1 }]);
    expect(screen.getByTestId('search-animation')).toBeInTheDocument();
    expect(screen.queryByTestId('face-prompt')).not.toBeInTheDocument();
  });

  // criterion: 1 — order independence: a face frame arriving BEFORE the WS opens must not send a
  // premature join; the join fires once the WS subsequently opens.
  it('face-present-sends-join: face frame before ws-open still joins once the WS opens (order independent)', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    tickFrame(setResult, FACE_FRAME);
    expect(ws.send).not.toHaveBeenCalled();

    act(() => {
      fireOpen();
    });

    const sent = sentMessages(ws);
    expect(sent).toEqual([{ type: 'join', mode: 'ranked', level: 1 }]);
  });

  // criterion: 2 — losing the face while searching stops the search (leaves the queue) and
  // resets to the home screen.
  it('face-lost-resets-home: losing the face while searching sends leave, closes the WS, and returns home', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME); // joins

    // NO_FACE_WINDOW = 3 consecutive no-face frames trigger onFaceLost
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    const sent = sentMessages(ws);
    expect(sent).toContainEqual({ type: 'leave' });
    expect(ws.close).toHaveBeenCalled();
    expect(screen.getByText('HOME')).toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — losing the face for fewer than NO_FACE_WINDOW frames must
  // NOT stop the search — the home screen must not appear prematurely.
  it('face-lost-resets-home violation guard: fewer than 3 consecutive no-face frames does not leave the queue', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(sentMessages(ws)).not.toContainEqual({ type: 'leave' });
    expect(ws.close).not.toHaveBeenCalled();
    expect(screen.queryByText('HOME')).not.toBeInTheDocument();
  });

  // criterion: 3 — on {type:"matched", room_id, opponent} the app transitions to the battle
  // screen, carrying room_id + opponent.
  it('matched-transitions-to-battle: a {type:"matched"} WS frame navigates to /battle carrying room_id + opponent', () => {
    const { ws, fireOpen, fireMessage } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);
    expect(screen.getByTestId('search-animation')).toBeInTheDocument();

    act(() => {
      fireMessage(
        JSON.stringify({ type: 'matched', room_id: 'room-7', opponent: { id: 'opp-1' } }),
      );
    });

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-7');
    expect(screen.getByTestId('battle-opponent').textContent).toBe(JSON.stringify({ id: 'opp-1' }));
  });

  // criterion: 4 (#159) — the trackId carried in via ModeSelect's location.state is forwarded
  // onward to /battle alongside roomId/opponent.
  it('matched-transitions-to-battle: forwards trackId from location.state to /battle', () => {
    const { ws, fireOpen, fireMessage } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner, 'track-9');

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    act(() => {
      fireMessage(
        JSON.stringify({ type: 'matched', room_id: 'room-7', opponent: { id: 'opp-1' } }),
      );
    });

    expect(screen.getByTestId('battle-track-id').textContent).toBe('track-9');
  });

  // criterion: 4 (#159) violation guard — with no trackId in location.state, /battle receives
  // none (not some hard-coded stand-in value).
  it('matched-transitions-to-battle violation guard: with no trackId, /battle receives none', () => {
    const { ws, fireOpen, fireMessage } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    act(() => {
      fireMessage(
        JSON.stringify({ type: 'matched', room_id: 'room-7', opponent: { id: 'opp-1' } }),
      );
    });

    expect(screen.getByTestId('battle-track-id').textContent).toBe('');
  });

  // criterion: 3 (violation guard) — a malformed / non-matched frame must NOT navigate away from
  // the search screen (the neutral search-animation placeholder stays put).
  it('matched-transitions-to-battle violation guard: a malformed WS frame is ignored, no navigation', () => {
    const { ws, fireOpen, fireMessage } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    act(() => {
      fireMessage('not json');
    });
    act(() => {
      fireMessage(JSON.stringify({ type: 'queued' }));
    });

    expect(screen.queryByTestId('battle-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('search-animation')).toBeInTheDocument();
  });

  // criterion: 3 (violation guard) — a {type:'matched'} frame with NO room_id must NOT navigate
  // to /battle (would otherwise carry roomId: undefined); the player stays on search-animation.
  it('matched-transitions-to-battle violation guard: a matched frame missing room_id does not navigate', () => {
    const { ws, fireOpen, fireMessage } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    act(() => {
      fireMessage(JSON.stringify({ type: 'matched', opponent: { id: 'x' } }));
    });

    expect(screen.queryByTestId('battle-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('search-animation')).toBeInTheDocument();
  });

  // criterion: 4 — unmounting (navigating away) sends leave and closes the WS: no ghost search.
  it('leave-on-exit: unmounting sends leave and closes the WS (no ghost search)', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    const { unmount } = renderSearch(ws, runner);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME);

    unmount();

    expect(sentMessages(ws)).toContainEqual({ type: 'leave' });
    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 4 (violation guard) — unmounting before ever joining must NOT send a leave frame
  // (there was nothing to leave), but must still close the WS.
  it('leave-on-exit violation guard: unmounting before joining closes the WS without sending leave', () => {
    const { ws } = makeMockWs();
    const { runner } = makeCvRunner();
    const { unmount } = renderSearch(ws, runner);

    unmount();

    expect(sentMessages(ws)).not.toContainEqual({ type: 'leave' });
    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 4 — StrictMode regression: a mount→cleanup→mount cycle (React.StrictMode in dev)
  // must not permanently latch the teardown guard. Without resetting the gate refs at the top of
  // the mount effect, the StrictMode-only synthetic cleanup sets teardownRef=true forever, so the
  // REAL unmount below early-returns and never sends `leave` for the connection that actually
  // joined — a ghost queue entry. This test FAILS without that reset.
  it('leave-on-exit: a StrictMode mount-cleanup-mount cycle still sends leave + closes the WS on the real unmount', () => {
    const { ws, fireOpen } = makeMockWs();
    const { runner, setResult } = makeCvRunner();
    const { unmount } = renderSearchStrict(ws, runner);

    // StrictMode double-invokes the mount effect in dev — confirm we actually exercised that path.
    expect(vi.mocked(ws.connect).mock.calls.length).toBeGreaterThanOrEqual(2);

    act(() => {
      fireOpen();
    });
    tickFrame(setResult, FACE_FRAME); // joins on the settled (second) connection

    expect(sentMessages(ws)).toContainEqual({ type: 'join', mode: 'ranked', level: 1 });

    unmount();

    expect(sentMessages(ws)).toContainEqual({ type: 'leave' });
    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 1 — entering search connects the matchmaking WS on the documented path.
  it('mount: connects the matchmaking WS on /ws/match', () => {
    const { ws } = makeMockWs();
    const { runner } = makeCvRunner();
    renderSearch(ws, runner);

    expect(ws.connect).toHaveBeenCalledWith('/ws/match');
  });

  // criteria: 1b/2b (default wiring regression guard) — with NO `cvRunner` prop supplied at all,
  // Search must fall back to the real `defaultCvRunner()` singleton, NOT an inline no-face
  // placeholder. Seeds the module-level singleton (reset + a controllable loader that resolves to
  // a runner reporting a real face) BEFORE rendering, so Search's own `cvRunner = defaultCvRunner()`
  // default-parameter call returns that SAME already-loaded instance. If Search.tsx's default were
  // reverted to an inline `{ detectForVideo: () => ({ faceLandmarks: [] }) }`, the seeded singleton
  // would never be consulted, the tick below would never report a face, and this test would fail
  // (no join sent, face-prompt still shown).
  it('production-default-wiring: with no cvRunner prop, the real defaultCvRunner() singleton drives face detection', async () => {
    __resetDefaultCvRunnerForTests();
    const seededRunner: LandmarkRunner = { detectForVideo: vi.fn(() => FACE_FRAME) };
    defaultCvRunner(() => Promise.resolve(seededRunner));
    // Flush the DeferredCvRunner's internal load().then(...) so the singleton is actually ready.
    await Promise.resolve();
    await Promise.resolve();

    const { ws, fireOpen } = makeMockWs();
    render(
      <MemoryRouter initialEntries={['/search']}>
        <Routes>
          <Route path="/search" element={<Search wsClient={ws} />} />
          <Route path="/home" element={<div>HOME</div>} />
          <Route path="/battle" element={<BattleProbe />} />
        </Routes>
      </MemoryRouter>,
    );

    act(() => {
      fireOpen();
    });
    const cb = rafCallbacks[rafCallbacks.length - 1];
    act(() => {
      cb(0);
    });

    expect(sentMessages(ws)).toEqual([{ type: 'join', mode: 'ranked', level: 1 }]);
    expect(screen.getByTestId('search-animation')).toBeInTheDocument();
    expect(screen.queryByTestId('face-prompt')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// #172 - the camera picked on Home is honored by the game screen
// ---------------------------------------------------------------------------

describe('Search - camera device passthrough (#172)', () => {
  function makeIdleRunner(): LandmarkRunner {
    return { detectForVideo: vi.fn().mockReturnValue({ faceLandmarks: [] }) };
  }

  it('requests the persisted deviceId with an exact constraint', async () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue('cam-123'),
      setItem: vi.fn(),
    });
    const gum = vi.fn().mockResolvedValue({ getTracks: () => [] });
    Object.defineProperty(globalThis.navigator, 'mediaDevices', {
      value: { getUserMedia: gum },
      configurable: true,
    });
    renderSearch(makeMockWs().ws, makeIdleRunner());
    await act(async () => {});
    expect(gum).toHaveBeenCalledWith({ video: { deviceId: { exact: 'cam-123' } } });
  });

  it('falls back to the default device when the selected one fails', async () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn().mockReturnValue('cam-123'),
      setItem: vi.fn(),
    });
    const gum = vi
      .fn()
      .mockRejectedValueOnce(new Error('NotReadableError'))
      .mockResolvedValue({ getTracks: () => [] });
    Object.defineProperty(globalThis.navigator, 'mediaDevices', {
      value: { getUserMedia: gum },
      configurable: true,
    });
    renderSearch(makeMockWs().ws, makeIdleRunner());
    await act(async () => {});
    expect(gum).toHaveBeenCalledTimes(2);
    expect(gum).toHaveBeenLastCalledWith({ video: true });
  });
});
