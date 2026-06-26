import { createBrowserRouter, RouterProvider, Outlet } from 'react-router-dom';
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
      { index: true, element: <Landing /> },
      { path: 'register', element: <Register /> },
      { path: 'home', element: <Home /> },
      { path: 'search', element: <Search /> },
      { path: 'battle', element: <Battle /> },
      { path: 'results', element: <Results /> },
      { path: 'profile', element: <Profile /> },
      { path: 'store', element: <Store /> },
    ],
  },
];

const browserRouter = createBrowserRouter(routes);

export function App() {
  return <RouterProvider router={browserRouter} />;
}
