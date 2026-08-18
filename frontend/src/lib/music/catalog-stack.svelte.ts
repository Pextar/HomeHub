/**
 * The ladder into the catalog: an artist's page over the results, a record's
 * listing over that, and back climbing one rung at a time.
 *
 * `catalog-cache.svelte.ts` shared the *reading* of those pages between the
 * three surfaces that drill into the catalog and left the stack with each
 * host, on the grounds that only a host knows what a level looks like and
 * where it was scrolled. That was true of the app's Music view, whose stack
 * carries its whole screen router — home, browse, speakers, the catalog
 * pages — with per-screen teardown. It was not true of the other two. The
 * wall's depth and the kid module's search both had a stack of exactly this
 * shape, and both wrote the same four functions around it:
 *
 *   - push a level, and never re-push the one already on top
 *   - wire the cache's two URI getters to whichever kind is on top
 *   - tell the host a page was opened from a row, so it can remember the
 *     query behind it and let go of the software keyboard
 *   - take the level back when the read behind it fails
 *
 * All four are rules about what the ladder means, not about how a rung is
 * drawn, so they live here now. What genuinely differed stayed behind as
 * hooks: the wall stashes and restores scroll and folds its search layout
 * away on a push, and the kid module answers a rung with a haptic.
 *
 * Scroll is kept here rather than by the host because the host would have to
 * keep it keyed by depth anyway, and index 0 — where the results themselves
 * were scrolled to before any of this opened — is the entry the host is
 * likeliest to forget it owns.
 */

import { tick as flushDOM } from "svelte";
import { createCatalogCache, type CatalogCache } from "./catalog-cache.svelte";

/** The picture to file a remembered query under: a round crop for an
 *  artist, square for anything with a cover. */
export type ItemArt = { art_url?: string; round?: boolean };

export type CatalogLevel = { kind: "artist" | "context"; uri: string };

export interface CatalogStackOptions {
  /**
   * An artist's page was opened from a search row — the one way into this
   * stack that starts from a result. The host files the query behind that
   * row (the search store only remembers submissions, and both these
   * surfaces are typed at and then tapped, with no Enter in between) and
   * lets go of the keyboard, since the tap that got here was the choosing.
   *
   * Not fired by `openContext`: a record is only ever opened from a page
   * that is already up, so there is no query underneath it to file.
   */
  onOpened?: (art?: ItemArt) => void;
  /** Fired before a level goes on: the host lets go of the keyboard, and
   *  stands down anything the page is about to cover. */
  onPush?: () => void;
  /** Fired as a level comes off, before the DOM has caught up. */
  onPop?: () => void;
  /** Where the level being left is scrolled to. Omit on a surface whose
   *  levels don't scroll independently. */
  scrollOf?: () => number;
  /** Put the revealed level back where it was. Called after the DOM has
   *  caught up, so `depth` already reads as the level being restored. */
  scrollTo?: (y: number) => void;
  /** Toast title when an artist page won't load — the kid module words its
   *  own. */
  artistError?: string;
}

export interface CatalogStack extends CatalogCache {
  /** The level on top, or null when the results themselves are showing. */
  readonly top: CatalogLevel | null;
  /** 0 while nothing is open — the number of rungs climbed. */
  readonly depth: number;
  /**
   * Open an artist's page. A no-op when it is already the page on top, so a
   * double tap doesn't stack a level on itself. `art` is the picture off the
   * row tapped, filed with the query it answered.
   */
  openArtist(uri: string, art?: ItemArt): Promise<void>;
  /** Open an album or playlist's listing. Same guard. */
  openContext(uri: string): Promise<void>;
  /** Climb down one rung. A no-op at the bottom. */
  pop(): Promise<void>;
}

export function createCatalogStack(opts: CatalogStackOptions = {}): CatalogStack {
  let levels = $state<CatalogLevel[]>([]);
  // Indexed by depth: `scrolls[0]` is where the host's own list was left
  // before the first page went on, `scrolls[1]` the level above it, and so
  // on. Only ever read back by a pop, so a stale tail costs nothing.
  const scrolls: number[] = [];

  const top = () => (levels.length ? levels[levels.length - 1] : null);

  function push(kind: CatalogLevel["kind"], uri: string) {
    scrolls[levels.length] = opts.scrollOf?.() ?? 0;
    opts.onPush?.();
    levels = [...levels, { kind, uri }];
  }

  async function pop() {
    if (!levels.length) return;
    opts.onPop?.();
    levels = levels.slice(0, -1);
    await flushDOM();
    opts.scrollTo?.(scrolls[levels.length] ?? 0);
  }

  const cache = createCatalogCache({
    artistUri: () => {
      const t = top();
      return t?.kind === "artist" ? t.uri : null;
    },
    contextUri: () => {
      const t = top();
      return t?.kind === "context" ? t.uri : null;
    },
    // The read behind the level we just pushed failed, so the level goes
    // back — the cache has already checked it is still the one on top.
    onFail: () => void pop(),
    artistError: opts.artistError,
  });

  return {
    get top() {
      return top();
    },
    get depth() {
      return levels.length;
    },
    get artistDetail() {
      return cache.artistDetail;
    },
    get artistLoading() {
      return cache.artistLoading;
    },
    get contextDetail() {
      return cache.contextDetail;
    },
    get contextLoading() {
      return cache.contextLoading;
    },
    loadArtist: (uri) => cache.loadArtist(uri),
    loadContext: (uri) => cache.loadContext(uri),

    async openArtist(uri, art) {
      const t = top();
      if (t?.kind === "artist" && t.uri === uri) return;
      opts.onOpened?.(art);
      push("artist", uri);
      await cache.loadArtist(uri);
    },

    async openContext(uri) {
      const t = top();
      if (t?.kind === "context" && t.uri === uri) return;
      push("context", uri);
      await cache.loadContext(uri);
    },

    pop,
  };
}
