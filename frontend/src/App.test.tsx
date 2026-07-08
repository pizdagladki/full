import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import type { RouteObject } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import { routes } from './App';
import { AuthContext } from './features';
import type { AuthState } from './features';

// Provide an authenticated context so ProtectedRoute doesn't redirect.
// consent must be set so ProtectedRoute does not gate to /consent.
const authenticatedState: AuthState = {
  user: {
    id: '1',
    email: 'test@test.com',
    consent: {
      is_adult: true,
      consent_recording: true,
      consent_tos: true,
      accepted_at: '2026-01-01T00:00:00Z',
    },
  },
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

function renderWithAuth(
  routesList: RouteObject[],
  path: string,
  authState: AuthState = authenticatedState,
) {
  const router = createMemoryRouter(routesList, { initialEntries: [path] });
  return render(
    <AuthContext.Provider value={authState}>
      <RouterProvider router={router} />
    </AuthContext.Provider>,
  );
}

// Mock fetch so AuthProvider (used in full App) doesn't blow up
vi.stubGlobal(
  'fetch',
  vi.fn().mockResolvedValue({
    ok: false,
    status: 401,
    statusText: 'Unauthorized',
    json: () => Promise.resolve({}),
  }),
);

// Mock navigator.mediaDevices so Home/Search components don't crash in jsdom
Object.defineProperty(globalThis.navigator, 'mediaDevices', {
  value: {
    enumerateDevices: vi.fn().mockResolvedValue([]),
    getUserMedia: vi.fn().mockResolvedValue({ getTracks: () => [] }),
  },
  writable: true,
  configurable: true,
});

// Mock WebSocket so Search's real WsClient (no VITE_WS_URL configured in tests) doesn't crash on
// `new WebSocket('/ws/matchmaking')` (jsdom rejects relative WS URLs).
class MockWebSocket {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  send = vi.fn();
  close = vi.fn();
}
vi.stubGlobal('WebSocket', MockWebSocket);

const unauthenticatedState: AuthState = {
  user: null,
  loading: false,
  error: null,
  refreshUser: vi.fn().mockResolvedValue(undefined),
};

describe('App routes', () => {
  it('renders Login at root /', () => {
    // criterion: 1 — Login screen renders at the root route for unauthenticated users
    renderWithAuth(routes, '/', unauthenticatedState);
    expect(screen.getByText('Sign in with Google')).toBeInTheDocument();
  });

  it('renders Login at /auth/callback', () => {
    // criterion: 2 — callback path also renders Login for code exchange (unauthenticated)
    renderWithAuth(routes, '/auth/callback', unauthenticatedState);
    expect(screen.getByText('Sign in with Google')).toBeInTheDocument();
  });

  it('renders Home screen at /home when authenticated', () => {
    // criterion: 3 — authenticated user can reach protected home route
    renderWithAuth(routes, '/home');
    expect(screen.getByTestId('home-screen')).toBeInTheDocument();
  });

  it('renders Search screen at /search when authenticated', () => {
    renderWithAuth(routes, '/search');
    expect(screen.getByTestId('search-screen')).toBeInTheDocument();
  });

  it('renders the Battle screen at /battle when authenticated', () => {
    renderWithAuth(routes, '/battle');
    expect(screen.getByTestId('battle-screen')).toBeInTheDocument();
  });

  it('renders Results placeholder at /results when authenticated', () => {
    renderWithAuth(routes, '/results');
    expect(screen.getByTestId('results-screen')).toBeInTheDocument();
  });

  it('renders Profile placeholder at /profile when authenticated', () => {
    renderWithAuth(routes, '/profile');
    expect(screen.getByText('Profile')).toBeInTheDocument();
  });

  it('renders Store placeholder at /store when authenticated', () => {
    renderWithAuth(routes, '/store');
    expect(screen.getByText('Store')).toBeInTheDocument();
  });

  it('renders Register at /register', () => {
    renderWithAuth(routes, '/register');
    expect(screen.getByText('Register')).toBeInTheDocument();
  });

  it('renders layout shell with header for root route', () => {
    // criterion: 4 — base layout/shell with header is present
    renderWithAuth(routes, '/');
    expect(screen.getByRole('banner')).toBeInTheDocument();
    expect(screen.getByText('App')).toBeInTheDocument();
  });

  it('fails if Login route is missing — criterion 1 guard', () => {
    // criterion: 1 — this test fails if routes array does not include index route showing Login
    const routesWithoutLogin = routes.map((r) => ({
      ...r,
      children: r.children?.filter((c) => !('index' in c && c.index)),
    })) as RouteObject[];
    const router = createMemoryRouter(routesWithoutLogin, {
      initialEntries: ['/'],
    });
    render(
      <AuthContext.Provider value={authenticatedState}>
        <RouterProvider router={router} />
      </AuthContext.Provider>,
    );
    expect(screen.queryByText('Sign in with Google')).not.toBeInTheDocument();
  });

  it('redirects unauthenticated user from /home to / — criterion 3 guard', () => {
    // criterion: 3 — unauthenticated user visiting protected route is redirected to login
    const unauthState: AuthState = {
      user: null,
      loading: false,
      error: null,
      refreshUser: vi.fn().mockResolvedValue(undefined),
    };
    renderWithAuth(routes, '/home', unauthState);
    expect(screen.queryByTestId('home-screen')).not.toBeInTheDocument();
    expect(screen.getByText('Sign in with Google')).toBeInTheDocument();
  });
});
