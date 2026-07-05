import {
  FaceLandmarker,
  FilesetResolver,
  type Classifications,
  type NormalizedLandmark as MpNormalizedLandmark,
} from '@mediapipe/tasks-vision';
import type { FaceLandmarkResult, LandmarkRunner, NormalizedLandmark } from './types';

// Local, CDN-free static assets — see the `mediapipeWasmPlugin` in vite.config.ts (dev middleware
// + build-time copy of `@mediapipe/tasks-vision`'s wasm/ into `dist/mediapipe/wasm`) and the
// committed model at `public/models/face_landmarker.task`. Never point these at a CDN URL.
const DEFAULT_WASM_BASE_PATH = '/mediapipe/wasm';
const DEFAULT_MODEL_ASSET_PATH = '/models/face_landmarker.task';

export interface CreateFaceLandmarkerRunnerOptions {
  wasmBasePath?: string;
  modelAssetPath?: string;
}

/** Maps MediaPipe's landmark shape (x, y, z, visibility) onto our narrower domain shape. */
function mapFaceLandmarks(faceLandmarks: MpNormalizedLandmark[][]): NormalizedLandmark[][] {
  return faceLandmarks.map((face) => face.map(({ x, y, z }) => ({ x, y, z })));
}

/**
 * Derives a real, per-frame face-confidence proxy from a `FaceLandmarker` result.
 *
 * `@mediapipe/tasks-vision`'s `FaceLandmarkerResult` does NOT surface a raw face-detection score
 * (that only exists as an INPUT threshold — `minFaceDetectionConfidence` — not an output field).
 * The best available real per-frame signal is the blendshape classification scores: when present,
 * we prefer the `_neutral` category's score (a stable, always-present baseline expression score),
 * falling back to the highest-scoring category. When blendshapes weren't requested/returned at
 * all, a face WAS still detected (this is only called once `faceLandmarks.length > 0`), so we
 * report full confidence for it rather than fabricate a lower number.
 */
export function mapFaceConfidence(faceBlendshapes: Classifications[] | undefined): number {
  const categories = faceBlendshapes?.[0]?.categories;
  if (!categories || categories.length === 0) return 1;
  const neutral = categories.find((category) => category.categoryName === '_neutral');
  if (neutral) return neutral.score;
  return categories.reduce((max, category) => Math.max(max, category.score), categories[0].score);
}

/**
 * Production `LandmarkRunner` backed by MediaPipe's `FaceLandmarker`.
 *
 * `detectForVideo` stays SYNCHRONOUS to match `LandmarkRunner` (mirrors `FaceLandmarker`'s own
 * synchronous `detectForVideo`, which is only valid in `VIDEO` running mode). Only the MODEL
 * loading is asynchronous — see `createFaceLandmarkerRunner`. Until a model is assigned (loading,
 * or never loaded), or when no face is found in a frame, `detectForVideo` returns
 * `{ faceLandmarks: [] }` — it NEVER fabricates a face.
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
      faceConfidences: [mapFaceConfidence(result.faceBlendshapes)],
    };
  }
}

/**
 * Asynchronously loads the FaceLandmarker WASM runtime + model and returns a ready
 * `FaceLandmarkerRunner`. Assets are resolved from LOCAL static paths only (never a CDN) — see
 * `DEFAULT_WASM_BASE_PATH` / `DEFAULT_MODEL_ASSET_PATH` and `vite.config.ts`.
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
    outputFaceBlendshapes: true,
  });
  runner.setLandmarker(landmarker);
  return runner;
}
