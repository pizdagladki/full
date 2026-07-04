import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { WsClient } from '../api/ws';
import type { WsClientApi } from '../api/ws';

// ---------------------------------------------------------------------------
// WS message shapes (invite-a-friend private room protocol) — kept local, no
// untyped `any`.
// ---------------------------------------------------------------------------

interface CreateRoomMsg {
  type: 'create_room';
}

interface JoinRoomMsg {
  type: 'join_room';
  code: string;
}

/** Shape of a WS frame we don't yet know — only `type` is guaranteed. */
interface UnknownServerMsg {
  type?: string;
  room_id?: string;
  code?: string;
  error?: string;
}

const SIGNALING_WS_PATH = '/ws';

type Phase = 'menu' | 'creating' | 'waiting' | 'joining' | 'error';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface InviteRoomProps {
  /** Injectable WS client (swap with a mock in tests). Defaults to a lazily-built WsClient. */
  wsClient?: WsClientApi;
}

// ---------------------------------------------------------------------------
// InviteRoom component — invite-a-friend private room: create/copy a code, or
// join by code, then hand off to the battle screen (unranked).
// ---------------------------------------------------------------------------

export function InviteRoom({ wsClient }: InviteRoomProps) {
  const navigate = useNavigate();

  // Lazily build the default WsClient once — never rebuilt on re-render (same seam as
  // Search/Battle: a useRef that's only initialized once).
  const wsRef = useRef<WsClientApi>();
  if (wsRef.current == null) {
    wsRef.current = wsClient ?? new WsClient();
  }

  // Guards teardown so it only runs once per connection (mirrors Battle.tsx's teardown pattern).
  const teardownRef = useRef(false);

  const [phase, setPhase] = useState<Phase>('menu');
  const [code, setCode] = useState('');
  const [roomId, setRoomId] = useState('');
  const [joinCodeInput, setJoinCodeInput] = useState('');
  const [errorMessage, setErrorMessage] = useState('');
  const [copyLabel, setCopyLabel] = useState('Copy');

  const teardown = useCallback(() => {
    if (teardownRef.current) return;
    teardownRef.current = true;
    wsRef.current?.close();
  }, []);

  const resetToMenu = useCallback(() => {
    teardown();
    // A future create/join needs a fresh connection — reset the guard so teardown can run again.
    teardownRef.current = false;
    setPhase('menu');
    setCode('');
    setRoomId('');
    setErrorMessage('');
    setCopyLabel('Copy');
  }, [teardown]);

  const handleMessage = useCallback(
    (data: string) => {
      try {
        const msg = JSON.parse(data) as UnknownServerMsg;
        if (msg.type === 'room_created' && typeof msg.room_id === 'string' && msg.room_id) {
          setRoomId(msg.room_id);
          setCode(typeof msg.code === 'string' ? msg.code : '');
          setPhase('waiting');
          return;
        }
        if (msg.type === 'room_joined' && typeof msg.room_id === 'string' && msg.room_id) {
          // Per the signaling protocol the creator receives NO push when the friend joins — only
          // the joiner gets `room_joined`. Receiving it here proves the room already had the
          // creator present, so both peers are now in the room; navigate straight to battle.
          teardown();
          navigate('/battle', { state: { roomId: msg.room_id } });
          return;
        }
        if (msg.type === 'error') {
          setErrorMessage(msg.error ?? 'Something went wrong. Please try again.');
          setPhase('error');
        }
      } catch {
        // ignore malformed WS frames — never let a bad frame crash the screen
      }
    },
    [teardown, navigate],
  );

  // The native WebSocket (and WsClient, which does no readyState guarding) throws
  // InvalidStateError if send() is called while the socket is still CONNECTING. So the
  // create_room/join_room frame must NOT be sent synchronously right after connect() — it has to
  // wait for the socket to actually open. Mirrors Search.tsx's onOpen-gated join.
  const startConnection = useCallback(
    (onOpenSend: () => void) => {
      const ws = wsRef.current!;
      ws.connect(SIGNALING_WS_PATH);
      ws.onOpen(onOpenSend);
      ws.onMessage(handleMessage);
    },
    [handleMessage],
  );

  const handleCreateRoom = useCallback(() => {
    setPhase('creating');
    setErrorMessage('');
    startConnection(() => {
      const msg: CreateRoomMsg = { type: 'create_room' };
      wsRef.current?.send(JSON.stringify(msg));
    });
  }, [startConnection]);

  const handleJoinSubmit = useCallback(() => {
    const trimmed = joinCodeInput.trim();
    if (!trimmed) return;
    setPhase('joining');
    setErrorMessage('');
    startConnection(() => {
      const msg: JoinRoomMsg = { type: 'join_room', code: trimmed };
      wsRef.current?.send(JSON.stringify(msg));
    });
  }, [joinCodeInput, startConnection]);

  const handleStartBattle = useCallback(() => {
    teardown();
    navigate('/battle', { state: { roomId } });
  }, [teardown, navigate, roomId]);

  const handleCopy = useCallback(() => {
    if (!navigator.clipboard?.writeText) return;
    navigator.clipboard
      .writeText(code)
      .then(() => setCopyLabel('Copied!'))
      .catch(() => {
        // clipboard write failed — leave the label as-is, no crash
      });
  }, [code]);

  // Unmount cleanup — mirrors Battle.tsx's teardown pattern: close the WS unconditionally (guarded
  // by teardownRef so it only runs once) whether a connection was opened or not. The guard MUST be
  // re-armed in the effect body: under React.StrictMode the mount effect runs setup → synthetic
  // cleanup → setup on the same refs, and without the reset the latched guard turns every later
  // teardown into a no-op — leaving a ghost WS/room on Leave/navigate (criterion 4).
  useEffect(() => {
    teardownRef.current = false;
    return () => teardown();
  }, [teardown]);

  return (
    <div data-testid="invite-room-screen">
      {phase === 'menu' && (
        <div data-testid="invite-menu">
          <button type="button" data-testid="create-room-button" onClick={handleCreateRoom}>
            Create room
          </button>
          <div data-testid="join-room-form">
            <input
              type="text"
              data-testid="join-code-input"
              value={joinCodeInput}
              onChange={(e) => setJoinCodeInput(e.target.value)}
              placeholder="Enter invite code"
            />
            <button type="button" data-testid="join-room-button" onClick={handleJoinSubmit}>
              Join by code
            </button>
          </div>
        </div>
      )}

      {phase === 'creating' && <div data-testid="invite-creating">Creating room…</div>}

      {phase === 'waiting' && (
        <div data-testid="invite-waiting">
          <div data-testid="invite-code">{code}</div>
          <button type="button" data-testid="copy-code-button" onClick={handleCopy}>
            {copyLabel}
          </button>
          <div data-testid="invite-waiting-message">Waiting for your friend to join…</div>
          {/*
            The signaling server never pushes a peer-joined notification to the room creator
            (JoinByCode/handleJoinRoom only writes room_joined back to the joiner's own
            connection — there is no broadcast to the other room member). Rather than pretend
            this is automatic, the creator manually confirms the friend has joined out-of-band
            (e.g. a chat message) and starts the battle themselves.
          */}
          <button type="button" data-testid="start-battle-button" onClick={handleStartBattle}>
            Start Battle
          </button>
          <button type="button" data-testid="leave-button" onClick={resetToMenu}>
            Leave
          </button>
        </div>
      )}

      {phase === 'joining' && (
        <div data-testid="invite-joining">
          <div>Joining room…</div>
          <button type="button" data-testid="leave-button" onClick={resetToMenu}>
            Leave
          </button>
        </div>
      )}

      {phase === 'error' && (
        <div data-testid="invite-error-screen">
          <div data-testid="invite-error">{errorMessage}</div>
          <button type="button" data-testid="retry-button" onClick={resetToMenu}>
            Try again
          </button>
        </div>
      )}
    </div>
  );
}
