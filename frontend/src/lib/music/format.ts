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
