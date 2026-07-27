import type { Room, RoomKind, RoomsModel } from "./rooms.svelte";

/**
 * Where playback lands — and, now that Music speaks one noun, also *which room
 * the whole module is looking at*.
 *
 * These used to be two ideas. A chip row picked a "destination" for favorites
 * and search, and separately a tap somewhere else opened a player for a room;
 * you could easily be reading one room and queueing to another. They are the
 * same choice, so this is now one piece of state: the focused room. The hero
 * shows it, the player opens it, and anything you start goes to it.
 *
 * It is still expressed as a `{ kind, id }` rather than a room object, because
 * the room list is rebuilt on every poll and a held object would go stale —
 * and because the three kinds are genuinely not interchangeable underneath: a
 * Sonos room takes content through its coordinator's queue, a KEF speaker
 * through Spotify Connect, a zone through the media layer's route engine.
 */
export type Dest = { kind: RoomKind; id: string };

export interface Destination {
  /** The focused room, or null in a house with no speakers. */
  current: Dest | null;
  /** The room itself, resolved against the current poll. */
  readonly room: Room | null;
  /** Everywhere music can be sent, in the order the picker lists them. */
  readonly list: Room[];
  /** The Sonos coordinator, when the focus is one. Favorites and the queue
   *  exist only on that side, so they read this and not `current`. */
  readonly sonosTarget: string | null;
  /** What to call it in a toast or the picker's label. */
  readonly label: string;
  /** Stable key for per-destination state (the search history). */
  readonly key: string | null;

  focus(r: Room): void;
  is(r: Room): boolean;
  /**
   * Keep the focus pointing at something that exists, preferring a room that
   * is already playing — "play this too" almost always means the room the
   * music is coming out of. Call it from an effect.
   */
  settle(): void;
}

export function createDestination(rooms: RoomsModel): Destination {
  const s = $state({ current: null as Dest | null });

  const room = $derived(s.current ? rooms.byKey(`${s.current.kind}:${s.current.id}`) : null);

  return {
    get current() {
      return s.current;
    },
    set current(d: Dest | null) {
      s.current = d;
    },
    get room() {
      return room;
    },
    get list() {
      return rooms.list;
    },
    get sonosTarget() {
      return room?.kind === "sonos" ? room.id : null;
    },
    get label() {
      return room?.name ?? "";
    },
    get key() {
      return room?.key ?? null;
    },

    focus: (r) => (s.current = { kind: r.kind, id: r.id }),
    is: (r) => s.current?.kind === r.kind && s.current.id === r.id,

    settle() {
      const options = rooms.list; // read first: this has to re-run when it changes
      if (room) return;
      const pick = rooms.playing[0] ?? rooms.withTrack[0] ?? options[0];
      if (pick) s.current = { kind: pick.kind, id: pick.id };
    },
  };
}
