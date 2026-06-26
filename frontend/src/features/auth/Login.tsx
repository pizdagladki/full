import { useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { defaultAuthApi } from '../../api/auth';
import type { AuthApi } from '../../api/auth';
import { ApiError } from '../../api/auth';

function buildGoogleAuthUrl(): string {
  const clientId = (import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined) ?? '';
  const redirectUri = (import.meta.env.VITE_GOOGLE_REDIRECT_URI as string | undefined) ?? '';
  const params = new URLSearchParams({
    response_type: 'code',
    client_id: clientId,
    redirect_uri: redirectUri,
    scope: 'openid email',
  });
  return `https://accounts.google.com/o/oauth2/v2/auth?${params.toString()}`;
}

interface LoginProps {
  authApi?: AuthApi;
}

export function Login({ authApi = defaultAuthApi }: LoginProps) {
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

    let cancelled = false;

    const doExchange = async () => {
      try {
        await authApi.googleLogin(code);
        await authApi.getMe();
        if (!cancelled) {
          navigate('/home', { replace: true });
        }
      } catch (err: unknown) {
        if (cancelled) return;
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

    return () => {
      cancelled = true;
    };
  }, [code, authApi, navigate]);

  if (loading) {
    return <div>Loading...</div>;
  }

  const googleAuthUrl = buildGoogleAuthUrl();

  return (
    <div>
      <h1>Sign in</h1>
      {error && <div role="alert">{error}</div>}
      <a href={googleAuthUrl} data-testid="google-signin-link">
        Sign in with Google
      </a>
    </div>
  );
}
