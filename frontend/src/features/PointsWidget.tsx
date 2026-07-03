import { useEffect, useState } from 'react';
import { defaultPointsApi } from '../api/points';
import type { PointsApi } from '../api/points';

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PointsWidgetProps {
  /** Injectable points API (swap with a mock in tests). Defaults to the real client. */
  pointsApi?: PointsApi;
}

// ---------------------------------------------------------------------------
// PointsWidget — home top-right balance + info panel
// ---------------------------------------------------------------------------
//
// [AGENT-SCAFFOLD] neutral-design placeholder — final icon/visual/style are
// out of scope. This is just a placeholder icon + plain static text.

export function PointsWidget({ pointsApi = defaultPointsApi }: PointsWidgetProps) {
  const [balance, setBalance] = useState<number | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [panelOpen, setPanelOpen] = useState<boolean>(false);

  useEffect(() => {
    let cancelled = false;

    // Wrapped in an async IIFE so a synchronous throw (e.g. fetch unavailable)
    // can never crash render — it degrades to the error placeholder instead.
    (async () => {
      try {
        const data = await pointsApi.getBalance();
        if (cancelled) return;
        setBalance(data.balance);
        setLoading(false);
      } catch (err: unknown) {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : 'Failed to load points balance';
        setError(msg);
        setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [pointsApi]);

  const showPlaceholder = loading || error !== null || balance === null;

  return (
    <div style={{ position: 'absolute', top: 0, right: 0 }}>
      <button
        type="button"
        data-testid="points-widget"
        aria-label="Points balance — open info panel"
        onClick={() => setPanelOpen((open) => !open)}
      >
        <span data-testid="points-icon" aria-hidden="true">
          ◆
        </span>
        {showPlaceholder ? (
          <span data-testid="points-placeholder">&mdash;</span>
        ) : (
          <span data-testid="points-balance">{balance}</span>
        )}
      </button>

      {panelOpen && (
        <div data-testid="points-info-panel" role="dialog" aria-label="Points info">
          <button type="button" aria-label="Close points info" onClick={() => setPanelOpen(false)}>
            Close
          </button>

          <section aria-label="How you earn points">
            <h3>How you earn points</h3>
            <ul>
              <li>Winning a match</li>
              <li>Leveling up in ranked play</li>
            </ul>
          </section>

          <section aria-label="Where points are spent">
            <h3>Where points are spent</h3>
            <p>Points are spent in the in-game store.</p>
          </section>

          <section aria-label="Holding points">
            <h3>Holding points</h3>
            <p>
              Points can be held for a possible future airdrop. If one happens, the pool may be
              shared pro-rata based on how much you hold &mdash; with no guarantees, no promised
              value, and no figures.
            </p>
          </section>
        </div>
      )}
    </div>
  );
}
