import { describe, it, expect } from "vitest";
import { rowSub, topLine } from "./catalog";
import type { SpotifyItem } from "../types";

/**
 * What a catalog row is allowed to say about itself.
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
