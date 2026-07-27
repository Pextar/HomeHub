import { api } from "../api";
import { toasts } from "../stores.svelte";
import { copyText } from "../clipboard";
import type { SpotifyStatus, SpotifyItem, SpotifyResults } from "../types";

/**
 * Spotify: the one-time account setup, and the search that feeds every
 * speaker in the module.
 *
 * Two things it deliberately does not own. The **confirm** before
 * disconnecting is the caller's, because raising a dialog is a surface's job.
 * And **focus** — putting the caret in the box — stays with the component that
 * owns the input, since only it has the element.
 */
export type SpotifyKind = "all" | "tracks" | "albums" | "playlists" | "artists";

export interface SpotifyStore {
  /** Null when the integration is unavailable — the whole card hides. */
  readonly status: SpotifyStatus | null;
  readonly connected: boolean;
  /** What the results list shows: matches, or the account's own playlists. */
  readonly shownItems: SpotifyItem[];
  /** The single best match across all four kinds, for the "Top result" card —
   *  an artist whose name matches the query outright, otherwise the top
   *  track, falling back down the kind order. Null with no results. */
  readonly topResult: SpotifyItem | null;
  readonly results: SpotifyResults | null;
  readonly searching: boolean;
  readonly myPlaylists: SpotifyItem[];
  query: string;
  kindFilter: SpotifyKind;

  // The client-ID form, expanded either on first run or on request.
  setupOpen: boolean;
  clientId: string;
  readonly saving: boolean;
  pasteUrl: string;
  readonly finishing: boolean;
  /** Briefly true after the redirect URI is copied, so the button can say so. */
  readonly copied: boolean;

  load(): Promise<void>;
  saveClientId(): Promise<void>;
  copyRedirect(): Promise<void>;
  connect(): Promise<void>;
  finishConnect(): Promise<void>;
  /** Drops the tokens. Confirm before calling — this does not ask. */
  disconnect(): Promise<void>;

  /** Typing: debounced by 400ms. */
  onQueryInput(): void;
  /** Run what is in the box now, skipping the debounce. */
  runNow(): void;
  /** Set the box to `q`, remember it, and run it. */
  runQuery(q: string): void;
  clearQuery(): void;
}

const DEBOUNCE_MS = 400;

/**
 * `remember` is how a run gets into the history — passed in because the
 * history is keyed by destination, which this module has no business knowing
 * about.
 */
export function createSpotify(remember: (q: string) => void): SpotifyStore {
  const s = $state({
    status: null as SpotifyStatus | null,
    query: "",
    searching: false,
    results: null as SpotifyResults | null,
    kindFilter: "all" as SpotifyKind,
    myPlaylists: [] as SpotifyItem[],
    setupOpen: false,
    clientId: "",
    saving: false,
    pasteUrl: "",
    finishing: false,
    copied: false,
  });

  let playlistsLoaded = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let seq = 0;

  async function load() {
    try {
      s.status = await api.spotifyStatus();
      if (s.status.connected && !playlistsLoaded) {
        playlistsLoaded = true;
        s.myPlaylists = await api.spotifyMyPlaylists().catch(() => []);
      }
    } catch {
      s.status = null; // integration unavailable — hide the card
    }
  }

  async function search() {
    const q = s.query.trim();
    const mine = ++seq;
    if (!q) {
      s.results = null;
      s.searching = false;
      return;
    }
    // A freshly submitted query always opens on the broad overview — a
    // kind filter left over from the previous search would otherwise hide
    // whichever section answers this one best.
    s.kindFilter = "all";
    s.searching = true;
    try {
      const r = await api.spotifySearch(q, 8);
      if (mine !== seq) return;
      s.results = r;
    } catch (e) {
      if (mine !== seq) return;
      toasts.error("Search failed", (e as Error).message);
    } finally {
      if (mine === seq) s.searching = false;
    }
  }

  return {
    get status() {
      return s.status;
    },
    get connected() {
      return !!s.status?.connected;
    },
    get results() {
      return s.results;
    },
    get searching() {
      return s.searching;
    },
    get myPlaylists() {
      return s.myPlaylists;
    },
    // With no query the list browses the account's playlists instead of
    // sitting empty. "all" has no array of its own — it's every kind at
    // once, which is also what an empty-results check needs to see.
    get shownItems() {
      if (!s.results) return s.myPlaylists;
      if (s.kindFilter === "all") {
        return [...s.results.tracks, ...s.results.albums, ...s.results.playlists, ...s.results.artists];
      }
      return s.results[s.kindFilter];
    },
    get topResult() {
      if (!s.results) return null;
      const { tracks, artists, albums, playlists } = s.results;
      const q = s.query.trim().toLowerCase();
      // An artist whose name is the query outright is what a search for a
      // name is almost always after — otherwise the top track wins, since
      // playing a song is the most common reason to search at all.
      if (artists[0] && artists[0].name.toLowerCase() === q) return artists[0];
      return tracks[0] ?? artists[0] ?? albums[0] ?? playlists[0] ?? null;
    },
    get query() {
      return s.query;
    },
    set query(v: string) {
      s.query = v;
    },
    get kindFilter() {
      return s.kindFilter;
    },
    set kindFilter(v: SpotifyKind) {
      s.kindFilter = v;
    },
    get setupOpen() {
      return s.setupOpen;
    },
    set setupOpen(v: boolean) {
      s.setupOpen = v;
    },
    get clientId() {
      return s.clientId;
    },
    set clientId(v: string) {
      s.clientId = v;
    },
    get saving() {
      return s.saving;
    },
    get pasteUrl() {
      return s.pasteUrl;
    },
    set pasteUrl(v: string) {
      s.pasteUrl = v;
    },
    get finishing() {
      return s.finishing;
    },
    get copied() {
      return s.copied;
    },

    load,

    async saveClientId() {
      if (s.saving || !s.clientId.trim()) return;
      s.saving = true;
      try {
        await api.spotifySetConfig(s.clientId.trim());
        s.setupOpen = false;
        await load();
        toasts.success("Client ID saved", "Now connect your Spotify account.");
      } catch (e) {
        toasts.error("Save failed", (e as Error).message);
      } finally {
        s.saving = false;
      }
    },

    async copyRedirect() {
      if (!s.status) return;
      if (await copyText(s.status.redirect_uri)) {
        s.copied = true;
        setTimeout(() => (s.copied = false), 1800);
      }
    },

    async connect() {
      // Manual flow: keep this page open — the consent tab is opened
      // synchronously (before the await) so popup blockers allow it, then
      // pointed at the authorize URL once it arrives.
      const tab = s.status?.manual ? window.open("about:blank", "_blank") : null;
      try {
        const { url } = await api.spotifyLoginURL();
        if (s.status?.manual) {
          if (tab) tab.location.href = url;
          else window.location.href = url; // popup blocked — same tab still works
        } else {
          window.location.href = url; // bounces back here automatically
        }
      } catch (e) {
        tab?.close();
        toasts.error("Couldn't start Spotify login", (e as Error).message);
      }
    },

    async finishConnect() {
      if (s.finishing || !s.pasteUrl.trim()) return;
      s.finishing = true;
      try {
        await api.spotifyExchange(s.pasteUrl);
        s.pasteUrl = "";
        toasts.success("Spotify connected");
        await load();
      } catch (e) {
        toasts.error("Couldn't finish the login", (e as Error).message);
      } finally {
        s.finishing = false;
      }
    },

    async disconnect() {
      try {
        await api.spotifyDisconnect();
        s.results = null;
        s.query = "";
        s.myPlaylists = [];
        playlistsLoaded = false;
        await load();
      } catch (e) {
        toasts.error("Disconnect failed", (e as Error).message);
      }
    },

    onQueryInput() {
      clearTimeout(timer);
      timer = setTimeout(search, DEBOUNCE_MS);
    },

    runNow() {
      clearTimeout(timer);
      const q = s.query.trim();
      if (q) remember(q);
      void search();
    },

    runQuery(q) {
      clearTimeout(timer);
      s.query = q;
      remember(q);
      void search();
    },

    clearQuery() {
      clearTimeout(timer);
      seq++; // drop an in-flight search
      s.query = "";
      s.results = null;
      s.searching = false;
    },
  };
}
