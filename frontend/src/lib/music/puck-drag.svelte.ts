import type { SonosSpeakerView } from "../types";

/**
 * The grouping gesture: drag one room onto another.
 *
 * Dragging one thing onto another *is* the grouping gesture (DESIGN.md §15);
 * the tap-to-select-then-"Group" flow this replaced needed a whole selection
 * mode, with its own way out, to say the same thing. So there is no second
 * control on a puck, and everything grouping needs lives here:
 *
 *   - the pointer drag, with a ghost that follows the finger;
 *   - the **hold** that starts it on touch, because a finger that moves
 *     straight away is scrolling the sheet;
 *   - edge auto-scroll, because the grid outgrows the sheet at a few rooms
 *     and a target must not become unreachable with the finger already down;
 *   - the keyboard equivalent (G to pick up, Enter to drop, Escape to put
 *     back), because a pointer-only gesture would leave grouping with no
 *     keyboard path at all;
 *   - the live-region messages, because a drag has no running commentary.
 *
 * It is a module rather than part of the Zones sheet because the ghost has to
 * render *outside* that sheet: `.sheet` takes a transform while it is being
 * dragged, which would re-anchor a `position: fixed` descendant to it.
 */

/** A puck in flight, in viewport coordinates. */
export interface PuckDrag {
    id: string;
    name: string;
    playing: boolean;
    sub: string;
    w: number;
    h: number;
    offX: number;
    offY: number;
    x: number;
    y: number;
}

interface Pending {
    sp: SonosSpeakerView;
    el: HTMLElement;
    pid: number;
    startX: number;
    startY: number;
    touch: boolean;
}

const LIFT_PX = 8; // below this it's a tap, not a drag
const HOLD_MS = 260; // touch presses lift on a hold, so the sheet still scrolls
const EDGE_PX = 72;
const EDGE_MAX = 16; // px per frame at the very edge

export interface PuckDragOptions {
    /** The sheet's scroll container, for edge auto-scroll. */
    scroller: () => HTMLElement | null;
    /** Which zone a speaker currently belongs to, so it can't re-join it. */
    zoneOf: (speakerId: string) => string | undefined;
    /** Whether a room is playing, and what it is playing — for the ghost. */
    describe: (speakerId: string) => { playing: boolean; sub: string };
    /** Do the grouping. Called once, on drop. */
    group: (sourceId: string, targetId: string) => void;
    /** Say something a screen reader can hear. */
    announce: (msg: string) => void;
}

export function createPuckDrag(opts: PuckDragOptions) {
    const s = $state({
        /** The puck in flight, or null. */
        drag: null as PuckDrag | null,
        /** The room under the pointer, if it's a room. */
        dropId: null as string | null,
        /** The zone under the pointer, when the pointer is on its enclosure. */
        dropZone: null as string | null,
        /** The room held by the keyboard, awaiting a drop. */
        grabId: null as string | null,
    });

    let pending: Pending | null = null;
    let holdTimer: ReturnType<typeof setTimeout> | undefined;
    /** True from the lift until the click it would otherwise fire is eaten. */
    let consumedClick = false;
    let lastPoint: { x: number; y: number } | null = null;
    let scrollStep = 0;
    let scrollFrame: number | undefined;

    /**
     * While a puck is lifted the page must not scroll under it. `touch-action`
     * can't be changed mid-gesture, so the scroll is refused the only way it
     * can be once a pointer is already down: a non-passive touchmove listener.
     */
    function blockTouchScroll(e: TouchEvent) {
        e.preventDefault();
    }

    /**
     * What the pointer is over: a room, or the enclosure around an existing
     * zone. The enclosure counts because "drag a third onto an existing group
     * adds it" reads as dropping on the *group* — landing in the gap between
     * its pucks shouldn't be a miss.
     */
    function aimAt(x: number, y: number) {
        if (!s.drag) return;
        const under = document.elementFromPoint(x, y);
        const hit = under?.closest?.(".puck, .group-wrap") as HTMLElement | null;
        // A room already sharing the dragged one's zone can't be dropped onto
        // — the same "already together" case the enclosure check below has
        // always excluded. Without this a puck inside your own group still
        // rang as a valid target, and dropping there silently did nothing.
        const mine = opts.zoneOf(s.drag.id);
        const speaker = hit?.dataset.speaker ?? null;
        if (speaker) {
            s.dropId = speaker !== s.drag.id && opts.zoneOf(speaker) !== mine ? speaker : null;
            s.dropZone = null;
            return;
        }
        const zone = hit?.dataset.zone ?? null;
        // A room already in this zone can't be dropped into it again.
        s.dropZone = zone && zone !== mine ? zone : null;
        s.dropId = null;
    }

    function edgeScroll(y: number) {
        const el = opts.scroller();
        if (!el || !s.drag) return (scrollStep = 0);
        const r = el.getBoundingClientRect();
        const over = y - (r.bottom - EDGE_PX);
        const under = r.top + EDGE_PX - y;
        scrollStep =
            over > 0 ? Math.min(1, over / EDGE_PX) * EDGE_MAX
            : under > 0 ? -Math.min(1, under / EDGE_PX) * EDGE_MAX
            : 0;
        if (scrollStep !== 0 && scrollFrame === undefined) stepScroll();
    }
    function stepScroll() {
        scrollFrame = requestAnimationFrame(() => {
            scrollFrame = undefined;
            const el = opts.scroller();
            if (!s.drag || !el || scrollStep === 0) return;
            const before = el.scrollTop;
            el.scrollTop += scrollStep;
            if (el.scrollTop === before) return; // hit the end — nothing to chase
            // The ghost is pinned to the viewport, so scrolling moves a
            // different room under a finger that hasn't budged.
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
        const { sp, el, pid } = pending;
        const r = el.getBoundingClientRect();
        try {
            el.setPointerCapture(pid);
        } catch {
            // The pointer may already be gone — the drag still works without
            // capture, it just ends on the first pointerup we see.
        }
        document.addEventListener("touchmove", blockTouchScroll, { passive: false });
        const { playing, sub } = opts.describe(sp.id);
        s.drag = {
            id: sp.id,
            name: sp.name,
            playing,
            sub,
            w: r.width,
            h: r.height,
            offX: x - r.left,
            offY: y - r.top,
            x: r.left,
            y: r.top,
        };
        opts.announce(`${sp.name} lifted. Drop it on another room to group them.`);
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
        s.dropId = null;
        s.dropZone = null;
    }

    function dropGrab(sp: SonosSpeakerView) {
        const src = s.grabId;
        s.grabId = null;
        if (!src) return;
        // Same check the pointer path's ring already draws: a room already
        // sharing this one's zone can't be dropped onto it again. Without
        // this a keyboard drop here just went silent — no toast, no
        // live-region message, nothing the pointer's ring wasn't already
        // steering people away from.
        if (src !== sp.id && opts.zoneOf(src) === opts.zoneOf(sp.id)) {
            opts.announce(`${sp.name} is already grouped with that room.`);
            return;
        }
        opts.group(src, sp.id);
    }

    return {
        get drag() {
            return s.drag;
        },
        get dropId() {
            return s.dropId;
        },
        get dropZone() {
            return s.dropZone;
        },
        get grabId() {
            return s.grabId;
        },
        /** The held room's name, for the "Drop X here" copy. */
        grabbedName: (nameOf: (id: string) => string | undefined) =>
            s.grabId ? (nameOf(s.grabId) ?? "") : "",

        onPointerDown(e: PointerEvent, sp: SonosSpeakerView) {
            if (e.button !== 0 || s.drag) return;
            const el = e.currentTarget as HTMLElement;
            const touch = e.pointerType !== "mouse";
            pending = { sp, el, pid: e.pointerId, startX: e.clientX, startY: e.clientY, touch };
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
            const src = s.drag?.id;
            const target = s.dropId ?? s.dropZone;
            if (s.drag) consumedClick = true;
            end();
            if (src && target) opts.group(src, target);
        },

        /** Swallow the click a finished drag would otherwise fire on the puck. */
        onClickCapture(e: MouseEvent) {
            if (!consumedClick) return;
            consumedClick = false;
            e.preventDefault();
            e.stopPropagation();
        },

        /**
         * G picks a room up, Tab moves, Enter drops it in, Escape puts it
         * back. Stated once as a footnote at the bottom of the sheet, not as
         * chrome on every puck.
         */
        onKeyDown(e: KeyboardEvent, sp: SonosSpeakerView, nameOf: (id: string) => string | undefined) {
            if (e.metaKey || e.ctrlKey || e.altKey) return;
            const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
            if (key === "g") {
                e.preventDefault();
                if (s.grabId === null) {
                    s.grabId = sp.id;
                    opts.announce(
                        `${sp.name} picked up. Move to another room and press Enter to group, ` +
                            `Escape to put it back.`,
                    );
                } else if (s.grabId === sp.id) {
                    s.grabId = null;
                    opts.announce(`${sp.name} put back.`);
                } else {
                    dropGrab(sp);
                }
                return;
            }
            // Enter on a puck normally opens that room's player. While
            // something is held it means "drop it here" instead — so the
            // default activation has to be stopped before it fires a click.
            if (key === "Enter" && s.grabId !== null && s.grabId !== sp.id) {
                e.preventDefault();
                dropGrab(sp);
            }
            void nameOf;
        },

        /** Put down whatever is held, by pointer or by keyboard. */
        release() {
            s.grabId = null;
            end();
        },

        /** Let go of a room that dropped off the network mid-gesture. */
        prune(live: Set<string>) {
            if (s.grabId && !live.has(s.grabId)) s.grabId = null;
            if (s.drag && !live.has(s.drag.id)) end();
        },

        end,
    };
}
