import { api } from "../api";
import { toasts } from "../stores.svelte";
import { kefSourceLabel } from "../kef";
import type { KEFStatus, KEFSpeakerView, KEFSource } from "../types";
import type { Busy } from "./busy.svelte";
import { clock } from "./clock.svelte";
import { clampVol, createVolumeThrottle } from "./volume";

/**
 * The KEF bridge, as state.
 *
 * It sits *beside* the Sonos one rather than being folded into it: a KEF
 * speaker is one standalone stereo pair with an input selector, where a Sonos
 * household is zones that group and share a queue (DESIGN.md §15). No groups,
 * no queue, no favorites — so it gets its own poll and its own surfaces
 * instead of being bent into the group model.
 *
 * Effects live in the component that owns this: the factory exposes `refresh`
 * and the view decides when to call it.
 */
export interface KEFBridge {
  /** The last poll, or null before the first one lands. */
  readonly status: KEFStatus | null;
  /** Registered speakers, reachable ones first, then by name. */
  readonly speakers: KEFSpeakerView[];
  readonly reachable: KEFSpeakerView[];
  readonly playing: KEFSpeakerView[];

  refresh(): Promise<void>;
  byId(id: string | null): KEFSpeakerView | null;

  /** The one place "is this KEF speaker playing?" is answered. */
  isPlaying(sp: KEFSpeakerView): boolean;
  nowLine(sp: KEFSpeakerView): string;
  subLine(sp: KEFSpeakerView): string;
  /** How far through the track, 0–1. Sources with no duration get no line. */
  progress(sp: KEFSpeakerView): number;
  /** Where the track has got to, extrapolated the same way the bar is. */
  positionMs(sp: KEFSpeakerView): number;
  /** Volume the slider shows: the live drag if there is one, else the read. */
  shownVolume(sp: KEFSpeakerView): number;

  togglePlay(sp: KEFSpeakerView): Promise<void>;
  skip(sp: KEFSpeakerView, dir: "next" | "previous"): void;
  /** The live value while a finger is on the slider, sent to the speaker on
   *  a short throttle so a drag doesn't flood it with calls. */
  dragVolume(sp: KEFSpeakerView, v: number): void;
  setVolume(sp: KEFSpeakerView, v: number): void;
  toggleMute(sp: KEFSpeakerView): void;
  setSource(sp: KEFSpeakerView, source: KEFSource): void;
}

export function createKEFBridge(busy: Busy): KEFBridge {
  const s = $state({ status: null as KEFStatus | null });
  let seq = 0;

  /**
   * "Transport is optimistic" (DESIGN.md §15) is a rule about the whole
   * module, not about Sonos. Without this a tapped KEF play/pause sat
   * unchanged until the next read landed — up to a poll away — and the card,
   * its icon and the zone chip all disagreed with the finger that just
   * pressed them.
   */
  const playOverride = $state<Record<string, { playing: boolean; at: number }>>({});

  // The volume the user last set locally, and when — keyed by speaker. Both
  // dragging and committing stamp the time: the window is "this value is the
  // user's, not the poll's", and a finger on the slider is exactly that.
  const vol = $state<Record<string, number>>({});
  const volAt: Record<string, number> = {};
  const VOL_HOLD_MS = 4000;

  const dragThrottle = createVolumeThrottle((id, level) => {
    void api.kefSetVolume(id, level).catch(() => {}); // a dropped mid-drag frame self-heals on release or the next poll
  });

  const speakers = $derived.by(() => {
    const list = [...(s.status?.speakers ?? [])];
    list.sort((a, b) => {
      if (a.reachable !== b.reachable) return a.reachable ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    return list;
  });

  function isPlaying(sp: KEFSpeakerView): boolean {
    const ov = playOverride[sp.id];
    return ov ? ov.playing : !!sp.state?.playing;
  }

  const reachable = $derived(speakers.filter((x) => x.reachable));
  const playing = $derived(speakers.filter((x) => isPlaying(x)));

  const run = (key: string, fn: () => Promise<unknown>, errTitle: string) =>
    busy.run(key, fn, errTitle, refresh);

  async function refresh() {
    const mine = ++seq;
    try {
      const st = await api.kefStatus();
      if (mine !== seq) return;
      s.status = st;
      // Retire an optimistic flip once the speaker agrees with it — or after
      // 6s, so a command it quietly ignored can't leave a card claiming to
      // play forever. Same contract as the Sonos poll's.
      const now = Date.now();
      for (const [id, ov] of Object.entries(playOverride)) {
        const sp = st.speakers.find((x) => x.id === id);
        if (!sp || !!sp.state?.playing === ov.playing || now - ov.at > 6000) {
          delete playOverride[id];
        }
      }
    } catch {
      // A home with no KEF speakers must not see an error every poll; an
      // empty list is indistinguishable from a failed one here, and the
      // Speakers screen is where a broken registration shows up.
      if (mine === seq && !s.status) s.status = { speakers: [] };
    }
  }

  return {
    get status() {
      return s.status;
    },
    get speakers() {
      return speakers;
    },
    get reachable() {
      return reachable;
    },
    get playing() {
      return playing;
    },

    refresh,
    byId: (id) => (id ? (speakers.find((x) => x.id === id) ?? null) : null),
    isPlaying,

    nowLine(sp) {
      if (!sp.state?.powered_on) return "In standby";
      const t = sp.state.track;
      if (t?.title) return t.title;
      return sp.state.source ? `${kefSourceLabel(sp.state.source)} input` : "Idle";
    },

    subLine(sp) {
      const t = sp.state?.track;
      return [t?.artist, t?.album].filter(Boolean).join(" · ");
    },

    progress(sp) {
      void clock.beat; // re-derive once a second, exactly as the Sonos one does
      const total = sp.state?.duration_ms ?? 0;
      if (total <= 0) return 0;
      // Extrapolated from when the reading was taken, like the Sonos
      // scrubber, so the line advances between polls instead of stepping.
      // Without the beat above this only recomputed when a poll replaced the
      // object — every 20s once Sonos push is up — so the hairline sat dead
      // beside a Sonos one creeping every second.
      const base = sp.state?.position_ms ?? 0;
      const since = isPlaying(sp) && sp.read_at ? Date.now() - sp.read_at : 0;
      return Math.max(0, Math.min(1, (base + since) / total));
    },

    positionMs(sp) {
      void clock.beat; // re-read every second so the clock counts rather than jumps
      const total = sp.state?.duration_ms ?? 0;
      const base = sp.state?.position_ms ?? 0;
      const since = isPlaying(sp) && sp.read_at ? Date.now() - sp.read_at : 0;
      return total > 0 ? Math.min(total, base + since) : base + since;
    },

    shownVolume(sp) {
      const ov = vol[sp.id];
      const fresh = ov !== undefined && Date.now() - (volAt[sp.id] ?? 0) < VOL_HOLD_MS;
      return fresh ? ov : (sp.state?.volume ?? 0);
    },

    async togglePlay(sp) {
      const next = !isPlaying(sp);
      await busy.claim("kefplay:" + sp.id, async () => {
        playOverride[sp.id] = { playing: next, at: Date.now() };
        try {
          await (next ? api.kefPlay(sp.id) : api.kefPause(sp.id));
          await refresh();
        } catch (e) {
          delete playOverride[sp.id];
          toasts.error(
            next ? "Couldn't start playback" : "Couldn't pause",
            (e as Error).message,
          );
        }
      });
    },

    skip(sp, dir) {
      void run(
        `kef${dir}:` + sp.id,
        () => (dir === "next" ? api.kefNext(sp.id) : api.kefPrevious(sp.id)),
        "Skip failed",
      );
    },

    dragVolume(sp, v) {
      const level = clampVol(v);
      vol[sp.id] = level;
      // Stamped, or `shownVolume` would read the finger's own value as stale
      // and hand the slider back the polled one mid-drag.
      volAt[sp.id] = Date.now();
      dragThrottle.schedule(sp.id, level);
    },

    setVolume(sp, v) {
      const level = clampVol(v);
      dragThrottle.cancel(sp.id);
      vol[sp.id] = level;
      volAt[sp.id] = Date.now();
      void run("kefvol:" + sp.id, () => api.kefSetVolume(sp.id, level), "Volume failed");
    },

    toggleMute(sp) {
      const next = !sp.state?.muted;
      void run("kefmute:" + sp.id, () => api.kefSetMute(sp.id, next), "Mute failed");
    },

    setSource(sp, source) {
      if (sp.state?.source === source) return;
      void run("kefsrc:" + sp.id, () => api.kefSetSource(sp.id, source), "Couldn't switch input");
    },
  };
}
