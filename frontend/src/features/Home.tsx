import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from './auth/AuthContext';
import { defaultRatingsApi } from '../api/ratings';
import type { RatingsApi, RatingData } from '../api/ratings';
import { PointsWidget } from './PointsWidget';
import type { PointsApi } from '../api/points';

// ---------------------------------------------------------------------------
// Mock TikTok tracks (no real audio yet — placeholder list)
// ---------------------------------------------------------------------------

const MOCK_TRACKS = [
  { id: 'track-1', name: 'Beat Drop #1' },
  { id: 'track-2', name: 'Viral Dance Mix' },
  { id: 'track-3', name: 'Chill Vibes' },
];

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface HomeProps {
  /** Injectable ratings API (swap with a mock in tests). Defaults to the real client. */
  ratingsApi?: RatingsApi;
  /** Injectable points API (swap with a mock in tests). Defaults to the real client. */
  pointsApi?: PointsApi;
}

// ---------------------------------------------------------------------------
// Home component
// ---------------------------------------------------------------------------

export function Home({ ratingsApi = defaultRatingsApi, pointsApi }: HomeProps) {
  const { user } = useAuth();

  // --- camera devices ---
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>('');

  // --- track selection ---
  const [selectedTrackId, setSelectedTrackId] = useState<string>(MOCK_TRACKS[0].id);

  // --- rating / level ---
  const [rating, setRating] = useState<RatingData | null>(null);
  // If there's no user we'll never fetch, so start as not-loading.
  const [ratingLoading, setRatingLoading] = useState<boolean>(user != null);
  const [ratingError, setRatingError] = useState<string | null>(null);

  // --- camera preview ---
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);

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

  // Fetch rating for the current user
  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    ratingsApi
      .getRating(user.id)
      .then((data) => {
        if (cancelled) return;
        setRating(data);
        setRatingLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load rating';
        setRatingError(msg);
        setRatingLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [user, ratingsApi]);

  // Acquire camera stream whenever selected device changes
  useEffect(() => {
    if (!navigator.mediaDevices?.getUserMedia) return;
    let cancelled = false;

    // Stop any previous stream
    if (streamRef.current) {
      streamRef.current.getTracks().forEach((t) => t.stop());
      streamRef.current = null;
    }

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
    };
  }, [selectedDeviceId]);

  // ---------------------------------------------------------------------------
  // Level progress bar helpers
  // ---------------------------------------------------------------------------

  const MAX_LEVEL = 10;
  const levelPercent =
    rating != null ? Math.min(100, (rating.level / MAX_LEVEL) * 100) : 0;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div data-testid="home-screen">
      {/* Points widget (balance + info panel) — top-right */}
      <PointsWidget pointsApi={pointsApi} />

      {/* Top AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />

      {/* Navigation */}
      <nav>
        <Link to="/store">Store</Link>
        <Link to="/profile">Profile</Link>
        <Link to="/mode-select">Play</Link>
      </nav>

      {/* Level progress bar */}
      <section aria-label="Level progress">
        {ratingLoading || ratingError || rating === null ? (
          <div data-testid="level-placeholder" aria-label="Level loading">
            {ratingError ? 'Could not load level' : 'Loading level…'}
          </div>
        ) : (
          <div>
            <div>
              Level {rating.level} &mdash; ELO {rating.elo}
            </div>
            <div
              role="progressbar"
              aria-valuenow={levelPercent}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={`Level ${rating.level} progress`}
              style={{ width: `${levelPercent}%`, background: '#4caf50', height: '8px' }}
            />
          </div>
        )}
      </section>

      {/* Camera device selector */}
      <section aria-label="Camera selection">
        <label htmlFor="camera-select">Camera</label>
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
      </section>

      {/* TikTok-style track selector */}
      <section aria-label="Track selection">
        <label htmlFor="track-select">Track</label>
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
      </section>

      {/* Camera preview */}
      <section aria-label="Camera preview">
        <video
          ref={videoRef}
          autoPlay
          muted
          playsInline
          data-testid="camera-preview"
          style={{ width: '100%', maxWidth: '480px', background: '#000' }}
        />
        <div data-testid="calibration-status">Calibrating…</div>
      </section>

      {/* Bottom AdSense banner slot */}
      <div data-testid="ad-slot" aria-hidden="true" />
    </div>
  );
}
