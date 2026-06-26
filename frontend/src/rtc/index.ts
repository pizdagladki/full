// rtc — WebRTC peer connection + STUN/TURN + signaling client.
// Imperative; accessed via refs. Fully injectable for testing.

import {
  forwardRef,
  useImperativeHandle,
  useRef,
  type ForwardedRef,
} from 'react';

// ---------------------------------------------------------------------------
// Signaling message types (discriminated union matching the #24 contract)
// ---------------------------------------------------------------------------

interface JoinMsg {
  type: 'join';
  room_id: string;
}

interface SdpMsg {
  type: 'sdp';
  description: RTCSessionDescriptionInit;
}

interface IceMsg {
  type: 'ice';
  candidate: RTCIceCandidateInit;
}

interface PeerLeftMsg {
  type: 'peer_left';
}

type SignalingMsg = JoinMsg | SdpMsg | IceMsg | PeerLeftMsg;

// ---------------------------------------------------------------------------
// Minimal WebSocket interface (so tests can inject a mock)
// ---------------------------------------------------------------------------

export interface WsLike {
  send(data: string): void;
  close(): void;
  set onopen(cb: (() => void) | null);
  set onmessage(cb: ((ev: { data: string }) => void) | null);
}

// ---------------------------------------------------------------------------
// Minimal RTCPeerConnection interface (so tests can inject a mock)
// ---------------------------------------------------------------------------

export interface PcLike {
  addTrack(track: MediaStreamTrack, ...streams: MediaStream[]): RTCRtpSender;
  createOffer(options?: RTCOfferOptions): Promise<RTCSessionDescriptionInit>;
  createAnswer(
    options?: RTCAnswerOptions,
  ): Promise<RTCSessionDescriptionInit>;
  setLocalDescription(
    description: RTCSessionDescriptionInit,
  ): Promise<void>;
  setRemoteDescription(
    description: RTCSessionDescriptionInit,
  ): Promise<void>;
  addIceCandidate(candidate: RTCIceCandidateInit): Promise<void>;
  close(): void;
  set onnegotiationneeded(cb: (() => void) | null);
  set onicecandidate(
    cb: ((ev: { candidate: RTCIceCandidate | null }) => void) | null,
  );
  set ontrack(cb: ((ev: RTCTrackEvent) => void) | null);
}

// ---------------------------------------------------------------------------
// Factories (defaults use real browser APIs; tests inject mocks)
// ---------------------------------------------------------------------------

export type WsFactory = (url: string) => WsLike;
export type PcFactory = () => PcLike;

const DEFAULT_STUN: RTCConfiguration = {
  iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
};

const defaultWsFactory: WsFactory = (url: string) =>
  new WebSocket(url) as unknown as WsLike;

const defaultPcFactory: PcFactory = () =>
  new RTCPeerConnection(DEFAULT_STUN) as unknown as PcLike;

// ---------------------------------------------------------------------------
// RtcPeerImpl — the core logic class (exported for direct unit testing)
// ---------------------------------------------------------------------------

export interface RtcPeerImplOpts {
  signalingUrl: string;
  room_id: string;
  localStream: MediaStream;
  wsFactory?: WsFactory;
  pcFactory?: PcFactory;
}

export class RtcPeerImpl {
  private readonly pc: PcLike;
  private readonly ws: WsLike;
  private readonly roomId: string;
  private remoteStreamCb: ((stream: MediaStream) => void) | undefined;
  private peerLeftCb: (() => void) | undefined;

  constructor(opts: RtcPeerImplOpts) {
    const {
      signalingUrl,
      room_id,
      localStream,
      wsFactory = defaultWsFactory,
      pcFactory = defaultPcFactory,
    } = opts;

    this.roomId = room_id;

    // Create peer connection and add local tracks
    this.pc = pcFactory();
    for (const track of localStream.getTracks()) {
      this.pc.addTrack(track, localStream);
    }

    // Wire negotiation: when PC needs to negotiate, create + send an offer
    this.pc.onnegotiationneeded = () => {
      this.handleNegotiationNeeded().catch((err: unknown) => {
        console.error('[rtc] onnegotiationneeded error', err);
      });
    };

    // Relay outgoing ICE candidates over the WS
    this.pc.onicecandidate = (ev) => {
      if (ev.candidate) {
        const msg: IceMsg = { type: 'ice', candidate: ev.candidate };
        this.ws.send(JSON.stringify(msg));
      }
    };

    // Fire remoteStreamCb when a remote track arrives
    this.pc.ontrack = (ev) => {
      if (this.remoteStreamCb) {
        this.remoteStreamCb(ev.streams[0]);
      }
    };

    // Create WS and wire open / message handlers
    this.ws = wsFactory(signalingUrl);

    this.ws.onopen = () => {
      const msg: JoinMsg = { type: 'join', room_id: this.roomId };
      this.ws.send(JSON.stringify(msg));
    };

    this.ws.onmessage = (ev) => {
      let parsed: unknown;
      try {
        parsed = JSON.parse(ev.data) as unknown;
      } catch {
        console.error('[rtc] failed to parse WS message', ev.data);
        return;
      }
      this.handleSignalingMessage(parsed as SignalingMsg).catch(
        (err: unknown) => {
          console.error('[rtc] handleSignalingMessage error', err);
        },
      );
    };
  }

  onRemoteStream(cb: (stream: MediaStream) => void): void {
    this.remoteStreamCb = cb;
  }

  onPeerLeft(cb: () => void): void {
    this.peerLeftCb = cb;
  }

  close(): void {
    this.ws.close();
    this.pc.close();
  }

  private async handleNegotiationNeeded(): Promise<void> {
    const offer = await this.pc.createOffer();
    await this.pc.setLocalDescription(offer);
    const msg: SdpMsg = { type: 'sdp', description: offer };
    this.ws.send(JSON.stringify(msg));
  }

  private async handleSignalingMessage(msg: SignalingMsg): Promise<void> {
    switch (msg.type) {
      case 'sdp': {
        const { description } = msg;
        if (description.type === 'offer') {
          await this.pc.setRemoteDescription(description);
          const answer = await this.pc.createAnswer();
          await this.pc.setLocalDescription(answer);
          const reply: SdpMsg = { type: 'sdp', description: answer };
          this.ws.send(JSON.stringify(reply));
        } else if (description.type === 'answer') {
          await this.pc.setRemoteDescription(description);
        }
        break;
      }
      case 'ice': {
        await this.pc.addIceCandidate(msg.candidate);
        break;
      }
      case 'peer_left': {
        this.peerLeftCb?.();
        this.pc.close();
        this.ws.close();
        break;
      }
      case 'join':
        // Server should not relay join back; ignore
        break;
    }
  }
}

// ---------------------------------------------------------------------------
// RtcHandle — the imperative handle shape exposed via ref
// ---------------------------------------------------------------------------

export interface RtcHandle {
  connect(opts: { room_id: string; localStream: MediaStream }): void;
  onRemoteStream(cb: (stream: MediaStream) => void): void;
  onPeerLeft(cb: () => void): void;
  close(): void;
}

// ---------------------------------------------------------------------------
// RtcComponentProps — injectable deps for the React component
// ---------------------------------------------------------------------------

export interface RtcComponentProps {
  signalingUrl?: string;
  wsFactory?: WsFactory;
  pcFactory?: PcFactory;
}

// ---------------------------------------------------------------------------
// RtcComponent — null-rendering React component; exposes RtcHandle via ref
// ---------------------------------------------------------------------------

export const RtcComponent = forwardRef(function RtcComponent(
  props: RtcComponentProps,
  ref: ForwardedRef<RtcHandle>,
) {
  const peerRef = useRef<RtcPeerImpl | null>(null);
  const { signalingUrl = '', wsFactory, pcFactory } = props;

  useImperativeHandle(ref, () => ({
    connect({ room_id, localStream }) {
      if (peerRef.current) {
        peerRef.current.close();
      }
      peerRef.current = new RtcPeerImpl({
        signalingUrl,
        room_id,
        localStream,
        wsFactory,
        pcFactory,
      });
    },
    onRemoteStream(cb) {
      peerRef.current?.onRemoteStream(cb);
    },
    onPeerLeft(cb) {
      peerRef.current?.onPeerLeft(cb);
    },
    close() {
      peerRef.current?.close();
      peerRef.current = null;
    },
  }));

  return null;
});
