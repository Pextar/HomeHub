/**
 * Artist and record pages, fetched once per URI and kept for the session.
 *
 * Three surfaces drill into the catalog — the app's Music view, the panel's
 * browse depth, and the kid module's search — and each keeps its own idea of
 * *where* a page sits: Music pushes a screen onto its router stack, the panel
 * pushes a level inside the search pane, the kid surface pushes a plain level.
 * What none of them differ on is the fetch behind the page: cache by URI,
 * flag the one in flight, toast and back out when it fails.
 *
 * So only the loading is shared here. The host says which URI each kind is
 * showing (`artistUri` / `contextUri` — getters, since they follow the host's
 * stack), pushes its own level, then calls `loadArtist` / `loadContext`. A
 * failed load calls `onFail` to undo the push the host just made, and only
 * when the level that failed is still the one on top: by then the reader may
 * have gone somewhere else, and popping a level they since opened would take
 * away a page that loaded fine.
 *
 * Two of the three hosts turned out to keep the *same* stack around this —
 * artist over results, record over artist — and that ladder is shared in
 * `catalog-stack.svelte.ts`, which wraps this. Reach for that one first; come
 * here directly only for a host whose levels are its own, as the Music view's
 * screen router is.
 *
 * An artist or an album changes slowly and the stack wanders back and forth
 * (artist → album → back), so a URI already read renders instantly instead of
 * replaying its skeleton.
 */

import { api } from "../api";
import { toasts } from "../stores.svelte";
import type { SpotifyArtistDetail, SpotifyContextDetail, SpotifyItem } from "../types";

export interface CatalogCache {
  /** The artist page for `artistUri`, or null when it hasn't been read yet. */
  readonly artistDetail: SpotifyArtistDetail | null;
  /** The read behind the artist page currently on top is still out. */
  readonly artistLoading: boolean;
  readonly contextDetail: SpotifyContextDetail | null;
  readonly contextLoading: boolean;
  /**
   * Fetch the artist page, unless this URI has already been read. Call it
   * *after* pushing the level that shows it — a failure calls `onFail` to
   * take that level back.
   */
  loadArtist(uri: string): Promise<void>;
  loadContext(uri: string): Promise<void>;
}

export interface CatalogCacheOptions {
  /** The artist URI the host's stack is showing, or null for none. */
  artistUri: () => string | null;
  /** The album/playlist URI the host's stack is showing, or null for none. */
  contextUri: () => string | null;
  /** Undo the push behind a failed load — the host's own back step. */
  onFail: () => void;
  /** Toast title when an artist page won't load. The kid module words its
   *  own; everywhere else takes the default. */
  artistError?: string;
}

/**
 * The opened record as an item — the same shape a search row carries, so the
 * one-tap "play this album/playlist" makes the call a tap on that row makes.
 */
export function contextItem(c: SpotifyContextDetail): SpotifyItem {
  return { kind: c.kind, uri: c.uri, name: c.name, sub: c.sub, art_url: c.art_url };
}

export function createCatalogCache(opts: CatalogCacheOptions): CatalogCache {
  const artistError = opts.artistError ?? "Couldn't load artist";

  const artists = $state<Record<string, SpotifyArtistDetail>>({});
  const contexts = $state<Record<string, SpotifyContextDetail>>({});
  let artistLoadingUri = $state<string | null>(null);
  let contextLoadingUri = $state<string | null>(null);

  return {
    get artistDetail() {
      const uri = opts.artistUri();
      return uri ? (artists[uri] ?? null) : null;
    },
    get artistLoading() {
      const uri = opts.artistUri();
      return !!uri && artistLoadingUri === uri;
    },
    get contextDetail() {
      const uri = opts.contextUri();
      return uri ? (contexts[uri] ?? null) : null;
    },
    get contextLoading() {
      const uri = opts.contextUri();
      return !!uri && contextLoadingUri === uri;
    },

    async loadArtist(uri) {
      if (artists[uri]) return; // been here — renders instantly
      artistLoadingUri = uri;
      try {
        artists[uri] = await api.spotifyArtist(uri);
      } catch (e) {
        toasts.error(artistError, (e as Error).message);
        if (opts.artistUri() === uri) opts.onFail();
      } finally {
        if (artistLoadingUri === uri) artistLoadingUri = null;
      }
    },

    async loadContext(uri) {
      if (contexts[uri]) return;
      contextLoadingUri = uri;
      try {
        contexts[uri] = await api.spotifyContext(uri);
      } catch (e) {
        toasts.error("Couldn't open it", (e as Error).message);
        if (opts.contextUri() === uri) opts.onFail();
      } finally {
        if (contextLoadingUri === uri) contextLoadingUri = null;
      }
    },
  };
}
