import { useEffect, useState } from 'react';
import { useAuth } from './auth/AuthContext';
import { defaultRatingsApi } from '../api/ratings';
import type { RatingsApi, RatingData } from '../api/ratings';
import { defaultMatchHistoryApi } from '../api/matches';
import type { MatchHistoryApi, MatchEntry } from '../api/matches';
import { defaultClipsApi } from '../api/clips';
import type { ClipsApi, Clip } from '../api/clips';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MAX_LEVEL = 10;
const MAX_CLIPS = 10;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ProfileProps {
  ratingsApi?: RatingsApi;
  matchHistoryApi?: MatchHistoryApi;
  clipsApi?: ClipsApi;
  onReshare?: (clip: Clip) => void;
}

// ---------------------------------------------------------------------------
// Profile component
// ---------------------------------------------------------------------------

export function Profile({
  ratingsApi = defaultRatingsApi,
  matchHistoryApi = defaultMatchHistoryApi,
  clipsApi = defaultClipsApi,
  onReshare,
}: ProfileProps) {
  const { user } = useAuth();

  // --- stats section ---
  const [rating, setRating] = useState<RatingData | null>(null);
  const [statsLoading, setStatsLoading] = useState<boolean>(user != null);
  const [statsError, setStatsError] = useState<string | null>(null);

  // --- match history section ---
  const [matches, setMatches] = useState<MatchEntry[] | null>(null);
  const [historyLoading, setHistoryLoading] = useState<boolean>(true);
  const [historyError, setHistoryError] = useState<string | null>(null);

  // --- clips section ---
  const [clips, setClips] = useState<Clip[] | null>(null);
  const [clipsLoading, setClipsLoading] = useState<boolean>(true);
  const [clipsError, setClipsError] = useState<string | null>(null);

  // Fetch stats
  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    ratingsApi
      .getRating(user.id)
      .then((data) => {
        if (cancelled) return;
        setRating(data);
        setStatsLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load rating';
        setStatsError(msg);
        setStatsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [user, ratingsApi]);

  // Fetch match history
  useEffect(() => {
    let cancelled = false;
    matchHistoryApi
      .getMatchHistory()
      .then((data) => {
        if (cancelled) return;
        // Sort newest first by played_at
        const sorted = [...data].sort(
          (a, b) => new Date(b.played_at).getTime() - new Date(a.played_at).getTime(),
        );
        setMatches(sorted);
        setHistoryLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load match history';
        setHistoryError(msg);
        setHistoryLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [matchHistoryApi]);

  // Fetch clips
  useEffect(() => {
    let cancelled = false;
    clipsApi
      .getClips()
      .then((data) => {
        if (cancelled) return;
        // Defensive slice to max 10
        setClips(data.slice(0, MAX_CLIPS));
        setClipsLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load clips';
        setClipsError(msg);
        setClipsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [clipsApi]);

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  function handleReshare(clip: Clip) {
    if (onReshare) {
      onReshare(clip);
      return;
    }
    if (navigator.share) {
      void navigator.share({ url: clip.mp4_url ?? clipsApi.getClipDownloadUrl(clip.id) });
    }
  }

  // ---------------------------------------------------------------------------
  // Derived values
  // ---------------------------------------------------------------------------

  const levelPercent =
    rating != null ? Math.min(100, (rating.level / MAX_LEVEL) * 100) : 0;

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div data-testid="profile-screen">
      <h1>Profile</h1>

      {/* ------------------------------------------------------------------ */}
      {/* Stats section                                                        */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Stats">
        {statsLoading || (!user && !rating) ? (
          <div data-testid="stats-loading">Loading stats…</div>
        ) : statsError ? (
          <div data-testid="stats-error">Could not load stats</div>
        ) : rating ? (
          <div data-testid="stats-content">
            <div>ELO: {rating.elo}</div>
            <div>Level: {rating.level}</div>
            <div>Games played: {rating.games_played}</div>
            <div
              role="progressbar"
              aria-valuenow={levelPercent}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={`Level ${rating.level} progress`}
              style={{ width: `${levelPercent}%`, background: '#4caf50', height: '8px' }}
            />
          </div>
        ) : (
          <div data-testid="stats-loading">Loading stats…</div>
        )}
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Match history section                                               */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Match history">
        {historyLoading ? (
          <div data-testid="history-loading">Loading match history…</div>
        ) : historyError ? (
          <div data-testid="history-error">Could not load match history</div>
        ) : matches && matches.length === 0 ? (
          <div data-testid="history-empty">No matches yet</div>
        ) : (
          <ul>
            {(matches ?? []).map((match) => {
              const eloDeltaStr =
                match.elo_delta >= 0 ? `+${match.elo_delta}` : `${match.elo_delta}`;
              const durationSec = Math.round(match.duration_ms / 1000);
              return (
                <li key={match.match_id} data-testid="match-row">
                  <span data-testid="match-opponent">
                    {match.opponent_name ?? match.opponent_id}
                  </span>
                  <span data-testid="match-result">{match.result}</span>
                  <span data-testid="match-mode">{match.mode}</span>
                  <span data-testid="match-elo-delta">{eloDeltaStr}</span>
                  <span data-testid="match-duration">{durationSec}s</span>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Clips gallery section                                               */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Saved clips">
        {clipsLoading ? (
          <div data-testid="clips-loading">Loading clips…</div>
        ) : clipsError ? (
          <div data-testid="clips-error">Could not load clips</div>
        ) : clips && clips.length === 0 ? (
          <div data-testid="clips-empty">No clips yet</div>
        ) : (
          <ul>
            {(clips ?? []).map((clip) => (
              <li key={clip.id} data-testid="clip-item">
                <a
                  data-testid="clip-view"
                  href={clip.mp4_url ?? clipsApi.getClipDownloadUrl(clip.id)}
                  target="_blank"
                  rel="noreferrer"
                >
                  View
                </a>
                <a
                  data-testid="clip-download"
                  href={clipsApi.getClipDownloadUrl(clip.id)}
                  download
                >
                  Download
                </a>
                <button data-testid="clip-reshare" onClick={() => handleReshare(clip)}>
                  Share
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
