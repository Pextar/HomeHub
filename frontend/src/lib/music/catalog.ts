// What the catalog's rows and cards say about an item, in the one place
// that decides it (DESIGN.md §15.9).
//
// The app's search screen and the panel's music depth lay their results
// out very differently — a phone's grid of shelves against a wall's dense
// list — but "what kind of thing is this, and which one stat identifies it
// fastest" is the same question on both, and a top result that reads
// differently depending on which screen you found it on is a bug.
//
// Only the genuinely shared answer lives here. The per-row subtitle is
// *not* shared: a card under a cover has room for two facts and a row in a
// list has room for four, so each surface still writes its own.

import { fmtCount, fmtMs } from "./format";
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
