/**
 * The featured Sonos group's queue, and everything that changes it.
 *
 * A Sonos group is the only thing in the house that *has* a queue — a KEF
 * speaker and a HomeHub zone play what they were handed — so every call here
 * checks the featured room is Sonos and does nothing otherwise, rather than
 * offering a control that would be refused (DESIGN.md §15.1).
 *
 * Two rules run through all of it.
 *
 * **A mutation renumbers the rest, so it re-reads.** Removing track 3 makes
 * the old 4 into the new 3; splicing the local copy instead would leave every
 * row below the edit pointing at the wrong song, and the next tap would play
 * the wrong one. Every mutation therefore ends in `loadQueue`.
 *
 * **Queue order is play order only while the group plays straight through
 * it.** Under shuffle the speaker picks its own next track, and under
 * repeat-one it plays this one again, so "up next" would be a guess — and the
 * wall doesn't guess. `nextInQueue` is undefined in both cases, and the
 * surfaces that draw it say nothing rather than something wrong.
 *
 * The skeleton is only for the featured room's *first* read; the re-reads a
 * poll triggers after that are silent, or the pane would flash every few
 * seconds while nothing about it had changed.
 */

import { api } from "../api";
import type { SonosQueueItem, SpotifyItem } from "../types";
import type { PanelSource, PanelRunner } from "./types";

export interface PanelQueueDeps {
  /** The room the wall is pointed at. A getter: it moves under this. */
  featured: () => PanelSource | undefined;
  run: PanelRunner;
}

export interface PanelQueueStore {
  readonly queue: SonosQueueItem[];
  readonly queueLoading: boolean;
  /** The track that will actually play next, or undefined when the speaker
   *  is the one choosing. */
  readonly nextInQueue: SonosQueueItem | undefined;
  /** Queue order is play order — false under shuffle or repeat-one, where a
   *  surface must say nothing about what comes next rather than guess. */
  readonly queueOrderKnown: boolean;
  /** What was last added, for the few seconds a surface says so. */
  readonly lastQueued: { title: string; next: boolean; at: number } | null;

  jumpTo(track: number): void;
  removeQueued(track: number): void;
  clearQueue(): void;
  /** Move one queued track one place up or down. */
  moveQueued(track: number, dir: -1 | 1): void;
  /** Add an item without interrupting: `next` drops it after the current
   *  track rather than at the end. */
  enqueue(item: SpotifyItem, next: boolean): void;
  /** Re-read a coordinator's queue. Called after anything that changes it. */
  load(coordinatorId: string): Promise<void>;
}

export function createPanelQueue(deps: PanelQueueDeps): PanelQueueStore {
  let queue = $state<SonosQueueItem[]>([]);
  let queueLoading = $state(false);
  let queueSeq = 0;
  /** Whose queue is loaded/loading, and whose answered last. */
  let queueFor = "";
  let loadedFor = "";

  async function load(coordinatorId: string) {
    const mine = ++queueSeq;
    if (loadedFor !== coordinatorId) queueLoading = true;
    try {
      const q = await api.sonosQueue(coordinatorId);
      if (mine !== queueSeq) return;
      queue = q;
      loadedFor = coordinatorId;
    } catch {
      if (mine !== queueSeq) return;
      queue = [];
      loadedFor = coordinatorId;
    } finally {
      if (mine === queueSeq) queueLoading = false;
    }
  }

  // The queue belongs to whatever Sonos group is featured — and to nothing
  // else. Reading `featured` here re-runs this on each poll, which is exactly
  // the cadence the queue wants: it only changes on a mutation, and those
  // re-read it themselves anyway.
  $effect(() => {
    const f = deps.featured();
    const id = f?.kind === "sonos" ? f.id : "";
    if (id !== queueFor) {
      queueFor = id;
      queueSeq++; // cancel any load for the previous room
      queue = [];
    }
    if (id) void load(id);
  });

  const queueOrderKnown = $derived.by(() => {
    const gs = deps.featured()?.groupState;
    if (!gs) return false;
    return !gs.shuffle && gs.repeat !== "one";
  });
  const nextInQueue = $derived(
    queueOrderKnown
      ? queue.find((q) => q.track > (deps.featured()?.queueTrack ?? 0))
      : undefined,
  );

  let lastQueued = $state<{ title: string; next: boolean; at: number } | null>(null);

  /** The featured room, but only when it is one that has a queue at all. */
  function sonosFeatured(): PanelSource | undefined {
    const f = deps.featured();
    return f && f.kind === "sonos" ? f : undefined;
  }

  return {
    get queue() {
      return queue;
    },
    get queueLoading() {
      return queueLoading;
    },
    get nextInQueue() {
      return nextInQueue;
    },
    get queueOrderKnown() {
      return queueOrderKnown;
    },
    get lastQueued() {
      return lastQueued;
    },
    load,

    jumpTo(track) {
      const f = sonosFeatured();
      if (!f) return;
      void deps.run(
        "jump:" + track,
        () => api.sonosSeekTrack(f.id, track),
        "Couldn't play that track",
      );
    },

    removeQueued(track) {
      const f = sonosFeatured();
      if (!f) return;
      void deps.run(
        "qrm:" + track,
        () => api.sonosQueueRemove(f.id, track).then(() => load(f.id)),
        "Couldn't remove that track",
      );
    },

    clearQueue() {
      const f = sonosFeatured();
      if (!f) return;
      void deps.run(
        "qclear:" + f.id,
        () => api.sonosQueueClear(f.id).then(() => load(f.id)),
        "Couldn't clear the queue",
      );
    },

    /**
     * One place at a time, by tap, because this is a wall: the app's drag
     * would be an imprecise aim at arm's length over a five-second poll
     * (§16's argument for tap-based grouping applies unchanged here).
     */
    moveQueued(track, dir) {
      const f = sonosFeatured();
      if (!f) return;
      const to = track + dir;
      if (to < 1 || to > queue.length) return;
      void deps.run(
        "qmv:" + track,
        () => api.sonosQueueMove(f.id, track, to).then(() => load(f.id)),
        "Couldn't move that track",
      );
    },

    /**
     * Nothing on screen moves when this lands — adding to a group playing
     * radio is legal but silent — so what went in is noted here and the
     * player column says so for a few seconds. A success toast would be the
     * wrong instrument: the app answers quietly, and a kiosk has nobody to
     * dismiss cards.
     */
    enqueue(item, next) {
      const f = sonosFeatured();
      if (!f) return;
      void deps.run(
        "q:" + item.uri,
        async () => {
          await api.sonosQueueAdd(f.id, {
            service: "Spotify",
            uri: item.uri,
            title: item.name,
            next,
          });
          await load(f.id);
          lastQueued = { title: item.name, next, at: Date.now() };
        },
        "Couldn't add to the queue",
      );
    },
  };
}
