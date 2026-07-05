import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import { ModeSelect } from './ModeSelect';

// ---------------------------------------------------------------------------
// Probe screens — render the target route + capture location.state so tests
// can assert both the navigate target AND the state it carries.
// ---------------------------------------------------------------------------

function SearchProbe() {
  const location = useLocation();
  const state = location.state as { mode?: string; trackId?: string } | null;
  return (
    <div data-testid="search-probe">
      <span data-testid="search-mode">{state?.mode}</span>
      <span data-testid="search-track-id">{state?.trackId}</span>
    </div>
  );
}

function InviteProbe() {
  return <div data-testid="invite-probe" />;
}

function KothProbe() {
  return <div data-testid="koth-probe" />;
}

function renderModeSelect(trackId?: string) {
  return render(
    <MemoryRouter
      initialEntries={[{ pathname: '/mode-select', state: trackId ? { trackId } : undefined }]}
    >
      <Routes>
        <Route path="/mode-select" element={<ModeSelect />} />
        <Route path="/search" element={<SearchProbe />} />
        <Route path="/invite" element={<InviteProbe />} />
        <Route path="/koth" element={<KothProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('ModeSelect', () => {
  // criterion: 1 — the screen renders all four selectable options.
  it('criterion-1: renders four selectable options — Ranked, Unranked, Invite a friend, King of the Hill', () => {
    renderModeSelect();

    expect(screen.getByTestId('mode-select-screen')).toBeInTheDocument();
    expect(screen.getByTestId('mode-ranked')).toHaveTextContent('Ranked');
    expect(screen.getByTestId('mode-unranked')).toHaveTextContent('Unranked');
    expect(screen.getByTestId('mode-invite')).toHaveTextContent('Invite a friend');
    expect(screen.getByTestId('mode-koth')).toHaveTextContent('King of the Hill');
  });

  // criterion: 1 (violation guard) — a screen missing one of the four options would fail this.
  it('criterion-1 violation guard: exactly four options are present, not fewer', () => {
    renderModeSelect();

    const options = screen.getAllByRole('button');
    expect(options).toHaveLength(4);
  });

  // Table-driven cases for criteria 2 & 3 — each row selects an option and asserts the resulting
  // navigate target (+ router state where applicable).
  const routingCases: {
    name: string;
    testId: string;
    expectedProbe: string;
    expectedMode?: string;
  }[] = [
    {
      name: 'criterion-2: selecting Ranked navigates to /search with { mode: "ranked" }',
      testId: 'mode-ranked',
      expectedProbe: 'search-probe',
      expectedMode: 'ranked',
    },
    {
      name: 'criterion-2: selecting Unranked navigates to /search with { mode: "unranked" }',
      testId: 'mode-unranked',
      expectedProbe: 'search-probe',
      expectedMode: 'unranked',
    },
    {
      name: 'criterion-3: selecting Invite a friend navigates to /invite',
      testId: 'mode-invite',
      expectedProbe: 'invite-probe',
    },
    {
      name: 'criterion-3: selecting King of the Hill navigates to /koth',
      testId: 'mode-koth',
      expectedProbe: 'koth-probe',
    },
  ];

  it.each(routingCases)('$name', ({ testId, expectedProbe, expectedMode }) => {
    renderModeSelect();

    fireEvent.click(screen.getByTestId(testId));

    expect(screen.getByTestId(expectedProbe)).toBeInTheDocument();
    if (expectedMode !== undefined) {
      expect(screen.getByTestId('search-mode').textContent).toBe(expectedMode);
    }
  });

  // criterion: 2 (violation guard) — Ranked and Unranked must carry DIFFERENT mode values; if the
  // implementation hard-coded one mode for both buttons this test would fail.
  it('criterion-2 violation guard: Ranked and Unranked hand off distinct mode values', () => {
    const { unmount } = renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-ranked'));
    const rankedMode = screen.getByTestId('search-mode').textContent;
    unmount();

    renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-unranked'));
    const unrankedMode = screen.getByTestId('search-mode').textContent;

    expect(rankedMode).toBe('ranked');
    expect(unrankedMode).toBe('unranked');
    expect(rankedMode).not.toBe(unrankedMode);
  });

  // criterion: 3 (violation guard) — Invite and King of the Hill must route to DIFFERENT screens;
  // if both mapped to the same placeholder this would fail.
  it('criterion-3 violation guard: Invite a friend and King of the Hill route to different screens', () => {
    const { unmount } = renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-invite'));
    expect(screen.getByTestId('invite-probe')).toBeInTheDocument();
    expect(screen.queryByTestId('koth-probe')).not.toBeInTheDocument();
    unmount();

    renderModeSelect();
    fireEvent.click(screen.getByTestId('mode-koth'));
    expect(screen.getByTestId('koth-probe')).toBeInTheDocument();
    expect(screen.queryByTestId('invite-probe')).not.toBeInTheDocument();
  });

  // criterion: 4 (#159) — the trackId carried in via Home's Play link is forwarded onward to
  // /search for both the ranked and unranked branches.
  it.each([
    { name: 'criterion-4/#159: Ranked forwards trackId to /search', testId: 'mode-ranked' },
    { name: 'criterion-4/#159: Unranked forwards trackId to /search', testId: 'mode-unranked' },
  ])('$name', ({ testId }) => {
    renderModeSelect('track-3');

    fireEvent.click(screen.getByTestId(testId));

    expect(screen.getByTestId('search-track-id').textContent).toBe('track-3');
  });

  // criterion: 4 (#159) violation guard — with NO trackId in location.state, /search must receive
  // an EMPTY trackId (not some hard-coded default) — the field is genuinely being threaded through,
  // not synthesized.
  it('criterion-4/#159 violation guard: with no trackId in location.state, /search receives none', () => {
    renderModeSelect(undefined);

    fireEvent.click(screen.getByTestId('mode-ranked'));

    expect(screen.getByTestId('search-track-id').textContent).toBe('');
  });
});
