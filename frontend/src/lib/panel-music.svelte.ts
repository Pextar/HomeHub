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
import type { PlayItemBody } from "./api";
import type {
    SonosStatus,
    SonosGroupView,
    SonosSpeakerView,
    SonosGroupState,
    SonosQueueItem,
    SonosFavorite,
    KEFStatus,
    KEFSource,
    SpotifyItem,
    MediaZone,
    MediaZoneSpeaker,
    MediaRoute,
    MediaPlay,
    AnnounceStatus,
} from "./types";

/** Which bridge a member speaker's own volume/mute calls go to. */
export type PanelVendor = "sonos" | "kef";

/** One speaker inside a featured group or zone, coordinator/lead first. */
export interface PanelMember {
    id: string;
    name: string;
    vendor: PanelVendor;
    volume: number;
    muted: boolean;
    coordinator: boolean;
}

/** One playable source on the wall: a HomeHub zone, a reachable Sonos
 *  group, or a reachable KEF speaker not already claimed by a zone. */
export interface PanelSource {
    key: string;
    kind: "sonos" | "kef" | "zone";
    id: string; // coordinator id (sonos), speaker id (kef), zone id (zone)
    title: string; // zone / group / speaker name
    playing: boolean;
    standby: boolean; // kef only — powered off, no transport
    volume: number;
    muted: boolean;
    /** A skip would reach something: a queue, a stream, a network input. */
    canSkip: boolean;
    trackTitle?: string;
    trackSub?: string;
    /** The artist alone, where the sub line is "artist · album". What the
     *  radio seeds from and what the artist page is opened by name with. */
    trackArtist?: string;
    /** The canonical Spotify URI of what is playing, when the source is
     *  Spotify. Absent on radio, line-in and anything else — saving and
     *  "open the artist" render only where it is present (§15.1 applied to
     *  a track rather than to a room). */
    trackURI?: string;
    art?: string;
    /** Every speaker inside it, when there is more than one to balance. */
    members?: PanelMember[];
    // Sonos extras — the group's, so only on a Sonos source.
    groupState?: SonosGroupState;
    /** HomeHub's own "keep going with similar music" preference (§15.5). */
    autoplay?: boolean;
    queueTrack?: number; // 1-based position in the queue, when playing from it
    position?: string; // H:MM:SS — the wire form, parsed by the position deriveds
    duration?: string;
    // KEF extras.
    input?: string; // current physical input
    // KEF + zone extras: milliseconds, and when the reading was taken.
    positionMs?: number;
    durationMs?: number;
    readAt?: number; // unix ms the reading was taken
    // Zone extras.
    zone?: MediaZone;
    /** How content reaches a zone right now — what makes skip honest. */
    route?: MediaRoute;
}

export interface PanelMusicStore {
    readonly hasSpeakers: boolean;
    /** Speakers are registered but none answered — the column stays and
     *  says so rather than vanishing and reflowing the panel grid. */
    readonly unreachable: boolean;
    readonly sources: PanelSource[];
    readonly featured: PanelSource | undefined;
    /** The user's chip pick; null falls back to whatever is playing. */
    selected: string | null;
    /** Pin the current fallback as an explicit pick, so a room that starts
     *  playing elsewhere can't move the destination under a finger. */
    latchFeatured(): void;
    readonly busy: Record<string, boolean>;
    /** What the ambient face shows while music plays. */
    readonly nowPlaying: PanelNowPlaying | null;
    /** Anything playing anywhere — what "Pause everything" is offered for. */
    readonly anyPlaying: boolean;
    /** While the panel sleeps, the poll can slow right down. */
    setIdle(idle: boolean): void;

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
    /** One step of the ± buttons — a fader is imprecise at arm's length. */
    nudgeVolume(s: PanelSource, delta: number): void;
    /** Per-member faders, when a group or zone has more than one speaker. */
    readonly memVol: Record<string, number>;
    dragMemberVolume(id: string, level: number): void;
    setMemberVolume(id: string, level: number): void;
    /** No memberId mutes the whole room; one mutes that speaker. */
    toggleMute(s: PanelSource, memberId?: string): void;

    // ── Transport & play modes ──
    togglePlay(s: PanelSource): void;
    skip(s: PanelSource, dir: "next" | "previous"): void;
    /** Wake a KEF speaker out of standby — the panel's own job, not the
     *  full view's: waking a speaker isn't configuring one. */
    wake(s: PanelSource): void;
    /** Pause every source that is playing, in one tap. */
    pauseAll(): void;
    toggleShuffle(): void;
    cycleRepeat(): void;
    toggleCrossfade(): void;
    /** "Play similar": keep the room going once the queue runs out (§15.5). */
    toggleAutoplay(): void;

    // ── KEF ──
    setKefSource(s: PanelSource, source: KEFSource): void;

    // ── Sleep timer (a Sonos group's, group-scoped like the play modes) ──
    /** Whole minutes left, counted down locally between reads; 0 for none. */
    readonly sleepMinutes: number;
    setSleep(minutes: number): void;

    // ── Queue (a Sonos group's — empty for anything else) ──
    readonly queue: SonosQueueItem[];
    readonly queueLoading: boolean;
    /** The first queued track after the one playing — the Up-next row. */
    readonly nextInQueue: SonosQueueItem | undefined;
    /** False when shuffle or repeat-one means queue order isn't play order,
     *  so nothing on the wall may claim to know what comes next. */
    readonly queueOrderKnown: boolean;
    jumpTo(track: number): void;
    removeQueued(track: number): void;
    /** Move a queued track one place up (-1) or down (+1). One place at a
     *  time, by tap: the app's drag is an imprecise aim at arm's length. */
    moveQueued(track: number, dir: -1 | 1): void;
    clearQueue(): void;
    /** Add a search result without disturbing what's playing. */
    enqueue(item: SpotifyItem, next: boolean): void;
    /** The last thing queued, for the player column's inline confirmation —
     *  a queued track changes nothing visible on its own. */
    readonly lastQueued: { title: string; next: boolean; at: number } | null;

    // ── Sonos favorites (a household list — radio, and what was starred) ──
    readonly favorites: SonosFavorite[];
    playFavorite(f: SonosFavorite): void;

    // ── Starting something ──
    playItem(item: SpotifyItem): Promise<void>;

    // ── What this room played before (HomeHub's own memory, per room) ──
    readonly history: MediaPlay[];
    /** True when the list is the household's rather than this room's own —
     *  the shelf says which, because a wall must never imply a room played
     *  something it didn't. */
    readonly historyHousehold: boolean;
    playFromHistory(p: MediaPlay): void;

    // ── Saving what's playing ──
    /** The login may write to the library. False hides the heart rather
     *  than offering a tap that will be refused. */
    readonly canSave: boolean;
    /** Whether what's playing is in the account's library. Meaningless
     *  unless the featured source has a trackURI. */
    readonly saved: boolean;
    toggleSaved(): void;

    // ── More like this ──
    /** Something is playing with an artist to seed from. */
    readonly canRadio: boolean;
    /** Queue more of what's on (Sonos), or play the first of it (anywhere
     *  else, which has no queue to fill). */
    startRadio(): void;
    /** What the last run added, for the in-place confirmation — queuing
     *  changes nothing visible on its own. */
    readonly lastRadio: { count: number; artist: string; at: number } | null;

    // ── Announcements ──
    /** Where an announcement would go and whether it would be spoken.
     *  Null while the read is out or when the server has no answer. */
    readonly announce: AnnounceStatus | null;
    sendAnnouncement(text: string): void;
    readonly lastAnnounce: { text: string; rooms: string[]; spoken: boolean; at: number } | null;

    // ── Grouping (Sonos-native only) ──
    /** The Sonos rooms that could join the featured one — every other
     *  reachable Sonos source. Empty unless a Sonos room is featured:
     *  nothing else groups natively, and a control that would be refused
     *  is worse than one that isn't there (§15.1). */
    readonly joinable: PanelSource[];
    /** There is something to say about grouping at all: a Sonos room with
     *  either somewhere to join from or more than one speaker to split. */
    readonly canGroup: boolean;
    /** Every speaker in src's group joins the featured group. */
    joinSource(src: PanelSource): void;
    /** Every joinable room at once — the wall's "play everywhere" tap. */
    joinAll(): void;
    /** Take the music with you: the featured room's group moves to dest and
     *  leaves where it was. Sonos-native, like every other grouping call. */
    moveTo(dest: PanelSource): void;
    /** Every non-coordinator member leaves the featured group. */
    ungroupFeatured(): void;
    /** One member steps out of the featured group. */
    leaveMember(memberId: string): void;

    refresh(): Promise<void>;
}

const POLL_MS = 15_000;
const LIVE_POLL_MS = 45_000;
/** Asleep on the ambient face: the screen shows a clock, so the speakers
 *  only need catching up with often enough that waking is current. */
const IDLE_POLL_MS = 120_000;

/** KEF inputs a skip means anything on. There is nothing to step through
 *  on the TV or the analog input — the speaker would simply refuse. */
const KEF_SKIPPABLE = new Set(["wifi", "bluetooth"]);

export interface PanelMusicOptions {
    /** The kid surface: Sonos only — never poll KEF or zones, never list one. */
    sonosOnly?: boolean;
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
    /** Wall-clock of the last successful Sonos read — the position deriveds
     *  advance from here so the rail creeps instead of stepping. */
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
        if (sonosRes.status === "fulfilled") {
            status = sonosRes.value;
            polledAt = Date.now();
        }
        if (kefRes?.status === "fulfilled") kef = kefRes.value;
        if (zoneRes?.status === "fulfilled") zones = zoneRes.value;
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
    // to a crawl while the ambient face is up. A pushed change still wakes
    // it immediately — this only changes the backstop's cadence.
    let idle = $state(false);

    $effect(() => {
        if (!session.isAdmin && !session.user?.kid) return;
        void refresh();
        const onVisible = () => {
            if (!document.hidden) void refresh();
        };
        const every = idle ? IDLE_POLL_MS : livePush ? LIVE_POLL_MS : POLL_MS;
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
    const byId = $derived(new SvelteMap(speakers.map((s) => [s.id, s])));

    /** Every speaker a zone already claims, so it can't also stand alone. */
    const claimed = $derived.by(() => {
        // Plain sets: rebuilt whole by this derivation, never mutated after.
        /* eslint-disable svelte/prefer-svelte-reactivity */
        const s = new Set<string>();
        const k = new Set<string>();
        /* eslint-enable svelte/prefer-svelte-reactivity */
        for (const z of zones ?? []) {
            for (const sp of z.speakers) {
                if (sp.missing) continue;
                (sp.vendor === "kef" ? k : s).add(sp.id);
            }
        }
        return { sonos: s, kef: k };
    });

    /** How many speakers this home has registered, answering or not — what
     *  tells "no speakers here" from "nothing answered this minute". */
    const registered = $derived(
        (status?.speakers.length ?? 0) + (kef?.speakers.length ?? 0) + (zones?.length ?? 0),
    );

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return byId.get(g.coordinator_id) ?? byId.get(g.member_ids[0]);
    }
    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids.map((id) => byId.get(id)?.name).filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    /** The zone member whose reading stands for the zone — the app's rule
     *  (zones.svelte.ts): the first that is playing something with a name. */
    function leadOf(z: MediaZone): MediaZoneSpeaker | undefined {
        const live = z.speakers.filter((sp) => !sp.missing);
        return (
            live.find((sp) => sp.state?.state === "playing" && sp.state.track?.title) ??
            live.find((sp) => sp.state?.track?.title) ??
            live.find((sp) => sp.state) ??
            live[0]
        );
    }

    function zoneSource(z: MediaZone): PanelSource {
        const live = z.speakers.filter((sp) => !sp.missing);
        const lead = leadOf(z);
        const withState = live.filter((sp) => sp.state);
        // The mean, because a zone-wide set writes one level to every
        // speaker: showing the loudest would jump the fader when touched.
        const volume = withState.length
            ? Math.round(
                  withState.reduce((n, sp) => n + (sp.state?.volume ?? 0), 0) / withState.length,
              )
            : 0;
        const at = lead?.state?.at ? Date.parse(lead.state.at) : NaN;
        return {
            key: "z:" + z.id,
            kind: "zone",
            id: z.id,
            title: z.name,
            playing: live.some((sp) => sp.state?.state === "playing"),
            standby: false,
            volume,
            // Audible if any one speaker is: muting all is what the zone
            // mute does, so anything less reads as unmuted.
            muted: withState.length > 0 && withState.every((sp) => sp.state?.muted),
            // On the stream route HomeHub is the Spotify device and the
            // speakers pull a live stream: `next` is a call they refuse.
            canSkip: z.route !== "stream",
            trackTitle: lead?.state?.track?.title,
            trackSub: [lead?.state?.track?.artist, lead?.state?.track?.album]
                .filter(Boolean)
                .join(" · "),
            trackArtist: lead?.state?.track?.artist,
            art: lead?.state?.track?.art_uri,
            members:
                live.length > 1
                    ? live.map((sp) => ({
                          id: sp.id,
                          name: sp.name,
                          vendor: sp.vendor,
                          volume: sp.state?.volume ?? 0,
                          muted: !!sp.state?.muted,
                          coordinator: sp.id === lead?.id,
                      }))
                    : undefined,
            positionMs: lead?.state?.position_ms,
            durationMs: lead?.state?.duration_ms,
            readAt: Number.isNaN(at) ? undefined : at,
            zone: z,
            route: z.route,
        };
    }

    const sources = $derived.by((): PanelSource[] => {
        const out: PanelSource[] = [];
        for (const z of zones ?? []) {
            if (z.speakers.some((sp) => !sp.missing)) out.push(zoneSource(z));
        }
        for (const g of status?.groups ?? []) {
            if (g.member_ids.some((id) => claimed.sonos.has(id))) continue;
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
                // A group is muted only when every speaker in it is: one
                // audible speaker means the room is audible. Reading the
                // coordinator's own flag here made the icon disagree with
                // what the button then did.
                muted: members.length > 0 && members.every((x) => !!x.state?.muted),
                canSkip: true,
                trackTitle: st?.track?.title,
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                trackArtist: st?.track?.artist,
                trackURI: st?.track?.spotify_uri,
                art: st?.track?.art_uri,
                members: members.map((x) => ({
                    id: x.id,
                    name: x.name,
                    vendor: "sonos" as const,
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
            if (!sp.reachable || claimed.kef.has(sp.id)) continue;
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
                // A skip reaches something on a network source; on the TV or
                // the analog input there is nothing to step through.
                canSkip: !!st && st.powered_on && KEF_SKIPPABLE.has(st.source),
                trackTitle:
                    st?.track?.title ??
                    (st?.playing && st.source ? `${kefSourceLabel(st.source)} input` : undefined),
                trackSub: [st?.track?.artist, st?.track?.album].filter(Boolean).join(" · "),
                trackArtist: st?.track?.artist,
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
        return memberVendor(id) === "kef"
            ? api.kefSetVolume(id, level)
            : api.sonosSetVolume(id, level);
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

    // ── Sleep timer ──────────────────────────────────────────────────────
    // Group-scoped like the play modes, and the one setting a wall panel
    // has more claim to than the phone does: "stop in half an hour" is
    // asked at the light switch on the way to bed. The full settings read
    // is a handful of SOAP calls, so it runs once per featured room rather
    // than on the poll, and the minutes left are counted down locally
    // between reads instead of being re-read to stay honest.
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
    const sleepMinutes = $derived.by(() => {
        void clock.beat; // the countdown is the display, not another read
        if (!sleep.mins || !sleep.at) return 0;
        return Math.max(0, sleep.mins - Math.floor((Date.now() - sleep.at) / 60_000));
    });
    function setSleep(minutes: number) {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        void run(
            "sleep:" + f.id,
            async () => {
                await api.sonosUpdateSettings(f.id, { sleep_minutes: minutes });
                sleep = { mins: minutes, at: Date.now() };
            },
            "Couldn't set the sleep timer",
        );
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

    /** The destination key the media layer files plays under. */
    function roomKey(s: PanelSource | undefined): string {
        if (!s) return "";
        return (s.kind === "sonos" ? "sonos:" : s.kind === "kef" ? "kef:" : "zone:") + s.id;
    }

    async function loadHistory(key: string) {
        const mine = ++historySeq;
        try {
            const h = await api.mediaHistory(key, 12);
            if (mine !== historySeq) return;
            history = h.plays;
            historyHousehold = h.household;
        } catch {
            if (mine !== historySeq) return;
            history = [];
            historyHousehold = false;
        }
    }

    $effect(() => {
        const key = roomKey(featured);
        if (key === historyFor) return;
        historyFor = key;
        historySeq++;
        history = [];
        historyHousehold = false;
        if (key) void loadHistory(key);
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
                }).then(() => loadHistory(roomKey(s))),
            "Couldn't play that again",
        );
    }

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
            if (busy["save:" + uri] === false && featured?.trackURI === uri) {
                // run() has already toasted a failure; re-read so the heart
                // tells the truth rather than what was hoped for.
                void api
                    .spotifySaved(uri)
                    .then((r) => {
                        if (featured?.trackURI === uri) saved = r.saved;
                    })
                    .catch(() => {});
            }
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
                    // In order, each after the last, so the run plays as a
                    // set rather than in reverse.
                    for (const [i, item] of items.entries()) {
                        await api.sonosQueueAdd(s.id, {
                            service: "Spotify",
                            uri: item.uri,
                            title: item.name,
                            next: i === 0,
                        });
                    }
                    await loadQueue(s.id);
                    lastRadio = { count: items.length, artist, at: Date.now() };
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

    void api
        .announceStatus()
        .then((st) => {
            announce = st;
        })
        .catch(() => {
            announce = null;
        });

    function sendAnnouncement(text: string) {
        void run(
            "announce",
            async () => {
                const res = await api.announce(text);
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
            async () => {
                for (const id of members) await api.sonosJoin(id, f.id);
            },
            "Grouping failed",
            () => {
                selected = f.key; // the group stays featured through the reshuffle
            },
        );
    }

    /** Everything at once. Sequential, like a single join: a household that
     *  is handed four `SetAVTransportURI`s in the same instant re-elects its
     *  coordinators mid-flight and lands with a speaker or two left out. */
    function joinAll() {
        const f = featured;
        if (!f || f.kind !== "sonos") return;
        const ids = joinable.flatMap((s) => (s.members ?? []).map((m) => m.id));
        if (!ids.length) return;
        void run(
            "joinall",
            async () => {
                for (const id of ids) await api.sonosJoin(id, f.id);
            },
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
     *  Order matters and is the whole reason this is a function: the
     *  destination joins the group *first*, so the queue and the stream are
     *  handed over while the old room is still coordinating, and only then
     *  does the old room step out. Doing it the other way round stops the
     *  music between the two calls. */
    function moveTo(dest: PanelSource) {
        const f = featured;
        if (!f || f.kind !== "sonos" || dest.kind !== "sonos" || dest.key === f.key) return;
        const leaving = (f.members ?? []).map((m) => m.id);
        const arriving = (dest.members ?? []).map((m) => m.id);
        if (!leaving.length || !arriving.length) return;
        void run(
            "move:" + dest.id,
            async () => {
                for (const id of arriving) await api.sonosJoin(id, f.id);
                for (const id of leaving) await api.sonosLeave(id);
            },
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
        get sleepMinutes() {
            return sleepMinutes;
        },
        setSleep,
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
