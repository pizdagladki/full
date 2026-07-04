// recording — canvas + MediaRecorder (WebM) + captureStream.
// Imperative; accessed via refs. Fully injectable for testing (mirrors rtc/index.ts).
//
// startRingBuffer() stays light: it only reference-holds the source. All heavy
// work — captureStream()/MediaRecorder/AudioContext — happens inside
// captureWin(), which is the only place encoding occurs.

import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  type ForwardedRef,
} from 'react';
import { EDIT_SLOT_DURATION_MS_DEFAULT, runEditSlot } from '../canvas';
import type { EditSlotOptions } from '../canvas';
import type { ClipsApi } from '../api/clips';

// ---------------------------------------------------------------------------
// Minimal interfaces so tests can inject mocks (never touch real browser
// media APIs), mirroring WsLike/PcLike in rtc/index.ts.
// ---------------------------------------------------------------------------

export interface MediaRecorderLike {
  start(timeslice?: number): void;
  stop(): void;
  readonly state: 'inactive' | 'recording' | 'paused';
  set ondataavailable(cb: ((ev: { data: Blob }) => void) | null);
  set onstop(cb: (() => void) | null);
}

export interface AudioNodeLike {
  connect(destination: AudioNodeLike | MediaStreamAudioDestinationNodeLike): void;
  start?(): void;
  stop?(): void;
}

export interface MediaStreamAudioDestinationNodeLike {
  stream: MediaStream;
}

export interface AudioContextLike {
  createOscillator(): AudioNodeLike;
  createMediaStreamDestination(): MediaStreamAudioDestinationNodeLike;
  close(): void;
}

export type MediaRecorderFactory = (
  stream: MediaStream,
  options?: MediaRecorderOptions,
) => MediaRecorderLike;

export type CaptureStreamFactory = (canvas: HTMLCanvasElement) => MediaStream;

export type AudioContextFactory = () => AudioContextLike;

/**
 * Builds a brand-new MediaStream from a list of tracks. Used to mix the engine's own
 * placeholder audio track onto a caller-supplied MediaStream source WITHOUT mutating the
 * caller's original stream object (see captureWin()'s MediaStream-source branch).
 */
export type MediaStreamFactory = (tracks: MediaStreamTrack[]) => MediaStream;

// ---------------------------------------------------------------------------
// Factories (defaults use real browser APIs; tests inject mocks)
// ---------------------------------------------------------------------------

const defaultCaptureStreamFactory: CaptureStreamFactory = (canvas) => {
  const withCapture = canvas as HTMLCanvasElement & {
    captureStream(frameRate?: number): MediaStream;
  };
  return withCapture.captureStream();
};

const defaultMediaRecorderFactory: MediaRecorderFactory = (stream, options) =>
  new MediaRecorder(stream, options) as unknown as MediaRecorderLike;

const defaultAudioContextFactory: AudioContextFactory = () =>
  new AudioContext() as unknown as AudioContextLike;

const defaultMediaStreamFactory: MediaStreamFactory = (tracks) => new MediaStream(tracks);

// ---------------------------------------------------------------------------
// RecordingEngineImpl — the core logic class (exported for direct unit testing)
// ---------------------------------------------------------------------------

export interface RecordingEngineOpts {
  captureStreamFactory?: CaptureStreamFactory;
  mediaRecorderFactory?: MediaRecorderFactory;
  audioContextFactory?: AudioContextFactory;
  /** Builds the combined stream when the source is a caller-supplied MediaStream (see captureWin()). */
  mediaStreamFactory?: MediaStreamFactory;
  /** Duration the edit slot (src/canvas) plays for while the win-part records, ms. */
  editSlotDurationMs?: number;
  /** Injectable edit-slot draw override, forwarded to `runEditSlot`. */
  editSlotDraw?: EditSlotOptions['draw'];
}

export class RecordingEngineImpl {
  private source: HTMLCanvasElement | MediaStream | null = null;
  private stream: MediaStream | null = null;
  // Tracks the engine itself created (and therefore owns) and must stop() on teardown. For a
  // canvas source this is every track of the captured stream (the engine owns that stream
  // outright); for a caller-supplied MediaStream source this is ONLY the engine's own placeholder
  // audio track(s) — the caller's original tracks are never stopped by this engine.
  private ownedTracks: MediaStreamTrack[] = [];
  private recorder: MediaRecorderLike | null = null;
  private audioCtx: AudioContextLike | null = null;

  private readonly captureStreamFactory: CaptureStreamFactory;
  private readonly mediaRecorderFactory: MediaRecorderFactory;
  private readonly audioContextFactory: AudioContextFactory;
  private readonly mediaStreamFactory: MediaStreamFactory;
  private readonly editSlotDurationMs: number;
  private readonly editSlotDraw: EditSlotOptions['draw'];

  constructor(opts: RecordingEngineOpts = {}) {
    this.captureStreamFactory = opts.captureStreamFactory ?? defaultCaptureStreamFactory;
    this.mediaRecorderFactory = opts.mediaRecorderFactory ?? defaultMediaRecorderFactory;
    this.audioContextFactory = opts.audioContextFactory ?? defaultAudioContextFactory;
    this.mediaStreamFactory = opts.mediaStreamFactory ?? defaultMediaStreamFactory;
    this.editSlotDurationMs = opts.editSlotDurationMs ?? EDIT_SLOT_DURATION_MS_DEFAULT;
    this.editSlotDraw = opts.editSlotDraw;
  }

  /**
   * Light: stores the source for later use. Does NOT call captureStream()/
   * MediaRecorder/AudioContext — the ring buffer must stay cheap while the
   * battle is running; all heavy encoding happens in captureWin().
   */
  startRingBuffer(sourceCanvasOrStream: HTMLCanvasElement | MediaStream): void {
    this.source = sourceCanvasOrStream;
  }

  /**
   * Heavy: obtains a MediaStream (via captureStream() for a canvas source, or
   * uses the MediaStream directly), mixes in a placeholder audio track via an
   * AudioContext, plays the edit-slot placeholder (src/canvas) for
   * `editSlotDurationMs` while recording, and resolves with the resulting
   * WebM Blob.
   */
  async captureWin(): Promise<Blob> {
    if (!this.source) {
      throw new Error('captureWin() called before startRingBuffer()');
    }

    const isCanvasSource =
      typeof HTMLCanvasElement !== 'undefined' && this.source instanceof HTMLCanvasElement;

    // Mix in a placeholder "TikTok track" audio source via AudioContext.
    const audioCtx = this.audioContextFactory();
    this.audioCtx = audioCtx;
    const destination = audioCtx.createMediaStreamDestination();
    const placeholderTrackSource = audioCtx.createOscillator();
    placeholderTrackSource.connect(destination);
    placeholderTrackSource.start?.();
    const audioTracks = destination.stream.getTracks();

    let recordedStream: MediaStream;
    if (isCanvasSource) {
      // captureStream() mints a stream the engine owns outright — safe to mutate in place and
      // to stop() every one of its tracks on teardown.
      const videoStream = this.captureStreamFactory(this.source as HTMLCanvasElement);
      for (const track of audioTracks) {
        videoStream.addTrack(track);
      }
      recordedStream = videoStream;
      this.ownedTracks = recordedStream.getTracks();
    } else {
      // The MediaStream source is the CALLER'S OWN stream object — it may be shared with, e.g., a
      // live WebRTC call. Never mutate it (no addTrack on it) and never let stop()/teardown stop
      // its tracks: build a brand-new stream referencing the caller's tracks (not owned by this
      // engine) plus the engine's own placeholder audio track(s) (owned).
      const callerStream = this.source as MediaStream;
      recordedStream = this.mediaStreamFactory([...callerStream.getTracks(), ...audioTracks]);
      this.ownedTracks = audioTracks;
    }
    this.stream = recordedStream;

    const chunks: Blob[] = [];
    const recorder = this.mediaRecorderFactory(this.stream);
    this.recorder = recorder;
    recorder.ondataavailable = (ev) => {
      if (ev.data.size > 0) chunks.push(ev.data);
    };

    const stopped = new Promise<void>((resolve) => {
      recorder.onstop = () => resolve();
    });

    recorder.start();

    if (isCanvasSource) {
      await runEditSlot(this.source as HTMLCanvasElement, {
        durationMs: this.editSlotDurationMs,
        draw: this.editSlotDraw,
      });
    } else {
      await new Promise<void>((resolve) => {
        setTimeout(resolve, this.editSlotDurationMs);
      });
    }

    if (recorder.state !== 'inactive') {
      recorder.stop();
    }
    await stopped;

    placeholderTrackSource.stop?.();
    // stop() may have already torn this engine down concurrently (e.g. the caller aborted
    // mid-capture) — guard so we don't double-close/null out a context stop() already replaced.
    if (this.audioCtx === audioCtx) {
      audioCtx.close();
      this.audioCtx = null;
    }
    if (this.recorder === recorder) {
      this.recorder = null;
    }

    return new Blob(chunks, { type: 'video/webm' });
  }

  /** Tears down any live recorder/stream/audio context. */
  stop(): void {
    if (this.recorder && this.recorder.state !== 'inactive') {
      this.recorder.stop();
    }
    this.recorder = null;
    if (this.stream) {
      // Only stop tracks the engine itself created/owns — for a MediaStream source that's just
      // the placeholder audio track(s); the caller's original tracks are never stopped here.
      this.ownedTracks.forEach((track) => track.stop());
      this.stream = null;
      this.ownedTracks = [];
    }
    if (this.audioCtx) {
      this.audioCtx.close();
      this.audioCtx = null;
    }
  }
}

// ---------------------------------------------------------------------------
// submitWinClip — orchestration helper; upload + convert is opt-in, never
// forced inside captureWin(). The caller decides win (submit) vs loss (skip).
// ---------------------------------------------------------------------------

export async function submitWinClip(
  blob: Blob,
  clipsApi: ClipsApi,
): Promise<{ id: string }> {
  const { id } = await clipsApi.uploadClip(blob);
  await clipsApi.convertClip(id);
  return { id };
}

// ---------------------------------------------------------------------------
// RecordingHandle — the imperative handle shape exposed via ref
// ---------------------------------------------------------------------------

export interface RecordingHandle {
  startRingBuffer(sourceCanvasOrStream: HTMLCanvasElement | MediaStream): void;
  captureWin(): Promise<Blob>;
  stop(): void;
}

// ---------------------------------------------------------------------------
// RecordingComponentProps — injectable deps for the React component
// ---------------------------------------------------------------------------

export interface RecordingComponentProps {
  captureStreamFactory?: CaptureStreamFactory;
  mediaRecorderFactory?: MediaRecorderFactory;
  audioContextFactory?: AudioContextFactory;
  mediaStreamFactory?: MediaStreamFactory;
  editSlotDurationMs?: number;
  editSlotDraw?: EditSlotOptions['draw'];
}

// ---------------------------------------------------------------------------
// RecordingComponent — null-rendering React component; exposes RecordingHandle
// via ref. Mirrors RtcComponent: heavy media work lives entirely in
// RecordingEngineImpl, outside React's render path.
// ---------------------------------------------------------------------------

export const RecordingComponent = forwardRef(function RecordingComponent(
  props: RecordingComponentProps,
  ref: ForwardedRef<RecordingHandle>,
) {
  const engineRef = useRef<RecordingEngineImpl | null>(null);
  const {
    captureStreamFactory,
    mediaRecorderFactory,
    audioContextFactory,
    mediaStreamFactory,
    editSlotDurationMs,
    editSlotDraw,
  } = props;

  if (!engineRef.current) {
    engineRef.current = new RecordingEngineImpl({
      captureStreamFactory,
      mediaRecorderFactory,
      audioContextFactory,
      mediaStreamFactory,
      editSlotDurationMs,
      editSlotDraw,
    });
  }

  useEffect(() => {
    return () => {
      engineRef.current?.stop();
    };
  }, []);

  useImperativeHandle(ref, () => ({
    startRingBuffer(sourceCanvasOrStream) {
      engineRef.current?.startRingBuffer(sourceCanvasOrStream);
    },
    captureWin() {
      if (!engineRef.current) {
        return Promise.reject(new Error('RecordingComponent not mounted'));
      }
      return engineRef.current.captureWin();
    },
    stop() {
      engineRef.current?.stop();
    },
  }));

  return null;
});
