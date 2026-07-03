import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import { KothHillSelect } from './KothHillSelect';

// ---------------------------------------------------------------------------
// Probe screens — render the target route + capture location.state so tests
// can assert both the navigate target AND the state it carries.
// ---------------------------------------------------------------------------

function MountainProbe() {
  const location = useLocation();
  const state = location.state as { hillType?: string } | null;
  return (
    <div data-testid="mountain-probe">
      <span data-testid="mountain-hillType">{state?.hillType}</span>
    </div>
  );
}

function RankedProbe() {
  return <div data-testid="ranked-probe" />;
}

function renderHillSelect() {
  return render(
    <MemoryRouter initialEntries={['/koth']}>
      <Routes>
        <Route path="/koth" element={<KothHillSelect />} />
        <Route path="/koth/mountain" element={<MountainProbe />} />
        <Route path="/koth/ranked" element={<RankedProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('KothHillSelect', () => {
  // criterion: renders three selectable options.
  it('criterion-1: renders three selectable options — daily, monthly, ranked', () => {
    renderHillSelect();

    expect(screen.getByTestId('koth-hill-select-screen')).toBeInTheDocument();
    expect(screen.getByTestId('hill-daily')).toBeInTheDocument();
    expect(screen.getByTestId('hill-monthly')).toBeInTheDocument();
    expect(screen.getByTestId('hill-ranked')).toBeInTheDocument();
  });

  // criterion-1 (violation guard) — exactly three options, not fewer.
  it('criterion-1 violation guard: exactly three options are present, not fewer', () => {
    renderHillSelect();

    const options = screen.getAllByRole('button');
    expect(options).toHaveLength(3);
  });

  const routingCases: { name: string; testId: string; expectedHillType?: string }[] = [
    {
      name: 'criterion-2: selecting daily navigates to /koth/mountain with hillType daily',
      testId: 'hill-daily',
      expectedHillType: 'daily',
    },
    {
      name: 'criterion-2: selecting monthly navigates to /koth/mountain with hillType monthly',
      testId: 'hill-monthly',
      expectedHillType: 'monthly',
    },
  ];

  it.each(routingCases)('$name', ({ testId, expectedHillType }) => {
    renderHillSelect();

    fireEvent.click(screen.getByTestId(testId));

    expect(screen.getByTestId('mountain-probe')).toBeInTheDocument();
    expect(screen.getByTestId('mountain-hillType').textContent).toBe(expectedHillType);
  });

  // criterion: selecting ranked navigates to /koth/ranked (no hillType state needed there).
  it('criterion-3: selecting ranked navigates to /koth/ranked', () => {
    renderHillSelect();

    fireEvent.click(screen.getByTestId('hill-ranked'));

    expect(screen.getByTestId('ranked-probe')).toBeInTheDocument();
    expect(screen.queryByTestId('mountain-probe')).not.toBeInTheDocument();
  });

  // criterion-2 (violation guard) — daily and monthly must carry DIFFERENT hillType values; if the
  // implementation hard-coded one for both buttons this test would fail.
  it('criterion-2 violation guard: daily and monthly hand off distinct hillType values', () => {
    const { unmount } = renderHillSelect();
    fireEvent.click(screen.getByTestId('hill-daily'));
    const dailyHillType = screen.getByTestId('mountain-hillType').textContent;
    unmount();

    renderHillSelect();
    fireEvent.click(screen.getByTestId('hill-monthly'));
    const monthlyHillType = screen.getByTestId('mountain-hillType').textContent;

    expect(dailyHillType).toBe('daily');
    expect(monthlyHillType).toBe('monthly');
    expect(dailyHillType).not.toBe(monthlyHillType);
  });
});
