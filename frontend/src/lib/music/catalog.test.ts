import { describe, it, expect } from "vitest";
import { rowSub, topLine, searchSections, SEARCH_KINDS, KIND_LABEL } from "./catalog";
import type { SpotifyItem, SpotifyResults } from "../types";

/**
 * What a catalog row is allowed to say about itself, and how a result set is
 * cut into shelves.
 *
 * Every list of rows in the app draws this line — the wall's dense list and
 * the kid module's big one — so these are rules about the vocabulary, not
 * about one screen's layout: which fact identifies each kind fastest, and
 * what a row says when the fact behind it is missing.
 */

const item = (over: Partial<SpotifyItem>): SpotifyItem =>
  ({ kind: "track", uri: "spotify:track:1", name: "Sound and Vision", ...over }) as SpotifyItem;

describe("rowSub", () => {
  it("sizes an artist by its following", () => {
    expect(rowSub(item({ kind: "artist", followers: 1_500_000 }))).toBe("1.5M followers");
  });

  it("falls back to an artist's genre where the following is unknown", () => {
    expect(rowSub(item({ kind: "artist", genres: ["art rock", "glam"] }))).toBe("Art rock");
  });

  it("says nothing about an artist it knows nothing about", () => {
    expect(rowSub(item({ kind: "artist" }))).toBe("");
  });

  it("gives an album its maker, its year and its length", () => {
    const sub = rowSub(item({ kind: "album", sub: "David Bowie", year: "1977", total_tracks: 11 }));
    expect(sub).toBe("David Bowie · 1977 · 11 songs");
  });

  it("gives a playlist its owner and its length, and never a year", () => {
    expect(rowSub(item({ kind: "playlist", sub: "Spotify", total_tracks: 50 }))).toBe(
      "Spotify · 50 songs",
    );
  });

  it("gives a song its artist and the record it came off", () => {
    expect(rowSub(item({ sub: "David Bowie", album: "Low" }))).toBe("David Bowie · Low");
  });

  it("drops the separator rather than leaving a line starting with one", () => {
    expect(rowSub(item({ sub: "David Bowie" }))).toBe("David Bowie");
    expect(rowSub(item({ album: "Low" }))).toBe("Low");
    expect(rowSub(item({}))).toBe("");
  });
});

describe("topLine", () => {
  it("leads with what kind of thing it is", () => {
    expect(topLine(item({ kind: "album", sub: "David Bowie" }))).toBe("Album · David Bowie");
  });

  it("sizes an artist by its following and stops there — genres get a page", () => {
    expect(topLine(item({ kind: "artist", followers: 1_500_000, genres: ["art rock"] }))).toBe(
      "Artist · 1.5M followers",
    );
  });
});

describe("searchSections", () => {
  const results = (over: Partial<SpotifyResults> = {}): SpotifyResults => ({
    tracks: [],
    albums: [],
    playlists: [],
    artists: [],
    ...over,
  });
  const one = (kind: SpotifyItem["kind"], name: string) => item({ kind, name, uri: `x:${name}` });

  it("has nothing to shelve before a search has answered", () => {
    expect(searchSections(null, "all")).toEqual([]);
  });

  it("shelves songs first — the commonest reason to search at all", () => {
    const all = searchSections(
      results({
        tracks: [one("track", "Heroes")],
        albums: [one("album", "Low")],
        playlists: [one("playlist", "Mix")],
        artists: [one("artist", "Bowie")],
      }),
      "all",
    );
    expect(all.map((s) => s.id)).toEqual(["tracks", "albums", "playlists", "artists"]);
    expect(all.map((s) => s.label)).toEqual(["Songs", "Albums", "Playlists", "Artists"]);
  });

  it("gives no shelf to a kind that didn't match", () => {
    const all = searchSections(results({ tracks: [one("track", "Heroes")] }), "all");
    expect(all.map((s) => s.id)).toEqual(["tracks"]);
  });

  it("narrows to the one shelf a chip names", () => {
    const only = searchSections(
      results({ tracks: [one("track", "Heroes")], albums: [one("album", "Low")] }),
      "albums",
    );
    expect(only.map((s) => s.id)).toEqual(["albums"]);
    expect(only[0].items).toHaveLength(1);
  });

  it("keeps the named shelf when it is empty — that emptiness answers the chip", () => {
    const only = searchSections(results({ tracks: [one("track", "Heroes")] }), "playlists");
    expect(only.map((s) => s.id)).toEqual(["playlists"]);
    expect(only[0].items).toEqual([]);
  });
});

describe("the kind vocabulary", () => {
  it("pairs each shelf with the name one item off it goes by", () => {
    expect(SEARCH_KINDS.map((k) => [k.id, k.kind])).toEqual([
      ["tracks", "track"],
      ["albums", "album"],
      ["playlists", "playlist"],
      ["artists", "artist"],
    ]);
  });

  it("names every kind it shelves — a surface reading this map never blanks", () => {
    for (const k of SEARCH_KINDS) expect(KIND_LABEL[k.kind]).toBeTruthy();
  });
});
