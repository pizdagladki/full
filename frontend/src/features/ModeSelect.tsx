import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { CvComponent, defaultCvRunner } from '../cv';
import type { CvCallbacks, CvHandleRef, LandmarkRunner } from '../cv';
import { writeLocal } from '../utils/storage';

// ---------------------------------------------------------------------------
// ModeSelect — the «Онлайн-батлы» screen. Home's battle window lands here; the
// player picks ranked/unranked and heads to /search. The camera preview, device
// picker, invisible auto-calibration and the win-clip track picker all live
// HERE now (moved from Home by the owner's design decision): the face gate
// belongs on the road into a match. KotH and invite-a-friend are reached
// directly from Home's mode windows, so they are no longer options here.
// ---------------------------------------------------------------------------

type GameMode = 'ranked' | 'unranked';

/** Calibration status shown under the camera preview — driven by the real CvEngine state
 * (face-present vs. still calibrating/no face), never a hard-coded string. */
type CalibrationStatus = 'calibrating' | 'ready';

// Mock TikTok tracks (no real audio yet — placeholder list, moved from Home)
const MOCK_TRACKS = [
  { id: 'track-1', name: 'Beat Drop #1' },
  { id: 'track-2', name: 'Viral Dance Mix' },
  { id: 'track-3', name: 'Chill Vibes' },
];

export interface ModeSelectProps {
  /** Injectable CV landmark runner (swap with a mock in tests). Defaults to the real
   * MediaPipe FaceLandmarker runner (`defaultCvRunner()`). */
  cvRunner?: LandmarkRunner;
}

export function ModeSelect({ cvRunner = defaultCvRunner() }: ModeSelectProps) {
  const navigate = useNavigate();

  // --- camera devices ---
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>('');

  // --- track selection (threaded to /search → Battle as the win-clip edit audio, #159) ---
  const [selectedTrackId, setSelectedTrackId] = useState<string>(MOCK_TRACKS[0].id);

  // --- camera preview + invisible auto-calibration ---
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const cvRef = useRef<CvHandleRef>(null);
  const [calibrationStatus, setCalibrationStatus] = useState<CalibrationStatus>('calibrating');

  // Intentionally stable ([]): CvComponent freezes these callbacks on first render.
  const onFacePresent = useCallback(() => {
    setCalibrationStatus('ready');
  }, []);
  const onFaceLost = useCallback(() => {
    setCalibrationStatus('calibrating');
  }, []);
  // No onBlink wired here — this screen only cares about face-present/calibration status.
  const cvCallbacks = useMemo<CvCallbacks>(
    () => ({ onFacePresent, onFaceLost }),
    // Intentionally stable ([]): passed to CvComponent, which builds its engine ONCE.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  // Enumerate video input devices on mount
  useEffect(() => {
    if (!navigator.mediaDevices?.enumerateDevices) return;
    let cancelled = false;
    navigator.mediaDevices
      .enumerateDevices()
      .then((all) => {
        if (cancelled) return;
        const videoInputs = all.filter((d) => d.kind === 'videoinput');
        setDevices(videoInputs);
        if (videoInputs.length > 0 && !selectedDeviceId) {
          setSelectedDeviceId(videoInputs[0].deviceId);
        }
      })
      .catch(() => {
        // Device enumeration failed — leave list empty, no crash
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Acquire camera stream whenever selected device changes
  useEffect(() => {
    if (!navigator.mediaDevices?.getUserMedia) return;
    let cancelled = false;
    // Captured up-front (not read from the ref again in the cleanup below).
    const cv = cvRef.current;

    // Stop any previous stream
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }

    // #172: persist the selection so game screens open the same device.
    if (selectedDeviceId) writeLocal('cameraDeviceId', selectedDeviceId);

    const constraints: MediaStreamConstraints = {
      video: selectedDeviceId ? { deviceId: { exact: selectedDeviceId } } : true,
    };

    navigator.mediaDevices
      .getUserMedia(constraints)
      .then((stream) => {
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop());
          return;
        }
        streamRef.current = stream;
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
        }
        // Preview stream is ready — (re)start the invisible auto-calibration CV engine on it.
        setCalibrationStatus('calibrating');
        if (videoRef.current) cv?.start(videoRef.current);
      })
      .catch(() => {
        // getUserMedia failed — show camera area without a live feed, no crash
      });

    return () => {
      cancelled = true;
      if (streamRef.current) {
        streamRef.current.getTracks().forEach((t) => t.stop());
        streamRef.current = null;
      }
      // Stop the CV engine on unmount AND whenever the selected camera changes.
      cv?.stop();
    };
  }, [selectedDeviceId]);

  const go = (mode: GameMode) => {
    navigate('/search', { state: { mode, trackId: selectedTrackId } });
  };

  return (
    <div className="panel-screen" data-testid="mode-select-screen">
      <h1 className="panel-title">Онлайн-батлы</h1>

      <div className="panel-card">
        <CvComponent ref={cvRef} runner={cvRunner} callbacks={cvCallbacks} />
        <video
          ref={videoRef}
          className="panel-video"
          autoPlay
          muted
          playsInline
          data-testid="camera-preview"
        />
        <div className="panel-status" data-testid="calibration-status" data-status={calibrationStatus}>
          {calibrationStatus === 'ready' ? 'Лицо в кадре — готов' : 'Калибровка…'}
        </div>

        <div className="panel-row">
          <label htmlFor="camera-select">Камера</label>
          <select
            id="camera-select"
            value={selectedDeviceId}
            onChange={(e) => setSelectedDeviceId(e.target.value)}
          >
            {devices.map((d) => (
              <option key={d.deviceId} value={d.deviceId}>
                {d.label || d.deviceId}
              </option>
            ))}
          </select>
        </div>

        <div className="panel-row">
          <label htmlFor="track-select">🎵 Трек</label>
          <select
            id="track-select"
            value={selectedTrackId}
            onChange={(e) => setSelectedTrackId(e.target.value)}
          >
            {MOCK_TRACKS.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </div>
      </div>

      <nav className="panel-actions" aria-label="Battle modes">
        <button type="button" className="btn-mode" data-testid="mode-ranked" onClick={() => go('ranked')}>
          Ранкед
        </button>
        <button
          type="button"
          className="btn-mode btn-mode--unranked"
          data-testid="mode-unranked"
          onClick={() => go('unranked')}
        >
          Не ранк
        </button>
      </nav>
    </div>
  );
}
