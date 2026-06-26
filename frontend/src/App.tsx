import { BrowserRouter, Routes, Route, Outlet } from 'react-router-dom';
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

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Layout />}>
          <Route index element={<Landing />} />
          <Route path="register" element={<Register />} />
          <Route path="home" element={<Home />} />
          <Route path="search" element={<Search />} />
          <Route path="battle" element={<Battle />} />
          <Route path="results" element={<Results />} />
          <Route path="profile" element={<Profile />} />
          <Route path="store" element={<Store />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
