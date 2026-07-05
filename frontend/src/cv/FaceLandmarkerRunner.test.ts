import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Classifications, FaceLandmarker, FaceLandmarkerResult, NormalizedLandmark as MpNormalizedLandmark } from '@mediapipe/tasks-vision';

// Mock the whole `@mediapipe/tasks-vision` module — no real model/WASM/network/camera is ever
// touched by this suite (criterion 6).
vi.mock('@mediapipe/tasks-vision', () => ({
  FaceLandmarker: { createFromOptions: vi.fn() },
  FilesetResolver: { forVisionTasks: vi.fn() },
}));

import { FaceLandmarker as MockedFaceLandmarker, FilesetResolver as MockedFilesetResolver } from '@mediapipe/tasks-vision';
import { createFaceLandmarkerRunner, FaceLandmarkerRunner, mapFaceConfidence } from './FaceLandmarkerRunner';
import * as cvIndex from './index';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function mpLandmark(x: number, y: number, z: number, visibility = 1): MpNormalizedLandmark {
  return { x, y, z, visibility };
}

/** A fake `FaceLandmarker` instance — only `detectForVideo` is ever called on it by our runner. */
function makeFakeMpLandmarker(result: FaceLandmarkerResult): FaceLandmarker {
  return { detectForVideo: vi.fn().mockReturnValue(result) } as unknown as FaceLandmarker;
}

function classifications(categories: Classifications['categories']): Classifications[] {
  return [{ categories, headIndex: 0, headName: 'blendshapes' }];
}

function category(categoryName: string, score: number): Classifications['categories'][number] {
  return { categoryName, score, index: 0, displayName: categoryName };
}

function makeFakeVideo(): HTMLVideoElement {
  return document.createElement('video');
}

beforeEach(() => {
  vi.mocked(MockedFaceLandmarker.createFromOptions).mockReset();
  vi.mocked(MockedFilesetResolver.forVisionTasks).mockReset();
});

// ---------------------------------------------------------------------------
// criterion 1 — detectForVideo maps FaceLandmarker output into FaceLandmarkResult
// ---------------------------------------------------------------------------
describe('FaceLandmarkerRunner.detectForVideo — mapping (criterion 1)', () => {
  it('criterion 1: maps faceLandmarks (x,y,z, dropping visibility) and faceConfidences from a mocked result', () => {
    const mpResult: FaceLandmarkerResult = {
      faceLandmarks: [[mpLandmark(0.1, 0.2, 0.3, 0.9), mpLandmark(0.4, 0.5, 0.6, 0.8)]],
      faceBlendshapes: classifications([category('_neutral', 0.87)]),
      facialTransformationMatrixes: [],
    };
    const runner = new FaceLandmarkerRunner();
    runner.setLandmarker(makeFakeMpLandmarker(mpResult));

    const video = makeFakeVideo();
    const result = runner.detectForVideo(video, 1000);

    expect(result.faceLandmarks).toEqual([
      [
        { x: 0.1, y: 0.2, z: 0.3 },
        { x: 0.4, y: 0.5, z: 0.6 },
      ],
    ]);
    expect(result.faceConfidences).toEqual([0.87]);
  });

  it('fails-on-violation: dropping the confidence mapping would leave faceConfidences unset for a detected face', () => {
    // If criterion 1/3's mapping were removed, a genuinely-detected face would report NO
    // confidence at all — CvEngine's confidence gate (which treats "absent" as "confident") would
    // then never distinguish a real low-confidence frame. Asserting the field IS populated here
    // pins the mapping in place.
    const mpResult: FaceLandmarkerResult = {
      faceLandmarks: [[mpLandmark(0, 0, 0)]],
      faceBlendshapes: classifications([category('_neutral', 0.42)]),
      facialTransformationMatrixes: [],
    };
    const runner = new FaceLandmarkerRunner();
    runner.setLandmarker(makeFakeMpLandmarker(mpResult));

    const result = runner.detectForVideo(makeFakeVideo(), 0);
    expect(result.faceConfidences).toBeDefined();
    expect(result.faceConfidences?.[0]).toBe(0.42);
  });

  it('passes the video element and timestamp straight through to FaceLandmarker.detectForVideo (VIDEO mode)', () => {
    const mpResult: FaceLandmarkerResult = { faceLandmarks: [], faceBlendshapes: [], facialTransformationMatrixes: [] };
    const landmarker = makeFakeMpLandmarker(mpResult);
    const runner = new FaceLandmarkerRunner();
    runner.setLandmarker(landmarker);

    const video = makeFakeVideo();
    runner.detectForVideo(video, 1234);

    expect(landmarker.detectForVideo).toHaveBeenCalledWith(video, 1234);
  });
});

// ---------------------------------------------------------------------------
// criterion 2 — never fabricate a face: not-ready and no-face guards
// ---------------------------------------------------------------------------
describe('FaceLandmarkerRunner.detectForVideo — never fabricates a face (criterion 2)', () => {
  it('criterion 2: a runner with no model loaded yet returns { faceLandmarks: [] }', () => {
    const runner = new FaceLandmarkerRunner();
    const result = runner.detectForVideo(makeFakeVideo(), 0);
    expect(result).toEqual({ faceLandmarks: [] });
  });

  it('criterion 2: a ready runner with an empty-face MediaPipe result returns { faceLandmarks: [] }', () => {
    const mpResult: FaceLandmarkerResult = { faceLandmarks: [], faceBlendshapes: [], facialTransformationMatrixes: [] };
    const runner = new FaceLandmarkerRunner();
    runner.setLandmarker(makeFakeMpLandmarker(mpResult));

    const result = runner.detectForVideo(makeFakeVideo(), 0);
    expect(result).toEqual({ faceLandmarks: [] });
  });

  it('fails-on-violation: a not-ready runner must not report a fabricated confidence either', () => {
    // A buggy implementation might default faceConfidences to some fixed value even when there's
    // no model — that would violate "never fabricate a face". Assert the whole shape, not just
    // faceLandmarks, so a stray faceConfidences: [1] would fail this test.
    const runner = new FaceLandmarkerRunner();
    const result = runner.detectForVideo(makeFakeVideo(), 0);
    expect(result.faceConfidences).toBeUndefined();
    expect(Object.keys(result)).toEqual(['faceLandmarks']);
  });

  it('criterion 2: once the model is assigned via setLandmarker, subsequent frames are mapped normally', () => {
    const runner = new FaceLandmarkerRunner();
    expect(runner.detectForVideo(makeFakeVideo(), 0)).toEqual({ faceLandmarks: [] });

    const mpResult: FaceLandmarkerResult = {
      faceLandmarks: [[mpLandmark(0.5, 0.5, 0)]],
      faceBlendshapes: [],
      facialTransformationMatrixes: [],
    };
    runner.setLandmarker(makeFakeMpLandmarker(mpResult));

    const result = runner.detectForVideo(makeFakeVideo(), 33);
    expect(result.faceLandmarks).toEqual([[{ x: 0.5, y: 0.5, z: 0 }]]);
  });
});

// ---------------------------------------------------------------------------
// criterion 3 — confidence mapping helper
// ---------------------------------------------------------------------------
describe('mapFaceConfidence (criterion 3)', () => {
  const cases: Array<{ name: string; blendshapes: Classifications[] | undefined; expected: number }> = [
    {
      name: 'criterion 3: prefers the _neutral category score when present',
      blendshapes: classifications([category('mouthSmile', 0.2), category('_neutral', 0.73), category('browUp', 0.05)]),
      expected: 0.73,
    },
    {
      name: 'criterion 3: falls back to the max category score when _neutral is absent',
      blendshapes: classifications([category('mouthSmile', 0.2), category('browUp', 0.65)]),
      expected: 0.65,
    },
    {
      name: 'criterion 3: falls back to full confidence (1) when categories are empty',
      blendshapes: classifications([]),
      expected: 1,
    },
    {
      name: 'criterion 3: falls back to full confidence (1) when blendshapes were not requested/returned',
      blendshapes: undefined,
      expected: 1,
    },
    {
      name: 'criterion 3: falls back to full confidence (1) when faceBlendshapes array itself is empty',
      blendshapes: [],
      expected: 1,
    },
  ];

  it.each(cases)('$name', ({ blendshapes, expected }) => {
    expect(mapFaceConfidence(blendshapes)).toBe(expected);
  });

  it('fails-on-violation: swapping max-score fallback for the first category would misreport confidence', () => {
    // categories are NOT always sorted (mocked out of order here) — picking "the first" instead
    // of the max would silently under/over-report confidence. Pin the max-of-all-categories rule.
    const blendshapes = classifications([category('a', 0.1), category('b', 0.9), category('c', 0.5)]);
    expect(mapFaceConfidence(blendshapes)).toBe(0.9);
  });
});

// ---------------------------------------------------------------------------
// criterion 4 — local static assets, no CDN
// ---------------------------------------------------------------------------
describe('createFaceLandmarkerRunner — local assets only (criterion 4)', () => {
  it('criterion 4: defaults to local origin-relative wasm and model paths (no CDN URL)', async () => {
    const fakeFileset = { wasmLoaderPath: '/mediapipe/wasm/x.js', wasmBinaryPath: '/mediapipe/wasm/x.wasm' };
    vi.mocked(MockedFilesetResolver.forVisionTasks).mockResolvedValue(fakeFileset);
    vi.mocked(MockedFaceLandmarker.createFromOptions).mockResolvedValue(makeFakeMpLandmarker({
      faceLandmarks: [],
      faceBlendshapes: [],
      facialTransformationMatrixes: [],
    }));

    await createFaceLandmarkerRunner();

    expect(MockedFilesetResolver.forVisionTasks).toHaveBeenCalledWith('/mediapipe/wasm');
    const [, options] = vi.mocked(MockedFaceLandmarker.createFromOptions).mock.calls[0];
    expect(options.baseOptions?.modelAssetPath).toBe('/models/face_landmarker.task');
    expect(options.runningMode).toBe('VIDEO');
    // Assert neither asset path references an external CDN host.
    expect(String(options.baseOptions?.modelAssetPath)).not.toMatch(/^https?:\/\//);
  });

  it('criterion 4: honors injected wasmBasePath/modelAssetPath overrides (so tests never hit real assets)', async () => {
    const fakeFileset = { wasmLoaderPath: '/custom/wasm/x.js', wasmBinaryPath: '/custom/wasm/x.wasm' };
    vi.mocked(MockedFilesetResolver.forVisionTasks).mockResolvedValue(fakeFileset);
    vi.mocked(MockedFaceLandmarker.createFromOptions).mockResolvedValue(makeFakeMpLandmarker({
      faceLandmarks: [],
      faceBlendshapes: [],
      facialTransformationMatrixes: [],
    }));

    await createFaceLandmarkerRunner({ wasmBasePath: '/custom/wasm', modelAssetPath: '/custom/model.task' });

    expect(MockedFilesetResolver.forVisionTasks).toHaveBeenCalledWith('/custom/wasm');
    const [, options] = vi.mocked(MockedFaceLandmarker.createFromOptions).mock.calls[0];
    expect(options.baseOptions?.modelAssetPath).toBe('/custom/model.task');
  });

  it('criterion 2: the factory awaits the full load, and the returned runner then maps real detections', async () => {
    const mpResult: FaceLandmarkerResult = {
      faceLandmarks: [[mpLandmark(0.2, 0.3, 0.1)]],
      faceBlendshapes: classifications([category('_neutral', 0.6)]),
      facialTransformationMatrixes: [],
    };
    vi.mocked(MockedFilesetResolver.forVisionTasks).mockResolvedValue({
      wasmLoaderPath: '/mediapipe/wasm/x.js',
      wasmBinaryPath: '/mediapipe/wasm/x.wasm',
    });
    vi.mocked(MockedFaceLandmarker.createFromOptions).mockResolvedValue(makeFakeMpLandmarker(mpResult));

    const runner = await createFaceLandmarkerRunner();
    const result = runner.detectForVideo(makeFakeVideo(), 500);

    expect(result.faceLandmarks).toEqual([[{ x: 0.2, y: 0.3, z: 0.1 }]]);
    expect(result.faceConfidences).toEqual([0.6]);
  });
});

// ---------------------------------------------------------------------------
// criterion 5 — exported from src/cv/index.ts
// ---------------------------------------------------------------------------
describe('src/cv/index.ts exports (criterion 5)', () => {
  it('criterion 5: FaceLandmarkerRunner and createFaceLandmarkerRunner are exported from the cv barrel', () => {
    expect(cvIndex.FaceLandmarkerRunner).toBe(FaceLandmarkerRunner);
    expect(cvIndex.createFaceLandmarkerRunner).toBe(createFaceLandmarkerRunner);
  });

  it('fails-on-violation: the exported runner must implement the LandmarkRunner contract', () => {
    const runner = new cvIndex.FaceLandmarkerRunner();
    expect(typeof runner.detectForVideo).toBe('function');
    expect(runner.detectForVideo(makeFakeVideo(), 0)).toEqual({ faceLandmarks: [] });
  });
});
