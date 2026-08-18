/**
 * What a room can actually do — the one place that decides it.
 *
 * DESIGN.md §15 is a rule about honesty rather than about layout: what a
 * source can't do is *absent*, never dead. That only holds if every surface
 * agrees on what a source can do, and the app and the panel are two
 * surfaces drawing transport controls over the same three bridges.
 *
 * They did not agree. Each wrote its own copy of these rules — the app in
 * `rooms.svelte.ts`, the panel in `panel-music/sources.ts` — and the copies
 * drifted: the panel gated a KEF speaker's skip on it being awake and on a
 * network input, while the app offered skip on every KEF unconditionally,
 * including the TV and analog inputs where there is nothing to step through
 * and the speaker simply refuses the call.
 *
 * So the rules live here and the two room models read them. The models
 * themselves stay different on purpose — a phone's room card and a wall's
 * source carry different fields because they draw different things — but
 * what those fields *mean* is one answer in one place.
 */

import type { KEFState, MediaRoute } from "../types";

/** Which transport controls reach something on a given room. */
export interface Capabilities {
  /** A skip would land on another track. */
  canSkip: boolean;
  /** There is a position to scrub and an endpoint that accepts one. */
  canSeek: boolean;
  /** There is a queue to add to. */
  canQueue: boolean;
  /** Shuffle, repeat and crossfade — a Sonos group's, because only Sonos
   *  stores them. */
  canPlayMode: boolean;
  /** A physical input selector. */
  canPickInput: boolean;
}

/**
 * KEF inputs a skip means anything on. There is nothing to step through on
 * the TV or the analog input, so the speaker refuses — and a control that
 * will be refused is worse than one that isn't drawn.
 */
const KEF_SKIPPABLE = new Set(["wifi", "bluetooth"]);

/**
 * A Sonos group: the only make here with a queue, a seek and play modes,
 * because it is the only one whose API has them.
 */
export function sonosCapabilities(): Capabilities {
  return {
    canSkip: true,
    // Per track, actually — a duration of zero is the real gate, and the
    // surface that has one applies it on top of this.
    canSeek: true,
    canQueue: true,
    canPlayMode: true,
    canPickInput: false,
  };
}

/**
 * A KEF speaker: an input selector, and skips only where the current input
 * has something to step through. Asleep it has no transport at all.
 */
export function kefCapabilities(state: KEFState | undefined): Capabilities {
  return {
    canSkip: !!state && state.powered_on && KEF_SKIPPABLE.has(state.source),
    canSeek: false, // KEF's API has no seek at all
    canQueue: false,
    canPlayMode: false,
    canPickInput: true,
  };
}

/**
 * A HomeHub zone: it skips unless HomeHub is the one doing the decoding.
 * On the stream and AirPlay routes the speakers are pulling a live stream
 * rather than playing a track they could step off — see rooms.svelte.ts.
 */
export function zoneCapabilities(route: MediaRoute | undefined): Capabilities {
  return {
    canSkip: route !== "stream" && route !== "airplay",
    canSeek: false, // a fan-out of a stream can't be scrubbed
    canQueue: false,
    canPlayMode: false,
    canPickInput: false,
  };
}
