import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from './auth';
import { defaultRatingsApi } from '../api/ratings';
import type { RatingsApi, RatingData } from '../api/ratings';
import { defaultReportsApi } from '../api/reports';
import type { ReportsApi } from '../api/reports';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const MAX_LEVEL = 10;

// ---------------------------------------------------------------------------
// location.state — handed off by Battle.tsx's `navigate('/results', { state })`.
// Battle only ever sets result/durationMs/winnerId/loserId/ranked today; matchId,
// a reported opponent id, an ELO delta, and a clip url are all OPTIONAL and may
// be wired in by later work (recording engine, matchmaking) — this screen must
// render sensible neutral placeholders when any of them (or the whole state,
// e.g. a page refresh) are absent, and never crash.
// ---------------------------------------------------------------------------

interface ResultsLocationState {
  result?: 'win' | 'loss';
  durationMs?: number;
  winnerId?: number;
  loserId?: number;
  ranked?: boolean;
  matchId?: string;
  reportedId?: number;
  opponentId?: number;
  eloDelta?: number;
  clipUrl?: string;
  mp4Url?: string;
}

type ReportStatus = 'idle' | 'pending' | 'success' | 'error';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ResultsProps {
  ratingsApi?: RatingsApi;
  reportsApi?: ReportsApi;
  /** Injectable share handler (swap with a mock in tests). Defaults to `navigator.share`. */
  onShare?: (url: string) => void;
  /** Rematch only signals intent — no mutual-agreement mechanic is implemented here. */
  onRematch?: () => void;
}

// ---------------------------------------------------------------------------
// Results component
// ---------------------------------------------------------------------------

export function Results({
  ratingsApi = defaultRatingsApi,
  reportsApi = defaultReportsApi,
  onShare,
  onRematch,
}: ResultsProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  const state = (location.state as ResultsLocationState | null) ?? null;

  // --- rating (level progress bar) ---
  const [rating, setRating] = useState<RatingData | null>(null);
  const [statsLoading, setStatsLoading] = useState<boolean>(user != null);
  const [statsError, setStatsError] = useState<string | null>(null);

  // --- rematch (intent-only signal) ---
  const [rematchRequested, setRematchRequested] = useState(false);

  // --- report statuses ---
  const [cheatStatus, setCheatStatus] = useState<ReportStatus>('idle');
  const [bugStatus, setBugStatus] = useState<ReportStatus>('idle');

  // Fetch rating (criterion 1 — level progress bar).
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

  // ---------------------------------------------------------------------------
  // Derived values
  // ---------------------------------------------------------------------------

  const levelPercent = rating != null ? Math.min(100, (rating.level / MAX_LEVEL) * 100) : 0;

  const result = state?.result;
  const durationMs = state?.durationMs;
  const durationSec = durationMs != null ? Math.round(durationMs / 1000) : null;

  const eloDelta = state?.eloDelta;
  const eloDeltaStr = eloDelta != null ? (eloDelta >= 0 ? `+${eloDelta}` : `${eloDelta}`) : '—';

  const clipUrl = state?.mp4Url ?? state?.clipUrl;

  // The opponent to report: an explicit reportedId/opponentId wins; otherwise fall back to
  // whichever of winnerId/loserId is NOT the current user (both are only set for ranked matches).
  const reportedId =
    state?.reportedId ??
    state?.opponentId ??
    (state?.winnerId != null && String(state.winnerId) !== user?.id
      ? state.winnerId
      : state?.loserId != null && String(state.loserId) !== user?.id
        ? state.loserId
        : undefined);

  const matchId = state?.matchId;
  const canReportCheat = reportedId != null && matchId != null;

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  function handlePlayAgain() {
    navigate('/mode-select');
  }

  function handleRematch() {
    setRematchRequested(true);
    onRematch?.();
  }

  function handleShare() {
    if (!clipUrl) return;
    if (onShare) {
      onShare(clipUrl);
      return;
    }
    if (navigator.share) {
      void navigator.share({ url: clipUrl });
    }
  }

  function handleReportCheat() {
    if (!canReportCheat) return;
    setCheatStatus('pending');
    reportsApi
      .reportCheat({ reported_id: reportedId as number, match_id: matchId as string })
      .then(() => setCheatStatus('success'))
      .catch(() => setCheatStatus('error'));
  }

  function handleReportBug() {
    setBugStatus('pending');
    reportsApi
      .reportBug({ device: 'mobile', description: 'Reported from the results screen' })
      .then(() => setBugStatus('success'))
      .catch(() => setBugStatus('error'));
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div
      className={`panel-screen results ${result === 'win' ? 'results--win' : result === 'loss' ? 'results--loss' : ''}`}
      data-testid="results-screen"
    >
      {/* Outcome — win/loss, ±ELO, level progress, duration */}
      <section aria-label="Outcome" className="results-outcome">
        {result ? (
          <div className="results-verdict" data-testid="result-outcome">
            {result === 'win' ? 'Победа!' : 'Поражение'}
          </div>
        ) : (
          <div className="results-verdict results-verdict--none" data-testid="result-outcome-placeholder">
            —
          </div>
        )}

        <div className="results-chips">
          <div
            className={`results-chip ${eloDeltaStr.startsWith('+') ? 'results-chip--up' : eloDeltaStr.startsWith('-') ? 'results-chip--down' : ''}`}
            data-testid="elo-delta"
          >
            {eloDeltaStr}
          </div>
          <div className="results-chip" data-testid="match-duration">
            {durationSec != null ? `${durationSec}s` : '—'}
          </div>
        </div>

        {statsLoading || (!user && !rating) ? (
          <div className="results-note" data-testid="stats-loading">
            Считаем рейтинг…
          </div>
        ) : statsError ? (
          <div className="results-note" data-testid="stats-error">
            Не удалось загрузить рейтинг
          </div>
        ) : rating ? (
          <div className="results-levelbar">
            <div
              role="progressbar"
              data-testid="level-progress"
              aria-valuenow={levelPercent}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={`Level ${rating.level} progress`}
              style={{ width: `${levelPercent}%` }}
            />
          </div>
        ) : (
          <div className="results-note" data-testid="stats-loading">
            Считаем рейтинг…
          </div>
        )}
      </section>

      {/* Navigation — play again / rematch */}
      <section aria-label="Navigation" className="panel-actions">
        <button type="button" className="btn-mode" data-testid="play-again" onClick={handlePlayAgain}>
          Играть ещё
        </button>
        <button
          type="button"
          className="btn-mode btn-mode--unranked"
          data-testid="rematch"
          onClick={handleRematch}
          disabled={rematchRequested}
        >
          {rematchRequested ? 'Реванш заказан' : 'Реванш'}
        </button>
      </section>

      {/* Share to TikTok */}
      <section aria-label="Share" className="results-share">
        {clipUrl ? (
          <button type="button" className="results-share-btn" data-testid="share-tiktok" onClick={handleShare}>
            🎬 Шеринг в TikTok
          </button>
        ) : (
          <div className="results-note" data-testid="share-unavailable">
            Клип ещё не готов
          </div>
        )}
      </section>

      {/* Reports — cheat / bug */}
      <section aria-label="Reports" className="results-reports">
        <button
          type="button"
          className="results-report-btn"
          data-testid="report-cheat"
          onClick={handleReportCheat}
          disabled={!canReportCheat || cheatStatus === 'pending'}
        >
          Читер!
        </button>
        {cheatStatus === 'success' && (
          <div className="results-note" data-testid="cheat-report-success">
            Жалоба отправлена
          </div>
        )}
        {cheatStatus === 'error' && (
          <div className="results-note" data-testid="cheat-report-error">
            Не удалось отправить
          </div>
        )}

        <button
          type="button"
          className="results-report-btn"
          data-testid="report-bug"
          onClick={handleReportBug}
          disabled={bugStatus === 'pending'}
        >
          Сломалось?
        </button>
        {bugStatus === 'success' && (
          <div className="results-note" data-testid="bug-report-success">
            Жалоба отправлена
          </div>
        )}
        {bugStatus === 'error' && (
          <div className="results-note" data-testid="bug-report-error">
            Не удалось отправить
          </div>
        )}
      </section>
    </div>
  );
}
