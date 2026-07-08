import { useState } from 'react';
import { Navigate, useNavigate } from 'react-router-dom';
import { useAuth } from './auth/AuthContext';
import { defaultAuthApi } from '../api/auth';
import type { AuthApi } from '../api/auth';

interface ConsentProps {
  authApi?: AuthApi;
}

export function Consent({ authApi = defaultAuthApi }: ConsentProps) {
  const { user, loading, refreshUser } = useAuth();
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
      await refreshUser();
      navigate('/home');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'An error occurred';
      setErrorMessage(msg);
    }
  };

  return (
    <div className="panel-screen">
      <h1 className="panel-title">Пара галочек</h1>
      <form className="sheet consent-form" onSubmit={handleSubmit}>
        <div>
          <label className="consent-row">
            <input
              type="checkbox"
              data-testid="checkbox-adult"
              checked={adultChecked}
              onChange={(e) => setAdultChecked(e.target.checked)}
            />
            {' '}Мне есть 18 лет
          </label>
        </div>
        <div>
          <label className="consent-row">
            <input
              type="checkbox"
              data-testid="checkbox-recording"
              checked={recordingChecked}
              onChange={(e) => setRecordingChecked(e.target.checked)}
            />
            {' '}Я согласен на запись и публикацию моего лица в видео боёв
          </label>
        </div>
        <div>
          <label className="consent-row">
            <input
              type="checkbox"
              data-testid="checkbox-tos"
              checked={tosChecked}
              onChange={(e) => setTosChecked(e.target.checked)}
            />
            {' '}Принимаю пользовательское соглашение
          </label>
        </div>
        <button
          type="submit"
          className="btn-mode consent-continue"
          data-testid="btn-continue"
          disabled={!allChecked}
        >
          Дальше
        </button>
      </form>
      {errorMessage != null && (
        <div className="panel-status" data-testid="error-message" role="alert">
          {errorMessage}
        </div>
      )}
    </div>
  );
}
