import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { KothRanked } from './KothRanked';
import type { KothApi, RankCount, RankedMeResult } from '../api/koth';

// ---------------------------------------------------------------------------
// Fake koth API doubles
// ---------------------------------------------------------------------------

function makeKothApi(
  options: {
    leaderboard?: RankCount[] | Error;
    me?: RankedMeResult | Error;
  } = {},
): KothApi {
  const { leaderboard = [], me = { current_rank: 1, next_target_ms: 1000 } } = options;
  return {
    challengeHill: vi.fn(),
    submitRankedAttempt: vi.fn(),
    getKing: vi.fn(),
    getRankedLeaderboard:
      leaderboard instanceof Error
        ? vi.fn().mockRejectedValue(leaderboard)
        : vi.fn().mockResolvedValue(leaderboard),
    getRankedMe: me instanceof Error ? vi.fn().mockRejectedValue(me) : vi.fn().mockResolvedValue(me),
  };
}

function makePendingKothApi(): KothApi {
  return {
    challengeHill: vi.fn(),
    submitRankedAttempt: vi.fn(),
    getKing: vi.fn(),
    getRankedLeaderboard: vi.fn().mockReturnValue(new Promise(() => {})),
    getRankedMe: vi.fn().mockReturnValue(new Promise(() => {})),
  };
}

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

function renderRanked(kothApi: KothApi) {
  return render(
    <MemoryRouter initialEntries={['/koth/ranked']}>
      <Routes>
        <Route path="/koth/ranked" element={<KothRanked kothApi={kothApi} />} />
        <Route path="/koth/battle" element={<BattleProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('KothRanked', () => {
  // criterion: both sections show their own loading state.
  it('criterion-loading: both leaderboard and me sections show their own loading placeholder', () => {
    const kothApi = makePendingKothApi();
    renderRanked(kothApi);

    expect(screen.getByTestId('koth-ranked-screen')).toBeInTheDocument();
    expect(screen.getByTestId('ranked-leaderboard-loading')).toBeInTheDocument();
    expect(screen.getByTestId('ranked-me-loading')).toBeInTheDocument();
  });

  // criterion: leaderboard success renders each {rank, count} row.
  it('criterion-leaderboard-success: renders each rank/count row from the leaderboard', async () => {
    const kothApi = makeKothApi({
      leaderboard: [
        { rank: 1, count: 3 },
        { rank: 2, count: 10 },
      ],
    });
    renderRanked(kothApi);

    await waitFor(() => expect(screen.getByTestId('rank-row-1')).toBeInTheDocument());
    expect(screen.getByTestId('rank-row-1').textContent).toContain('3');
    expect(screen.getByTestId('rank-row-2').textContent).toContain('10');
  });

  // criterion: leaderboard empty array renders the neutral empty placeholder.
  it('criterion-leaderboard-empty: an empty leaderboard array renders the empty placeholder', async () => {
    const kothApi = makeKothApi({ leaderboard: [] });
    renderRanked(kothApi);

    await waitFor(() =>
      expect(screen.getByTestId('ranked-leaderboard-empty')).toBeInTheDocument(),
    );
    expect(screen.queryByTestId('rank-row-1')).not.toBeInTheDocument();
  });

  // criterion: leaderboard fetch rejection renders a neutral placeholder, never crashes.
  it('criterion-leaderboard-error: a rejected leaderboard fetch renders a neutral error placeholder', async () => {
    const kothApi = makeKothApi({ leaderboard: new Error('down') });
    expect(() => renderRanked(kothApi)).not.toThrow();

    await waitFor(() =>
      expect(screen.getByTestId('ranked-leaderboard-error')).toBeInTheDocument(),
    );
  });

  // criterion: me success renders current_rank + next_target_ms.
  it('criterion-me-success: renders current_rank and next_target_ms on success', async () => {
    const kothApi = makeKothApi({ me: { current_rank: 5, next_target_ms: 4200 } });
    renderRanked(kothApi);

    await waitFor(() => expect(screen.getByTestId('ranked-me-current')).toBeInTheDocument());
    expect(screen.getByTestId('ranked-me-current').textContent).toContain('5');
    expect(screen.getByTestId('ranked-me-target').textContent).toContain('4200');
  });

  // criterion: me fetch rejection renders a neutral placeholder, never crashes.
  it('criterion-me-error: a rejected me fetch renders a neutral error placeholder', async () => {
    const kothApi = makeKothApi({ me: new Error('down') });
    expect(() => renderRanked(kothApi)).not.toThrow();

    await waitFor(() => expect(screen.getByTestId('ranked-me-error')).toBeInTheDocument());
  });

  // criterion (independence guard): leaderboard succeeds while me fails — one must not block the
  // other.
  it('criterion-independence: leaderboard succeeds while me fails, without blocking either section', async () => {
    const kothApi = makeKothApi({
      leaderboard: [{ rank: 1, count: 2 }],
      me: new Error('me down'),
    });
    renderRanked(kothApi);

    await waitFor(() => expect(screen.getByTestId('rank-row-1')).toBeInTheDocument());
    await waitFor(() => expect(screen.getByTestId('ranked-me-error')).toBeInTheDocument());
    expect(screen.queryByTestId('ranked-leaderboard-error')).not.toBeInTheDocument();
  });

  // criterion (independence guard): me succeeds while leaderboard fails — the reverse case.
  it('criterion-independence-reverse: me succeeds while leaderboard fails, without blocking either section', async () => {
    const kothApi = makeKothApi({
      leaderboard: new Error('leaderboard down'),
      me: { current_rank: 2, next_target_ms: 900 },
    });
    renderRanked(kothApi);

    await waitFor(() =>
      expect(screen.getByTestId('ranked-leaderboard-error')).toBeInTheDocument(),
    );
    await waitFor(() => expect(screen.getByTestId('ranked-me-current')).toBeInTheDocument());
    expect(screen.queryByTestId('ranked-me-error')).not.toBeInTheDocument();
  });

  // criterion: the play control navigates to /koth/battle with hillType ranked.
  it('criterion-play: the play control navigates to /koth/battle with hillType ranked', async () => {
    const kothApi = makeKothApi();
    renderRanked(kothApi);

    await waitFor(() => expect(screen.getByTestId('ranked-leaderboard-empty')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('ranked-play'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-hillType').textContent).toBe('ranked');
  });
});
