import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { PointsWidget } from './PointsWidget';
import type { PointsApi, PointsBalance } from '../api/points';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makePointsApi(data: PointsBalance): PointsApi {
  return { getBalance: vi.fn().mockResolvedValue(data) };
}

function makePointsApiError(message = 'fetch failed'): PointsApi {
  return { getBalance: vi.fn().mockRejectedValue(new Error(message)) };
}

function makePointsApiPending(): PointsApi {
  return { getBalance: vi.fn().mockReturnValue(new Promise(() => {})) };
}

afterEach(() => {
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// Criterion 1 — balance render + loading/error placeholder, never crashes
// ---------------------------------------------------------------------------

describe('Criterion 1 — points balance', () => {
  it('criterion-1: renders the balance number once getBalance resolves', async () => {
    // criterion: 1 — a resolved balance must be shown as data-testid=points-balance
    const api = makePointsApi({ balance: 1234 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => {
      expect(screen.getByTestId('points-balance')).toHaveTextContent('1234');
    });
    expect(screen.queryByTestId('points-placeholder')).not.toBeInTheDocument();
  });

  it('criterion-1: shows a neutral placeholder while loading (never the raw number)', () => {
    // criterion: 1 — before the promise resolves the widget must show a loading placeholder
    const api = makePointsApiPending();
    render(<PointsWidget pointsApi={api} />);

    expect(screen.getByTestId('points-placeholder')).toBeInTheDocument();
    expect(screen.queryByTestId('points-balance')).not.toBeInTheDocument();
  });

  it('criterion-1: shows a neutral placeholder on API error and never crashes', async () => {
    // criterion: 1 — a rejected getBalance must degrade to a placeholder, not throw or show a raw error
    const api = makePointsApiError('Network Error');
    expect(() => render(<PointsWidget pointsApi={api} />)).not.toThrow();

    await waitFor(() => {
      expect(screen.getByTestId('points-placeholder')).toBeInTheDocument();
    });
    expect(screen.queryByTestId('points-balance')).not.toBeInTheDocument();
    // The raw error message must never leak into the DOM.
    expect(screen.queryByText('Network Error')).not.toBeInTheDocument();
  });

  it('criterion-1 violation guard: the icon element renders regardless of loading state', () => {
    // criterion: 1 — the placeholder icon element must always be present alongside the widget
    const api = makePointsApiPending();
    render(<PointsWidget pointsApi={api} />);

    expect(screen.getByTestId('points-icon')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 2 — click-to-open info panel with static earn/spend/hold text
// ---------------------------------------------------------------------------

describe('Criterion 2 — info panel', () => {
  it('criterion-2: the info panel is closed by default', async () => {
    // criterion: 2 — panel must not be in the document until the widget is clicked
    const api = makePointsApi({ balance: 10 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    expect(screen.queryByTestId('points-info-panel')).not.toBeInTheDocument();
  });

  it('criterion-2: clicking the widget opens the panel showing earn/spend/hold text', async () => {
    // criterion: 2 — clicking must reveal earn list, spend text and hold/airdrop text
    const api = makePointsApi({ balance: 10 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('points-widget'));

    const panel = screen.getByTestId('points-info-panel');
    expect(panel).toBeInTheDocument();
    expect(panel).toHaveTextContent(/winning a match/i);
    expect(panel).toHaveTextContent(/leveling up in ranked play/i);
    expect(panel).toHaveTextContent(/in-game store/i);
    expect(panel).toHaveTextContent(/possible future airdrop/i);
    expect(panel).toHaveTextContent(/pro-rata/i);
    expect(panel).toHaveTextContent(/no guarantees/i);
  });

  it('criterion-2: clicking the widget again (toggle) closes the panel', async () => {
    // criterion: 2 — there must be a way to close the panel (toggle behaviour)
    const api = makePointsApi({ balance: 10 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    const widget = screen.getByTestId('points-widget');
    fireEvent.click(widget);
    expect(screen.getByTestId('points-info-panel')).toBeInTheDocument();

    fireEvent.click(widget);
    expect(screen.queryByTestId('points-info-panel')).not.toBeInTheDocument();
  });

  it('criterion-2: the panel close control closes it', async () => {
    // criterion: 2 — an explicit close control (accessible name) must close the panel
    const api = makePointsApi({ balance: 10 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('points-widget'));
    expect(screen.getByTestId('points-info-panel')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /close points info/i }));
    expect(screen.queryByTestId('points-info-panel')).not.toBeInTheDocument();
  });

  it('criterion-2 violation guard: opening the panel does not remove the balance widget', async () => {
    // criterion: 2 guard — opening the panel is additive, the widget itself stays mounted
    const api = makePointsApi({ balance: 42 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('points-widget'));

    expect(screen.getByTestId('points-widget')).toBeInTheDocument();
    expect(screen.getByTestId('points-info-panel')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Constraint — NO concrete point numbers/figures anywhere in the info panel
// ---------------------------------------------------------------------------

describe('Constraint — no concrete numbers in the static info text', () => {
  it('criterion-2/constraint: the info panel text contains no digit characters', async () => {
    // criterion: 2 — the earn list, spend text and hold/airdrop text must have NO concrete figures
    const api = makePointsApi({ balance: 999 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('points-widget'));

    const panel = screen.getByTestId('points-info-panel');
    expect(panel.textContent).not.toMatch(/[0-9]/);
  });

  it('criterion-2/constraint: the hold/airdrop text avoids guarantee language', async () => {
    // criterion: 2 — the airdrop text must be hedged, never promise value or guarantee anything
    const api = makePointsApi({ balance: 5 });
    render(<PointsWidget pointsApi={api} />);

    await waitFor(() => expect(screen.getByTestId('points-balance')).toBeInTheDocument());
    fireEvent.click(screen.getByTestId('points-widget'));

    const panel = screen.getByTestId('points-info-panel');
    expect(panel.textContent).toMatch(/no guarantees/i);
    expect(panel.textContent).not.toMatch(/guaranteed/i);
  });
});
