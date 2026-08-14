/**
 * What the focused room has been heard playing.
 *
 * The queue answers "what's next" and, since it keeps what it has already
 * passed, a little of "what was that?" — but only within the queue it is
 * holding. Put a different record on and the old queue is gone, and with it
 * the name of the song someone liked twenty minutes ago. That is what this
 * asks the hub for: a log written from what the speakers reported, so it
 * outlives the queue, the app being closed, and music started from somewhere
 * that isn't HomeHub at all.
 *
 * Loaded on demand — nothing about the player needs it until someone opens
 * the pane — and re-read when the room changes under it, because a log
 * belonging to the kitchen is not an answer about the bedroom.
 */
import { api } from "../api";
import { toasts } from "../stores.svelte";
import type { HeardTrack } from "../types";

export interface HeardLog {
  readonly list: HeardTrack[];
  /** True while the first read for a room is out — the skeleton's cue. */
  readonly loading: boolean;
  /** True when the list is the household's rather than this room's own. */
  readonly household: boolean;
  /** The room key the list belongs to, or null before anything was asked. */
  readonly key: string | null;
  /** Read (or re-read) one room's log. Cheap to call again for the same key. */
  load(roomKey: string | null): Promise<void>;
  /** Forget one room's log, then re-read so the pane shows the emptiness. */
  clear(roomKey: string): Promise<void>;
}

export function createHeardLog(): HeardLog {
  const s = $state({
    list: [] as HeardTrack[],
    loading: false,
    household: false,
    key: null as string | null,
  });

  // Same guard the queue load uses: a room switched twice while the first
  // read is still out must not be overwritten by the answer to a question
  // nobody is asking any more.
  let seq = 0;

  async function load(roomKey: string | null) {
    const mine = ++seq;
    if (!roomKey) {
      s.list = [];
      s.household = false;
      s.key = null;
      s.loading = false;
      return;
    }
    // Only the first read for a room shows a skeleton. A re-read after a
    // clear, or after the pane is reopened, keeps what is on screen until
    // the answer arrives rather than blinking the list away.
    s.loading = s.key !== roomKey;
    s.key = roomKey;
    try {
      const res = await api.mediaHeard(roomKey);
      if (mine !== seq) return;
      s.list = res.tracks ?? [];
      s.household = !!res.household;
    } catch {
      if (mine !== seq) return;
      s.list = [];
      s.household = false;
    } finally {
      if (mine === seq) s.loading = false;
    }
  }

  return {
    get list() {
      return s.list;
    },
    get loading() {
      return s.loading;
    },
    get household() {
      return s.household;
    },
    get key() {
      return s.key;
    },
    load,
    async clear(roomKey: string) {
      try {
        await api.mediaForgetHeard(roomKey);
      } catch (e) {
        toasts.error("Couldn't clear it", (e as Error).message);
        return;
      }
      s.key = null; // force the re-read to count as this room's first
      await load(roomKey);
    },
  };
}
