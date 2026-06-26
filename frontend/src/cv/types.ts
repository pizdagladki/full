export interface NormalizedLandmark {
  x: number;
  y: number;
  z: number;
}

export interface FaceLandmarkResult {
  faceLandmarks: NormalizedLandmark[][];
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
