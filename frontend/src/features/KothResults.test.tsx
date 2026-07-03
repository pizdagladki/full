import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import { KothResults } from './KothResults';

// ---------------------------------------------------------------------------
// Probe screens the results screen's nav buttons should land on.
// ---------------------------------------------------------------------------

function BattleProbe() {
  const location = useLocation();
  const state = location.state as { hillType?: string } | null;
  return (
    <div data-testid="battle-probe">
      <span data-testid="battle-probe-hillType">{state?.hillType}</span>
    </div>
  );
}

function KothPlaceholderProbe() {
  return <div data-testid="koth-placeholder-probe">koth home</div>;
}

function renderResults(state: unknown) {
  return render(
    <MemoryRouter initialEntries={[{ pathname: '/koth/results', state }]}>
      <Routes>
        <Route path="/koth/results" element={<KothResults />} />
        <Route path="/koth/battle" element={<BattleProbe />} />
        <Route path="/koth" element={<KothPlaceholderProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('KothResults', () => {
  // criterion: 4 — daily/monthly win renders "new King" messaging.
  it('daily-won-renders-new-king: won:true for daily renders the new-king message', () => {
    renderResults({ hillType: 'daily', won: true, survivedMs: 500 });

    expect(screen.getByTestId('koth-won')).toHaveTextContent('You are the new King!');
    expect(screen.queryByTestId('koth-lost')).not.toBeInTheDocument();
  });

  // criterion: 4 (violation guard) — a loss must render the "undefeated" message, not the win one.
  it('daily-won-renders-new-king violation guard: won:false renders the undefeated message instead', () => {
    renderResults({ hillType: 'monthly', won: false, survivedMs: 200 });

    expect(screen.getByTestId('koth-lost')).toHaveTextContent('The king remains undefeated');
    expect(screen.queryByTestId('koth-won')).not.toBeInTheDocument();
  });

  // criterion: 4 — a neutral error state renders instead of crashing when KothBattle degraded.
  it('error-state-renders-neutral-message: an error outcome renders a neutral message, not a crash', () => {
    expect(() => renderResults({ hillType: 'daily', error: true })).not.toThrow();

    expect(screen.getByTestId('koth-error')).toBeInTheDocument();
    expect(screen.queryByTestId('koth-won')).not.toBeInTheDocument();
    expect(screen.queryByTestId('koth-lost')).not.toBeInTheDocument();
  });

  // criterion: 4 — ranked renders the achieved/current rank info.
  it('ranked-renders-rank-reached: newlyReached true renders the rank-reached message', () => {
    renderResults({
      hillType: 'ranked',
      achievedRank: 3,
      currentRank: 5,
      newlyReached: true,
    });

    expect(screen.getByTestId('koth-rank-reached')).toHaveTextContent('Reached rank 3!');
    expect(screen.queryByTestId('koth-rank-current')).not.toBeInTheDocument();
  });

  // criterion: 4 (violation guard) — newlyReached false must show current-rank, not rank-reached.
  it('ranked-renders-rank-reached violation guard: newlyReached false renders current-rank instead', () => {
    renderResults({
      hillType: 'ranked',
      achievedRank: 6,
      currentRank: 6,
      newlyReached: false,
    });

    expect(screen.getByTestId('koth-rank-current')).toHaveTextContent('Current rank: 6');
    expect(screen.queryByTestId('koth-rank-reached')).not.toBeInTheDocument();
  });

  // criterion: 4 — a ranked sanity-check failure (no attempt) renders a neutral message.
  it('ranked-no-attempt-renders-neutral: noAttempt renders a neutral "no attempt" message', () => {
    renderResults({ hillType: 'ranked', noAttempt: true });

    expect(screen.getByTestId('koth-no-attempt')).toHaveTextContent('No attempt recorded');
    expect(screen.queryByTestId('koth-rank-reached')).not.toBeInTheDocument();
    expect(screen.queryByTestId('koth-rank-current')).not.toBeInTheDocument();
  });

  // criterion: 4 — the rewards section is always a neutral placeholder (issue #108 not wired yet).
  it('rewards-placeholder-renders: a neutral rewards placeholder always renders', () => {
    renderResults({ hillType: 'daily', won: true });

    expect(screen.getByTestId('rewards-placeholder')).toBeInTheDocument();
  });

  // criterion: 4 — distractions are NOT available in solo mode: no distraction-related control
  // must ever be rendered on this screen.
  it('no-distraction-control-rendered: no element with a distraction-related test id exists', () => {
    renderResults({ hillType: 'daily', won: true });

    const distractionEls = document.querySelectorAll('[data-testid*="distraction" i]');
    expect(distractionEls.length).toBe(0);
  });

  // criterion: 4 — "Play again" navigates back to /koth/battle carrying the same hillType.
  it('play-again-navigates-to-battle: clicking play-again routes to /koth/battle with the same hillType', () => {
    renderResults({ hillType: 'ranked', noAttempt: true });

    fireEvent.click(screen.getByTestId('play-again'));

    expect(screen.getByTestId('battle-probe')).toBeInTheDocument();
    expect(screen.getByTestId('battle-probe-hillType').textContent).toBe('ranked');
  });

  // criterion: 4 — "Back" navigates to /koth (the hill-select screen owned by #110).
  it('back-navigates-to-koth: clicking back routes to /koth', () => {
    renderResults({ hillType: 'daily', won: false });

    fireEvent.click(screen.getByTestId('back-to-koth'));

    expect(screen.getByTestId('koth-placeholder-probe')).toBeInTheDocument();
  });

  // criterion: default hillType — missing location.state must not crash and defaults to daily.
  it('missing-state-defaults-to-daily: no location.state at all falls back gracefully', () => {
    expect(() =>
      render(
        <MemoryRouter initialEntries={['/koth/results']}>
          <Routes>
            <Route path="/koth/results" element={<KothResults />} />
          </Routes>
        </MemoryRouter>,
      ),
    ).not.toThrow();

    expect(screen.getByTestId('koth-lost')).toBeInTheDocument();
  });
});
