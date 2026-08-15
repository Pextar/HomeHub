import { api } from "../api";
import { toasts } from "../stores.svelte";
import type { UPnPRenderer } from "../types";

/**
 * The UPnP renderer bridge, as state.
 *
 * The smallest of the four, and for a reason worth stating rather than
 * apologising for: a renderer has almost nothing of its own to manage. It holds
 * no account, no queue and no source list, and what it is playing is whatever
 * HomeHub pointed it at — which the room's own player already shows. So this is
 * an inventory and nothing else.
 *
 * What it is *for* is the thing that doesn't fit in a list: a renderer fetches
 * the stream rather than being pushed it, so it reads the WAV header and plays
 * whatever rate and word length arrives. It is the only endpoint here that can
 * take 24-bit/192 kHz, and the same physical box reached over AirPlay instead
 * would be capped at CD quality by the protocol. That is why a renderer row is
 * worth showing even though there is nothing on it to adjust.
 */
export interface UPnPBridge {
  /** Registered renderers, by name. Empty before the first read lands. */
  readonly renderers: UPnPRenderer[];
  /** True once a read has answered, so "none" can be told from "not yet". */
  readonly loaded: boolean;

  refresh(): Promise<void>;
  byId(id: string | null): UPnPRenderer | null;
  /** Re-read a renderer's device description after it moved ports. */
  rediscover(rn: UPnPRenderer): Promise<void>;
}

export function createUPnPBridge(): UPnPBridge {
  const s = $state({
    renderers: [] as UPnPRenderer[],
    loaded: false,
  });
  let seq = 0;

  async function refresh() {
    const mine = ++seq;
    try {
      const list = await api.upnpRenderers();
      // A read that landed after a newer one started is stale and dropped —
      // the same guard the other bridges use.
      if (mine !== seq) return;
      s.renderers = list;
    } catch {
      // Silent: the speakers screen shows an empty section rather than an
      // error toast for a list that simply hasn't loaded. A renderer that
      // genuinely fails is found when something is played to it.
    } finally {
      if (mine === seq) s.loaded = true;
    }
  }

  return {
    get renderers() {
      return s.renderers;
    },
    get loaded() {
      return s.loaded;
    },
    refresh,
    byId(id) {
      return s.renderers.find((r) => r.id === id) ?? null;
    },
    async rediscover(rn) {
      try {
        await api.upnpRefreshRenderer(rn.id);
        await refresh();
      } catch (e) {
        toasts.error("Couldn't re-read " + rn.name, (e as Error).message);
      }
    },
  };
}
