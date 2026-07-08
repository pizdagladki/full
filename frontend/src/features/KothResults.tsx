import { useLocation, useNavigate } from 'react-router-dom';
import type { HillType } from './KothBattle';
import type { KingInfo } from '../api/koth';

// ---------------------------------------------------------------------------
// KotH solo results screen — reads the outcome purely from location.state
// (computed by KothBattle already); no async fetch. Distractions are NOT
// available in solo mode — this screen (and KothBattle) never render one.
// ---------------------------------------------------------------------------

interface KothResultsLocationState {
  hillType?: HillType;
  error?: boolean;
  // daily / monthly
  won?: boolean;
  king?: KingInfo;
  survivedMs?: number;
  // ranked
  achievedRank?: number;
  currentRank?: number;
  newlyReached?: boolean;
  noAttempt?: boolean;
}

export function KothResults() {
  const location = useLocation();
  const navigate = useNavigate();

  const state = (location.state as KothResultsLocationState | null) ?? null;
  const hillType: HillType = state?.hillType ?? 'daily';

  function handlePlayAgain() {
    navigate('/koth/battle', { state: { hillType } });
  }

  function handleBack() {
    navigate('/koth');
  }

  return (
    <div className="koth-screen" data-testid="koth-results-screen">
      {state?.error ? (
        <div className="panel-status" data-testid="koth-error">Не получилось записать попытку</div>
      ) : hillType === 'ranked' ? (
        <div data-testid="koth-ranked-outcome">
          {state?.noAttempt ? (
            <div className="results-note" data-testid="koth-no-attempt">Попытка не записана</div>
          ) : (
            <>
              {state?.newlyReached ? (
                <div className="results-verdict koth-verdict" data-testid="koth-rank-reached">Новый ранг: {state?.achievedRank}!</div>
              ) : (
                <div className="koth-me-line" data-testid="koth-rank-current">Твой ранг: {state?.currentRank}</div>
              )}
            </>
          )}
        </div>
      ) : (
        <div data-testid="koth-hill-outcome">
          {state?.won ? (
            <div className="results-verdict koth-verdict" data-testid="koth-won">Ты — новый король! 👑</div>
          ) : (
            <div className="results-verdict koth-verdict koth-verdict--lost" data-testid="koth-lost">Король устоял</div>
          )}
        </div>
      )}

      <div className="results-note" data-testid="rewards-placeholder">Награды скоро подъедут</div>

      <div className="panel-actions">
        <button type="button" className="btn-mode" data-testid="play-again" onClick={handlePlayAgain}>
          Играть ещё
        </button>
        <button type="button" className="results-report-btn" data-testid="back-to-koth" onClick={handleBack}>
          Назад
        </button>
      </div>
    </div>
  );
}
