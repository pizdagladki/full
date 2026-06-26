import { createContext, useContext, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { defaultAuthApi } from '../../api/auth';
import type { AuthApi, User } from '../../api/auth';
import { ApiError } from '../../api/auth';

export interface AuthState {
  user: User | null;
  loading: boolean;
  error: string | null;
  refreshUser: () => Promise<void>;
}

const noopRefresh = () => Promise.resolve();

const defaultState: AuthState = { user: null, loading: true, error: null, refreshUser: noopRefresh };

export const AuthContext = createContext<AuthState>(defaultState);

export function useAuth(): AuthState {
  return useContext(AuthContext);
}

interface AuthProviderProps {
  children: ReactNode;
  authApi?: AuthApi;
}

export function AuthProvider({ children, authApi = defaultAuthApi }: AuthProviderProps) {
  const [state, setState] = useState<Omit<AuthState, 'refreshUser'>>({
    user: null,
    loading: true,
    error: null,
  });

  useEffect(() => {
    const cancelled = { value: false };
    authApi
      .getMe()
      .then((user) => {
        if (!cancelled.value) setState({ user, loading: false, error: null });
      })
      .catch((err: unknown) => {
        if (cancelled.value) return;
        if (err instanceof ApiError && err.status === 401) {
          setState({ user: null, loading: false, error: null });
        } else {
          const msg = err instanceof Error ? err.message : 'Unknown error';
          setState({ user: null, loading: false, error: msg });
        }
      });
    return () => {
      cancelled.value = true;
    };
  }, [authApi]);

  const refreshUser = async () => {
    try {
      const user = await authApi.getMe();
      setState({ user, loading: false, error: null });
    } catch (err: unknown) {
      if (err instanceof ApiError && err.status === 401) {
        setState({ user: null, loading: false, error: null });
      } else {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        setState({ user: null, loading: false, error: msg });
      }
    }
  };

  const contextValue: AuthState = { ...state, refreshUser };

  return <AuthContext.Provider value={contextValue}>{children}</AuthContext.Provider>;
}
