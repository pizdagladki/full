import { createRef } from 'react';
import { render } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { RtcComponent } from './index';
import type { RtcHandle, WsFactory, PcFactory, WsLike, PcLike } from './index';

// ---------------------------------------------------------------------------
// MockWebSocket — minimal stub; setters accept callbacks but don't need to fire them
// ---------------------------------------------------------------------------

class MockWebSocket implements WsLike {
  send = vi.fn();
  close = vi.fn();

  // Setters store callbacks; they are wired by RtcPeerImpl (setter-only interface)
  set onopen(_cb: (() => void) | null) {
    // no-op in this component-level test
  }
  set onmessage(_cb: ((ev: { data: string }) => void) | null) {
    // no-op in this component-level test
  }
}

// ---------------------------------------------------------------------------
// MockRTCPeerConnection — minimal stub
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

  // Setters required by PcLike; not exercised in component-level tests
  set onnegotiationneeded(_cb: (() => void) | null) {
    // no-op
  }
  set onicecandidate(
    _cb: ((ev: { candidate: RTCIceCandidate | null }) => void) | null,
  ) {
    // no-op
  }
  set ontrack(_cb: ((ev: RTCTrackEvent) => void) | null) {
    // no-op
  }
}

// ---------------------------------------------------------------------------
// MockMediaStream
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
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('RtcComponent', () => {
  // criterion: RtcComponent — connect calls RtcPeerImpl; onRemoteStream, onPeerLeft, close work
  it('connect → calls RtcPeerImpl; onRemoteStream, onPeerLeft, close work', () => {
    const ws = new MockWebSocket();
    const pc = new MockRTCPeerConnection();
    const wsFactory: WsFactory = (() => ws) as WsFactory;
    const pcFactory: PcFactory = () => pc;

    const ref = createRef<RtcHandle>();
    render(
      <RtcComponent
        ref={ref}
        signalingUrl="ws://sig"
        wsFactory={wsFactory}
        pcFactory={pcFactory}
      />,
    );

    const stream = new MockMediaStream() as unknown as MediaStream;
    ref.current!.connect({ room_id: 'r1', localStream: stream });

    // Verify that local tracks were added to the peer connection
    expect(pc.addTrack).toHaveBeenCalled();

    // onRemoteStream callback registered — does not throw
    const streamCb = vi.fn();
    ref.current!.onRemoteStream(streamCb);

    // onPeerLeft callback registered — does not throw
    const leftCb = vi.fn();
    ref.current!.onPeerLeft(leftCb);

    // close tears down ws and pc
    ref.current!.close();

    expect(ws.close).toHaveBeenCalled();
    expect(pc.close).toHaveBeenCalled();
  });

  // criterion: RtcComponent — connect called twice closes the previous peer first
  it('connect called twice closes the previous peer first', () => {
    const ws1 = new MockWebSocket();
    const pc1 = new MockRTCPeerConnection();
    const ws2 = new MockWebSocket();
    const pc2 = new MockRTCPeerConnection();

    const wsMocks = [ws1, ws2];
    const pcMocks = [pc1, pc2];
    let wsIdx = 0;
    let pcIdx = 0;

    const wsFactory: WsFactory = (() => wsMocks[wsIdx++]) as WsFactory;
    const pcFactory: PcFactory = () => pcMocks[pcIdx++];

    const ref = createRef<RtcHandle>();
    render(
      <RtcComponent
        ref={ref}
        signalingUrl="ws://sig"
        wsFactory={wsFactory}
        pcFactory={pcFactory}
      />,
    );

    const stream = new MockMediaStream() as unknown as MediaStream;

    // First connect — uses ws1/pc1
    ref.current!.connect({ room_id: 'r1', localStream: stream });
    expect(pc1.addTrack).toHaveBeenCalled();

    // Second connect — should close the first peer (ws1.close, pc1.close) before creating the new one
    ref.current!.connect({ room_id: 'r2', localStream: stream });

    expect(ws1.close).toHaveBeenCalled();
    expect(pc1.close).toHaveBeenCalled();
    expect(pc2.addTrack).toHaveBeenCalled();
  });

  // criterion: RtcComponent — onRemoteStream/onPeerLeft/close are no-ops when no peer connected
  it('onRemoteStream, onPeerLeft, and close are no-ops when no peer is connected', () => {
    const ref = createRef<RtcHandle>();
    render(<RtcComponent ref={ref} signalingUrl="ws://sig" />);

    // Should not throw even without a connected peer
    expect(() => ref.current!.onRemoteStream(vi.fn())).not.toThrow();
    expect(() => ref.current!.onPeerLeft(vi.fn())).not.toThrow();
    expect(() => ref.current!.close()).not.toThrow();
  });

  // criterion: RtcComponent — renders null and exposes a ref handle
  it('renders null and exposes ref handle', () => {
    const ws = new MockWebSocket();
    const pc = new MockRTCPeerConnection();
    const wsFactory: WsFactory = (() => ws) as WsFactory;
    const pcFactory: PcFactory = () => pc;

    const ref = createRef<RtcHandle>();
    const { container } = render(
      <RtcComponent
        ref={ref}
        wsFactory={wsFactory}
        pcFactory={pcFactory}
      />,
    );

    // Component renders null — DOM is empty
    expect(container.firstChild).toBeNull();
    // ref handle is available
    expect(ref.current).not.toBeNull();
  });
});
