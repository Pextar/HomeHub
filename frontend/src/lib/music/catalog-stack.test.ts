import { describe, it, expect, beforeEach, vi } from "vitest";
import type { SpotifyArtistDetail, SpotifyContextDetail } from "../types";

/**
 * The ladder into the catalog, as the wall's depth and the kid module's
 * search both climb it.
 *
 * These are rules about what a rung means — when one goes on, what it costs
 * to ask for the one already showing, what a failed read owes back, and what
 * a surface is told so it can remember the query underneath — rather than
 * about how either of them draws a page.
 */

let fail: Error | null = null;

const artistMock = vi.fn(async (uri: string): Promise<SpotifyArtistDetail> => {
  if (fail) throw fail;
  return { uri, name: "Bowie" } as SpotifyArtistDetail;
});
const contextMock = vi.fn(async (uri: string): Promise<SpotifyContextDetail> => {
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

const { createCatalogStack } = await import("./catalog-stack.svelte");

/** A stack plus a note of everything it asked its host to do. */
function make(opts: { artistError?: string } = {}) {
  const opened: (unknown | undefined)[] = [];
  const pushes: number[] = [];
  const pops: number[] = [];
  /** Where each level is scrolled, as a scrollport the test moves by hand. */
  let position = 0;
  const restored: number[] = [];
  const stack = createCatalogStack({
    onOpened: (art) => opened.push(art),
    // A level that goes on is a fresh scrollport at its own top, the way a
    // newly mounted element is. `scrollOf` has already run by here.
    onPush: () => {
      pushes.push(1);
      position = 0;
    },
    onPop: () => pops.push(1),
    scrollOf: () => position,
    scrollTo: (y) => {
      restored.push(y);
      position = y;
    },
    ...opts,
  });
  return {
    stack,
    opened,
    pushes,
    pops,
    restored,
    /** Scroll whatever level is showing. */
    scrollTo(y: number) {
      position = y;
    },
    get position() {
      return position;
    },
  };
}

beforeEach(() => {
  fail = null;
  artistMock.mockClear();
  contextMock.mockClear();
  errorToast.mockClear();
});

describe("catalog stack", () => {
  it("starts on the results themselves — no rung climbed", () => {
    const h = make();
    expect(h.stack.depth).toBe(0);
    expect(h.stack.top).toBeNull();
  });

  it("puts an artist's page on top and reads it", async () => {
    const h = make();
    await h.stack.openArtist("spotify:artist:1");
    expect(h.stack.depth).toBe(1);
    expect(h.stack.top).toEqual({ kind: "artist", uri: "spotify:artist:1" });
    expect(h.stack.artistDetail?.name).toBe("Bowie");
  });

  it("stacks a record over the artist it was opened from", async () => {
    const h = make();
    await h.stack.openArtist("spotify:artist:1");
    await h.stack.openContext("spotify:album:1");
    expect(h.stack.depth).toBe(2);
    expect(h.stack.contextDetail?.name).toBe("Low");
    // The artist underneath is no longer the page showing, so it says nothing.
    expect(h.stack.artistDetail).toBeNull();
  });

  it("won't stack the page already on top on itself", async () => {
    const h = make();
    await h.stack.openArtist("spotify:artist:1");
    await h.stack.openArtist("spotify:artist:1");
    expect(h.stack.depth).toBe(1);
    expect(h.opened).toHaveLength(1);
  });

  it("does re-open a page one level down — that is a step, not a repeat", async () => {
    const h = make();
    await h.stack.openArtist("spotify:artist:1");
    await h.stack.openContext("spotify:album:1");
    await h.stack.openArtist("spotify:artist:1");
    expect(h.stack.depth).toBe(3);
    // Read once, though: the second visit comes off the cache.
    expect(artistMock).toHaveBeenCalledTimes(1);
  });

  it("climbs down one rung at a time, and stops at the bottom", async () => {
    const h = make();
    await h.stack.openArtist("spotify:artist:1");
    await h.stack.openContext("spotify:album:1");
    await h.stack.pop();
    expect(h.stack.top?.kind).toBe("artist");
    await h.stack.pop();
    expect(h.stack.depth).toBe(0);
    await h.stack.pop();
    expect(h.stack.depth).toBe(0);
    expect(h.pops).toHaveLength(2); // the no-op told the host nothing
  });

  it("takes the level back when the read behind it fails", async () => {
    const h = make();
    fail = new Error("nope");
    await h.stack.openArtist("spotify:artist:1");
    expect(h.stack.depth).toBe(0);
    expect(errorToast).toHaveBeenCalled();
  });

  it("lets the kid module word its own failure", async () => {
    const h = make({ artistError: "Couldn't load the artist" });
    fail = new Error("nope");
    await h.stack.openArtist("spotify:artist:1");
    expect(errorToast.mock.calls[0][0]).toBe("Couldn't load the artist");
  });

  describe("what the host is told", () => {
    it("hands over the picture off the row an artist was opened from", async () => {
      const h = make();
      await h.stack.openArtist("spotify:artist:1", { art_url: "a.jpg", round: true });
      expect(h.opened).toEqual([{ art_url: "a.jpg", round: true }]);
    });

    it("says nothing about a record — no query sits under one", async () => {
      const h = make();
      await h.stack.openContext("spotify:album:1");
      expect(h.opened).toEqual([]);
      expect(h.pushes).toHaveLength(1);
    });
  });

  describe("scroll", () => {
    it("puts the results back where they were left", async () => {
      const h = make();
      h.scrollTo(420); // read a way down the results
      await h.stack.openArtist("spotify:artist:1");
      await h.stack.pop();
      expect(h.restored).toEqual([420]);
    });

    it("remembers each level separately, all the way down", async () => {
      const h = make();
      h.scrollTo(100); // the results
      await h.stack.openArtist("spotify:artist:1");
      h.scrollTo(250); // partway down the discography
      await h.stack.openContext("spotify:album:1");
      await h.stack.pop();
      expect(h.position).toBe(250);
      await h.stack.pop();
      expect(h.position).toBe(100);
    });

    it("opens a fresh page at the top rather than at the last one's offset", async () => {
      const h = make();
      h.scrollTo(300);
      await h.stack.openArtist("spotify:artist:1");
      await h.stack.openContext("spotify:album:1");
      await h.stack.pop();
      // Back on the artist, which was never scrolled: its own top.
      expect(h.position).toBe(0);
    });

    it("asks for nothing on a surface whose levels don't scroll", async () => {
      const stack = createCatalogStack({ onPop: () => {} });
      await stack.openArtist("spotify:artist:1");
      await stack.pop();
      expect(stack.depth).toBe(0);
    });
  });
});
