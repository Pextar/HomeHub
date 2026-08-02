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
  /** A search is in flight. On its own this is *not* a reason to clear the
   *  screen — see `pending` and `stale`. */
  readonly searching: boolean;
  /** Nothing to show yet and something on the way: the skeleton's moment,
   *  and the only one. */
  readonly pending: boolean;
  /** Results on screen, and a newer search running behind them. The list
   *  stays put and dims — a search that blanks what you were reading every
   *  time you type another letter is unusable. */
  readonly stale: boolean;
  /** The query the results on screen actually answer. */
  readonly resultsQuery: string;
  /** Why the last search didn't land. Cleared when one does. */
  readonly error: string | null;
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

  /** Typing: debounced by 400ms, and not run at all under two characters. */
  onQueryInput(): void;
  /** Run what is in the box now, skipping the debounce and the minimum. */
  runNow(): void;
  /** Set the box to `q` and run it. */
  runQuery(q: string): void;
  clearQuery(): void;
  /** Run the failed search again — the only useful answer to an error. */
  retry(): void;

  // ── Paging ──
  /** More of one kind, appended. Spotify caps a search at ten results per
   *  kind, so this is the only way to an eleventh. */
  loadMore(kind: Exclude<SpotifyKind, "all">): void;
  /** Whether that kind has more behind it — false once a page comes back
   *  short, which is how the end of the list announces itself. */
  hasMore(kind: Exclude<SpotifyKind, "all">): boolean;
  readonly loadingMore: boolean;
}

const DEBOUNCE_MS = 400;
/** Below this, typing isn't a query yet — one letter matches everything and
 *  costs a round trip to say so. Enter still runs whatever is in the box. */
const MIN_QUERY = 2;
/** Spotify's own per-search cap; also the page size for "show more". */
const PAGE = 10;

/**
 * The same "one thing this search was almost certainly after" pick the top
 * result card uses — shared so a history chip's picture always matches the
 * card the search itself led with.
 */
function topOf(r: SpotifyResults, q: string): SpotifyItem | null {
  const { tracks, artists, albums, playlists } = r;
  const ql = q.trim().toLowerCase();
  if (artists[0] && artists[0].name.toLowerCase() === ql) return artists[0];
  return tracks[0] ?? artists[0] ?? albums[0] ?? playlists[0] ?? null;
}

/** Every result across the four kinds — "did this search find anything". */
function countOf(r: SpotifyResults): number {
  return r.tracks.length + r.albums.length + r.playlists.length + r.artists.length;
}

/**
 * `remember` is how a run gets into the history — passed in because the
 * history is keyed by destination, which this module has no business knowing
 * about.
 *
 * It is called once, *after* a search comes back having found something.
 * Remembering on the way in instead meant every typo, every zero-result
 * query and every search that failed outright took one of the eight slots
 * a room's history has, evicting the queries that actually worked.
 */
export function createSpotify(
  remember: (q: string, art?: { art_url?: string; round?: boolean }) => void,
): SpotifyStore {
  const s = $state({
    status: null as SpotifyStatus | null,
    query: "",
    searching: false,
    results: null as SpotifyResults | null,
    /** The query `results` answer — not necessarily what is in the box. */
    resultsQuery: "",
    error: null as string | null,
    /** Kinds a further page came back short on: the end of that list. */
    ended: {} as Record<string, boolean>,
    loadingMore: false,
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
  /** The request the current search owns, so a superseded one is called off
   *  rather than left to finish and be thrown away. A fast typist otherwise
   *  leaves four searches in flight, all counting against the rate limit. */
  let inflight: AbortController | undefined;

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

  /**
   * One search. The results already on screen are kept until this one
   * answers — `stale` is what a surface dims, and only a screen with
   * nothing on it yet gets a skeleton.
   *
   * A failure clears them instead: a list from the previous query, sitting
   * under this query's text, reads as an answer to a question nobody asked.
   * `error` says what happened and `retry()` is the way out.
   */
  async function search() {
    const q = s.query.trim();
    const mine = ++seq;
    // Whatever was in flight is now answering a question that has changed.
    inflight?.abort();
    inflight = new AbortController();
    const signal = inflight.signal;
    if (!q) {
      s.results = null;
      s.resultsQuery = "";
      s.error = null;
      s.searching = false;
      return;
    }
    // A freshly submitted query always opens on the broad overview — a
    // kind filter left over from the previous search would otherwise hide
    // whichever section answers this one best.
    s.kindFilter = "all";
    s.searching = true;
    s.error = null;
    try {
      // 10 is Spotify's own cap for /search now — asking for more is a 400;
      // `loadMore` pages past it with an offset.
      const r = await api.spotifySearch(q, PAGE, { signal });
      if (mine !== seq) return;
      s.results = r;
      s.resultsQuery = q;
      s.ended = {};
      // Worth remembering only now, and only having found something.
      if (countOf(r) > 0) {
        const top = topOf(r, q);
        remember(q, top?.art_url ? { art_url: top.art_url, round: top.kind === "artist" } : undefined);
      }
    } catch (e) {
      // An abort is this function superseding itself, not a failure.
      if (mine !== seq || (e as Error).name === "AbortError") return;
      s.results = null;
      s.resultsQuery = "";
      s.error = (e as Error).message || "Spotify didn't answer.";
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
    get pending() {
      return s.searching && !s.results;
    },
    get stale() {
      return s.searching && !!s.results;
    },
    get resultsQuery() {
      return s.resultsQuery;
    },
    get error() {
      return s.error;
    },
    get loadingMore() {
      return s.loadingMore;
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
      // An artist whose name is the query outright is what a search for a
      // name is almost always after — otherwise the top track wins, since
      // playing a song is the most common reason to search at all.
      return s.results ? topOf(s.results, s.query) : null;
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
      const q = s.query.trim();
      // Emptying the box goes back to the idle shelves at once — waiting out
      // a debounce to show what is already known is just a stutter.
      if (!q) {
        void search();
        return;
      }
      // One letter matches everything and costs a round trip to say so.
      // Enter still runs whatever is in the box, for the names that short.
      if (q.length < MIN_QUERY) return;
      timer = setTimeout(() => void search(), DEBOUNCE_MS);
    },

    runNow() {
      clearTimeout(timer);
      void search();
    },

    runQuery(q) {
      clearTimeout(timer);
      s.query = q;
      void search();
    },

    clearQuery() {
      clearTimeout(timer);
      seq++; // drop an in-flight search
      inflight?.abort();
      s.query = "";
      s.results = null;
      s.resultsQuery = "";
      s.error = null;
      s.searching = false;
    },

    retry() {
      clearTimeout(timer);
      void search();
    },

    hasMore(kind) {
      const r = s.results;
      if (!r) return false;
      // A kind is worth paging while its list is a whole number of pages
      // and no page has come back short. Spotify reports no total we can
      // trust across kinds, so the short page *is* the end signal.
      const n = r[kind].length;
      return n > 0 && n % PAGE === 0 && !s.ended[kind];
    },

    loadMore(kind) {
      const r = s.results;
      const q = s.resultsQuery;
      if (!r || !q || s.loadingMore || s.ended[kind]) return;
      const offset = r[kind].length;
      s.loadingMore = true;
      const mine = seq;
      void api
        .spotifySearch(q, PAGE, { kind, offset })
        .then((page) => {
          // A search that started in the meantime owns the screen now.
          if (mine !== seq || !s.results || s.resultsQuery !== q) return;
          const more = page[kind];
          if (more.length < PAGE) s.ended[kind] = true;
          // De-duped by URI: Spotify's paging can repeat an item when the
          // catalog shifts between requests, and a repeated key would
          // break the keyed each blocks the shelves are built from. A plain
          // Set: built and read inside this callback, never observed.
          // eslint-disable-next-line svelte/prefer-svelte-reactivity
          const seen = new Set(s.results[kind].map((x) => x.uri));
          s.results = {
            ...s.results,
            [kind]: [...s.results[kind], ...more.filter((x) => !seen.has(x.uri))],
          };
        })
        .catch((e: Error) => {
          if (mine !== seq) return;
          toasts.error("Couldn't load more", e.message);
        })
        .finally(() => {
          s.loadingMore = false;
        });
    },
  };
}
