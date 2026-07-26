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

export interface SearchHistory {
  /** Everything remembered for the current destination, newest first. */
  readonly list: string[];
  /** The same list cut to a row, for the player's "Start something". */
  readonly recent: string[];
  add(q: string): void;
  remove(q: string): void;
  /** Forget this destination's searches. Other rooms keep theirs. */
  clear(): void;
}

function read(): Record<string, string[]> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function write(all: Record<string, string[]>) {
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
  const s = $state({ all: read() as Record<string, string[]> });

  // Computed per read rather than `$derived`: the key comes from outside, so a
  // cached derivation would only invalidate on `all` changing and would keep
  // serving the previous room's list after the destination moved.
  const key = () => keyOf() ?? FALLBACK_KEY;
  const listFor = () => s.all[key()] ?? [];

  // Persisted on mutation rather than from an effect, so this module needs no
  // effect root and can be created outside a component if it ever has to be.
  function commit(next: Record<string, string[]>) {
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
    add(q) {
      // Case-insensitive de-dupe, so re-running a search moves it to the top
      // rather than listing it twice in two capitalisations.
      const rest = (s.all[key()] ?? []).filter((x) => x.toLowerCase() !== q.toLowerCase());
      commit({ ...s.all, [key()]: [q, ...rest].slice(0, MAX) });
    },
    remove(q) {
      commit({ ...s.all, [key()]: (s.all[key()] ?? []).filter((x) => x !== q) });
    },
    clear() {
      const next = { ...s.all };
      delete next[key()];
      commit(next);
    },
  };
}
