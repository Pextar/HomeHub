import type { Room } from "./rooms.svelte";

/**
 * The grouping gesture: drag one room onto another.
 *
 * Unchanged in feel from the version that only ever moved Sonos pucks around a
 * sheet — the hold, the ghost, the edge auto-scroll and the keyboard path are
 * the parts of this that were right — but it now works on **rooms**, on the
 * home screen, where the rooms already are. Grouping used to live one sheet
 * deep, in a grid of small tiles that were a second, dimmer copy of the room
 * list above them; there is only one list now, and the gesture happens on it.
 *
 * What it can group is not this module's business. It asks `canDrop`, draws a
 * ring where the answer is yes, and hands the pair to `group` — which decides
 * between Sonos' own grouping and a HomeHub zone (see `rooms.svelte.ts`).
 *
 * It stays a module rather than part of a component because the ghost has to
 * render *outside* any sheet: `.sheet` takes a transform while it is being
 * dragged, which would re-anchor a `position: fixed` descendant to it.
 */

/** A card in flight, in viewport coordinates. */
export interface RoomDrag {
  key: string;
  name: string;
  sub: string;
  playing: boolean;
  w: number;
  h: number;
  offX: number;
  offY: number;
  x: number;
  y: number;
}

interface Pending {
  room: Room;
  el: HTMLElement;
  pid: number;
  startX: number;
  startY: number;
  touch: boolean;
}

const LIFT_PX = 8; // below this it's a tap, not a drag
const HOLD_MS = 260; // touch presses lift on a hold, so the page still scrolls
const EDGE_PX = 72;
const EDGE_MAX = 16; // px per frame at the very edge

export interface RoomDragOptions {
  /** The scroll container to auto-scroll near the edges of. */
  scroller: () => HTMLElement | null;
  /** Find a room by the key a DOM node carries. */
  roomOf: (key: string) => Room | null;
  /** Whether this pair would actually do something. */
  canDrop: (source: Room, target: Room) => boolean;
  /** Do the grouping. Called once, on drop. */
  group: (source: Room, target: Room) => void;
  /** Say something a screen reader can hear. */
  announce: (msg: string) => void;
  /** What the ghost says under the name. */
  describe: (r: Room) => { playing: boolean; sub: string };
}

export function createRoomDrag(opts: RoomDragOptions) {
  const s = $state({
    /** The card in flight, or null. */
    drag: null as RoomDrag | null,
    /** The room under the pointer, when it is a legal target. */
    dropKey: null as string | null,
    /** The room held by the keyboard, awaiting a drop. */
    grabKey: null as string | null,
  });

  let pending: Pending | null = null;
  let holdTimer: ReturnType<typeof setTimeout> | undefined;
  /** True from the lift until the click it would otherwise fire is eaten. */
  let consumedClick = false;
  let lastPoint: { x: number; y: number } | null = null;
  let scrollStep = 0;
  let scrollFrame: number | undefined;

  /**
   * While a card is lifted the page must not scroll under it. `touch-action`
   * can't be changed mid-gesture, so the scroll is refused the only way it can
   * be once a pointer is already down: a non-passive touchmove listener.
   */
  function blockTouchScroll(e: TouchEvent) {
    e.preventDefault();
  }

  function aimAt(x: number, y: number) {
    if (!s.drag) return;
    const source = opts.roomOf(s.drag.key);
    const under = document.elementFromPoint(x, y);
    const hit = (under?.closest?.("[data-room]") as HTMLElement | null)?.dataset.room ?? null;
    const target = hit ? opts.roomOf(hit) : null;
    s.dropKey = source && target && opts.canDrop(source, target) ? target.key : null;
  }

  function edgeScroll(y: number) {
    const el = opts.scroller();
    if (!el || !s.drag) return (scrollStep = 0);
    const r = el === document.scrollingElement ? viewportRect() : el.getBoundingClientRect();
    const over = y - (r.bottom - EDGE_PX);
    const under = r.top + EDGE_PX - y;
    scrollStep =
      over > 0 ? Math.min(1, over / EDGE_PX) * EDGE_MAX
      : under > 0 ? -Math.min(1, under / EDGE_PX) * EDGE_MAX
      : 0;
    if (scrollStep !== 0 && scrollFrame === undefined) stepScroll();
  }
  /** The window's own scroll box, when the scroller is the page itself. */
  function viewportRect() {
    return { top: 0, bottom: window.innerHeight } as DOMRect;
  }
  function stepScroll() {
    scrollFrame = requestAnimationFrame(() => {
      scrollFrame = undefined;
      const el = opts.scroller();
      if (!s.drag || !el || scrollStep === 0) return;
      const before = el.scrollTop;
      el.scrollTop += scrollStep;
      if (el.scrollTop === before) return; // hit the end — nothing to chase
      // The ghost is pinned to the viewport, so scrolling moves a different
      // card under a finger that hasn't budged.
      if (lastPoint) aimAt(lastPoint.x, lastPoint.y);
      stepScroll();
    });
  }
  function stopEdgeScroll() {
    if (scrollFrame !== undefined) cancelAnimationFrame(scrollFrame);
    scrollFrame = undefined;
    scrollStep = 0;
    lastPoint = null;
  }

  function lift(x: number, y: number) {
    if (!pending || s.drag) return;
    const { room, el, pid } = pending;
    const r = el.getBoundingClientRect();
    try {
      el.setPointerCapture(pid);
    } catch {
      // The pointer may already be gone — the drag still works without
      // capture, it just ends on the first pointerup we see.
    }
    document.addEventListener("touchmove", blockTouchScroll, { passive: false });
    const { playing, sub } = opts.describe(room);
    s.drag = {
      key: room.key,
      name: room.name,
      playing,
      sub,
      w: r.width,
      h: r.height,
      offX: x - r.left,
      offY: y - r.top,
      x: r.left,
      y: r.top,
    };
    opts.announce(`${room.name} lifted. Drop it on another room to play them together.`);
  }

  function cancelPending() {
    clearTimeout(holdTimer);
    pending = null;
  }

  function end() {
    clearTimeout(holdTimer);
    if (pending && s.drag) {
      try {
        pending.el.releasePointerCapture(pending.pid);
      } catch {
        // Already released with the pointer.
      }
    }
    if (s.drag) document.removeEventListener("touchmove", blockTouchScroll);
    stopEdgeScroll();
    pending = null;
    s.drag = null;
    s.dropKey = null;
  }

  function dropGrab(target: Room) {
    const src = s.grabKey;
    s.grabKey = null;
    if (!src) return;
    const source = opts.roomOf(src);
    if (!source) return;
    if (!opts.canDrop(source, target)) {
      opts.announce(`${source.name} already plays with ${target.name}.`);
      return;
    }
    opts.group(source, target);
  }

  return {
    get drag() {
      return s.drag;
    },
    get dropKey() {
      return s.dropKey;
    },
    get grabKey() {
      return s.grabKey;
    },
    /** The held room's name, for the "Drop it on…" copy. */
    get grabbedName() {
      return s.grabKey ? (opts.roomOf(s.grabKey)?.name ?? "") : "";
    },
    /** Whether this card would take the held room — what draws the ring. */
    aiming(r: Room): boolean {
      if (!s.grabKey || s.grabKey === r.key) return false;
      const source = opts.roomOf(s.grabKey);
      return !!source && opts.canDrop(source, r);
    },

    onPointerDown(e: PointerEvent, room: Room) {
      if (e.button !== 0 || s.drag) return;
      const el = e.currentTarget as HTMLElement;
      const touch = e.pointerType !== "mouse";
      pending = { room, el, pid: e.pointerId, startX: e.clientX, startY: e.clientY, touch };
      if (touch) {
        // A quick swipe is a scroll; a press that stays put is a lift.
        clearTimeout(holdTimer);
        holdTimer = setTimeout(() => lift(e.clientX, e.clientY), HOLD_MS);
      }
    },

    onPointerMove(e: PointerEvent) {
      if (!pending) return;
      const dx = e.clientX - pending.startX;
      const dy = e.clientY - pending.startY;
      const moved = Math.hypot(dx, dy);
      if (!s.drag) {
        if (pending.touch) {
          // Moved before the hold landed — that was a scroll.
          if (moved > LIFT_PX) cancelPending();
        } else if (moved > LIFT_PX) {
          lift(e.clientX, e.clientY);
        }
        if (!s.drag) return;
      }
      e.preventDefault();
      s.drag.x = e.clientX - s.drag.offX;
      s.drag.y = e.clientY - s.drag.offY;
      lastPoint = { x: e.clientX, y: e.clientY };
      aimAt(e.clientX, e.clientY);
      edgeScroll(e.clientY);
    },

    onPointerUp() {
      if (!pending) return;
      const sourceKey = s.drag?.key;
      const targetKey = s.dropKey;
      if (s.drag) consumedClick = true;
      end();
      if (!sourceKey || !targetKey) return;
      const source = opts.roomOf(sourceKey);
      const target = opts.roomOf(targetKey);
      if (source && target) opts.group(source, target);
    },

    /** Swallow the click a finished drag would otherwise fire on the card. */
    onClickCapture(e: MouseEvent) {
      if (!consumedClick) return;
      consumedClick = false;
      e.preventDefault();
      e.stopPropagation();
    },

    /**
     * G picks a room up, Tab moves, Enter drops it in, Escape puts it back.
     * Stated once as a footnote under the grid, not as chrome on every card.
     */
    onKeyDown(e: KeyboardEvent, room: Room) {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
      if (key === "g") {
        e.preventDefault();
        if (s.grabKey === null) {
          s.grabKey = room.key;
          opts.announce(
            `${room.name} picked up. Move to another room and press Enter to group, ` +
              `Escape to put it back.`,
          );
        } else if (s.grabKey === room.key) {
          s.grabKey = null;
          opts.announce(`${room.name} put back.`);
        } else {
          dropGrab(room);
        }
        return;
      }
      // Enter on a card normally focuses that room. While something is held it
      // means "drop it here" instead — so the default activation has to be
      // stopped before it fires a click.
      if (key === "Enter" && s.grabKey !== null && s.grabKey !== room.key) {
        e.preventDefault();
        dropGrab(room);
      }
    },

    /** Put down whatever is held, by pointer or by keyboard. */
    release() {
      s.grabKey = null;
      end();
    },

    /** Let go of a room that stopped existing mid-gesture. */
    prune(live: Set<string>) {
      if (s.grabKey && !live.has(s.grabKey)) s.grabKey = null;
      if (s.drag && !live.has(s.drag.key)) end();
    },

    end,
  };
}
