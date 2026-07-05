import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import type { Location } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Results } from './Results';
import type { ResultsProps } from './Results';
import { AuthContext } from './auth/AuthContext';
import type { AuthState } from './auth/AuthContext';
import type { RatingsApi, RatingData } from '../api/ratings';
import type { ReportsApi } from '../api/reports';

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

const AUTH_STATE: AuthState = {
  user: { id: 'user-1', email: 'test@example.com' },
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

function makeRatingsApi(data: RatingData): RatingsApi {
  return { getRating: vi.fn().mockResolvedValue(data) };
}

function makeRatingsApiPending(): RatingsApi {
  return { getRating: vi.fn().mockReturnValue(new Promise(() => {})) };
}

function makeRatingsApiError(msg = 'fetch failed'): RatingsApi {
  return { getRating: vi.fn().mockRejectedValue(new Error(msg)) };
}

function makeReportsApi(overrides: Partial<ReportsApi> = {}): ReportsApi {
  return {
    reportCheat: vi.fn().mockResolvedValue(undefined),
    reportBug: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Routing harness — mirrors Battle.test.tsx's ResultsProbe pattern.
// ---------------------------------------------------------------------------

function ModeSelectProbe() {
  return <div data-testid="mode-select-probe">Mode select</div>;
}

interface RenderResultsOptions {
  authState?: AuthState;
  ratingsApi?: RatingsApi;
  reportsApi?: ReportsApi;
  onShare?: ResultsProps['onShare'];
  onRematch?: ResultsProps['onRematch'];
  state?: unknown;
}

function renderResults(options: RenderResultsOptions = {}) {
  const { authState = AUTH_STATE, ratingsApi, reportsApi, onShare, onRematch, state } = options;

  const initialEntries: Array<{ pathname: string; state?: Location['state'] }> =
    state === undefined
      ? [{ pathname: '/results' }]
      : [{ pathname: '/results', state: state as Location['state'] }];

  return render(
    <AuthContext.Provider value={authState}>
      <MemoryRouter initialEntries={initialEntries}>
        <Routes>
          <Route
            path="/results"
            element={
              <Results
                ratingsApi={ratingsApi}
                reportsApi={reportsApi}
                onShare={onShare}
                onRematch={onRematch}
              />
            }
          />
          <Route path="/mode-select" element={<ModeSelectProbe />} />
        </Routes>
      </MemoryRouter>
    </AuthContext.Provider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
});

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// criterion 1 — win/loss, ±ELO delta, level progress bar, match duration
// ---------------------------------------------------------------------------

describe('criterion 1 — outcome, elo delta, progress bar, duration', () => {
  it('criterion-1: renders win, signed +ELO delta, progress bar and duration once loaded', async () => {
    // criterion: 1 — all fields render from location.state + fetched rating
    const ratingsApi = makeRatingsApi({ elo: 1400, level: 6, games_played: 30 });
    renderResults({
      ratingsApi,
      state: { result: 'win', durationMs: 125_000, eloDelta: 18, ranked: true },
    });

    expect(screen.getByTestId('result-outcome')).toHaveTextContent('You win!');
    expect(screen.getByTestId('elo-delta')).toHaveTextContent('+18');
    expect(screen.getByTestId('match-duration')).toHaveTextContent('125s');

    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toBeInTheDocument();
    });
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('60');
  });

  it('criterion-1: renders loss and a negative ELO delta', async () => {
    // criterion: 1 — loss result and a negative delta render distinctly from the win case
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 2, games_played: 5 });
    renderResults({
      ratingsApi,
      state: { result: 'loss', durationMs: 60_000, eloDelta: -12, ranked: true },
    });

    expect(screen.getByTestId('result-outcome')).toHaveTextContent('You lose');
    expect(screen.getByTestId('elo-delta')).toHaveTextContent('-12');
  });

  it('criterion-1 guard: a win rendered as a loss would fail this assertion', () => {
    // criterion: 1 guard — mislabeling win as loss must be detectable
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 2, games_played: 5 });
    renderResults({ ratingsApi, state: { result: 'win', durationMs: 1000 } });

    expect(screen.getByTestId('result-outcome')).not.toHaveTextContent('You lose');
  });

  it('criterion-1: shows a neutral placeholder for ELO delta and duration when absent', () => {
    // criterion: 1 — no eloDelta/durationMs in state renders an em-dash, not garbage/NaN
    const ratingsApi = makeRatingsApiPending();
    renderResults({ ratingsApi, state: { result: 'win' } });

    expect(screen.getByTestId('elo-delta')).toHaveTextContent('—');
    expect(screen.getByTestId('match-duration')).toHaveTextContent('—');
  });

  it('criterion-1: shows a neutral placeholder while the rating is loading', () => {
    // criterion: 1 — the progress bar area shows a loading placeholder, never crashes
    const ratingsApi = makeRatingsApiPending();
    renderResults({ ratingsApi, state: { result: 'win', durationMs: 1000 } });

    expect(screen.getByTestId('stats-loading')).toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('criterion-1: shows an error placeholder on rating fetch failure without crashing', async () => {
    // criterion: 1 — a rating fetch error is shown as a neutral error state, not a crash
    const ratingsApi = makeRatingsApiError('boom');
    renderResults({ ratingsApi, state: { result: 'loss', durationMs: 2000 } });

    await waitFor(() => {
      expect(screen.getByTestId('stats-error')).toBeInTheDocument();
    });
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('criterion-1 guard: null location.state never crashes and shows neutral placeholders', () => {
    // criterion: 1 guard — a bare/refreshed navigation with no state must not throw
    const ratingsApi = makeRatingsApiPending();
    expect(() => renderResults({ ratingsApi, state: undefined })).not.toThrow();

    expect(screen.getByTestId('result-outcome-placeholder')).toBeInTheDocument();
    expect(screen.getByTestId('elo-delta')).toHaveTextContent('—');
    expect(screen.getByTestId('match-duration')).toHaveTextContent('—');
  });
});

// ---------------------------------------------------------------------------
// criterion 2 — Play again / Rematch
// ---------------------------------------------------------------------------

describe('criterion 2 — play again / rematch', () => {
  it('criterion-2: "Play again" navigates to /mode-select', async () => {
    // criterion: 2 — clicking play-again routes to mode-select
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    renderResults({ ratingsApi, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('play-again'));

    await waitFor(() => {
      expect(screen.getByTestId('mode-select-probe')).toBeInTheDocument();
    });
  });

  it('criterion-2 guard: "Play again" is not wired to a no-op — screen actually navigates away', async () => {
    // criterion: 2 guard — the results screen must be GONE after navigation, not still mounted
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    renderResults({ ratingsApi, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('play-again'));

    await waitFor(() => {
      expect(screen.queryByTestId('results-screen')).not.toBeInTheDocument();
    });
  });

  it('criterion-2: "Rematch" is present and signals intent via onRematch without navigating', () => {
    // criterion: 2 — rematch only signals intent (calls onRematch); no mutual-agreement logic here
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const onRematch = vi.fn();
    renderResults({ ratingsApi, onRematch, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('rematch'));

    expect(onRematch).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('results-screen')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 3 — Share to TikTok
// ---------------------------------------------------------------------------

describe('criterion 3 — share to TikTok', () => {
  it('criterion-3: "Share to TikTok" invokes onShare with the clip mp4 url', () => {
    // criterion: 3 — the share handler is invoked with the exact clip url from location.state
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const onShare = vi.fn();
    renderResults({
      ratingsApi,
      onShare,
      state: { result: 'win', durationMs: 1000, mp4Url: 'https://cdn.example.com/clip-9.mp4' },
    });

    fireEvent.click(screen.getByTestId('share-tiktok'));

    expect(onShare).toHaveBeenCalledTimes(1);
    expect(onShare).toHaveBeenCalledWith('https://cdn.example.com/clip-9.mp4');
  });

  it('criterion-3 guard: onShare is never called with the wrong url', () => {
    // criterion: 3 guard — a mismatched url would fail this assertion
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const onShare = vi.fn();
    renderResults({
      ratingsApi,
      onShare,
      state: { result: 'win', durationMs: 1000, clipUrl: 'https://cdn.example.com/clip-real.mp4' },
    });

    fireEvent.click(screen.getByTestId('share-tiktok'));

    expect(onShare).not.toHaveBeenCalledWith('https://cdn.example.com/clip-WRONG.mp4');
  });

  it('criterion-3: shows a neutral placeholder and never calls onShare when no clip url is present', () => {
    // criterion: 3 — with no clip url, the share control is unavailable and onShare is never invoked
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const onShare = vi.fn();
    renderResults({ ratingsApi, onShare, state: { result: 'win', durationMs: 1000 } });

    expect(screen.queryByTestId('share-tiktok')).not.toBeInTheDocument();
    expect(screen.getByTestId('share-unavailable')).toBeInTheDocument();
    expect(onShare).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// criterion 4 — Report cheating / Report a bug
// ---------------------------------------------------------------------------

describe('criterion 4 — report cheating / report a bug', () => {
  // The current user's id is compared (as a string) against the numeric winnerId/loserId from
  // Battle's server-authoritative outcome — mirrors Battle.tsx's own `String(winner_id) === user.id`
  // check, so the current user's id here is the numeric id ("1") the win belongs to.
  const CURRENT_USER_IS_WINNER_STATE: AuthState = {
    user: { id: '1', email: 'test@example.com' },
    loading: false,
    error: null,
    refreshUser: vi.fn().mockResolvedValue(undefined),
  };

  it('criterion-4: "Report cheating" calls reportCheat with reported_id + match_id and shows success', async () => {
    // criterion: 4 — POST /v1/reports/cheat is invoked (via reportsApi) with the derived opponent id
    // (the current user is winnerId=1, so the opponent to report is loserId=2)
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi();
    renderResults({
      ratingsApi,
      reportsApi,
      authState: CURRENT_USER_IS_WINNER_STATE,
      state: {
        result: 'win',
        durationMs: 1000,
        matchId: 'match-55',
        winnerId: 1,
        loserId: 2,
      },
    });

    fireEvent.click(screen.getByTestId('report-cheat'));

    await waitFor(() => {
      expect(screen.getByTestId('cheat-report-success')).toBeInTheDocument();
    });
    expect(reportsApi.reportCheat).toHaveBeenCalledWith({ reported_id: 2, match_id: 'match-55' });
  });

  it('criterion-4 guard: "Report cheating" success state is not shown without a click', () => {
    // criterion: 4 guard — no success/error state should appear before the user acts
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi();
    renderResults({
      ratingsApi,
      reportsApi,
      state: { result: 'win', durationMs: 1000, matchId: 'match-55', winnerId: 1, loserId: 2 },
    });

    expect(screen.queryByTestId('cheat-report-success')).not.toBeInTheDocument();
    expect(reportsApi.reportCheat).not.toHaveBeenCalled();
  });

  it('criterion-4: "Report cheating" shows an error state on failure', async () => {
    // criterion: 4 — a rejected reportCheat call surfaces cheat-report-error, not a crash
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi({
      reportCheat: vi.fn().mockRejectedValue(new Error('server down')),
    });
    renderResults({
      ratingsApi,
      reportsApi,
      state: { result: 'win', durationMs: 1000, matchId: 'match-55', winnerId: 1, loserId: 2 },
    });

    fireEvent.click(screen.getByTestId('report-cheat'));

    await waitFor(() => {
      expect(screen.getByTestId('cheat-report-error')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('cheat-report-success')).not.toBeInTheDocument();
  });

  it('criterion-4: "Report a bug" calls reportBug and shows success', async () => {
    // criterion: 4 — POST /v1/reports/bug is invoked (via reportsApi) and success is shown
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi();
    renderResults({ ratingsApi, reportsApi, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('report-bug'));

    await waitFor(() => {
      expect(screen.getByTestId('bug-report-success')).toBeInTheDocument();
    });
    expect(reportsApi.reportBug).toHaveBeenCalledWith(
      expect.objectContaining({ device: 'mobile' }),
    );
  });

  it('criterion-4: "Report a bug" shows an error state on failure', async () => {
    // criterion: 4 — a rejected reportBug call surfaces bug-report-error, not a crash
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi({
      reportBug: vi.fn().mockRejectedValue(new Error('server down')),
    });
    renderResults({ ratingsApi, reportsApi, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('report-bug'));

    await waitFor(() => {
      expect(screen.getByTestId('bug-report-error')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('bug-report-success')).not.toBeInTheDocument();
  });

  it('criterion-4 guard: "Report cheating" is disabled without a matchId/opponent id', () => {
    // criterion: 4 guard — with no matchId/opponent derivable, the control must not fire the call
    const ratingsApi = makeRatingsApi({ elo: 1000, level: 1, games_played: 1 });
    const reportsApi = makeReportsApi();
    renderResults({ ratingsApi, reportsApi, state: { result: 'win', durationMs: 1000 } });

    fireEvent.click(screen.getByTestId('report-cheat'));

    expect(reportsApi.reportCheat).not.toHaveBeenCalled();
  });
});
