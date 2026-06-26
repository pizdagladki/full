import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useAuth } from './auth/AuthContext';
import { defaultAuthApi } from '../api/auth';
import type { AuthApi } from '../api/auth';

interface ConsentProps {
  authApi?: AuthApi;
}

export function Consent({ authApi = defaultAuthApi }: ConsentProps) {
  const { user, loading } = useAuth();
  const navigate = useNavigate();

  const [adultChecked, setAdultChecked] = useState(false);
  const [recordingChecked, setRecordingChecked] = useState(false);
  const [tosChecked, setTosChecked] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  if (loading) return null;

  if (user?.consent != null) {
    return <Navigate to="/home" replace />;
  }

  const allChecked = adultChecked && recordingChecked && tosChecked;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!allChecked) return;
    setErrorMessage(null);
    try {
      await authApi.submitConsent({
        is_adult: true,
        consent_recording: true,
        consent_tos: true,
      });
      navigate('/home');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'An error occurred';
      setErrorMessage(msg);
    }
  };

  return (
    <div>
      <h1>Consent</h1>
      <form onSubmit={handleSubmit}>
        <div>
          <label>
            <input
              type="checkbox"
              data-testid="checkbox-adult"
              checked={adultChecked}
              onChange={(e) => setAdultChecked(e.target.checked)}
            />
            {' '}I confirm I am 18 years of age or older
          </label>
        </div>
        <div>
          <label>
            <input
              type="checkbox"
              data-testid="checkbox-recording"
              checked={recordingChecked}
              onChange={(e) => setRecordingChecked(e.target.checked)}
            />
            {' '}I consent to recording and publishing my face in battle video
          </label>
        </div>
        <div>
          <label>
            <input
              type="checkbox"
              data-testid="checkbox-tos"
              checked={tosChecked}
              onChange={(e) => setTosChecked(e.target.checked)}
            />
            {' '}I agree to the full user agreement
          </label>
        </div>
        <button
          type="submit"
          data-testid="btn-continue"
          disabled={!allChecked}
        >
          Continue
        </button>
      </form>
      {errorMessage != null && (
        <div data-testid="error-message" role="alert">
          {errorMessage}
        </div>
      )}
    </div>
  );
}
