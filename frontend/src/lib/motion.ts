// Motion helpers. Svelte's JS-driven transitions (fly/scale/flip) and the
// Tween class don't honour the prefers-reduced-motion CSS media query, so
// we gate their durations here instead.

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
