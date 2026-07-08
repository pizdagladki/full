import { useEffect, useRef, useState } from 'react';
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { defaultAuthApi } from '../../api/auth';
import type { AuthApi } from '../../api/auth';
import { ApiError } from '../../api/auth';
import { useAuth } from './AuthContext';

function generateState(): string {
  return crypto.randomUUID();
}

function buildGoogleAuthUrl(): { url: string; state: string } {
  const clientId = (import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined) ?? '';
  const redirectUri = (import.meta.env.VITE_GOOGLE_REDIRECT_URI as string | undefined) ?? '';
  const state = generateState();
  sessionStorage.setItem('oauth_state', state);
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: clientId,
    redirect_uri: redirectUri,
    scope: 'openid email',
    state,
  });
  return { url: `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`, state };
}

interface LoginProps {
  authApi?: AuthApi;
}

export function Login({ authApi = defaultAuthApi }: LoginProps) {
  const { user, loading: authLoading } = useAuth();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const code = searchParams.get('code');

  // When a code is present we are immediately in the exchange flow.
  // Initialise loading from the presence of the code so no synchronous
  // setState is needed inside the effect (satisfies react-hooks/set-state-in-effect).
  const [loading, setLoading] = useState(() => !!code);
  const [error, setError] = useState<string | null>(null);

  // Track whether the exchange has already been triggered to avoid double-fire.
  const exchangeStarted = useRef(false);

  useEffect(() => {
    if (!code) return;
    if (exchangeStarted.current) return;
    exchangeStarted.current = true;

    // NO cancelled/cleanup gating here (#169): under StrictMode the mount effect runs
    // setup -> synthetic cleanup -> setup, and the exchangeStarted ref (which must stay —
    // the authorization code is single-use) blocks the second run. The ONLY run that ever
    // performs the exchange is the synthetically-"cancelled" first one, so gating its
    // outcome on a cancelled flag froze BOTH the successful navigate and the error state
    // on the eternal spinner. Post-unmount setState is a safe no-op in React 18.
    const doExchange = async () => {
      // Validate OAuth state parameter to prevent CSRF attacks
      const storedState = sessionStorage.getItem('oauth_state');
      const returnedState = searchParams.get('state');
      sessionStorage.removeItem('oauth_state');

      if (!storedState || storedState !== returnedState) {
        setError('Authentication state mismatch. Please try again.');
        setLoading(false);
        return;
      }

      try {
        await authApi.googleLogin(code);
        await authApi.getMe();
        navigate('/home', { replace: true });
      } catch (err: unknown) {
        if (err instanceof ApiError && err.status === 401) {
          setError('Authentication failed: unauthorized. Please try again.');
        } else if (err instanceof Error) {
          setError(`Authentication error: ${err.message}`);
        } else {
          setError('Authentication failed. Please try again.');
        }
        setLoading(false);
      }
    };

    void doExchange();
  }, [code, authApi, navigate, searchParams]);

  // If already authenticated and not in exchange flow, redirect to home
  if (!code && authLoading) return null;
  if (!code && user) return <Navigate to="/home" replace />;

  if (loading) {
    return (
      <div className="panel-screen entrance">
        <div className="entrance-logo">ГЛЯДЕЛКИ</div>
        <div className="results-note">Секунду…</div>
      </div>
    );
  }

  const { url: googleAuthUrl } = buildGoogleAuthUrl();

  return (
    <div className="panel-screen entrance">
      <div className="entrance-logo">ГЛЯДЕЛКИ</div>
      <div className="entrance-tagline">кто моргнул — тот проиграл</div>
      {error && (
        <div className="panel-status" role="alert">
          {error}
        </div>
      )}
      <a className="btn-mode entrance-google" href={googleAuthUrl} data-testid="google-signin-link">
        Войти через Google
      </a>
    </div>
  );
}
