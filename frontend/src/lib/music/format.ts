/**
 * Number and duration formatting for the catalog surfaces — the search
 * results, the artist page, the album and playlist headers.
 *
 * Pure on purpose, same discipline as time.ts: the arithmetic behind every
 * "1.2M followers" and "43 min" in the module, in one tested place rather
 * than inlined in the views.
 */

import { fmtSecs } from "./time";

/**
 * A big count the way Spotify writes it: 12,345,678 → "12.3M",
 * 12,345 → "12.3K", 987 → "987". One decimal, and only while it's
 * informative — "12.0M" reads as false precision, so it becomes "12M".
 */
export function fmtCount(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0";
  if (n < 1000) return String(Math.floor(n));
  const units: [number, string][] = [
    [1_000_000_000, "B"],
    [1_000_000, "M"],
    [1_000, "K"],
  ];
  for (let i = 0; i < units.length; i++) {
    const [div, suffix] = units[i];
    if (n < div) continue;
    const v = n / div;
    const rounded = v >= 100 ? Math.round(v) : Math.round(v * 10) / 10;
    if (rounded >= 1000) {
      // The 999.9K boundary reads better rolled over to the bigger unit.
      const bigger = units[i - 1];
      return bigger ? `1${bigger[1]}` : `1000${suffix}`;
    }
    return `${rounded % 1 === 0 ? rounded.toFixed(0) : rounded.toFixed(1)}${suffix}`;
  }
  return String(n);
}

/** 12,345,678 → "12,345,678 followers" — the exact count on an artist page. */
export function fmtFollowers(n: number): string {
  return `${n.toLocaleString("en-US")} follower${n === 1 ? "" : "s"}`;
}

/** Milliseconds → "3:12", reusing the module's one clock formatter. */
export function fmtMs(ms: number): string {
  return fmtSecs(ms / 1000);
}

/**
 * A track list's total length: 2,580,000 → "43 min", past the hour
 * "1 hr 12 min". Zero renders as nothing — a made-up total is worse
 * than none.
 */
export function fmtTotalMs(ms: number): string {
  const mins = Math.round(ms / 60000);
  if (mins <= 0) return "";
  if (mins < 60) return `${mins} min`;
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return m === 0 ? `${h} hr` : `${h} hr ${m} min`;
}

/** "art rock" → "Art rock" — genre labels arrive lower-cased. */
export function capFirst(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}

/**
 * How many times a room has started something, reading a missing tally as
 * the one play that must have created the entry — the same rule the store
 * applies on its side, said once here so no surface invents a second one.
 */
export function playCount(p: { count?: number }): number {
  return p.count && p.count > 0 ? p.count : 1;
}

/**
 * A local hour as a wall clock says it: `8` → "08:00". Mono digits, because
 * every number on the wall is (§2), and no am/pm — HomeHub's clock is 24h
 * everywhere else on the panel.
 */
export function fmtHour(hour: number): string {
  const h = Number.isFinite(hour) ? Math.max(0, Math.min(23, Math.floor(hour))) : 0;
  return `${String(h).padStart(2, "0")}:00`;
}

/**
 * How long until a moment, in the words a row of things-about-to-happen
 * wants: "now", "in 6 min", "in 3 h 20", "in 9 h", "Mon 06:45".
 *
 * Relative for anything inside a day, because the rows this labels already
 * say what o'clock they are — "06:45 · Weekdays · tomorrow 06:45" repeats
 * itself, and the part nobody can work out at a glance is how far away that
 * is. Past a day the weekday becomes the useful half and the clock rejoins
 * it, since "in 31 h" is a number nobody converts.
 *
 * Past its moment it says "now" rather than a negative: the only reason a
 * scheduled time is behind us on screen is that the tick hasn't landed yet,
 * and "3 minutes ago" about something that is on its way is worse than
 * rounding to the truth.
 */
export function fmtUntil(iso?: string, from: Date = new Date()): string {
  if (!iso) return "";
  const at = new Date(iso);
  const ms = at.getTime() - from.getTime();
  if (Number.isNaN(ms)) return "";
  if (ms <= 30_000) return "now";
  const mins = Math.round(ms / 60_000);
  if (mins < 60) return `in ${mins} min`;
  if (mins < 24 * 60) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    return m === 0 ? `in ${h} h` : `in ${h} h ${m}`;
  }
  const hhmm = at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return `${at.toLocaleDateString([], { weekday: "short" })} ${hhmm}`;
}

/**
 * The days a recurring timer runs, as a row reads them: "Every day",
 * "Weekdays", "Weekends", else the short names in week order. An empty list
 * is every day — the store's own normalisation, repeated here rather than
 * rendered as nothing.
 */
export function fmtDays(days?: number[]): string {
  const set = [...new Set(days ?? [])].sort((a, b) => a - b);
  if (set.length === 0 || set.length === 7) return "Every day";
  if (set.length === 5 && set.every((d) => d >= 1 && d <= 5)) return "Weekdays";
  if (set.length === 2 && set.includes(0) && set.includes(6)) return "Weekends";
  const names = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  return set
    .map((d) => names[d] ?? "")
    .filter(Boolean)
    .join(" ");
}

/** The fields any bridge's now-playing metadata may carry. */
export interface TrackLike {
  title?: string;
  artist?: string;
  album?: string;
  /** Radio: what the station says is on air. */
  stream?: string;
  /** Radio: what the room is tuned to. */
  station?: string;
}

/** The two lines every surface names a room's audio with. */
export interface TrackLines {
  title: string;
  /** "" when there is nothing to say that the title doesn't already. */
  sub: string;
}

/**
 * What is playing, as a headline and a subline.
 *
 * A queued track answers this trivially — title over "artist · album" — and
 * that is all this used to do, in five places that each did it themselves.
 * A **radio stream doesn't fit that shape**: the song on air arrives in its
 * own field (`stream`), and `title` holds the stream rather than the music,
 * so a station played through the old rule announced itself as "P2" for
 * three hours while the actual song went unnamed.
 *
 * So the rule is: the headline is the most specific thing the source knows —
 * the song on air, else the track, else the station — and the subline is
 * where it is coming from, or the record it is off. A line is never repeated
 * across the two, because a card saying "P2" twice says it once.
 */
export function trackLines(t: TrackLike | null | undefined): TrackLines {
  const meta = [t?.artist, t?.album].filter(Boolean).join(" · ");
  const title = t?.stream || t?.title || t?.station || "";
  // On air: the station is the source, and its own name for the stream is
  // the fallback when the station didn't come through.
  // On air: where it is coming from, in descending order of how specific
  // the name is — the station, else the stream's own name for itself. Off a
  // record: the record. Whichever it is, the first candidate that isn't
  // already the headline wins, so no line is ever said twice.
  const candidates = t?.stream ? [t.station, t.title, meta] : [meta, t?.station];
  return { title, sub: candidates.find((c) => !!c && c !== title) ?? "" };
}
