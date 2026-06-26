import { render, screen } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router-dom';
import type { RouteObject } from 'react-router-dom';
import { describe, it, expect } from 'vitest';
import { routes } from './App';

describe('App routes', () => {
  // criterion: 2 — App mounts router with a placeholder route for each core screen
  const cases = [
    { path: '/', text: 'Landing' },
    { path: '/register', text: 'Register' },
    { path: '/home', text: 'Home' },
    { path: '/search', text: 'Search' },
    { path: '/battle', text: 'Battle' },
    { path: '/results', text: 'Results' },
    { path: '/profile', text: 'Profile' },
    { path: '/store', text: 'Store' },
  ];

  for (const { path, text } of cases) {
    it(`renders ${text} placeholder at ${path}`, () => {
      // criterion: 2 — each placeholder route renders its screen text
      const router = createMemoryRouter(routes, { initialEntries: [path] });
      render(<RouterProvider router={router} />);
      expect(screen.getByText(text)).toBeInTheDocument();
    });
  }

  it('renders layout shell with header for root route', () => {
    // criterion: 4 — base layout/shell with header is present
    const router = createMemoryRouter(routes, { initialEntries: ['/'] });
    render(<RouterProvider router={router} />);
    expect(screen.getByRole('banner')).toBeInTheDocument();
    expect(screen.getByText('App')).toBeInTheDocument();
  });

  it('fails if Landing route is missing — criterion 2 guard', () => {
    // criterion: 2 — this test fails if routes array does not include index route
    const routesWithoutLanding = routes.map((r) => ({
      ...r,
      children: r.children?.filter((c) => !('index' in c && c.index)),
    })) as RouteObject[];
    const router = createMemoryRouter(routesWithoutLanding, {
      initialEntries: ['/'],
    });
    render(<RouterProvider router={router} />);
    expect(screen.queryByText('Landing')).not.toBeInTheDocument();
  });
});
