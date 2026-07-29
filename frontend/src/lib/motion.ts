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
    breakpoint = 600,
  }: {
    duration?: number;
    instant?: boolean;
    mode?: "auto" | "slide" | "pop";
    breakpoint?: number;
  } = {},
): TransitionConfig {
  if (instant || reducedMotion) return { duration: 0 };
  const slide =
    mode === "slide" ||
    (mode === "auto" &&
      typeof window !== "undefined" &&
      window.matchMedia(`(max-width: ${breakpoint}px)`).matches);
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

// ── Container transform ────────────────────────────────────────────────
// A sheet that opens from a card, a bar or a tile should look like *that
// thing* growing — not like a second surface arriving from off-screen while
// the first fades. The dock and the player are the strongest case: they carry
// the same track, the same art and the same transport, so a cut between them
// reads as two players rather than one player at two sizes.
//
// The frame is animated with `clip-path`, never a scale: scaling a sheet
// stretches its type and its radii on the way, and the text is the part the
// eye is on. The window grows, the content is revealed, and the one element
// both surfaces share — the art — flies between the two positions.

/** A snapshot of the thing a sheet is growing out of, in viewport pixels. */
export interface Box {
  top: number;
  left: number;
  width: number;
  height: number;
}
export interface Origin {
  /** The frame itself. */
  box: Box;
  /** Its art thumbnail (`[data-morph]`), if it had one — the piece that flies. */
  art?: Box;
  /** Its corner radius, so the growing window starts the shape it left. */
  radius: number;
  /** When it was taken. The art only flies if it is still the same gesture. */
  at: number;
}

const boxOf = (r: DOMRect): Box => ({
  top: r.top,
  left: r.left,
  width: r.width,
  height: r.height,
});

/**
 * Measure what a sheet is about to grow out of. Called at the tap, because
 * the opener is usually gone by the time the sheet is mounted — and the
 * body's scroll is locked for the sheet's whole life, so the snapshot is
 * still true when it collapses back.
 */
export function originOf(el: HTMLElement | null | undefined): Origin | null {
  if (!el || reducedMotion) return null;
  const box = el.getBoundingClientRect();
  if (!box.width || !box.height) return null;
  const art = el.querySelector<HTMLElement>("[data-morph]")?.getBoundingClientRect();
  return {
    box: boxOf(box),
    art: art?.width ? boxOf(art) : undefined,
    radius: parseFloat(getComputedStyle(el).borderTopLeftRadius) || 0,
    at: Date.now(),
  };
}

/**
 * The sheet's own half of it: the panel is clipped to `origin` and unfolds to
 * its full frame, ending on the radii it actually has (so a phone sheet keeps
 * its square bottom corners and a desktop dialog rounds all four).
 *
 * Falls back to the plain `sheet` transition whenever there is nothing to grow
 * from — reached from a back gesture, from the keyboard, or under reduced
 * motion.
 */
export function grow(
  node: Element,
  {
    origin = null,
    out = false,
    instant = false,
    duration = 420,
    content = () => null,
    bar = () => null,
  }: {
    origin?: Origin | null;
    out?: boolean;
    instant?: boolean;
    duration?: number;
    /** The panel's scrolling content, faded with the window rather than sitting
     *  finished inside the opener's outline from the first frame. Read lazily:
     *  the binding lands after this runs. */
    content?: () => HTMLElement | null;
    /** Its frosted head, if it has one. */
    bar?: () => HTMLElement | null;
  } = {},
): TransitionConfig {
  if (instant || reducedMotion) return { duration: 0 };
  if (!origin) return sheet(node, { instant });
  const to = node.getBoundingClientRect();
  if (!to.width || !to.height) return sheet(node, { instant });

  const cs = getComputedStyle(node);
  const rTop = parseFloat(cs.borderTopLeftRadius) || 0;
  const rBottom = parseFloat(cs.borderBottomLeftRadius) || 0;
  const o = origin.box;
  // Insets are taken in the sheet's own box. Its only transform is the
  // desktop centering translate, and a translation preserves distances, so
  // viewport measurements carry over unchanged.
  const top = Math.max(0, o.top - to.top);
  const left = Math.max(0, o.left - to.left);
  const right = Math.max(0, to.right - (o.left + o.width));
  const bottom = Math.max(0, to.bottom - (o.top + o.height));
  const r0 = origin.radius;
  const ms = out ? Math.round(duration * 0.75) : duration;

  // Everything below is driven imperatively rather than by a class the
  // component toggles: Svelte pauses a removed subtree's effects before the
  // outro runs, so a class set from `onoutrostart` never reaches the DOM and
  // the panel would close with its content still fully lit.
  const el = node as HTMLElement;
  const head = bar();
  el.style.willChange = "clip-path, transform";
  if (head) {
    // A blurred backdrop escapes an animating clip in more than one engine,
    // and costs a repaint of the whole band on every frame of it. It is
    // inside a fading layer while the window moves; it takes the blur back
    // when the sheet settles.
    head.style.setProperty("backdrop-filter", "none");
    head.style.setProperty("-webkit-backdrop-filter", "none");
  }
  setTimeout(() => {
    el.style.willChange = "";
    if (head) {
      head.style.removeProperty("backdrop-filter");
      head.style.removeProperty("-webkit-backdrop-filter");
    }
    const c = content();
    if (c) c.style.opacity = "";
  }, ms + 60);

  const clamp = (v: number) => (v < 0 ? 0 : v > 1 ? 1 : v);
  // In: held back while the window is small, up to full around a third of the
  // way through. Out: away with the window, gone before it lands. Both are cut against the *eased* t the tick receives, not against
  // elapsed time — the curves above are steep enough at their fast end that
  // shaping this on the clock would put it up before the window had moved.
  const ink = (t: number) => (out ? clamp((t - 0.08) / 0.7) : clamp((t - 0.18) / 0.62));

  return {
    // Out is quicker: an exit that lingers reads as lag.
    duration: ms,
    // Decelerate both ways — the window has to leave under the tap that asked
    // it to, so an accelerating exit (which spends its first half barely
    // moving) reads as a control that didn't take. The art below rides the
    // same curve, so the two never pull apart mid-flight and clip each other.
    easing: cubicOut,
    tick: (t) => {
      const c = content();
      if (c) c.style.opacity = String(ink(t));
    },
    css: (t, u) => {
      const rt = r0 + (rTop - r0) * t;
      const rb = r0 + (rBottom - r0) * t;
      return (
        `clip-path: inset(${top * u}px ${right * u}px ${bottom * u}px ${left * u}px` +
        ` round ${rt}px ${rt}px ${rb}px ${rb}px);` +
        // Only the first frames: the window is opaque for the rest of the run.
        ` opacity: ${Math.min(1, t * 6)}`
      );
    },
  };
}

/**
 * The shared element: the opener's thumbnail flies to where the full-size art
 * has laid out. FLIP, so nothing about the layout moves — the art is drawn at
 * its final size from the first frame and transformed back to the small one.
 *
 * The radius is divided by the scale so the corners look the size they were
 * at both ends rather than shrinking with the box.
 */
export function morph(node: HTMLElement, origin?: Origin | null): void {
  const from = origin?.art;
  // Art that only arrives once the sheet has settled is not part of the
  // gesture any more; it appears where it is instead of flying in from a
  // dock the user left behind seconds ago.
  if (!from || !origin || Date.now() - origin.at > 900) return;
  if (reducedMotion || typeof node.animate !== "function") return;
  const to = node.getBoundingClientRect();
  if (!to.width || !to.height || !from.width) return;
  const sx = from.width / to.width;
  const sy = from.height / to.height;
  const r = parseFloat(getComputedStyle(node).borderTopLeftRadius) || 0;
  node.animate(
    [
      {
        transformOrigin: "0 0",
        transform:
          `translate(${from.left - to.left}px, ${from.top - to.top}px)` +
          ` scale(${sx}, ${sy})`,
        borderRadius: `${Math.min(r / sx, to.width / 2)}px`,
      },
      { transformOrigin: "0 0", transform: "none", borderRadius: `${r}px` },
    ],
    { duration: dur(420), easing: "cubic-bezier(0.33, 1, 0.68, 1)" },
  );
}
