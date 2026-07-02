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

// Wire contract (services/signaling/CLAUDE.md): sdp/ice REQUIRE room_id —
// the server routes by env.RoomID and silently drops frames without it —
// and the SDP payload field is `sdp`, not `description`.
interface SdpMsg {
  type: 'sdp';
  room_id: string;
  sdp: RTCSessionDescriptionInit;
}

interface IceMsg {
  type: 'ice';
  room_id: string;
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

// Fix 3: export DEFAULT_STUN so tests can assert the STUN configuration
export const DEFAULT_STUN: RTCConfiguration = {
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
  // Fix 1: isOfferer controls which peer sends the initial offer
  isOfferer?: boolean;
  wsFactory?: WsFactory;
  pcFactory?: PcFactory;
}

export class RtcPeerImpl {
  private readonly pc: PcLike;
  private readonly ws: WsLike;
  private readonly roomId: string;
  private remoteStreamCb: ((stream: MediaStream) => void) | undefined;
  private peerLeftCb: (() => void) | undefined;
  // Fix 4: ICE candidate queue — hold candidates until remote description is set
  private pendingCandidates: RTCIceCandidateInit[] = [];
  private remoteDescriptionSet = false;

  constructor(opts: RtcPeerImplOpts) {
    const {
      signalingUrl,
      room_id,
      localStream,
      isOfferer = false,
      wsFactory = defaultWsFactory,
      pcFactory = defaultPcFactory,
    } = opts;

    this.roomId = room_id;

    // Create peer connection and add local tracks
    this.pc = pcFactory();
    for (const track of localStream.getTracks()) {
      this.pc.addTrack(track, localStream);
    }

    // Fix 1: only the offerer wires onnegotiationneeded → createOffer.
    // The callee sets it to null to prevent offer glare.
    this.pc.onnegotiationneeded = isOfferer
      ? () => {
          this.handleNegotiationNeeded().catch((err: unknown) => {
            console.error('[rtc] onnegotiationneeded error', err);
          });
        }
      : null; // callee never initiates offers

    // Relay outgoing ICE candidates over the WS
    this.pc.onicecandidate = (ev) => {
      if (ev.candidate) {
        const msg: IceMsg = { type: 'ice', room_id: this.roomId, candidate: ev.candidate };
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
    const msg: SdpMsg = { type: 'sdp', room_id: this.roomId, sdp: offer };
    this.ws.send(JSON.stringify(msg));
  }

  // Fix 4: flush all queued ICE candidates now that remote description is set
  private async flushPendingCandidates(): Promise<void> {
    for (const c of this.pendingCandidates) {
      await this.pc.addIceCandidate(c);
    }
    this.pendingCandidates = [];
  }

  private async handleSignalingMessage(msg: SignalingMsg): Promise<void> {
    switch (msg.type) {
      case 'sdp': {
        const { sdp: description } = msg;
        if (description.type === 'offer') {
          await this.pc.setRemoteDescription(description);
          // Fix 4: mark remote description set and flush any queued candidates
          this.remoteDescriptionSet = true;
          await this.flushPendingCandidates();
          const answer = await this.pc.createAnswer();
          await this.pc.setLocalDescription(answer);
          const reply: SdpMsg = { type: 'sdp', room_id: this.roomId, sdp: answer };
          this.ws.send(JSON.stringify(reply));
        } else if (description.type === 'answer') {
          await this.pc.setRemoteDescription(description);
          // Fix 4: mark remote description set and flush any queued candidates
          this.remoteDescriptionSet = true;
          await this.flushPendingCandidates();
        }
        break;
      }
      case 'ice': {
        // Fix 4: queue ICE candidates until remote description is established
        if (this.remoteDescriptionSet) {
          await this.pc.addIceCandidate(msg.candidate);
        } else {
          this.pendingCandidates.push(msg.candidate);
        }
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
  // Fix 2: signalingUrl is required (was optional, defaulting to '' which
  // causes new WebSocket('') to throw SyntaxError)
  signalingUrl: string;
  // Fix 1: pass isOfferer through to RtcPeerImpl
  isOfferer?: boolean;
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
  // Fix 2: no default for signalingUrl — it is required in RtcComponentProps
  const { signalingUrl, wsFactory, pcFactory } = props;

  useImperativeHandle(ref, () => ({
    connect({ room_id, localStream }) {
      if (peerRef.current) {
        peerRef.current.close();
      }
      peerRef.current = new RtcPeerImpl({
        signalingUrl,
        room_id,
        localStream,
        // Fix 1: pass isOfferer through from props
        isOfferer: props.isOfferer,
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
