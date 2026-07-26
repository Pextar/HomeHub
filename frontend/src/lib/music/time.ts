/**
 * Track-position arithmetic, shared by every Music surface that shows one.
 *
 * Two clock formats meet here. Sonos speaks `H:MM:SS` on the wire — always
 * with the leading hour, even for a three-minute song — and KEF speaks
 * milliseconds. Both end up as seconds in the view layer, and only the seek
 * endpoint needs the wire format back.
 *
 * Pure on purpose: this is the arithmetic behind every scrubber, progress
 * hairline and duration label in the module, so it is worth having under test
 * rather than inlined in a 5,000-line view.
 */

/** `"0:03:12"` → `192`. Anything unparseable, empty or absent → `0`. */
export function secs(t?: string): number {
  if (!t) return 0;
  const p = t.split(":").map(Number);
  return p.reduce((acc, n) => acc * 60 + (Number.isFinite(n) ? n : 0), 0);
}

/**
 * `"0:03:12"` → `"3:12"`. Sonos always sends the leading hour, and a queue
 * row showing `0:03:12` for a three-minute track reads as three hours at a
 * glance. An hour-long track keeps its hour.
 */
export function trimClock(t?: string): string {
  if (!t) return "";
  return t.replace(/^0:0?/, "");
}

/** Seconds → `"3:12"`, or `"1:04:00"` once there is an hour to show. */
export function fmtSecs(t: number): string {
  const total = Math.max(0, Math.round(t));
  const s = String(total % 60).padStart(2, "0");
  const m = Math.floor(total / 60);
  if (m < 60) return `${m}:${s}`;
  return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}:${s}`;
}

/** Seconds → `"0:03:12"`, the `H:MM:SS` form the Sonos seek endpoint takes. */
export function toClock(t: number): string {
  const total = Math.max(0, Math.round(t));
  const h = Math.floor(total / 3600);
  const m = String(Math.floor(total / 60) % 60).padStart(2, "0");
  const s = String(total % 60).padStart(2, "0");
  return `${h}:${m}:${s}`;
}
