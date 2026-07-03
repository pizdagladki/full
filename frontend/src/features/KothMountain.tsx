import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { defaultKothApi } from '../api/koth';
import type { KingInfo, KothApi } from '../api/koth';

// ---------------------------------------------------------------------------
// KotH daily/monthly "mountain" leaderboard — a NEUTRAL functional stub, not
// the final visual. Serves both the daily and monthly hill via location.state
// (mirrors KothBattle.tsx's KothBattleLocationState pattern). There is no
// backend top-10 endpoint yet (#104 only exposes the single current king), so
// the screen renders the real king plus a static placeholder zigzag list —
// never fabricated fake user data for those extra slots.
// ---------------------------------------------------------------------------

export type MountainHillType = 'daily' | 'monthly';

interface KothMountainLocationState {
  hillType?: MountainHillType | 'ranked';
}

/** 9 placeholder slots rendered under the real king — a functional stub, not real data. */
const PLACEHOLDER_SLOT_COUNT = 9;

type KingState =
  | { status: 'loading' }
  | { status: 'loaded'; king: KingInfo }
  | { status: 'empty' };

export interface KothMountainProps {
  /** Injectable koth API (swap with a mock in tests). Defaults to the real client. */
  kothApi?: KothApi;
}

function resolveHillType(raw: MountainHillType | 'ranked' | undefined): MountainHillType {
  // Defensive: this screen only ever serves daily/monthly — a stray 'ranked' (or missing state)
  // falls back to 'daily' rather than crashing (mirrors KothBattle.tsx's `?? 'daily'` fallback).
  if (raw === 'monthly') return 'monthly';
  return 'daily';
}

export function KothMountain({ kothApi = defaultKothApi }: KothMountainProps) {
  const location = useLocation();
  const navigate = useNavigate();

  const locationState = (location.state as KothMountainLocationState | null) ?? null;
  const hillType = resolveHillType(locationState?.hillType);

  const [kingState, setKingState] = useState<KingState>({ status: 'loading' });

  useEffect(() => {
    let cancelled = false;
    kothApi
      .getKing(hillType)
      .then((king) => {
        if (cancelled) return;
        setKingState(king ? { status: 'loaded', king } : { status: 'empty' });
      })
      .catch(() => {
        if (cancelled) return;
        // A fetch error degrades to the same neutral empty state, never a distinct crash/error UI
        // (mirrors kingClipsApi.getCurrent's null-on-404 contract).
        setKingState({ status: 'empty' });
      });
    return () => {
      cancelled = true;
    };
  }, [hillType, kothApi]);

  function handlePlay() {
    navigate('/koth/battle', { state: { hillType } });
  }

  function handleBack() {
    navigate('/koth');
  }

  return (
    <div data-testid="koth-mountain-screen">
      <h1>{hillType === 'monthly' ? 'Monthly hill' : 'Daily hill'}</h1>

      {kingState.status === 'loading' && (
        <div data-testid="mountain-loading">Loading the hill…</div>
      )}

      {kingState.status === 'empty' && (
        <div data-testid="mountain-empty">No king yet — be the first to claim this hill</div>
      )}

      {kingState.status === 'loaded' && (
        <div data-testid="mountain-king">
          <span data-testid="mountain-king-user">{kingState.king.user_id}</span>
          <span data-testid="mountain-king-blink">{kingState.king.blink_ts_ms}</span>
          {/* Functional stub: no backend top-10 endpoint exists yet (#104 only exposes the single
              current king) — these are neutral placeholder slots, never fabricated fake data. */}
          <ol>
            {Array.from({ length: PLACEHOLDER_SLOT_COUNT }, (_, i) => i + 2).map((slot) => (
              <li key={slot} data-testid={`mountain-slot-${slot}`}>
                —
              </li>
            ))}
          </ol>
        </div>
      )}

      <div>
        <button type="button" data-testid="mountain-play" onClick={handlePlay}>
          Challenge
        </button>
        <button type="button" data-testid="mountain-back" onClick={handleBack}>
          Back
        </button>
      </div>
    </div>
  );
}
