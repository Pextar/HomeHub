// What the catalog's rows and cards say about an item, in the one place
// that decides it (DESIGN.md §15.9).
//
// The app's search screen and the panel's music depth lay their results
// out very differently — a phone's grid of shelves against a wall's dense
// list — but "what kind of thing is this, and which one stat identifies it
// fastest" is the same question on both, and a top result that reads
// differently depending on which screen you found it on is a bug.
//
// Only the genuinely shared answer lives here, and the split that matters
// is card against row rather than surface against surface: a card under a
// cover has room for two facts and a row in a list has room for four. So
// every list of rows shares `rowSub` below, and a screen that lays its
// results out as cards still writes its own.

import { fmtCount, fmtMs, capFirst } from "./format";
import type { SpotifyItem } from "../types";

/** The kinds a search answers with, in the order results are shelved.
 *  Songs lead — playing one is the commonest reason to search at all. */
export const SEARCH_KINDS = [
  { id: "tracks", label: "Songs" },
  { id: "albums", label: "Albums" },
  { id: "playlists", label: "Playlists" },
  { id: "artists", label: "Artists" },
] as const;

/** What one item is, named the way a person would name it. */
export const KIND_LABEL: Record<string, string> = {
  artist: "Artist",
  album: "Album",
  playlist: "Playlist",
  track: "Song",
};

/**
 * The top result's own line: the kind first, then what identifies it
 * fastest, and nothing beyond that. An artist's genres were here once and
 * pushed the line past a phone's width — so the one stat that sizes a name
 * stays, and the genres wait for the artist page where they have a row of
 * their own.
 */
export function topLine(item: SpotifyItem): string {
  const bits = [KIND_LABEL[item.kind]];
  if (item.kind === "artist") {
    if (item.followers) bits.push(`${fmtCount(item.followers)} followers`);
  } else {
    if (item.sub) bits.push(item.sub);
    if (item.year) bits.push(item.year);
    if (item.album) bits.push(item.album);
    if (item.duration_ms) bits.push(fmtMs(item.duration_ms));
    if (item.total_tracks) bits.push(`${item.total_tracks} songs`);
  }
  return bits.filter(Boolean).join(" · ");
}

/**
 * What a row says under its name — different per kind, because what makes
 * each one worth choosing is different. An artist is sized by its
 * following (or named by its genre, where the following is unknown); a
 * record says who made it, when, and how long it is; a song says its
 * artist and the album it came off.
 *
 * The wall's dense list and the kid module's big one draw the same row and
 * so read the same line. A card's shorter subtitle is its own — see the
 * note at the top of this file.
 */
export function rowSub(item: SpotifyItem): string {
  if (item.kind === "artist") {
    if (item.followers) return `${fmtCount(item.followers)} followers`;
    return item.genres?.[0] ? capFirst(item.genres[0]) : "";
  }
  if (item.kind === "album") {
    return [item.sub, item.year, item.total_tracks ? `${item.total_tracks} songs` : ""]
      .filter(Boolean)
      .join(" · ");
  }
  if (item.kind === "playlist") {
    return [item.sub, item.total_tracks ? `${item.total_tracks} songs` : ""]
      .filter(Boolean)
      .join(" · ");
  }
  return [item.sub, item.album].filter(Boolean).join(" · ");
}
