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

/**
 * How stale a reading may honestly claim to be, in ms. The Sonos monitor
 * re-reads every speaker on a 30s ticker and refuses to serve anything older
 * than 95s, and the KEF poller is faster still — so an age past this is two
 * clocks disagreeing, not a genuinely old reading.
 */
const MAX_READ_AGE_MS = 95_000;

/**
 * Seconds elapsed since the reading a position is being extrapolated from
 * was actually taken.
 *
 * The naive answer — `now - whenWePolled` — assumes the number arrived fresh,
 * and for Sonos it usually hasn't: the hub answers from its event cache, and
 * events carry a track change but never a position (Sonos pushes no RelTime),
 * so the position is as old as the last authoritative read. Extrapolating
 * from the request instead of from the read ran that whole gap into every
 * scrubber, hairline and elapsed time in the app — a rail reading 45% on a
 * song that was 53% through.
 *
 * `readAt` is the hub's clock and `polledAt`/`now` are the browser's, so the
 * two are mixed exactly once, at the poll, and the result is clamped: a wall
 * panel with a drifting clock then gets a bounded error instead of a wild
 * one, and everything after the poll ticks on the browser's clock alone. An
 * absent `readAt` — an older hub, a speaker never read — collapses to the
 * old behaviour rather than guessing.
 */
export function sinceRead(readAt: number | undefined, polledAt: number, now: number): number {
  const ageAtPoll = readAt ? Math.min(Math.max(0, polledAt - readAt), MAX_READ_AGE_MS) : 0;
  return (ageAtPoll + Math.max(0, now - polledAt)) / 1000;
}

/** Seconds → `"0:03:12"`, the `H:MM:SS` form the Sonos seek endpoint takes. */
export function toClock(t: number): string {
  const total = Math.max(0, Math.round(t));
  const h = Math.floor(total / 3600);
  const m = String(Math.floor(total / 60) % 60).padStart(2, "0");
  const s = String(total % 60).padStart(2, "0");
  return `${h}:${m}:${s}`;
}
