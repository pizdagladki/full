import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RtcPeerImpl, DEFAULT_STUN } from './index';
import type { WsLike, PcLike, WsFactory, PcFactory } from './index';

// ---------------------------------------------------------------------------
// MockWebSocket
// ---------------------------------------------------------------------------

class MockWebSocket implements WsLike {
  readonly url: string;
  send = vi.fn();
  close = vi.fn();

  private _onopen: (() => void) | null = null;
  private _onmessage: ((ev: { data: string }) => void) | null = null;

  set onopen(cb: (() => void) | null) {
    this._onopen = cb;
  }
  set onmessage(cb: ((ev: { data: string }) => void) | null) {
    this._onmessage = cb;
  }

  constructor(url: string) {
    this.url = url;
  }

  simulateOpen(): void {
    this._onopen?.();
  }

  simulateMessage(data: string): void {
    this._onmessage?.({ data });
  }
}

// ---------------------------------------------------------------------------
// MockRTCPeerConnection
// ---------------------------------------------------------------------------

class MockRTCPeerConnection implements PcLike {
  addTrack = vi.fn();
  close = vi.fn();

  createOffer = vi.fn().mockResolvedValue({
    type: 'offer',
    sdp: 'mock-offer-sdp',
  } as RTCSessionDescriptionInit);

  createAnswer = vi.fn().mockResolvedValue({
    type: 'answer',
    sdp: 'mock-answer-sdp',
  } as RTCSessionDescriptionInit);

  setLocalDescription = vi.fn().mockResolvedValue(undefined);
  setRemoteDescription = vi.fn().mockResolvedValue(undefined);
  addIceCandidate = vi.fn().mockResolvedValue(undefined);

  private _onnegotiationneeded: (() => void) | null = null;
  private _onicecandidate:
    | ((ev: { candidate: RTCIceCandidate | null }) => void)
    | null = null;
  private _ontrack: ((ev: RTCTrackEvent) => void) | null = null;

  set onnegotiationneeded(cb: (() => void) | null) {
    this._onnegotiationneeded = cb;
  }
  set onicecandidate(
    cb: ((ev: { candidate: RTCIceCandidate | null }) => void) | null,
  ) {
    this._onicecandidate = cb;
  }
  set ontrack(cb: ((ev: RTCTrackEvent) => void) | null) {
    this._ontrack = cb;
  }

  simulateNegotiationNeeded(): void {
    this._onnegotiationneeded?.();
  }

  simulateIceCandidate(candidate: RTCIceCandidate | null): void {
    this._onicecandidate?.({ candidate });
  }

  simulateTrack(stream: MediaStream): void {
    this._ontrack?.({ streams: [stream] } as unknown as RTCTrackEvent);
  }
}

// ---------------------------------------------------------------------------
// MockMediaStream / MockMediaStreamTrack
// ---------------------------------------------------------------------------

class MockMediaStreamTrack {
  kind = 'video';
}

class MockMediaStream {
  private tracks: MockMediaStreamTrack[];

  constructor(tracks?: MockMediaStreamTrack[]) {
    this.tracks = tracks ?? [new MockMediaStreamTrack()];
  }

  getTracks(): MockMediaStreamTrack[] {
    return this.tracks;
  }

  addTrack(track: MockMediaStreamTrack): void {
    this.tracks.push(track);
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeFactories(): {
  ws: MockWebSocket;
  pc: MockRTCPeerConnection;
  wsFactory: WsFactory;
  pcFactory: PcFactory;
} {
  const ws = new MockWebSocket('ws://test');
  const pc = new MockRTCPeerConnection();

  const wsFactory: WsFactory = (() => ws) as WsFactory;
  const pcFactory: PcFactory = () => pc;

  return { ws, pc, wsFactory, pcFactory };
}

function makeStream(): MediaStream {
  return new MockMediaStream() as unknown as MediaStream;
}

function makePeer(
  wsFactory: WsFactory,
  pcFactory: PcFactory,
  roomId = 'room-1',
  isOfferer = false,
): RtcPeerImpl {
  return new RtcPeerImpl({
    signalingUrl: 'ws://sig.test',
    room_id: roomId,
    localStream: makeStream(),
    isOfferer,
    wsFactory,
    pcFactory,
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('RtcPeerImpl', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Fix 3: DEFAULT_STUN export — criterion: AC2 STUN server configured
  // ---------------------------------------------------------------------------

  // criterion: 2 — DEFAULT_STUN is configured with a public STUN server URL
  it('DEFAULT_STUN is configured with a public STUN server URL', () => {
    expect(DEFAULT_STUN.iceServers).toBeDefined();
    expect(DEFAULT_STUN.iceServers).toHaveLength(1);
    expect((DEFAULT_STUN.iceServers![0] as RTCIceServer).urls).toBe(
      'stun:stun.l.google.com:19302',
    );
  });

  // criterion: 2 — FAILS if STUN iceServers is empty or missing
  it('DEFAULT_STUN FAILS if iceServers is empty or missing', () => {
    // Verify it is NOT empty — any regression to [] or undefined fails here
    expect(DEFAULT_STUN.iceServers).not.toHaveLength(0);
    expect(DEFAULT_STUN.iceServers).not.toBeUndefined();
  });

  // ---------------------------------------------------------------------------
  // Fix 1: isOfferer role — criterion: AC2 one peer offers, other answers
  // ---------------------------------------------------------------------------

  // criterion: 2 — isOfferer=true: onnegotiationneeded sends offer
  it('isOfferer=true: onnegotiationneeded sends offer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory, 'room-1', true /* isOfferer */);

    ws.simulateOpen();
    ws.send.mockClear(); // clear the join message

    pc.simulateNegotiationNeeded();
    await vi.waitFor(() => expect(ws.send).toHaveBeenCalled());

    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toMatchObject({ type: 'sdp', room_id: 'room-1', sdp: { type: 'offer' } }); // room_id REQUIRED by the wire contract — server drops frames without it
    expect(pc.createOffer).toHaveBeenCalled();
    expect(pc.setLocalDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'offer' }),
    );
  });

  // criterion: 2 — isOfferer=false (default): onnegotiationneeded does NOT send offer
  it('isOfferer=false (default): onnegotiationneeded does NOT send offer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // isOfferer defaults to false
    makePeer(wsFactory, pcFactory, 'room-1', false);

    ws.simulateOpen();
    ws.send.mockClear(); // clear the join message

    // Fire negotiationneeded — callee must do nothing
    pc.simulateNegotiationNeeded();
    await new Promise((r) => setTimeout(r, 20));

    expect(pc.createOffer).not.toHaveBeenCalled();
    expect(ws.send).not.toHaveBeenCalled();
  });

  // criterion: 1 — join on open
  it('join on open: sends {type:"join", room_id} when WS opens', () => {
    const { ws, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory, 'room-42');

    ws.simulateOpen();

    expect(ws.send).toHaveBeenCalledTimes(1);
    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toEqual({ type: 'join', room_id: 'room-42' });
  });

  // criterion: 1 — fails if join is NOT sent on open
  it('join on open FAILS if join message is not sent on WS open', () => {
    const { ws, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory, 'room-42');
    // Do NOT call simulateOpen — ws.send should NOT have been called yet
    expect(ws.send).not.toHaveBeenCalled();
  });

  // criterion: 2 — offer side (isOfferer=true): onnegotiationneeded → sends sdp offer
  it('offer→answer flow: onnegotiationneeded sends sdp offer over WS', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory, 'room-1', true /* isOfferer */);

    ws.simulateOpen();
    ws.send.mockClear(); // clear the join message

    pc.simulateNegotiationNeeded();
    // Wait for the async handler to settle
    await vi.waitFor(() => expect(ws.send).toHaveBeenCalled());

    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toMatchObject({ type: 'sdp', room_id: 'room-1', sdp: { type: 'offer' } }); // room_id REQUIRED by the wire contract — server drops frames without it
    expect(pc.createOffer).toHaveBeenCalled();
    expect(pc.setLocalDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'offer' }),
    );
  });

  // criterion: 2 — answer received → setRemoteDescription called
  it('offer→answer flow: receiving sdp answer calls setRemoteDescription', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // isOfferer=true: sent the offer, now receives the answer
    makePeer(wsFactory, pcFactory, 'room-1', true);
    ws.simulateOpen();

    const answerMsg = JSON.stringify({
      type: 'sdp',
      sdp: { type: 'answer', sdp: 'remote-answer-sdp' },
    });
    ws.simulateMessage(answerMsg);
    await vi.waitFor(() => expect(pc.setRemoteDescription).toHaveBeenCalled());

    expect(pc.setRemoteDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'answer', sdp: 'remote-answer-sdp' }),
    );
  });

  // criterion: 2 — FAILS if setRemoteDescription not called on answer
  it('offer→answer flow FAILS if setRemoteDescription is not called on answer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    // Send a non-answer — setRemoteDescription must NOT be called
    ws.simulateMessage(JSON.stringify({ type: 'peer_left' }));
    await new Promise((r) => setTimeout(r, 10));
    expect(pc.setRemoteDescription).not.toHaveBeenCalled();
  });

  // criterion: 2 — answer side (incoming offer): createAnswer + send answer
  // Works with isOfferer=false (default) — callee always handles incoming offers
  it('answer flow: incoming offer triggers createAnswer and sends sdp answer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // isOfferer=false: this peer is the callee; it answers incoming offers
    makePeer(wsFactory, pcFactory, 'room-1', false);
    ws.simulateOpen();
    ws.send.mockClear();

    const offerMsg = JSON.stringify({
      type: 'sdp',
      sdp: { type: 'offer', sdp: 'remote-offer-sdp' },
    });
    ws.simulateMessage(offerMsg);

    await vi.waitFor(() =>
      expect(ws.send).toHaveBeenCalledWith(
        expect.stringContaining('"type":"sdp"'),
      ),
    );

    expect(pc.setRemoteDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'offer', sdp: 'remote-offer-sdp' }),
    );
    expect(pc.createAnswer).toHaveBeenCalled();
    expect(pc.setLocalDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'answer' }),
    );

    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toMatchObject({
      type: 'sdp',
      room_id: 'room-1', // room_id REQUIRED by the wire contract - server drops frames without it
      sdp: { type: 'answer' },
    });
  });

  // criterion: 2 — FAILS if answer is not sent when offer arrives
  it('answer flow FAILS if answer is not sent after receiving an offer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // Make createAnswer return nothing useful (override)
    pc.createAnswer = vi.fn().mockResolvedValue({
      type: 'answer',
      sdp: 'mock-answer-sdp',
    });
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    ws.send.mockClear();

    // Only send a non-offer — answer WS send should NOT be triggered
    ws.simulateMessage(
      JSON.stringify({
        type: 'ice',
        candidate: { candidate: 'c', sdpMid: '0', sdpMLineIndex: 0 },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));
    expect(ws.send).not.toHaveBeenCalled();
  });

  // criterion: 2 — ICE relay outgoing
  it('ICE relay — outgoing: onicecandidate sends ice message over WS', () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    ws.send.mockClear();

    const mockCandidate = {
      candidate: 'candidate:1 udp 12345 192.168.1.1 12345 typ host',
      sdpMid: '0',
      sdpMLineIndex: 0,
      toJSON: () => ({
        candidate: 'candidate:1 udp 12345 192.168.1.1 12345 typ host',
        sdpMid: '0',
        sdpMLineIndex: 0,
      }),
    } as unknown as RTCIceCandidate;

    pc.simulateIceCandidate(mockCandidate);

    expect(ws.send).toHaveBeenCalledTimes(1);
    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toMatchObject({ type: 'ice', room_id: 'room-1', candidate: expect.anything() }); // room_id REQUIRED by the wire contract
  });

  // criterion: 2 — FAILS if outgoing ICE is not relayed
  it('ICE relay — outgoing FAILS if null candidate is sent over WS', () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    ws.send.mockClear();

    // Null candidate must be ignored (end-of-candidates signal)
    pc.simulateIceCandidate(null);

    expect(ws.send).not.toHaveBeenCalled();
  });

  // criterion: 2 — ICE relay incoming (after remote description is set)
  it('ICE relay — incoming: ice message calls pc.addIceCandidate', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    // First establish the remote description so ICE is not queued
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        sdp: { type: 'offer', sdp: 'remote-offer-sdp' },
      }),
    );
    await vi.waitFor(() => expect(pc.setRemoteDescription).toHaveBeenCalled());

    const iceMsg = JSON.stringify({
      type: 'ice',
      candidate: {
        candidate: 'candidate:0 1 UDP 12345 192.168.0.1 54321 typ host',
        sdpMid: '0',
        sdpMLineIndex: 0,
      },
    });
    ws.simulateMessage(iceMsg);

    await vi.waitFor(() => expect(pc.addIceCandidate).toHaveBeenCalled());
    expect(pc.addIceCandidate).toHaveBeenCalledWith(
      expect.objectContaining({ candidate: expect.stringContaining('host') }),
    );
  });

  // criterion: 2 — FAILS if incoming ICE is not forwarded to addIceCandidate
  it('ICE relay — incoming FAILS if sdp message does not call addIceCandidate', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    // Send an SDP answer message — addIceCandidate must NOT be called
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        sdp: { type: 'answer', sdp: 'x' },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
  });

  // ---------------------------------------------------------------------------
  // Fix 4: ICE candidate queue — criterion: AC2 no addIceCandidate before setRemoteDescription
  // ---------------------------------------------------------------------------

  // criterion: 4 — ICE candidates received before remote description are queued and flushed after sdp answer
  it('ICE candidates received before remote description are queued and flushed after sdp answer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // isOfferer=true peer: sends offer, receives answer, may receive ICE before the answer arrives
    makePeer(wsFactory, pcFactory, 'room-1', true);
    ws.simulateOpen();
    ws.send.mockClear();

    // Receive an ICE candidate BEFORE the remote description (answer) is set
    ws.simulateMessage(
      JSON.stringify({
        type: 'ice',
        candidate: { candidate: 'early-cand', sdpMid: '0', sdpMLineIndex: 0 },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));

    // addIceCandidate must NOT have been called yet — no remote description
    expect(pc.addIceCandidate).not.toHaveBeenCalled();

    // Now receive the answer (sets remoteDescription) — queued candidate must be flushed
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        sdp: { type: 'answer', sdp: 'answer-sdp' },
      }),
    );
    await vi.waitFor(() => expect(pc.addIceCandidate).toHaveBeenCalled());

    // addIceCandidate must have been called AFTER setRemoteDescription
    const setRemoteOrder = pc.setRemoteDescription.mock.invocationCallOrder[0];
    const addIceOrder = pc.addIceCandidate.mock.invocationCallOrder[0];
    expect(addIceOrder).toBeGreaterThan(setRemoteOrder);

    expect(pc.addIceCandidate).toHaveBeenCalledWith(
      expect.objectContaining({ candidate: 'early-cand' }),
    );
  });

  // criterion: 4 — ICE candidates received before remote description are queued and flushed after sdp offer-answer
  it('ICE candidates received before remote description are queued and flushed after sdp offer-answer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    // isOfferer=false peer: receives offer and answers; may get ICE before the offer
    makePeer(wsFactory, pcFactory, 'room-1', false);
    ws.simulateOpen();
    ws.send.mockClear();

    // Receive an ICE candidate BEFORE the offer (remote description) arrives
    ws.simulateMessage(
      JSON.stringify({
        type: 'ice',
        candidate: { candidate: 'early-callee-cand', sdpMid: '0', sdpMLineIndex: 0 },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));

    // addIceCandidate must NOT have been called yet
    expect(pc.addIceCandidate).not.toHaveBeenCalled();

    // Now receive the offer (sets remoteDescription) — queued candidate must be flushed before answer
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        sdp: { type: 'offer', sdp: 'remote-offer-sdp' },
      }),
    );
    await vi.waitFor(() => expect(pc.addIceCandidate).toHaveBeenCalled());

    // addIceCandidate must have been called AFTER setRemoteDescription
    const setRemoteOrder = pc.setRemoteDescription.mock.invocationCallOrder[0];
    const addIceOrder = pc.addIceCandidate.mock.invocationCallOrder[0];
    expect(addIceOrder).toBeGreaterThan(setRemoteOrder);

    expect(pc.addIceCandidate).toHaveBeenCalledWith(
      expect.objectContaining({ candidate: 'early-callee-cand' }),
    );
  });

  // criterion: 3 — remote-stream callback fires on ontrack
  it('remote-stream callback: ontrack fires onRemoteStream with the stream', () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    const peer = makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    const cb = vi.fn();
    peer.onRemoteStream(cb);

    const remoteStream = makeStream();
    pc.simulateTrack(remoteStream);

    expect(cb).toHaveBeenCalledTimes(1);
    expect(cb).toHaveBeenCalledWith(remoteStream);
  });

  // criterion: 3 — FAILS if remote-stream callback is not fired
  it('remote-stream callback FAILS if onRemoteStream is not called on track event', () => {
    const { ws, wsFactory, pcFactory } = makeFactories();
    const peer = makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    const cb = vi.fn();
    peer.onRemoteStream(cb);

    // Don't simulate a track — cb must not have been called
    expect(cb).not.toHaveBeenCalled();
  });

  // criterion: 3 — peer_left teardown
  it('peer_left teardown: peerLeftCb fires, pc.close() and ws.close() called', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    const peer = makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    const leftCb = vi.fn();
    peer.onPeerLeft(leftCb);

    ws.simulateMessage(JSON.stringify({ type: 'peer_left' }));

    await vi.waitFor(() => expect(leftCb).toHaveBeenCalled());
    expect(pc.close).toHaveBeenCalled();
    expect(ws.close).toHaveBeenCalled();
  });

  // criterion: 3 — FAILS if peer_left does not close pc and ws
  it('peer_left teardown FAILS if pc.close is not called on peer_left', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    // Send ICE, not peer_left — pc.close must NOT be called
    ws.simulateMessage(
      JSON.stringify({
        type: 'ice',
        candidate: { candidate: 'x', sdpMid: '0', sdpMLineIndex: 0 },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));
    expect(pc.close).not.toHaveBeenCalled();
  });

  // criterion: 4 — all signaling I/O goes through WS (no direct peer transport)
  // Room filter: unknown/malformed messages don't crash and are silently ignored
  it('room filter: malformed WS message does not throw', () => {
    const { ws, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    // Should not throw
    expect(() => ws.simulateMessage('not-valid-json')).not.toThrow();
    expect(() =>
      ws.simulateMessage(JSON.stringify({ type: 'unknown_type', data: 42 })),
    ).not.toThrow();
  });

  // criterion: 4 — FAILS if room filter is bypassed (signals for unknown types cause errors)
  it('room filter FAILS if unknown message type causes unhandled error', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    ws.send.mockClear();

    // Send a message with an irrelevant type — nothing should be called on pc
    ws.simulateMessage(JSON.stringify({ type: 'join', room_id: 'other' }));
    await new Promise((r) => setTimeout(r, 10));

    // pc methods must not be called for a "join" message from server
    expect(pc.setRemoteDescription).not.toHaveBeenCalled();
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
    expect(pc.createAnswer).not.toHaveBeenCalled();
  });

  // criterion: 1, 2 — local tracks are added to pc from the localStream
  it('constructor adds localStream tracks to pc', () => {
    const { pc, wsFactory, pcFactory } = makeFactories();
    const track = new MockMediaStreamTrack() as unknown as MediaStreamTrack;
    const stream = new MockMediaStream([
      track as unknown as MockMediaStreamTrack,
    ]) as unknown as MediaStream;

    new RtcPeerImpl({
      signalingUrl: 'ws://sig.test',
      room_id: 'room-1',
      localStream: stream,
      wsFactory,
      pcFactory,
    });

    expect(pc.addTrack).toHaveBeenCalledWith(track, stream);
  });

  // criterion: 1 — close() calls ws.close and pc.close
  it('close() shuts down ws and pc', () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    const peer = makePeer(wsFactory, pcFactory);

    peer.close();

    expect(ws.close).toHaveBeenCalled();
    expect(pc.close).toHaveBeenCalled();
  });

  // criterion: error path — onnegotiationneeded error is caught
  it('onnegotiationneeded error is caught and does not throw', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    pc.createOffer = vi.fn().mockRejectedValue(new Error('offer-rejected'));
    makePeer(wsFactory, pcFactory, 'room-1', true /* isOfferer */);
    ws.simulateOpen();
    pc.simulateNegotiationNeeded();
    // Let the async catch run
    await new Promise((r) => setTimeout(r, 50));
    // no unhandled rejection = pass
    expect(pc.createOffer).toHaveBeenCalled();
  });

  // criterion: error path — handleSignalingMessage error is caught
  it('handleSignalingMessage error is caught and does not throw', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    pc.addIceCandidate = vi.fn().mockRejectedValue(new Error('ice-rejected'));
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    // First set remote description so ICE is not queued
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        sdp: { type: 'offer', sdp: 'offer-sdp' },
      }),
    );
    await vi.waitFor(() => expect(pc.setRemoteDescription).toHaveBeenCalled());

    ws.simulateMessage(
      JSON.stringify({
        type: 'ice',
        candidate: { candidate: 'c', sdpMid: '0', sdpMLineIndex: 0 },
      }),
    );
    await new Promise((r) => setTimeout(r, 50));
    // no unhandled rejection = pass
    expect(pc.addIceCandidate).toHaveBeenCalled();
  });
});
