import { StrictMode } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { InviteRoom } from './InviteRoom';
import type { WsClientApi } from '../api/ws';
import type { FaceLandmarkResult, LandmarkRunner } from '../cv';
import { defaultCvRunner, __resetDefaultCvRunnerForTests } from '../cv';

// ---------------------------------------------------------------------------
// Mock WS client — hand-rolled class implementing WsClientApi, capturing the
// onMessage callback so tests can fire server frames directly (same seam-
// injection style as Search.test.tsx / Battle.test.tsx).
// ---------------------------------------------------------------------------

class MockWs implements WsClientApi {
  connect = vi.fn();
  send = vi.fn();
  close = vi.fn();
  private msgCb: ((data: string) => void) | undefined;
  private openCb: (() => void) | undefined;
  private closeCb: (() => void) | undefined;

  onMessage(cb: (data: string) => void): void {
    this.msgCb = cb;
  }

  onOpen(cb: () => void): void {
    this.openCb = cb;
  }

  onClose(cb: () => void): void {
    this.closeCb = cb;
  }

  emitMessage(data: string): void {
    this.msgCb?.(data);
  }

  emitOpen(): void {
    this.openCb?.();
  }

  emitClose(): void {
    this.closeCb?.();
  }
}

// ---------------------------------------------------------------------------
// RAF stub — same pattern as Search.test.tsx / CvComponent.test.tsx: collect
// scheduled callbacks and tick them manually so frames are driven
// deterministically (needed to drive the real CvEngine behind the face gate).
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

  Object.defineProperty(globalThis.navigator, 'clipboard', {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    writable: true,
    configurable: true,
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  __resetDefaultCvRunnerForTests();
});

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

/**
 * A runner that always reports no face — used ONLY as an explicit test-only injection (never a
 * production default; InviteRoom.tsx defaults `cvRunner` to the real `defaultCvRunner()` now, so
 * tests that need the no-face behavior must inject it themselves).
 */
function noFaceRunner(): LandmarkRunner {
  return { detectForVideo: vi.fn(() => NO_FACE_FRAME) };
}

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

function BattleProbe() {
  const location = useLocation();
  const state = location.state as { roomId?: string; ranked?: boolean } | null;
  return (
    <div data-testid="battle-probe">
      <span data-testid="battle-room-id">{state?.roomId}</span>
      <span data-testid="battle-ranked">{String(state?.ranked)}</span>
    </div>
  );
}

function HomeProbe() {
  return <div data-testid="home-probe">HOME</div>;
}

function renderInviteRoom(wsClient: WsClientApi, cvRunner?: LandmarkRunner) {
  return render(
    <MemoryRouter initialEntries={['/invite']}>
      <Routes>
        <Route path="/invite" element={<InviteRoom wsClient={wsClient} cvRunner={cvRunner} />} />
        <Route path="/battle" element={<BattleProbe />} />
        <Route path="/home" element={<HomeProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

/** Same as renderInviteRoom, but wrapped in React.StrictMode (mount→cleanup→mount in dev). */
function renderInviteRoomStrict(wsClient: WsClientApi, cvRunner?: LandmarkRunner) {
  return render(
    <StrictMode>
      <MemoryRouter initialEntries={['/invite']}>
        <Routes>
          <Route path="/invite" element={<InviteRoom wsClient={wsClient} cvRunner={cvRunner} />} />
          <Route path="/battle" element={<BattleProbe />} />
          <Route path="/home" element={<HomeProbe />} />
        </Routes>
      </MemoryRouter>
    </StrictMode>,
  );
}

/**
 * Renders InviteRoom with a face already present (ticks one FACE_FRAME through a real CvEngine).
 * This is the default setup for tests that aren't specifically about the face gate itself
 * (criteria 1/2/4) — those flows now require a present face before Create/Join can proceed.
 */
function renderInviteRoomFacePresent(wsClient: WsClientApi) {
  const { runner, setResult } = makeCvRunner();
  const rendered = renderInviteRoom(wsClient, runner);
  tickFrame(setResult, FACE_FRAME);
  return rendered;
}

function renderInviteRoomStrictFacePresent(wsClient: WsClientApi) {
  const { runner, setResult } = makeCvRunner();
  const rendered = renderInviteRoomStrict(wsClient, runner);
  tickFrame(setResult, FACE_FRAME);
  return rendered;
}

function sentMessages(ws: WsClientApi): unknown[] {
  return vi.mocked(ws.send).mock.calls.map((call) => JSON.parse(call[0] as string) as unknown);
}

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('InviteRoom', () => {
  // Menu renders both controls (no dedicated criterion, exercised as setup for every flow below).
  it('renders the Create room button and the Join by code form', () => {
    const ws = new MockWs();
    renderInviteRoom(ws, noFaceRunner());

    expect(screen.getByTestId('create-room-button')).toBeInTheDocument();
    expect(screen.getByTestId('join-code-input')).toBeInTheDocument();
    expect(screen.getByTestId('join-room-button')).toBeInTheDocument();
  });

  // criterion: 1 — "Create room" sends {type:"create_room"} and shows a copyable invite code.
  it('create-shows-code: clicking Create room sends create_room and renders the code from room_created', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(ws.connect).toHaveBeenCalledWith('/ws/signal');

    act(() => {
      ws.emitOpen();
    });
    expect(sentMessages(ws)).toEqual([{ type: 'create_room' }]);

    act(() => {
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    expect(screen.getByTestId('invite-code').textContent).toBe('ABC123');
    expect(screen.getByTestId('invite-waiting-message')).toBeInTheDocument();
    expect(screen.getByTestId('start-battle-button')).toBeInTheDocument();
  });

  // criterion: 1 (violation guard) — the create_room frame must NOT be sent synchronously right
  // after clicking Create room (the socket is still CONNECTING at that point); it must only be
  // sent once the WS actually reports open. This is the case that catches the InvalidStateError
  // regression: a mock/native WebSocket that ignores readyState would let this pass, but the fix
  // must only send from inside onOpen.
  it('create-shows-code violation guard: create_room is sent only after the WS actually opens, not synchronously on click', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(ws.send).not.toHaveBeenCalled();

    act(() => {
      ws.emitOpen();
    });

    expect(sentMessages(ws)).toEqual([{ type: 'create_room' }]);
  });

  // criterion: 1 (violation guard) — without a real room_created reply the code must NOT render;
  // a screen that fakes the code before the server confirms it would fail this.
  it('create-shows-code violation guard: no code renders until room_created actually arrives', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(screen.queryByTestId('invite-code')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-creating')).toBeInTheDocument();
  });

  // criterion: 1 — the "Start Battle" button (manual hand-off, since the server never pushes a
  // peer-joined notice to the creator) navigates to /battle carrying the room_id.
  it('create-shows-code: Start Battle navigates to /battle with the created room_id', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-42', code: 'XYZ' }));
    });

    fireEvent.click(screen.getByTestId('start-battle-button'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-42');
  });

  // criterion: 2 — the invite-a-friend room is the spec's UNRANKED branch: Start Battle's
  // create->battle hand-off must mark the navigation state `ranked: false`. This FAILS if the
  // `ranked: false` marker is dropped from the navigate('/battle', ...) call.
  it('unranked-marker: Start Battle navigates to /battle with ranked: false', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-42', code: 'XYZ' }));
    });

    fireEvent.click(screen.getByTestId('start-battle-button'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-42');
    expect(screen.getByTestId('battle-ranked').textContent).toBe('false');
  });

  // criterion: 2 — "Join by code" sends {type:"join_room", code} and on room_joined transitions to
  // the battle screen carrying the shared room_id.
  it('join-by-code-transitions-to-battle: submitting a code sends join_room and room_joined navigates to /battle', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'FRIEND1' } });
    fireEvent.click(screen.getByTestId('join-room-button'));

    expect(ws.connect).toHaveBeenCalledWith('/ws/signal');

    act(() => {
      ws.emitOpen();
    });
    expect(sentMessages(ws)).toEqual([{ type: 'join_room', code: 'FRIEND1' }]);

    act(() => {
      ws.emitMessage(JSON.stringify({ type: 'room_joined', room_id: 'room-99' }));
    });

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-99');
  });

  // criterion: 2 — same UNRANKED requirement as above but for the join->battle transition (the
  // `room_joined` handler). This FAILS if the `ranked: false` marker is dropped from that
  // navigate('/battle', ...) call.
  it('unranked-marker: room_joined navigates to /battle with ranked: false', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'FRIEND1' } });
    fireEvent.click(screen.getByTestId('join-room-button'));

    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_joined', room_id: 'room-99' }));
    });

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-99');
    expect(screen.getByTestId('battle-ranked').textContent).toBe('false');
  });

  // criterion: 2 (violation guard) — join_room must NOT be sent synchronously right after clicking
  // Join by code (the socket is still CONNECTING); it must only be sent once the WS actually
  // opens. Catches the same InvalidStateError regression as the create_room case.
  it('join-by-code-transitions-to-battle violation guard: join_room is sent only after the WS actually opens, not synchronously on click', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'FRIEND1' } });
    fireEvent.click(screen.getByTestId('join-room-button'));

    expect(ws.send).not.toHaveBeenCalled();

    act(() => {
      ws.emitOpen();
    });

    expect(sentMessages(ws)).toEqual([{ type: 'join_room', code: 'FRIEND1' }]);
  });

  // criterion: 2 (violation guard) — an invalid/expired code (an `error` frame) must show a
  // non-crashing error and must NOT navigate to /battle.
  const errorCases: { name: string; error: string }[] = [
    { name: 'invalid-code error: an invalid code renders the server error message', error: 'invalid or expired code' },
    { name: 'invalid-code error: room-full renders the server error message', error: 'room is full' },
  ];

  it.each(errorCases)('$name', ({ error }) => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'BADCODE' } });
    fireEvent.click(screen.getByTestId('join-room-button'));

    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'error', error }));
    });

    expect(screen.getByTestId('invite-error').textContent).toBe(error);
    expect(screen.queryByTestId('battle-probe')).not.toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — a malformed WS frame during join must not throw and must not
  // navigate away from the joining screen.
  it('invalid-code error violation guard: a malformed frame does not crash and does not navigate', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'CODE1' } });
    fireEvent.click(screen.getByTestId('join-room-button'));
    act(() => {
      ws.emitOpen();
    });

    expect(() => {
      act(() => {
        ws.emitMessage('not json');
      });
    }).not.toThrow();

    expect(screen.queryByTestId('battle-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-joining')).toBeInTheDocument();
  });

  // criterion: 2 (violation guard) — after an error the join form must be usable again (retry),
  // not stuck in a dead error state.
  it('invalid-code error: Try again resets to the menu so the user can retry', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'BADCODE' } });
    fireEvent.click(screen.getByTestId('join-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'error', error: 'invalid or expired code' }));
    });

    fireEvent.click(screen.getByTestId('retry-button'));

    expect(screen.getByTestId('invite-menu')).toBeInTheDocument();
    expect(screen.queryByTestId('invite-error')).not.toBeInTheDocument();
  });

  // criterion: 4 — leaving (unmount) closes the WS: no ghost room.
  it('leave cleanup: unmounting after creating a room closes the WS', () => {
    const ws = new MockWs();
    const { unmount } = renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    unmount();

    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 4 (violation guard, StrictMode) — the mount effect's synthetic cleanup latches the
  // teardown guard; without re-arming it in the effect body every later teardown is a no-op and
  // the WS survives unmount as a ghost room. Renders under StrictMode like main.tsx does.
  it('leave cleanup (StrictMode): unmounting after creating a room still closes the WS', () => {
    const ws = new MockWs();
    const { unmount } = renderInviteRoomStrictFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    // StrictMode's synthetic cleanup already called close() once BEFORE the connection existed —
    // discard those calls so the assertion below can only be satisfied by the REAL unmount
    // teardown (otherwise a latched guard leaks the live WS while the test stays green).
    vi.mocked(ws.close).mockClear();
    unmount();

    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 4 (violation guard) — clicking Leave during the waiting phase must ALSO close the
  // WS and return to the menu (not just leave it hanging as a ghost connection).
  it('leave cleanup: clicking Leave during waiting closes the WS and resets to the menu', () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    fireEvent.click(screen.getByTestId('leave-button'));

    expect(ws.close).toHaveBeenCalled();
    expect(screen.getByTestId('invite-menu')).toBeInTheDocument();
  });

  // criterion: 4 (violation guard) — unmounting BEFORE ever creating/joining a room must still not
  // throw, and must not spuriously call close on a connection that was never opened is fine either
  // way, but calling teardown must be safe (no crash) even with no connection.
  it('leave cleanup violation guard: unmounting from the bare menu does not throw', () => {
    const ws = new MockWs();
    const { unmount } = renderInviteRoom(ws, noFaceRunner());

    expect(() => unmount()).not.toThrow();
  });

  // Copy button: writes the code to the clipboard when available, and never crashes when it isn't.
  it('copy-code-button writes the invite code to the clipboard', async () => {
    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    fireEvent.click(screen.getByTestId('copy-code-button'));

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('ABC123');
  });

  it('copy-code-button does not crash when navigator.clipboard is unavailable', () => {
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      value: undefined,
      writable: true,
      configurable: true,
    });

    const ws = new MockWs();
    renderInviteRoomFacePresent(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    expect(() => fireEvent.click(screen.getByTestId('copy-code-button'))).not.toThrow();
  });

  // -------------------------------------------------------------------------
  // criterion: 3 — face gate (start-gate half + continuous-gate half)
  // -------------------------------------------------------------------------

  const startActions: { name: string; act: () => void }[] = [
    {
      name: 'Create room',
      act: () => fireEvent.click(screen.getByTestId('create-room-button')),
    },
    {
      name: 'Join by code',
      act: () => {
        fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'CODE1' } });
        fireEvent.click(screen.getByTestId('join-room-button'));
      },
    },
  ];

  // criterion: 3 (start-gate, violation guard) — with no face present, clicking Create room /
  // Join by code must send NOTHING over the WS (not even connect()) and must show the face
  // prompt instead. A screen that ignores the face gate and connects/sends anyway fails this.
  it.each(startActions)(
    'no-face-blocks-start: clicking $name with no face present sends nothing over the WS and shows the face prompt',
    ({ act: doAction }) => {
      const ws = new MockWs();
      // Explicit no-face runner — InviteRoom.tsx now defaults `cvRunner` to the real
      // `defaultCvRunner()`, so this test injects a no-face runner itself to keep
      // facePresentRef.current false deterministically (never touches the real default).
      renderInviteRoom(ws, noFaceRunner());

      doAction();

      expect(ws.connect).not.toHaveBeenCalled();
      expect(ws.send).not.toHaveBeenCalled();
      expect(screen.getByTestId('invite-face-prompt')).toBeInTheDocument();
      expect(screen.getByTestId('invite-menu')).toBeInTheDocument();
    },
  );

  // criterion: 3 (start-gate) — once a face is present, Create room / Join by code proceed
  // exactly as before (connect + send), and the face prompt never appears.
  it.each(startActions)(
    'no-face-blocks-start violation guard: clicking $name WITH a face present connects the WS and shows no face prompt',
    ({ act: doAction }) => {
      const ws = new MockWs();
      renderInviteRoomFacePresent(ws);

      doAction();

      expect(ws.connect).toHaveBeenCalledWith('/ws/signal');
      expect(screen.queryByTestId('invite-face-prompt')).not.toBeInTheDocument();
    },
  );

  // criterion: 3 (continuous gate) — losing the face while waiting for a friend to join tears the
  // room down (closes the WS) and navigates home, exactly like Search.tsx's onFaceLost.
  it('face-lost-while-waiting-resets: losing the face while waiting for a friend closes the WS and returns home', () => {
    const ws = new MockWs();
    const { runner, setResult } = makeCvRunner();
    renderInviteRoom(ws, runner);

    tickFrame(setResult, FACE_FRAME); // opens the start-gate

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });
    expect(screen.getByTestId('invite-waiting')).toBeInTheDocument();

    // Discard the close() calls (if any) from setup so the assertion below can only be satisfied
    // by the face-lost teardown — mirrors the file's existing StrictMode mockClear() pattern.
    vi.mocked(ws.close).mockClear();

    // NO_FACE_WINDOW = 3 consecutive no-face frames trigger onFaceLost.
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(ws.close).toHaveBeenCalled();
    expect(screen.getByTestId('home-probe')).toBeInTheDocument();
  });

  // criterion: 3 (continuous gate, violation guard) — fewer than the required consecutive
  // no-face frames must NOT tear the room down; a screen that fires on the first no-face frame
  // would fail this (the room would be gone/home would render prematurely).
  it('face-lost-while-waiting-resets violation guard: fewer than 3 consecutive no-face frames does not tear down', () => {
    const ws = new MockWs();
    const { runner, setResult } = makeCvRunner();
    renderInviteRoom(ws, runner);

    tickFrame(setResult, FACE_FRAME);
    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    vi.mocked(ws.close).mockClear();

    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(ws.close).not.toHaveBeenCalled();
    expect(screen.queryByTestId('home-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-waiting')).toBeInTheDocument();
  });

  // criterion: 3 (continuous gate, violation guard) — in the `menu` phase (nothing in flight) a
  // face-lost event must be a no-op: no WS close, no navigation. A screen that tears down
  // unconditionally on every face-lost event (ignoring phase) would fail this.
  it('face-lost-in-menu-is-noop: losing the face while still on the menu does not close the WS or navigate', () => {
    const ws = new MockWs();
    const { runner, setResult } = makeCvRunner();
    renderInviteRoom(ws, runner);

    tickFrame(setResult, FACE_FRAME); // face present, but user hasn't started anything (still menu)
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(ws.close).not.toHaveBeenCalled();
    expect(screen.queryByTestId('home-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-menu')).toBeInTheDocument();
  });

  // criterion: 3 (continuous gate, violation guard) — in the `error` phase a face-lost event must
  // also be a no-op: no WS close, no navigation away from the error screen.
  it('face-lost-in-error-is-noop: losing the face on the error screen does not close the WS or navigate', () => {
    const ws = new MockWs();
    const { runner, setResult } = makeCvRunner();
    renderInviteRoom(ws, runner);

    tickFrame(setResult, FACE_FRAME);
    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'BADCODE' } });
    fireEvent.click(screen.getByTestId('join-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'error', error: 'invalid or expired code' }));
    });
    expect(screen.getByTestId('invite-error-screen')).toBeInTheDocument();

    vi.mocked(ws.close).mockClear();

    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);
    tickFrame(setResult, NO_FACE_FRAME);

    expect(ws.close).not.toHaveBeenCalled();
    expect(screen.queryByTestId('home-probe')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-error-screen')).toBeInTheDocument();
  });

  // criteria: 1b/2b (default wiring regression guard) — with NO `cvRunner` prop supplied at all,
  // InviteRoom must fall back to the real `defaultCvRunner()` singleton, NOT an inline no-face
  // placeholder. Seeds the module-level singleton (reset + a controllable loader resolving to a
  // runner reporting a real face) BEFORE rendering, so InviteRoom's own
  // `cvRunner = defaultCvRunner()` default-parameter call returns that SAME already-loaded
  // instance. If InviteRoom.tsx's default were reverted to an inline
  // `{ detectForVideo: () => ({ faceLandmarks: [] }) }`, the seeded singleton would never be
  // consulted, the tick below would never report a face, and Create room would stay gated behind
  // the face prompt — this test would fail.
  it('production-default-wiring: with no cvRunner prop, the real defaultCvRunner() singleton drives the face gate', async () => {
    __resetDefaultCvRunnerForTests();
    const seededRunner: LandmarkRunner = { detectForVideo: vi.fn(() => FACE_FRAME) };
    defaultCvRunner(() => Promise.resolve(seededRunner));
    // Flush the DeferredCvRunner's internal load().then(...) so the singleton is actually ready.
    await Promise.resolve();
    await Promise.resolve();

    const ws = new MockWs();
    render(
      <MemoryRouter initialEntries={['/invite']}>
        <Routes>
          <Route path="/invite" element={<InviteRoom wsClient={ws} />} />
          <Route path="/battle" element={<BattleProbe />} />
          <Route path="/home" element={<HomeProbe />} />
        </Routes>
      </MemoryRouter>,
    );

    const cb = rafCallbacks[rafCallbacks.length - 1];
    act(() => {
      cb(0);
    });

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(ws.connect).toHaveBeenCalledWith('/ws/signal');
    expect(screen.queryByTestId('invite-face-prompt')).not.toBeInTheDocument();
  });
});
