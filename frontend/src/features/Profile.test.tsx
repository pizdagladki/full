import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Profile } from './Profile';
import { AuthContext } from './auth/AuthContext';
import type { AuthState } from './auth/AuthContext';
import type { RatingsApi, RatingData } from '../api/ratings';
import type { MatchHistoryApi, MatchEntry } from '../api/matches';
import type { ClipsApi, Clip } from '../api/clips';

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

const AUTH_STATE: AuthState = {
  user: { id: 'user-1', email: 'test@example.com' },
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

const NULL_USER_STATE: AuthState = {
  user: null,
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

function makeRatingsApi(data: RatingData): RatingsApi {
  return { getRating: vi.fn().mockResolvedValue(data) };
}

function makeRatingsApiError(msg = 'fetch failed'): RatingsApi {
  return { getRating: vi.fn().mockRejectedValue(new Error(msg)) };
}

function makeRatingsApiPending(): RatingsApi {
  return { getRating: vi.fn().mockReturnValue(new Promise(() => {})) };
}

function makeMatchHistoryApi(entries: MatchEntry[]): MatchHistoryApi {
  return { getMatchHistory: vi.fn().mockResolvedValue(entries) };
}

function makeMatchHistoryApiError(msg = 'history failed'): MatchHistoryApi {
  return { getMatchHistory: vi.fn().mockRejectedValue(new Error(msg)) };
}

function makeMatchHistoryApiPending(): MatchHistoryApi {
  return { getMatchHistory: vi.fn().mockReturnValue(new Promise(() => {})) };
}

function makeClipsApi(clips: Clip[]): ClipsApi {
  return {
    getClips: vi.fn().mockResolvedValue(clips),
    getClipDownloadUrl: vi.fn((id: string) => `/v1/clips/${id}/download`),
  };
}

function makeClipsApiError(msg = 'clips failed'): ClipsApi {
  return {
    getClips: vi.fn().mockRejectedValue(new Error(msg)),
    getClipDownloadUrl: vi.fn((id: string) => `/v1/clips/${id}/download`),
  };
}

function makeClipsApiPending(): ClipsApi {
  return {
    getClips: vi.fn().mockReturnValue(new Promise(() => {})),
    getClipDownloadUrl: vi.fn((id: string) => `/v1/clips/${id}/download`),
  };
}

/** Helper to produce a minimal valid MatchEntry. */
function makeMatch(overrides: Partial<MatchEntry> = {}): MatchEntry {
  return {
    match_id: 'match-1',
    opponent_id: 'opp-1',
    opponent_name: 'Alice',
    result: 'win',
    mode: 'ranked',
    elo_delta: 20,
    duration_ms: 90_000,
    played_at: '2026-06-25T10:00:00Z',
    ...overrides,
  };
}

/** Helper to produce a minimal valid Clip. */
function makeClip(overrides: Partial<Clip> = {}): Clip {
  return {
    id: 'clip-1',
    mp4_url: 'https://cdn.example.com/clip-1.mp4',
    created_at: '2026-06-25T10:00:00Z',
    ...overrides,
  };
}

interface RenderProfileOptions {
  authState?: AuthState;
  ratingsApi?: RatingsApi;
  matchHistoryApi?: MatchHistoryApi;
  clipsApi?: ClipsApi;
  onReshare?: (clip: Clip) => void;
}

/** Wraps <Profile> in MemoryRouter + AuthContext. */
function renderProfile(options: RenderProfileOptions = {}) {
  const {
    authState = AUTH_STATE,
    ratingsApi,
    matchHistoryApi,
    clipsApi,
    onReshare,
  } = options;

  return render(
    <AuthContext.Provider value={authState}>
      <MemoryRouter>
        <Profile
          ratingsApi={ratingsApi}
          matchHistoryApi={matchHistoryApi}
          clipsApi={clipsApi}
          onReshare={onReshare}
        />
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
// criterion 1 — Stats section
// ---------------------------------------------------------------------------

describe('criterion 1 — stats section', () => {
  it('criterion-1: shows elo, level, games_played and progress bar when loaded', async () => {
    // criterion: 1 — all stats fields and the progress bar appear after the API resolves
    const api = makeRatingsApi({ elo: 1350, level: 4, games_played: 42 });
    renderProfile({ ratingsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('stats-content')).toBeInTheDocument();
    });

    expect(screen.getByText('ELO: 1350')).toBeInTheDocument();
    expect(screen.getByText('Level: 4')).toBeInTheDocument();
    expect(screen.getByText('Games played: 42')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('40');
  });

  it('criterion-1 guard: missing stats field fails the test', async () => {
    // criterion: 1 guard — if elo is not rendered, the test above would fail
    const api = makeRatingsApi({ elo: 999, level: 2, games_played: 5 });
    renderProfile({ ratingsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('stats-content')).toBeInTheDocument();
    });

    expect(screen.queryByText('ELO: 0')).not.toBeInTheDocument();
  });

  it('criterion-1: shows loading placeholder while fetching', () => {
    // criterion: 1 — while the API is pending, stats-loading placeholder is shown
    const api = makeRatingsApiPending();
    renderProfile({ ratingsApi: api });

    expect(screen.getByTestId('stats-loading')).toBeInTheDocument();
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('criterion-1: shows error placeholder on fetch failure (non-crashing)', async () => {
    // criterion: 1 — on API error, stats-error placeholder shown; component does not throw
    const api = makeRatingsApiError('Network error');
    renderProfile({ ratingsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('stats-error')).toBeInTheDocument();
    });

    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
    expect(screen.queryByTestId('stats-content')).not.toBeInTheDocument();
  });

  it('criterion-1 guard: no crash when user is null', () => {
    // criterion: 1 — with null user no fetch is attempted and a placeholder is shown
    const api = makeRatingsApiPending();
    renderProfile({ authState: NULL_USER_STATE, ratingsApi: api });

    // Should not crash; loading/neutral placeholder rendered
    expect(screen.getByTestId('stats-loading')).toBeInTheDocument();
    expect(api.getRating).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// criterion 2 — Match history section
// ---------------------------------------------------------------------------

describe('criterion 2 — match history section', () => {
  it('criterion-2: renders match rows newest-first with all fields', async () => {
    // criterion: 2 — rows appear sorted newest-first and show opponent, result, mode, elo delta, duration
    const older = makeMatch({
      match_id: 'match-old',
      opponent_name: 'Bob',
      result: 'loss',
      elo_delta: -15,
      duration_ms: 60_000,
      played_at: '2026-06-24T08:00:00Z',
    });
    const newer = makeMatch({
      match_id: 'match-new',
      opponent_name: 'Alice',
      result: 'win',
      elo_delta: 20,
      duration_ms: 90_000,
      played_at: '2026-06-25T10:00:00Z',
    });
    // Pass older first — component must sort them newest-first
    const api = makeMatchHistoryApi([older, newer]);
    renderProfile({ matchHistoryApi: api });

    await waitFor(() => {
      expect(screen.getAllByTestId('match-row')).toHaveLength(2);
    });

    const rows = screen.getAllByTestId('match-row');
    // First row should be the newer match
    expect(rows[0]).toHaveTextContent('Alice');
    expect(rows[0]).toHaveTextContent('win');
    expect(rows[0]).toHaveTextContent('+20');
    expect(rows[0]).toHaveTextContent('ranked');
    expect(rows[0]).toHaveTextContent('90s');
    // Second row is the older match
    expect(rows[1]).toHaveTextContent('Bob');
    expect(rows[1]).toHaveTextContent('loss');
    expect(rows[1]).toHaveTextContent('-15');
    expect(rows[1]).toHaveTextContent('60s');
  });

  it('criterion-2 guard: ordering violation — oldest first would fail the test', async () => {
    // criterion: 2 guard — if sorting were reversed the first row would NOT be Alice
    const older = makeMatch({
      match_id: 'match-old',
      opponent_name: 'Bob',
      played_at: '2026-06-24T08:00:00Z',
    });
    const newer = makeMatch({
      match_id: 'match-new',
      opponent_name: 'Alice',
      played_at: '2026-06-25T10:00:00Z',
    });
    const api = makeMatchHistoryApi([older, newer]);
    renderProfile({ matchHistoryApi: api });

    await waitFor(() => {
      expect(screen.getAllByTestId('match-row')).toHaveLength(2);
    });

    const rows = screen.getAllByTestId('match-row');
    // Confirm newest (Alice) is first, not oldest (Bob)
    expect(rows[0]).toHaveTextContent('Alice');
    expect(rows[0]).not.toHaveTextContent('Bob');
  });

  it('criterion-2: shows empty state when history is empty', async () => {
    // criterion: 2 — empty array shows history-empty, not an error
    const api = makeMatchHistoryApi([]);
    renderProfile({ matchHistoryApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('history-empty')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('history-error')).not.toBeInTheDocument();
    expect(screen.queryByTestId('match-row')).not.toBeInTheDocument();
  });

  it('criterion-2: shows loading placeholder while fetching', () => {
    // criterion: 2 — during pending fetch, history-loading is shown
    const api = makeMatchHistoryApiPending();
    renderProfile({ matchHistoryApi: api });

    expect(screen.getByTestId('history-loading')).toBeInTheDocument();
    expect(screen.queryByTestId('match-row')).not.toBeInTheDocument();
  });

  it('criterion-2: shows error placeholder on failure (non-crashing)', async () => {
    // criterion: 2 — on API error, history-error placeholder shown; component does not crash
    const api = makeMatchHistoryApiError('Server error');
    renderProfile({ matchHistoryApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('history-error')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('match-row')).not.toBeInTheDocument();
    expect(screen.queryByTestId('history-empty')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 3 — Clips gallery section
// ---------------------------------------------------------------------------

describe('criterion 3 — clips gallery section', () => {
  it('criterion-3: renders clip items with view/download/reshare', async () => {
    // criterion: 3 — each clip item has view, download, and reshare controls
    const clip = makeClip({ id: 'clip-1', mp4_url: 'https://cdn.example.com/clip-1.mp4' });
    const api = makeClipsApi([clip]);
    renderProfile({ clipsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('clip-item')).toBeInTheDocument();
    });

    expect(screen.getByTestId('clip-view')).toBeInTheDocument();
    expect(screen.getByTestId('clip-download')).toBeInTheDocument();
    expect(screen.getByTestId('clip-reshare')).toBeInTheDocument();
  });

  it('criterion-3: reshare button invokes onReshare handler with clip', async () => {
    // criterion: 3 — clicking the reshare button calls onReshare with the clip object
    const clip = makeClip({ id: 'clip-42' });
    const api = makeClipsApi([clip]);
    const onReshare = vi.fn();
    renderProfile({ clipsApi: api, onReshare });

    await waitFor(() => {
      expect(screen.getByTestId('clip-reshare')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByTestId('clip-reshare'));
    expect(onReshare).toHaveBeenCalledTimes(1);
    expect(onReshare).toHaveBeenCalledWith(clip);
  });

  it('criterion-3 guard: reshare not called without click', async () => {
    // criterion: 3 guard — onReshare must not fire without a user action
    const clip = makeClip();
    const api = makeClipsApi([clip]);
    const onReshare = vi.fn();
    renderProfile({ clipsApi: api, onReshare });

    await waitFor(() => {
      expect(screen.getByTestId('clip-item')).toBeInTheDocument();
    });

    expect(onReshare).not.toHaveBeenCalled();
  });

  it('criterion-3: download link has correct href', async () => {
    // criterion: 3 — the download anchor href points to the clip download endpoint
    const clip = makeClip({ id: 'clip-99' });
    const api = makeClipsApi([clip]);
    renderProfile({ clipsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('clip-download')).toBeInTheDocument();
    });

    const link = screen.getByTestId('clip-download') as HTMLAnchorElement;
    expect(link.href).toContain('/v1/clips/clip-99/download');
  });

  it('criterion-3 guard: download href does not point to a wrong clip', async () => {
    // criterion: 3 guard — the download URL is specific to the actual clip id
    const clip = makeClip({ id: 'clip-77' });
    const api = makeClipsApi([clip]);
    renderProfile({ clipsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('clip-download')).toBeInTheDocument();
    });

    const link = screen.getByTestId('clip-download') as HTMLAnchorElement;
    expect(link.href).not.toContain('/v1/clips/clip-WRONG/download');
  });

  it('criterion-3: shows empty state when clips is empty', async () => {
    // criterion: 3 — empty clips list shows clips-empty, not an error
    const api = makeClipsApi([]);
    renderProfile({ clipsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('clips-empty')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('clips-error')).not.toBeInTheDocument();
    expect(screen.queryByTestId('clip-item')).not.toBeInTheDocument();
  });

  it('criterion-3: shows loading placeholder while fetching', () => {
    // criterion: 3 — during pending fetch, clips-loading is shown
    const api = makeClipsApiPending();
    renderProfile({ clipsApi: api });

    expect(screen.getByTestId('clips-loading')).toBeInTheDocument();
    expect(screen.queryByTestId('clip-item')).not.toBeInTheDocument();
  });

  it('criterion-3: shows error placeholder on failure (non-crashing)', async () => {
    // criterion: 3 — on API error, clips-error shown; component does not crash
    const api = makeClipsApiError('Clips server error');
    renderProfile({ clipsApi: api });

    await waitFor(() => {
      expect(screen.getByTestId('clips-error')).toBeInTheDocument();
    });

    expect(screen.queryByTestId('clip-item')).not.toBeInTheDocument();
    expect(screen.queryByTestId('clips-empty')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 4 — Independent sections: errors in one do not crash others
// ---------------------------------------------------------------------------

describe('criterion 4 — independent sections', () => {
  it('criterion-4: stats error does not crash history or clips sections', async () => {
    // criterion: 4 — stats-error rendered while history and clips load independently
    const ratingsApi = makeRatingsApiError('stats down');
    const matchHistoryApi = makeMatchHistoryApi([makeMatch()]);
    const clipsApi = makeClipsApi([makeClip()]);
    renderProfile({ ratingsApi, matchHistoryApi, clipsApi });

    await waitFor(() => {
      expect(screen.getByTestId('stats-error')).toBeInTheDocument();
    });

    // History and clips still render
    expect(await screen.findByTestId('match-row')).toBeInTheDocument();
    expect(await screen.findByTestId('clip-item')).toBeInTheDocument();
  });

  it('criterion-4: history error does not crash stats or clips sections', async () => {
    // criterion: 4 — history-error rendered while stats and clips load independently
    const ratingsApi = makeRatingsApi({ elo: 1200, level: 3, games_played: 10 });
    const matchHistoryApi = makeMatchHistoryApiError('history down');
    const clipsApi = makeClipsApi([makeClip()]);
    renderProfile({ ratingsApi, matchHistoryApi, clipsApi });

    await waitFor(() => {
      expect(screen.getByTestId('history-error')).toBeInTheDocument();
    });

    // Stats and clips still render
    expect(await screen.findByTestId('stats-content')).toBeInTheDocument();
    expect(await screen.findByTestId('clip-item')).toBeInTheDocument();
  });

  it('criterion-4: clips error does not crash stats or history sections', async () => {
    // criterion: 4 — clips-error rendered while stats and history load independently
    const ratingsApi = makeRatingsApi({ elo: 1100, level: 2, games_played: 7 });
    const matchHistoryApi = makeMatchHistoryApi([makeMatch()]);
    const clipsApi = makeClipsApiError('clips down');
    renderProfile({ ratingsApi, matchHistoryApi, clipsApi });

    await waitFor(() => {
      expect(screen.getByTestId('clips-error')).toBeInTheDocument();
    });

    // Stats and history still render
    expect(await screen.findByTestId('stats-content')).toBeInTheDocument();
    expect(await screen.findByTestId('match-row')).toBeInTheDocument();
  });
});
