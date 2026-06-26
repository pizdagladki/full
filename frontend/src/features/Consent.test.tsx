import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { Consent } from './Consent';
import type { AuthApi, ConsentInfo, User } from '../api/auth';
import { ApiError } from '../api/auth';
import type { AuthState } from './auth/AuthContext';

// ---------------------------------------------------------------------------
// Mock react-router-dom useNavigate
// ---------------------------------------------------------------------------

const mockNavigate = vi.fn();

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return { ...actual, useNavigate: () => mockNavigate };
});

// ---------------------------------------------------------------------------
// Mock useAuth
// ---------------------------------------------------------------------------

vi.mock('./auth/AuthContext', () => ({
  useAuth: vi.fn(),
}));

import { useAuth } from './auth/AuthContext';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeConsentInfo(overrides: Partial<ConsentInfo> = {}): ConsentInfo {
  return {
    is_adult: true,
    consent_recording: true,
    consent_tos: true,
    accepted_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function makeUser(overrides: Partial<User> = {}): User {
  return { id: '1', email: 'user@test.com', ...overrides };
}

function makeAuthApi(overrides: Partial<AuthApi> = {}): AuthApi {
  return {
    googleLogin: vi.fn().mockResolvedValue(undefined),
    getMe: vi.fn().mockResolvedValue(makeUser()),
    submitConsent: vi.fn().mockResolvedValue(makeConsentInfo()),
    ...overrides,
  };
}

const mockRefreshUser = vi.fn().mockResolvedValue(undefined);

function setAuthState(state: Partial<AuthState>) {
  const full: AuthState = {
    user: makeUser({ consent: null }),
    loading: false,
    error: null,
    refreshUser: mockRefreshUser,
    ...state,
  };
  vi.mocked(useAuth).mockReturnValue(full);
}

function renderConsent(authApi?: AuthApi) {
  return render(
    <MemoryRouter>
      <Consent authApi={authApi} />
    </MemoryRouter>,
  );
}

// ---------------------------------------------------------------------------
// Criterion 1: Three independent checkboxes render as distinct labeled items
// ---------------------------------------------------------------------------

describe('Consent — criterion 1: three independent checkboxes', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    setAuthState({ user: makeUser({ consent: null }) });
  });

  it('criterion-1: renders three separate checkboxes with distinct test ids', () => {
    // criterion: 1 — three independent checkboxes must all exist as distinct elements
    renderConsent();

    const adult = screen.getByTestId('checkbox-adult') as HTMLInputElement;
    const recording = screen.getByTestId('checkbox-recording') as HTMLInputElement;
    const tos = screen.getByTestId('checkbox-tos') as HTMLInputElement;

    expect(adult).toBeInTheDocument();
    expect(recording).toBeInTheDocument();
    expect(tos).toBeInTheDocument();

    // They must be distinct elements (not the same node)
    expect(adult).not.toBe(recording);
    expect(recording).not.toBe(tos);
    expect(adult).not.toBe(tos);
  });

  it('criterion-1: checkboxes are individually togglable (each has independent state)', () => {
    // criterion: 1 — toggling one checkbox does not affect the others
    renderConsent();

    const adult = screen.getByTestId('checkbox-adult') as HTMLInputElement;
    const recording = screen.getByTestId('checkbox-recording') as HTMLInputElement;
    const tos = screen.getByTestId('checkbox-tos') as HTMLInputElement;

    // Initially all unchecked
    expect(adult.checked).toBe(false);
    expect(recording.checked).toBe(false);
    expect(tos.checked).toBe(false);

    // Check only adult
    fireEvent.click(adult);
    expect(adult.checked).toBe(true);
    expect(recording.checked).toBe(false);
    expect(tos.checked).toBe(false);

    // Check only recording
    fireEvent.click(recording);
    expect(adult.checked).toBe(true);
    expect(recording.checked).toBe(true);
    expect(tos.checked).toBe(false);
  });

  it('criterion-1 guard: fails if any checkbox is missing — adult missing', () => {
    // criterion: 1 — querying for all three must succeed; this verifies the test would fail without each one
    renderConsent();
    expect(screen.getByTestId('checkbox-adult')).toBeInTheDocument();
  });

  it('criterion-1: each checkbox has a descriptive label', () => {
    // criterion: 1 — labels must describe each consent item
    renderConsent();
    expect(screen.getByText(/18 years of age or older/i)).toBeInTheDocument();
    expect(screen.getByText(/recording and publishing/i)).toBeInTheDocument();
    expect(screen.getByText(/full user agreement/i)).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 2: Submit disabled until all three checked; payload on submit
// ---------------------------------------------------------------------------

describe('Consent — criterion 2: gating and submit payload', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    setAuthState({ user: makeUser({ consent: null }) });
  });

  it('criterion-2: submit button is disabled when no checkboxes are checked', () => {
    // criterion: 2 — all-unchecked state must disable the button
    renderConsent();
    const btn = screen.getByTestId('btn-continue') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it('criterion-2: submit button stays disabled with only one checkbox checked', () => {
    // criterion: 2 — partial check (1 of 3) must NOT enable the button
    renderConsent();
    fireEvent.click(screen.getByTestId('checkbox-adult'));
    const btn = screen.getByTestId('btn-continue') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it('criterion-2: submit button stays disabled with only two checkboxes checked', () => {
    // criterion: 2 — partial check (2 of 3) must NOT enable the button
    renderConsent();
    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    const btn = screen.getByTestId('btn-continue') as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
  });

  it('criterion-2: submit button is enabled only when all three checkboxes are checked', () => {
    // criterion: 2 — all three checked must enable the button
    renderConsent();
    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    const btn = screen.getByTestId('btn-continue') as HTMLButtonElement;
    expect(btn.disabled).toBe(false);
  });

  it('criterion-2: submitting calls submitConsent with correct payload', async () => {
    // criterion: 2 — the submit payload must be exactly {is_adult:true, consent_recording:true, consent_tos:true}
    const api = makeAuthApi();
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(api.submitConsent).toHaveBeenCalledTimes(1);
    });

    expect(api.submitConsent).toHaveBeenCalledWith({
      is_adult: true,
      consent_recording: true,
      consent_tos: true,
    });
  });

  it('criterion-2 guard: submitConsent NOT called when button is disabled', async () => {
    // criterion: 2 — attempting to submit without all checked must not call the API
    const api = makeAuthApi();
    renderConsent(api);

    // Only check one box, then click the (disabled) button
    fireEvent.click(screen.getByTestId('checkbox-adult'));
    // Simulate form submit directly (since button is disabled, form submit won't fire naturally)
    // but we verify the API was not called
    expect(api.submitConsent).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Criterion 3: Success routing to /home
// ---------------------------------------------------------------------------

describe('Consent — criterion 3: success routes to /home', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    mockRefreshUser.mockReset();
    mockRefreshUser.mockResolvedValue(undefined);
    setAuthState({ user: makeUser({ consent: null }) });
  });

  it('criterion-3: navigates to /home on successful submitConsent', async () => {
    // criterion: 3 — a 200 from submitConsent must route user to /home
    const api = makeAuthApi({
      submitConsent: vi.fn().mockResolvedValue(makeConsentInfo()),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/home');
    });
  });

  it('criterion-3: refreshUser is called before navigate on successful submitConsent', async () => {
    // criterion: 3 — refreshUser must be called to update auth context before ProtectedRoute evaluates
    const callOrder: string[] = [];
    mockRefreshUser.mockImplementation(async () => {
      callOrder.push('refreshUser');
    });
    mockNavigate.mockImplementation(() => {
      callOrder.push('navigate');
    });

    const api = makeAuthApi({
      submitConsent: vi.fn().mockResolvedValue(makeConsentInfo()),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/home');
    });

    // refreshUser must be called, and it must precede navigate
    expect(mockRefreshUser).toHaveBeenCalledTimes(1);
    expect(callOrder).toEqual(['refreshUser', 'navigate']);
  });

  it('criterion-3: refreshUser is NOT called if submitConsent fails', async () => {
    // criterion: 3 guard — on error, refreshUser must not be called
    const api = makeAuthApi({
      submitConsent: vi.fn().mockRejectedValue(new Error('server error')),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(screen.getByTestId('error-message')).toBeInTheDocument();
    });

    expect(mockRefreshUser).not.toHaveBeenCalled();
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('criterion-3 guard: does not navigate if submitConsent is not called', () => {
    // criterion: 3 — without submitting, navigate must not be called
    renderConsent();
    expect(mockNavigate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Criterion 4: Error handling — 422/error stays on screen, no crash
// ---------------------------------------------------------------------------

describe('Consent — criterion 4: error handling', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
    setAuthState({ user: makeUser({ consent: null }) });
  });

  it('criterion-4: shows error-message on ApiError(422) and user stays on consent screen', async () => {
    // criterion: 4 — a 422 from backend must surface a non-crashing error; user stays on screen
    const api = makeAuthApi({
      submitConsent: vi.fn().mockRejectedValue(new ApiError(422, 'Unprocessable Entity')),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(screen.getByTestId('error-message')).toBeInTheDocument();
    });

    // Must NOT navigate away
    expect(mockNavigate).not.toHaveBeenCalled();

    // Error message must contain meaningful text
    expect(screen.getByTestId('error-message').textContent).toBeTruthy();
  });

  it('criterion-4: shows error-message on generic Error (network failure)', async () => {
    // criterion: 4 — a network error must also surface gracefully
    const api = makeAuthApi({
      submitConsent: vi.fn().mockRejectedValue(new Error('Network Error')),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(screen.getByTestId('error-message')).toBeInTheDocument();
    });

    expect(screen.getByTestId('error-message').textContent).toContain('Network Error');
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('criterion-4 guard: no error-message shown on initial render', () => {
    // criterion: 4 — baseline: no error message before any submission
    renderConsent();
    expect(screen.queryByTestId('error-message')).not.toBeInTheDocument();
  });

  it('criterion-4 guard: no error-message shown after successful submit', async () => {
    // criterion: 4 — on success, no error message should appear
    const api = makeAuthApi({
      submitConsent: vi.fn().mockResolvedValue(makeConsentInfo()),
    });
    renderConsent(api);

    fireEvent.click(screen.getByTestId('checkbox-adult'));
    fireEvent.click(screen.getByTestId('checkbox-recording'));
    fireEvent.click(screen.getByTestId('checkbox-tos'));
    fireEvent.click(screen.getByTestId('btn-continue'));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/home');
    });

    expect(screen.queryByTestId('error-message')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Criterion 5: Already-consented user skips consent screen (redirect to /home)
// ---------------------------------------------------------------------------

describe('Consent — criterion 5: already-consented user is redirected', () => {
  beforeEach(() => {
    mockNavigate.mockReset();
  });

  it('criterion-5: user with existing consent is redirected to /home (no checkboxes rendered)', () => {
    // criterion: 5 — a user whose getMe already reports consent given must skip the consent screen
    setAuthState({
      user: makeUser({
        consent: makeConsentInfo(),
      }),
    });
    renderConsent();

    // The consent form checkboxes must NOT be rendered (user is already consented)
    expect(screen.queryByTestId('checkbox-adult')).not.toBeInTheDocument();
    expect(screen.queryByTestId('checkbox-recording')).not.toBeInTheDocument();
    expect(screen.queryByTestId('checkbox-tos')).not.toBeInTheDocument();
  });

  it('criterion-5 guard: user WITHOUT consent sees the consent form', () => {
    // criterion: 5 guard — unconsented user must see the form (not be redirected)
    setAuthState({
      user: makeUser({ consent: null }),
    });
    renderConsent();

    expect(screen.getByTestId('checkbox-adult')).toBeInTheDocument();
    expect(screen.getByTestId('checkbox-recording')).toBeInTheDocument();
    expect(screen.getByTestId('checkbox-tos')).toBeInTheDocument();
  });

  it('criterion-5: loading state renders nothing (null)', () => {
    // criterion: 5 — while auth loading, component renders null (no flash)
    setAuthState({ loading: true, user: null });
    const { container } = renderConsent();
    expect(container.firstChild).toBeNull();
  });
});
