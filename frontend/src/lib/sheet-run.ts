/**
 * The "one sheet at a time" rule, as a value.
 *
 * DESIGN.md §2 forbids a sheet opening another sheet: two scrims, two swipes
 * and an ambiguous Escape. But a sheet does sometimes need to hand over to
 * another one — tapping a room inside Music's Zones sheet has to open that
 * room's player — so sheets *swap*: the second replaces the first and puts it
 * back on the way out. One scrim, one Escape, and no lost place.
 *
 * That's a small state machine with invariants worth keeping honest, so it
 * lives here rather than as four `let`s inside a view:
 *
 *   - at most one sheet is ever on screen;
 *   - `under` never chains — a swap out of a swapped-to sheet forgets the
 *     first, because three levels of "back" is a navigation stack, and a
 *     navigation stack is what screens are for;
 *   - dismissing returns to `under` if there is one, otherwise to the bare
 *     page;
 *   - `isUp` is the single fact the body-scroll lock keys on, so a swap
 *     doesn't release and retake it (which on iOS unpins and re-pins the
 *     body — a visible jump).
 */

export interface SheetRun<T extends string> {
  /** The sheet on screen, or null when the page is bare. */
  open: T | null;
  /** The one sheet `open` returns to when it closes. Never chains. */
  under: T | null;
}

/** The bare page: nothing up, nothing to come back to. */
export function closed<T extends string>(): SheetRun<T> {
  return { open: null, under: null };
}

/**
 * Raise a sheet over the page. Anything already up is replaced outright and
 * *not* remembered — this is "start here", not "go deeper".
 */
export function raise<T extends string>(_run: SheetRun<T>, next: T): SheetRun<T> {
  return { open: next, under: null };
}

/**
 * Hand over from the open sheet to another, remembering where to come back
 * to. With nothing open this is an ordinary `raise`; from an already
 * swapped-to sheet the earlier one is forgotten rather than stacked.
 */
export function swapTo<T extends string>(run: SheetRun<T>, next: T): SheetRun<T> {
  if (run.open === null) return raise(run, next);
  if (run.open === next) return run;
  return { open: next, under: run.open };
}

/**
 * Close the open sheet: back to whatever it was raised over, or to the bare
 * page. Dismissing the restored sheet then leaves for good.
 */
export function dismiss<T extends string>(run: SheetRun<T>): SheetRun<T> {
  if (run.open === null) return run;
  if (run.under !== null) return { open: run.under, under: null };
  return closed();
}

/** Leave sheets entirely, whatever is up — a screen push, say. */
export function closeAll<T extends string>(run: SheetRun<T>): SheetRun<T> {
  return run.open === null && run.under === null ? run : closed();
}

/** True while any sheet is on screen. What the body-scroll lock keys on. */
export function isUp<T extends string>(run: SheetRun<T>): boolean {
  return run.open !== null;
}
