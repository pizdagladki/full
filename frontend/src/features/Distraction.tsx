import { useCallback, useEffect, useRef, useState } from 'react';
import type { MutableRefObject } from 'react';

// ---------------------------------------------------------------------------
// [AGENT-SCAFFOLD] Distraction mechanic — neutral-design placeholder.
//
// This is a self-contained scaffold: a control that unlocks 30s after battle
// start, applies a ONE-TIME tiered gray overlay over the victim's screen,
// plays a minimal (injectable) sound, and records what happened into a
// battle-meta object for later sharing/recording use. It is NOT wired into
// Battle.tsx — a later task mounts it there. Real visuals/animation and
// counter-measures ("shake off"/shield) are explicitly out of scope; this
// component only renders a flat gray box sized to the tier's coverage
// fraction, as a stand-in the real design will replace.
// ---------------------------------------------------------------------------

export type DistractionTier = 1 | 2 | 3;

export interface DistractionTierConfig {
  tier: DistractionTier;
  /** Fraction of the screen the placeholder overlay covers, 0..1 (≈0.8/0.6/0.4 for tier 1/2/3). */
  coverageFraction: number;
  /** How long the overlay stays visible before auto-hiding, in ms. */
  durationMs: number;
}

export interface DistractionMeta {
  tier: DistractionTier;
  /** ms since battle start (NOT raw wall-clock) — meaningful for later sharing/recording. */
  applied_at_ms: number;
}

/** Minimal battle-meta shape the recording/sharing layer reads later. */
export interface BattleMeta {
  distractions: DistractionMeta[];
}

export function makeEmptyBattleMeta(): BattleMeta {
  return { distractions: [] };
}

const UNLOCK_DELAY_MS_DEFAULT = 30000;

export const DEFAULT_TIER_CONFIGS: readonly DistractionTierConfig[] = [
  { tier: 1, coverageFraction: 0.8, durationMs: 3000 },
  { tier: 2, coverageFraction: 0.6, durationMs: 3000 },
  { tier: 3, coverageFraction: 0.4, durationMs: 3000 },
];

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface DistractionProps {
  /** Battle-start timestamp (`Date.now()`-compatible). Defaults to `now()` captured at mount. */
  battleStartMs?: number;
  /** Delay (ms) before the control unlocks. Defaults to the criterion's 30000ms. */
  unlockDelayMs?: number;
  /** Injectable tier configs (coverage fraction + overlay duration per tier). */
  tierConfigs?: readonly DistractionTierConfig[];
  /** Injectable minimal-sound trigger — defaults to a no-op-safe stub (swap with a mock in tests). */
  playSound?: (tier: DistractionTier) => void;
  /** Injectable battle-meta object the recording/sharing layer reads later; appended to in place. */
  battleMetaRef?: MutableRefObject<BattleMeta>;
  /** Injectable clock (swap with a fake in tests). Defaults to `Date.now`. */
  now?: () => number;
  /** Called synchronously right after a distraction is applied and written into `battleMetaRef`. */
  onApply?: (meta: DistractionMeta) => void;
}

// ---------------------------------------------------------------------------
// Distraction — unlock-after-30s, one-shot tiered gray-overlay control
// ---------------------------------------------------------------------------

export function Distraction({
  battleStartMs,
  unlockDelayMs = UNLOCK_DELAY_MS_DEFAULT,
  tierConfigs = DEFAULT_TIER_CONFIGS,
  playSound = () => {},
  battleMetaRef,
  now = Date.now,
  onApply,
}: DistractionProps) {
  // Captured once — a later prop change must not reset the battle-start anchor.
  const battleStartRef = useRef(battleStartMs ?? now());
  const unlockTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const hideTimerRef = useRef<ReturnType<typeof setTimeout>>();
  // Refs (not state) drive the one-shot gate synchronously — state would still allow a second
  // click to slip through between the click handler running and the re-render committing.
  const usedRef = useRef(false);
  const teardownRef = useRef(false);

  const [unlocked, setUnlocked] = useState(false);
  const [active, setActive] = useState<DistractionTierConfig | null>(null);

  // Mount effect — arms the unlock timer. In production this runs exactly once; under
  // React.StrictMode (dev) it runs mount → cleanup → mount on the same fiber, so the guard ref is
  // reset to a clean slate HERE (not just at declaration) — mirrors Battle.tsx's StrictMode-safe
  // pattern with teardownRef.
  useEffect(() => {
    teardownRef.current = false;

    unlockTimerRef.current = setTimeout(() => {
      if (teardownRef.current) return;
      setUnlocked(true);
    }, unlockDelayMs);

    return () => {
      teardownRef.current = true;
      if (unlockTimerRef.current) clearTimeout(unlockTimerRef.current);
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current);
    };
    // Intentionally stable ([]): the unlock delay/battle-start anchor are fixed at mount, mirroring
    // Battle.tsx's sanity/countdown timers.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const applyTier = useCallback(
    (config: DistractionTierConfig) => {
      // One-shot per battle: once used (or before unlock), further clicks are no-ops.
      if (usedRef.current || !unlocked) return;
      usedRef.current = true;
      setUnlocked(false); // keeps the control disabled for the rest of the battle

      const meta: DistractionMeta = {
        tier: config.tier,
        applied_at_ms: now() - battleStartRef.current,
      };
      if (battleMetaRef) {
        battleMetaRef.current.distractions.push(meta);
      }
      playSound(config.tier);
      onApply?.(meta);

      setActive(config);
      hideTimerRef.current = setTimeout(() => {
        if (teardownRef.current) return;
        setActive(null);
      }, config.durationMs);
    },
    [unlocked, now, battleMetaRef, playSound, onApply],
  );

  // `unlocked` alone is sufficient here — applyTier() flips it back to false the instant a
  // distraction is applied, so it already reflects the one-shot gate; usedRef is only consulted
  // from the (non-render) click handler above, since reading a ref during render is disallowed.
  const disabled = !unlocked;

  return (
    <div data-testid="distraction-control">
      {tierConfigs.map((config) => (
        <button
          key={config.tier}
          type="button"
          data-testid={`distraction-tier-${config.tier}-button`}
          disabled={disabled}
          onClick={() => applyTier(config)}
        >
          Distraction (Tier {config.tier})
        </button>
      ))}
      {active && (
        <div
          data-testid="distraction-overlay"
          data-tier={active.tier}
          data-coverage={active.coverageFraction}
          style={{
            position: 'absolute',
            inset: 0,
            width: '100%',
            height: `${active.coverageFraction * 100}%`,
            background: '#808080',
          }}
        />
      )}
    </div>
  );
}
