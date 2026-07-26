import { tick as flushDOM } from "svelte";

/**
 * Putting a scroller back where the user left it.
 *
 * Music stacks up to two levels inside one route, and both of them unmount
 * what was underneath: a sheet swap replaces the sheet below it, and pushing
 * the Speakers screen replaces Home. Coming back has to mean coming back to
 * where you *were* — otherwise the row you tapped, which lives at the bottom
 * of Home, is off screen the moment you return from it (DESIGN.md §15).
 */

/**
 * Restore `top` once the returning content has **laid out**, not merely
 * rendered.
 *
 * One tick is too early: art and rows are still sizing, so the container's
 * scroll height is short and the browser clamps the offset to whatever fits
 * at that instant. So the offset is re-applied for a few frames until it
 * sticks, then abandoned — a target that hasn't taken after eight frames
 * isn't going to, and spinning on it would outlive the surface.
 *
 * `target` is a getter rather than an element because the caller usually
 * doesn't have the element yet: a swapped-in sheet's node only exists after
 * the flush below.
 */
export function settleScroll(target: () => Window | HTMLElement | null, top: number) {
  if (top <= 0) return;
  let tries = 0;
  const at = (el: Window | HTMLElement) =>
    el === window ? window.scrollY : (el as HTMLElement).scrollTop;
  const step = () => {
    const el = target();
    if (!el) return;
    el.scrollTo({ top, behavior: "instant" });
    if (Math.abs(at(el) - top) > 1 && tries++ < 8) requestAnimationFrame(step);
  };
  void flushDOM().then(() => requestAnimationFrame(step));
}

/** Put the page itself back where it was, once the screen has re-rendered. */
export function restoreScroll(top: number) {
  settleScroll(() => window, top);
}

/**
 * A screen change is a navigation, so it starts at the top — the same thing
 * the app shell does for a route change (App.svelte).
 */
export function toTop() {
  window.scrollTo({ top: 0, behavior: "instant" });
}
