import { api } from "../api";
import { toasts } from "../stores.svelte";
import type { AirPlaySpeakerView } from "../types";
import { clampVol, createVolumeThrottle } from "./volume";

/**
 * The AirPlay bridge, as state.
 *
 * Much smaller than the Sonos and KEF bridges, and the smallness is the
 * protocol rather than an omission. Those two poll a device that knows what it
 * is playing. A RAOP receiver knows nothing: it is a sink, and everything
 * about the audio — what it is, where it got to, whether it is paused — lives
 * in HomeHub, which is the thing sending it. So there is no now-playing to
 * read here and no transport to drive; a receiver's playback is its zone's
 * playback, and the Player already reads that.
 *
 * What is left is the inventory and the one control that is genuinely the
 * receiver's own: its volume. Even that is stored as much as sent — a receiver
 * only accepts a level inside a session, so with nothing casting the level is
 * remembered for the next one rather than dropped.
 */
export interface AirPlayBridge {
  /** Registered receivers, by name. Empty before the first read lands. */
  readonly receivers: AirPlaySpeakerView[];
  /** True once a read has answered, so "none" can be told from "not yet". */
  readonly loaded: boolean;
  /** The receivers HomeHub is sending to right now. */
  readonly casting: AirPlaySpeakerView[];

  refresh(): Promise<void>;
  byId(id: string | null): AirPlaySpeakerView | null;
  /** Volume the slider shows: the live drag if there is one, else the read. */
  shownVolume(sp: AirPlaySpeakerView): number;
  dragVolume(sp: AirPlaySpeakerView, v: number): void;
  setVolume(sp: AirPlaySpeakerView, v: number): void;
}

/** How long a just-dragged volume is trusted over what the server reports, so
 *  a read landing mid-drag doesn't yank the slider back. Same rule as the
 *  other two bridges. */
const VOL_HOLD_MS = 2000;

export function createAirPlayBridge(): AirPlayBridge {
  const s = $state({
    receivers: [] as AirPlaySpeakerView[],
    loaded: false,
  });
  let seq = 0;

  // Local volume overrides while a finger is on a slider.
  const local = $state<Record<string, number>>({});
  const touched: Record<string, number> = {};

  const throttle = createVolumeThrottle((id, level) => {
    void api.airplaySetVolume(id, level).catch((e: unknown) => {
      toasts.error("Volume failed", (e as Error).message);
    });
  });

  async function refresh() {
    const mine = ++seq;
    try {
      const list = await api.airplayStatus();
      // A slower read landing after a newer one would show older state.
      if (mine !== seq) return;
      s.receivers = list;
      s.loaded = true;
    } catch {
      // Quiet: the receiver list is a background read on a screen that has
      // other things on it, and a toast per failed poll is noise. The list
      // simply keeps showing what it last knew.
      if (mine === seq) s.loaded = true;
    }
  }

  return {
    get receivers() {
      return s.receivers;
    },
    get loaded() {
      return s.loaded;
    },
    get casting() {
      return s.receivers.filter((r) => r.casting);
    },
    refresh,
    byId(id) {
      return id ? (s.receivers.find((r) => r.id === id) ?? null) : null;
    },
    shownVolume(sp) {
      const held = touched[sp.id] && Date.now() - touched[sp.id] < VOL_HOLD_MS;
      return held ? (local[sp.id] ?? sp.volume ?? 0) : (sp.volume ?? 0);
    },
    dragVolume(sp, v) {
      const level = clampVol(v);
      local[sp.id] = level;
      touched[sp.id] = Date.now();
      throttle.schedule(sp.id, level);
    },
    setVolume(sp, v) {
      const level = clampVol(v);
      local[sp.id] = level;
      touched[sp.id] = Date.now();
      // The release of a drag: cancel whatever the throttle still holds and
      // send the final value, so the level that sticks is the one let go of.
      throttle.cancel(sp.id);
      void api
        .airplaySetVolume(sp.id, level)
        .then(refresh)
        .catch((e: unknown) => toasts.error("Volume failed", (e as Error).message));
    },
  };
}
