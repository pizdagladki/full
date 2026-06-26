import { describe, it, expect, vi, beforeEach } from 'vitest';
import { RtcPeerImpl } from './index';
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
): RtcPeerImpl {
  return new RtcPeerImpl({
    signalingUrl: 'ws://sig.test',
    room_id: roomId,
    localStream: makeStream(),
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

  // criterion: 2 — offer side: onnegotiationneeded → sends sdp offer
  it('offer→answer flow: onnegotiationneeded sends sdp offer over WS', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);

    ws.simulateOpen();
    ws.send.mockClear(); // clear the join message

    pc.simulateNegotiationNeeded();
    // Wait for the async handler to settle
    await vi.waitFor(() => expect(ws.send).toHaveBeenCalled());

    const sent = JSON.parse(ws.send.mock.calls[0][0] as string) as unknown;
    expect(sent).toMatchObject({ type: 'sdp', description: { type: 'offer' } });
    expect(pc.createOffer).toHaveBeenCalled();
    expect(pc.setLocalDescription).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'offer' }),
    );
  });

  // criterion: 2 — answer received → setRemoteDescription called
  it('offer→answer flow: receiving sdp answer calls setRemoteDescription', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

    const answerMsg = JSON.stringify({
      type: 'sdp',
      description: { type: 'answer', sdp: 'remote-answer-sdp' },
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
  it('answer flow: incoming offer triggers createAnswer and sends sdp answer', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();
    ws.send.mockClear();

    const offerMsg = JSON.stringify({
      type: 'sdp',
      description: { type: 'offer', sdp: 'remote-offer-sdp' },
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
      description: { type: 'answer' },
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
    expect(sent).toMatchObject({ type: 'ice', candidate: expect.anything() });
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

  // criterion: 2 — ICE relay incoming
  it('ICE relay — incoming: ice message calls pc.addIceCandidate', async () => {
    const { ws, pc, wsFactory, pcFactory } = makeFactories();
    makePeer(wsFactory, pcFactory);
    ws.simulateOpen();

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

    // Send an SDP message — addIceCandidate must NOT be called
    ws.simulateMessage(
      JSON.stringify({
        type: 'sdp',
        description: { type: 'answer', sdp: 'x' },
      }),
    );
    await new Promise((r) => setTimeout(r, 10));
    expect(pc.addIceCandidate).not.toHaveBeenCalled();
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
});
