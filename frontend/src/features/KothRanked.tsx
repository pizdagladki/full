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
    <div className="koth-screen" data-testid="koth-ranked-screen">
      <h1 className="panel-title koth-title">Ранговая гора</h1>

      <section className="koth-panel" aria-label="leaderboard">
        <h2 className="koth-panel-title">Игроков на рангах</h2>
        {leaderboard.status === 'loading' && (
          <div className="results-note" data-testid="ranked-leaderboard-loading">Загружаем таблицу…</div>
        )}
        {leaderboard.status === 'error' && (
          <div className="results-note" data-testid="ranked-leaderboard-error">Не удалось загрузить таблицу</div>
        )}
        {leaderboard.status === 'loaded' && leaderboard.counts.length === 0 && (
          <div className="results-note" data-testid="ranked-leaderboard-empty">Ранги пока никто не взял</div>
        )}
        {leaderboard.status === 'loaded' && leaderboard.counts.length > 0 && (
          <ul className="koth-ranklist">
            {leaderboard.counts.map((row) => (
              <li key={row.rank} data-testid={`rank-row-${row.rank}`}>
                Ранг {row.rank}: {row.count}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="koth-panel" aria-label="me">
        <h2 className="koth-panel-title">Твой ранг</h2>
        {me.status === 'loading' && <div className="results-note" data-testid="ranked-me-loading">Загружаем твой ранг…</div>}
        {me.status === 'error' && (
          <div className="results-note" data-testid="ranked-me-error">Не удалось загрузить твой ранг</div>
        )}
        {me.status === 'loaded' && (
          <>
            <div className="koth-me-line" data-testid="ranked-me-current">Твой ранг: {me.me.current_rank}</div>
            <div className="koth-me-line" data-testid="ranked-me-target">Следующая цель: {me.me.next_target_ms}</div>
          </>
        )}
      </section>

      <div className="panel-actions">
        <button type="button" className="btn-mode" data-testid="ranked-play" onClick={handlePlay}>
          Бросить вызов
        </button>
      </div>
    </div>
  );
}
