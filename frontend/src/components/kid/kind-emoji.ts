/**
 * The kid module's emoji for each kind of thing in the catalog.
 *
 * DESIGN.md §2 keeps emoji inside the kid module, and this file is inside it:
 * the catalog's own shared rules (`lib/music/catalog.ts`) stay plain, and the
 * surfaces in here pick one up on the way past. Two components draw a kind
 * now — the search results and the catalog pages behind them — so the map is
 * a module rather than a copy in each.
 *
 * Keyed the way an item names its own kind. `SEARCH_KINDS` carries the
 * pairing to a shelf's plural id, so a shelf label reaches this through that
 * rather than keeping a second map of its own.
 */

import type { ItemKind } from "../../lib/music/catalog";

export const KIND_EMOJI: Record<ItemKind, string> = {
  track: "🎵",
  album: "💿",
  playlist: "📃",
  artist: "🎤",
};
