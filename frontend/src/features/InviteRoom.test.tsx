import { StrictMode } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { InviteRoom } from './InviteRoom';
import type { WsClientApi } from '../api/ws';

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

function BattleProbe() {
  const location = useLocation();
  const state = location.state as { roomId?: string } | null;
  return (
    <div data-testid="battle-probe">
      <span data-testid="battle-room-id">{state?.roomId}</span>
    </div>
  );
}

function renderInviteRoom(wsClient: WsClientApi) {
  return render(
    <MemoryRouter initialEntries={['/invite']}>
      <Routes>
        <Route path="/invite" element={<InviteRoom wsClient={wsClient} />} />
        <Route path="/battle" element={<BattleProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

/** Same as renderInviteRoom, but wrapped in React.StrictMode (mount→cleanup→mount in dev). */
function renderInviteRoomStrict(wsClient: WsClientApi) {
  return render(
    <StrictMode>
      <MemoryRouter initialEntries={['/invite']}>
        <Routes>
          <Route path="/invite" element={<InviteRoom wsClient={wsClient} />} />
          <Route path="/battle" element={<BattleProbe />} />
        </Routes>
      </MemoryRouter>
    </StrictMode>,
  );
}

function sentMessages(ws: WsClientApi): unknown[] {
  return vi.mocked(ws.send).mock.calls.map((call) => JSON.parse(call[0] as string) as unknown);
}

beforeEach(() => {
  Object.defineProperty(globalThis.navigator, 'clipboard', {
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
    writable: true,
    configurable: true,
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('InviteRoom', () => {
  // Menu renders both controls (no dedicated criterion, exercised as setup for every flow below).
  it('renders the Create room button and the Join by code form', () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

    expect(screen.getByTestId('create-room-button')).toBeInTheDocument();
    expect(screen.getByTestId('join-code-input')).toBeInTheDocument();
    expect(screen.getByTestId('join-room-button')).toBeInTheDocument();
  });

  // criterion: 1 — "Create room" sends {type:"create_room"} and shows a copyable invite code.
  it('create-shows-code: clicking Create room sends create_room and renders the code from room_created', () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(ws.connect).toHaveBeenCalledWith('/ws');

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
    renderInviteRoom(ws);

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
    renderInviteRoom(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));

    expect(screen.queryByTestId('invite-code')).not.toBeInTheDocument();
    expect(screen.getByTestId('invite-creating')).toBeInTheDocument();
  });

  // criterion: 1 — the "Start Battle" button (manual hand-off, since the server never pushes a
  // peer-joined notice to the creator) navigates to /battle carrying the room_id.
  it('create-shows-code: Start Battle navigates to /battle with the created room_id', () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-42', code: 'XYZ' }));
    });

    fireEvent.click(screen.getByTestId('start-battle-button'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-room-id').textContent).toBe('room-42');
  });

  // criterion: 2 — "Join by code" sends {type:"join_room", code} and on room_joined transitions to
  // the battle screen carrying the shared room_id.
  it('join-by-code-transitions-to-battle: submitting a code sends join_room and room_joined navigates to /battle', () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

    fireEvent.change(screen.getByTestId('join-code-input'), { target: { value: 'FRIEND1' } });
    fireEvent.click(screen.getByTestId('join-room-button'));

    expect(ws.connect).toHaveBeenCalledWith('/ws');

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

  // criterion: 2 (violation guard) — join_room must NOT be sent synchronously right after clicking
  // Join by code (the socket is still CONNECTING); it must only be sent once the WS actually
  // opens. Catches the same InvalidStateError regression as the create_room case.
  it('join-by-code-transitions-to-battle violation guard: join_room is sent only after the WS actually opens, not synchronously on click', () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

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
    renderInviteRoom(ws);

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
    renderInviteRoom(ws);

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
    renderInviteRoom(ws);

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
    const { unmount } = renderInviteRoom(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    unmount();

    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 4 (violation guard, StrictMode) — the mount effect's synthetic cleanup latches the
  // teardown guard; without re-arming it in the effect body every later teardown is a no-op and
  // the WS survives unmount as a ghost room. Renders under StrictMode like main.tsx does.
  it('leave cleanup (StrictMode): unmounting after creating a room still closes the WS', () => {
    const ws = new MockWs();
    const { unmount } = renderInviteRoomStrict(ws);

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
    renderInviteRoom(ws);

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
    const { unmount } = renderInviteRoom(ws);

    expect(() => unmount()).not.toThrow();
  });

  // Copy button: writes the code to the clipboard when available, and never crashes when it isn't.
  it('copy-code-button writes the invite code to the clipboard', async () => {
    const ws = new MockWs();
    renderInviteRoom(ws);

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
    renderInviteRoom(ws);

    fireEvent.click(screen.getByTestId('create-room-button'));
    act(() => {
      ws.emitOpen();
      ws.emitMessage(JSON.stringify({ type: 'room_created', room_id: 'room-1', code: 'ABC123' }));
    });

    expect(() => fireEvent.click(screen.getByTestId('copy-code-button'))).not.toThrow();
  });
});
