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
// The panel travels from the opener's frame to its own and its window opens
// as it goes: the source's centre is mapped onto the target's, the crop is
// the source's rect, and both interpolate together. Travel matters most where
// the two are furthest apart — on desktop the dock is a bar along the bottom
// edge and the player is a centred stage, so a window that only grew would
// unroll over content already sitting in its final place, which reads as a
// panel being wiped in rather than a bar becoming a player.
//
// The frame moves by `clip-path` and translation, never a scale: scaling a
// sheet stretches its type and its radii on the way, and the text is the part
// the eye is on. The one element both surfaces share — the art — flies
// between the two positions, discounting the panel's own travel so its path
// is the one the eye expects rather than that plus the container's.

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
  };
}

/** The shared-element easing, as a CSS curve. Matches `cubicOut` below. */
const EASE = "cubic-bezier(0.33, 1, 0.68, 1)";

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
  // A window can never be bigger than the thing it is cut out of, and the
  // desktop dock is a full-width bar opening a narrower stage. Cropping that
  // centrally would drop both ends of the bar — including the end the art is
  // on, which is then flying in from outside its own window. So an oversized
  // opener is narrowed to the panel's size around its art first, and the
  // whole transform is built from the part that is actually being expanded.
  const o = { ...origin.box };
  const fit = (span: number, size: number, start: number, mid: number) => {
    if (size <= span) return start;
    const want = mid - span / 2;
    return Math.min(Math.max(want, start), start + size - span);
  };
  if (o.width > to.width) {
    o.left = fit(
      to.width,
      o.width,
      o.left,
      origin.art ? origin.art.left + origin.art.width / 2 : o.left,
    );
    o.width = to.width;
  }
  if (o.height > to.height) {
    o.top = fit(
      to.height,
      o.height,
      o.top,
      origin.art ? origin.art.top + origin.art.height / 2 : o.top,
    );
    o.height = to.height;
  }

  // The panel is moved so its centre sits on the opener's, and cropped to the
  // opener's size — which makes the crop symmetric, and means the window and
  // the panel behind it stay locked together for the whole run. Its own
  // transform (the desktop centring translate) is kept in front of ours;
  // percentages are already resolved in the computed matrix, so composing is
  // exact.
  const base = cs.transform && cs.transform !== "none" ? `${cs.transform} ` : "";
  const dx = o.left + o.width / 2 - (to.left + to.width / 2);
  const dy = o.top + o.height / 2 - (to.top + to.height / 2);
  // Crop, per side, at progress t: the window is the opener's rect, opening
  // to the panel's own.
  const cropX = (t: number) => ((to.width - o.width) * (1 - t)) / 2;
  const cropY = (t: number) => ((to.height - o.height) * (1 - t)) / 2;

  const r0 = origin.radius;
  const ms = out ? Math.round(duration * 0.75) : duration;

  // The shared element: the art the opener carried, flying to (or back to)
  // the size the panel lays it out at. FLIP, so no layout moves — and the
  // panel's own travel is taken out of the offset, because the art is inside
  // the panel and would otherwise be carried by it twice.
  const art = node.querySelector<HTMLElement>("[data-morph]");
  if (art && origin.art) {
    const at = art.getBoundingClientRect();
    if (at.width && at.height) {
      const sx = origin.art.width / at.width;
      const sy = origin.art.height / at.height;
      const r = parseFloat(getComputedStyle(art).borderTopLeftRadius) || 0;
      const small = {
        transformOrigin: "0 0",
        transform:
          `translate(${origin.art.left - at.left - dx}px, ${origin.art.top - at.top - dy}px)` +
          ` scale(${sx}, ${sy})`,
        // Divided by the scale, so the corners look the size they were at
        // both ends instead of shrinking with the box.
        borderRadius: `${Math.min(r / sx, at.width / 2)}px`,
      };
      const full = { transformOrigin: "0 0", transform: "none", borderRadius: `${r}px` };
      // Same curve either way — on the way out the panel's own progress is
      // the mirror of the intro's, so the art plays its keyframes forward
      // from full to small rather than running the intro in reverse.
      art.animate(out ? [full, small] : [small, full], {
        duration: ms,
        easing: EASE,
        fill: out ? "both" : "backwards",
      });
    }
  }

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
      const x = cropX(t);
      const y = cropY(t);
      return (
        `transform: ${base}translate(${dx * u}px, ${dy * u}px);` +
        ` clip-path: inset(${y}px ${x}px ${y}px ${x}px` +
        ` round ${rt}px ${rt}px ${rb}px ${rb}px);` +
        // Only the first frames: the window is opaque for the rest of the run.
        ` opacity: ${Math.min(1, t * 6)}`
      );
    },
  };
}
