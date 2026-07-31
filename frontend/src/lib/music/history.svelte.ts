/**
 * Recent searches, keyed by the room they were played on.
 *
 * "Recent searches" reads differently in the kitchen than in the bedroom, so
 * the history is scoped to the destination. A single-room home only ever has
 * one key, which collapses this to a plain unscoped history without any extra
 * code path — which is why it is one keyed map rather than a scoped mode
 * bolted onto a flat list.
 */

const STORAGE_KEY = "music.searchHistory.v1";
const MAX = 8;
/** Used before a destination exists — no speakers yet, or none reachable. */
const FALLBACK_KEY = "_all";

export interface SearchHistoryEntry {
  q: string;
  /** The query's own top result's picture, once the search that named it has
   *  come back — absent right when a search is first run, and absent for a
   *  query whose top result carries no art. */
  art_url?: string;
  /** True when `art_url` is an artist's circular photo rather than square
   *  cover art — so a chip can round it the way the rest of the module does. */
  round?: boolean;
}

export interface SearchHistory {
  /** Everything remembered for the current destination, newest first. */
  readonly list: SearchHistoryEntry[];
  /** The same list cut to a row, for the player's "Start something". */
  readonly recent: SearchHistoryEntry[];
  /** `art` fills in (or updates) the picture once the search behind `q`
   *  answers — omit it to just record the query. */
  add(q: string, art?: { art_url?: string; round?: boolean }): void;
  remove(q: string): void;
  /** Forget this destination's searches. Other rooms keep theirs. */
  clear(): void;
}

/** Storage predates the picture field, so an older entry is a bare string —
 *  read it back as a query with no art rather than dropping it. */
function normalize(raw: unknown): SearchHistoryEntry[] {
  if (!Array.isArray(raw)) return [];
  const out: SearchHistoryEntry[] = [];
  for (const e of raw) {
    if (typeof e === "string") out.push({ q: e });
    else if (e && typeof e === "object" && typeof (e as { q?: unknown }).q === "string") {
      out.push(e as SearchHistoryEntry);
    }
  }
  return out;
}

function read(): Record<string, SearchHistoryEntry[]> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const out: Record<string, SearchHistoryEntry[]> = {};
    for (const [k, v] of Object.entries(parsed)) out[k] = normalize(v);
    return out;
  } catch {
    return {};
  }
}

function write(all: Record<string, SearchHistoryEntry[]>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(all));
  } catch {
    /* private mode */
  }
}

/**
 * `keyOf` is a getter, not a value: the destination changes under this while
 * the sheet is open, and the list has to follow it.
 */
export function createSearchHistory(keyOf: () => string | null): SearchHistory {
  const s = $state({ all: read() as Record<string, SearchHistoryEntry[]> });

  // Computed per read rather than `$derived`: the key comes from outside, so a
  // cached derivation would only invalidate on `all` changing and would keep
  // serving the previous room's list after the destination moved.
  const key = () => keyOf() ?? FALLBACK_KEY;
  const listFor = () => s.all[key()] ?? [];

  // Persisted on mutation rather than from an effect, so this module needs no
  // effect root and can be created outside a component if it ever has to be.
  function commit(next: Record<string, SearchHistoryEntry[]>) {
    s.all = next;
    write(next);
  }

  return {
    get list() {
      return listFor();
    },
    get recent() {
      return listFor().slice(0, 6);
    },
    add(q, art) {
      // Case-insensitive de-dupe, so re-running a search moves it to the top
      // rather than listing it twice in two capitalisations. A second call
      // for the same query — the art arriving after the search that named it
      // returned — updates the existing entry in place instead of adding one.
      const cur = listFor();
      const prior = cur.find((x) => x.q.toLowerCase() === q.toLowerCase());
      const rest = cur.filter((x) => x.q.toLowerCase() !== q.toLowerCase());
      const entry: SearchHistoryEntry = {
        q,
        art_url: art?.art_url ?? prior?.art_url,
        round: art?.round ?? prior?.round,
      };
      commit({ ...s.all, [key()]: [entry, ...rest].slice(0, MAX) });
    },
    remove(q) {
      commit({ ...s.all, [key()]: listFor().filter((x) => x.q !== q) });
    },
    clear() {
      const next = { ...s.all };
      delete next[key()];
      commit(next);
    },
  };
}
