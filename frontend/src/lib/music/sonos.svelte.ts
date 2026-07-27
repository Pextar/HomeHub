import { api } from "../api";
import { toasts } from "../stores.svelte";
import type {
  SonosStatus,
  SonosSpeakerView,
  SonosGroupView,
  SonosFavorite,
  SonosQueueItem,
  SonosRepeat,
} from "../types";
import type { Busy } from "./busy.svelte";
import { clock } from "./clock.svelte";
import { secs, toClock } from "./time";
import { clampVol } from "./volume";

/**
 * The Sonos bridge, as state: the topology poll, the optimistic overrides
 * that keep controls honest between polls, and every action the local bridge
 * can back.
 *
 * Effects live in the component that owns this — the factory exposes
 * `refresh` and the view decides the interval, which is the point: it runs
 * four times slower when the speakers are pushing their own changes
 * (DESIGN.md §15) and that decision belongs to the surface, not here.
 */

/** Sonos stores shuffle and repeat as one composite value; this is the cycle. */
export const NEXT_REPEAT: Record<SonosRepeat, SonosRepeat> = {
  off: "all",
  all: "one",
  one: "off",
};

/** Repeat reports its *next* state, since that is what the tap will do. */
export function repeatLabel(r?: SonosRepeat): string {
  if (r === "all") return "Repeat all — tap for repeat one";
  if (r === "one") return "Repeat one — tap to turn repeat off";
  return "Repeat off — tap to repeat all";
}

export interface SonosBridge {
  readonly status: SonosStatus | null;
  /** False until the first poll answers, however it answered. */
  readonly loaded: boolean;
  /** Whether the speakers are pushing changes, or we are polling them. */
  readonly livePush: boolean;
  /** Wall-clock of the last successful poll — what positions extrapolate from. */
  readonly polledAt: number;

  readonly speakerById: Map<string, SonosSpeakerView>;
  readonly groups: SonosGroupView[];
  /** Registered speakers the live topology doesn't mention — offline or elsewhere. */
  readonly offline: SonosSpeakerView[];
  readonly reachable: SonosSpeakerView[];
  readonly playingGroups: SonosGroupView[];
  /** Multi-speaker zones — the ones that get a dashed enclosure in the grid. */
  readonly multiGroups: SonosGroupView[];
  /** Everything reachable that isn't in one, shown as a loose puck. */
  readonly soloSpeakers: SonosSpeakerView[];
  /** Every registered speaker, reachable first then by name — the Speakers list. */
  readonly allSpeakers: SonosSpeakerView[];
  readonly favorites: SonosFavorite[];

  readonly queue: SonosQueueItem[];
  readonly queueLoading: boolean;

  refresh(): Promise<void>;

  coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined;
  /** Shuffle / repeat / crossfade / queue length: the backend reports them on
   *  the coordinator only, since they belong to the group. */
  groupStateOf(g: SonosGroupView): SonosSpeakerView["group_state"];
  groupTitle(g: SonosGroupView): string;
  groupOfSpeaker(id: string): SonosGroupView | undefined;
  groupById(coordinatorId: string | null): SonosGroupView | undefined;
  /** The one place "is this playing?" is answered, so a tapped play/pause
   *  flips every waveform, card and icon at once. */
  isPlaying(g: SonosGroupView | undefined): boolean;
  speakerPlaying(id: string): boolean;
  speakerNowLine(id: string): string;
  /** Speakers outside this group that could join it. */
  joinables(g: SonosGroupView): SonosSpeakerView[];

  /** Extrapolated track position for any group, in seconds. */
  positionOf(g: SonosGroupView): number;
  /** 0–1, or 0 when the source reports no duration (radio, line-in, TV). */
  progressOf(g: SonosGroupView): number;
  /** The player's position: `positionOf` plus a just-issued seek. */
  livePosition(g: SonosGroupView | undefined): number;

  shownVolume(sp: SonosSpeakerView): number;
  shownGroupVolume(coordinatorId: string): number;
  dragVolume(id: string, v: number): void;
  dragGroupVolume(coordinatorId: string, v: number): void;
  setVolume(id: string, v: number): void;
  setGroupVolume(coordinatorId: string, v: number): void;
  nudgeVolume(g: SonosGroupView, delta: number): void;
  toggleMute(sp: SonosSpeakerView): void;
  /** Mutes the whole zone: muting only the coordinator of a three-room group
   *  would look like the key did nothing. */
  toggleMuteGroup(g: SonosGroupView): void;

  togglePlay(g: SonosGroupView): Promise<void>;
  skip(g: SonosGroupView, dir: "next" | "previous"): void;
  seek(g: SonosGroupView, sec: number): void;
  /** Drop the seek override — the player calls this when the track changes. */
  clearSeek(): void;
  setPlayMode(g: SonosGroupView, patch: { shuffle?: boolean; repeat?: SonosRepeat }): void;
  toggleCrossfade(g: SonosGroupView): void;
  /** "Continue play similar" — keep going with similar tracks once the
   *  queue runs out. A preference, not a device state, the same shape as
   *  crossfade. */
  toggleAutoplay(g: SonosGroupView): void;

  join(speakerId: string, g: SonosGroupView): void;
  leave(speakerId: string): void;
  ungroup(g: SonosGroupView): Promise<void>;
  /** Drop one room onto another. Only the dragged speaker moves. */
  groupOnto(sourceId: string, targetId: string): Promise<{ source: string; target: string } | null>;

  loadQueue(coordinatorId: string, skeleton?: boolean): Promise<void>;
  dropQueue(): void;
  /** The first queued track after the one playing. */
  nextInQueue(current: number | undefined): SonosQueueItem | undefined;
  jumpTo(g: SonosGroupView, track: number): void;
  removeQueued(g: SonosGroupView, track: number): Promise<void>;
  clearQueue(coordinatorId: string): Promise<void>;
  enqueue(
    target: string,
    item: { uri: string; title?: string; service?: string; metadata?: string },
    next: boolean,
  ): Promise<{ track: number; length: number } | undefined>;
}

export function createSonosBridge(busy: Busy): SonosBridge {
  const s = $state({
    status: null as SonosStatus | null,
    loaded: false,
    favorites: [] as SonosFavorite[],
    // Wall-clock of the last successful poll. The player advances the track
    // position from here so the scrubber moves every second instead of
    // jumping every five.
    polledAt: 0,
    queue: [] as SonosQueueItem[],
    queueLoading: false,
  });

  let favsLoaded = false;
  let statusSeq = 0;
  let queueSeq = 0;

  // Volume the user just set, keyed by speaker id. The poll must not yank the
  // slider back to a stale value while the command is still propagating, so
  // recent local sets win over polled state briefly.
  const volOverride: Record<string, { v: number; at: number }> = {};
  const localVol = $state<Record<string, number>>({});
  const groupVol = $state<Record<string, number>>({});

  // A play/pause round-trip plus the refresh behind it takes long enough that
  // an un-flipped button reads as a dropped tap. The new state is applied
  // locally and wins until the poll reports it. Rolled back if the call fails.
  const playOverride = $state<Record<string, { playing: boolean; at: number }>>({});

  // A just-issued seek wins over the polled position until the speaker has had
  // time to report it — same idea as volOverride.
  let seekOverride = $state<{ sec: number; at: number } | null>(null);

  // A plain Map, deliberately: it is rebuilt by the derivation whenever the
  // poll lands and nothing ever mutates it in place, so the reactivity is the
  // $derived's and a SvelteMap would only add bookkeeping.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const speakerById = $derived(new Map((s.status?.speakers ?? []).map((x) => [x.id, x])));
  const groups = $derived(s.status?.groups ?? []);
  const offline = $derived(
    (s.status?.speakers ?? []).filter((x) => !groups.some((g) => g.member_ids.includes(x.id))),
  );
  const reachable = $derived((s.status?.speakers ?? []).filter((x) => x.reachable));

  function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
    return speakerById.get(g.coordinator_id) ?? speakerById.get(g.member_ids[0]);
  }

  function isPlaying(g: SonosGroupView | undefined): boolean {
    if (!g) return false;
    const ov = playOverride[g.coordinator_id];
    return ov ? ov.playing : !!coordinatorOf(g)?.state?.playing;
  }

  function groupOfSpeaker(id: string): SonosGroupView | undefined {
    return groups.find((g) => g.member_ids.includes(id));
  }

  function groupTitle(g: SonosGroupView): string {
    const names = g.member_ids
      .map((id) => speakerById.get(id)?.name)
      .filter((n): n is string => !!n);
    if (names.length <= 2) return names.join(" + ");
    return `${names[0]} + ${names.length - 1} more`;
  }

  const playingGroups = $derived(groups.filter((g) => isPlaying(g)));
  const multiGroups = $derived(groups.filter((g) => g.member_ids.length > 1));
  const soloSpeakers = $derived(
    reachable.filter((x) => !multiGroups.some((g) => g.member_ids.includes(x.id))),
  );
  const allSpeakers = $derived.by(() => {
    const list = [...(s.status?.speakers ?? [])];
    list.sort((a, b) => {
      if (a.reachable !== b.reachable) return a.reachable ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    return list;
  });

  function positionOf(g: SonosGroupView): number {
    void clock.beat; // re-derive once a second
    const st = coordinatorOf(g)?.state;
    const base = secs(st?.position);
    if (!isPlaying(g) || !s.polledAt) return base;
    const total = secs(st?.duration);
    const advanced = base + (Date.now() - s.polledAt) / 1000;
    return total ? Math.min(total, advanced) : advanced;
  }

  const run = (key: string, fn: () => Promise<unknown>, errTitle: string) =>
    busy.run(key, fn, errTitle, refresh);

  async function refresh() {
    const mine = ++statusSeq;
    try {
      const st = await api.sonosStatus();
      if (mine !== statusSeq) return;
      s.status = st;
      s.polledAt = Date.now();
      const now = s.polledAt;
      // Retire an optimistic play/pause as soon as the speaker agrees with it
      // — or after 6s, so a command the speaker quietly ignored can't leave a
      // button lying about its state forever.
      for (const [id, ov] of Object.entries(playOverride)) {
        const sp = st.speakers.find((x) => x.id === id);
        if (!sp || sp.state?.playing === ov.playing || now - ov.at > 6000) {
          delete playOverride[id];
        }
      }
      for (const sp of st.speakers) {
        const ov = volOverride[sp.id];
        if (ov && now - ov.at < 3000) continue; // user just moved it
        if (sp.state) localVol[sp.id] = sp.state.volume;
      }
      for (const g of st.groups) {
        // Group volume isn't reported by the status poll; seed the slider with
        // the members' average unless recently set.
        const ov = volOverride["g:" + g.coordinator_id];
        if (ov && now - ov.at < 3000) continue;
        const vols = g.member_ids
          .map((id) => st.speakers.find((x) => x.id === id)?.state?.volume)
          .filter((v): v is number => v !== undefined);
        if (vols.length) {
          groupVol[g.coordinator_id] = Math.round(vols.reduce((a, b) => a + b, 0) / vols.length);
        }
      }
      if (!favsLoaded && st.speakers.some((x) => x.reachable)) {
        void loadFavorites(st.speakers.find((x) => x.reachable)!.id);
      }
    } catch (e) {
      if (mine !== statusSeq) return;
      if (!s.loaded) toasts.error("Couldn't reach Sonos", (e as Error).message);
    } finally {
      if (mine === statusSeq) s.loaded = true;
    }
  }

  async function loadFavorites(speakerId: string) {
    favsLoaded = true;
    try {
      s.favorites = await api.sonosFavorites(speakerId);
    } catch {
      favsLoaded = false; // retry on a later poll
    }
  }

  // Sliders update the local value live (oninput) and send on release
  // (onchange), so dragging doesn't flood the speaker with SOAP calls.
  function setVolume(id: string, v: number) {
    localVol[id] = v;
    volOverride[id] = { v, at: Date.now() };
    api.sonosSetVolume(id, v).catch((e) => toasts.error("Volume failed", (e as Error).message));
  }

  function setGroupVolume(coordinatorId: string, v: number) {
    groupVol[coordinatorId] = v;
    volOverride["g:" + coordinatorId] = { v, at: Date.now() };
    api
      .sonosSetVolume(coordinatorId, v, true)
      .catch((e) => toasts.error("Volume failed", (e as Error).message));
  }

  async function loadQueue(coordinatorId: string, skeleton = false) {
    const mine = ++queueSeq;
    if (skeleton) s.queueLoading = true;
    try {
      const q = await api.sonosQueue(coordinatorId);
      if (mine !== queueSeq) return;
      s.queue = q;
    } catch {
      if (mine === queueSeq) s.queue = []; // an unreachable coordinator shows empty
    } finally {
      if (mine === queueSeq) s.queueLoading = false;
    }
  }

  return {
    get status() {
      return s.status;
    },
    get loaded() {
      return s.loaded;
    },
    get livePush() {
      return !!s.status?.live;
    },
    get polledAt() {
      return s.polledAt;
    },
    get speakerById() {
      return speakerById;
    },
    get groups() {
      return groups;
    },
    get offline() {
      return offline;
    },
    get reachable() {
      return reachable;
    },
    get playingGroups() {
      return playingGroups;
    },
    get multiGroups() {
      return multiGroups;
    },
    get soloSpeakers() {
      return soloSpeakers;
    },
    get allSpeakers() {
      return allSpeakers;
    },
    get favorites() {
      return s.favorites;
    },
    get queue() {
      return s.queue;
    },
    get queueLoading() {
      return s.queueLoading;
    },

    refresh,
    coordinatorOf,
    groupStateOf: (g) => coordinatorOf(g)?.group_state,
    groupTitle,
    groupOfSpeaker,
    groupById: (id) => (id === null ? undefined : groups.find((g) => g.coordinator_id === id)),
    isPlaying,
    speakerPlaying: (id) => isPlaying(groupOfSpeaker(id)),

    speakerNowLine(id) {
      const g = groupOfSpeaker(id);
      const st = g && coordinatorOf(g)?.state;
      if (!st?.track?.title) return "Idle";
      return isPlaying(g) ? st.track.title : `Paused · ${st.track.title}`;
    },

    joinables: (g) => reachable.filter((x) => !g.member_ids.includes(x.id)),

    positionOf,

    progressOf(g) {
      const total = secs(coordinatorOf(g)?.state?.duration);
      return total > 0 ? Math.min(1, positionOf(g) / total) : 0;
    },

    /**
     * Deliberately not the same as `positionOf`: a just-issued seek wins here
     * for four seconds, so the scrubber the finger let go of stays where it
     * was put. The progress hairlines keep reading `positionOf`, which is the
     * speaker's own answer — they are a report, not a control.
     */
    livePosition(g) {
      void clock.beat; // re-derive once a second
      if (!g) return 0;
      const st = coordinatorOf(g)?.state;
      const total = secs(st?.duration);
      const now = Date.now();
      const ov = seekOverride;
      const held = ov && now - ov.at < 4000;
      const base = held ? ov.sec : secs(st?.position);
      const since = held ? ov.at : s.polledAt;
      if (!isPlaying(g) || !since) return base;
      const advanced = base + (now - since) / 1000;
      return total ? Math.min(total, advanced) : advanced;
    },

    shownVolume: (sp) => localVol[sp.id] ?? sp.state?.volume ?? 0,
    shownGroupVolume: (coordinatorId) => groupVol[coordinatorId] ?? 0,
    dragVolume(id, v) {
      localVol[id] = v;
    },
    dragGroupVolume(coordinatorId, v) {
      groupVol[coordinatorId] = v;
    },

    setVolume,
    setGroupVolume,

    // Volume steps, used by the player's arrow-key shortcuts. Grouped zones
    // move together (the same "All rooms" fader the sheet shows); a lone
    // speaker moves on its own.
    nudgeVolume(g, delta) {
      if (g.member_ids.length > 1) {
        const cur = groupVol[g.coordinator_id] ?? coordinatorOf(g)?.state?.volume ?? 0;
        setGroupVolume(g.coordinator_id, clampVol(cur + delta));
        return;
      }
      const sp = coordinatorOf(g);
      if (!sp) return;
      const cur = localVol[sp.id] ?? sp.state?.volume ?? 0;
      setVolume(sp.id, clampVol(cur + delta));
    },

    toggleMute(sp) {
      void run("mute:" + sp.id, () => api.sonosSetMute(sp.id, !sp.state?.muted), "Mute failed");
    },

    toggleMuteGroup(g) {
      const members = g.member_ids
        .map((id) => speakerById.get(id))
        .filter((x): x is SonosSpeakerView => !!x);
      if (!members.length) return;
      const next = !members.some((x) => x.state?.muted);
      void run(
        "mute:" + g.coordinator_id,
        () => Promise.all(members.map((x) => api.sonosSetMute(x.id, next))),
        "Mute failed",
      );
    },

    async togglePlay(g) {
      const c = coordinatorOf(g);
      if (!c) return;
      const next = !isPlaying(g);
      await busy.claim("play:" + c.id, async () => {
        playOverride[c.id] = { playing: next, at: Date.now() };
        try {
          await (next ? api.sonosPlay(c.id) : api.sonosPause(c.id));
          await refresh();
        } catch (e) {
          delete playOverride[c.id]; // the speaker never took it — roll back
          toasts.error(next ? "Play failed" : "Pause failed", (e as Error).message);
        }
      });
    },

    skip(g, dir) {
      const c = coordinatorOf(g);
      if (!c) return;
      void run(
        dir + ":" + c.id,
        () => (dir === "next" ? api.sonosNext(c.id) : api.sonosPrevious(c.id)),
        "Skip failed",
      );
    },

    seek(g, sec) {
      const c = coordinatorOf(g);
      if (!c) return;
      seekOverride = { sec, at: Date.now() };
      api.sonosSeek(c.id, toClock(sec)).catch((e) => {
        seekOverride = null;
        toasts.error("Seek failed", (e as Error).message);
      });
    },

    clearSeek() {
      seekOverride = null;
    },

    setPlayMode(g, patch) {
      const c = coordinatorOf(g);
      const gs = coordinatorOf(g)?.group_state;
      if (!c || !gs) return;
      void run(
        "mode:" + c.id,
        () => api.sonosSetPlayMode(c.id, patch.shuffle ?? gs.shuffle, patch.repeat ?? gs.repeat),
        "Couldn't change play mode",
      );
    },

    toggleCrossfade(g) {
      const c = coordinatorOf(g);
      const gs = coordinatorOf(g)?.group_state;
      if (!c || !gs) return;
      void run(
        "xfade:" + c.id,
        () => api.sonosSetCrossfade(c.id, !gs.crossfade),
        "Couldn't change crossfade",
      );
    },

    toggleAutoplay(g) {
      const c = coordinatorOf(g);
      if (!c) return;
      void run(
        "autoplay:" + c.id,
        () => api.sonosSetAutoplay(c.id, !c.autoplay),
        "Couldn't change autoplay",
      );
    },

    join(speakerId, g) {
      void run(
        "join:" + speakerId,
        () => api.sonosJoin(speakerId, g.coordinator_id),
        "Grouping failed",
      );
    },

    leave(speakerId) {
      void run("leave:" + speakerId, () => api.sonosLeave(speakerId), "Ungrouping failed");
    },

    async ungroup(g) {
      await busy.claim("ungroup:" + g.coordinator_id, async () => {
        try {
          for (const id of g.member_ids) {
            if (id !== g.coordinator_id) await api.sonosLeave(id);
          }
          await refresh();
        } catch (e) {
          toasts.error("Ungrouping failed", (e as Error).message);
        }
      });
    },

    /**
     * Only the dragged speaker moves. If it was leading a zone, Sonos
     * re-elects behind us — "this room now plays with that one" is the whole
     * promise, and carrying its former partners along would be a second
     * change nobody asked for.
     *
     * Answers with the two room names when it worked, so the caller can say
     * so out loud: a drag has no running commentary of its own.
     */
    async groupOnto(sourceId, targetId) {
      const target = groupOfSpeaker(targetId)?.coordinator_id ?? targetId;
      if (sourceId === target) return null;
      if (groupOfSpeaker(sourceId)?.coordinator_id === target) return null; // already together
      let done: { source: string; target: string } | null = null;
      await busy.claim("group:" + target, async () => {
        try {
          await api.sonosJoin(sourceId, target);
          await refresh();
          done = {
            source: speakerById.get(sourceId)?.name ?? "Room",
            target: speakerById.get(targetId)?.name ?? "the other room",
          };
        } catch (e) {
          toasts.error("Grouping failed", (e as Error).message);
        }
      });
      return done;
    },

    loadQueue,

    dropQueue() {
      queueSeq++; // cancel any in-flight load
      s.queue = [];
    },

    nextInQueue: (current) => s.queue.find((q) => q.track > (current ?? 0)),

    jumpTo(g, track) {
      const c = coordinatorOf(g);
      if (!c) return;
      void run("jump:" + track, () => api.sonosSeekTrack(c.id, track), "Couldn't play that track");
    },

    async removeQueued(g, track) {
      const c = coordinatorOf(g);
      if (!c) return;
      await run("qrm:" + track, () => api.sonosQueueRemove(c.id, track), "Couldn't remove that track");
      // Removing renumbers everything below it, so re-read rather than
      // splicing locally.
      void loadQueue(c.id);
    },

    async clearQueue(coordinatorId) {
      await run("qclear:" + coordinatorId, () => api.sonosQueueClear(coordinatorId), "Couldn't clear the queue");
      void loadQueue(coordinatorId);
    },

    /**
     * Enqueue without disturbing what's playing; `next` drops it in after the
     * current track. The queue is a Sonos group's, so this is only ever
     * offered for a Sonos destination. Answers with where it landed, since
     * queueing onto a group playing radio is legal but silent.
     */
    async enqueue(target, item, next) {
      return await busy.claim("q:" + item.uri, async () => {
        try {
          return await api.sonosQueueAdd(target, { ...item, next });
        } catch (e) {
          toasts.error("Couldn't add to the queue", (e as Error).message);
          return undefined;
        }
      });
    },
  };
}
