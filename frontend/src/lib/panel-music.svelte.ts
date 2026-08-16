// The panel's speaker brain (DESIGN.md §16). Both depths of the kiosk
// surface read the same instance — the dashboard's music column and the
// music depth's player/search/queue/rooms — so one poll feeds them, a
// source picked on one is featured on the other, and playback started
// from search lands on the room the chips name. Created by
// views/Panel.svelte, which keeps it alive across depth swaps.
//
// The kid surface (views/KidMusic.svelte, DESIGN.md §17) drives its own
// instance with { sonosOnly: true }: a kid profile may only drive Sonos —
// the backend gates the bridges to match — so neither the KEF nor the zone
// poll fires and only Sonos sources ever appear.
//
// Same data deal as Home's "Playing now" card: speaker state isn't in the
// shared store, so it arrives pushed on the "music" SSE topic with a slow
// poll behind. The queue rides the same cadence — it only changes on a
// mutation, which always re-reads it anyway.
//
// Capability honesty follows §15, per source rather than per make: the
// queue, seek, play modes and grouping are a Sonos group's, because only
// Sonos has them; a KEF speaker gets its input selector and skips only on
// a network source, since there is nothing to skip through on the TV
// input; a HomeHub zone skips unless it is being streamed. What a source
// can't do is absent, never dead.
//
// Rooms follow the app's own model (lib/music/rooms.svelte.ts): a zone is
// an arrangement someone made on purpose, so it outranks the household's
// grouping and the speakers inside it stop appearing under their own
// names. Showing both would put one speaker on two cards under two names.

import { api } from "./api";
import { session, toasts } from "./stores.svelte";
import { onLive } from "./live";
import { haptic } from "./utils";
import { secs, toClock, sinceRead } from "./music/time";
import { NEXT_REPEAT } from "./music/sonos.svelte";
import { clock } from "./music/clock.svelte";
import { clampVol, createVolumeThrottle } from "./music/volume";
import { buildSources, registeredCount, roomKeyOf } from "./panel-music/sources";
import type {
    PanelMusicOptions,
    PanelMusicStore,
    PanelSource,
    PanelVendor,
} from "./panel-music/types";
import type { PanelNowPlaying } from "./panel";
import type { PlayItemBody } from "./api";
import type {
    SonosStatus,
    SonosQueueItem,
    SonosFavorite,
    KEFStatus,
    KEFSource,
    SpotifyItem,
    MediaZone,
    MediaPlay,
    MusicTimer,
    MusicTimerView,
    Listening,
    AnnounceStatus,
} from "./types";

// The vocabulary is the same module's, just kept in its own file — every
// component that names a `PanelSource` still imports it from here.
export type {
    PanelVendor,
    PanelMember,
    PanelSource,
    PanelMusicStore,
    PanelMusicOptions,
} from "./panel-music/types";
export { roomKeyOf } from "./panel-music/sources";


const POLL_MS = 15_000;
const LIVE_POLL_MS = 45_000;
/** Asleep on the ambient face with a quiet house: the screen shows a clock,
 *  so the speakers only need catching up with often enough that waking is
 *  current. */
const IDLE_POLL_MS = 120_000;
/** Asleep on the ambient face with a record on. The face is *showing* the
 *  track and how far through it is (§16), so this is not a sleeping screen
 *  in the sense the number above was chosen for — it is a now-playing
 *  display, and a display that is two minutes behind is wrong rather than
 *  economical. Still half the awake rate: a wall asleep on a record must
 *  not cost more than one open browser tab. */
const IDLE_PLAYING_POLL_MS = 30_000;

/**
 * How often to ask, given what the panel is doing and what the bridge can
 * do. Pure and exported so the rule is one statement under test rather than
 * a nested ternary inside an effect.
 *
 * `live` is the Sonos event monitor answering from its own cache: pushed
 * changes already wake the poll, so the interval is only a backstop.
 */
export function pollEveryMs(opts: { idle: boolean; playing: boolean; live: boolean }): number {
    if (opts.idle) return opts.playing ? IDLE_PLAYING_POLL_MS : IDLE_POLL_MS;
    return opts.live ? LIVE_POLL_MS : POLL_MS;
}

function seenSpeakers(): boolean {
    try {
        return localStorage.getItem("speakers-seen") === "true";
    } catch {
        return false;
    } // private browsing
}

export function createPanelMusic(opts: PanelMusicOptions = {}): PanelMusicStore {
    let status = $state<SonosStatus | null>(null);
    let kef = $state<KEFStatus | null>(null);
    let zones = $state<MediaZone[] | null>(null);
    let failed = $state(false);
    const busy = $state<Record<string, boolean>>({});
    let seq = 0;
    /** Browser wall-clock of the last poll that got an answer from any
     *  bridge. Every position derivation ticks from here so the rail creeps
     *  instead of stepping — and pairs it with the reading's own `readAt`
     *  (`sinceRead`), because a poll landing now does not mean the position
     *  in it was read now. */
    let polledAt = 0;

    async function refresh() {
        const mine = ++seq;
        // Every bridge in one pass, settled: one brand being absent or down
        // must not blank the others. (sonosOnly: the one bridge it is.)
        const [sonosRes, kefRes, zoneRes] = await Promise.allSettled(
            opts.sonosOnly
                ? [api.sonosStatus()]
                : [api.sonosStatus(), api.kefStatus(), api.mediaZones()],
        );
        if (mine !== seq) return;
        if (sonosRes.status === "fulfilled") status = sonosRes.value;
        if (kefRes?.status === "fulfilled") kef = kefRes.value;
        if (zoneRes?.status === "fulfilled") zones = zoneRes.value;
        // Any bridge answering stamps the pass: the positions below are all
        // extrapolated from it, and a KEF that answered while Sonos was down
        // still has a fresh reading to tick forward from.
        if (
            sonosRes.status === "fulfilled" ||
            kefRes?.status === "fulfilled" ||
            zoneRes?.status === "fulfilled"
        ) {
            polledAt = Date.now();
        }
        failed =
            sonosRes.status === "rejected" &&
            kefRes?.status !== "fulfilled" &&
            zoneRes?.status !== "fulfilled";
        // Keep the "speakers-seen" memory fresh — the panel sizes its grid
        // from it before the first poll lands (NowPlaying is the other
        // writer). Registered, not reachable: a speaker that isn't answering
        // right now is still a speaker this home has.
        if (!failed) {
            try {
                localStorage.setItem("speakers-seen", String(registered > 0));
            } catch {
                /* private browsing */
            }
        }
    }

    // The speaker endpoints answer admins and kid profiles (the backend's
    // requireAdminOrKid); anyone else would just poll a 403. Derived, not
    // read straight off `status`: this effect calls refresh(), which
    // reassigns `status` — reading it here directly would retrigger the
    // effect forever.
    const livePush = $derived(!!status?.live);

    // The panel tells the store when it falls asleep, so the poll can slow
    // right down while the ambient face is up. A pushed change still wakes
    // it immediately — this only changes the backstop's cadence.
    let idle = $state(false);

    $effect(() => {
        if (!session.isAdmin && !session.user?.kid) return;
        void refresh();
        const onVisible = () => {
            if (!document.hidden) void refresh();
        };
        // `anyPlaying` belongs in here for the same reason `livePush` does,
        // and with the same care: it is a *derived boolean*, so the effect
        // re-runs when the house starts or stops playing and not on every
        // poll. Reading `status` directly here would retrigger forever,
        // since refresh() reassigns it.
        const every = pollEveryMs({ idle, playing: anyPlaying, live: livePush });
        const t = setInterval(onVisible, every);
        const stopLive = onLive("music", () => {
            if (!document.hidden) void refresh();
        });
        document.addEventListener("visibilitychange", onVisible);
        return () => {
            clearInterval(t);
            stopLive();
            document.removeEventListener("visibilitychange", onVisible);
        };
    });

    // The 1s beat the rail's extrapolation subscribes to. Held for the
    // store's life — the panel already ticks a clock at this cadence.
    $effect(() => clock.start());

    // ── Sources and the featured one ─────────────────────────────────────
    // A panel shows one room's playback at a time. Zones first — they are
    // the arrangement someone made on purpose — then the household's own
    // Sonos grouping, then the KEF speakers standing alone; the featured
    // one is the user's pick, else whatever is playing, else the first.
    const speakers = $derived(status?.speakers ?? []);
    const sources = $derived(buildSources({ status, kef, zones }));
    const registered = $derived(registeredCount({ status, kef, zones }));


    let selected = $state<string | null>(null);
    const featured = $derived(
        sources.find((s) => s.key === selected) ?? sources.find((s) => s.playing) ?? sources[0],
    );

    /** Turn the current fallback into a real pick. Without this, a room that
     *  starts playing somewhere else re-derives `featured` mid-search and
     *  the next tap lands in a room nobody chose. */
    function latchFeatured() {
        if (!selected && featured) selected = featured.key;
    }

    // Seeded from the "speakers-seen" memory so a home with speakers doesn't
    // watch the music column pop in after the poll; the seed holds until the
    // first real answer lands, then the truth takes over. A *failed* read is
    // not an answer: it says nothing about what this home owns, so the last
    // word stands and the column reports the outage instead of vanishing —
    // a wall panel must not reflow its grid on a dropped packet.
    let hasSpeakers = $state(seenSpeakers());
    $effect(() => {
        if (failed) return;
        if (status || kef || zones) hasSpeakers = registered > 0 || sources.length > 0;
    });
    const unreachable = $derived(hasSpeakers && sources.length === 0);

    const anyPlaying = $derived(sources.some((s) => s.playing));

    // ── Actions ──────────────────────────────────────────────────────────
    async function run(key: string, fn: () => Promise<unknown>, errTitle: string, ok?: () => void) {
        if (busy[key]) return;
        busy[key] = true;
        haptic();
        try {
            await fn();
            await refresh();
            ok?.();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    function togglePlay(s: PanelSource) {
        const label = s.playing ? "Pause failed" : "Play failed";
        const call = () => {
            if (s.kind === "zone") return s.playing ? api.mediaZonePause(s.id) : api.mediaZoneResume(s.id);
            if (s.kind === "kef") return s.playing ? api.kefPause(s.id) : api.kefPlay(s.id);
            return s.playing ? api.sonosPause(s.id) : api.sonosPlay(s.id);
        };
        void run("play:" + s.id, call, label);
    }

    /** Every source that is playing, paused at once — the bedtime tap a
     *  wall panel gets asked for and had no button for. */
    function pauseAll() {
        const live = sources.filter((s) => s.playing);
        if (!live.length) return;
        void run(
            "pauseall",
            () =>
                Promise.all(
                    live.map((s) =>
                        s.kind === "zone"
                            ? api.mediaZonePause(s.id)
                            : s.kind === "kef"
                              ? api.kefPause(s.id)
                              : api.sonosPause(s.id),
                    ),
                ),
            "Couldn't pause everything",
        );
    }

    // Honest per source, not per make: a Sonos group always has a queue to
    // step through, a zone has one unless it is being streamed, and a KEF
    // speaker has one only on a network input (`canSkip`, built above).
    function skip(s: PanelSource, dir: "next" | "previous") {
        if (!s.canSkip) return;
        const call = () => {
            if (s.kind === "zone") return dir === "next" ? api.mediaZoneNext(s.id) : api.mediaZonePrevious(s.id);
            if (s.kind === "kef") return dir === "next" ? api.kefNext(s.id) : api.kefPrevious(s.id);
            return dir === "next" ? api.sonosNext(s.id) : api.sonosPrevious(s.id);
        };
        void run(dir + ":" + s.id, call, "Skip failed");
    }

    /** Waking a speaker is not configuring one, so the wall does it itself
     *  rather than sending someone to the full view for a power toggle. */
    function wake(s: PanelSource) {
        if (s.kind !== "kef") return;
        void run("power:" + s.id, () => api.kefSetPower(s.id, true), "Couldn't wake it", () => {
            // The speaker takes a moment to come up and report itself awake.
            for (const ms of [1500, 4000]) setTimeout(() => void refresh(), ms);
        });
    }

    // ── Position and seek ────────────────────────────────────────────────
    // A just-issued seek wins over the polled position until the speaker
    // has had time to report it — same contract as the module's bridges.
    let seekOv = $state<{ sec: number; at: number } | null>(null);

    const durSec = $derived.by(() => {
        const f = featured;
        if (!f) return 0;
        return f.kind === "sonos" ? secs(f.duration) : (f.durationMs ?? 0) / 1000;
    });

    const posSec = $derived.by(() => {
        void clock.beat; // re-derive once a second so the rail creeps
        const f = featured;
        if (!f) return 0;
        const now = Date.now();
        // A just-issued seek is the user's own number on the user's own
        // clock, so it advances from the tap rather than from any reading.
        const held = seekOv && now - seekOv.at < 4000;
        if (f.kind === "sonos") {
            const base = held ? seekOv!.sec : secs(f.position);
            if (!f.playing || !polledAt) return base;
            const since = held ? (now - seekOv!.at) / 1000 : sinceRead(f.readAt, polledAt, now);
            const adv = base + since;
            return durSec ? Math.min(durSec, adv) : adv;
        }
        const base = (f.positionMs ?? 0) / 1000;
        if (!f.playing || !polledAt) return base;
        const adv = base + sinceRead(f.readAt, polledAt, now);
        return durSec ? Math.min(durSec, adv) : adv;
    });

    // A new track must never inherit the previous one's seek override.
    let lastTrack = "";
    $effect(() => {
        const t = `${featured?.id}:${featured?.trackTitle}:${durSec}`;
        if (t === lastTrack) return;
        lastTrack = t;
        seekOv = null;
    });

    // Seek exists where there is a track with a length behind it — a Sonos
    // queued track. Radio reports no duration, KEF has no seek endpoint at
    // all, and a zone can't be scrubbed however it is routed, so all three
    // render as honest rails instead (TrackRail's job).
    const seekable = $derived(!!featured && featured.kind === "sonos" && durSec > 0);

    function seek(sec: number) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        seekOv = { sec, at: Date.now() };
        void run("seek:" + f.id, () => api.sonosSeek(f.id, toClock(sec)), "Seek failed");
    }

    // ── Volume ───────────────────────────────────────────────────────────
    // The room answers the finger while it moves, not when it lifts: the
    // local value shows at once and a throttled send goes out as the drag
    // runs (`lib/music/volume.ts`, the same helper the Music view's faders
    // use), with the authoritative value on release. Group volume for Sonos
    // and zone-wide for a zone, so the whole room answers rather than one
    // speaker in it.
    //
    // The drag flags are deliberately not reactive — the sync effects must
    // re-run when the speaker's reported volume moves, never when the finger
    // lifts. `…At` extends the same claim past release: for a moment after
    // the send, a poll that hasn't caught up yet must not flinch the slider
    // back to the value the speaker was last read at.
    const VOL_HOLD_MS = 2500;
    let vol = $state(0);
    let dragging = false;
    let volAt = 0;
    let dragSource: PanelSource | null = null;
    $effect(() => {
        const level = featured?.volume;
        if (level === undefined) return;
        if (dragging || Date.now() - volAt < VOL_HOLD_MS) return;
        vol = level;
    });

    function sendVolume(s: PanelSource, level: number): Promise<void> {
        if (s.kind === "zone") return api.mediaZoneVolume(s.id, level);
        if (s.kind === "kef") return api.kefSetVolume(s.id, level);
        return api.sonosSetVolume(s.id, level, true);
    }

    const volThrottle = createVolumeThrottle((id, level) => {
        const s = dragSource;
        if (!s || s.id !== id) return;
        // A dropped mid-drag frame self-heals on release or the next poll.
        void sendVolume(s, level).catch(() => { });
    });

    function dragVolume(s: PanelSource, level: number) {
        const v = clampVol(level);
        dragging = true;
        dragSource = s;
        vol = v;
        volAt = Date.now();
        volThrottle.schedule(s.id, v);
    }
    function setVolume(s: PanelSource, level: number) {
        const v = clampVol(level);
        volThrottle.cancel(s.id);
        dragging = false;
        vol = v;
        volAt = Date.now();
        void run("vol:" + s.id, () => sendVolume(s, v), "Volume failed");
    }
    /** One step of the ± buttons. A 10px rail is a poor aim at arm's length,
     *  so the wall gets a discrete way to move the volume as well. */
    function nudgeVolume(s: PanelSource, delta: number) {
        setVolume(s, clampVol(vol + delta));
    }

    // Per-member faders, one per speaker in a multi-speaker group or zone.
    // Same contract, keyed by speaker id — and vendor-aware, because a zone
    // can hold both makes and each takes its own call.
    const memVol = $state<Record<string, number>>({});
    const memDrag: Record<string, boolean> = {};
    const memAt: Record<string, number> = {};
    $effect(() => {
        const now = Date.now();
        for (const m of featured?.members ?? []) {
            if (memDrag[m.id] || now - (memAt[m.id] ?? 0) < VOL_HOLD_MS) continue;
            memVol[m.id] = m.volume;
        }
    });
    function memberVendor(id: string): PanelVendor {
        return featured?.members?.find((m) => m.id === id)?.vendor ?? "sonos";
    }
    function sendMemberVolume(id: string, level: number): Promise<void> {
        switch (memberVendor(id)) {
            case "kef":
                return api.kefSetVolume(id, level);
            case "airplay":
                return api.airplaySetVolume(id, level);
            case "upnp":
                return api.upnpSetVolume(id, level);
            default:
                return api.sonosSetVolume(id, level);
        }
    }
    const memThrottle = createVolumeThrottle((id, level) => {
        void sendMemberVolume(id, level).catch(() => { });
    });
    function dragMemberVolume(id: string, level: number) {
        const v = clampVol(level);
        memDrag[id] = true;
        memVol[id] = v;
        memAt[id] = Date.now();
        memThrottle.schedule(id, v);
    }
    function setMemberVolume(id: string, level: number) {
        const v = clampVol(level);
        memThrottle.cancel(id);
        memDrag[id] = false;
        memVol[id] = v;
        memAt[id] = Date.now();
        void run("vol:" + id, () => sendMemberVolume(id, v), "Volume failed");
    }

    function toggleMute(s: PanelSource, memberId?: string) {
        if (memberId) {
            const m = s.members?.find((x) => x.id === memberId);
            const call = () =>
                memberVendor(memberId) === "kef"
                    ? api.kefSetMute(memberId, !m?.muted)
                    : api.sonosSetMute(memberId, !m?.muted);
            void run("mute:" + memberId, call, "Mute failed");
            return;
        }
        if (s.kind === "kef") {
            void run("mute:" + s.id, () => api.kefSetMute(s.id, !s.muted), "Mute failed");
            return;
        }
        if (s.kind === "zone") {
            void run("mute:" + s.id, () => api.mediaZoneMute(s.id, !s.muted), "Mute failed");
            return;
        }
        // The group reads as muted only when every speaker is; anything
        // less mutes all, so one tap always takes the room somewhere clear.
        // (This said `some` and did the reverse: with one speaker of a pair
        // muted, the button labelled "Mute" unmuted the other one.)
        const members = s.members ?? [];
        if (!members.length) return;
        const next = !members.every((m) => m.muted);
        void run("mute:" + s.id, () => Promise.all(members.map((m) => api.sonosSetMute(m.id, next))), "Mute failed");
    }

    // ── Play modes (a Sonos coordinator's group_state) ───────────────────
    function toggleShuffle() {
        const f = featured;
        const gs = f?.groupState;
        if (!f || !gs) return;
        void run("mode:" + f.id, () => api.sonosSetPlayMode(f.id, !gs.shuffle, gs.repeat), "Couldn't change play mode");
    }
    function cycleRepeat() {
        const f = featured;
        const gs = f?.groupState;
        if (!f || !gs) return;
        void run(
            "mode:" + f.id,
            () => api.sonosSetPlayMode(f.id, gs.shuffle, NEXT_REPEAT[gs.repeat]),
            "Couldn't change play mode",
        );
    }
    function toggleCrossfade() {
        const f = featured;
        const gs = f?.groupState;
        if (!f || !gs) return;
        void run("xfade:" + f.id, () => api.sonosSetCrossfade(f.id, !gs.crossfade), "Couldn't change crossfade");
    }
    // "Play similar" is the hub's own preference rather than something the
    // speaker reports (§15.5), but it hangs off a coordinator exactly like
    // crossfade does, so it rides with the play modes.
    function toggleAutoplay() {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run(
            "autoplay:" + f.id,
            () => api.sonosSetAutoplay(f.id, !f.autoplay),
            "Couldn't change what plays next",
        );
    }

    // ── KEF input — the "play this" control a KEF raises that nothing
    // else does (§15). Every model shows the same list; a model without
    // one simply refuses it.
    function setKefSource(s: PanelSource, source: KEFSource) {
        if (s.kind !== "kef") return;
        void run("src:" + s.id, () => api.kefSetSource(s.id, source), "Couldn't switch input");
    }

    // ── Sonos' own sleep timer ───────────────────────────────────────────
    // Set in the Sonos app, kept by the speaker, and reported by it. HomeHub
    // has its own timers now (below) and those are what the wall *sets* —
    // they reach a KEF and a zone, they fade, and they put the volume back.
    // This read stays because a timer somebody set elsewhere is still going
    // to stop this room, and a panel that knows and doesn't say would be
    // lying by omission. The two are never folded into one number: they are
    // different clocks kept by different things.
    //
    // The full settings read is a handful of SOAP calls, so it runs once per
    // featured room rather than on the poll, and the minutes left are
    // counted down locally between reads.
    let sleep = $state<{ mins: number; at: number }>({ mins: 0, at: 0 });
    let sleepFor = "";
    async function loadSleep(coordinatorId: string) {
        try {
            const s = await api.sonosSettings(coordinatorId);
            if (sleepFor !== coordinatorId) return;
            sleep = { mins: s.sleep_minutes, at: Date.now() };
        } catch {
            if (sleepFor === coordinatorId) sleep = { mins: 0, at: 0 };
        }
    }
    $effect(() => {
        const f = featured;
        const id = f?.kind === "sonos" ? f.id : "";
        if (id === sleepFor) return;
        sleepFor = id;
        sleep = { mins: 0, at: 0 };
        if (id) void loadSleep(id);
    });
    const sonosSleepMinutes = $derived.by(() => {
        void clock.beat; // the countdown is the display, not another read
        if (!sleep.mins || !sleep.at) return 0;
        return Math.max(0, sleep.mins - Math.floor((Date.now() - sleep.at) / 60_000));
    });
    function setSonosSleep(minutes: number) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run(
            "sleep:" + f.id,
            async () => {
                await api.sonosUpdateSettings(f.id, { sleep_minutes: minutes });
                sleep = { mins: minutes, at: Date.now() };
            },
            "Couldn't change the speaker's timer",
        );
    }

    // ── HomeHub's music timers ───────────────────────────────────────────
    // The wall's own "quiet in forty minutes", and the other end of the same
    // mechanism: "wake this room at 06:45". Both reach every kind of room
    // the panel can feature — a Sonos group, a KEF speaker, a HomeHub zone —
    // where the speaker's own timer reaches only the one make that has one.
    //
    // Read on a slow beat of its own rather than on the speaker poll: a
    // timer changes when somebody sets one and when one fires, and the
    // countdown between reads is arithmetic, not a round trip. The one thing
    // that genuinely moves on its own is a ramp in flight, and a minute's
    // granularity is right for something that takes five.
    //
    // Not on the kid surface: the endpoints are admin-only, and asking would
    // be a guaranteed 403 on every load of a screen with no control to draw
    // with the answer.
    const TIMERS_MS = 60_000;
    let timers = $state<MusicTimerView[]>([]);

    async function loadTimers() {
        if (opts.sonosOnly) return;
        try {
            timers = await api.musicTimers();
        } catch {
            timers = [];
        }
    }

    const roomTimers = $derived.by(() => {
        const key = roomKeyOf(featured);
        return key ? timers.filter((t) => t.room === key) : [];
    });
    /** A sleep timer is the one-shot stop: no time of day, nothing to play.
     *  A recurring "stop at 23:00" is a standing instruction and belongs in
     *  the list with the wake-ups, not on the "quiet in…" row. */
    const sleepTimer = $derived(
        roomTimers.find((t) => t.action === "stop" && !t.time && !!t.fires_at),
    );
    const fading = $derived(roomTimers.some((t) => t.fading));

    /** Minutes until the room is actually quiet: the timer fires when the
     *  fade *starts*, so the fade's own length is still to come. */
    const sleepMinutesLeft = $derived.by(() => {
        void clock.beat;
        const t = sleepTimer;
        if (!t?.fires_at || !t.enabled) return 0;
        const quietAt = Date.parse(t.fires_at) + (t.fade_minutes ?? 0) * 60_000;
        return Math.max(0, Math.ceil((quietAt - Date.now()) / 60_000));
    });

    function setSleepIn(minutes: number) {
        const key = roomKeyOf(featured);
        if (!key) return;
        void run(
            "sleepin:" + key,
            () => api.musicSleep({ room: key, minutes }).then(loadTimers),
            "Couldn't set the sleep timer",
        );
    }

    function clearSleep() {
        const t = sleepTimer;
        if (!t) return;
        // Deleting cancels the ramp too, which puts the volume back and
        // leaves the music playing — "I'm still up" said with the timer.
        void run(
            "sleepin:" + t.room,
            () => api.musicDeleteTimer(t.id).then(loadTimers),
            "Couldn't clear the sleep timer",
        );
    }

    function cancelFade() {
        const key = roomKeyOf(featured);
        if (!key) return;
        void run(
            "fade:" + key,
            () => api.musicCancelFade(key).then(loadTimers),
            "Couldn't stop the fade",
        );
    }

    function setWake(o: {
        time: string;
        days: number[];
        volume?: number;
        fadeMinutes?: number;
        item: MusicTimer["item"];
        name?: string;
    }) {
        const key = roomKeyOf(featured);
        if (!key || !o.item?.uri) return;
        void run(
            "wake:" + key + ":" + o.time,
            () =>
                api
                    .musicCreateTimer({
                        room: key,
                        action: "start",
                        enabled: true,
                        time: o.time,
                        days: o.days,
                        item: o.item,
                        volume: o.volume,
                        fade_minutes: o.fadeMinutes,
                        name: o.name,
                    })
                    .then(loadTimers),
            "Couldn't set that alarm",
        );
    }

    function setTimerEnabled(t: MusicTimerView, enabled: boolean) {
        void run(
            "timer:" + t.id,
            () =>
                api
                    .musicUpdateTimer(t.id, {
                        room: t.room,
                        action: t.action,
                        enabled,
                        fires_at: t.fires_at,
                        time: t.time,
                        days: t.days,
                        item: t.item,
                        volume: t.volume,
                        fade_minutes: t.fade_minutes,
                        name: t.name,
                    })
                    .then(loadTimers),
            "Couldn't change that timer",
        );
    }

    function deleteTimer(t: MusicTimerView) {
        void run(
            "timer:" + t.id,
            () => api.musicDeleteTimer(t.id).then(loadTimers),
            "Couldn't remove that timer",
        );
    }

    // ── What the household listens to ────────────────────────────────────
    // The one picture no single room can give, and the reason it rides on
    // the timers' beat rather than the speakers': it changes when something
    // is played, which the panel finds out about anyway.
    let insights = $state<Listening | null>(null);
    async function loadInsights() {
        if (opts.sonosOnly) return;
        try {
            insights = await api.mediaInsights(8);
        } catch {
            insights = null;
        }
    }

    $effect(() => {
        if (opts.sonosOnly) return;
        if (!session.isAdmin) return;
        void loadTimers();
        void loadInsights();
        const t = setInterval(() => {
            if (document.hidden) return;
            void loadTimers();
            void loadInsights();
        }, TIMERS_MS);
        return () => clearInterval(t);
    });

    // ── Queue ────────────────────────────────────────────────────────────
    let queue = $state<SonosQueueItem[]>([]);
    let queueLoading = $state(false);
    let queueSeq = 0;
    /** Whose queue is loaded/loading, and whose answered last — a skeleton
     *  is only worth showing while the featured room's first read is out;
     *  the re-reads the poll triggers after that are silent. */
    let queueFor = "";
    let loadedFor = "";

    async function loadQueue(coordinatorId: string) {
        const mine = ++queueSeq;
        if (loadedFor !== coordinatorId) queueLoading = true;
        try {
            const q = await api.sonosQueue(coordinatorId);
            if (mine !== queueSeq) return;
            queue = q;
            loadedFor = coordinatorId;
        } catch {
            if (mine !== queueSeq) return;
            queue = [];
            loadedFor = coordinatorId;
        } finally {
            if (mine === queueSeq) queueLoading = false;
        }
    }

    // The queue belongs to whatever Sonos group is featured — and to
    // nothing else. Reading `featured` here re-runs this on each poll,
    // which is exactly the cadence the queue wants: it only changes on a
    // mutation, and those re-read it below anyway.
    $effect(() => {
        const f = featured;
        const id = f?.kind === "sonos" ? f.id : "";
        if (id !== queueFor) {
            queueFor = id;
            queueSeq++; // cancel any load for the previous room
            queue = [];
        }
        if (id) void loadQueue(id);
    });

    /** Queue order is play order only while the group plays straight
     *  through it. Under shuffle the speaker picks its own next track, and
     *  under repeat-one it plays this one again — so "up next" would be a
     *  guess, and the wall doesn't guess. */
    const queueOrderKnown = $derived.by(() => {
        const gs = featured?.groupState;
        if (!gs) return false;
        return !gs.shuffle && gs.repeat !== "one";
    });
    const nextInQueue = $derived(
        queueOrderKnown ? queue.find((q) => q.track > (featured?.queueTrack ?? 0)) : undefined,
    );

    // ── What the ambient face says ───────────────────────────────────────
    // It lives down here rather than up with the sources because the face
    // now says more than the record's name (§16): where the room is in its
    // queue, and what comes after this — both the queue's facts, and both
    // held to the same rule as the player's own Up-next row. Under shuffle
    // or repeat-one the speaker picks its own next track, so the wall says
    // nothing rather than guessing.
    //
    // Everything here changes on a poll. The position doesn't — it ticks
    // once a second — so it stays a live value the face reads (`posSec`),
    // never a field in here that would rebuild the object every beat.
    const nowPlaying = $derived.by((): PanelNowPlaying | null => {
        const f = featured;
        if (!f?.playing) return null;
        const gs = f.groupState;
        // A queue position is only a fact while the room is walking through
        // a queue: radio and line-in report no track number, and "3 of 0"
        // is worse than saying nothing.
        const inQueue = !!gs?.from_queue && !!gs.queue_length && !!f.queueTrack;
        const n = nextInQueue;
        return {
            title: f.trackTitle ?? "Playing",
            // The room used to be the tail of this line. It is a different
            // kind of fact from the artist and the album, and the face has
            // a row for it now, beside the waveform that says this is the
            // room making the noise.
            sub: f.trackSub ?? "",
            art: f.art,
            room: f.title,
            queueTrack: inQueue ? f.queueTrack : undefined,
            queueLength: inQueue ? gs?.queue_length : undefined,
            next: n
                ? {
                      title: n.title || "Untitled",
                      sub: [n.artist, n.album].filter(Boolean).join(" · ") || undefined,
                  }
                : undefined,
            // The other rooms with something of their own on. A wall panel
            // is the one screen in the house that can answer "is anything
            // still playing upstairs" without being walked over to.
            elsewhere: sources.filter((s) => s.playing && s.key !== f.key).map((s) => s.title),
        };
    });

    function jumpTo(track: number) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run("jump:" + track, () => api.sonosSeekTrack(f.id, track), "Couldn't play that track");
    }
    function removeQueued(track: number) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        // Removing renumbers everything below it, so re-read rather than
        // splicing locally.
        void run("qrm:" + track, () => api.sonosQueueRemove(f.id, track).then(() => loadQueue(f.id)), "Couldn't remove that track");
    }
    function clearQueue() {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run("qclear:" + f.id, () => api.sonosQueueClear(f.id).then(() => loadQueue(f.id)), "Couldn't clear the queue");
    }

    /** Move one queued track one place up or down.
     *
     *  One place at a time, by tap, because this is a wall: the app's drag
     *  would be an imprecise aim at arm's length over a five-second poll
     *  (§16's argument for tap-based grouping applies unchanged here). The
     *  move renumbers the rest of the queue, so it re-reads rather than
     *  splicing locally — the same reason removing does. */
    function moveQueued(track: number, dir: -1 | 1) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        const to = track + dir;
        if (to < 1 || to > queue.length) return;
        void run(
            "qmv:" + track,
            () => api.sonosQueueMove(f.id, track, to).then(() => loadQueue(f.id)),
            "Couldn't move that track",
        );
    }

    // Queueing never interrupts: `next` drops the item after the current
    // track instead of at the end. Nothing on screen moves when it lands —
    // adding to a group playing radio is legal but silent — so the store
    // notes what went in and the player column says so for a few seconds.
    // (An success toast would be the wrong instrument: the app answers
    // quietly, and a kiosk has no one to dismiss cards.)
    let lastQueued = $state<{ title: string; next: boolean; at: number } | null>(null);

    function enqueue(item: SpotifyItem, next: boolean) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run(
            "q:" + item.uri,
            async () => {
                await api.sonosQueueAdd(f.id, { service: "Spotify", uri: item.uri, title: item.name, next });
                await loadQueue(f.id);
                lastQueued = { title: item.name, next, at: Date.now() };
            },
            "Couldn't add to the queue",
        );
    }

    // ── Sonos favorites ──────────────────────────────────────────────────
    // The household's own list — radio stations, and whatever was starred
    // in the Sonos app. Kept on the wall because it is the only thing a
    // home without a linked Spotify account can start from here at all,
    // and because a station is a one-tap job that search can't be.
    let favorites = $state<SonosFavorite[]>([]);
    let favsFor = "";
    $effect(() => {
        // Household-wide, so any Sonos speaker can answer for the list —
        // read once per household rather than per featured room.
        const anySonos = speakers.find((sp) => sp.reachable);
        const id = anySonos?.id ?? "";
        if (!id || id === favsFor) return;
        favsFor = id;
        void api
            .sonosFavorites(id)
            .then((f) => {
                if (favsFor === id) favorites = f;
            })
            .catch(() => {
                if (favsFor === id) favorites = [];
            });
    });

    /** Favorites are a Sonos household list, so only a Sonos room takes one. */
    function playFavorite(f: SonosFavorite) {
        const s = featured;
        if (!s || s.kind !== "sonos") return;
        void run("fav:" + f.id, () => api.sonosPlayFavorite(s.id, f), "Couldn't play that");
    }

    // ── Starting something from search ───────────────────────────────────
    // The featured source is the destination — the chips above the player
    // are how it is chosen — and the player is the confirmation, since
    // playback is invisible until the next poll lands.
    async function playItem(item: SpotifyItem) {
        const s = featured;
        if (!s) return;
        const key = "item:" + item.uri;
        if (busy[key]) return;
        busy[key] = true;
        haptic();
        try {
            let body: PlayItemBody = {
                service: "Spotify",
                uri: item.uri,
                title: item.name,
                kind: item.kind,
                // Carried for the room's history rather than for the
                // speaker: a shelf tile needs a picture and a second line,
                // and asking the catalog for them again later would be a
                // service round-trip to redraw a row we already have.
                sub: item.sub,
                art_uri: item.art_url,
            };
            if (item.kind === "artist") {
                // No speaker takes an artist URI (DESIGN.md §15), so an
                // artist starts their top track — which the player then names.
                const d = await api.spotifyArtist(item.uri);
                const top = d.top_tracks[0];
                if (!top) throw new Error(`No tracks found for ${item.name}`);
                body = {
                    service: "Spotify",
                    uri: top.uri,
                    title: top.name,
                    kind: "track",
                    sub: top.sub ?? item.name,
                    art_uri: top.art_url ?? item.art_url,
                };
            }
            await startOn(s, body);
        } catch (e) {
            toasts.error("Couldn't play", (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    /** Hand one item to whichever bridge the destination belongs to. The
     *  three roads differ (queue-based on Sonos, Connect on a KEF, a route
     *  the media layer picks for a zone) and nothing above this line should
     *  have to know which. */
    async function startOn(s: PanelSource, body: PlayItemBody) {
        if (s.kind === "zone") {
            // The media layer resolves a route across whatever makes are
            // in the zone and answers with the one it chose.
            await api.mediaZonePlay(s.id, {
                provider: "spotify",
                uri: body.uri,
                title: body.title,
                kind: body.kind,
                sub: body.sub,
                art_uri: body.art_uri,
            });
        } else if (s.kind === "sonos") {
            await api.sonosPlayItem(s.id, body);
        } else {
            await api.kefPlayItem(s.id, body);
        }
        await refresh();
        // A KEF or streamed play answers as soon as *Spotify* accepted it
        // — the audio goes out to the cloud and comes back — so the read
        // above can still say "stopped". Backstops for that gap.
        if (s.kind !== "sonos") for (const ms of [1200, 4000]) setTimeout(() => void refresh(), ms);
    }

    // ── What this room played before ─────────────────────────────────────
    // HomeHub's own memory, per room, because Spotify's is one list for the
    // whole household and cannot say that the kitchen gets radio at
    // breakfast. It is what the band's shelf falls back to for a room that
    // has no queue and no favorites — a KEF or a zone, which until now got
    // a third of the wall's height as air (§16).
    //
    // A room with no history of its own answers with the household's, and
    // says which it is: the wall must never imply a room played something
    // it didn't.
    let history = $state<MediaPlay[]>([]);
    let historyHousehold = $state(false);
    let historyFor = "";
    let historySeq = 0;

    // And what it keeps *coming back to*, which is a different question and
    // usually the better answer to "put something on": the plain list says
    // what was on last, and by eight in the evening the kitchen's breakfast
    // radio and its dinner records are equally recent. Asked for at the hour
    // it currently is, so a room with a habit at this hour gets that habit
    // and a room without one gets its favourites overall — the answer says
    // which, and the shelf's label repeats it rather than inventing one.
    let topPlays = $state<MediaPlay[]>([]);
    let topPlaysByHour = $state(false);
    // Plain dates throughout this block: each one is read for the hour it
    // is now and thrown away in the same statement, never held and never
    // mutated, so there is nothing for a reactive Date to be reactive about.
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    let topPlaysHour = $state(new Date().getHours());

    async function loadHistory(key: string) {
        const mine = ++historySeq;
        // Both shelves in one pass, settled: they answer different
        // questions about the same room and either can come back empty
        // without costing the other its list.
        const [plain, top] = await Promise.allSettled([
            api.mediaHistory(key, 12),
            api.mediaTopPlays(key, { limit: 8, hour: "now" }),
        ]);
        if (mine !== historySeq) return;
        if (plain.status === "fulfilled") {
            history = plain.value.plays;
            historyHousehold = plain.value.household;
        } else {
            history = [];
            historyHousehold = false;
        }
        if (top.status === "fulfilled") {
            topPlays = top.value.plays;
            topPlaysByHour = top.value.by_hour;
            topPlaysHour = top.value.hour;
        } else {
            topPlays = [];
            topPlaysByHour = false;
        }
    }

    $effect(() => {
        const key = roomKeyOf(featured);
        if (key === historyFor) return;
        historyFor = key;
        historySeq++;
        history = [];
        historyHousehold = false;
        topPlays = [];
        topPlaysByHour = false;
        if (key) void loadHistory(key);
    });

    // The hour is half of what the ranking above was asked for, so when the
    // wall clock rolls into a new one the shelf is answering yesterday
    // evening's question. Re-read on the minute beat the store already has,
    // and only when the hour has actually changed.
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    let shelfHour = new Date().getHours();
    $effect(() => {
        void clock.beat;
        // eslint-disable-next-line svelte/prefer-svelte-reactivity
        const h = new Date().getHours();
        if (h === shelfHour) return;
        shelfHour = h;
        if (historyFor) void loadHistory(historyFor);
    });

    /** Start something out of the history again.
     *
     *  A Sonos favorite is replayed through the favorites path it came from,
     *  matched by URI against the household's current list — a favorite that
     *  has since been deleted simply stops being offered, which is better
     *  than a tile that fails. Everything else is a Spotify item and goes
     *  back the way it was started. */
    function playFromHistory(p: MediaPlay) {
        const s = featured;
        if (!s) return;
        if (p.provider === "sonos") {
            const fav = favorites.find((f) => f.uri === p.uri);
            if (fav) playFavorite(fav);
            return;
        }
        void run(
            "hist:" + p.uri,
            () =>
                startOn(s, {
                    service: "Spotify",
                    uri: p.uri,
                    title: p.title,
                    kind: p.kind,
                    sub: p.sub,
                    art_uri: p.art_uri,
                }).then(() => loadHistory(roomKeyOf(s))),
            "Couldn't play that again",
        );
    }

    /** Forget one thing this room played.
     *
     *  The counterweight to a ranked shelf. `topPlays` puts what a room keeps
     *  coming back to at the front of the wall, which is the right answer
     *  right up until the thing it keeps coming back to is a mistake — and a
     *  mistake is exactly what gets replayed, because it is the tile in the
     *  first slot. Until this existed the cures were to out-play it thirty
     *  times or to delete the speaker.
     *
     *  Never the household's list: the fallback shelf is other rooms' plays,
     *  and one room is not the place to edit them. `canForget` is what keeps
     *  the control off that shelf. */
    function forgetPlay(p: MediaPlay) {
        const key = roomKeyOf(featured);
        if (!key || historyHousehold || !session.isAdmin) return;
        void run(
            "forget:" + p.uri,
            () => api.mediaForgetPlay(key, p.uri).then(() => loadHistory(key)),
            "Couldn't forget that",
        );
    }

    const canForget = $derived(!!featured && !historyHousehold && session.isAdmin);

    // ── Saving what's playing ────────────────────────────────────────────
    // The heart. It needs a catalog id, which only a Spotify source has —
    // radio and line-in carry an artist line and nothing to save — so the
    // control renders only where `trackURI` does (§15.1 applied to a track).
    //
    // The saved *state* is read on whatever login the panel has, because
    // reading has always been in the grant; only the write needs the newer
    // scope, so an old login shows an honest heart and refuses the tap
    // rather than being offered a control that will fail.
    let savedURI = $state("");
    let saved = $state(false);
    let canSave = $state(false);
    let savedSeq = 0;

    void api
        .spotifyStatus()
        .then((st) => {
            canSave = st.connected && !!st.library;
        })
        .catch(() => {
            canSave = false;
        });

    $effect(() => {
        const uri = featured?.trackURI ?? "";
        if (uri === savedURI) return;
        savedURI = uri;
        saved = false;
        const mine = ++savedSeq;
        if (!uri) return;
        void api
            .spotifySaved(uri)
            .then((r) => {
                if (mine === savedSeq) saved = r.saved;
            })
            .catch(() => {});
    });

    function toggleSaved() {
        const uri = featured?.trackURI;
        if (!uri || !canSave) return;
        const next = !saved;
        // Optimistic: the heart is the confirmation, and a wall panel has
        // nothing else to show while a round trip to Spotify completes.
        saved = next;
        void run(
            "save:" + uri,
            () => api.spotifySetSaved(uri, next),
            next ? "Couldn't save that song" : "Couldn't remove that song",
        ).then(() => {
            // Then re-read, because the optimistic flip above is a guess
            // until Spotify agrees — and a refused write (an older grant,
            // a dropped connection) has already been toasted by run(),
            // which must not leave a heart claiming otherwise.
            if (featured?.trackURI !== uri) return;
            void api
                .spotifySaved(uri)
                .then((r) => {
                    if (featured?.trackURI === uri) saved = r.saved;
                })
                .catch(() => {});
        });
    }

    // ── More like this ───────────────────────────────────────────────────
    // The same engine "play similar" uses when a queue runs dry (§15.5),
    // asked for on purpose instead of automatically. Seeded by artist name
    // because that is what a speaker reports — a room on radio has an
    // artist line and no catalog id at all.
    //
    // On Sonos it fills the queue behind what is playing, so the record you
    // are listening to isn't interrupted by asking for more of it. Anywhere
    // else there is no queue to fill, so the first result plays.
    let lastRadio = $state<{ count: number; artist: string; at: number } | null>(null);

    const canRadio = $derived(!!featured?.trackArtist);

    function startRadio() {
        const s = featured;
        const artist = s?.trackArtist;
        if (!s || !artist) return;
        void run(
            "radio",
            async () => {
                const items = await api.spotifySimilar(artist, 8);
                if (!items.length) throw new Error(`Nothing else by ${artist} came back`);
                if (s.kind === "sonos") {
                    // The whole run in one request. This used to be eight
                    // sequential calls sent *backwards* — Sonos resolves each
                    // "play next" against wherever the queue is at that
                    // moment, so a forwards loop scatters the run and a
                    // reversed one happens to come out in order. That trick
                    // worked and was a trick, and it cost eight round trips
                    // from the slowest client this app has. The hub does the
                    // dealing now: one request, one position read, and the
                    // order of the array is the order they land in.
                    const added = await api.sonosQueueAddMany(
                        s.id,
                        items.map((item) => ({
                            service: "Spotify",
                            uri: item.uri,
                            title: item.name,
                        })),
                        true,
                    );
                    await loadQueue(s.id);
                    lastRadio = { count: added.added || items.length, artist, at: Date.now() };
                } else {
                    await startOn(s, {
                        service: "Spotify",
                        uri: items[0].uri,
                        title: items[0].name,
                        kind: "track",
                        sub: items[0].sub,
                        art_uri: items[0].art_url,
                    });
                    lastRadio = { count: 1, artist, at: Date.now() };
                }
            },
            "Couldn't find more like this",
        );
    }

    // ── Announcements ────────────────────────────────────────────────────
    // Calling the house from the wall — the panel's own feature more than
    // the app's, since "dinner's ready" is shouted from a hallway and not
    // typed on a phone. It goes to every reachable Sonos room at once; the
    // status read is what says whether there is anywhere to announce to and
    // whether there will be words or only a chime.
    let announce = $state<AnnounceStatus | null>(null);
    let lastAnnounce = $state<{ text: string; rooms: string[]; spoken: boolean; at: number } | null>(
        null,
    );

    // Not on the kid surface: announcing is a household action and the
    // endpoint is admin-only, so asking would be one guaranteed 403 on
    // every load of a screen that has no control to draw with the answer.
    if (!opts.sonosOnly) {
        void api
            .announceStatus()
            .then((st) => {
                announce = st;
            })
            .catch(() => {
                announce = null;
            });
    }

    function sendAnnouncement(text: string, rooms?: string[]) {
        void run(
            "announce",
            async () => {
                const res = await api.announce(text, rooms);
                lastAnnounce = {
                    text,
                    rooms: res.rooms,
                    spoken: res.spoken,
                    at: Date.now(),
                };
                // Every room has been interrupted and will be put back a few
                // seconds from now; re-read once that has happened so the
                // panel doesn't show the announcement as what's playing.
                setTimeout(() => void refresh(), res.duration_ms + 1500);
            },
            "Couldn't announce that",
        );
    }

    // ── Grouping ─────────────────────────────────────────────────────────
    // Sonos-native only, the daily "play together": joining is the whole
    // card, not one speaker — a room that moves takes its partners with
    // it. Cross-vendor zones are played from the wall but never built
    // there; making a persistent routed room is configuration.
    /** What a Sonos room could group with right now. A KEF speaker and a
     *  HomeHub zone are absent rather than refused: neither joins a Sonos
     *  household, and a zone is arranged in the Music view, never here. */
    const joinable = $derived.by(() => {
        const f = featured;
        if (!f || f.kind !== "sonos") return [];
        return sources.filter((s) => s.kind === "sonos" && s.key !== f.key);
    });

    const canGroup = $derived(
        !!featured &&
            featured.kind === "sonos" &&
            (joinable.length > 0 || (featured.members?.length ?? 0) > 1),
    );

    function joinSource(src: PanelSource) {
        const f = featured;
        if (!f || f.kind !== "sonos" || src.kind !== "sonos") return;
        const members = (src.members ?? []).map((m) => m.id);
        if (!members.length) return;
        void run(
            "join:" + src.id,
            () => api.sonosGroup(f.id, { join: members }),
            "Grouping failed",
            () => {
                selected = f.key; // the group stays featured through the reshuffle
            },
        );
    }

    /** Everything at once — one request, and the hub walks the household
     *  through it in order. It has to be walked rather than fired off at
     *  once: a household handed four `SetAVTransportURI`s in the same instant
     *  re-elects its coordinators mid-flight and lands with a speaker or two
     *  left out. That sequencing used to live here, which meant it survived
     *  only as long as the page did — an iPad that slept in the middle of
     *  "play it everywhere" left the house half grouped. */
    function joinAll() {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        const ids = joinable.flatMap((s) => (s.members ?? []).map((m) => m.id));
        if (!ids.length) return;
        void run(
            "joinall",
            () => api.sonosGroup(f.id, { join: ids }),
            "Grouping failed",
            () => {
                selected = f.key;
            },
        );
    }

    /** Take the music with you: the featured room's group moves to `dest`
     *  and leaves where it was.
     *
     *  This is the gesture a wall gets asked for on the way into the
     *  kitchen, and until now it cost two: join, then split the room you
     *  walked out of. Composed from the same two calls rather than given a
     *  bridge of its own — Sonos has no "move", and what a move *is* on a
     *  household is exactly this pair.
     *
     *  Order matters and is the whole reason this is one call rather than a
     *  loop: the destination joins the group *first*, so the queue and the
     *  stream are handed over while the old room is still coordinating, and
     *  only then does the old room step out. Doing it the other way round
     *  stops the music between the two calls — and doing it from here meant
     *  the ordering held only as long as this page did, so a wall that fell
     *  asleep between the two halves left the music where you weren't. The
     *  hub owns the sequence now; this states the pair. */
    function moveTo(dest: PanelSource) {
        const f = featured;
        if (!f || f.kind !== "sonos" || dest.kind !== "sonos" || dest.key === f.key) return;
        const leaving = (f.members ?? []).map((m) => m.id);
        const arriving = (dest.members ?? []).map((m) => m.id);
        if (!leaving.length || !arriving.length) return;
        void run(
            "move:" + dest.id,
            () => api.sonosGroup(f.id, { join: arriving, leave: leaving }),
            "Couldn't move the music",
            () => {
                // Follow the sound: the destination is what the panel should
                // be pointed at once the music is there.
                selected = dest.key;
            },
        );
    }

    function ungroupFeatured() {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        const members = (f.members ?? []).filter((m) => !m.coordinator);
        if (!members.length) return;
        void run(
            "ungroup:" + f.id,
            () => api.sonosGroup(f.id, { leave: members.map((m) => m.id) }),
            "Ungrouping failed",
        );
    }

    function leaveMember(memberId: string) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run("leave:" + memberId, () => api.sonosLeave(memberId), "Ungrouping failed");
    }

    return {
        get hasSpeakers() {
            return hasSpeakers;
        },
        get unreachable() {
            return unreachable;
        },
        get sources() {
            return sources;
        },
        get featured() {
            return featured;
        },
        get selected() {
            return selected;
        },
        set selected(v: string | null) {
            selected = v;
        },
        latchFeatured,
        get busy() {
            return busy;
        },
        get nowPlaying() {
            return nowPlaying;
        },
        get anyPlaying() {
            return anyPlaying;
        },
        setIdle(v: boolean) {
            idle = v;
        },
        get posSec() {
            return posSec;
        },
        get durSec() {
            return durSec;
        },
        get seekable() {
            return seekable;
        },
        seek,
        get vol() {
            return vol;
        },
        dragVolume,
        setVolume,
        nudgeVolume,
        get memVol() {
            return memVol;
        },
        dragMemberVolume,
        setMemberVolume,
        toggleMute,
        togglePlay,
        skip,
        wake,
        pauseAll,
        toggleShuffle,
        cycleRepeat,
        toggleCrossfade,
        toggleAutoplay,
        setKefSource,
        get timers() {
            return timers;
        },
        get roomTimers() {
            return roomTimers;
        },
        get sleepTimer() {
            return sleepTimer;
        },
        get sleepMinutesLeft() {
            return sleepMinutesLeft;
        },
        get fading() {
            return fading;
        },
        setSleepIn,
        clearSleep,
        cancelFade,
        setWake,
        setTimerEnabled,
        deleteTimer,
        get sonosSleepMinutes() {
            return sonosSleepMinutes;
        },
        setSonosSleep,
        get queue() {
            return queue;
        },
        get queueLoading() {
            return queueLoading;
        },
        get nextInQueue() {
            return nextInQueue;
        },
        get queueOrderKnown() {
            return queueOrderKnown;
        },
        jumpTo,
        removeQueued,
        moveQueued,
        clearQueue,
        enqueue,
        get lastQueued() {
            return lastQueued;
        },
        get favorites() {
            return favorites;
        },
        playFavorite,
        playItem,
        get history() {
            return history;
        },
        get historyHousehold() {
            return historyHousehold;
        },
        playFromHistory,
        forgetPlay,
        get canForget() {
            return canForget;
        },
        get topPlays() {
            return topPlays;
        },
        get topPlaysByHour() {
            return topPlaysByHour;
        },
        get topPlaysHour() {
            return topPlaysHour;
        },
        get insights() {
            return insights;
        },
        get canSave() {
            return canSave;
        },
        get saved() {
            return saved;
        },
        toggleSaved,
        get canRadio() {
            return canRadio;
        },
        startRadio,
        get lastRadio() {
            return lastRadio;
        },
        get announce() {
            return announce;
        },
        sendAnnouncement,
        get lastAnnounce() {
            return lastAnnounce;
        },
        get joinable() {
            return joinable;
        },
        get canGroup() {
            return canGroup;
        },
        joinSource,
        joinAll,
        moveTo,
        ungroupFeatured,
        leaveMember,
        refresh,
    };
}
