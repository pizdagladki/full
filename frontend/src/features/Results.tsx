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
    <div data-testid="results-screen">
      <h1>Results</h1>

      {/* ------------------------------------------------------------------ */}
      {/* Outcome — win/loss, ±ELO, level progress, duration                   */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Outcome">
        {result ? (
          <div data-testid="result-outcome">{result === 'win' ? 'You win!' : 'You lose'}</div>
        ) : (
          <div data-testid="result-outcome-placeholder">—</div>
        )}

        <div data-testid="elo-delta">{eloDeltaStr}</div>

        <div data-testid="match-duration">{durationSec != null ? `${durationSec}s` : '—'}</div>

        {statsLoading || (!user && !rating) ? (
          <div data-testid="stats-loading">Loading rating…</div>
        ) : statsError ? (
          <div data-testid="stats-error">Could not load rating</div>
        ) : rating ? (
          <div
            role="progressbar"
            data-testid="level-progress"
            aria-valuenow={levelPercent}
            aria-valuemin={0}
            aria-valuemax={100}
            aria-label={`Level ${rating.level} progress`}
            style={{ width: `${levelPercent}%`, background: '#4caf50', height: '8px' }}
          />
        ) : (
          <div data-testid="stats-loading">Loading rating…</div>
        )}
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Navigation — play again / rematch                                    */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Navigation">
        <button type="button" data-testid="play-again" onClick={handlePlayAgain}>
          Play again
        </button>
        <button
          type="button"
          data-testid="rematch"
          onClick={handleRematch}
          disabled={rematchRequested}
        >
          {rematchRequested ? 'Rematch requested' : 'Rematch'}
        </button>
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Share to TikTok                                                      */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Share">
        {clipUrl ? (
          <button type="button" data-testid="share-tiktok" onClick={handleShare}>
            Share to TikTok
          </button>
        ) : (
          <div data-testid="share-unavailable">No clip to share yet</div>
        )}
      </section>

      {/* ------------------------------------------------------------------ */}
      {/* Reports — cheat / bug                                               */}
      {/* ------------------------------------------------------------------ */}
      <section aria-label="Reports">
        <button
          type="button"
          data-testid="report-cheat"
          onClick={handleReportCheat}
          disabled={!canReportCheat || cheatStatus === 'pending'}
        >
          Report cheating
        </button>
        {cheatStatus === 'success' && (
          <div data-testid="cheat-report-success">Report submitted</div>
        )}
        {cheatStatus === 'error' && (
          <div data-testid="cheat-report-error">Could not submit report</div>
        )}

        <button
          type="button"
          data-testid="report-bug"
          onClick={handleReportBug}
          disabled={bugStatus === 'pending'}
        >
          Report a bug
        </button>
        {bugStatus === 'success' && <div data-testid="bug-report-success">Report submitted</div>}
        {bugStatus === 'error' && <div data-testid="bug-report-error">Could not submit report</div>}
      </section>
    </div>
  );
}
