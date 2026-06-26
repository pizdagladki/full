import { forwardRef, useImperativeHandle, useRef } from 'react';
import { CvEngine } from './CvEngine';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from './types';

interface CvComponentProps {
  runner: LandmarkRunner;
  callbacks?: CvCallbacks;
}

/**
 * CvComponent — thin React wrapper around CvEngine.
 *
 * Exposes the imperative CvHandleRef API via forwardRef + useImperativeHandle.
 * The per-frame RAF loop runs entirely outside React render (inside CvEngine).
 * Renders null — no DOM output.
 */
export const CvComponent = forwardRef<CvHandleRef, CvComponentProps>(({ runner, callbacks = {} }, ref) => {
  const engineRef = useRef<CvEngine | null>(null);
  if (!engineRef.current) {
    engineRef.current = new CvEngine(runner, callbacks);
  }

  useImperativeHandle(ref, () => ({
    start: (videoEl: HTMLVideoElement) => engineRef.current!.start(videoEl),
    stop: () => engineRef.current!.stop(),
    getState: () => engineRef.current!.getState(),
  }));

  return null;
});

CvComponent.displayName = 'CvComponent';
