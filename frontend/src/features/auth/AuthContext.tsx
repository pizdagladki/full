import { createContext, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { defaultAuthApi } from '../../api/auth';
import type { AuthApi, User } from '../../api/auth';
import { ApiError } from '../../api/auth';

export interface AuthState {
  user: User | null;
  loading: boolean;
  error: string | null;
}

const defaultState: AuthState = { user: null, loading: true, error: null };

export const AuthContext = createContext<AuthState>(defaultState);

export function useAuth(): AuthState {
  return useContext(AuthContext);
}

interface AuthProviderProps {
  children: ReactNode;
  authApi?: AuthApi;
}

export function AuthProvider({ children, authApi = defaultAuthApi }: AuthProviderProps) {
  const [state, setState] = useState<AuthState>(defaultState);

  useEffect(() => {
    let cancelled = false;
    authApi
      .getMe()
      .then((user) => {
        if (!cancelled) setState({ user, loading: false, error: null });
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          setState({ user: null, loading: false, error: null });
        } else {
          const msg = err instanceof Error ? err.message : 'Unknown error';
          setState({ user: null, loading: false, error: msg });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [authApi]);

  return <AuthContext.Provider value={state}>{children}</AuthContext.Provider>;
}
