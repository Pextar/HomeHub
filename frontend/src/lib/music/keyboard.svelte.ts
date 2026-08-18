/**
 * How much of the screen the software keyboard is eating.
 *
 * Two surfaces lay themselves out around this and both had their own copy:
 * the wall's music depth (the iPad's docked keyboard takes ~350pt off the
 * bottom, which left the results a one-row strip) and the kid module's search
 * (the results go dense, the sticky pane chips stand down, and the mini bar
 * hides rather than sit behind the keys).
 *
 * `visualViewport` measures the real thing — docked, floating or split — and
 * degrades to zero where there is no software keyboard at all, so a desktop
 * simply never reports one.
 *
 * The two copies had already drifted: only one of them took a reading when it
 * mounted, so the other stayed at zero until the next resize or scroll. That
 * is not hypothetical on the wall, where the depth is a route away and coming
 * back to it with the keyboard already up is the ordinary case. Measuring on
 * mount is the shared behaviour.
 */

/** Below this, it isn't a keyboard — it's a toolbar, or the URL bar
 *  collapsing on scroll. */
const KEYBOARD_MIN_PX = 150;

export interface SoftKeyboard {
  /** Pixels of viewport the keyboard is covering. Zero where there is none. */
  readonly height: number;
  /** Enough of the screen is gone that a layout should answer to it. */
  readonly open: boolean;
}

/**
 * Call from component init. The listeners live for as long as the effect
 * root that created them — a component, in practice.
 */
export function createSoftKeyboard(): SoftKeyboard {
  let height = $state(0);

  $effect(() => {
    const vv = window.visualViewport;
    if (!vv) return;
    const measure = () => {
      height = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
    };
    // Once now: a surface can mount with the keyboard already up.
    measure();
    vv.addEventListener("resize", measure);
    vv.addEventListener("scroll", measure);
    return () => {
      vv.removeEventListener("resize", measure);
      vv.removeEventListener("scroll", measure);
    };
  });

  return {
    get height() {
      return height;
    },
    get open() {
      return height > KEYBOARD_MIN_PX;
    },
  };
}
