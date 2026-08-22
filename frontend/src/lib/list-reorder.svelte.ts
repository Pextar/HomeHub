/**
 * Reordering a vertical list, by hand or by keyboard.
 *
 * The grouping drag (`lib/music/room-drag.svelte.ts`) aims one card at
 * another; this one aims a row at a *gap*, which is a different problem and
 * gets a different mechanism:
 *
 * - **The handle is the gesture.** A row lifts the moment a pointer goes down
 *   on its grip, with no hold timer — the grip exists only while the list is
 *   being arranged, so there is nothing for a press on it to be ambiguous
 *   with. (The room grid has the opposite problem: the whole card is also the
 *   thing you tap and scroll past, which is what the 260ms hold buys.)
 *   `touch-action: none` on the grip is what keeps the page still under it.
 * - **Nothing moves in the DOM until the drop.** The lifted row rides the
 *   finger and the rows it has passed slide one slot out of its way, all of it
 *   `transform` on the compositor. Reordering the array mid-drag instead would
 *   mean re-anchoring the finger to a row that just jumped under it on every
 *   crossing, and fighting the list's own FLIP animation for the same element.
 * - **The keyboard doesn't emulate the drag.** Arrow keys on a focused grip
 *   move the row one slot per press and say where it landed. A pick-up/put-
 *   down mode would be a truer mirror of the pointer gesture and a worse way
 *   to move a row three places.
 *
 * Row heights are read from the DOM at lift rather than assumed, but the
 * arithmetic does take the list to be evenly spaced — which is what makes a
 * pointer offset a slot count. It is a list of identical rows; a list of
 * mixed-height items would need per-row midpoints instead.
 */

export interface ListReorderOptions {
  /** The list, in its current order. */
  ids: () => string[];
  /** Commit a move: put `id` at index `to`. Called once, on drop or key. */
  move: (id: string, to: number) => void;
  /** The row element for an id, so the gesture can measure the list. */
  rowOf: (id: string) => HTMLElement | null;
  /** What to call a row when saying what happened to it. */
  label: (id: string) => string;
  /** Say something a screen reader can hear. */
  announce: (msg: string) => void;
}

const EDGE_PX = 96; // how close to an edge starts the auto-scroll
const EDGE_MAX = 18; // px per frame at the very edge

export function createListReorder(opts: ListReorderOptions) {
  const s = $state({
    /** The row in flight, or null. */
    lifted: null as string | null,
    /** Where it started, and where it would land right now. */
    from: 0,
    to: 0,
    /** How far the finger has taken it, in px. */
    dy: 0,
    /** One frame after a drop, while the DOM catches up with the screen. */
    settling: false,
  });

  let rowH = 0;
  let startY = 0;
  let startScroll = 0;
  let lastY = 0;
  let pid = -1;
  let el: HTMLElement | null = null;
  let scrollStep = 0;
  let scrollFrame: number | undefined;
  /** True from the lift until the click the drag would otherwise fire is eaten. */
  let consumedClick = false;

  /** The pitch of the list: one row plus whatever gap sits under it. */
  function measure(id: string): number {
    const row = opts.rowOf(id);
    if (!row) return 0;
    const ids = opts.ids();
    const i = ids.indexOf(id);
    const neighbour = opts.rowOf(ids[i + 1] ?? ids[i - 1] ?? "");
    const self = row.getBoundingClientRect();
    if (neighbour) {
      const other = neighbour.getBoundingClientRect();
      const pitch = Math.abs(other.top - self.top);
      if (pitch > 1) return pitch;
    }
    return self.height;
  }

  function aim() {
    if (!s.lifted || rowH <= 0) return;
    const slots = Math.round(s.dy / rowH);
    const last = opts.ids().length - 1;
    s.to = Math.max(0, Math.min(last, s.from + slots));
  }

  function edgeScroll(clientY: number) {
    if (!s.lifted) return;
    const over = clientY - (window.innerHeight - EDGE_PX);
    const under = EDGE_PX - clientY;
    scrollStep =
      over > 0 ? Math.min(1, over / EDGE_PX) * EDGE_MAX
      : under > 0 ? -Math.min(1, under / EDGE_PX) * EDGE_MAX
      : 0;
    if (scrollStep !== 0 && scrollFrame === undefined) stepScroll();
  }
  function stepScroll() {
    scrollFrame = requestAnimationFrame(() => {
      scrollFrame = undefined;
      if (!s.lifted || scrollStep === 0) return;
      const before = window.scrollY;
      window.scrollBy(0, scrollStep);
      if (window.scrollY === before) return; // hit the end — nothing to chase
      // The finger hasn't moved but the list has, so the row is now over a
      // different gap. Recompute from the page position, not the viewport one.
      s.dy = lastY + window.scrollY - (startY + startScroll);
      aim();
      stepScroll();
    });
  }
  function stopEdgeScroll() {
    if (scrollFrame !== undefined) cancelAnimationFrame(scrollFrame);
    scrollFrame = undefined;
    scrollStep = 0;
  }

  function reset() {
    stopEdgeScroll();
    if (el && pid >= 0) {
      try {
        el.releasePointerCapture(pid);
      } catch {
        // Already released with the pointer.
      }
    }
    el = null;
    pid = -1;
    s.lifted = null;
    s.dy = 0;
    s.from = 0;
    s.to = 0;
  }

  /** Say where a row ended up. The position is what matters, not the delta. */
  function announceMove(id: string, to: number) {
    opts.announce(`${opts.label(id)} moved to position ${to + 1} of ${opts.ids().length}.`);
  }

  return {
    get lifted() {
      return s.lifted;
    },
    get settling() {
      return s.settling;
    },

    /**
     * How far this row is currently displaced, in px. The lifted row follows
     * the finger; every row between where it came from and where it is going
     * steps one slot the other way.
     */
    offset(id: string): number {
      if (!s.lifted) return 0;
      if (id === s.lifted) return s.dy;
      const i = opts.ids().indexOf(id);
      if (i < 0) return 0;
      if (s.to > s.from && i > s.from && i <= s.to) return -rowH;
      if (s.to < s.from && i >= s.to && i < s.from) return rowH;
      return 0;
    },

    onPointerDown(e: PointerEvent, id: string) {
      if (e.button !== 0 || s.lifted) return;
      e.preventDefault(); // the grip is a button; don't start a text selection
      el = e.currentTarget as HTMLElement;
      pid = e.pointerId;
      try {
        el.setPointerCapture(pid);
      } catch {
        // The pointer may already be gone — the drag still ends on pointerup.
      }
      rowH = measure(id);
      startY = e.clientY;
      startScroll = window.scrollY;
      lastY = e.clientY;
      s.from = opts.ids().indexOf(id);
      s.to = s.from;
      s.dy = 0;
      s.lifted = id;
      opts.announce(`${opts.label(id)} lifted. Drag to move it, let go to drop it.`);
    },

    onPointerMove(e: PointerEvent) {
      if (!s.lifted) return;
      lastY = e.clientY;
      s.dy = e.clientY + window.scrollY - (startY + startScroll);
      aim();
      edgeScroll(e.clientY);
    },

    onPointerUp() {
      if (!s.lifted) return;
      const id = s.lifted;
      const to = s.to;
      const moved = to !== s.from;
      consumedClick = true;
      // Everything is already where it will be — the lifted row is under the
      // finger and the others have stepped aside. Committing the order swaps
      // the DOM to match, so transitions are off for that one frame or every
      // row would animate from its shoved position back to the same place.
      s.settling = true;
      reset();
      if (moved) {
        opts.move(id, to);
        announceMove(id, to);
      }
      requestAnimationFrame(() => {
        s.settling = false;
      });
    },

    /** Swallow the click a finished drag fires on the grip. */
    onClickCapture(e: MouseEvent) {
      if (!consumedClick) return;
      consumedClick = false;
      e.preventDefault();
      e.stopPropagation();
    },

    /** Up and down move the row a slot at a time; focus rides along with it. */
    onKeyDown(e: KeyboardEvent, id: string) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const delta = e.key === "ArrowUp" ? -1 : e.key === "ArrowDown" ? 1 : 0;
      if (delta === 0) return;
      e.preventDefault();
      const ids = opts.ids();
      const from = ids.indexOf(id);
      const to = from + delta;
      if (from < 0 || to < 0 || to >= ids.length) return;
      opts.move(id, to);
      announceMove(id, to);
    },

    /** Let go of a row that stopped existing mid-gesture. */
    prune(live: Set<string>) {
      if (s.lifted && !live.has(s.lifted)) reset();
    },

    end: reset,
  };
}
