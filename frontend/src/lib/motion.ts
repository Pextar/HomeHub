// Motion helpers. Svelte's JS-driven transitions (fly/scale/flip) and the
// Tween class don't honour the prefers-reduced-motion CSS media query, so
// we gate their durations here instead.

import { cubicOut } from "svelte/easing";
import type { TransitionConfig } from "svelte/transition";

export const reducedMotion =
  typeof window !== "undefined" &&
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

/** Duration in ms, collapsed to 0 when the user prefers reduced motion. */
export const dur = (ms: number): number => (reducedMotion ? 0 : ms);

/**
 * Staggered entrance delay for list items, capped so long lists don't
 * crawl in one-by-one. `i` is the item index.
 */
export const stagger = (i: number, step = 35, cap = 6): number =>
  reducedMotion ? 0 : Math.min(i, cap) * step;

/**
 * Sheet/dialog transition that adapts to the viewport:
 *   - Mobile (≤600px): slides up from the bottom edge, travelling its own
 *     full height so the panel always clears the screen regardless of how
 *     tall its content is — the iOS bottom-sheet feel.
 *   - Desktop: a gentle fade + lift + scale "pop".
 *
 * `instant` short-circuits to a zero-duration transition. The drag-to-dismiss
 * handlers use it on the way out: the panel has already been slid off-screen
 * by hand, so re-animating it from the open position would snap it back up
 * before flinging it down again.
 */
export function sheet(
  _node: Element,
  {
    duration = 340,
    instant = false,
    mode = "auto",
  }: { duration?: number; instant?: boolean; mode?: "auto" | "slide" | "pop" } = {},
): TransitionConfig {
  if (instant || reducedMotion) return { duration: 0 };
  const slide =
    mode === "slide" ||
    (mode === "auto" &&
      typeof window !== "undefined" &&
      window.matchMedia("(max-width: 600px)").matches);
  if (slide) {
    return {
      duration,
      easing: cubicOut,
      // u = 1 - t: 1 when closed (fully below), 0 when open.
      css: (t, u) =>
        `transform: translateY(${u * 100}%); opacity: ${Math.min(1, t * 1.6)}`,
    };
  }
  return {
    duration: Math.min(duration, 220),
    easing: cubicOut,
    css: (t, u) =>
      `opacity: ${t}; transform: translateY(${u * 8}px) scale(${1 - u * 0.02})`,
  };
}
