import type { NormalizedLandmark } from './types';

// MediaPipe FaceLandmarker landmark indices (6 per eye)
export const LEFT_EYE_INDICES: [number, number, number, number, number, number] = [362, 385, 387, 263, 373, 380];
export const RIGHT_EYE_INDICES: [number, number, number, number, number, number] = [33, 160, 158, 133, 153, 144];

function dist(a: NormalizedLandmark, b: NormalizedLandmark): number {
  return Math.sqrt((a.x - b.x) ** 2 + (a.y - b.y) ** 2);
}

/**
 * Compute Eye Aspect Ratio (EAR):
 *   EAR = (||p2-p6|| + ||p3-p5||) / (2 * ||p1-p4||)
 *
 * indices: [p1, p2, p3, p4, p5, p6] — corners and vertical pairs.
 * Returns 0 if any landmark is missing.
 */
export function computeEAR(
  landmarks: NormalizedLandmark[],
  indices: [number, number, number, number, number, number],
): number {
  const [i1, i2, i3, i4, i5, i6] = indices;
  const p1 = landmarks[i1];
  const p2 = landmarks[i2];
  const p3 = landmarks[i3];
  const p4 = landmarks[i4];
  const p5 = landmarks[i5];
  const p6 = landmarks[i6];
  if (!p1 || !p2 || !p3 || !p4 || !p5 || !p6) return 0;
  return (dist(p2, p6) + dist(p3, p5)) / (2 * dist(p1, p4));
}
