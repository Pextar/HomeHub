/**
 * Putting something on, on the wall.
 *
 * Everything that starts audio in the featured room comes through here: a
 * search result, a household favorite, and "more like this". They look like
 * three features and share the one thing that is actually hard — `startOn`,
 * which knows that a play takes a different road per make and that nothing
 * above it should have to.
 *
 * Three roads, one call:
 *   **Sonos** takes the item into its queue and plays it.
 *   **KEF** goes out through Spotify Connect.
 *   **A zone** hands the media layer a provider and a URI and lets it resolve
 *   a route across whatever makes are in the room.
 *
 * Two things that look like details and are not.
 *
 * **An artist has no URI a speaker takes** (DESIGN.md §15), so starting one
 * resolves their top track first and plays *that* — which the player then
 * names, rather than claiming to be playing an artist.
 *
 * **A non-Sonos play answers before the audio does.** A KEF or a streamed
 * zone is accepted by Spotify, goes out to the cloud and comes back, so the
 * read immediately after can still honestly say "stopped". Two backstop
 * re-reads cover that gap; without them the wall shows a stopped room while
 * music is playing in it.
 *
 * The favorites list is a Sonos *household's*, not a room's, so it is read
 * once off any reachable speaker rather than per featured room — and only a
 * Sonos room can take one.
 */

import { api } from "../api";
import { toasts } from "../stores.svelte";
import { haptic } from "../utils";
import type { PlayItemBody } from "../api";
import type { SonosFavorite, SonosSpeakerView, SpotifyItem } from "../types";
import type { PanelSource, PanelRunner } from "./types";

export interface PanelStartingDeps {
  /** The room the wall is pointed at. A getter: it moves under this. */
  featured: () => PanelSource | undefined;
  /** Every Sonos speaker the hub knows, for finding one to ask about the
   *  household's favorites. */
  speakers: () => SonosSpeakerView[];
  /** The store's busy map, so a tile disables itself while its call is out. */
  busy: Record<string, boolean>;
  run: PanelRunner;
  /** Re-read everything — a play changes what every surface says. */
  refresh: () => Promise<void>;
  /** Take the featured room's queue again after adding to it. */
  reloadQueue: (coordinatorId: string) => Promise<void>;
}

export interface PanelStartingStore {
  readonly favorites: SonosFavorite[];
  playFavorite(f: SonosFavorite): void;
  playItem(item: SpotifyItem): Promise<void>;
  /** Hand one item to whichever bridge the destination belongs to. Exposed
   *  because the history shelf starts things too. */
  startOn(s: PanelSource, body: PlayItemBody): Promise<void>;

  /** Something is playing with an artist to seed from. */
  readonly canRadio: boolean;
  startRadio(): void;
  /** What the last run added, for the in-place confirmation — queueing
   *  changes nothing visible on its own. */
  readonly lastRadio: { count: number; artist: string; at: number } | null;
}

export function createPanelStarting(deps: PanelStartingDeps): PanelStartingStore {
  // ── The household's own list ────────────────────────────────────────
  let favorites = $state<SonosFavorite[]>([]);
  let favsFor = "";
  $effect(() => {
    // Household-wide, so any Sonos speaker can answer for the list — read
    // once per household rather than per featured room.
    const anySonos = deps.speakers().find((sp) => sp.reachable);
    const id = anySonos?.id ?? "";
    if (!id || id === favsFor) return;
    favsFor = id;
    void api
      .sonosFavorites(id)
      .then((f) => {
        if (favsFor === id) favorites = f;
      })
      .catch(() => {
        if (favsFor === id) favorites = [];
      });
  });

  async function startOn(s: PanelSource, body: PlayItemBody) {
    if (s.kind === "zone") {
      await api.mediaZonePlay(s.id, {
        provider: "spotify",
        uri: body.uri,
        title: body.title,
        kind: body.kind,
        sub: body.sub,
        art_uri: body.art_uri,
      });
    } else if (s.kind === "sonos") {
      await api.sonosPlayItem(s.id, body);
    } else {
      await api.kefPlayItem(s.id, body);
    }
    await deps.refresh();
    if (s.kind !== "sonos") for (const ms of [1200, 4000]) setTimeout(() => void deps.refresh(), ms);
  }

  // ── More like this ──────────────────────────────────────────────────
  let lastRadio = $state<{ count: number; artist: string; at: number } | null>(null);

  return {
    get favorites() {
      return favorites;
    },

    playFavorite(f) {
      const s = deps.featured();
      if (!s || s.kind !== "sonos") return;
      void deps.run("fav:" + f.id, () => api.sonosPlayFavorite(s.id, f), "Couldn't play that");
    },

    startOn,

    /**
     * The featured source is the destination — the chips above the player are
     * how it is chosen — and the player is the confirmation, since playback
     * is invisible until the next poll lands.
     */
    async playItem(item) {
      const s = deps.featured();
      if (!s) return;
      const key = "item:" + item.uri;
      if (deps.busy[key]) return;
      deps.busy[key] = true;
      haptic();
      try {
        let body: PlayItemBody = {
          service: "Spotify",
          uri: item.uri,
          title: item.name,
          kind: item.kind,
          // Carried for the room's history rather than for the speaker: a
          // shelf tile needs a picture and a second line, and asking the
          // catalog for them again later would be a service round-trip to
          // redraw a row we already have.
          sub: item.sub,
          art_uri: item.art_url,
        };
        if (item.kind === "artist") {
          const d = await api.spotifyArtist(item.uri);
          const top = d.top_tracks[0];
          if (!top) throw new Error(`No tracks found for ${item.name}`);
          body = {
            service: "Spotify",
            uri: top.uri,
            title: top.name,
            kind: "track",
            sub: top.sub ?? item.name,
            art_uri: top.art_url ?? item.art_url,
          };
        }
        await startOn(s, body);
      } catch (e) {
        toasts.error("Couldn't play", (e as Error).message);
      } finally {
        deps.busy[key] = false;
      }
    },

    get canRadio() {
      return !!deps.featured()?.trackArtist;
    },
    get lastRadio() {
      return lastRadio;
    },

    /**
     * The same engine "play similar" uses when a queue runs dry (§15.5),
     * asked for on purpose instead of automatically. Seeded by artist *name*
     * because that is what a speaker reports — a room on radio has an artist
     * line and no catalog id at all.
     *
     * On Sonos it fills the queue behind what is playing, so the record you
     * are listening to isn't interrupted by asking for more of it. Anywhere
     * else there is no queue to fill, so the first result plays.
     */
    startRadio() {
      const s = deps.featured();
      const artist = s?.trackArtist;
      if (!s || !artist) return;
      void deps.run(
        "radio",
        async () => {
          const items = await api.spotifySimilar(artist, 8);
          if (!items.length) throw new Error(`Nothing else by ${artist} came back`);
          if (s.kind === "sonos") {
            // The whole run in one request. This used to be eight sequential
            // calls sent *backwards* — Sonos resolves each "play next"
            // against wherever the queue is at that moment, so a forwards
            // loop scatters the run and a reversed one happens to come out in
            // order. That trick worked and was a trick, and it cost eight
            // round trips from the slowest client this app has. The hub does
            // the dealing now: one request, one position read, and the order
            // of the array is the order they land in.
            const added = await api.sonosQueueAddMany(
              s.id,
              items.map((item) => ({ service: "Spotify", uri: item.uri, title: item.name })),
              true,
            );
            await deps.reloadQueue(s.id);
            lastRadio = { count: added.added || items.length, artist, at: Date.now() };
          } else {
            await startOn(s, {
              service: "Spotify",
              uri: items[0].uri,
              title: items[0].name,
              kind: "track",
              sub: items[0].sub,
              art_uri: items[0].art_url,
            });
            lastRadio = { count: 1, artist, at: Date.now() };
          }
        },
        "Couldn't find more like this",
      );
    },
  };
}
