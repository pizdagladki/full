import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { defaultKothApi } from '../api/koth';
import type { KothApi, RankCount, RankedMeResult } from '../api/koth';

// ---------------------------------------------------------------------------
// KotH ranked leaderboard — shows the accounts-per-rank distribution plus the
// player's own rank/next target. Two independent fetches (mirrors Store.tsx's
// separate catalog/inventory effects) so one section failing never blocks the
// other; each section owns its own loading/error/empty handling.
// ---------------------------------------------------------------------------

type LeaderboardState =
  | { status: 'loading' }
  | { status: 'loaded'; counts: RankCount[] }
  | { status: 'error' };

type MeState =
  | { status: 'loading' }
  | { status: 'loaded'; me: RankedMeResult }
  | { status: 'error' };

export interface KothRankedProps {
  /** Injectable koth API (swap with a mock in tests). Defaults to the real client. */
  kothApi?: KothApi;
}

export function KothRanked({ kothApi = defaultKothApi }: KothRankedProps) {
  const navigate = useNavigate();

  const [leaderboard, setLeaderboard] = useState<LeaderboardState>({ status: 'loading' });
  const [me, setMe] = useState<MeState>({ status: 'loading' });

  // Leaderboard fetch — independent of the "me" fetch below.
  useEffect(() => {
    let cancelled = false;
    kothApi
      .getRankedLeaderboard()
      .then((counts) => {
        if (cancelled) return;
        setLeaderboard({ status: 'loaded', counts });
      })
      .catch(() => {
        if (cancelled) return;
        setLeaderboard({ status: 'error' });
      });
    return () => {
      cancelled = true;
    };
  }, [kothApi]);

  // "Me" fetch — independent of the leaderboard fetch above; a rejection here must never block
  // the leaderboard section from rendering.
  useEffect(() => {
    let cancelled = false;
    kothApi
      .getRankedMe()
      .then((result) => {
        if (cancelled) return;
        setMe({ status: 'loaded', me: result });
      })
      .catch(() => {
        if (cancelled) return;
        setMe({ status: 'error' });
      });
    return () => {
      cancelled = true;
    };
  }, [kothApi]);

  function handlePlay() {
    navigate('/koth/battle', { state: { hillType: 'ranked' } });
  }

  return (
    <div data-testid="koth-ranked-screen">
      <h1>Ranked hill</h1>

      <section aria-label="leaderboard">
        <h2>Accounts per rank</h2>
        {leaderboard.status === 'loading' && (
          <div data-testid="ranked-leaderboard-loading">Loading leaderboard…</div>
        )}
        {leaderboard.status === 'error' && (
          <div data-testid="ranked-leaderboard-error">Could not load the leaderboard</div>
        )}
        {leaderboard.status === 'loaded' && leaderboard.counts.length === 0 && (
          <div data-testid="ranked-leaderboard-empty">No ranks reached yet</div>
        )}
        {leaderboard.status === 'loaded' && leaderboard.counts.length > 0 && (
          <ul>
            {leaderboard.counts.map((row) => (
              <li key={row.rank} data-testid={`rank-row-${row.rank}`}>
                Rank {row.rank}: {row.count}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-label="me">
        <h2>Your rank</h2>
        {me.status === 'loading' && <div data-testid="ranked-me-loading">Loading your rank…</div>}
        {me.status === 'error' && (
          <div data-testid="ranked-me-error">Could not load your rank</div>
        )}
        {me.status === 'loaded' && (
          <>
            <div data-testid="ranked-me-current">Current rank: {me.me.current_rank}</div>
            <div data-testid="ranked-me-target">Next target: {me.me.next_target_ms}</div>
          </>
        )}
      </section>

      <div>
        <button type="button" data-testid="ranked-play" onClick={handlePlay}>
          Challenge
        </button>
      </div>
    </div>
  );
}
