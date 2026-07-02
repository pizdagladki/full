export interface NormalizedLandmark {
  x: number;
  y: number;
  z: number;
}

export interface FaceLandmarkResult {
  faceLandmarks: NormalizedLandmark[][];
  /**
   * Optional per-face detection confidence, parallel to `faceLandmarks` (index [0] is the
   * primary/first face). OPTIONAL for backward compatibility: an absent value means "confident" —
   * every existing result/helper that omits it keeps compiling and behaving exactly as before.
   * Real runners (FaceLandmarker) populate this from the detection score.
   */
  faceConfidences?: number[];
}

// Injectable runner — production wraps real FaceLandmarker; tests provide mocks
export interface LandmarkRunner {
  detectForVideo(videoElement: HTMLVideoElement, timestamp: number): FaceLandmarkResult;
}

export type CvState = 'idle' | 'calibrating' | 'running';

export interface CvCallbacks {
  onBlink?: () => void;
  onFaceLost?: () => void;
  onFacePresent?: () => void;
}

export interface CvHandleRef {
  start(videoEl: HTMLVideoElement): void;
  stop(): void;
  getState(): CvState;
}
