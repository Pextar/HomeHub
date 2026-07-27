import type { KEFSpeakerView, MediaZone } from "../types";
import type { SonosBridge } from "./sonos.svelte";
import type { KEFBridge } from "./kef.svelte";
import type { ZonesBridge } from "./zones.svelte";

/**
 * Where playback lands — one destination for the whole module (DESIGN.md §15,
 * "one visible destination"), shared by favorites and search.
 *
 * It spans every way HomeHub can start music, which is why it carries a *kind*
 * rather than a bare id. A Sonos zone is started through its coordinator's
 * queue; a KEF speaker through Spotify Connect, because its own API can play
 * and pause but has nothing to be handed; a HomeHub zone through
 * `/api/media/zones/{id}/play`, which resolves a route across whatever makes
 * are in it. The three are not interchangeable, and saying which is which is
 * the whole job of this type.
 *
 * The third kind is what the media protocol added. It does not supersede the
 * other two: a zone is a thing the user built and named, and a house that
 * hasn't built one still has to be able to play to a room.
 */
export type Dest = { kind: "sonos" | "kef" | "zone"; id: string };

export interface Destination {
  /** The chosen destination, or null in a house with no speakers. */
  current: Dest | null;
  /** Everywhere music can be sent, in the order the row lists them. */
  readonly list: Dest[];
  /** The Sonos coordinator, when the destination is one. Favorites and the
   *  queue exist only on that side, so they read this and not `current`. */
  readonly sonosTarget: string | null;
  /** The KEF speaker, when the destination is one. */
  readonly kefTarget: string | null;
  readonly kefSpeaker: KEFSpeakerView | null;
  /** The HomeHub zone, when the destination is one. */
  readonly zone: MediaZone | null;
  /** What to call it in a toast or the one-destination label. */
  readonly label: string;
  /** Stable key for per-destination state (the search history). */
  readonly key: string | null;

  name(d: Dest): string;
  is(d: Dest): boolean;
  /**
   * Keep the destination pointing at something that exists, preferring a room
   * that is already playing — "play this too" almost always means the room the
   * music is coming out of. Call it from an effect: it reads both bridges, so
   * a KEF speaker that answers first is a perfectly good default in a house
   * without Sonos.
   */
  settle(): void;
}

export function createDestination(
  sonos: SonosBridge,
  kef: KEFBridge,
  zones: ZonesBridge,
): Destination {
  const s = $state({ current: null as Dest | null });

  // Rooms first, then single KEF speakers, then the zones the user built —
  // the order the row labels its groups in. A zone with no speakers in it is
  // left out: it stores, but the media layer refuses to play to it, so
  // offering it as a destination would be offering a failure.
  // Unreachable KEF speakers are left out for the same kind of reason: they
  // have no Connect device while they're off the network.
  const list = $derived<Dest[]>([
    ...sonos.groups.map((g) => ({ kind: "sonos" as const, id: g.coordinator_id })),
    ...kef.reachable.map((x) => ({ kind: "kef" as const, id: x.id })),
    ...zones.zones
      .filter((z) => zones.speakersOf(z).length > 0)
      .map((z) => ({ kind: "zone" as const, id: z.id })),
  ]);

  const is = (d: Dest) => s.current?.kind === d.kind && s.current.id === d.id;

  function name(d: Dest): string {
    if (d.kind === "kef") return kef.byId(d.id)?.name ?? "Speaker";
    if (d.kind === "zone") return zones.byId(d.id)?.name ?? "Zone";
    const g = sonos.groupById(d.id);
    return g ? sonos.groupTitle(g) : "Zone";
  }

  const kefTarget = $derived(s.current?.kind === "kef" ? s.current.id : null);
  const zoneTarget = $derived(s.current?.kind === "zone" ? s.current.id : null);

  return {
    get current() {
      return s.current;
    },
    set current(d: Dest | null) {
      s.current = d;
    },
    get list() {
      return list;
    },
    get sonosTarget() {
      return s.current?.kind === "sonos" ? s.current.id : null;
    },
    get kefTarget() {
      return kefTarget;
    },
    get kefSpeaker() {
      return kef.byId(kefTarget);
    },
    get zone() {
      return zones.byId(zoneTarget);
    },
    get label() {
      return s.current ? name(s.current) : "";
    },
    get key() {
      return s.current ? `${s.current.kind}:${s.current.id}` : null;
    },
    name,
    is,
    settle() {
      const options = list; // read first: this has to re-run when it changes
      if (s.current && options.some((d) => is(d))) return;
      // A playing zone wins over a playing room inside it: the zone is the
      // thing the audio is actually coming out of, and picking one of its
      // members would send the next track to a subset of what you can hear.
      const zonePick = zones.playing[0] && { kind: "zone" as const, id: zones.playing[0].id };
      const livePick = sonos.playingGroups[0] && {
        kind: "sonos" as const,
        id: sonos.playingGroups[0].coordinator_id,
      };
      const kefPick = kef.playing[0] && { kind: "kef" as const, id: kef.playing[0].id };
      s.current = zonePick ?? livePick ?? kefPick ?? options[0] ?? null;
    },
  };
}
