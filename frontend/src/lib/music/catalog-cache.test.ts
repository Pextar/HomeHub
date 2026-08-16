import { describe, it, expect, beforeEach, vi } from "vitest";
import type { SpotifyArtistDetail, SpotifyContextDetail } from "../types";

/**
 * The catalog cache's contract with the three surfaces that drill into it.
 *
 * Each of these is a rule about what a page may claim, not a detail of one
 * layout: what a second visit costs, what happens to the level a failed read
 * opened, and what a stale failure is allowed to take away from someone who
 * has since navigated on.
 */

let fail: Error | null = null;
/** Requests can be held mid-flight, the only way to observe a page while its
 *  read is out and to land a stale failure after the reader moved on. */
let holding = false;
let held: (() => void)[] = [];
async function release() {
  const waiting = held;
  held = [];
  for (const go of waiting) go();
  await Promise.resolve();
  await Promise.resolve();
}

const artistMock = vi.fn(async (uri: string): Promise<SpotifyArtistDetail> => {
  if (holding) await new Promise<void>((r) => held.push(r));
  if (fail) throw fail;
  return { uri, name: "Bowie" } as SpotifyArtistDetail;
});
const contextMock = vi.fn(async (uri: string): Promise<SpotifyContextDetail> => {
  if (holding) await new Promise<void>((r) => held.push(r));
  if (fail) throw fail;
  return { uri, kind: "album", name: "Low" } as SpotifyContextDetail;
});
const errorToast = vi.fn();

vi.mock("../api", () => ({
  api: {
    spotifyArtist: (uri: string) => artistMock(uri),
    spotifyContext: (uri: string) => contextMock(uri),
  },
}));
vi.mock("../stores.svelte", () => ({ toasts: { error: (...a: unknown[]) => errorToast(...a) } }));

const { createCatalogCache, contextItem } = await import("./catalog-cache.svelte");

/** A cache over a host stack this test drives directly, plus the back steps
 *  it was asked to take. */
function make(opts: { artistError?: string } = {}) {
  let artistUri: string | null = null;
  let contextUri: string | null = null;
  const popped: string[] = [];
  const cache = createCatalogCache({
    artistUri: () => artistUri,
    contextUri: () => contextUri,
    onFail: () => popped.push(artistUri ?? contextUri ?? ""),
    ...opts,
  });
  return {
    cache,
    popped,
    /** What the host does before every load: push the level. */
    openArtist(uri: string | null) {
      artistUri = uri;
    },
    openContext(uri: string | null) {
      contextUri = uri;
    },
  };
}

beforeEach(() => {
  fail = null;
  holding = false;
  held = [];
  artistMock.mockClear();
  contextMock.mockClear();
  errorToast.mockClear();
});

describe("catalog cache", () => {
  it("reads an artist page and serves it under its own URI", async () => {
    const h = make();
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    expect(h.cache.artistDetail?.name).toBe("Bowie");
    expect(h.cache.artistLoading).toBe(false);
  });

  it("shows nothing for a level that isn't open", async () => {
    const h = make();
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    h.openArtist(null);
    expect(h.cache.artistDetail).toBeNull();
  });

  it("reads each URI once — coming back is instant", async () => {
    const h = make();
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    h.openArtist(null);
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    expect(artistMock).toHaveBeenCalledTimes(1);
    expect(h.cache.artistDetail?.name).toBe("Bowie");
    // A cached page never flashes its skeleton on the way back.
    expect(h.cache.artistLoading).toBe(false);
  });

  it("flags loading only for the level whose read is out", async () => {
    const h = make();
    holding = true;
    h.openArtist("spotify:artist:1");
    const done = h.cache.loadArtist("spotify:artist:1");
    expect(h.cache.artistLoading).toBe(true);
    // Gone deeper while the read is out: the level on top isn't waiting.
    h.openArtist("spotify:artist:2");
    expect(h.cache.artistLoading).toBe(false);
    await release();
    await done;
  });

  it("backs out of the level a failed read opened, and says why", async () => {
    const h = make();
    fail = new Error("offline");
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    expect(h.popped).toEqual(["spotify:artist:1"]);
    expect(errorToast).toHaveBeenCalledWith("Couldn't load artist", "offline");
  });

  it("takes the kid module's wording for a failed artist read", async () => {
    const h = make({ artistError: "Couldn't load the artist" });
    fail = new Error("offline");
    h.openArtist("spotify:artist:1");
    await h.cache.loadArtist("spotify:artist:1");
    expect(errorToast).toHaveBeenCalledWith("Couldn't load the artist", "offline");
  });

  it("leaves a page the reader has since moved off alone when a stale read fails", async () => {
    const h = make();
    holding = true;
    fail = new Error("offline");
    h.openArtist("spotify:artist:1");
    const done = h.cache.loadArtist("spotify:artist:1");
    // Moved on before the failure lands — popping now would take away a page
    // that is fine.
    h.openArtist("spotify:artist:2");
    await release();
    await done;
    expect(h.popped).toEqual([]);
    expect(errorToast).toHaveBeenCalled();
  });

  it("keeps records on their own shelf, apart from artists", async () => {
    const h = make();
    h.openContext("spotify:album:1");
    await h.cache.loadContext("spotify:album:1");
    expect(h.cache.contextDetail?.name).toBe("Low");
    expect(h.cache.artistDetail).toBeNull();
    expect(artistMock).not.toHaveBeenCalled();
  });

  it("backs out of a record that won't open", async () => {
    const h = make();
    fail = new Error("gone");
    h.openContext("spotify:album:1");
    await h.cache.loadContext("spotify:album:1");
    expect(h.popped).toEqual(["spotify:album:1"]);
    expect(errorToast).toHaveBeenCalledWith("Couldn't open it", "gone");
  });

  it("hands an opened record back as the item its search row carries", () => {
    const det = {
      kind: "album",
      uri: "spotify:album:1",
      name: "Low",
      sub: "David Bowie",
      art_url: "https://img/low.jpg",
      tracks: [],
    } as unknown as SpotifyContextDetail;
    expect(contextItem(det)).toEqual({
      kind: "album",
      uri: "spotify:album:1",
      name: "Low",
      sub: "David Bowie",
      art_url: "https://img/low.jpg",
    });
  });
});
