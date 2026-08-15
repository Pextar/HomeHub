import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { SpotifyResults, SpotifyItem } from "../types";

/**
 * The search store's contract with the three screens that draw it.
 *
 * Each of these is a rule about what a search box may claim, not a detail
 * of one layout: what stays on screen while a newer search runs, what
 * happens to a list when the search behind it fails, which queries earn a
 * place in a room's short history, and how a shelf gets past the tenth
 * result Spotify will answer with.
 */

let results: SpotifyResults;
let fail: Error | null = null;
let pages: Record<string, SpotifyItem[]> = {};

/**
 * Requests can be held mid-flight and released later — the only way to
 * observe what a screen shows *while* a search runs, and the only way to
 * land a stale answer after a newer one has taken the screen.
 */
let holding = false;
let held: (() => void)[] = [];
/** Let every held request finish, and settle what they resolve into. */
async function release() {
  const waiting = held;
  held = [];
  for (const go of waiting) go();
  await vi.advanceTimersByTimeAsync(0);
}

const searchMock = vi.fn(
  async (_q: string, _limit: number, opts: { kind?: string; offset?: number; signal?: AbortSignal } = {}) => {
    if (holding) await new Promise<void>((r) => held.push(r));
    if (opts.signal?.aborted) throw Object.assign(new Error("aborted"), { name: "AbortError" });
    if (fail) throw fail;
    if (opts.kind) return { ...empty(), [opts.kind]: pages[`${opts.kind}:${opts.offset ?? 0}`] ?? [] };
    return results;
  },
);

/** What /spotify/status answers — the grant's shape, in practice. */
let status = { configured: true, connected: true, playback: true, listening: true };
const listeningMock = vi.fn(async () => ({
  recent: [track("Recent")],
  top: [track("Top")],
}));

/** What the neutral media search answers for a second provider. */
const mediaSearchMock = vi.fn(async (_q: string, _opts?: { provider?: string; limit?: number }) => ({
  tracks: [
    {
      provider: "qobuz",
      kind: "track",
      uri: "qobuz:track:42",
      title: "Blue in Green",
      subtitle: "Miles Davis · Kind of Blue",
      art_uri: "https://img/large.jpg",
    },
  ],
  albums: [],
  playlists: [],
  artists: [],
}));
const providersMock = vi.fn(async () => [
  { id: "spotify", name: "Spotify", availability: { ok: true, configured: true }, routes: [], streaming: { ok: true, configured: true } },
  { id: "qobuz", name: "Qobuz", availability: { ok: true, configured: true }, routes: [], streaming: { ok: true, configured: true } },
  { id: "broken", name: "Broken", availability: { ok: false, configured: false, reason: "not set up" }, routes: [], streaming: { ok: false, configured: false } },
]);

vi.mock("../api", () => ({
  api: {
    spotifySearch: (...args: Parameters<typeof searchMock>) => searchMock(...args),
    spotifyStatus: vi.fn(async () => status),
    spotifyMyPlaylists: vi.fn(async () => []),
    spotifyListening: () => listeningMock(),
    mediaSearch: (...args: Parameters<typeof mediaSearchMock>) => mediaSearchMock(...args),
    mediaProviders: () => providersMock(),
  },
}));
vi.mock("../stores.svelte", () => ({ toasts: { error: vi.fn() } }));
vi.mock("../clipboard", () => ({ copyText: vi.fn(async () => true) }));

const { createSpotify } = await import("./spotify.svelte");

function empty(): SpotifyResults {
  return { tracks: [], albums: [], playlists: [], artists: [] };
}
function track(name: string): SpotifyItem {
  return { kind: "track", uri: "spotify:track:" + name, name } as SpotifyItem;
}
function withTracks(...names: string[]): SpotifyResults {
  return { ...empty(), tracks: names.map(track) };
}

/** A store plus the remembered queries it wrote. */
function make() {
  const remembered: string[] = [];
  const store = createSpotify((q) => remembered.push(q));
  return { store, remembered };
}

/** Type `q` and let the debounce fire. */
async function type(store: ReturnType<typeof make>["store"], q: string) {
  store.query = q;
  store.onQueryInput();
  await vi.advanceTimersByTimeAsync(500);
}

beforeEach(() => {
  vi.useFakeTimers();
  results = withTracks("One", "Two");
  fail = null;
  pages = {};
  holding = false;
  held = [];
  status = { configured: true, connected: true, playback: true, listening: true };
  searchMock.mockClear();
  listeningMock.mockClear();
});
afterEach(() => vi.useRealTimers());

// ── What stays on screen ─────────────────────────────────────────────────

describe("a search in flight", () => {
  it("is pending — not stale — when there is nothing on screen yet", async () => {
    const { store } = make();
    holding = true;
    void type(store, "adele");
    await vi.advanceTimersByTimeAsync(500);

    expect(store.pending).toBe(true);
    expect(store.stale).toBe(false);

    holding = false;
    await release();
    expect(store.pending).toBe(false);
  });

  it("keeps the previous results and marks them stale, not pending", async () => {
    // Typing another letter used to blank the list and replace it with
    // skeletons — twice on the way to one word.
    const { store } = make();
    await type(store, "adele");
    expect(store.results?.tracks).toHaveLength(2);

    holding = true;
    void type(store, "adeles");
    await vi.advanceTimersByTimeAsync(500);

    expect(store.stale).toBe(true);
    expect(store.pending).toBe(false);
    expect(store.results?.tracks).toHaveLength(2); // still readable

    holding = false;
    results = withTracks("Three");
    await release();
    expect(store.stale).toBe(false);
    expect(store.results?.tracks).toHaveLength(1);
  });
});

// ── What a failure may leave behind ──────────────────────────────────────

describe("a search that fails", () => {
  it("clears the previous query's results rather than passing them off", async () => {
    const { store } = make();
    await type(store, "adele");
    expect(store.results).not.toBeNull();

    fail = new Error("network down");
    await type(store, "adelex");

    expect(store.results).toBeNull();
    expect(store.error).toBe("network down");
  });

  it("recovers on retry", async () => {
    const { store } = make();
    fail = new Error("network down");
    await type(store, "adele");
    expect(store.error).not.toBeNull();

    fail = null;
    store.retry();
    await vi.advanceTimersByTimeAsync(0);

    expect(store.error).toBeNull();
    expect(store.results?.tracks).toHaveLength(2);
  });
});

// ── What earns a place in the history ────────────────────────────────────

describe("recent searches", () => {
  it("remembers a search that found something", async () => {
    const { store, remembered } = make();
    await type(store, "adele");
    expect(remembered).toEqual(["adele"]);
  });

  it("does not remember a search that found nothing", async () => {
    // Eight slots per room: a typo that takes one evicts a query that works.
    const { store, remembered } = make();
    results = empty();
    await type(store, "adelle");
    expect(remembered).toEqual([]);
  });

  it("does not remember a search that failed", async () => {
    const { store, remembered } = make();
    fail = new Error("network down");
    await type(store, "adele");
    expect(remembered).toEqual([]);
  });
});

// ── What reaches the wire ────────────────────────────────────────────────

describe("typing", () => {
  it("does not search a single character", async () => {
    const { store } = make();
    await type(store, "a");
    expect(searchMock).not.toHaveBeenCalled();
  });

  it("searches a single character when it is submitted outright", async () => {
    const { store } = make();
    store.query = "a";
    store.runNow();
    await vi.advanceTimersByTimeAsync(0);
    expect(searchMock).toHaveBeenCalled();
  });

  it("empties back to the idle shelves without waiting out the debounce", async () => {
    const { store } = make();
    await type(store, "adele");

    store.query = "";
    store.onQueryInput();
    await vi.advanceTimersByTimeAsync(0); // no debounce to wait out

    expect(store.results).toBeNull();
  });

  it("calls off the search it supersedes", async () => {
    // A fast typist otherwise leaves four searches in flight, all counting
    // against the rate limit for answers nobody will read.
    const { store } = make();
    holding = true;
    void type(store, "adele");
    await vi.advanceTimersByTimeAsync(500);

    const first = searchMock.mock.calls[0][2]?.signal;
    holding = false;
    void type(store, "beatles");
    await vi.advanceTimersByTimeAsync(500);

    expect(first?.aborted).toBe(true);

    // And the abandoned request, finishing late, changes nothing.
    await release();
    expect(store.resultsQuery).toBe("beatles");
  });
});

// ── The other thing people type: a name ──────────────────────────────────

describe("the artist a query names", () => {
  function artist(name: string): SpotifyItem {
    return { kind: "artist", uri: "spotify:artist:" + name, name } as SpotifyItem;
  }

  it("is found from a half-typed name", async () => {
    const { store } = make();
    results = { ...withTracks("Someone Like You"), artists: [artist("Adele")] };
    await type(store, "adel");

    expect(store.artistMatch?.name).toBe("Adele");
  });

  it("ignores case and stray spacing", async () => {
    const { store } = make();
    results = { ...empty(), artists: [artist("Adele")] };
    await type(store, "  ADELE ");

    expect(store.artistMatch?.name).toBe("Adele");
  });

  it("still finds the name when the query goes on past it", async () => {
    const { store } = make();
    results = { ...withTracks("Hello"), artists: [artist("Adele")] };
    await type(store, "adele hello");

    expect(store.artistMatch?.name).toBe("Adele");
  });

  it("answers nothing for a song title that merely turns up artists", async () => {
    // Spotify answers a title with artists too; matching one anywhere in
    // the name would put a stranger above the song that was asked for.
    const { store } = make();
    results = { ...withTracks("Rolling in the Deep"), artists: [artist("Adele")] };
    await type(store, "rolling in the deep");

    expect(store.artistMatch).toBeNull();
  });

  it("answers against the query the results are for, not the box", async () => {
    // Mid-word the two differ, and a name matched against a query the list
    // hasn't caught up with is a row nobody asked for.
    const { store } = make();
    results = { ...empty(), artists: [artist("Adele")] };
    await type(store, "adele");
    store.query = "adele rolling";

    expect(store.artistMatch?.name).toBe("Adele");
  });
});

// ── The shelves that mean nobody has to type ─────────────────────────────

describe("listening shelves", () => {
  it("loads what the account has been playing", async () => {
    const { store } = make();
    await store.load();

    expect(store.recentTracks.map((t) => t.name)).toEqual(["Recent"]);
    expect(store.topTracks.map((t) => t.name)).toEqual(["Top"]);
    expect(store.needsListeningScope).toBe(false);
  });

  it("doesn't ask on a grant that predates the scope, and says why", async () => {
    // A login made by an older build searches and plays perfectly well and
    // simply cannot answer this. Asking anyway would just be a 409 per load.
    status = { ...status, listening: false };
    const { store } = make();
    await store.load();

    expect(listeningMock).not.toHaveBeenCalled();
    expect(store.recentTracks).toEqual([]);
    expect(store.needsListeningScope).toBe(true);
  });

  it("costs nothing when the read fails", async () => {
    listeningMock.mockRejectedValueOnce(new Error("gateway"));
    const { store } = make();
    await store.load();

    // The shelves are a convenience: a refusal loses them, not the screen.
    expect(store.recentTracks).toEqual([]);
    expect(store.connected).toBe(true);
  });
});

// ── Getting past the tenth result ────────────────────────────────────────

describe("paging", () => {
  const ten = Array.from({ length: 10 }, (_, i) => `t${i}`);

  it("offers more only while the last page came back full", async () => {
    const { store } = make();
    results = withTracks(...ten);
    await type(store, "adele");
    expect(store.hasMore("tracks")).toBe(true);

    // A short page is how the end of the list announces itself.
    pages["tracks:10"] = [track("t10")];
    store.loadMore("tracks");
    await vi.advanceTimersByTimeAsync(0);

    expect(store.results?.tracks).toHaveLength(11);
    expect(store.hasMore("tracks")).toBe(false);
  });

  it("does not offer more when the first page was already short", async () => {
    const { store } = make();
    await type(store, "adele"); // two tracks
    expect(store.hasMore("tracks")).toBe(false);
  });

  it("asks for the kind and offset the shelf is at", async () => {
    const { store } = make();
    results = withTracks(...ten);
    await type(store, "adele");

    store.loadMore("tracks");
    await vi.advanceTimersByTimeAsync(0);

    expect(searchMock).toHaveBeenLastCalledWith("adele", 10, { kind: "tracks", offset: 10 });
  });

  it("drops a repeat the catalog hands back twice", async () => {
    // Spotify's paging can repeat an item when the catalog shifts between
    // requests, and a repeated key breaks the keyed each blocks.
    const { store } = make();
    results = withTracks(...ten);
    await type(store, "adele");

    pages["tracks:10"] = [track("t9"), track("t10")];
    store.loadMore("tracks");
    await vi.advanceTimersByTimeAsync(0);

    expect(store.results?.tracks).toHaveLength(11);
  });

  it("discards a page that lands after a new search took the screen", async () => {
    const { store } = make();
    results = withTracks(...ten);
    await type(store, "adele");

    // "More songs" goes out, and is still in the air…
    holding = true;
    pages["tracks:10"] = [track("t10")];
    store.loadMore("tracks");
    await vi.advanceTimersByTimeAsync(0);

    // …when a different search answers and takes the screen.
    holding = false;
    results = withTracks("Something else");
    await type(store, "beatles");
    expect(store.results?.tracks).toHaveLength(1);

    // The page now lands. It belongs to a query nobody is looking at.
    await release();
    expect(store.resultsQuery).toBe("beatles");
    expect(store.results?.tracks).toHaveLength(1);
  });
});


describe("searching a second service", () => {
  it("offers only the services that are actually usable", async () => {
    const { store: sp } = make();
    await sp.loadProviders();
    // The unconfigured one is absent rather than present-and-failing: a chip
    // that searches nothing is worse than no chip.
    expect(sp.providers.map((p) => p.id)).toEqual(["spotify", "qobuz"]);
  });

  it("maps the neutral search into the shape every row already draws", async () => {
    const { store: sp } = make();
    await sp.loadProviders();
    sp.provider = "qobuz";
    await type(sp, "kind of blue");

    expect(mediaSearchMock).toHaveBeenCalledWith("kind of blue", expect.objectContaining({ provider: "qobuz" }));
    const t = sp.results?.tracks[0];
    expect(t?.name).toBe("Blue in Green");
    expect(t?.sub).toBe("Miles Davis · Kind of Blue");
    expect(t?.art_url).toBe("https://img/large.jpg");
    // The provider rides on the item, because a URI is only playable by the
    // service that issued it and the play path has to know which.
    expect(t?.provider).toBe("qobuz");
  });

  it("clears the other service's results when the chip changes", async () => {
    results = withTracks("A Spotify Song");
    const { store: sp } = make();
    await sp.loadProviders();
    await type(sp, "blue");
    expect(sp.results?.tracks[0]?.name).toBe("A Spotify Song");

    sp.provider = "qobuz";
    // Cleared immediately — leaving them on screen under a new chip would be
    // a list that lies about where it came from.
    expect(sp.results).toBeNull();
    await vi.advanceTimersByTimeAsync(500);
    expect(sp.results?.tracks[0]?.provider).toBe("qobuz");
  });

  it("keeps using Spotify's own endpoint for Spotify", async () => {
    results = withTracks("Native");
    const { store: sp } = make();
    await type(sp, "blue");
    // Spotify's endpoint knows about artists, saved albums and the rest that
    // the neutral one does not, so it must not be routed through it.
    expect(searchMock).toHaveBeenCalled();
    expect(sp.results?.tracks[0]?.provider).toBeUndefined();
  });
});
