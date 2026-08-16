/**
 * Starting something, on whichever kind of room has the focus.
 *
 * One tap, three roads. A Sonos room loads the item into its queue and
 * streams it with the household's linked account; a KEF speaker is started
 * through Spotify Connect, because its own API has no way to be handed
 * content; a HomeHub room hands it to the media layer, which resolves a
 * route across whatever makes are in it and answers with the one it chose.
 *
 * Which road a tap takes is a property of the room, not of the screen the
 * tap came from — so it lives here rather than in the view, where it sat
 * between the router and the sheet bookkeeping.
 *
 * There is no toast on success on purpose: the player is the confirmation,
 * and the track, the room and the route all land on it as soon as the
 * re-read returns. Saying so in a card as well meant every tap on a search
 * result was followed by a repeat of what the screen already showed.
 */

import { api } from "../api";
import { toasts } from "../stores.svelte";
import type { Busy } from "./busy.svelte";
import type { SonosBridge } from "./sonos.svelte";
import type { KEFBridge } from "./kef.svelte";
import type { ZonesBridge } from "./zones.svelte";
import type { Destination } from "./destination.svelte";
import type { Room } from "./rooms.svelte";
import type { HeardTrack, SonosFavorite, SpotifyItem } from "../types";

/** What a queue call takes: a URI, and whatever the caller knows to label
 *  it with. Narrower than `SpotifyItem` because a favorite is queueable
 *  too and is not one. */
export interface QueueItem {
  uri: string;
  title?: string;
  service?: string;
  metadata?: string;
}

export interface Playback {
  /** A search result, an album, a playlist — played on the focused room. */
  playItem(item: SpotifyItem): void;
  /** Something out of the listening log, played again in the room it was
   *  heard in. */
  playHeard(t: HeardTrack): void;
  /** Favorites are a Sonos household list, so only a Sonos room takes one. */
  playFavorite(f: SonosFavorite, target?: string | null): void;
  /** Queue without disturbing what's playing. */
  enqueue(item: QueueItem, next: boolean, target?: string | null): Promise<void>;
  /** Drop the delayed re-reads still pending — the view's `onDestroy`. */
  dispose(): void;
}

export interface PlaybackDeps {
  busy: Busy;
  sonos: SonosBridge;
  kef: KEFBridge;
  zones: ZonesBridge;
  destination: Destination;
  /**
   * The room whose queue is on screen, if any. A getter, because the player
   * opens and closes under this and a queue re-read must follow it — read
   * once at construction it would keep re-reading the room the player was
   * showing when the view mounted.
   */
  playerRoom: () => Room | null | undefined;
}

export function createPlayback(deps: PlaybackDeps): Playback {
  const { busy, sonos, kef, zones, destination } = deps;

  /** Delayed re-reads that must not outlive the view. Nothing renders this
   *  set — it exists only so `dispose` can cancel what's still pending — so
   *  a reactive one would be bookkeeping for no reader. */
  // eslint-disable-next-line svelte/prefer-svelte-reactivity
  const followUps = new Set<ReturnType<typeof setTimeout>>();
  function followUp(ms: number, fn: () => void) {
    const t = setTimeout(() => {
      followUps.delete(t);
      fn();
    }, ms);
    followUps.add(t);
  }

  async function startPlayback(
    key: string,
    fn: () => Promise<unknown>,
    kind: "sonos" | "kef" | "zone" = "sonos",
  ) {
    await busy.claim(key, async () => {
      try {
        await fn();
        await (kind === "kef" ? kef.refresh() : kind === "zone" ? zones.refresh() : sonos.refresh());
        // A KEF play answers as soon as *Spotify* accepted it — the audio
        // then goes out to the cloud and comes back — so the read above
        // still says "stopped". A streamed room has the same gap: the
        // decoder has to start and every speaker has to fill its buffer.
        // These are the backstop for an install where the backend's own
        // push isn't getting through.
        if (kind !== "sonos") {
          const again = kind === "kef" ? kef.refresh : zones.refresh;
          for (const ms of [1200, 4000]) followUp(ms, again);
        }
      } catch (e) {
        toasts.error("Couldn't play", (e as Error).message);
      }
    });
  }

  function playItem(item: SpotifyItem) {
    const r = destination.room;
    if (!r) return;
    const provider = item.provider ?? "spotify";
    if (r.zone) {
      const z = r.zone;
      void startPlayback(
        "item:" + item.uri,
        () => zones.play(z, { uri: item.uri, title: item.name, kind: item.kind, provider }),
        "zone",
      );
      return;
    }
    // A bare speaker is played through its own bridge, and those two doors
    // take a *native service* the speaker streams from its own account
    // link. Only Spotify has one here. Anything else has to go through the
    // media layer, which addresses zones — so the honest answer is to say
    // that rather than send a URI the speaker will ignore.
    if (provider !== "spotify") {
      toasts.error(
        `${item.name} can't play here`,
        "This service is decoded by HomeHub rather than by the speaker, so it plays to a zone. Put this speaker in a zone and pick that instead.",
      );
      return;
    }
    const body = { service: "Spotify", uri: item.uri, title: item.name };
    void startPlayback(
      "item:" + item.uri,
      () => (r.kind === "kef" ? api.kefPlayItem(r.id, body) : api.sonosPlayItem(r.id, body)),
      r.kind,
    );
  }

  return {
    playItem,

    playHeard(t) {
      if (!t.uri) return; // radio and line-in leave nothing to hand back
      // It is a track and it carries the service URI the speaker was given,
      // so this is the same road a search result takes — the row was only
      // ever a remembered version of one.
      playItem({
        kind: "track",
        uri: t.uri,
        name: t.title,
        sub: t.artist,
        art_url: t.art_uri,
      });
    },

    playFavorite(f, target = destination.sonosTarget) {
      if (!target) return;
      void startPlayback("fav:" + f.id, () => api.sonosPlayFavorite(target, f));
    },

    async enqueue(item, next, target = destination.sonosTarget) {
      if (!target) return;
      const added = await sonos.enqueue(target, item, next);
      if (!added) return;
      if (deps.playerRoom()?.id === target) void sonos.loadQueue(target);
    },

    dispose() {
      for (const t of followUps) clearTimeout(t);
      followUps.clear();
    },
  };
}
