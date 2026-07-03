import { act, render, screen, waitFor } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { KothMountain } from './KothMountain';
import type { KingInfo, KothApi } from '../api/koth';

// ---------------------------------------------------------------------------
// Fake koth API doubles
// ---------------------------------------------------------------------------

function makeKothApi(options: { king?: KingInfo | null | Error } = {}): KothApi {
  const { king = null } = options;
  return {
    challengeHill: vi.fn(),
    submitRankedAttempt: vi.fn(),
    getKing: king instanceof Error ? vi.fn().mockRejectedValue(king) : vi.fn().mockResolvedValue(king),
    getRankedLeaderboard: vi.fn(),
    getRankedMe: vi.fn(),
  };
}

function makePendingKothApi(): KothApi {
  return {
    challengeHill: vi.fn(),
    submitRankedAttempt: vi.fn(),
    getKing: vi.fn().mockReturnValue(new Promise(() => {})),
    getRankedLeaderboard: vi.fn(),
    getRankedMe: vi.fn(),
  };
}

// ---------------------------------------------------------------------------
// Routing harness
// ---------------------------------------------------------------------------

interface BattleState {
  hillType?: string;
}

function BattleProbe() {
  const location = useLocation();
  const state = (location.state as BattleState | null) ?? null;
  return (
    <div data-testid="battle-probe">
      <span data-testid="battle-hillType">{state?.hillType}</span>
    </div>
  );
}

function renderMountain(
  hillType: string | undefined,
  kothApi: KothApi,
  entry: { pathname?: string; withState?: boolean } = {},
) {
  const { pathname = '/koth/mountain', withState = true } = entry;
  const initialEntry = withState
    ? { pathname, state: { hillType } }
    : pathname;
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/koth/mountain" element={<KothMountain kothApi={kothApi} />} />
        <Route path="/koth/battle" element={<BattleProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('KothMountain', () => {
  // criterion: loading state renders.
  it('criterion-loading: renders the loading placeholder while the king fetch is pending', () => {
    const kothApi = makePendingKothApi();
    renderMountain('daily', kothApi);

    expect(screen.getByTestId('koth-mountain-screen')).toBeInTheDocument();
    expect(screen.getByTestId('mountain-loading')).toBeInTheDocument();
    expect(screen.queryByTestId('mountain-king')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mountain-empty')).not.toBeInTheDocument();
  });

  // criterion: king-loaded renders king info + placeholder zigzag slots 2..10.
  it('criterion-king-loaded: renders the king user_id/blink_ts_ms plus 9 placeholder slots', async () => {
    const king: KingInfo = { user_id: 7, clip_id: 'abc', blink_ts_ms: 1234 };
    const kothApi = makeKothApi({ king });
    renderMountain('daily', kothApi);

    await waitFor(() => expect(screen.getByTestId('mountain-king')).toBeInTheDocument());
    expect(screen.getByTestId('mountain-king-user').textContent).toBe('7');
    expect(screen.getByTestId('mountain-king-blink').textContent).toBe('1234');
    for (let slot = 2; slot <= 10; slot += 1) {
      expect(screen.getByTestId(`mountain-slot-${slot}`)).toBeInTheDocument();
    }
    expect(screen.queryByTestId('mountain-slot-11')).not.toBeInTheDocument();
    expect(screen.queryByTestId('mountain-empty')).not.toBeInTheDocument();
  });

  // criterion: 404/null-king renders the empty state.
  it('criterion-no-king: a null king (404) renders the neutral empty state', async () => {
    const kothApi = makeKothApi({ king: null });
    renderMountain('daily', kothApi);

    await waitFor(() => expect(screen.getByTestId('mountain-empty')).toBeInTheDocument());
    expect(screen.queryByTestId('mountain-king')).not.toBeInTheDocument();
  });

  // criterion (violation guard): a rejected fetch also renders the empty state, never crashes.
  it('criterion-fetch-error: a rejected king fetch degrades to the empty state, never crashes', async () => {
    const kothApi = makeKothApi({ king: new Error('network down') });
    expect(() => renderMountain('daily', kothApi)).not.toThrow();

    await waitFor(() => expect(screen.getByTestId('mountain-empty')).toBeInTheDocument());
  });

  // criterion: the play control navigates to /koth/battle with the right hillType — daily case.
  it('criterion-play-daily: the play control navigates to /koth/battle with hillType daily', async () => {
    const kothApi = makeKothApi({ king: null });
    renderMountain('daily', kothApi);

    await waitFor(() => expect(screen.getByTestId('mountain-empty')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('mountain-play'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-hillType').textContent).toBe('daily');
  });

  // criterion: the play control navigates to /koth/battle with the right hillType — monthly case.
  it('criterion-play-monthly: the play control navigates to /koth/battle with hillType monthly', async () => {
    const kothApi = makeKothApi({ king: null });
    renderMountain('monthly', kothApi);

    await waitFor(() => expect(screen.getByTestId('mountain-empty')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('mountain-play'));

    expect(screen.getByTestId('battle-hillType').textContent).toBe('monthly');
  });

  // criterion: play control is available regardless of king loaded/empty state.
  it('criterion-play-always-available: the play control is present even while a king is loaded', async () => {
    const king: KingInfo = { user_id: 1, clip_id: 'x', blink_ts_ms: 10 };
    const kothApi = makeKothApi({ king });
    renderMountain('daily', kothApi);

    await waitFor(() => expect(screen.getByTestId('mountain-king')).toBeInTheDocument());
    expect(screen.getByTestId('mountain-play')).toBeInTheDocument();
  });

  // criterion (violation guard): missing location.state defaults to daily.
  it('criterion-default-daily: missing location.state falls back to hillType daily', async () => {
    const kothApi = makeKothApi({ king: null });
    renderMountain(undefined, kothApi, { withState: false });

    await waitFor(() => expect(kothApi.getKing).toHaveBeenCalledWith('daily'));
  });

  // criterion (violation guard): a stray 'ranked' hillType is treated as daily, never crashes.
  it('criterion-defensive-ranked: a ranked hillType in location.state is treated as daily', async () => {
    const kothApi = makeKothApi({ king: null });
    expect(() => renderMountain('ranked', kothApi)).not.toThrow();

    await waitFor(() => expect(kothApi.getKing).toHaveBeenCalledWith('daily'));
  });

  // Extra coverage: the optional back control navigates to /koth.
  it('optional back control navigates to /koth', async () => {
    const kothApi = makeKothApi({ king: null });
    render(
      <MemoryRouter initialEntries={[{ pathname: '/koth/mountain', state: { hillType: 'daily' } }]}>
        <Routes>
          <Route path="/koth/mountain" element={<KothMountain kothApi={kothApi} />} />
          <Route path="/koth" element={<div data-testid="koth-probe" />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => expect(screen.getByTestId('mountain-empty')).toBeInTheDocument());
    act(() => {
      fireEvent.click(screen.getByTestId('mountain-back'));
    });
    expect(screen.getByTestId('koth-probe')).toBeInTheDocument();
  });
});
