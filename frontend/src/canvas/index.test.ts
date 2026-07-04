import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  EditSlotImpl,
  EDIT_SLOT_DURATION_MS_DEFAULT,
  drawNeutralSlot,
  runEditSlot,
} from './index';

function makeCanvasAndCtx(): {
  canvas: HTMLCanvasElement;
  ctx: CanvasRenderingContext2D;
} {
  const canvas = document.createElement('canvas');
  canvas.width = 320;
  canvas.height = 240;
  const ctx = { fillStyle: '', fillRect: vi.fn() } as unknown as CanvasRenderingContext2D;
  vi.spyOn(canvas, 'getContext').mockReturnValue(ctx);
  return { canvas, ctx };
}

describe('EditSlotImpl', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  // criterion: 2 — the edit-slot placeholder plays for ~10 seconds (default duration) before
  // completing.
  it('~10s-default-duration: play() does not resolve before EDIT_SLOT_DURATION_MS_DEFAULT elapses, resolves exactly at it', async () => {
    const { canvas, ctx } = makeCanvasAndCtx();
    const slot = new EditSlotImpl(canvas, ctx);

    let resolved = false;
    const done = slot.play().then(() => {
      resolved = true;
    });

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT - 1);
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await done;
    expect(resolved).toBe(true);
  });

  // criterion: 2 (violation guard) — an injected durationMs is honored instead of always using the
  // ~10s default; proves the timing is actually configurable/derived from options, not hardcoded.
  it('~10s-default-duration violation guard: a custom durationMs overrides the ~10s default', async () => {
    const { canvas, ctx } = makeCanvasAndCtx();
    const slot = new EditSlotImpl(canvas, ctx, { durationMs: 2000 });

    let resolved = false;
    const done = slot.play().then(() => {
      resolved = true;
    });

    await vi.advanceTimersByTimeAsync(1999);
    expect(resolved).toBe(false);

    await vi.advanceTimersByTimeAsync(1);
    await done;
    expect(resolved).toBe(true);
  });

  // criterion: 2 — a neutral placeholder rectangle (NOT a final animation) is drawn onto the
  // canvas when the slot plays.
  it('neutral-slot-draw: play() draws the neutral placeholder rectangle onto the canvas', async () => {
    const { canvas, ctx } = makeCanvasAndCtx();
    const slot = new EditSlotImpl(canvas, ctx);

    const done = slot.play();
    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, canvas.width, canvas.height);

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await done;
  });

  // criterion: 2 (violation guard) — an injected draw callback is used instead of the built-in
  // drawNeutralSlot, so callers can swap in their own placeholder drawing without forking the class.
  it('neutral-slot-draw violation guard: an injected draw callback replaces drawNeutralSlot', async () => {
    const { canvas, ctx } = makeCanvasAndCtx();
    const customDraw = vi.fn();
    const slot = new EditSlotImpl(canvas, ctx, { draw: customDraw });

    const done = slot.play();
    expect(customDraw).toHaveBeenCalledWith(ctx, canvas);
    expect(ctx.fillRect).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(EDIT_SLOT_DURATION_MS_DEFAULT);
    await done;
  });

  // criterion: 2 — stop() cancels a running slot: onComplete/play() must not fire afterward.
  it('stop-cancels: calling stop() before the duration elapses prevents onComplete from firing', async () => {
    const { canvas, ctx } = makeCanvasAndCtx();
    const slot = new EditSlotImpl(canvas, ctx, { durationMs: 5000 });
    const onComplete = vi.fn();
    slot.onComplete(onComplete);

    void slot.play();
    await vi.advanceTimersByTimeAsync(2000);
    slot.stop();
    await vi.advanceTimersByTimeAsync(10000);

    expect(onComplete).not.toHaveBeenCalled();
  });

  it('drawNeutralSlot fills the full canvas with a neutral color', () => {
    const ctx = { fillStyle: '', fillRect: vi.fn() } as unknown as CanvasRenderingContext2D;
    const canvas = { width: 100, height: 50 } as HTMLCanvasElement;

    drawNeutralSlot(ctx, canvas);

    expect(ctx.fillRect).toHaveBeenCalledWith(0, 0, 100, 50);
    expect(ctx.fillStyle).not.toBe('');
  });

  // Violation guard: runEditSlot must resolve without throwing when getContext('2d') returns null
  // (e.g. an unsupported canvas) rather than hanging or crashing the caller.
  it('runEditSlot resolves gracefully when the canvas has no 2d context', async () => {
    const canvas = document.createElement('canvas');
    vi.spyOn(canvas, 'getContext').mockReturnValue(null);

    await expect(runEditSlot(canvas)).resolves.toBeUndefined();
  });

  it('runEditSlot plays the slot on a real canvas 2d context and resolves after the duration', async () => {
    const { canvas } = makeCanvasAndCtx();

    const done = runEditSlot(canvas, { durationMs: 1000 });
    await vi.advanceTimersByTimeAsync(1000);
    await expect(done).resolves.toBeUndefined();
  });
});
