/**
 * What this room played before, and what it keeps coming back to.
 *
 * HomeHub's own memory, per room, because Spotify's is one list for the
 * whole household and cannot say that the kitchen gets radio at breakfast —
 * which is exactly the question a wall is asked. It is what the band's
 * shelf falls back to for a room with no queue and no favorites (a KEF or a
 * zone), which until it existed got a third of the wall's height as air.
 *
 * Two shelves, not one. The plain history says what was on last, and by
 * eight in the evening the kitchen's breakfast radio and its dinner records
 * are equally recent; the ranked one says what the room keeps returning to,
 * and where the room has a habit at the hour it currently is, what it plays
 * *then*. A room with no history of its own answers with the household's
 * and says which it is — the wall must never imply a room played something
 * it didn't.
 */

import { api } from "../api";
import { session } from "../stores.svelte";
import { clock } from "../music/clock.svelte";
import type { PanelSource } from "./types";
import type { PanelRunner } from "./timers.svelte";
import type { PlayItemBody } from "../api";
import type { MediaPlay } from "../types";

export interface PanelHistoryStore {
  readonly history: MediaPlay[];
  /** True when the list is the household's rather than this room's own. */
  readonly historyHousehold: boolean;
  readonly topPlays: MediaPlay[];
  /** True when `topPlays` is this room's habit at `topPlaysHour` rather
   *  than its favourites overall. The shelf's label depends on it. */
  readonly topPlaysByHour: boolean;
  readonly topPlaysHour: number;
  playFromHistory(p: MediaPlay): void;
  forgetPlay(p: MediaPlay): void;
  /** Whether `forgetPlay` would reach anything. Absent rather than
   *  refused, per §15.1. */
  readonly canForget: boolean;
}

export interface PanelHistoryDeps {
  /** The featured room's destination key, or "" when there is none. */
  roomKey: () => string;
  featured: () => PanelSource | undefined;
  run: PanelRunner;
  /**
   * Replay a Sonos favorite by its URI, if the household still lists one.
   * A favorite that has since been deleted simply stops being offered,
   * which is better than a tile that fails — so this reports nothing and
   * the caller does not branch on it.
   */
  playFavoriteByURI: (uri: string) => void;
  /** Start a Spotify item on the featured source. */
  startOn: (s: PanelSource, body: PlayItemBody) => Promise<void>;
}

export function createPanelHistory(deps: PanelHistoryDeps): PanelHistoryStore {
  // ── What this room played before ─────────────────────────────────────
  // HomeHub's own memory, per room, because Spotify's is one list for the
  // whole household and cannot say that the kitchen gets radio at
  // breakfast. It is what the band's shelf falls back to for a room that
  // has no queue and no favorites — a KEF or a zone, which until now got
  // a third of the wall's height as air (§16).
  //
  // A room with no history of its own answers with the household's, and
  // says which it is: the wall must never imply a room played something
  // it didn't.
  let history = $state<MediaPlay[]>([]);
  let historyHousehold = $state(false);
  let historyFor = "";
  let historySeq = 0;

  // And what it keeps *coming back to*, which is a different question and
  // usually the better answer to "put something on": the plain list says
  // what was on last, and by eight in the evening the kitchen's breakfast
  // radio and its dinner records are equally recent. Asked for at the hour
  // it currently is, so a room with a habit at this hour gets that habit
  // and a room without one gets its favourites overall — the answer says
  // which, and the shelf's label repeats it rather than inventing one.
  let topPlays = $state<MediaPlay[]>([]);
  let topPlaysByHour = $state(false);
  // Plain dates throughout this block: each one is read for the hour it
  // is now and thrown away in the same statement, never held and never
  // mutated, so there is nothing for a reactive Date to be reactive about.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  let topPlaysHour = $state(new Date().getHours());

  async function loadHistory(key: string) {
    const mine = ++historySeq;
    // Both shelves in one pass, settled: they answer different
    // questions about the same room and either can come back empty
    // without costing the other its list.
    const [plain, top] = await Promise.allSettled([
      api.mediaHistory(key, 12),
      api.mediaTopPlays(key, { limit: 8, hour: "now" }),
    ]);
    if (mine !== historySeq) return;
    if (plain.status === "fulfilled") {
      history = plain.value.plays;
      historyHousehold = plain.value.household;
    } else {
      history = [];
      historyHousehold = false;
    }
    if (top.status === "fulfilled") {
      topPlays = top.value.plays;
      topPlaysByHour = top.value.by_hour;
      topPlaysHour = top.value.hour;
    } else {
      topPlays = [];
      topPlaysByHour = false;
    }
  }

  $effect(() => {
    const key = deps.roomKey();
    if (key === historyFor) return;
    historyFor = key;
    historySeq++;
    history = [];
    historyHousehold = false;
    topPlays = [];
    topPlaysByHour = false;
    if (key) void loadHistory(key);
  });

  // The hour is half of what the ranking above was asked for, so when the
  // wall clock rolls into a new one the shelf is answering yesterday
  // evening's question. Re-read on the minute beat the store already has,
  // and only when the hour has actually changed.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  let shelfHour = new Date().getHours();
  $effect(() => {
    void clock.beat;
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    const h = new Date().getHours();
    if (h === shelfHour) return;
    shelfHour = h;
    if (historyFor) void loadHistory(historyFor);
  });

  /** Start something out of the history again.
   *
   *  A Sonos favorite is replayed through the favorites path it came from,
   *  matched by URI against the household's current list — a favorite that
   *  has since been deleted simply stops being offered, which is better
   *  than a tile that fails. Everything else is a Spotify item and goes
   *  back the way it was started. */
  function playFromHistory(p: MediaPlay) {
    const s = deps.featured();
    if (!s) return;
    if (p.provider === "sonos") {
      deps.playFavoriteByURI(p.uri);
      return;
    }
    void deps.run(
      "hist:" + p.uri,
      () =>
        deps
          .startOn(s, {
            service: "Spotify",
            uri: p.uri,
            title: p.title,
            kind: p.kind,
            sub: p.sub,
            art_uri: p.art_uri,
          })
          .then(() => loadHistory(deps.roomKey())),
      "Couldn't play that again",
    );
  }

  /** Forget one thing this room played.
   *
   *  The counterweight to a ranked shelf. `topPlays` puts what a room keeps
   *  coming back to at the front of the wall, which is the right answer
   *  right up until the thing it keeps coming back to is a mistake — and a
   *  mistake is exactly what gets replayed, because it is the tile in the
   *  first slot. Until this existed the cures were to out-play it thirty
   *  times or to delete the speaker.
   *
   *  Never the household's list: the fallback shelf is other rooms' plays,
   *  and one room is not the place to edit them. `canForget` is what keeps
   *  the control off that shelf. */
  function forgetPlay(p: MediaPlay) {
    const key = deps.roomKey();
    if (!key || historyHousehold || !session.isAdmin) return;
    void deps.run(
      "forget:" + p.uri,
      () => api.mediaForgetPlay(key, p.uri).then(() => loadHistory(key)),
      "Couldn't forget that",
    );
  }

  const canForget = $derived(!!deps.featured() && !historyHousehold && session.isAdmin);

  return {
    get history() {
      return history;
    },
    get historyHousehold() {
      return historyHousehold;
    },
    get topPlays() {
      return topPlays;
    },
    get topPlaysByHour() {
      return topPlaysByHour;
    },
    get topPlaysHour() {
      return topPlaysHour;
    },
    get canForget() {
      return canForget;
    },
    playFromHistory,
    forgetPlay,
  };
}
