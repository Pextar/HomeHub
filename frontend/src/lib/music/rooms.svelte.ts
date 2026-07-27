import { api } from "../api";
import { toasts } from "../stores.svelte";
import { kefSourceLabel } from "../kef";
import type { Busy } from "./busy.svelte";
import type { SonosBridge } from "./sonos.svelte";
import type { KEFBridge } from "./kef.svelte";
import type { ZonesBridge } from "./zones.svelte";
import { secs } from "./time";
import { clampVol } from "./volume";
import type {
  KEFSpeakerView,
  MediaZone,
  SonosGroupView,
  SonosSpeakerView,
} from "../types";

/**
 * Rooms: the one noun the Music module speaks.
 *
 * Underneath there are still three very different things — a Sonos group, a
 * lone KEF speaker, and a HomeHub zone that can span both — and the bridges
 * that drive them stay exactly as they are. What was wrong was that the *user*
 * had to know which of the three they were looking at: the same speaker
 * appeared as a zone chip, a group member and a draggable puck on three
 * separate surfaces, each with its own way in and its own transport.
 *
 * So this is the adapter that collapses them. A room is "a thing you can play
 * music on", it owns a set of speakers, and every speaker belongs to exactly
 * one room — a zone wins over a Sonos group, which wins over a lone speaker,
 * because that is the order of how much the user deliberately built it.
 *
 * Capabilities are carried, never guessed at from the make: a KEF has no
 * queue, a streamed zone has no skip, a radio stream has no seek. Surfaces ask
 * the room what it can do and render only that, which is what keeps a control
 * that would be refused off the screen.
 *
 * Live values (position, volume, what's playing) are **methods**, not fields.
 * The list is rebuilt when the topology changes; a progress bar that ticks
 * once a second must not rebuild it, or every card in the grid would churn.
 */

export type RoomKind = "sonos" | "kef" | "zone";

/** One speaker's fader inside a room, wired to whichever bridge owns it. */
export interface RoomFader {
  key: string;
  name: string;
  value: number;
  muted: boolean;
  muteBusy: boolean;
  onInput: (v: number) => void;
  onChange: (v: number) => void;
  onMute: () => void;
  /** Present only where dropping this speaker out of the room is meaningful. */
  onRemove?: () => void;
  removeBusy?: boolean;
}

export interface Room {
  /** `"<kind>:<id>"` — stable across polls, and what focus and drag key on. */
  key: string;
  kind: RoomKind;
  id: string;
  name: string;
  /** Speaker display names inside it, in the room's own order. */
  members: string[];
  /** Sonos speaker ids inside it. */
  sonosIds: string[];
  /** KEF speaker ids inside it. */
  kefIds: string[];
  /** Bridge-qualified member ids (`"sonos:x"`), the zone API's currency. */
  memberIds: string[];
  /** More than one speaker — the thing a grouping gesture built. */
  grouped: boolean;
  reachable: boolean;

  canSkip: boolean;
  canSeek: boolean;
  canQueue: boolean;
  /** Sonos shuffle/repeat/crossfade — only a group has them. */
  canPlayMode: boolean;
  /** A KEF speaker's input selector. */
  canPickInput: boolean;

  group?: SonosGroupView;
  speaker?: KEFSpeakerView;
  zone?: MediaZone;
}

export interface RoomsModel {
  /** Every room, in a stable order that a play or a pause never reshuffles. */
  readonly list: Room[];
  readonly playing: Room[];
  /** Rooms with a track loaded, playing or paused — what the dock can hold. */
  readonly withTrack: Room[];
  /** False until all three reads have answered at least once. */
  readonly loaded: boolean;
  /** Registered speakers no room can contain, because nothing answered. */
  readonly offline: { id: string; name: string }[];

  byKey(key: string | null | undefined): Room | null;
  keyOf(kind: RoomKind, id: string): string;

  isPlaying(r: Room): boolean;
  /** True when there is a track loaded, playing or not. */
  hasTrack(r: Room): boolean;
  art(r: Room): string | undefined;
  /** The track's title, or "" — the test for "is there anything loaded". */
  title(r: Room): string;
  /** The headline: a track title, or an honest stand-in ("Idle", "Standby"). */
  nowLine(r: Room): string;
  /** Artist · album, or the input, or "" when there is nothing to add. */
  subLine(r: Room): string;
  /** "Living Room + Kitchen" — what this room is made of. */
  memberLine(r: Room): string;
  positionSec(r: Room): number;
  durationSec(r: Room): number;
  progress(r: Room): number;
  /** The scrubber's position: like `positionSec`, but a just-issued seek wins. */
  livePosition(r: Room): number;
  volume(r: Room): number;
  muted(r: Room): boolean;

  playBusy(r: Room): boolean;
  prevBusy(r: Room): boolean;
  nextBusy(r: Room): boolean;

  togglePlay(r: Room): void;
  skip(r: Room, dir: "next" | "previous"): void;
  seek(r: Room, sec: number): void;
  clearSeek(): void;
  dragVolume(r: Room, v: number): void;
  setVolume(r: Room, v: number): void;
  nudgeVolume(r: Room, delta: number): void;
  toggleMute(r: Room): void;
  /** One fader per speaker, plus the room-wide one where there is a choice. */
  faders(r: Room): RoomFader[];
  /** Sonos shuffle/repeat/crossfade/queue length, where they exist. */
  groupState(r: Room): SonosSpeakerView["group_state"];

  /** Whether dropping `source` on `target` would do anything at all. */
  canGroup(source: Room, target: Room): boolean;
  /** Do it. Answers with a sentence to announce, or null if nothing happened. */
  group(source: Room, target: Room): Promise<string | null>;
  /** Take a room apart. Zones are the caller's to confirm and delete. */
  ungroup(r: Room): Promise<void>;
  ungroupBusy(r: Room): boolean;
  groupingBusy(r: Room): boolean;
}

/** `"sonos:RINCON_x"` — the id shape the zone API stores members under. */
function qualify(vendor: "sonos" | "kef", id: string): string {
  return `${vendor}:${id}`;
}

export function createRooms(
  sonos: SonosBridge,
  kef: KEFBridge,
  zones: ZonesBridge,
  busy: Busy,
): RoomsModel {
  /**
   * Every speaker a zone already claims. A zone is something the user built
   * and named, so it outranks the topology's own grouping: showing both would
   * put the same speaker on two cards under two names, which is the exact
   * confusion this model exists to remove.
   */
  const claimed = $derived.by(() => {
    // Plain sets: rebuilt whole by this derivation, never mutated after.
    /* eslint-disable svelte/prefer-svelte-reactivity */
    const s = new Set<string>();
    const k = new Set<string>();
    /* eslint-enable svelte/prefer-svelte-reactivity */
    for (const z of zones.zones) {
      for (const sp of zones.speakersOf(z)) (sp.vendor === "kef" ? k : s).add(sp.id);
    }
    return { sonos: s, kef: k };
  });

  function zoneRoom(z: MediaZone): Room {
    const members = zones.speakersOf(z);
    const sonosIds = members.filter((m) => m.vendor !== "kef").map((m) => m.id);
    const kefIds = members.filter((m) => m.vendor === "kef").map((m) => m.id);
    return {
      key: "zone:" + z.id,
      kind: "zone",
      id: z.id,
      name: z.name,
      members: members.map((m) => m.name),
      sonosIds,
      kefIds,
      memberIds: members.map((m) => m.member),
      grouped: members.length > 1,
      reachable: members.length > 0,
      // On the stream route HomeHub is the Spotify device and the speakers are
      // pulling a live stream: `next` sent to one is a call it refuses.
      canSkip: z.route !== "stream",
      canSeek: false, // there is no zone seek — a fan-out of a stream can't be scrubbed
      canQueue: false,
      canPlayMode: false,
      canPickInput: false,
      zone: z,
    };
  }

  function sonosRoom(g: SonosGroupView): Room {
    const speakers = g.member_ids
      .map((id) => sonos.speakerById.get(id))
      .filter((x): x is SonosSpeakerView => !!x);
    return {
      key: "sonos:" + g.coordinator_id,
      kind: "sonos",
      id: g.coordinator_id,
      name: sonos.groupTitle(g),
      members: speakers.map((x) => x.name),
      sonosIds: g.member_ids,
      kefIds: [],
      memberIds: g.member_ids.map((id) => qualify("sonos", id)),
      grouped: g.member_ids.length > 1,
      reachable: speakers.some((x) => x.reachable),
      canSkip: true,
      canSeek: true, // per track, actually — `durationSec` is the real gate
      canQueue: true,
      canPlayMode: true,
      canPickInput: false,
      group: g,
    };
  }

  function kefRoom(sp: KEFSpeakerView): Room {
    return {
      key: "kef:" + sp.id,
      kind: "kef",
      id: sp.id,
      name: sp.name,
      members: [sp.name],
      sonosIds: [],
      kefIds: [sp.id],
      memberIds: [qualify("kef", sp.id)],
      grouped: false,
      reachable: sp.reachable,
      canSkip: true,
      canSeek: false, // KEF's API has no seek at all
      canQueue: false,
      canPlayMode: false,
      canPickInput: true,
      speaker: sp,
    };
  }

  /**
   * Zones first — they are the arrangement someone made on purpose — then the
   * household's own grouping, then the speakers that stand alone. The order is
   * deliberately *not* "playing first": a card that jumps across the grid the
   * moment you press play is a card you can't drag onto anything.
   */
  const list = $derived.by<Room[]>(() => {
    const out: Room[] = [];
    for (const z of zones.zones) {
      if (zones.speakersOf(z).length > 0) out.push(zoneRoom(z));
    }
    for (const g of sonos.groups) {
      if (g.member_ids.some((id) => claimed.sonos.has(id))) continue;
      out.push(sonosRoom(g));
    }
    for (const sp of kef.speakers) {
      if (!sp.reachable || claimed.kef.has(sp.id)) continue;
      out.push(kefRoom(sp));
    }
    return out;
  });

  function isPlaying(r: Room): boolean {
    if (r.zone) return zones.isPlaying(r.zone);
    if (r.speaker) return kef.isPlaying(r.speaker);
    return r.group ? sonos.isPlaying(r.group) : false;
  }

  function title(r: Room): string {
    if (r.zone) return zones.leadOf(r.zone)?.state?.track?.title ?? "";
    if (r.speaker) return r.speaker.state?.track?.title ?? "";
    return sonos.coordinatorOf(r.group!)?.state?.track?.title ?? "";
  }

  const playing = $derived(list.filter((r) => isPlaying(r)));
  const withTrack = $derived(list.filter((r) => isPlaying(r) || !!title(r)));

  /**
   * Registered speakers that couldn't become a room, from either bridge. A
   * Sonos speaker the live topology never mentioned and a KEF speaker that
   * isn't answering are the same problem wearing two names, and both are
   * fixed in the same place, so Home counts them together.
   */
  const offline = $derived<{ id: string; name: string }[]>([
    ...sonos.offline.map((sp) => ({ id: sp.id, name: sp.name })),
    ...kef.speakers.filter((sp) => !sp.reachable).map((sp) => ({ id: sp.id, name: sp.name })),
  ]);

  function durationSec(r: Room): number {
    if (r.zone) return zones.durationMs(r.zone) / 1000;
    if (r.speaker) return (r.speaker.state?.duration_ms ?? 0) / 1000;
    return secs(sonos.coordinatorOf(r.group!)?.state?.duration);
  }

  /** The fader for one Sonos speaker, wired to the Sonos bridge. */
  function sonosFader(sp: SonosSpeakerView, onRemove?: () => void): RoomFader {
    return {
      key: "sonos:" + sp.id,
      name: sp.name,
      value: sonos.shownVolume(sp),
      muted: !!sp.state?.muted,
      muteBusy: busy.is("mute:" + sp.id),
      onInput: (v) => sonos.dragVolume(sp.id, v),
      onChange: (v) => sonos.setVolume(sp.id, v),
      onMute: () => sonos.toggleMute(sp),
      onRemove,
      removeBusy: busy.is("leave:" + sp.id),
    };
  }

  function kefFader(sp: KEFSpeakerView, onRemove?: () => void): RoomFader {
    return {
      key: "kef:" + sp.id,
      name: sp.name,
      value: kef.shownVolume(sp),
      muted: !!sp.state?.muted,
      muteBusy: busy.is("kefmute:" + sp.id),
      onInput: (v) => kef.dragVolume(sp, v),
      onChange: (v) => kef.setVolume(sp, v),
      onMute: () => kef.toggleMute(sp),
      onRemove,
    };
  }

  /** Drop one speaker out of a zone, leaving the rest of it alone. */
  function dropFromZone(z: MediaZone, member: string) {
    void busy.claim("zmembers:" + member, async () => {
      await zones.update(z.id, { members: z.members.filter((m) => m !== member) });
    });
  }

  // ── Grouping ─────────────────────────────────────────────────────────
  // One gesture, two mechanisms, and the user is told which one it took
  // rather than having to pick it. Two Sonos rooms group natively, because
  // that is what the household is for and it is bit-perfect. Anything else —
  // a KEF in the mix, or a zone already in play — becomes a HomeHub zone,
  // because that is the only thing that can span makes.

  function isSonosPair(source: Room, target: Room): boolean {
    return source.kind === "sonos" && target.kind === "sonos";
  }

  function canGroup(source: Room, target: Room): boolean {
    if (source.key === target.key) return false;
    if (!source.reachable || !target.reachable) return false;
    // Nothing to add: everything the source holds is already in the target.
    return !source.memberIds.every((m) => target.memberIds.includes(m));
  }

  async function group(source: Room, target: Room): Promise<string | null> {
    if (!canGroup(source, target)) return null;

    if (isSonosPair(source, target)) {
      // The whole card moves, not just one speaker: what was dragged was a
      // room, and leaving half of it behind is not what the gesture said.
      return (
        (await busy.claim("group:" + target.key, async () => {
          try {
            for (const id of source.sonosIds) await api.sonosJoin(id, target.id);
            await sonos.refresh();
            return `${source.name} now plays with ${target.name}.`;
          } catch (e) {
            toasts.error("Grouping failed", (e as Error).message);
            return null;
          }
        })) ?? null
      );
    }

    // Everything else goes through a zone, which is the only thing that can
    // hold two makes at once. The target absorbs the source, so the card you
    // dropped onto is the one that survives — except when only the source is
    // a zone, where there is nothing else to absorb into.
    const host = target.zone ? target : source.zone ? source : null;
    const other = host === target ? source : target;

    return (
      (await busy.claim("group:" + target.key, async () => {
        try {
          if (host?.zone) {
            const merged = [...host.zone.members];
            for (const m of other.memberIds) if (!merged.includes(m)) merged.push(m);
            const saved = await zones.update(host.zone.id, { members: merged });
            if (!saved) return null;
            // Two zones merged: the emptied one would otherwise sit there as a
            // duplicate name over the same speakers.
            if (other.zone) await zones.remove(other.zone.id);
            await zones.refresh();
            return `${other.name} now plays with ${host.name}.`;
          }
          // Neither side is a zone yet — a KEF and a Sonos, or two KEFs. The
          // new zone takes the target's name, since that is the card that
          // stays put under the finger.
          const created = await zones.create({
            name: target.name,
            members: [...target.memberIds, ...source.memberIds],
          });
          if (!created) return null;
          await zones.refresh();
          return `${source.name} and ${target.name} now play together as ${created.name}.`;
        } catch (e) {
          toasts.error("Grouping failed", (e as Error).message);
          return null;
        }
      })) ?? null
    );
  }

  return {
    get list() {
      return list;
    },
    get playing() {
      return playing;
    },
    get withTrack() {
      return withTrack;
    },
    get loaded() {
      return sonos.loaded && zones.loaded;
    },
    get offline() {
      return offline;
    },

    byKey: (key) => (key ? (list.find((r) => r.key === key) ?? null) : null),
    keyOf: (kind, id) => `${kind}:${id}`,

    isPlaying,
    hasTrack: (r) => isPlaying(r) || !!title(r),
    title,

    art(r) {
      if (r.zone) return zones.leadOf(r.zone)?.state?.track?.art_uri;
      if (r.speaker) return r.speaker.state?.track?.art_uri;
      return sonos.coordinatorOf(r.group!)?.state?.track?.art_uri;
    },

    nowLine(r) {
      if (r.zone) return zones.nowLine(r.zone);
      if (r.speaker) return kef.nowLine(r.speaker);
      const st = sonos.coordinatorOf(r.group!)?.state;
      if (!st?.track?.title) return "Idle";
      return isPlaying(r) ? st.track.title : `Paused · ${st.track.title}`;
    },

    subLine(r) {
      if (r.zone) return zones.subLine(r.zone);
      if (r.speaker) {
        const line = kef.subLine(r.speaker);
        if (line) return line;
        return r.speaker.state?.source ? kefSourceLabel(r.speaker.state.source) : "";
      }
      const t = sonos.coordinatorOf(r.group!)?.state?.track;
      return [t?.artist, t?.album].filter(Boolean).join(" · ");
    },

    memberLine(r) {
      if (r.members.length <= 1) return r.members[0] ?? "";
      if (r.members.length === 2) return r.members.join(" + ");
      return `${r.members[0]} + ${r.members.length - 1} more`;
    },

    positionSec(r) {
      if (r.zone) return zones.positionMs(r.zone) / 1000;
      if (r.speaker) return kef.positionMs(r.speaker) / 1000;
      return sonos.positionOf(r.group!);
    },

    durationSec,

    progress(r) {
      if (r.zone) return zones.progress(r.zone);
      if (r.speaker) return kef.progress(r.speaker);
      return sonos.progressOf(r.group!);
    },

    livePosition(r) {
      if (r.group) return sonos.livePosition(r.group);
      return r.zone ? zones.positionMs(r.zone) / 1000 : kef.positionMs(r.speaker!) / 1000;
    },

    volume(r) {
      if (r.zone) return zones.shownVolume(r.zone);
      if (r.speaker) return kef.shownVolume(r.speaker);
      // A lone Sonos speaker's own level; a group's is the members' mean,
      // which is what a group-wide set writes back to them.
      if (r.grouped) return sonos.shownGroupVolume(r.id);
      const sp = sonos.coordinatorOf(r.group!);
      return sp ? sonos.shownVolume(sp) : 0;
    },

    muted(r) {
      if (r.zone) return zones.isMuted(r.zone);
      if (r.speaker) return !!r.speaker.state?.muted;
      // One audible speaker means the room is audible.
      return r.sonosIds.every((id) => !!sonos.speakerById.get(id)?.state?.muted);
    },

    playBusy: (r) =>
      r.zone ? busy.is("zplay:" + r.id)
        : r.speaker ? busy.is("kefplay:" + r.id)
          : busy.is("play:" + r.id),
    prevBusy: (r) =>
      r.zone ? busy.is("zprevious:" + r.id)
        : r.speaker ? busy.is("kefprevious:" + r.id)
          : busy.is("previous:" + r.id),
    nextBusy: (r) =>
      r.zone ? busy.is("znext:" + r.id)
        : r.speaker ? busy.is("kefnext:" + r.id)
          : busy.is("next:" + r.id),

    togglePlay(r) {
      if (r.zone) return void zones.togglePlay(r.zone);
      if (r.speaker) return void kef.togglePlay(r.speaker);
      void sonos.togglePlay(r.group!);
    },

    skip(r, dir) {
      if (!r.canSkip) return;
      if (r.zone) return zones.skip(r.zone, dir);
      if (r.speaker) return kef.skip(r.speaker, dir);
      sonos.skip(r.group!, dir);
    },

    seek(r, sec) {
      if (r.group) sonos.seek(r.group, sec);
    },
    clearSeek: () => sonos.clearSeek(),

    dragVolume(r, v) {
      if (r.zone) return zones.dragVolume(r.zone, v);
      if (r.speaker) return kef.dragVolume(r.speaker, v);
      if (r.grouped) return sonos.dragGroupVolume(r.id, v);
      sonos.dragVolume(r.id, v);
    },

    setVolume(r, v) {
      const level = clampVol(v);
      if (r.zone) return zones.setVolume(r.zone, level);
      if (r.speaker) return kef.setVolume(r.speaker, level);
      if (r.grouped) return sonos.setGroupVolume(r.id, level);
      sonos.setVolume(r.id, level);
    },

    nudgeVolume(r, delta) {
      if (r.group) return sonos.nudgeVolume(r.group, delta);
      if (r.zone) return zones.setVolume(r.zone, zones.shownVolume(r.zone) + delta);
      kef.setVolume(r.speaker!, kef.shownVolume(r.speaker!) + delta);
    },

    toggleMute(r) {
      if (r.zone) return zones.toggleMute(r.zone);
      if (r.speaker) return kef.toggleMute(r.speaker);
      sonos.toggleMuteGroup(r.group!);
    },

    faders(r) {
      const out: RoomFader[] = [];
      if (r.zone) {
        const zone = r.zone;
        for (const m of zones.speakersOf(zone)) {
          const remove = r.grouped ? () => dropFromZone(zone, m.member) : undefined;
          const sp = m.vendor === "kef" ? kef.byId(m.id) : sonos.speakerById.get(m.id);
          if (!sp) continue;
          const f = m.vendor === "kef" ? kefFader(sp as KEFSpeakerView, remove)
            : sonosFader(sp as SonosSpeakerView, remove);
          // A speaker here leaves the *room*, not a Sonos group, so the busy
          // key is the membership edit's rather than `leave:`.
          f.removeBusy = busy.is("zmembers:" + m.member);
          out.push(f);
        }
        return out;
      }
      if (r.speaker) return [kefFader(r.speaker)];
      for (const id of r.sonosIds) {
        const sp = sonos.speakerById.get(id);
        if (sp) out.push(sonosFader(sp, r.grouped ? () => sonos.leave(id) : undefined));
      }
      return out;
    },

    groupState: (r) => (r.group ? sonos.groupStateOf(r.group) : undefined),

    canGroup,
    group,

    async ungroup(r) {
      if (r.group) return await sonos.ungroup(r.group);
      // A zone is a thing with a name; taking it apart is deleting it, which
      // is the caller's to confirm. Nothing to do here.
    },
    ungroupBusy: (r) => busy.is("ungroup:" + r.id),
    groupingBusy: (r) => busy.is("group:" + r.key),
  };
}
