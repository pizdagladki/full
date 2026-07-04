// canvas — Canvas/WebGL edit templates rendered over video.
// Imperative; accessed via refs.
//
// [AGENT-SCAFFOLD] Edit-slot placeholder: draws a neutral rectangle/slot on a
// canvas for a fixed duration (~10s) then completes. This is explicitly a
// PLACEHOLDER — not a final animation, and not a general animation framework.
// Timer-driven (setTimeout), so it's directly testable with
// vi.useFakeTimers() and NOT tied to React state/render.

/** Default duration the edit slot plays for, ms (~10s per the acceptance criteria). */
export const EDIT_SLOT_DURATION_MS_DEFAULT = 10000;

/** Draws a neutral gray rectangle covering the whole canvas — the placeholder "slot". */
export const drawNeutralSlot: EditSlotDrawFn = (ctx, canvas) => {
  ctx.fillStyle = '#3a3a3a';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
};

export type EditSlotDrawFn = (
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
) => void;

export interface EditSlotOptions {
  /** Total duration the slot plays for, ms. Defaults to EDIT_SLOT_DURATION_MS_DEFAULT. */
  durationMs?: number;
  /** Injectable draw callback — defaults to `drawNeutralSlot`. */
  draw?: EditSlotDrawFn;
}

/**
 * EditSlotImpl — plays a neutral placeholder slot on a canvas for a fixed
 * duration, then resolves/completes. Deliberately simple: no keyframes, no
 * easing, no generic animation system — just a stand-in the recording engine
 * can capture over while the win-part plays out.
 */
export class EditSlotImpl {
  private readonly canvas: HTMLCanvasElement;
  private readonly ctx: CanvasRenderingContext2D;
  private readonly durationMs: number;
  private readonly draw: EditSlotDrawFn;
  private timeoutId: ReturnType<typeof setTimeout> | null = null;
  private onCompleteCb: (() => void) | undefined;

  constructor(
    canvas: HTMLCanvasElement,
    ctx: CanvasRenderingContext2D,
    opts: EditSlotOptions = {},
  ) {
    this.canvas = canvas;
    this.ctx = ctx;
    this.durationMs = opts.durationMs ?? EDIT_SLOT_DURATION_MS_DEFAULT;
    this.draw = opts.draw ?? drawNeutralSlot;
  }

  onComplete(cb: () => void): void {
    this.onCompleteCb = cb;
  }

  /** Draws the slot immediately, then resolves once `durationMs` has elapsed. */
  play(): Promise<void> {
    this.draw(this.ctx, this.canvas);
    return new Promise((resolve) => {
      this.timeoutId = setTimeout(() => {
        this.timeoutId = null;
        this.onCompleteCb?.();
        resolve();
      }, this.durationMs);
    });
  }

  /** Cancels a running slot without firing onComplete/resolving play(). */
  stop(): void {
    if (this.timeoutId !== null) {
      clearTimeout(this.timeoutId);
      this.timeoutId = null;
    }
  }
}

/** Convenience helper — runs a one-shot edit slot on `canvas` and resolves when it completes. */
export function runEditSlot(
  canvas: HTMLCanvasElement,
  opts: EditSlotOptions = {},
): Promise<void> {
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    return Promise.resolve();
  }
  return new EditSlotImpl(canvas, ctx, opts).play();
}
