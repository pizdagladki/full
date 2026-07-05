import { FaceLandmarker, FilesetResolver, type NormalizedLandmark as MpNormalizedLandmark } from '@mediapipe/tasks-vision';
import type { FaceLandmarkResult, LandmarkRunner, NormalizedLandmark } from './types';

// Local, CDN-free static assets — see the `mediapipeWasmPlugin` in vite.config.ts (dev middleware
// + build-time copy of `@mediapipe/tasks-vision`'s wasm/ into `dist/mediapipe/wasm`) and the
// committed model at `public/models/face_landmarker.task`. Never point these at a CDN URL.
const DEFAULT_WASM_BASE_PATH = '/mediapipe/wasm';
const DEFAULT_MODEL_ASSET_PATH = '/models/face_landmarker.task';
// The real confidence gate: a returned face has already survived these thresholds inside the
// model itself, so a low-confidence/uncertain-tracking frame simply yields NO face
// (`faceLandmarks: []`) rather than a face we'd then have to score ourselves.
const DETECTION_CONFIDENCE_THRESHOLD = 0.5;

export interface CreateFaceLandmarkerRunnerOptions {
  wasmBasePath?: string;
  modelAssetPath?: string;
}

/** Maps MediaPipe's landmark shape (x, y, z, visibility) onto our narrower domain shape. */
function mapFaceLandmarks(faceLandmarks: MpNormalizedLandmark[][]): NormalizedLandmark[][] {
  return faceLandmarks.map((face) => face.map(({ x, y, z }) => ({ x, y, z })));
}

/**
 * Production `LandmarkRunner` backed by MediaPipe's `FaceLandmarker`.
 *
 * `detectForVideo` stays SYNCHRONOUS to match `LandmarkRunner` (mirrors `FaceLandmarker`'s own
 * synchronous `detectForVideo`, which is only valid in `VIDEO` running mode). Only the MODEL
 * loading is asynchronous — see `createFaceLandmarkerRunner`. Until a model is assigned (loading,
 * or never loaded), or when no face is found in a frame, `detectForVideo` returns
 * `{ faceLandmarks: [] }` — it NEVER fabricates a face.
 *
 * Confidence: `@mediapipe/tasks-vision`'s `FaceLandmarkerResult` exposes NO scalar
 * detector/landmark-confidence field, so real confidence gating is delegated to the model's own
 * `minFaceDetectionConfidence` / `minFacePresenceConfidence` / `minTrackingConfidence` options
 * (set in `createFaceLandmarkerRunner`) — a low-confidence frame never comes back as a face at
 * all. A returned face has therefore already passed the real gate, so it is reported at full
 * confidence (`1`) rather than fabricated from an unrelated signal (do NOT use blendshapes /
 * expression weights / `visibility` here — those measure expression, not detection confidence,
 * and would incorrectly suppress genuine expressive frames like blinks).
 */
export class FaceLandmarkerRunner implements LandmarkRunner {
  private landmarker: FaceLandmarker | null = null;

  /** Assigns the loaded model. Called by `createFaceLandmarkerRunner` once it's ready. */
  setLandmarker(landmarker: FaceLandmarker): void {
    this.landmarker = landmarker;
  }

  detectForVideo(videoElement: HTMLVideoElement, timestamp: number): FaceLandmarkResult {
    if (!this.landmarker) return { faceLandmarks: [] };

    const result = this.landmarker.detectForVideo(videoElement, timestamp);
    if (result.faceLandmarks.length === 0) return { faceLandmarks: [] };

    return {
      faceLandmarks: mapFaceLandmarks(result.faceLandmarks),
      faceConfidences: [1],
    };
  }
}

/**
 * Asynchronously loads the FaceLandmarker WASM runtime + model and returns a ready
 * `FaceLandmarkerRunner`. Assets are resolved from LOCAL static paths only (never a CDN) — see
 * `DEFAULT_WASM_BASE_PATH` / `DEFAULT_MODEL_ASSET_PATH` and `vite.config.ts`. Detection
 * confidence thresholds are set explicitly so the model itself performs the real confidence
 * gating (see the class doc comment above).
 */
export async function createFaceLandmarkerRunner(
  options: CreateFaceLandmarkerRunnerOptions = {},
): Promise<FaceLandmarkerRunner> {
  const { wasmBasePath = DEFAULT_WASM_BASE_PATH, modelAssetPath = DEFAULT_MODEL_ASSET_PATH } = options;

  const runner = new FaceLandmarkerRunner();
  const fileset = await FilesetResolver.forVisionTasks(wasmBasePath);
  const landmarker = await FaceLandmarker.createFromOptions(fileset, {
    baseOptions: { modelAssetPath },
    runningMode: 'VIDEO',
    numFaces: 1,
    minFaceDetectionConfidence: DETECTION_CONFIDENCE_THRESHOLD,
    minFacePresenceConfidence: DETECTION_CONFIDENCE_THRESHOLD,
    minTrackingConfidence: DETECTION_CONFIDENCE_THRESHOLD,
  });
  runner.setLandmarker(landmarker);
  return runner;
}
