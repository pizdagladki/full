import { computeEAR, LEFT_EYE_INDICES, RIGHT_EYE_INDICES } from './ear';
import type { CvCallbacks, CvHandleRef, CvState, LandmarkRunner, NormalizedLandmark } from './types';

const CALIBRATION_FRAMES = 30;
const MIN_CALIBRATION_SAMPLES = 10;
const DEFAULT_THRESHOLD = 0.25;
const THRESHOLD_RATIO = 0.75;
const BLINK_FRAMES = 2;
const NO_FACE_WINDOW = 3;

/**
 * CvEngine drives the per-frame CV loop.
 *
 * Design: exposes `processFrame(timestamp)` — called internally by the RAF loop
 * AND directly in tests (no fake timers needed). RAF is used only for scheduling
 * in production; tests stub it and call processFrame manually.
 */
export class CvEngine implements CvHandleRef {
  private state: CvState = 'idle';
  private videoEl: HTMLVideoElement | null = null;
  private rafId: number | null = null;

  // Calibration
  private calibrationFrame = 0;
  private leftEarSamples: number[] = [];
  private rightEarSamples: number[] = [];
  private leftThreshold = DEFAULT_THRESHOLD;
  private rightThreshold = DEFAULT_THRESHOLD;

  // Blink detection (consecutive below-threshold frame counters)
  private leftBelow = 0;
  private rightBelow = 0;
  // One physical blink = one event: latched on fire, re-armed only after BOTH eyes reopen
  private blinkFired = false;

  // Face gating
  private facePresent = false;
  private noFaceCount = 0;

  constructor(
    private readonly runner: LandmarkRunner,
    private readonly callbacks: CvCallbacks = {},
  ) {}

  start(videoEl: HTMLVideoElement): void {
    if (this.state !== 'idle') this.stop();
    this.videoEl = videoEl;
    this.state = 'calibrating';
    this.calibrationFrame = 0;
    this.leftEarSamples = [];
    this.rightEarSamples = [];
    this.leftThreshold = DEFAULT_THRESHOLD;
    this.rightThreshold = DEFAULT_THRESHOLD;
    this.leftBelow = 0;
    this.rightBelow = 0;
    this.blinkFired = false;
    this.facePresent = false;
    this.noFaceCount = 0;
    this.scheduleFrame();
  }

  stop(): void {
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    this.state = 'idle';
    this.videoEl = null;
  }

  getState(): CvState {
    return this.state;
  }

  private scheduleFrame(): void {
    this.rafId = requestAnimationFrame((ts) => {
      this.processFrame(ts);
    });
  }

  /** Exposed for testing — processes exactly one frame synchronously. */
  processFrame(timestamp: number): void {
    // Capture state at the start; state can change during callbacks (e.g. stop()).
    const stateSnapshot = this.state;
    if (stateSnapshot === 'idle' || !this.videoEl) return;

    const result = this.runner.detectForVideo(this.videoEl, timestamp);
    const faces = result.faceLandmarks;

    if (faces.length === 0) {
      this.handleNoFace();
      // Re-read state: a callback inside handleNoFace could call stop()
      if ((this.state as CvState) !== 'idle') this.scheduleFrame();
      return;
    }

    // Face present
    if (!this.facePresent) {
      this.facePresent = true;
      this.noFaceCount = 0;
      this.callbacks.onFacePresent?.();
    } else {
      this.noFaceCount = 0;
    }

    const landmarks = faces[0] as NormalizedLandmark[];

    if (stateSnapshot === 'calibrating') {
      this.doCalibration(landmarks);
    } else {
      this.doDetection(landmarks);
    }

    // Re-read state: doCalibration may have changed it to 'running', or a callback could stop()
    if ((this.state as CvState) !== 'idle') this.scheduleFrame();
  }

  private handleNoFace(): void {
    this.noFaceCount++;
    if (this.noFaceCount === NO_FACE_WINDOW && this.facePresent) {
      this.facePresent = false;
      this.callbacks.onFaceLost?.();
    }
  }

  private doCalibration(landmarks: NormalizedLandmark[]): void {
    const leftEar = computeEAR(landmarks, LEFT_EYE_INDICES);
    const rightEar = computeEAR(landmarks, RIGHT_EYE_INDICES);

    if (leftEar > 0) this.leftEarSamples.push(leftEar);
    if (rightEar > 0) this.rightEarSamples.push(rightEar);

    this.calibrationFrame++;

    if (this.calibrationFrame >= CALIBRATION_FRAMES) {
      // Finalize calibration — compute mean * ratio as threshold
      if (this.leftEarSamples.length >= MIN_CALIBRATION_SAMPLES) {
        const mean = this.leftEarSamples.reduce((a, b) => a + b, 0) / this.leftEarSamples.length;
        this.leftThreshold = mean * THRESHOLD_RATIO;
      }
      if (this.rightEarSamples.length >= MIN_CALIBRATION_SAMPLES) {
        const mean = this.rightEarSamples.reduce((a, b) => a + b, 0) / this.rightEarSamples.length;
        this.rightThreshold = mean * THRESHOLD_RATIO;
      }
      // If not enough samples collected → keep DEFAULT_THRESHOLD (calibration fallback)
      this.state = 'running';
    }
  }

  private doDetection(landmarks: NormalizedLandmark[]): void {
    const leftEar = computeEAR(landmarks, LEFT_EYE_INDICES);
    const rightEar = computeEAR(landmarks, RIGHT_EYE_INDICES);

    // Advance per-eye counters
    if (leftEar < this.leftThreshold) {
      this.leftBelow++;
    } else {
      this.leftBelow = 0;
    }
    if (rightEar < this.rightThreshold) {
      this.rightBelow++;
    } else {
      this.rightBelow = 0;
    }

    // Re-arm only once BOTH eyes are back above threshold — a sustained closure
    // must not re-fire (one physical blink = one event).
    if (this.leftBelow === 0 && this.rightBelow === 0) {
      this.blinkFired = false;
    }

    // Fire onBlink once if EITHER eye has held below threshold for BLINK_FRAMES
    if (!this.blinkFired && (this.leftBelow >= BLINK_FRAMES || this.rightBelow >= BLINK_FRAMES)) {
      this.blinkFired = true;
      this.callbacks.onBlink?.();
    }
  }
}
