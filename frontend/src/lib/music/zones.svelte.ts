import { api } from "../api";
import { toasts } from "../stores.svelte";
import type {
  MediaEndpoint,
  MediaPlayResult,
  MediaZone,
  MediaZoneSpeaker,
  MediaVendor,
} from "../types";
import type { Busy } from "./busy.svelte";
import { clock } from "./clock.svelte";
import { clampVol } from "./volume";

/**
 * Zones, as state: the third bridge, and the only one that isn't a bridge.
 *
 * The Sonos and KEF modules speak to a make of speaker. This one speaks to
 * `/api/media` — the vendor-neutral layer — and its noun is the zone: "these
 * speakers, playing this, together", of any mix of makes
 * (docs/MEDIA-PROTOCOL.md). Which speakers belong together is the user's
 * arrangement and is persisted; *how* they are driven is the route engine's
 * decision per playback, and this module never guesses at it. Every zone read
 * carries the backend's own `route`, `sync` and `reason`, and the UI shows
 * those rather than inferring a route from the makes it can see — inference
 * here is exactly how a Sonos-only zone would end up wearing stream
 * affordances it never takes (DESIGN.md §15).
 *
 * Effects live in the component that owns this, like the other two: the
 * factory exposes `refresh` and the view decides when to call it.
 */

/** Which routes hold the Spotify account's single active session. */
const SESSION_ROUTES = new Set(["stream", "connect"]);

export interface ZonesBridge {
  /** False until the first read answers, however it answered. */
  readonly loaded: boolean;
  /** Every zone, by name — the order the backend sorted them into. */
  readonly zones: MediaZone[];
  /** Zones with at least one speaker actually playing. */
  readonly playing: MediaZone[];
  /** Every speaker the media layer knows, for the membership picker. */
  readonly endpoints: MediaEndpoint[];

  refresh(): Promise<void>;
  /** Endpoints alone — the picker needs them before any zone exists. */
  loadEndpoints(): Promise<void>;
  byId(id: string | null): MediaZone | null;

  /** Members that resolved to a live speaker, in the user's order. */
  speakersOf(z: MediaZone): MediaZoneSpeaker[];
  /** The makes in this zone, deduped, in member order. */
  vendorsOf(z: MediaZone): MediaVendor[];
  /** More than one make in one zone — the case the vendors refuse. */
  isMixed(z: MediaZone): boolean;
  /** The one place "is this zone playing?" is answered. */
  isPlaying(z: MediaZone): boolean;
  /** Every speaker muted. Anything less reads as unmuted, since one
   *  audible speaker means the zone is audible. */
  isMuted(z: MediaZone): boolean;
  /** The speaker whose reading stands for the zone — the first with a track. */
  leadOf(z: MediaZone): MediaZoneSpeaker | undefined;
  nowLine(z: MediaZone): string;
  subLine(z: MediaZone): string;
  /** Members named for a card's subline: "Living Room + Kitchen". */
  memberLine(z: MediaZone): string;
  /** 0–1, or 0 when the source reports no duration. */
  progress(z: MediaZone): number;
  positionMs(z: MediaZone): number;
  durationMs(z: MediaZone): number;
  /** Volume the zone fader shows: the live drag if there is one, else the
   *  mean of the members', which is what a zone-wide set writes to them. */
  shownVolume(z: MediaZone): number;

  /**
   * A zone that is playing on the account's single Spotify session and would
   * be stopped by starting this one. Null when there is no conflict — which
   * includes every all-Sonos zone, since those play from the household's own
   * account link and hold nothing.
   */
  wouldInterrupt(z: MediaZone): MediaZone | null;

  /** Sonos speaker ids inside a playing zone — what Home's dedupe reads. */
  readonly playingSonosIds: Set<string>;
  /** KEF speaker ids inside a playing zone. */
  readonly playingKefIds: Set<string>;
  /** Every zone a speaker belongs to, by qualified member id. */
  zonesWith(member: string): MediaZone[];

  togglePlay(z: MediaZone): Promise<void>;
  skip(z: MediaZone, dir: "next" | "previous"): void;
  stop(z: MediaZone): Promise<void>;
  dragVolume(z: MediaZone, v: number): void;
  setVolume(z: MediaZone, v: number): void;
  toggleMute(z: MediaZone): void;

  /** Start content on a zone. Answers with the route it took, so the caller
   *  can say what actually happened rather than promising one thing. */
  play(z: MediaZone, item: { uri: string; title?: string; kind?: string; provider?: string }):
    Promise<MediaPlayResult>;

  create(body: { name: string; members: string[]; room?: string }): Promise<MediaZone | null>;
  update(id: string, body: { name?: string; members?: string[]; room?: string }):
    Promise<MediaZone | null>;
  remove(id: string): Promise<boolean>;
}

export function createZonesBridge(busy: Busy): ZonesBridge {
  const s = $state({
    zones: [] as MediaZone[],
    endpoints: [] as MediaEndpoint[],
    loaded: false,
  });
  let seq = 0;

  /**
   * "Transport is optimistic" is a rule about the module, not about one bridge
   * (DESIGN.md §15). A zone tap has further to travel than either vendor's —
   * it can fan out to speakers of two makes — so it is the one that most
   * needs the flip to land before the call does.
   */
  const playOverride = $state<Record<string, { playing: boolean; at: number }>>({});

  // The zone fader's own value while a finger is on it, and when. Same
  // contract as the vendor bridges': for this window the value is the user's,
  // not the poll's.
  const vol = $state<Record<string, number>>({});
  const volAt: Record<string, number> = {};
  const VOL_HOLD_MS = 4000;

  function speakersOf(z: MediaZone): MediaZoneSpeaker[] {
    return z.speakers.filter((sp) => !sp.missing);
  }

  function isPlaying(z: MediaZone): boolean {
    const ov = playOverride[z.id];
    if (ov) return ov.playing;
    return z.speakers.some((sp) => sp.state?.state === "playing");
  }

  function leadOf(z: MediaZone): MediaZoneSpeaker | undefined {
    const live = speakersOf(z);
    return (
      live.find((sp) => sp.state?.state === "playing" && sp.state.track?.title) ??
      live.find((sp) => sp.state?.track?.title) ??
      live.find((sp) => sp.state) ??
      live[0]
    );
  }

  /** When a state reading was taken, as a timestamp. */
  function readAt(sp: MediaZoneSpeaker | undefined): number {
    const at = sp?.state?.at;
    if (!at) return 0;
    const t = Date.parse(at);
    return Number.isNaN(t) ? 0 : t;
  }

  const zones = $derived(s.zones);
  const playing = $derived(zones.filter((z) => isPlaying(z)));
  const endpoints = $derived(s.endpoints);

  /** Speaker ids covered by a playing zone, split by bridge. */
  const playingIds = $derived.by(() => {
    // Plain sets, deliberately: these are rebuilt by the derivation on every
    // change and never mutated afterwards, so there is nothing for a reactive
    // set to observe.
    /* eslint-disable svelte/prefer-svelte-reactivity */
    const sonos = new Set<string>();
    const kef = new Set<string>();
    /* eslint-enable svelte/prefer-svelte-reactivity */
    for (const z of playing) {
      for (const sp of speakersOf(z)) {
        (sp.vendor === "kef" ? kef : sonos).add(sp.id);
      }
    }
    return { sonos, kef };
  });

  const run = (key: string, fn: () => Promise<unknown>, errTitle: string) =>
    busy.run(key, fn, errTitle, refresh);

  async function refresh() {
    const mine = ++seq;
    try {
      const list = await api.mediaZones();
      if (mine !== seq) return;
      s.zones = list;
      // Retire an optimistic flip once the speakers agree with it — or after
      // 6s, so a command a speaker quietly ignored can't leave a card
      // claiming to play forever. Same contract as both vendor polls'.
      const now = Date.now();
      for (const [id, ov] of Object.entries(playOverride)) {
        const z = list.find((x) => x.id === id);
        const live = !!z?.speakers.some((sp) => sp.state?.state === "playing");
        if (!z || live === ov.playing || now - ov.at > 6000) delete playOverride[id];
      }
    } catch {
      // A house with no zones is indistinguishable from a failed read here,
      // and an error toast every poll would be the wrong way to say either.
      if (mine === seq && !s.loaded) s.zones = [];
    } finally {
      if (mine === seq) s.loaded = true;
    }
  }

  async function loadEndpoints() {
    try {
      s.endpoints = await api.mediaEndpoints();
    } catch {
      s.endpoints = [];
    }
  }

  return {
    get loaded() {
      return s.loaded;
    },
    get zones() {
      return zones;
    },
    get playing() {
      return playing;
    },
    get endpoints() {
      return endpoints;
    },
    get playingSonosIds() {
      return playingIds.sonos;
    },
    get playingKefIds() {
      return playingIds.kef;
    },

    refresh,
    loadEndpoints,
    byId: (id) => (id ? (zones.find((z) => z.id === id) ?? null) : null),

    speakersOf,
    isPlaying,
    leadOf,

    vendorsOf(z) {
      const out: MediaVendor[] = [];
      for (const sp of speakersOf(z)) if (!out.includes(sp.vendor)) out.push(sp.vendor);
      return out;
    },

    isMixed(z) {
      const live = speakersOf(z);
      return live.length > 1 && live.some((sp) => sp.vendor !== live[0].vendor);
    },

    isMuted(z) {
      const live = speakersOf(z).filter((sp) => sp.state);
      return live.length > 0 && live.every((sp) => sp.state?.muted);
    },

    nowLine(z) {
      const t = leadOf(z)?.state?.track;
      if (t?.title) return t.title;
      if (speakersOf(z).length === 0) return "No speakers yet";
      return isPlaying(z) ? "Live audio" : "Nothing playing";
    },

    subLine(z) {
      const t = leadOf(z)?.state?.track;
      return [t?.artist, t?.album].filter(Boolean).join(" · ");
    },

    memberLine(z) {
      const live = speakersOf(z);
      if (live.length === 0) return "No speakers yet";
      return live.map((sp) => sp.name).join(" + ");
    },

    progress(z) {
      void clock.beat; // re-derive once a second, like both vendor bridges
      const lead = leadOf(z);
      const total = lead?.state?.duration_ms ?? 0;
      if (total <= 0) return 0;
      const base = lead?.state?.position_ms ?? 0;
      const at = readAt(lead);
      const since = isPlaying(z) && at ? Date.now() - at : 0;
      return Math.max(0, Math.min(1, (base + since) / total));
    },

    positionMs(z) {
      void clock.beat;
      const lead = leadOf(z);
      const total = lead?.state?.duration_ms ?? 0;
      const base = lead?.state?.position_ms ?? 0;
      const at = readAt(lead);
      const since = isPlaying(z) && at ? Date.now() - at : 0;
      return total > 0 ? Math.min(total, base + since) : base + since;
    },

    durationMs(z) {
      return leadOf(z)?.state?.duration_ms ?? 0;
    },

    shownVolume(z) {
      const ov = vol[z.id];
      if (ov !== undefined && Date.now() - (volAt[z.id] ?? 0) < VOL_HOLD_MS) return ov;
      const live = speakersOf(z).filter((sp) => sp.state);
      if (live.length === 0) return 0;
      // The mean, because a zone-wide set writes one level to every speaker:
      // showing the loudest would jump the fader the moment it was touched.
      const sum = live.reduce((n, sp) => n + (sp.state?.volume ?? 0), 0);
      return Math.round(sum / live.length);
    },

    wouldInterrupt(z) {
      if (!z.route || !SESSION_ROUTES.has(z.route)) return null;
      return (
        zones.find(
          (other) =>
            other.id !== z.id &&
            isPlaying(other) &&
            !!other.route &&
            SESSION_ROUTES.has(other.route),
        ) ?? null
      );
    },

    zonesWith(member) {
      return zones.filter((z) => z.members.includes(member));
    },

    async togglePlay(z) {
      const next = !isPlaying(z);
      await busy.claim("zplay:" + z.id, async () => {
        playOverride[z.id] = { playing: next, at: Date.now() };
        try {
          await (next ? api.mediaZoneResume(z.id) : api.mediaZonePause(z.id));
          await refresh();
        } catch (e) {
          delete playOverride[z.id];
          toasts.error(next ? "Couldn't start playback" : "Couldn't pause", (e as Error).message);
        }
      });
    },

    skip(z, dir) {
      void run(
        `z${dir}:` + z.id,
        () => (dir === "next" ? api.mediaZoneNext(z.id) : api.mediaZonePrevious(z.id)),
        "Skip failed",
      );
    },

    async stop(z) {
      // Stop, not pause: it also releases the stream session, so librespot
      // stops holding the account's Spotify device. That is the whole reason
      // this verb exists separately, and why the player offers it.
      await busy.claim("zstop:" + z.id, async () => {
        playOverride[z.id] = { playing: false, at: Date.now() };
        try {
          await api.mediaZoneStop(z.id);
          await refresh();
        } catch (e) {
          delete playOverride[z.id];
          toasts.error("Couldn't stop", (e as Error).message);
        }
      });
    },

    dragVolume(z, v) {
      vol[z.id] = v;
      volAt[z.id] = Date.now();
    },

    setVolume(z, v) {
      const level = clampVol(v);
      vol[z.id] = level;
      volAt[z.id] = Date.now();
      void run("zvol:" + z.id, () => api.mediaZoneVolume(z.id, level), "Volume failed");
    },

    toggleMute(z) {
      const live = speakersOf(z).filter((sp) => sp.state);
      const allMuted = live.length > 0 && live.every((sp) => sp.state?.muted);
      void run("zmute:" + z.id, () => api.mediaZoneMute(z.id, !allMuted), "Mute failed");
    },

    async play(z, item) {
      const res = await api.mediaZonePlay(z.id, {
        provider: item.provider ?? "spotify",
        uri: item.uri,
        title: item.title,
        kind: item.kind,
      });
      // A zone play that just started is worth showing immediately: the
      // stream route in particular can take a second to reach the speakers,
      // and the card should already be on.
      playOverride[z.id] = { playing: true, at: Date.now() };
      await refresh();
      return res;
    },

    async create(body) {
      try {
        const z = await api.mediaCreateZone(body);
        await refresh();
        return z;
      } catch (e) {
        toasts.error("Couldn't create the zone", (e as Error).message);
        return null;
      }
    },

    async update(id, body) {
      try {
        const z = await api.mediaUpdateZone(id, body);
        await refresh();
        return z;
      } catch (e) {
        toasts.error("Couldn't save the zone", (e as Error).message);
        return null;
      }
    },

    async remove(id) {
      try {
        await api.mediaDeleteZone(id);
        await refresh();
        return true;
      } catch (e) {
        toasts.error("Couldn't delete the zone", (e as Error).message);
        return false;
      }
    },
  };
}
