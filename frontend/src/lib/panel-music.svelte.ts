// The panel's speaker brain (DESIGN.md §16). Both depths of the kiosk
// surface read the same instance — the dashboard's music column and the
// music depth's player/search/queue/rooms — so one poll feeds them, a
// source picked on one is featured on the other, and playback started
// from search lands on the room the chips name. Created by
// views/Panel.svelte, which keeps it alive across depth swaps.
//
// Same data deal as Home's "Playing now" card: speaker state isn't in the
// shared store, so it arrives pushed on the "music" SSE topic with a slow
// poll behind. The queue rides the same cadence — it only changes on a
// mutation, which always re-reads it anyway.
//
// Capability honesty follows §15: the queue, seek, play modes and
// grouping are a Sonos group's, because only Sonos has them; a KEF
// speaker gets its input selector instead; and cross-vendor HomeHub
// zones stay in the full Music view — making persistent routed rooms is
// configuration, not a wall gesture.

import { SvelteMap } from "svelte/reactivity";
import { api } from "./api";
import { session, toasts } from "./stores.svelte";
import { onLive } from "./live";
import { kefSourceLabel } from "./kef";
import { haptic } from "./utils";
import { secs, toClock } from "./music/time";
import { NEXT_REPEAT } from "./music/sonos.svelte";
import { clock } from "./music/clock.svelte";
import { clampVol, createVolumeThrottle } from "./music/volume";
import type { PanelNowPlaying } from "./panel";
import type {
    SonosStatus,
    SonosGroupView,
    SonosSpeakerView,
    SonosGroupState,
    SonosQueueItem,
    KEFStatus,
    KEFSource,
    SpotifyItem,
} from "./types";

/** One speaker inside a featured Sonos group, coordinator first. */
export interface PanelMember {
    id: string;
    name: string;
    volume: number;
    muted: boolean;
    coordinator: boolean;
}

/** One playable source on the wall: a reachable Sonos group or KEF speaker. */
export interface PanelSource {
    key: string;
    kind: "sonos" | "kef";
    id: string; // coordinator id (sonos) or speaker id (kef)
    title: string; // zone / speaker name
    playing: boolean;
    standby: boolean; // kef only — powered off, no transport
    volume: number;
    muted: boolean;
    trackTitle?: string;
    trackSub?: string;
    art?: string;
    // Sonos extras — the group's, so only on a Sonos source.
    members?: PanelMember[];
    groupState?: SonosGroupState;
    /** HomeHub's own "keep going with similar music" preference (§15.5). */
    autoplay?: boolean;
    queueTrack?: number; // 1-based position in the queue, when playing from it
    position?: string; // H:MM:SS — the wire form, parsed by the position deriveds
    duration?: string;
    // KEF extras.
    input?: string; // current physical input
    positionMs?: number;
    durationMs?: number;
    readAt?: number; // unix ms the KEF reading was taken
}

export interface PanelMusicStore {
    readonly hasSpeakers: boolean;
    readonly sources: PanelSource[];
    readonly featured: PanelSource | undefined;
    /** The user's chip pick; null falls back to whatever is playing. */
    selected: string | null;
    readonly busy: Record<string, boolean>;
    /** What the ambient face shows while music plays. */
    readonly nowPlaying: PanelNowPlaying | null;

    // ── Position (extrapolated between polls on the 1s beat) ──
    readonly posSec: number;
    readonly durSec: number;
    /** A seek endpoint exists: a Sonos track with a length behind it. */
    readonly seekable: boolean;
    seek(sec: number): void;

    // ── Volume ──
    /** Group (or single-speaker) fader value — the finger's while it's down. */
    readonly vol: number;
    /** Live, on every movement: shows at once, sends on a short throttle. */
    dragVolume(s: PanelSource, level: number): void;
    /** On release — the authoritative value. */
    setVolume(s: PanelSource, level: number): void;
    /** Per-member faders, when a Sonos group has more than one speaker. */
    readonly memVol: Record<string, number>;
    dragMemberVolume(id: string, level: number): void;
    setMemberVolume(id: string, level: number): void;
    /** No memberId mutes the whole group; one mutes that speaker. */
    toggleMute(s: PanelSource, memberId?: string): void;

    // ── Transport & play modes ──
    togglePlay(s: PanelSource): void;
    skip(s: PanelSource, dir: "next" | "previous"): void;
    toggleShuffle(): void;
    cycleRepeat(): void;
    toggleCrossfade(): void;
    /** "Play similar": keep the room going once the queue runs out (§15.5). */
    toggleAutoplay(): void;

    // ── KEF ──
    setKefSource(s: PanelSource, source: KEFSource): void;

    // ── Queue (a Sonos group's — empty for anything else) ──
    readonly queue: SonosQueueItem[];
    readonly queueLoading: boolean;
    /** The first queued track after the one playing — the Up-next row. */
    readonly nextInQueue: SonosQueueItem | undefined;
    jumpTo(track: number): void;
    removeQueued(track: number): void;
    clearQueue(): void;
    /** Add a search result without disturbing what's playing. */
    enqueue(item: SpotifyItem, next: boolean): void;

    // ── Starting something ──
    playItem(item: SpotifyItem): Promise<void>;

    // ── Grouping (Sonos-native only) ──
    /** Every speaker in src's group joins the featured group. */
    joinSource(src: PanelSource): void;
    /** Every non-coordinator member leaves the featured group. */
    ungroupFeatured(): void;
    /** One member steps out of the featured group. */
    leaveMember(memberId: string): void;

    refresh(): Promise<void>;
}

const POLL_MS = 15_000;
const LIVE_POLL_MS = 45_000;

function seenSpeakers(): boolean {
    try {
        return localStorage.getItem("speakers-seen") === "true";
    } catch {
        return false;
    } // private browsing
}

export function createPanelMusic(): PanelMusicStore {
    let status = $state<SonosStatus | null>(null);
    let kef = $state<KEFStatus | null>(null);
    let failed = $state(false);
    const busy = $state<Record<string, boolean>>({});
    let seq = 0;
    /** Wall-clock of the last successful Sonos read — the position deriveds
     *  advance from here so the rail creeps instead of stepping. */
    let polledAt = 0;

    async function refresh() {
        const mine = ++seq;
        // Both bridges in one pass, settled: one brand being absent or down
        // must not blank the other.
        const [sonosRes, kefRes] = await Promise.allSettled([api.sonosStatus(), api.kefStatus()]);
        if (mine !== seq) return;
        if (sonosRes.status === "fulfilled") {
            status = sonosRes.value;
            polledAt = Date.now();
        }
        if (kefRes.status === "fulfilled") kef = kefRes.value;
        failed = sonosRes.status === "rejected" && kefRes.status === "rejected";
        // Keep the "speakers-seen" memory fresh — the panel sizes its grid
        // from it before the first poll lands (NowPlaying is the other
        // writer).
        if (!failed) {
            try {
                const any = (status?.speakers.length ?? 0) + (kef?.speakers.length ?? 0) > 0;
                localStorage.setItem("speakers-seen", String(any));
            } catch {
                /* private browsing */
            }
        }
    }

    // The Sonos endpoints are admin-only. Derived, not read straight off
    // `status`: this effect calls refresh(), which reassigns `status` —
    // reading it here directly would retrigger the effect forever.
    const livePush = $derived(!!status?.live);

    $effect(() => {
        if (!session.isAdmin) return;
        void refresh();
        const onVisible = () => {
            if (!document.hidden) void refresh();
        };
        const t = setInterval(onVisible, livePush ? LIVE_POLL_MS : POLL_MS);
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
    // A panel shows one room's playback at a time. Every reachable Sonos
    // group and KEF speaker is a source; the featured one is the user's
    // pick, else whatever is playing, else the first.
    const speakers = $derived(status?.speakers ?? []);
    const byId = $derived(new SvelteMap(speakers.map((s) => [s.id, s])));

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return byId.get(g.coordinator_id) ?? byId.get(g.member_ids[0]);
    }
    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids.map((id) => byId.get(id)?.name).filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    const sources = $derived.by((): PanelSource[] => {
        const out: PanelSource[] = [];
        for (const g of status?.groups ?? []) {
            const c = coordinatorOf(g);
            if (!c?.reachable) continue;
            const st = c.state;
            const members = g.member_ids
                .map((id) => byId.get(id))
                .filter((x): x is SonosSpeakerView => !!x);
            // Group volume isn't reported by the status poll — only each
            // member's own — so the fader's value is the members' average,
            // same as the full Music view's Sonos bridge (sonos.svelte.ts).
            // Reading the coordinator's own volume here would desync the
            // fader the moment a group's members sit at different levels.
            const memberVols = members
                .map((x) => x.state?.volume)
                .filter((v): v is number => v !== undefined);
            const groupVolume = memberVols.length
                ? Math.round(memberVols.reduce((a, b) => a + b, 0) / memberVols.length)
                : (st?.volume ?? 0);
            out.push({
                key: "s:" + g.coordinator_id,
                kind: "sonos",
                id: g.coordinator_id,
                title: groupTitle(g),
                playing: !!st?.playing,
                standby: false,
                volume: groupVolume,
                muted: !!st?.muted,
                trackTitle: st?.track?.title,
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                art: st?.track?.art_uri,
                members: members.map((x) => ({
                    id: x.id,
                    name: x.name,
                    volume: x.state?.volume ?? 0,
                    muted: !!x.state?.muted,
                    coordinator: x.id === g.coordinator_id,
                })),
                groupState: c.group_state,
                autoplay: c.autoplay,
                queueTrack: st?.queue_track,
                position: st?.position,
                duration: st?.duration,
            });
        }
        for (const sp of kef?.speakers ?? []) {
            if (!sp.reachable) continue;
            const st = sp.state;
            out.push({
                key: "k:" + sp.id,
                kind: "kef",
                id: sp.id,
                title: sp.name,
                playing: !!st?.playing,
                standby: st ? !st.powered_on : false,
                volume: st?.volume ?? 0,
                muted: !!st?.muted,
                trackTitle:
                    st?.track?.title ??
                    (st?.playing && st.source ? `${kefSourceLabel(st.source)} input` : undefined),
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                art: st?.track?.art_uri,
                input: st?.source,
                positionMs: st?.position_ms,
                durationMs: st?.duration_ms,
                readAt: sp.read_at,
            });
        }
        return out;
    });

    let selected = $state<string | null>(null);
    const featured = $derived(
        sources.find((s) => s.key === selected) ?? sources.find((s) => s.playing) ?? sources[0],
    );

    // Seeded from the "speakers-seen" memory so a home with speakers doesn't
    // watch the music column pop in after the poll; the seed holds until the
    // first real answer lands, then the truth takes over.
    let hasSpeakers = $state(seenSpeakers());
    $effect(() => {
        if (failed) hasSpeakers = false;
        else if (status || kef) hasSpeakers = sources.length > 0;
    });

    const nowPlaying = $derived<PanelNowPlaying | null>(
        featured?.playing
            ? {
                title: featured.trackTitle ?? "Playing",
                sub: [featured.trackSub, featured.title].filter(Boolean).join(" · "),
                art: featured.art,
            }
            : null,
    );

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
        if (s.kind === "sonos") {
            void run("play:" + s.id, () => (s.playing ? api.sonosPause(s.id) : api.sonosPlay(s.id)), label);
        } else {
            void run("play:" + s.id, () => (s.playing ? api.kefPause(s.id) : api.kefPlay(s.id)), label);
        }
    }

    // Sonos-only on this surface, matching Home's card: KEF has no queue,
    // so on most of its sources there is nothing for a skip to step
    // through.
    function skip(s: PanelSource, dir: "next" | "previous") {
        void run(dir + ":" + s.id, () => (dir === "next" ? api.sonosNext(s.id) : api.sonosPrevious(s.id)), "Skip failed");
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
        if (f.kind === "sonos") {
            const base = seekOv && now - seekOv.at < 4000 ? seekOv.sec : secs(f.position);
            if (!f.playing || !polledAt) return base;
            const adv = base + (now - (seekOv && now - seekOv.at < 4000 ? seekOv.at : polledAt)) / 1000;
            return durSec ? Math.min(durSec, adv) : adv;
        }
        const base = (f.positionMs ?? 0) / 1000;
        if (!f.playing || !f.readAt) return base;
        const adv = base + (now - f.readAt) / 1000;
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
    // queued track. Radio reports no duration, and KEF has no seek endpoint
    // at all, so both render as honest rails instead (TrackRail's job).
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
    // use), with the authoritative value on release. Group volume for Sonos,
    // so the whole zone answers rather than just the coordinator.
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

    const volThrottle = createVolumeThrottle((id, level) => {
        const s = dragSource;
        if (!s || s.id !== id) return;
        // A dropped mid-drag frame self-heals on release or the next poll.
        void (s.kind === "sonos"
            ? api.sonosSetVolume(id, level, true)
            : api.kefSetVolume(id, level)
        ).catch(() => {});
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
        void run(
            "vol:" + s.id,
            () => (s.kind === "sonos" ? api.sonosSetVolume(s.id, v, true) : api.kefSetVolume(s.id, v)),
            "Volume failed",
        );
    }

    // Per-member faders, one per speaker in a multi-speaker group. Same
    // contract, keyed by speaker id — and always Sonos, since a group is.
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
    const memThrottle = createVolumeThrottle((id, level) => {
        void api.sonosSetVolume(id, level).catch(() => {});
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
        void run("vol:" + id, () => api.sonosSetVolume(id, v), "Volume failed");
    }

    function toggleMute(s: PanelSource, memberId?: string) {
        if (s.kind === "kef") {
            void run("mute:" + s.id, () => api.kefSetMute(s.id, !s.muted), "Mute failed");
            return;
        }
        if (memberId) {
            const m = s.members?.find((x) => x.id === memberId);
            void run("mute:" + memberId, () => api.sonosSetMute(memberId, !m?.muted), "Mute failed");
            return;
        }
        // The group reads as muted only when every speaker is; anything
        // less mutes all, so one tap always takes the room somewhere clear.
        const members = s.members ?? [];
        if (!members.length) return;
        const next = !members.some((m) => m.muted);
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

    const nextInQueue = $derived(queue.find((q) => q.track > (featured?.queueTrack ?? 0)));

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

    // Queueing never interrupts: `next` drops the item after the current
    // track instead of at the end. The toast names where it landed, since
    // adding to a group playing radio is legal but silent.
    function enqueue(item: SpotifyItem, next: boolean) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run(
            "q:" + item.uri,
            async () => {
                await api.sonosQueueAdd(f.id, { service: "Spotify", uri: item.uri, title: item.name, next });
                await loadQueue(f.id);
            },
            "Couldn't add to the queue",
        );
    }

    // ── Starting something from search ───────────────────────────────────
    // The featured source is the destination — the chips above the player
    // are how it is chosen — and the toast names both what started and
    // where, because playback is invisible until the next poll lands.
    async function playItem(item: SpotifyItem) {
        const s = featured;
        if (!s) return;
        const key = "item:" + item.uri;
        if (busy[key]) return;
        busy[key] = true;
        haptic();
        try {
            let body = { service: "Spotify", uri: item.uri, title: item.name };
            if (item.kind === "artist") {
                // No speaker takes an artist URI (DESIGN.md §15), so an
                // artist starts their top track — which the player then names.
                const d = await api.spotifyArtist(item.uri);
                const top = d.top_tracks[0];
                if (!top) throw new Error(`No tracks found for ${item.name}`);
                body = { service: "Spotify", uri: top.uri, title: top.name };
            }
            if (s.kind === "sonos") await api.sonosPlayItem(s.id, body);
            else await api.kefPlayItem(s.id, body);
            await refresh();
            // A KEF play answers as soon as *Spotify* accepted it — the
            // audio goes out to the cloud and comes back — so the read
            // above can still say "stopped". Backstops for that gap.
            if (s.kind === "kef") for (const ms of [1200, 4000]) setTimeout(() => void refresh(), ms);
        } catch (e) {
            toasts.error("Couldn't play", (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    // ── Grouping ─────────────────────────────────────────────────────────
    // Sonos-native only, the daily "play together": joining is the whole
    // card, not one speaker — a room that moves takes its partners with
    // it. Cross-vendor zones stay in the full Music view; making a
    // persistent routed room is configuration, not a wall gesture.
    function joinSource(src: PanelSource) {
        const f = featured;
        if (!f || f.kind !== "sonos" || src.kind !== "sonos") return;
        const members = (src.members ?? []).map((m) => m.id);
        if (!members.length) return;
        void run(
            "join:" + src.id,
            async () => {
                for (const id of members) await api.sonosJoin(id, f.id);
            },
            "Grouping failed",
            () => {
                selected = f.key; // the group stays featured through the reshuffle
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
            async () => {
                for (const m of members) await api.sonosLeave(m.id);
            },
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
        get busy() {
            return busy;
        },
        get nowPlaying() {
            return nowPlaying;
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
        get memVol() {
            return memVol;
        },
        dragMemberVolume,
        setMemberVolume,
        toggleMute,
        togglePlay,
        skip,
        toggleShuffle,
        cycleRepeat,
        toggleCrossfade,
        toggleAutoplay,
        setKefSource,
        get queue() {
            return queue;
        },
        get queueLoading() {
            return queueLoading;
        },
        get nextInQueue() {
            return nextInQueue;
        },
        jumpTo,
        removeQueued,
        clearQueue,
        enqueue,
        playItem,
        joinSource,
        ungroupFeatured,
        leaveMember,
        refresh,
    };
}
