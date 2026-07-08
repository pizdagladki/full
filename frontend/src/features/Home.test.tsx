import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { Home } from './Home';
import { AuthContext } from './auth/AuthContext';
import type { AuthState } from './auth/AuthContext';
import type { RatingsApi, RatingData } from '../api/ratings';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const AUTH_STATE: AuthState = {
  user: { id: 'user-42', email: 'test@example.com' },
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

function renderHome(authState: AuthState = AUTH_STATE, ratingsApi?: RatingsApi) {
  return render(
    <AuthContext.Provider value={authState}>
      <MemoryRouter initialEntries={['/home']}>
        <Routes>
          <Route path="/home" element={<Home ratingsApi={ratingsApi} />} />
          <Route path="/mode-select" element={<div data-testid="mode-select-probe" />} />
          <Route path="/koth" element={<div data-testid="koth-probe" />} />
          <Route path="/invite" element={<div data-testid="invite-probe" />} />
        </Routes>
      </MemoryRouter>
    </AuthContext.Provider>,
  );
}

function makeRatingsApi(data: RatingData): RatingsApi {
  return { getRating: vi.fn().mockResolvedValue(data) };
}

function makeRatingsApiError(message = 'fetch failed'): RatingsApi {
  return { getRating: vi.fn().mockRejectedValue(new Error(message)) };
}

const RATING: RatingData = { user_id: 'user-42', elo: 1000, level: 4, games_played: 12 } as RatingData;

// ---------------------------------------------------------------------------
// Rank widget (LVL + ELO)
// ---------------------------------------------------------------------------

describe('Home — rank widget', () => {
  it('shows the level plate and the ELO pill once the rating resolves', async () => {
    renderHome(AUTH_STATE, makeRatingsApi(RATING));
    await waitFor(() => {
      expect(screen.getByText('ELO 1000')).toBeInTheDocument();
    });
    expect(screen.getByText('4')).toBeInTheDocument();
  });

  it('renders a progressbar whose aria-valuenow reflects level/10 * 100', async () => {
    renderHome(AUTH_STATE, makeRatingsApi(RATING));
    await waitFor(() => {
      expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '40');
    });
  });

  it('shows a neutral placeholder on API error, does not crash', async () => {
    renderHome(AUTH_STATE, makeRatingsApiError());
    await waitFor(() => {
      expect(screen.getByTestId('level-placeholder')).toHaveTextContent('Could not load level');
    });
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument();
  });

  it('shows a neutral placeholder when user is null (no user id to fetch)', () => {
    renderHome({ ...AUTH_STATE, user: null });
    expect(screen.getByTestId('level-placeholder')).toHaveTextContent('Loading level…');
  });
});

// ---------------------------------------------------------------------------
// Mode windows — three entries replacing the single Play button
// ---------------------------------------------------------------------------

describe('Home — mode windows', () => {
  it('renders three mode windows: battles, koth, invite', () => {
    renderHome();
    expect(screen.getByTestId('mode-battles')).toBeInTheDocument();
    expect(screen.getByTestId('mode-koth')).toBeInTheDocument();
    expect(screen.getByTestId('mode-invite')).toBeInTheDocument();
  });

  it('battles window navigates to /mode-select', async () => {
    renderHome();
    fireEvent.click(screen.getByTestId('mode-battles'));
    expect(await screen.findByTestId('mode-select-probe')).toBeInTheDocument();
  });

  it('koth window navigates to /koth', async () => {
    renderHome();
    fireEvent.click(screen.getByTestId('mode-koth'));
    expect(await screen.findByTestId('koth-probe')).toBeInTheDocument();
  });

  it('invite window navigates to /invite', async () => {
    renderHome();
    fireEvent.click(screen.getByTestId('mode-invite'));
    expect(await screen.findByTestId('invite-probe')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Chrome: corner links, ad slots, design signature, no camera here
// ---------------------------------------------------------------------------

describe('Home — chrome', () => {
  it('renders corner links to /store and /profile', () => {
    renderHome();
    const links = screen.getAllByRole('link');
    const hrefs = links.map((l) => l.getAttribute('href'));
    expect(hrefs).toContain('/store');
    expect(hrefs).toContain('/profile');
  });

  it('renders two ad-slot placeholders (top + bottom)', () => {
    renderHome();
    expect(screen.getAllByTestId('ad-slot')).toHaveLength(2);
  });

  it('renders the sun-eye design signature', () => {
    renderHome();
    expect(screen.getByTestId('sun-eye')).toBeInTheDocument();
  });

  it('hosts NO camera preview — the camera moved to ModeSelect', () => {
    renderHome();
    expect(screen.queryByTestId('camera-preview')).not.toBeInTheDocument();
    expect(screen.queryByTestId('calibration-status')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/camera/i)).not.toBeInTheDocument();
  });
});
