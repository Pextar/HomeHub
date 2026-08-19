/**
 * A volume slider's own value, held against the poll.
 *
 * Every bridge in the module had this: a level the finger just set, kept
 * locally so the slider answers the drag rather than the network, and
 * surrendered back to the device a moment later because the device is the
 * authority on its own volume — someone turning the dial on the speaker
 * itself has to reach the screen.
 *
 * It was written five times: the Sonos bridge (twice, once for speakers and
 * once for groups), the KEF bridge, the zone bridge, and the panel store
 * (twice again, once for the room and once per member). No two agreed. The
 * window was 3s, 4s or 2.5s depending on which one you read, and the *place*
 * the rule applied differed too — Sonos filtered the poll as it landed, KEF
 * and zones resolved it when the slider was drawn, the panel synced it
 * through an effect. Filtering the poll has a tell: once the window lapses
 * the stale local value goes on showing until the next read comes in, which
 * on a 15-second poll is most of a minute of a slider that is quietly wrong.
 *
 * So the rule is one statement here, resolved where the slider is drawn.
 *
 * ## Why the window stopped mattering
 *
 * All three numbers were guesses at "how long until the device reports what
 * I just set", and all three were also doing a second job badly: holding the
 * value while the finger is still down. The bridges managed that by
 * re-stamping the clock on every drag frame, which holds a *moving* finger
 * and drops a still one — stop mid-drag for longer than the window and the
 * slider jumps back under your thumb. Only the panel got that right, with an
 * explicit flag.
 *
 * With `drag`/`commit` marking the gesture, the window's only remaining job
 * is the gap between letting go and the device agreeing, so one number
 * serves all five. It is deliberately short: it is a bridge across a round
 * trip, not a lease on the value.
 */

import { clampVol, createVolumeThrottle } from "./volume";

/** Long enough to cover a set and the read that confirms it; short enough
 *  that a change made on the device itself isn't ignored for long. */
const HOLD_MS = 3000;

/**
 * When to stop believing in a drag nobody ended.
 *
 * A gesture normally closes itself — every slider's `oninput` is followed by
 * an `onchange`. When one doesn't (a touch cancelled out from under the
 * element, a component swapped mid-drag) the flag would hold the local value
 * for as long as the store lives, which is worse than the snap-back it
 * exists to prevent. So the flag is a claim that expires: far beyond a
 * finger resting still on a slider, and far short of forever.
 */
const DRAG_ABANDON_MS = 30_000;

export interface Fader {
  /**
   * What the slider shows for `id`. The local value while it still holds —
   * the finger is down, or it only just lifted — and what the device
   * reported otherwise.
   *
   * `reported` is whatever the poll last said, which for a Sonos group or a
   * zone is a figure the caller computes from the members rather than one
   * the device gives out.
   */
  shown(id: string, reported: number | undefined): number;
  /** Whether `id`'s local value still wins over the poll. */
  holds(id: string): boolean;
  /**
   * One frame of a drag: show it at once, hold it, and throttle a call out
   * so the room answers the finger while it moves. Returns the clamped
   * level.
   */
  drag(id: string, level: number): number;
  /**
   * The finger lifted. Drops any queued mid-drag frame — a stale one landing
   * after this would undo it — ends the gesture and starts the hold. Returns
   * the clamped level for the caller to send authoritatively.
   */
  commit(id: string, level: number): number;
}

export function createFader(
  /** Send a mid-drag frame. Failures are ignored on purpose: a dropped frame
   *  self-heals on release or the next poll. */
  sendLive: (id: string, level: number) => void,
  opts: { holdMs?: number } = {},
): Fader {
  const holdMs = opts.holdMs ?? HOLD_MS;
  const local = $state<Record<string, number>>({});
  // Neither of these is `$state`, and that is deliberate: a component reads
  // the slider's value, which must change when the level does and not when
  // the finger lifts. Making the gesture reactive would re-run every reader
  // on a mouseup that changed nothing.
  const at: Record<string, number> = {};
  const dragging: Record<string, boolean> = {};

  const throttle = createVolumeThrottle(sendLive);

  function holds(id: string): boolean {
    if (local[id] === undefined) return false;
    const since = Date.now() - (at[id] ?? 0);
    if (dragging[id]) return since < DRAG_ABANDON_MS;
    return since < holdMs;
  }

  return {
    holds,

    shown(id, reported) {
      if (holds(id)) return local[id];
      return reported ?? local[id] ?? 0;
    },

    drag(id, level) {
      const v = clampVol(level);
      dragging[id] = true;
      local[id] = v;
      at[id] = Date.now();
      throttle.schedule(id, v);
      return v;
    },

    commit(id, level) {
      const v = clampVol(level);
      throttle.cancel(id);
      dragging[id] = false;
      local[id] = v;
      at[id] = Date.now();
      return v;
    },
  };
}
