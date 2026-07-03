import { createBrowserRouter, RouterProvider, Outlet, Navigate } from 'react-router-dom';
import type { ReactNode } from 'react';
import type { RouteObject } from 'react-router-dom';
import {
  Landing,
  Register,
  Home,
  ModeSelect,
  Search,
  Battle,
  Results,
  Profile,
  Store,
  KothBattle,
  KothResults,
  KothHillSelect,
  KothMountain,
  KothRanked,
  AuthProvider,
  Login,
  ProtectedRoute,
  useAuth,
} from './features';
import { Consent } from './features/Consent';

// Placeholder screen for the route owned by future issue #106 invite-a-friend — keeps mode-select
// navigation landing on a real route instead of the `*` catch-all redirect. Replace with the real
// screen when that issue lands.
function InvitePlaceholder() {
  return <div data-testid="invite-placeholder">Invite a friend — coming soon</div>;
}

// AuthRoute: checks auth (loading/user) but does NOT check consent (avoids /consent → /consent loop)
function AuthRoute({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth();
  if (loading) return null;
  if (!user) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function Layout() {
  return (
    <>
      <header>App</header>
      <main>
        <Outlet />
      </main>
    </>
  );
}

export const routes: RouteObject[] = [
  {
    path: '/',
    element: <Layout />,
    children: [
      { index: true, element: <Login /> },
      { path: 'auth/callback', element: <Login /> },
      { path: 'landing', element: <Landing /> },
      { path: 'register', element: <Register /> },
      {
        path: 'consent',
        element: (
          <AuthRoute>
            <Consent />
          </AuthRoute>
        ),
      },
      {
        path: 'home',
        element: (
          <ProtectedRoute>
            <Home />
          </ProtectedRoute>
        ),
      },
      {
        path: 'mode-select',
        element: (
          <ProtectedRoute>
            <ModeSelect />
          </ProtectedRoute>
        ),
      },
      {
        path: 'search',
        element: (
          <ProtectedRoute>
            <Search />
          </ProtectedRoute>
        ),
      },
      {
        path: 'invite',
        element: (
          <ProtectedRoute>
            <InvitePlaceholder />
          </ProtectedRoute>
        ),
      },
      {
        path: 'koth',
        element: (
          <ProtectedRoute>
            <KothHillSelect />
          </ProtectedRoute>
        ),
      },
      {
        path: 'koth/mountain',
        element: (
          <ProtectedRoute>
            <KothMountain />
          </ProtectedRoute>
        ),
      },
      {
        path: 'koth/ranked',
        element: (
          <ProtectedRoute>
            <KothRanked />
          </ProtectedRoute>
        ),
      },
      {
        path: 'battle',
        element: (
          <ProtectedRoute>
            <Battle />
          </ProtectedRoute>
        ),
      },
      {
        path: 'koth/battle',
        element: (
          <ProtectedRoute>
            <KothBattle />
          </ProtectedRoute>
        ),
      },
      {
        path: 'koth/results',
        element: (
          <ProtectedRoute>
            <KothResults />
          </ProtectedRoute>
        ),
      },
      {
        path: 'results',
        element: (
          <ProtectedRoute>
            <Results />
          </ProtectedRoute>
        ),
      },
      {
        path: 'profile',
        element: (
          <ProtectedRoute>
            <Profile />
          </ProtectedRoute>
        ),
      },
      {
        path: 'store',
        element: (
          <ProtectedRoute>
            <Store />
          </ProtectedRoute>
        ),
      },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
];

const browserRouter = createBrowserRouter(routes);

export function App() {
  return (
    <AuthProvider>
      <RouterProvider router={browserRouter} />
    </AuthProvider>
  );
}
