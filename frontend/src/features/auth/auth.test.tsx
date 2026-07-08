import { StrictMode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Login } from './Login';
import { ProtectedRoute } from './ProtectedRoute';
import { AuthContext } from './AuthContext';
import type { AuthState } from './AuthContext';
import type { AuthApi, User } from '../../api/auth';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAuthApi(overrides: Partial<AuthApi> = {}): AuthApi {
  return {
    googleLogin: vi.fn().mockResolvedValue(undefined),
    getMe: vi.fn().mockResolvedValue({ id: '1', email: 'test@test.com' } satisfies User),
    submitConsent: vi.fn().mockResolvedValue({
      is_adult: true,
      consent_recording: true,
      consent_tos: true,
      accepted_at: '2026-01-01T00:00:00Z',
    }),
    ...overrides,
  };
}

function renderLogin(
  { search = '', authApi }: { search?: string; authApi?: AuthApi } = {},
  authState: AuthState = { user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) },
) {
  const router = createMemoryRouter(
    [
      { path: '/', element: <AuthContext.Provider value={authState}><Login authApi={authApi} /></AuthContext.Provider> },
      { path: '/home', element: <div>Home</div> },
    ],
    { initialEntries: [search ? `/${search}` : '/'] },
  );
  return render(<RouterProvider router={router} />);
}

// ---------------------------------------------------------------------------
// criterion 1: Login URL construction
// ---------------------------------------------------------------------------

describe('Login — URL construction', () => {
  it('criterion-1: sign-in link contains correct Google authorize URL with client_id and redirect_uri', () => {
    // criterion: 1 — the login screen link contains the OAuth URL built from env vars
    vi.stubEnv('VITE_GOOGLE_CLIENT_ID', 'test-client-id');
    vi.stubEnv('VITE_GOOGLE_REDIRECT_URI', 'http://localhost:5173/auth/callback');

    renderLogin();

    const link = screen.getByTestId('google-signin-link') as HTMLAnchorElement;
    expect(link.href).toContain('accounts.google.com/o/oauth2/v2/auth');
    expect(link.href).toContain('client_id=test-client-id');
    expect(link.href).toContain(encodeURIComponent('http://localhost:5173/auth/callback'));
    expect(link.href).toContain('response_type=code');
    expect(link.href).toContain('scope=openid');
  });

  it('criterion-1 guard: fails when env vars are not set (link does not contain a client_id)', () => {
    // criterion: 1 — this verifies the URL building is actually reading env vars (not hardcoded)
    vi.stubEnv('VITE_GOOGLE_CLIENT_ID', '');
    vi.stubEnv('VITE_GOOGLE_REDIRECT_URI', '');

    renderLogin();

    const link = screen.getByTestId('google-signin-link') as HTMLAnchorElement;
    // When env vars are empty the client_id param should be empty string, not a real id
    expect(link.href).not.toContain('client_id=test-client-id');
  });
});

// ---------------------------------------------------------------------------
// criterion 2: code-exchange success → navigate to /home
// ---------------------------------------------------------------------------

describe('Login — code exchange success', () => {
  beforeEach(() => {
    vi.unstubAllEnvs();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    window.sessionStorage.clear();
  });

  it('criterion-2: exchanges code for session and navigates to /home on success', async () => {
    // criterion: 2 — on 200 from googleLogin + getMe, app routes to /home
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const api = makeAuthApi();
    renderLogin({ search: '?code=test_code&state=test-state', authApi: api });

    // Initially shows loading
    expect(screen.getByText('Loading...')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
    });

    expect(api.googleLogin).toHaveBeenCalledWith('test_code');
    expect(api.getMe).toHaveBeenCalledTimes(1);
  });

  it('criterion-2 guard: does not navigate when googleLogin is not called', async () => {
    // criterion: 2 — without code param no exchange occurs and /home is not shown
    const api = makeAuthApi();
    renderLogin({ authApi: api });

    await waitFor(() => {
      expect(screen.getByText('Sign in with Google')).toBeInTheDocument();
    });

    expect(api.googleLogin).not.toHaveBeenCalled();
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 2+4: 401 handling
// ---------------------------------------------------------------------------

describe('Login — 401 handling', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    window.sessionStorage.clear();
  });

  it('criterion-2-4: shows error and stays unauthenticated on 401 from googleLogin', async () => {
    // criterion: 2+4 — 401 from googleLogin shows an error; user stays on login page
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const { ApiError } = await import('../../api/auth');
    const api = makeAuthApi({
      googleLogin: vi.fn().mockRejectedValue(new ApiError(401, 'Unauthorized')),
    });

    renderLogin({ search: '?code=bad_code&state=test-state', authApi: api });

    expect(screen.getByText('Loading...')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain('unauthorized');
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });

  it('criterion-2-4 guard: no error shown on success path', async () => {
    // criterion: 2+4 — on success no error alert is rendered
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const api = makeAuthApi();
    renderLogin({ search: '?code=good_code&state=test-state', authApi: api });

    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
    });

    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 3: protected-route redirect when unauthenticated
// ---------------------------------------------------------------------------

describe('ProtectedRoute — redirect', () => {
  it('criterion-3: redirects unauthenticated user to / from a protected route', () => {
    // criterion: 3 — unauthenticated user visiting a protected route is redirected to login
    const unauthState: AuthState = { user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) };
    const router = createMemoryRouter(
      [
        { path: '/', element: <div>Login</div> },
        {
          path: '/protected',
          element: (
            <AuthContext.Provider value={unauthState}>
              <ProtectedRoute>
                <div>Secret</div>
              </ProtectedRoute>
            </AuthContext.Provider>
          ),
        },
      ],
      { initialEntries: ['/protected'] },
    );
    render(<RouterProvider router={router} />);

    expect(screen.queryByText('Secret')).not.toBeInTheDocument();
    expect(screen.getByText('Login')).toBeInTheDocument();
  });

  it('criterion-3: authenticated user sees the protected content', () => {
    // criterion: 3 — authenticated user with consent is NOT redirected
    const authState: AuthState = {
      user: {
        id: '1',
        email: 'u@u.com',
        consent: { is_adult: true, consent_recording: true, consent_tos: true, accepted_at: '2026-01-01T00:00:00Z' },
      },
      loading: false,
      error: null,
      refreshUser: vi.fn().mockResolvedValue(undefined),
    };
    const router = createMemoryRouter(
      [
        { path: '/', element: <div>Login</div> },
        { path: '/consent', element: <div>Consent</div> },
        {
          path: '/protected',
          element: (
            <AuthContext.Provider value={authState}>
              <ProtectedRoute>
                <div>Secret</div>
              </ProtectedRoute>
            </AuthContext.Provider>
          ),
        },
      ],
      { initialEntries: ['/protected'] },
    );
    render(<RouterProvider router={router} />);

    expect(screen.getByText('Secret')).toBeInTheDocument();
    expect(screen.queryByText('Login')).not.toBeInTheDocument();
  });

  it('criterion-3 guard: shows nothing while loading (no premature redirect)', () => {
    // criterion: 3 — while auth state is loading, ProtectedRoute renders null (no redirect yet)
    const loadingState: AuthState = { user: null, loading: true, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) };
    const router = createMemoryRouter(
      [
        { path: '/', element: <div>Login</div> },
        {
          path: '/protected',
          element: (
            <AuthContext.Provider value={loadingState}>
              <ProtectedRoute>
                <div>Secret</div>
              </ProtectedRoute>
            </AuthContext.Provider>
          ),
        },
      ],
      { initialEntries: ['/protected'] },
    );
    render(<RouterProvider router={router} />);

    expect(screen.queryByText('Secret')).not.toBeInTheDocument();
    expect(screen.queryByText('Login')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 4: typed api methods + network failure → non-crashing error
// ---------------------------------------------------------------------------

describe('Login — network failure', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    window.sessionStorage.clear();
  });

  it('criterion-4: non-crashing error state on network failure during code exchange', async () => {
    // criterion: 4 — network error during code exchange surfaces a non-crashing error state
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const api = makeAuthApi({
      googleLogin: vi.fn().mockRejectedValue(new Error('Network Error')),
    });

    renderLogin({ search: '?code=any_code&state=test-state', authApi: api });

    expect(screen.getByText('Loading...')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    // App did not crash — error message is shown
    const alert = screen.getByRole('alert');
    expect(alert.textContent).toContain('Network Error');
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });

  it('criterion-4 guard: without a network error no alert appears at login screen load', () => {
    // criterion: 4 — baseline: normal login screen has no error alert
    renderLogin();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 4: AuthApiClient types — googleLogin and getMe are typed methods
// ---------------------------------------------------------------------------

describe('AuthApiClient — typed API methods', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('criterion-4: googleLogin calls POST /v1/auth/google with credentials:include', async () => {
    // criterion: 4 — googleLogin uses credentials:include on the fetch call
    const { AuthApiClient } = await import('../../api/auth');
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: () => Promise.resolve({}),
    } as Response);

    const client = new AuthApiClient('http://api.test');
    await client.googleLogin('mycode');

    expect(fetchSpy).toHaveBeenCalledWith(
      'http://api.test/v1/auth/google',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        body: JSON.stringify({ code: 'mycode' }),
      }),
    );
  });

  it('criterion-4: getMe calls GET /v1/auth/me with credentials:include and returns User', async () => {
    // criterion: 4 — getMe uses credentials:include and returns typed User
    const { AuthApiClient } = await import('../../api/auth');
    const mockUser: User = { id: '42', email: 'x@x.com' };
    const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: () => Promise.resolve(mockUser),
    } as Response);

    const client = new AuthApiClient('http://api.test');
    const user = await client.getMe();

    expect(fetchSpy).toHaveBeenCalledWith(
      'http://api.test/v1/auth/me',
      expect.objectContaining({
        method: 'GET',
        credentials: 'include',
      }),
    );
    expect(user).toEqual(mockUser);
  });

  it('criterion-4: getMe throws ApiError with status 401 on unauthorized', async () => {
    // criterion: 4 — getMe surfaces a typed ApiError with status 401
    const { AuthApiClient, ApiError } = await import('../../api/auth');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: false,
      status: 401,
      statusText: 'Unauthorized',
      json: () => Promise.resolve({}),
    } as Response);

    const client = new AuthApiClient('http://api.test');
    await expect(client.getMe()).rejects.toBeInstanceOf(ApiError);

    try {
      await client.getMe();
    } catch (e) {
      expect(e instanceof ApiError && e.status).toBe(401);
    }
  });

  it('criterion-4 guard: getMe does not throw on 200 (positive baseline)', async () => {
    // criterion: 4 — getMe resolves successfully on 200
    const { AuthApiClient } = await import('../../api/auth');
    vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: () => Promise.resolve({ id: '1', email: 'ok@ok.com' }),
    } as Response);

    const client = new AuthApiClient('http://api.test');
    const user = await client.getMe();
    expect(user.id).toBe('1');
    expect(user.email).toBe('ok@ok.com');
  });
});

// ---------------------------------------------------------------------------
// AuthContext — populate auth state from getMe
// ---------------------------------------------------------------------------

describe('AuthContext — state population', () => {
  it('criterion-2: sets user in state when getMe returns 200', async () => {
    // criterion: 2 — on 200 from /me the auth state is populated with the user
    const { AuthProvider } = await import('./AuthContext');
    const { useAuth } = await import('./AuthContext');
    const mockUser: User = { id: '7', email: 'ctx@ctx.com' };
    const api = makeAuthApi({ getMe: vi.fn().mockResolvedValue(mockUser) });

    function Probe() {
      const { user, loading } = useAuth();
      if (loading) return <div>Loading...</div>;
      return <div>{user ? `user:${user.email}` : 'no-user'}</div>;
    }

    render(
      <AuthProvider authApi={api}>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('user:ctx@ctx.com')).toBeInTheDocument();
    });
  });

  it('criterion-2: user is null (unauthenticated, no error) when getMe returns 401', async () => {
    // criterion: 2 — on 401 from /me, user=null and no error (unauthenticated gracefully)
    const { AuthProvider, useAuth } = await import('./AuthContext');
    const { ApiError } = await import('../../api/auth');
    const api = makeAuthApi({
      getMe: vi.fn().mockRejectedValue(new ApiError(401, 'Unauthorized')),
    });

    function Probe() {
      const { user, loading, error } = useAuth();
      if (loading) return <div>Loading...</div>;
      return (
        <div>
          {user ? `user:${user.email}` : 'no-user'}
          {error ? ` error:${error}` : ''}
        </div>
      );
    }

    render(
      <AuthProvider authApi={api}>
        <Probe />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('no-user')).toBeInTheDocument();
    });

    expect(screen.queryByText(/error:/)).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// criterion 3: authenticated user visiting Login is redirected to /home
// ---------------------------------------------------------------------------

describe('Login — redirect authenticated user', () => {
  it('criterion-3: authenticated user visiting Login (/) is redirected to /home', async () => {
    // criterion: 3 — already-authenticated user visiting / should see /home, not the login screen
    const authState: AuthState = { user: { id: '1', email: 'u@u.com' }, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) };
    const router = createMemoryRouter(
      [
        { path: '/', element: <AuthContext.Provider value={authState}><Login /></AuthContext.Provider> },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/'] },
    );
    render(<RouterProvider router={router} />);
    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
    });
    expect(screen.queryByText('Sign in with Google')).not.toBeInTheDocument();
  });

  it('criterion-3 guard: unauthenticated user visiting Login (/) sees the login screen', () => {
    // criterion: 3 guard — unauthenticated user should NOT be redirected away from login
    const authState: AuthState = { user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) };
    const router = createMemoryRouter(
      [
        { path: '/', element: <AuthContext.Provider value={authState}><Login /></AuthContext.Provider> },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/'] },
    );
    render(<RouterProvider router={router} />);
    expect(screen.getByText('Sign in with Google')).toBeInTheDocument();
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// security: OAuth state CSRF protection
// ---------------------------------------------------------------------------

describe('Login — OAuth state CSRF protection', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    window.sessionStorage.clear();
  });

  it('security: state mismatch shows error and does not call googleLogin', async () => {
    // Simulate callback with mismatched state — CSRF attack scenario
    window.sessionStorage.setItem('oauth_state', 'expected-state');
    const api = makeAuthApi();

    const router = createMemoryRouter(
      [
        { path: '/', element: <AuthContext.Provider value={{ user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) }}><Login authApi={api} /></AuthContext.Provider> },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/?code=mycode&state=wrong-state'] },
    );
    render(<RouterProvider router={router} />);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    expect(api.googleLogin).not.toHaveBeenCalled();
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });

  it('security: matching state allows exchange to proceed', async () => {
    // Positive baseline — correct state allows the OAuth exchange to go through
    window.sessionStorage.setItem('oauth_state', 'correct-state');
    const api = makeAuthApi();

    const router = createMemoryRouter(
      [
        { path: '/', element: <AuthContext.Provider value={{ user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) }}><Login authApi={api} /></AuthContext.Provider> },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/?code=mycode&state=correct-state'] },
    );
    render(<RouterProvider router={router} />);

    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
    });

    expect(api.googleLogin).toHaveBeenCalledWith('mycode');
  });

  it('security: missing stored state shows error and does not call googleLogin', async () => {
    // No state in sessionStorage (e.g. user opened callback URL directly) — must reject
    window.sessionStorage.removeItem('oauth_state');
    const api = makeAuthApi();

    const router = createMemoryRouter(
      [
        { path: '/', element: <AuthContext.Provider value={{ user: null, loading: false, error: null, refreshUser: vi.fn().mockResolvedValue(undefined) }}><Login authApi={api} /></AuthContext.Provider> },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/?code=mycode&state=some-state'] },
    );
    render(<RouterProvider router={router} />);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
    });

    expect(api.googleLogin).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// #169 - StrictMode must not swallow the OAuth exchange result
// ---------------------------------------------------------------------------

describe('Login - StrictMode exchange (#169)', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });
  afterEach(() => {
    window.sessionStorage.clear();
  });

  function renderLoginStrict(authApi: AuthApi) {
    const authState: AuthState = {
      user: null,
      loading: false,
      error: null,
      refreshUser: vi.fn().mockResolvedValue(undefined),
    };
    const router = createMemoryRouter(
      [
        {
          path: '/',
          element: (
            <AuthContext.Provider value={authState}>
              <Login authApi={authApi} />
            </AuthContext.Provider>
          ),
        },
        { path: '/home', element: <div>Home</div> },
      ],
      { initialEntries: ['/?code=test_code&state=test-state'] },
    );
    return render(
      <StrictMode>
        <RouterProvider router={router} />
      </StrictMode>,
    );
  }

  it('a successful exchange navigates to /home under StrictMode double-mount', async () => {
    // Under StrictMode the mount effect runs setup -> synthetic cleanup -> setup; the exchange
    // is performed by the CANCELLED first run (the ref guard blocks run #2), so gating the
    // navigation on !cancelled froze a successful login on the spinner forever.
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const api = makeAuthApi();
    renderLoginStrict(api);

    await waitFor(() => {
      expect(screen.getByText('Home')).toBeInTheDocument();
    });
    // the single-use authorization code must be POSTed exactly once
    expect(api.googleLogin).toHaveBeenCalledTimes(1);
    expect(api.googleLogin).toHaveBeenCalledWith('test_code');
  });

  it('a failed exchange surfaces its error under StrictMode instead of an eternal spinner', async () => {
    window.sessionStorage.setItem('oauth_state', 'test-state');
    const api = makeAuthApi({
      googleLogin: vi.fn().mockRejectedValue(new Error('exchange blew up')),
    });
    renderLoginStrict(api);

    await waitFor(() => {
      expect(screen.getByText(/exchange blew up/)).toBeInTheDocument();
    });
    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });
});
