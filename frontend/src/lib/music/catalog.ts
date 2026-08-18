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
import type { SpotifyItem, SpotifyResults } from "../types";

/** The kinds a search answers with, in the order results are shelved.
 *  Songs lead — playing one is the commonest reason to search at all.
 *
 *  Each carries both names its kind goes by: `id` is the shelf — the key a
 *  result set and the kind chips use — and `kind` is what one item off that
 *  shelf calls itself. They lived as two lists that happened to agree, which
 *  is the arrangement where keying a map by the wrong one of the two renders
 *  a blank rather than failing to compile. */
export const SEARCH_KINDS = [
  { id: "tracks", kind: "track", label: "Songs" },
  { id: "albums", kind: "album", label: "Albums" },
  { id: "playlists", kind: "playlist", label: "Playlists" },
  { id: "artists", kind: "artist", label: "Artists" },
] as const;

/** The shelf a result set keys by. The vocabulary lives here rather than in
 *  the store, so the list of kinds and the order they shelve in are one
 *  fact. */
export type SearchKind = (typeof SEARCH_KINDS)[number]["id"];
/** What a single item off one of those shelves calls itself. */
export type ItemKind = (typeof SEARCH_KINDS)[number]["kind"];

export interface SearchSection {
  id: SearchKind;
  label: string;
  items: SpotifyItem[];
}

/**
 * A result set cut into shelves, in `SEARCH_KINDS` order.
 *
 * Unfiltered, only the kinds that matched get a shelf — an empty "Playlists"
 * heading is a row of screen spent saying nothing. With a chip narrowing the
 * list it is the one shelf, kept even when empty, because *that* emptiness is
 * the answer to the chip and the surface has an empty state to say so.
 *
 * The wall and the kid module shelve results identically and drew it twice;
 * the app's own search screen does not use this, and shouldn't be made to —
 * it pulls songs out ahead of the grids and pages the rest, which is a
 * different layout rather than this one with different CSS.
 */
export function searchSections(
  results: SpotifyResults | null,
  filter: SearchKind | "all",
): SearchSection[] {
  if (!results) return [];
  const all: SearchSection[] = SEARCH_KINDS.map((k) => ({
    id: k.id,
    label: k.label,
    items: results[k.id],
  }));
  if (filter === "all") return all.filter((s) => s.items.length > 0);
  return all.filter((s) => s.id === filter);
}

/** What one item is, named the way a person would name it. */
export const KIND_LABEL: Record<ItemKind, string> = {
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
