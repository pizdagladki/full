import { createRef } from 'react';
import { act, render, screen } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { Distraction, makeEmptyBattleMeta } from './Distraction';
import type { BattleMeta, DistractionMeta } from './Distraction';

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

function tierButton(tier: 1 | 2 | 3): HTMLButtonElement {
  return screen.getByTestId(`distraction-tier-${tier}-button`) as HTMLButtonElement;
}

// ---------------------------------------------------------------------------
// Tests — one named case per acceptance criterion
// ---------------------------------------------------------------------------

describe('Distraction', () => {
  // criterion: 1 — the control is disabled for the first 30 seconds of the battle.
  it('locked-before-30s: all tier buttons stay disabled before the 30s unlock delay elapses', () => {
    render(<Distraction battleStartMs={0} />);

    expect(tierButton(1).disabled).toBe(true);
    expect(tierButton(2).disabled).toBe(true);
    expect(tierButton(3).disabled).toBe(true);

    act(() => {
      vi.advanceTimersByTime(29999);
    });

    expect(tierButton(1).disabled).toBe(true);
    expect(tierButton(2).disabled).toBe(true);
    expect(tierButton(3).disabled).toBe(true);
  });

  // criterion: 1 — the control becomes enabled after 30 seconds.
  it('unlock-after-30s: all tier buttons become enabled once the 30s unlock delay elapses', () => {
    render(<Distraction battleStartMs={0} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });

    expect(tierButton(1).disabled).toBe(false);
    expect(tierButton(2).disabled).toBe(false);
    expect(tierButton(3).disabled).toBe(false);
  });

  // criterion: 1 (violation guard, custom delay) — an injected unlockDelayMs is honored, so a
  // control configured for a different delay must NOT unlock at the default 30s.
  it('unlock-after-30s violation guard: a custom unlockDelayMs is honored instead of the 30s default', () => {
    render(<Distraction battleStartMs={0} unlockDelayMs={5000} />);

    act(() => {
      vi.advanceTimersByTime(4999);
    });
    expect(tierButton(1).disabled).toBe(true);

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(tierButton(1).disabled).toBe(false);
  });

  // criterion: 2 — applying a distraction renders a gray overlay placeholder covering the
  // configured screen fraction for its tier (≈80/60/40% for tier 1/2/3) for a configured
  // duration, then auto-hides.
  it.each([
    { tier: 1 as const, coverage: 0.8, name: 'tier 1 (80%)' },
    { tier: 2 as const, coverage: 0.6, name: 'tier 2 (60%)' },
    { tier: 3 as const, coverage: 0.4, name: 'tier 3 (40%)' },
  ])('tier-overlay-and-auto-hide: $name renders the configured coverage then auto-hides', ({ tier, coverage }) => {
    render(<Distraction battleStartMs={0} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });

    expect(screen.queryByTestId('distraction-overlay')).not.toBeInTheDocument();

    act(() => {
      tierButton(tier).click();
    });

    const overlay = screen.getByTestId('distraction-overlay');
    expect(overlay.getAttribute('data-tier')).toBe(String(tier));
    expect(overlay.getAttribute('data-coverage')).toBe(String(coverage));

    // Still visible just before the configured 3000ms duration elapses.
    act(() => {
      vi.advanceTimersByTime(2999);
    });
    expect(screen.getByTestId('distraction-overlay')).toBeInTheDocument();

    // Auto-hides exactly when the configured duration elapses.
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.queryByTestId('distraction-overlay')).not.toBeInTheDocument();
  });

  // criterion: 2 (violation guard, custom tier config) — an injected tierConfigs coverage/duration
  // must be honored rather than falling back to the built-in 80/60/40 defaults.
  it('tier-overlay-and-auto-hide violation guard: a custom tierConfigs coverage/duration is honored', () => {
    render(
      <Distraction
        battleStartMs={0}
        tierConfigs={[{ tier: 1, coverageFraction: 0.5, durationMs: 1000 }]}
      />,
    );

    act(() => {
      vi.advanceTimersByTime(30000);
    });
    act(() => {
      tierButton(1).click();
    });

    expect(screen.getByTestId('distraction-overlay').getAttribute('data-coverage')).toBe('0.5');

    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(screen.queryByTestId('distraction-overlay')).not.toBeInTheDocument();
  });

  // criterion: 3 — a distraction is one-shot per battle: after one application the control stays
  // disabled for the rest of the battle.
  it('one-shot-enforcement: after one application, the control stays disabled for the rest of the battle', () => {
    const onApply = vi.fn();
    render(<Distraction battleStartMs={0} onApply={onApply} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });
    act(() => {
      tierButton(2).click();
    });

    expect(onApply).toHaveBeenCalledTimes(1);
    expect(tierButton(1).disabled).toBe(true);
    expect(tierButton(2).disabled).toBe(true);
    expect(tierButton(3).disabled).toBe(true);

    // Advancing well past the overlay duration and any further time must not re-enable anything,
    // and a second click attempt must not fire another application.
    act(() => {
      vi.advanceTimersByTime(60000);
    });
    act(() => {
      tierButton(3).click();
    });

    expect(onApply).toHaveBeenCalledTimes(1);
    expect(tierButton(3).disabled).toBe(true);
  });

  // criterion: 3 (violation guard) — clicking before unlock must be a complete no-op: no overlay,
  // no meta write, no sound, confirming the lock genuinely blocks application (not just the UI).
  it('one-shot-enforcement violation guard: clicking before unlock does not apply a distraction', () => {
    const playSound = vi.fn();
    const onApply = vi.fn();
    const battleMetaRef = { current: makeEmptyBattleMeta() };
    render(
      <Distraction
        battleStartMs={0}
        playSound={playSound}
        onApply={onApply}
        battleMetaRef={battleMetaRef}
      />,
    );

    act(() => {
      tierButton(1).click();
    });

    expect(screen.queryByTestId('distraction-overlay')).not.toBeInTheDocument();
    expect(playSound).not.toHaveBeenCalled();
    expect(onApply).not.toHaveBeenCalled();
    expect(battleMetaRef.current.distractions).toHaveLength(0);
  });

  // criterion: 4 — a minimal sound trigger fires on application (audio source injected/mockable).
  it('sound-trigger-on-apply: playSound fires with the applied tier when a distraction is applied', () => {
    const playSound = vi.fn();
    render(<Distraction battleStartMs={0} playSound={playSound} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });
    act(() => {
      tierButton(3).click();
    });

    expect(playSound).toHaveBeenCalledTimes(1);
    expect(playSound).toHaveBeenCalledWith(3);
  });

  // criterion: 4 (violation guard) — the default playSound is a safe no-op: mounting/applying
  // without an injected sound must never throw (e.g. from a bare `new Audio()` call).
  it('sound-trigger-on-apply violation guard: applying without an injected playSound does not throw', () => {
    render(<Distraction battleStartMs={0} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });

    expect(() => {
      act(() => {
        tierButton(1).click();
      });
    }).not.toThrow();
  });

  // criterion: 4 — the applied {tier, applied_at_ms} is written into the battle meta object passed
  // to the recording/sharing layer, relative to battle start (not raw wall-clock).
  it('meta-recording: applying a distraction writes {tier, applied_at_ms} into battleMetaRef', () => {
    const battleMetaRef = { current: makeEmptyBattleMeta() } as { current: BattleMeta };
    render(<Distraction battleStartMs={1000} battleMetaRef={battleMetaRef} now={() => 1000 + 31000} />);

    act(() => {
      vi.advanceTimersByTime(30000);
    });
    act(() => {
      tierButton(2).click();
    });

    expect(battleMetaRef.current.distractions).toEqual<DistractionMeta[]>([
      { tier: 2, applied_at_ms: 31000 },
    ]);
  });

  // criterion: 4 (violation guard) — applied_at_ms must be relative to battle start, NOT raw
  // wall-clock: a later battleStartMs anchor shifts the recorded offset down accordingly.
  it('meta-recording violation guard: applied_at_ms is relative to battleStartMs, not raw wall-clock', () => {
    const battleMetaRef = { current: makeEmptyBattleMeta() } as { current: BattleMeta };
    // Wall-clock "now" is fixed at 100000; battle started at 69000 -> offset should be 31000, not
    // the raw wall-clock value.
    render(
      <Distraction battleStartMs={69000} battleMetaRef={battleMetaRef} now={() => 100000} />,
    );

    act(() => {
      vi.advanceTimersByTime(30000);
    });
    act(() => {
      tierButton(1).click();
    });

    expect(battleMetaRef.current.distractions[0]?.applied_at_ms).toBe(31000);
  });

  // criterion: 5 (StrictMode-safety) — the guard ref is reset at the top of the mount effect body,
  // so a mount → unmount → remount cycle (as under React.StrictMode in dev) does not leave the
  // unlock timer permanently disarmed.
  it('strict-mode-safe-remount: unmounting and remounting still unlocks after 30s on the new mount', () => {
    const first = render(<Distraction battleStartMs={0} />);
    first.unmount();

    render(<Distraction battleStartMs={0} />);
    act(() => {
      vi.advanceTimersByTime(30000);
    });

    expect(tierButton(1).disabled).toBe(false);
  });

  it('exports a ref-free component usable without any props (defaults are all safe)', () => {
    const ref = createRef<HTMLDivElement>();
    expect(() => render(<div ref={ref}><Distraction /></div>)).not.toThrow();
    expect(screen.getByTestId('distraction-control')).toBeInTheDocument();
  });
});
