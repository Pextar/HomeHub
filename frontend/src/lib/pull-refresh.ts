import { haptic } from "./utils";

// Pull-to-refresh for touch devices, attached once to the view wrapper in
// App.svelte. When the window is scrolled to the very top and the user drags
// down, a small amber indicator follows the finger with resistance; crossing
// the threshold arms it (haptic tap), and releasing fires the refresh. It is
// a no-op on fine pointers (desktop) and never fights vertical scroll — the
// gesture only exists while the page is already at its top.
const THRESHOLD = 76;
const MAX_PULL = 130;

export function pullToRefresh(node: HTMLElement, onRefresh: () => Promise<unknown> | void) {
  if (typeof window === "undefined" || !window.matchMedia("(pointer: coarse)").matches) {
    return {};
  }

  let startY = 0;
  let pulling = false;
  let armed = false;
  let refreshing = false;
  let indicator: HTMLDivElement | null = null;

  function ensureIndicator(): HTMLDivElement {
    if (!indicator) {
      indicator = document.createElement("div");
      indicator.className = "ptr-indicator";
      indicator.innerHTML = '<span class="ptr-dot"></span>';
      indicator.setAttribute("aria-hidden", "true");
      document.body.appendChild(indicator);
    }
    return indicator;
  }

  function setPull(px: number) {
    const el = ensureIndicator();
    // -56 hides the pill above the viewport edge; the pull slides it in.
    el.style.transform = `translate(-50%, ${px - 56}px)`;
    el.classList.toggle("armed", px >= THRESHOLD);
  }

  function dismiss() {
    if (!indicator) return;
    indicator.style.transform = "translate(-50%, -56px)";
    indicator.classList.remove("armed", "busy");
  }

  function onTouchStart(e: TouchEvent) {
    if (refreshing || window.scrollY > 0 || e.touches.length !== 1) return;
    startY = e.touches[0].clientY;
    pulling = true;
    armed = false;
  }

  function onTouchMove(e: TouchEvent) {
    if (!pulling || refreshing) return;
    if (window.scrollY > 0) {
      pulling = false;
      dismiss();
      return;
    }
    const dy = e.touches[0].clientY - startY;
    if (dy <= 0) {
      dismiss();
      return;
    }
    // Resistance curve: the pill tracks at ~half finger speed, capped.
    const pull = Math.min(MAX_PULL, dy * 0.5);
    setPull(pull);
    if (pull >= THRESHOLD && !armed) {
      armed = true;
      haptic();
    }
    if (pull < THRESHOLD) armed = false;
  }

  async function onTouchEnd() {
    if (!pulling) return;
    pulling = false;
    if (!armed || refreshing) {
      dismiss();
      return;
    }
    armed = false;
    refreshing = true;
    const el = ensureIndicator();
    el.classList.add("busy");
    try {
      await onRefresh();
    } finally {
      refreshing = false;
      dismiss();
    }
  }

  // touchmove must be non-passive only in spirit — we never preventDefault,
  // so the native rubber-band coexists with the indicator. passive stays true.
  node.addEventListener("touchstart", onTouchStart, { passive: true });
  node.addEventListener("touchmove", onTouchMove, { passive: true });
  node.addEventListener("touchend", onTouchEnd, { passive: true });
  node.addEventListener("touchcancel", onTouchEnd, { passive: true });

  return {
    destroy() {
      node.removeEventListener("touchstart", onTouchStart);
      node.removeEventListener("touchmove", onTouchMove);
      node.removeEventListener("touchend", onTouchEnd);
      node.removeEventListener("touchcancel", onTouchEnd);
      indicator?.remove();
      indicator = null;
    },
  };
}
