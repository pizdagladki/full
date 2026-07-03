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
    <div data-testid="koth-results-screen">
      <h1>Results</h1>

      {state?.error ? (
        <div data-testid="koth-error">Something went wrong recording your attempt</div>
      ) : hillType === 'ranked' ? (
        <div data-testid="koth-ranked-outcome">
          {state?.noAttempt ? (
            <div data-testid="koth-no-attempt">No attempt recorded</div>
          ) : (
            <>
              {state?.newlyReached ? (
                <div data-testid="koth-rank-reached">Reached rank {state?.achievedRank}!</div>
              ) : (
                <div data-testid="koth-rank-current">Current rank: {state?.currentRank}</div>
              )}
            </>
          )}
        </div>
      ) : (
        <div data-testid="koth-hill-outcome">
          {state?.won ? (
            <div data-testid="koth-won">You are the new King!</div>
          ) : (
            <div data-testid="koth-lost">The king remains undefeated</div>
          )}
        </div>
      )}

      <div data-testid="rewards-placeholder">Rewards coming soon</div>

      <div>
        <button type="button" data-testid="play-again" onClick={handlePlayAgain}>
          Play again
        </button>
        <button type="button" data-testid="back-to-koth" onClick={handleBack}>
          Back
        </button>
      </div>
    </div>
  );
}
