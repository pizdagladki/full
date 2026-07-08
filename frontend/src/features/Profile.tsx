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
    <div className="panel-screen" data-testid="profile-screen">
      <h1 className="panel-title">Профиль</h1>

      {/* ------------------------------------------------------------------ */}
      {/* Stats section                                                        */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Stats">
        {statsLoading || (!user && !rating) ? (
          <div className="results-note" data-testid="stats-loading">Загружаем статистику…</div>
        ) : statsError ? (
          <div className="results-note" data-testid="stats-error">Не удалось загрузить статистику</div>
        ) : rating ? (
          <div className="profile-stats" data-testid="stats-content">
            <div className="results-chips">
              <div className="results-chip">ELO: {rating.elo}</div>
              <div className="results-chip">Уровень: {rating.level}</div>
              <div className="results-chip">Матчей: {rating.games_played}</div>
            </div>
            <div className="results-levelbar">
              <div
                role="progressbar"
                aria-valuenow={levelPercent}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={`Level ${rating.level} progress`}
                style={{ width: `${levelPercent}%` }}
              />
            </div>
          </div>
        ) : (
          <div className="results-note" data-testid="stats-loading">Загружаем статистику…</div>
        )}
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Match history section                                               */}
      {/* ------------------------------------------------------------------ */}
      <section className="sheet" aria-label="Match history">
        <h2 className="sheet-title">История матчей</h2>
        {historyLoading ? (
          <div className="results-note" data-testid="history-loading">Загружаем историю…</div>
        ) : historyError ? (
          <div className="results-note" data-testid="history-error">Не удалось загрузить историю</div>
        ) : matches && matches.length === 0 ? (
          <div className="results-note" data-testid="history-empty">Матчей пока нет</div>
        ) : (
          <ul className="sheet-list">
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
      <section className="sheet" aria-label="Saved clips">
        <h2 className="sheet-title">Клипы</h2>
        {clipsLoading ? (
          <div className="results-note" data-testid="clips-loading">Загружаем клипы…</div>
        ) : clipsError ? (
          <div className="results-note" data-testid="clips-error">Не удалось загрузить клипы</div>
        ) : clips && clips.length === 0 ? (
          <div className="results-note" data-testid="clips-empty">Клипов пока нет</div>
        ) : (
          <ul className="sheet-list">
            {(clips ?? []).map((clip) => (
              <li key={clip.id} data-testid="clip-item">
                <a
                  data-testid="clip-view"
                  href={clip.mp4_url ?? clipsApi.getClipDownloadUrl(clip.id)}
                  target="_blank"
                  rel="noreferrer"
                >
                  Смотреть
                </a>
                <a
                  data-testid="clip-download"
                  href={clipsApi.getClipDownloadUrl(clip.id)}
                  download
                >
                  Скачать
                </a>
                <button className="results-report-btn" data-testid="clip-reshare" onClick={() => handleReshare(clip)}>
                  Шеринг
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
