import { createBrowserRouter, RouterProvider, Outlet, Navigate } from 'react-router-dom';
import type { RouteObject } from 'react-router-dom';
import {
  Landing,
  Register,
  Home,
  Search,
  Battle,
  Results,
  Profile,
  Store,
  AuthProvider,
  Login,
  ProtectedRoute,
} from './features';

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
        path: 'home',
        element: (
          <ProtectedRoute>
            <Home />
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
        path: 'battle',
        element: (
          <ProtectedRoute>
            <Battle />
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
