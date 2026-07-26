<script lang="ts">
    import { onMount, onDestroy, tick as flushDOM } from "svelte";
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Icon from "../components/Icon.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import SpeakerModal from "../modals/SpeakerModal.svelte";
    import SonosSpeakerDetail from "./SonosSpeakerDetail.svelte";
    import KEFSpeakerDetail from "./KEFSpeakerDetail.svelte";
    import SonosEventsModal from "../modals/SonosEventsModal.svelte";
    import LiveStatusChip from "../components/LiveStatusChip.svelte";
    import { api } from "../lib/api";
    import { toasts, route, bottomBar } from "../lib/stores.svelte";
    import { onLive } from "../lib/live";
    import { openModal } from "../lib/modal.svelte";
    import { copyText } from "../lib/clipboard";
    import { fly, fade, scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur, sheet } from "../lib/motion";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
    import * as sheetRun from "../lib/sheet-run";
    import type { SheetRun } from "../lib/sheet-run";
    import { kefSourceLabel, KEF_SOURCES } from "../lib/kef";
    import { secs, trimClock, fmtSecs, toClock } from "../lib/music/time";
    import { settleScroll, restoreScroll, toTop } from "../lib/music/scroll";
    import { clock } from "../lib/music/clock.svelte";
    import { createBusy } from "../lib/music/busy.svelte";
    import type {
        SonosStatus, SonosSpeakerView, SonosGroupView, SonosFavorite,
        SonosQueueItem, SonosRepeat,
        KEFStatus, KEFSpeakerView, KEFSource,
        SpotifyStatus, SpotifyItem, SpotifyResults,
    } from "../lib/types";

    let status = $state<SonosStatus | null>(null);
    let loaded = $state(false);
    let favorites = $state<SonosFavorite[]>([]);
    let favsLoaded = $state(false);

    // Volume the user just set, keyed by speaker id. The 5s poll must not
    // yank the slider back to a stale value while the command is still
    // propagating, so recent local sets win over polled state briefly.
    let volOverride: Record<string, { v: number; at: number }> = {};
    let localVol = $state<Record<string, number>>({});
    let groupVol = $state<Record<string, number>>({});

    // Actions in flight (play/pause/join/…) keyed by "<action>:<id>". One map
    // for both bridges and the view's own calls; the key namespace is what
    // keeps a dozen cards' transports from disabling each other.
    const busy = createBusy();

    // A play/pause round-trip plus the refresh behind it takes long enough
    // that an un-flipped button reads as a dropped tap. The new state is
    // applied locally and wins until the poll reports it — the same trick
    // volOverride plays for the sliders. Rolled back if the call fails.
    let playOverride = $state<Record<string, { playing: boolean; at: number }>>({});

    // Wall-clock of the last successful poll. The player advances the track
    // position from here so the scrubber moves every second instead of
    // jumping every five.
    let polledAt = $state(0);

    const speakerById = $derived(new Map((status?.speakers ?? []).map((s) => [s.id, s])));
    const groups = $derived(status?.groups ?? []);
    // Registered speakers the live topology doesn't mention — offline or on
    // another network. Shown separately so they stay visible and editable.
    const offline = $derived(
        (status?.speakers ?? []).filter((s) => !groups.some((g) => g.member_ids.includes(s.id))),
    );
    const reachable = $derived((status?.speakers ?? []).filter((s) => s.reachable));
    const playingGroups = $derived(groups.filter((g) => isPlaying(g)));
    const playingCount = $derived(playingGroups.length);
    // Multi-speaker zones render inside a dashed enclosure in the room grid;
    // everything reachable that isn't in one shows as a loose puck.
    const multiGroups = $derived(groups.filter((g) => g.member_ids.length > 1));
    const soloSpeakers = $derived(
        reachable.filter((s) => !multiGroups.some((g) => g.member_ids.includes(s.id))),
    );

    function coordinatorOf(g: SonosGroupView): SonosSpeakerView | undefined {
        return speakerById.get(g.coordinator_id) ?? speakerById.get(g.member_ids[0]);
    }

    // Shuffle / repeat / crossfade / queue length belong to the group, so the
    // backend only reports them on the coordinator's view.
    function groupStateOf(g: SonosGroupView) {
        return coordinatorOf(g)?.group_state;
    }

    function groupTitle(g: SonosGroupView): string {
        const names = g.member_ids
            .map((id) => speakerById.get(id)?.name)
            .filter((n): n is string => !!n);
        if (names.length <= 2) return names.join(" + ");
        return `${names[0]} + ${names.length - 1} more`;
    }

    function groupOfSpeaker(id: string): SonosGroupView | undefined {
        return groups.find((g) => g.member_ids.includes(id));
    }
    // The one place "is this playing?" is answered, so a tapped play/pause
    // flips every waveform, card and icon at once instead of waiting out the
    // poll on each surface separately.
    function isPlaying(g: SonosGroupView | undefined): boolean {
        if (!g) return false;
        const ov = playOverride[g.coordinator_id];
        return ov ? ov.playing : !!coordinatorOf(g)?.state?.playing;
    }
    function speakerPlaying(id: string): boolean {
        return isPlaying(groupOfSpeaker(id));
    }
    function speakerNowLine(id: string): string {
        const g = groupOfSpeaker(id);
        const st = g && coordinatorOf(g)?.state;
        if (!st?.track?.title) return "Idle";
        return isPlaying(g) ? st.track.title : `Paused · ${st.track.title}`;
    }

    // ── Track position, for any group ────────────────────────────────────
    // Positions are polled every 5s; every surface that shows progress
    // extrapolates from the last reading so the bar creeps forward once a
    // second instead of stepping five at a time.
    function posOf(g: SonosGroupView): number {
        void clock.beat; // re-derive once a second
        const st = coordinatorOf(g)?.state;
        const base = secs(st?.position);
        if (!isPlaying(g) || !polledAt) return base;
        const total = secs(st?.duration);
        const advanced = base + (Date.now() - polledAt) / 1000;
        return total ? Math.min(total, advanced) : advanced;
    }
    // 0–1, or 0 when the source reports no duration (radio, line-in, TV) —
    // those get no progress line rather than a fake one.
    function progressOf(g: SonosGroupView): number {
        const total = secs(coordinatorOf(g)?.state?.duration);
        return total > 0 ? Math.min(1, posOf(g) / total) : 0;
    }

    // ── Data loading ─────────────────────────────────────────────────────
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let stopLive: (() => void) | undefined;
    let statusSeq = 0;

    async function refresh() {
        const seq = ++statusSeq;
        try {
            const st = await api.sonosStatus();
            if (seq !== statusSeq) return;
            status = st;
            polledAt = Date.now();
            const now = polledAt;
            // Retire an optimistic play/pause as soon as the speaker agrees
            // with it — or after 6s, so a command the speaker quietly ignored
            // can't leave a button lying about its state forever.
            for (const [id, ov] of Object.entries(playOverride)) {
                const sp = st.speakers.find((s) => s.id === id);
                if (!sp || sp.state?.playing === ov.playing || now - ov.at > 6000) {
                    delete playOverride[id];
                }
            }
            for (const sp of st.speakers) {
                const ov = volOverride[sp.id];
                if (ov && now - ov.at < 3000) continue; // user just moved it
                if (sp.state) localVol[sp.id] = sp.state.volume;
            }
            for (const g of st.groups) {
                // Group volume isn't reported by the status poll; seed the
                // slider with the members' average unless recently set.
                const key = "g:" + g.coordinator_id;
                const ov = volOverride[key];
                if (ov && now - ov.at < 3000) continue;
                const vols = g.member_ids
                    .map((id) => st.speakers.find((s) => s.id === id)?.state?.volume)
                    .filter((v): v is number => v !== undefined);
                if (vols.length) {
                    groupVol[g.coordinator_id] = Math.round(vols.reduce((a, b) => a + b, 0) / vols.length);
                }
            }
            // Picking the destination is the destinations effect's job — it
            // spans both bridges, so it can't live inside one bridge's poll.
            if (!favsLoaded && st.speakers.some((s) => s.reachable)) {
                void loadFavorites(st.speakers.find((s) => s.reachable)!.id);
            }
        } catch (e) {
            if (seq !== statusSeq) return;
            if (!loaded) toasts.error("Couldn't reach Sonos", (e as Error).message);
        } finally {
            if (seq === statusSeq) loaded = true;
        }
    }

    async function loadFavorites(speakerId: string) {
        favsLoaded = true;
        try {
            favorites = await api.sonosFavorites(speakerId);
        } catch {
            favsLoaded = false; // retry on a later poll
        }
    }

    // ── KEF ──────────────────────────────────────────────────────────────
    // The second bridge, alongside Sonos rather than folded into it: a KEF
    // speaker is one standalone stereo pair with an input selector, where a
    // Sonos household is zones that group and share a queue. Nothing above
    // this line applies to it — no groups, no queue, no favorites — so it
    // gets its own poll and its own surfaces instead of being bent into the
    // group model. See internal/kef.
    let kef = $state<KEFStatus | null>(null);
    let kefSeq = 0;

    // "Transport is optimistic" (DESIGN.md §15) is a rule about the whole
    // module, not about Sonos — this is playOverride's twin for the second
    // bridge. Without it a tapped KEF play/pause sat unchanged until the next
    // read landed, which on a KEF is up to a poll away, and the card, its icon
    // and the zone chip all disagreed with the finger that just pressed them.
    let kefPlayOverride = $state<Record<string, { playing: boolean; at: number }>>({});

    /** The one place "is this KEF speaker playing?" is answered. */
    function kefIsPlaying(sp: KEFSpeakerView): boolean {
        const ov = kefPlayOverride[sp.id];
        return ov ? ov.playing : !!sp.state?.playing;
    }

    async function refreshKEF() {
        const seq = ++kefSeq;
        try {
            const st = await api.kefStatus();
            if (seq !== kefSeq) return;
            kef = st;
            // Retire an optimistic flip once the speaker agrees with it — or
            // after 6s, so a command it quietly ignored can't leave a card
            // claiming to play forever. Same contract as the Sonos poll's.
            const now = Date.now();
            for (const [id, ov] of Object.entries(kefPlayOverride)) {
                const sp = st.speakers.find((s) => s.id === id);
                if (!sp || !!sp.state?.playing === ov.playing || now - ov.at > 6000) {
                    delete kefPlayOverride[id];
                }
            }
        } catch {
            // A home with no KEF speakers must not see an error every poll;
            // an empty list is indistinguishable from a failed one here, and
            // the Speakers screen is where a broken registration shows up.
            if (seq === kefSeq && !kef) kef = { speakers: [] };
        }
    }

    /** Registered KEF speakers, reachable ones first, then by name. */
    const kefSpeakers = $derived.by(() => {
        const list = [...(kef?.speakers ?? [])];
        list.sort((a, b) => {
            if (a.reachable !== b.reachable) return a.reachable ? -1 : 1;
            return a.name.localeCompare(b.name);
        });
        return list;
    });
    const kefReachable = $derived(kefSpeakers.filter((s) => s.reachable));
    const kefPlaying = $derived(kefSpeakers.filter((s) => kefIsPlaying(s)));
    /** Speakers that answered, across both bridges — "ready" on the Home head. */
    const readyCount = $derived(reachable.length + kefReachable.length);
    /** Every registered speaker across both bridges — what "is this view empty" means. */
    const totalSpeakers = $derived((status?.speakers.length ?? 0) + kefSpeakers.length);

    function kefNowLine(sp: KEFSpeakerView): string {
        if (!sp.state?.powered_on) return "In standby";
        const t = sp.state.track;
        if (t?.title) return t.title;
        return sp.state.source ? `${kefSourceLabel(sp.state.source)} input` : "Idle";
    }
    function kefSubLine(sp: KEFSpeakerView): string {
        const t = sp.state?.track;
        return [t?.artist, t?.album].filter(Boolean).join(" · ");
    }
    /** How far through the track, 0–1. Sources with no duration get no line. */
    function kefProgress(sp: KEFSpeakerView): number {
        void clock.beat; // re-derive once a second, exactly as posOf does for Sonos
        const total = sp.state?.duration_ms ?? 0;
        if (total <= 0) return 0;
        // Extrapolated from when the reading was taken, like the Sonos
        // scrubber, so the line advances between polls instead of stepping.
        // Without the tick above this only recomputed when a poll replaced
        // the object — every 20s once Sonos push is up — so the hairline sat
        // dead beside a Sonos one creeping every second.
        const base = sp.state?.position_ms ?? 0;
        const since = kefIsPlaying(sp) && sp.read_at ? Date.now() - sp.read_at : 0;
        return Math.max(0, Math.min(1, (base + since) / total));
    }

    /** Where the track has got to, extrapolated the same way the bar is. */
    function kefPosMs(sp: KEFSpeakerView): number {
        void clock.beat; // re-read every second so the clock counts rather than jumps
        const total = sp.state?.duration_ms ?? 0;
        const base = sp.state?.position_ms ?? 0;
        const since = kefIsPlaying(sp) && sp.read_at ? Date.now() - sp.read_at : 0;
        return total > 0 ? Math.min(total, base + since) : base + since;
    }

    async function kefTogglePlay(sp: KEFSpeakerView) {
        const next = !kefIsPlaying(sp);
        await busy.claim("kefplay:" + sp.id, async () => {
            kefPlayOverride[sp.id] = { playing: next, at: Date.now() };
            try {
                await (next ? api.kefPlay(sp.id) : api.kefPause(sp.id));
                await refreshKEF();
            } catch (e) {
                delete kefPlayOverride[sp.id];
                toasts.error(
                    next ? "Couldn't start playback" : "Couldn't pause",
                    (e as Error).message,
                );
            }
        });
    }
    function kefSkip(sp: KEFSpeakerView, dir: "next" | "previous") {
        void kefRun(
            `kef${dir}:` + sp.id,
            () => (dir === "next" ? api.kefNext(sp.id) : api.kefPrevious(sp.id)),
            "Skip failed",
        );
    }
    /** Volume the slider shows: the live drag if there is one, else the read. */
    let kefVol = $state<Record<string, number>>({});
    let kefVolAt: Record<string, number> = {};
    function kefShownVol(sp: KEFSpeakerView): number {
        const ov = kefVol[sp.id];
        const fresh = ov !== undefined && Date.now() - (kefVolAt[sp.id] ?? 0) < 4000;
        return fresh ? ov : (sp.state?.volume ?? 0);
    }
    function kefSetVolume(sp: KEFSpeakerView, v: number) {
        const level = clampVol(v);
        kefVol[sp.id] = level;
        kefVolAt[sp.id] = Date.now();
        void kefRun("kefvol:" + sp.id, () => api.kefSetVolume(sp.id, level), "Volume failed");
    }
    function kefToggleMute(sp: KEFSpeakerView) {
        const next = !sp.state?.muted;
        void kefRun("kefmute:" + sp.id, () => api.kefSetMute(sp.id, next), "Mute failed");
    }
    function kefSetSource(sp: KEFSpeakerView, source: KEFSource) {
        if (sp.state?.source === source) return;
        void kefRun(
            "kefsrc:" + sp.id,
            () => api.kefSetSource(sp.id, source),
            "Couldn't switch input",
        );
    }

    // ── Where playback lands ─────────────────────────────────────────────
    // One destination for the whole module (DESIGN.md §15, "one visible
    // destination"), and it spans both bridges — so it carries a kind rather
    // than being a bare id. A Sonos zone is started through its coordinator's
    // queue; a KEF speaker through Spotify Connect, because its own API can
    // play and pause but has nothing to be handed. The two are not
    // interchangeable, which is exactly why the destination says which it is.
    type Dest = { kind: "sonos" | "kef"; id: string };
    let dest = $state<Dest | null>(null);
    /** The Sonos coordinator, when the destination is one. Favorites and the
     *  queue exist only on that side, so they read this and not `dest`. */
    const sonosTarget = $derived(dest?.kind === "sonos" ? dest.id : null);
    /** The KEF speaker, when the destination is one. */
    const kefTarget = $derived(dest?.kind === "kef" ? dest.id : null);
    const kefTargetSpeaker = $derived(
        kefTarget ? (kefSpeakers.find((s) => s.id === kefTarget) ?? null) : null,
    );
    /** Everywhere music can be sent, in the order the destination row lists
     *  them. Unreachable KEF speakers are left out: they have no Connect
     *  device while they're off the network. */
    const destinations = $derived<Dest[]>([
        ...groups.map((g) => ({ kind: "sonos" as const, id: g.coordinator_id })),
        ...kefReachable.map((s) => ({ kind: "kef" as const, id: s.id })),
    ]);
    function destName(d: Dest): string {
        if (d.kind === "kef") return kefSpeakers.find((s) => s.id === d.id)?.name ?? "Speaker";
        const g = groups.find((x) => x.coordinator_id === d.id);
        return g ? groupTitle(g) : "Zone";
    }
    const isDest = (d: Dest) => dest?.kind === d.kind && dest.id === d.id;
    /** What to call the destination in a toast or the one-destination label. */
    const destLabel = $derived(dest ? destName(dest) : "");
    /** Stable key for per-destination state (the search history). */
    const destKey = $derived(dest ? `${dest.kind}:${dest.id}` : null);

    // Keep the destination pointing at something that exists, and prefer a
    // room that is already playing — "play this too" almost always means the
    // room the music is coming out of. Runs on both bridges' polls, so a KEF
    // speaker that answers first is a perfectly good default in a house
    // without Sonos.
    $effect(() => {
        const list = destinations;
        if (dest && list.some((d) => isDest(d))) return;
        const livePick =
            playingGroups[0] && { kind: "sonos" as const, id: playingGroups[0].coordinator_id };
        const kefPick = kefPlaying[0] && { kind: "kef" as const, id: kefPlaying[0].id };
        dest = livePick ?? kefPick ?? list[0] ?? null;
    });

    onMount(() => {
        void refresh();
        void refreshKEF();
        // Speaker changes arrive pushed — someone pressing play on the
        // speaker itself lands here in well under a second instead of
        // whenever the next poll happens to run.
        stopLive = onLive("music", () => {
            void refresh();
            // The KEF poller publishes on the same topic when a speaker
            // actually changes, so this catches both bridges.
            void refreshKEF();
        });
    });
    onDestroy(() => {
        clearInterval(pollTimer);
        stopLive?.();
        clearTimeout(announceTimer);
        for (const t of followUps) clearTimeout(t);
        followUps.clear();
        endPuckDrag(); // takes the document-level touchmove block with it
        // The body-scroll lock is the sheet effect's, and its teardown runs on
        // unmount — releasing it here as well would decrement it twice.
    });

    // The poll is the backstop, not the mechanism. When the backend has the
    // speakers' notifications it only has to catch what those don't carry —
    // the track position, which the player extrapolates between reads
    // anyway — so it can run four times slower. When it doesn't, this is
    // the only thing keeping the view current, and stays at the old rate.
    //
    // Derived rather than read straight off `status`, which every refresh
    // reassigns: the interval must be rebuilt when the answer changes, not
    // every time a poll lands.
    const livePush = $derived(!!status?.live);
    $effect(() => {
        clearInterval(pollTimer);
        pollTimer = setInterval(() => {
            void refresh();
            // KEF has no push to subscribe to, but the backend polls the
            // speakers once for the whole process and pushes `music` on a
            // real change — so this is a backstop for both, not the
            // mechanism, and rides the same interval.
            void refreshKEF();
        }, livePush ? 20_000 : 5_000);
        return () => clearInterval(pollTimer);
    });

    // ── Actions ──────────────────────────────────────────────────────────
    // A KEF call changes nothing on the Sonos side and vice versa, so each
    // action re-reads only the bridge it touched. Which one that is used to be
    // guessed from a "kef" prefix on the key; naming it here means a key that
    // doesn't follow the convention can't quietly re-read the wrong speakers.
    const run = (key: string, fn: () => Promise<unknown>, errTitle: string) =>
        busy.run(key, fn, errTitle, refresh);
    const kefRun = (key: string, fn: () => Promise<unknown>, errTitle: string) =>
        busy.run(key, fn, errTitle, refreshKEF);

    async function togglePlay(g: SonosGroupView) {
        const c = coordinatorOf(g);
        if (!c) return;
        const next = !isPlaying(g);
        await busy.claim("play:" + c.id, async () => {
            playOverride[c.id] = { playing: next, at: Date.now() };
            try {
                await (next ? api.sonosPlay(c.id) : api.sonosPause(c.id));
                await refresh();
            } catch (e) {
                delete playOverride[c.id]; // the speaker never took it — roll back
                toasts.error(next ? "Play failed" : "Pause failed", (e as Error).message);
            }
        });
    }

    function skip(g: SonosGroupView, dir: "next" | "previous") {
        const c = coordinatorOf(g);
        if (!c) return;
        void run(dir + ":" + c.id, () => (dir === "next" ? api.sonosNext(c.id) : api.sonosPrevious(c.id)), "Skip failed");
    }

    // Sliders update the local value live (oninput) and send on release
    // (onchange), so dragging doesn't flood the speaker with SOAP calls.
    function setVolume(id: string, v: number) {
        localVol[id] = v;
        volOverride[id] = { v, at: Date.now() };
        api.sonosSetVolume(id, v).catch((e) => toasts.error("Volume failed", (e as Error).message));
    }

    function setGroupVolume(coordinatorId: string, v: number) {
        groupVol[coordinatorId] = v;
        volOverride["g:" + coordinatorId] = { v, at: Date.now() };
        api.sonosSetVolume(coordinatorId, v, true).catch((e) => toasts.error("Volume failed", (e as Error).message));
    }

    function toggleMute(sp: SonosSpeakerView) {
        void run("mute:" + sp.id, () => api.sonosSetMute(sp.id, !sp.state?.muted), "Mute failed");
    }

    // Keyboard mute in the player acts on the whole zone: muting only the
    // coordinator of a three-room group would look like the key did nothing.
    function toggleMuteGroup(g: SonosGroupView) {
        const members = g.member_ids
            .map((id) => speakerById.get(id))
            .filter((s): s is SonosSpeakerView => !!s);
        if (!members.length) return;
        const next = !members.some((s) => s.state?.muted);
        void run(
            "mute:" + g.coordinator_id,
            () => Promise.all(members.map((s) => api.sonosSetMute(s.id, next))),
            "Mute failed",
        );
    }

    // Volume steps, used by the player's arrow-key shortcuts. Grouped zones
    // move together (the same "All rooms" fader the sheet shows); a lone
    // speaker moves on its own.
    function nudgeVolume(g: SonosGroupView, delta: number) {
        const grouped = g.member_ids.length > 1;
        if (grouped) {
            const cur = groupVol[g.coordinator_id] ?? coordinatorOf(g)?.state?.volume ?? 0;
            setGroupVolume(g.coordinator_id, clampVol(cur + delta));
            return;
        }
        const sp = coordinatorOf(g);
        if (!sp) return;
        const cur = localVol[sp.id] ?? sp.state?.volume ?? 0;
        setVolume(sp.id, clampVol(cur + delta));
    }
    const clampVol = (v: number) => Math.max(0, Math.min(100, Math.round(v)));

    function join(speakerId: string, g: SonosGroupView) {
        void run("join:" + speakerId, () => api.sonosJoin(speakerId, g.coordinator_id), "Grouping failed");
    }

    function leave(speakerId: string) {
        void run("leave:" + speakerId, () => api.sonosLeave(speakerId), "Ungrouping failed");
    }

    // Starting playback is invisible until the next poll lands, so every
    // "play this" path confirms in words — `where` names the room, which is
    // the one thing a tap can't show. The refresh is the destination's own
    // bridge: a KEF play would never show up in the Sonos poll.
    async function startPlayback(
        key: string,
        fn: () => Promise<unknown>,
        what: string,
        where: string,
        bridge: "sonos" | "kef" = "sonos",
    ) {
        await busy.claim(key, async () => {
            try {
                await fn();
                await (bridge === "kef" ? refreshKEF() : refresh());
                // A KEF play answers as soon as *Spotify* accepted it — the
                // audio then goes out to the cloud and comes back to the
                // speaker, so the read above still says "stopped" and the
                // toast promised music no card was showing yet. The backend
                // re-reads at 0.6s and 3s and publishes `music` when it finds
                // the change; these are the backstop for an install where that
                // push isn't getting through.
                if (bridge === "kef") {
                    for (const ms of [1200, 4000]) followUp(ms, refreshKEF);
                }
                toasts.success("Playing", [what, where].filter(Boolean).join(" · "));
            } catch (e) {
                toasts.error("Couldn't play", (e as Error).message);
            }
        });
    }

    /** A delayed re-read that doesn't outlive the view. */
    let followUps = new Set<ReturnType<typeof setTimeout>>();
    function followUp(ms: number, fn: () => void) {
        const t = setTimeout(() => {
            followUps.delete(t);
            fn();
        }, ms);
        followUps.add(t);
    }

    // Favorites play on the chip-selected destination, except inside the
    // player sheet, where the group being viewed is the obvious one. They are
    // a Sonos household list, so the destination here is always a coordinator.
    function playFavorite(f: SonosFavorite, target: string | null = sonosTarget) {
        if (!target) return;
        const g = groups.find((x) => x.coordinator_id === target);
        void startPlayback("fav:" + f.id, () => api.sonosPlayFavorite(target, f), f.title,
            g ? groupTitle(g) : "");
    }

    // ── Screens and sheets ───────────────────────────────────────────────
    // Home is the only fixed screen. The pill subnav is gone: Music's header
    // now behaves like every other header in HomeHub, with nothing riding
    // below it but content (DESIGN.md §15).
    //
    // Search and Zones open as sheets over Home, the same gesture as the
    // player. Speakers can't — its rows open a speaker's settings one level
    // further, and a sheet must never open another sheet — so it is a real
    // screen with a back chip.
    //
    // Zones and Speakers look adjacent but answer different questions: Zones
    // is about zones (what plays together), Speakers is about the devices
    // (what each one is and how it is configured).
    type Screen = "home" | "speakers";
    let screen = $state<Screen>("home");

    /**
     * Where Home was left. Pushing to Speakers is a navigation, so it starts
     * at the top — but coming *back* has to land where you were, or the row
     * you tapped (which lives at the bottom of Home) is off screen the moment
     * you return from it.
     */
    let homeScrollY = 0;

    function openSpeakers() {
        hideSheet(); // a screen replaces the sheet rather than stacking under it
        if (screen === "home") homeScrollY = window.scrollY;
        screen = "speakers";
        toTop();
    }
    function leaveSpeakers() {
        screen = "home";
        detailId = null;
        kefDetailId = null;
        restoreScroll(homeScrollY);
    }
    // Only ever one sheet at a time. Sheets *swap* — they never stack — so
    // there is only ever one scrim, one Escape, one thing to swipe away. The
    // rule and its invariants live in `lib/sheet-run.ts`, with tests, because
    // the swap is subtle enough to break by accident from in here.
    type Sheet = "player" | "search" | "zones";
    let sheets = $state<SheetRun<Sheet>>(sheetRun.closed());

    const openSheet = $derived(sheets.open);
    const searchOpen = $derived(sheets.open === "search");
    const zonesOpen = $derived(sheets.open === "zones");
    const sheetUp = $derived(sheetRun.isUp(sheets));

    // ── Back closes one level ────────────────────────────────────────────
    // Music stacks up to two levels inside a single route — the Speakers
    // screen, and a sheet (or a sheet swapped over a sheet). Without this,
    // an Android back gesture or a browser back button skips all of it and
    // leaves the module entirely, which on a phone reads as the app losing
    // your place rather than as navigation.
    //
    // One history entry is held for the whole time Music is deeper than its
    // home screen, and re-taken after each step back while depth remains. So
    // back always means "up one", exactly like Escape and the back chip, and
    // the entry is handed straight back when the last level closes by any
    // other route.
    const navDepth = $derived(
        (screen === "speakers" ? 1 : 0) + (sheets.open ? (sheets.under ? 2 : 1) : 0),
    );
    let holdsEntry = false;

    $effect(() => {
        if (navDepth > 0) {
            if (!holdsEntry) {
                history.pushState({ musicNav: true }, "");
                holdsEntry = true;
            }
        } else if (holdsEntry) {
            // Back at Home by some other means (Escape, a chip, a swipe) —
            // give the entry back so the next press leaves Music for real.
            holdsEntry = false;
            history.back();
        }
    });

    function onPopState() {
        if (navDepth === 0) return; // not our entry — a real route change
        holdsEntry = false; // the browser consumed it; the effect re-takes it
        if (sheetUp) dropSheet();
        else if (screen === "speakers") leaveSpeakers();
    }

    // The body-scroll lock keys on *whether* a sheet is up, never on which —
    // so a swap doesn't release and retake it, which on iOS would unpin and
    // re-pin the body for a frame. The teardown also runs on unmount, so a
    // navigation away with a sheet open can't strand the lock.
    $effect(() => {
        if (!sheetUp) return;
        lockBodyScroll();
        return unlockBodyScroll;
    });

    /** Gesture state a sheet must not inherit from the one before it. */
    function resetSheetGesture() {
        dragY = 0;
        dragging = false;
        dismissing = false;
        pendingBody = false;
    }
    /**
     * How far each sheet was scrolled when it handed over. A swap unmounts
     * the sheet underneath, so without this, opening a room from halfway down
     * Zones and dismissing the player would put Zones back at the top —
     * "puts it back" has to mean where you left it, not merely which one.
     */
    const sheetScroll: Partial<Record<Sheet, number>> = {};

    /** Note where the sheet on screen had got to, before it hands over. */
    function rememberSheetScroll() {
        if (sheets.open) sheetScroll[sheets.open] = scrollEl?.scrollTop ?? 0;
    }
    /** Put a returning sheet back where it was — `scrollEl` only points at
     *  the incoming sheet after the flush, and only settles a frame later. */
    function restoreSheetScroll(s: Sheet) {
        settleScroll(() => scrollEl, sheetScroll[s] ?? 0);
    }

    /** Raise a sheet over the page. Anything up is replaced, not remembered. */
    function showSheet(s: Sheet) {
        rememberSheetScroll();
        sheets = sheetRun.raise(sheets, s);
        sheetScroll[s] = 0; // raised fresh, not returned to
        resetSheetGesture();
    }
    /** Close the open sheet — back to the one it was raised over, if any. */
    function dropSheet() {
        if (!sheetUp) return;
        const back = sheetRun.dismiss(sheets);
        sheets = back;
        if (back.open) restoreSheetScroll(back.open);
        if (back.open !== "player") { playerGroupId = null; playerKefId = null; }
        queuePane = false;
        scrubSec = null;
        endPuckDrag();
        grabId = null;
        // A drag-out close keeps its offset until the sheet is gone — zeroing
        // it here would snap the sheet back up for one frame.
        if (back.open !== null || !dismissing) resetSheetGesture();
        else pendingBody = false;
    }
    /** Leave sheets entirely, whatever is up and whatever is under it. */
    function hideSheet() {
        if (!sheetUp) return;
        sheets = sheetRun.closeAll(sheets);
        playerGroupId = null;
        playerKefId = null;
        queuePane = false;
        scrubSec = null;
        endPuckDrag();
        grabId = null;
        if (!dismissing) resetSheetGesture();
        else pendingBody = false;
    }

    function openSearch() {
        showSheet("search");
        if (spotify?.connected) focusSearch(); // you came here to type
    }
    function openZones() {
        showSheet("zones");
    }

    // ── Zones: drag one room onto another to group ───────────────────────
    // Dragging one thing onto another *is* the grouping gesture; the
    // tap-to-select-then-"Group" flow this replaced needed a whole selection
    // mode, with its own way out, to say the same thing (DESIGN.md §15).
    //
    // Only the dragged speaker moves. If it was leading a zone, Sonos
    // re-elects behind us — "this room now plays with that one" is the whole
    // promise, and moving its former partners along with it would be a second
    // change the user never asked for.
    async function groupOnto(sourceId: string, targetId: string) {
        const target = groupOfSpeaker(targetId)?.coordinator_id ?? targetId;
        if (sourceId === target) return;
        if (groupOfSpeaker(sourceId)?.coordinator_id === target) return; // already together
        await busy.claim("group:" + target, async () => {
            try {
                await api.sonosJoin(sourceId, target);
                await refresh();
                announce(
                    `${speakerById.get(sourceId)?.name ?? "Room"} now plays with ` +
                        `${speakerById.get(targetId)?.name ?? "the other room"}.`,
                );
            } catch (e) {
                toasts.error("Grouping failed", (e as Error).message);
            }
        });
    }

    /**
     * What a screen reader is told about a grouping gesture it can't see.
     * Drag has no visible running commentary; the keyboard path needs one.
     */
    let liveMsg = $state("");
    let announceTimer: ReturnType<typeof setTimeout> | undefined;
    function announce(msg: string) {
        clearTimeout(announceTimer);
        liveMsg = msg;
        announceTimer = setTimeout(() => (liveMsg = ""), 4000);
    }

    // Keyboard grouping: focus a room, press G to pick it up, Tab to another
    // and press Enter to drop it in. A pointer-only gesture would have left
    // grouping with no keyboard path at all — the pucks carry no second
    // control to fall back on any more.
    let grabId = $state<string | null>(null);
    const grabbedName = $derived(grabId ? (speakerById.get(grabId)?.name ?? "") : "");

    function onPuckKey(e: KeyboardEvent, sp: SonosSpeakerView) {
        if (e.metaKey || e.ctrlKey || e.altKey) return;
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
        if (key === "g") {
            e.preventDefault();
            if (grabId === null) {
                grabId = sp.id;
                announce(
                    `${sp.name} picked up. Move to another room and press Enter to group, ` +
                        `Escape to put it back.`,
                );
            } else if (grabId === sp.id) {
                grabId = null;
                announce(`${sp.name} put back.`);
            } else {
                dropGrab(sp);
            }
            return;
        }
        // Enter on a puck normally opens that room's player. While something
        // is held, it means "drop it here" instead — so the default activation
        // has to be stopped before it fires a click.
        if (key === "Enter" && grabId !== null && grabId !== sp.id) {
            e.preventDefault();
            dropGrab(sp);
        }
    }
    function dropGrab(sp: SonosSpeakerView) {
        const src = grabId;
        grabId = null;
        if (src) void groupOnto(src, sp.id);
    }

    // Pointer drag. A ghost follows the finger and the room under it takes an
    // amber ring; below the move threshold nothing lifts at all, so opening a
    // room never mis-fires as an attempted group.
    type PuckDrag = {
        id: string;
        name: string;
        playing: boolean;
        sub: string;
        w: number;
        h: number;
        offX: number;
        offY: number;
        x: number;
        y: number;
    };
    let puckDrag = $state<PuckDrag | null>(null);
    /** The room under the pointer, if it's a room. */
    let dropId = $state<string | null>(null);
    /** The zone under the pointer, when the pointer is on its enclosure. */
    let dropZone = $state<string | null>(null);
    /** True from the lift until the click it would otherwise fire is eaten. */
    let dragConsumedClick = false;

    type PuckPending = {
        sp: SonosSpeakerView;
        el: HTMLElement;
        pid: number;
        startX: number;
        startY: number;
        touch: boolean;
    };
    let pending: PuckPending | null = null;
    let holdTimer: ReturnType<typeof setTimeout> | undefined;

    const LIFT_PX = 8; // below this it's a tap, not a drag
    const HOLD_MS = 260; // touch presses lift on a hold, so the sheet still scrolls

    /**
     * While a puck is lifted the page must not scroll under it. `touch-action`
     * can't be changed mid-gesture, so the scroll is refused the only way it
     * can be once a pointer is already down: a non-passive touchmove listener.
     */
    function blockTouchScroll(e: TouchEvent) {
        e.preventDefault();
    }

    function onPuckPointerDown(e: PointerEvent, sp: SonosSpeakerView) {
        if (e.button !== 0 || puckDrag) return;
        const el = e.currentTarget as HTMLElement;
        const touch = e.pointerType !== "mouse";
        pending = { sp, el, pid: e.pointerId, startX: e.clientX, startY: e.clientY, touch };
        if (touch) {
            // A quick swipe is a scroll; a press that stays put is a lift.
            clearTimeout(holdTimer);
            holdTimer = setTimeout(() => lift(e.clientX, e.clientY), HOLD_MS);
        }
    }

    function lift(x: number, y: number) {
        if (!pending || puckDrag) return;
        const { sp, el, pid } = pending;
        const r = el.getBoundingClientRect();
        try {
            el.setPointerCapture(pid);
        } catch {
            // The pointer may already be gone — the drag still works without
            // capture, it just ends on the first pointerup we see.
        }
        document.addEventListener("touchmove", blockTouchScroll, { passive: false });
        puckDrag = {
            id: sp.id,
            name: sp.name,
            playing: speakerPlaying(sp.id),
            sub: speakerNowLine(sp.id),
            w: r.width,
            h: r.height,
            offX: x - r.left,
            offY: y - r.top,
            x: r.left,
            y: r.top,
        };
        announce(`${sp.name} lifted. Drop it on another room to group them.`);
    }

    function onPuckPointerMove(e: PointerEvent) {
        if (!pending) return;
        const dx = e.clientX - pending.startX;
        const dy = e.clientY - pending.startY;
        const moved = Math.hypot(dx, dy);
        if (!puckDrag) {
            if (pending.touch) {
                // Moved before the hold landed — that was a scroll.
                if (moved > LIFT_PX) cancelPending();
            } else if (moved > LIFT_PX) {
                lift(e.clientX, e.clientY);
            }
            if (!puckDrag) return;
        }
        e.preventDefault();
        puckDrag.x = e.clientX - puckDrag.offX;
        puckDrag.y = e.clientY - puckDrag.offY;
        lastPoint = { x: e.clientX, y: e.clientY };
        aimAt(e.clientX, e.clientY);
        edgeScroll(e.clientY);
    }

    /**
     * What the pointer is over: a room, or the enclosure around an existing
     * zone. The enclosure counts because "drag a third onto an existing group
     * adds it" reads as dropping on the *group* — landing in the gap between
     * its pucks shouldn't be a miss.
     */
    function aimAt(x: number, y: number) {
        if (!puckDrag) return;
        const under = document.elementFromPoint(x, y);
        const hit = under?.closest?.(".puck, .group-wrap") as HTMLElement | null;
        const speaker = hit?.dataset.speaker ?? null;
        if (speaker) {
            dropId = speaker !== puckDrag.id ? speaker : null;
            dropZone = null;
            return;
        }
        const zone = hit?.dataset.zone ?? null;
        // A room already in this zone can't be dropped into it again.
        const mine = groupOfSpeaker(puckDrag.id)?.coordinator_id;
        dropZone = zone && zone !== mine ? zone : null;
        dropId = null;
    }

    // ── Edge auto-scroll ─────────────────────────────────────────────────
    // The zone grid is taller than the sheet as soon as there are a few
    // rooms, so a target can sit off-screen with the finger already down and
    // nothing left to reach it with. Holding the puck near an edge scrolls
    // the sheet under it, speed rising as it gets closer.
    let lastPoint: { x: number; y: number } | null = null;
    let scrollStep = 0;
    let scrollFrame: number | undefined;
    const EDGE_PX = 72;
    const EDGE_MAX = 16; // px per frame at the very edge

    function edgeScroll(y: number) {
        const el = scrollEl;
        if (!el || !puckDrag) return (scrollStep = 0);
        const r = el.getBoundingClientRect();
        const over = y - (r.bottom - EDGE_PX);
        const under = r.top + EDGE_PX - y;
        scrollStep =
            over > 0 ? Math.min(1, over / EDGE_PX) * EDGE_MAX
            : under > 0 ? -Math.min(1, under / EDGE_PX) * EDGE_MAX
            : 0;
        if (scrollStep !== 0 && scrollFrame === undefined) stepScroll();
    }
    function stepScroll() {
        scrollFrame = requestAnimationFrame(() => {
            scrollFrame = undefined;
            const el = scrollEl;
            if (!puckDrag || !el || scrollStep === 0) return;
            const before = el.scrollTop;
            el.scrollTop += scrollStep;
            if (el.scrollTop === before) return; // hit the end — nothing to chase
            // The ghost is pinned to the viewport, so scrolling moves a
            // different room under a finger that hasn't budged.
            if (lastPoint) aimAt(lastPoint.x, lastPoint.y);
            stepScroll();
        });
    }
    function stopEdgeScroll() {
        if (scrollFrame !== undefined) cancelAnimationFrame(scrollFrame);
        scrollFrame = undefined;
        scrollStep = 0;
        lastPoint = null;
    }

    function onPuckPointerUp() {
        if (!pending) return;
        const src = puckDrag?.id;
        const target = dropId ?? dropZone;
        if (puckDrag) dragConsumedClick = true;
        endPuckDrag();
        if (src && target) void groupOnto(src, target);
    }

    /** Swallow the click a finished drag would otherwise fire on the puck. */
    function onPuckClickCapture(e: MouseEvent) {
        if (!dragConsumedClick) return;
        dragConsumedClick = false;
        e.preventDefault();
        e.stopPropagation();
    }

    function cancelPending() {
        clearTimeout(holdTimer);
        pending = null;
    }

    function endPuckDrag() {
        clearTimeout(holdTimer);
        if (pending && puckDrag) {
            try {
                pending.el.releasePointerCapture(pending.pid);
            } catch {
                // Already released with the pointer.
            }
        }
        if (puckDrag) {
            document.removeEventListener("touchmove", blockTouchScroll);
        }
        stopEdgeScroll();
        pending = null;
        puckDrag = null;
        dropId = null;
        dropZone = null;
    }

    async function ungroup(g: SonosGroupView) {
        await busy.claim("ungroup:" + g.coordinator_id, async () => {
            try {
                for (const id of g.member_ids) {
                    if (id !== g.coordinator_id) await api.sonosLeave(id);
                }
                await refresh();
            } catch (e) {
                toasts.error("Ungrouping failed", (e as Error).message);
            }
        });
    }

    // ── Player sheet ─────────────────────────────────────────────────────
    // The docked mini-player expands into a full sheet. Rendered inline (not
    // via the modal stack) so it stays live against the 5s status poll.
    //
    // Both bridges open one. A KEF speaker used to send you to its settings
    // screen instead — two levels away, from a chip that sat beside Sonos
    // chips and looked exactly like them. That was the module's worst seam,
    // and the reasoning behind it ("a full player would be an art-led sheet
    // with two controls in it") simply wasn't true: KEF reports art, title,
    // artist, position, duration, volume, mute and an input, and answers
    // play/pause/next/previous. What it hasn't got is a queue and a group,
    // so its sheet drops those two sections and keeps the rest.
    let playerGroupId = $state<string | null>(null);
    let playerKefId = $state<string | null>(null);
    const activeGroup = $derived(
        groups.find((g) => g.coordinator_id === playerGroupId),
    );
    const activeKef = $derived(
        playerKefId ? (kefSpeakers.find((s) => s.id === playerKefId) ?? null) : null,
    );
    const playerOpen = $derived(
        openSheet === "player" && (playerGroupId !== null || playerKefId !== null),
    );
    // The group the docked mini-player represents. Normally the first thing
    // playing — but a pause must not carry the transport off the screen with
    // it, so the dock holds on to the last live zone while it still has a
    // track to resume. "Playing now" stays literal (DESIGN.md §15); the dock
    // is where a paused zone remains one tap from playing again.
    let lastLiveId = $state<string | null>(null);
    $effect(() => {
        const g = playingGroups[0];
        if (g) lastLiveId = g.coordinator_id;
    });
    const pausedGroup = $derived(
        groups.find((g) => {
            if (g.coordinator_id !== lastLiveId) return false;
            const st = coordinatorOf(g)?.state;
            return !!st?.track?.title && st.transport_state !== "STOPPED";
        }),
    );
    const dockGroup = $derived(playingGroups[0] ?? pausedGroup);

    // ── Dock visibility ──────────────────────────────────────────────────
    // The dock and the Home screen's "Playing now" card carry the same track
    // and the same play/pause, so showing both stacks one control on top of
    // its own duplicate. The dock is the *fallback*: it appears only once the
    // card it repeats has left the screen — which is always over the Zones and
    // Search sheets, and on Speakers, where no such card exists.
    //
    // It rides *over* the Zones and Search sheets rather than under them: the
    // transport persists across Home, Zones and Search (DESIGN.md §15), and
    // Search's whole job is to feed it. Tapping it there swaps that sheet for
    // the player rather than stacking one sheet on another.
    let dockCardOnScreen = $state(false);
    const overSheet = $derived(searchOpen || zonesOpen);
    const showDock = $derived(
        !!dockGroup && (overSheet || (!dockCardOnScreen && !playerOpen)),
    );

    // The dock runs the full width of the band the assistant FAB floats in.
    // While it is up the FAB stands down, so the transport gets the whole bar
    // instead of a 64px gutter round a button sitting on top of it
    // (DESIGN.md §7).
    $effect(() => {
        if (!showDock) return;
        return bottomBar.claim();
    });

    // Attached to every "Playing now" card, live only on the dock group's.
    // The bottom inset discounts the band the dock and the tab bar occupy, so
    // a card sitting behind them counts as gone rather than as visible.
    function dockAnchor(node: HTMLElement, isDock: boolean) {
        let obs: IntersectionObserver | undefined;
        let active = false;
        function attach(on: boolean) {
            obs?.disconnect();
            obs = undefined;
            if (active && !on) dockCardOnScreen = false;
            active = on;
            if (!on) return;
            obs = new IntersectionObserver(
                ([entry]) => (dockCardOnScreen = entry.isIntersecting),
                { threshold: 0.5, rootMargin: "0px 0px -96px 0px" },
            );
            obs.observe(node);
        }
        attach(isDock);
        return {
            update: (next: boolean) => attach(next),
            destroy: () => attach(false),
        };
    }

    let playerEl = $state<HTMLElement | null>(null);

    function openPlayer(g: SonosGroupView) {
        playerKefId = null; // one player at a time
        playerGroupId = g.coordinator_id;
        raisePlayer();
        // The room you just opened is also where you'd expect the next
        // favorite or search result to land, so opening the player sets the
        // destination too — one choice instead of two.
        dest = { kind: "sonos", id: g.coordinator_id };
    }
    /** The same gesture for a KEF room, so the chips beside it don't lie. */
    function openKEFPlayer(sp: KEFSpeakerView) {
        if (!sp.reachable) return void openKEFModal(sp); // fix the address instead
        playerGroupId = null;
        playerKefId = sp.id;
        raisePlayer();
        dest = { kind: "kef", id: sp.id };
    }
    function raisePlayer() {
        // Opened from Zones or Search, the player *replaces* that sheet and
        // puts it back on the way out — a swap, so there is never a sheet over
        // a sheet, and never a lost place either. "Its place" includes how far
        // it was scrolled, which is why the outgoing offset is kept.
        rememberSheetScroll();
        sheets = sheetRun.swapTo(sheets, "player");
        sheetScroll.player = 0;
        resetSheetGesture();
    }
    function closePlayer() {
        if (openSheet !== "player") return;
        dropSheet();
    }

    /**
     * Search from inside the player. The sheet *hands over* rather than
     * opening one over another, so closing Search comes back to the room you
     * started from — and that room is already the destination, set when the
     * player opened, so a result plays where you were looking.
     *
     * Without this the player was a dead end for the one thing it kept
     * pointing at: both sheets' idle copy says "or search Spotify", and a
     * KEF speaker has no favorites to offer instead, so its idle player named
     * the only way to start music and then didn't offer it.
     */
    function searchFromPlayer(q?: string) {
        rememberSheetScroll();
        sheets = sheetRun.swapTo(sheets, "search");
        sheetScroll.search = 0;
        resetSheetGesture();
        if (q) {
            // A recent search is a request to *run* it, so it runs — and the
            // caret stays out of the way, keyboard and all, since the results
            // are what was asked for.
            runHistoryQuery(q);
            return;
        }
        if (spotify?.connected) focusSearch();
    }

    // ── Drag-to-dismiss ──────────────────────────────────────────────────
    // The same gesture the shared Modal sheet carries, shared by all three of
    // Music's sheets (only one is ever up): the top bar always drags, and the
    // scroll body drags only from the top and only on a clear downward pull,
    // so a long queue still scrolls normally.
    let dragY = $state(0);
    let dragging = $state(false);
    let dismissing = $state(false);
    let pendingBody = false;
    let dragStartY = 0;
    let dragStartX = 0;

    // Mobile only: from 601px the sheet is a centered dialog whose transform
    // carries its centering, so a drag offset would knock it off-centre.
    function sheetDraggable(): boolean {
        return window.matchMedia("(max-width: 600px)").matches;
    }
    // Pointer events from the top bar bubble into the scroll container (and,
    // once captured, keep reporting it as their target) — the body handlers
    // ignore them so only one drag path is ever live.
    function fromTop(e: PointerEvent): boolean {
        return !!(e.target as HTMLElement | null)?.closest?.(".sheet-top");
    }
    /** Swiping down closes the open sheet — back to the one under it, if any. */
    const dismissSheet = () => dropSheet();

    function startDrag(e: PointerEvent, target: HTMLElement) {
        dragging = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
        dragY = 0;
        try { target.setPointerCapture(e.pointerId); } catch { /* not capturable */ }
    }
    function cancelDrag() {
        if (!dragging) return;
        dragging = false;
        requestAnimationFrame(() => { dragY = 0; });
    }
    function finishDrag() {
        dragging = false;
        if (dragY > 90) {
            // Ride the throw out instead of snapping back and then playing
            // the sheet's own exit — the finger already did that animation.
            dismissing = true;
            dragY = 600;
            setTimeout(dismissSheet, 220);
        } else {
            requestAnimationFrame(() => { dragY = 0; });
        }
    }

    // Top bar — always drags.
    function onTopPointerDown(e: PointerEvent) {
        if (dismissing || !sheetDraggable()) return;
        if ((e.target as HTMLElement).closest("button")) return; // close / back
        startDrag(e, e.currentTarget as HTMLElement);
        e.preventDefault();
    }
    function onTopPointerMove(e: PointerEvent) {
        if (!dragging) return;
        dragY = Math.max(0, e.clientY - dragStartY);
    }
    function onTopPointerUp() {
        if (dragging) finishDrag();
    }

    // Body — drags when the scroll is at the top, otherwise scrolls.
    function onBodyPointerDown(e: PointerEvent) {
        if (dismissing || !sheetDraggable() || fromTop(e)) return;
        if (e.pointerType === "mouse") return; // pointer devices use the bar
        if (!scrollEl || scrollEl.scrollTop > 0) return;
        if ((e.target as HTMLElement).closest("input, button, a, [role='slider']")) return;
        pendingBody = true;
        dragStartY = e.clientY;
        dragStartX = e.clientX;
    }
    function onBodyPointerMove(e: PointerEvent) {
        if (fromTop(e)) return;
        if (dragging) {
            dragY = Math.max(0, e.clientY - dragStartY);
            e.preventDefault(); // claimed: don't scroll as well
            return;
        }
        if (!pendingBody) return;
        const dy = e.clientY - dragStartY;
        const dx = e.clientX - dragStartX;
        if (dy > 8 && dy > Math.abs(dx)) {
            pendingBody = false;
            const from = dragStartY;
            startDrag(e, scrollEl!);
            dragStartY = from; // keep the origin so the sheet doesn't jump back
            dragY = dy;
            e.preventDefault();
        } else if (dy < -4 || Math.abs(dx) > 12) {
            pendingBody = false; // scrolling up or swiping sideways
        }
    }
    function onBodyPointerUp(e: PointerEvent) {
        if (fromTop(e)) return;
        pendingBody = false;
        if (dragging) finishDrag();
    }
    function onBodyPointerCancel(e: PointerEvent) {
        if (fromTop(e)) return;
        pendingBody = false;
        cancelDrag();
    }
    // ── Swipe the art to change track ────────────────────────────────────
    // The album art is the largest target in the sheet, so it carries the
    // gesture every phone player has: drag sideways, let go past a clear
    // threshold, and the track changes. A vertical pull is handed straight
    // back to the sheet's own drag-to-dismiss.
    let artDX = $state(0);
    let artSwiping = $state(false);
    let artStart: { x: number; y: number } | null = null;

    function onArtPointerDown(e: PointerEvent) {
        if (e.pointerType === "mouse" || dismissing) return;
        artStart = { x: e.clientX, y: e.clientY };
        artDX = 0;
    }
    function onArtPointerMove(e: PointerEvent) {
        if (!artStart) return;
        const dx = e.clientX - artStart.x;
        const dy = e.clientY - artStart.y;
        if (!artSwiping) {
            // Vertical wins early: that gesture belongs to the sheet.
            if (Math.abs(dy) > 10 && Math.abs(dy) >= Math.abs(dx)) {
                artStart = null;
                return;
            }
            if (Math.abs(dx) < 12) return;
            artSwiping = true;
            try { (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); }
            catch { /* not capturable */ }
        }
        // Half speed: the art nudges along with the finger rather than being
        // thrown off the screen.
        artDX = dx * 0.5;
        e.preventDefault();
    }
    function onArtPointerUp() {
        if (!artStart) return;
        const moved = artDX;
        artStart = null;
        artSwiping = false;
        artDX = 0;
        if (Math.abs(moved) < 30 || !activeGroup) return; // ~60px of travel
        skip(activeGroup, moved < 0 ? "next" : "previous");
    }
    function onArtPointerCancel() {
        artStart = null;
        artSwiping = false;
        artDX = 0;
    }

    // ── Keyboard ─────────────────────────────────────────────────────────
    // The player covers the whole screen, so while it is open it answers the
    // transport keys a music app is expected to answer to. Everything else
    // stays scoped: only Escape and "/" work from the view at large, so the
    // module never swallows keys the rest of the app might want.
    function editableTarget(e: KeyboardEvent): HTMLElement | null {
        return (
            (e.target as HTMLElement | null)?.closest?.(
                "input, textarea, select, [contenteditable='true']",
            ) ?? null
        ) as HTMLElement | null;
    }

    function onWindowKey(e: KeyboardEvent) {
        const field = editableTarget(e);
        // A range input (scrubber, volume) owns the arrow keys while the
        // caret is on it — we only borrow the ones it ignores.
        const slider = field instanceof HTMLInputElement && field.type === "range";
        const onControl = !!(e.target as HTMLElement | null)?.closest?.("button, a");
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

        if (key === "Escape") {
            // Escape always leaves the player outright rather than stepping
            // back through the queue pane — the sheet covers the nav, so one
            // press must always be enough to get out (DESIGN.md §15).
            if (menuFor) menuFor = null;
            else if (puckDrag || grabId) {
                // Put a held room back before leaving the sheet it was held in.
                const name = grabbedName || puckDrag?.name || "Room";
                endPuckDrag();
                grabId = null;
                announce(`${name} put back.`);
            } else if (openSheet) dropSheet();
            // Escape backs out of a speaker's settings the same way its back
            // chip does — a drill-down owes the user the key that leaves it.
            else if (detailId) detailId = null;
            else if (kefDetailId) kefDetailId = null;
            // …and out of Speakers itself, which is a screen, not a sheet.
            else if (screen === "speakers") leaveSpeakers();
            return;
        }
        if (e.metaKey || e.ctrlKey || e.altKey) return;

        if (!playerOpen) {
            // "/" is the one shortcut that works from anywhere in the view:
            // raise Search and put the caret in the box.
            if (key === "/" && !field) {
                e.preventDefault();
                openSearch();
            }
            return;
        }

        if (field && !slider) return; // typing, not controlling

        // "/" keeps its meaning inside the player — it just searches *for
        // this room*, and hands the sheet over rather than stacking one.
        if (key === "/") {
            e.preventDefault();
            searchFromPlayer();
            return;
        }

        // A KEF player answers the keys it can: play/pause, skip, volume. It
        // has no seek, no queue and no play modes, so those keys stay unbound
        // here rather than doing something almost-right.
        const kf = activeKef;
        if (kf) {
            if ((key === " " || key === "k") && !(key === " " && onControl)) {
                e.preventDefault();
                void kefTogglePlay(kf);
                return;
            }
            if (slider) return;
            switch (key) {
                case "ArrowRight": e.preventDefault(); kefSkip(kf, "next"); break;
                case "ArrowLeft": e.preventDefault(); kefSkip(kf, "previous"); break;
                case "ArrowUp": e.preventDefault(); kefSetVolume(kf, kefShownVol(kf) + 5); break;
                case "ArrowDown": e.preventDefault(); kefSetVolume(kf, kefShownVol(kf) - 5); break;
                case "n": kefSkip(kf, "next"); break;
                case "p": kefSkip(kf, "previous"); break;
                case "m": kefToggleMute(kf); break;
            }
            return;
        }

        const g = activeGroup;
        if (!g) return;

        // Space on a focused button belongs to that button, not to us.
        if ((key === " " || key === "k") && !(key === " " && onControl)) {
            e.preventDefault();
            void togglePlay(g);
            return;
        }
        if (slider) return;

        const gs = groupStateOf(g);
        switch (key) {
            case "ArrowRight":
                e.preventDefault();
                if (e.shiftKey || durationSec === 0) skip(g, "next");
                else commitSeek(g, Math.min(durationSec, livePos + 10));
                break;
            case "ArrowLeft":
                e.preventDefault();
                if (e.shiftKey || durationSec === 0) skip(g, "previous");
                else commitSeek(g, Math.max(0, livePos - 10));
                break;
            case "ArrowUp":
                e.preventDefault();
                nudgeVolume(g, 5);
                break;
            case "ArrowDown":
                e.preventDefault();
                nudgeVolume(g, -5);
                break;
            case "n": skip(g, "next"); break;
            case "p": skip(g, "previous"); break;
            case "m": toggleMuteGroup(g); break;
            case "s": if (gs) setPlayMode(g, { shuffle: !gs.shuffle }); break;
            case "r": if (gs) setPlayMode(g, { repeat: NEXT_REPEAT[gs.repeat] }); break;
            case "q":
                if (queuePane || (gs?.queue_length ?? 0) > 0) queuePane = !queuePane;
                break;
        }
    }
    // A regroup between polls can retire the coordinator the sheet is bound
    // to. Close instead of leaving an empty sheet — and, more importantly,
    // a permanently locked body scroll.
    $effect(() => {
        if (playerOpen && playerGroupId !== null && !activeGroup) closePlayer();
        if (playerOpen && playerKefId !== null && !activeKef) closePlayer();
    });
    // Move focus into the sheet when it opens so keyboard users land there.
    $effect(() => {
        if (playerOpen) playerEl?.focus();
    });
    // A room held for grouping that drops off the network can't be dropped
    // anywhere — let go of it rather than leaving a puck lifted over nothing.
    $effect(() => {
        const live = new Set(reachable.map((s) => s.id));
        if (grabId && !live.has(grabId)) grabId = null;
        if (puckDrag && !live.has(puckDrag.id)) endPuckDrag();
    });
    // Speakers outside the active group that could join it.
    function joinables(g: SonosGroupView): SonosSpeakerView[] {
        return reachable.filter((s) => !g.member_ids.includes(s.id));
    }

    // ── Scrubbing ────────────────────────────────────────────────────────
    // The position is only polled every 5s, so between polls every surface
    // showing progress extrapolates from the last reading; `clock.beat`
    // re-runs those derivations once a second. Held only while something is
    // actually moving — which is this view's judgement to make, not the
    // clock's.
    $effect(() => {
        // Both bridges, or a house with only KEF speakers never started the
        // clock at all: `playingCount` is the Sonos count, so a KEF card's
        // progress hairline stood still unless a Sonos zone happened to be
        // playing beside it.
        if (!playerOpen && playingCount === 0 && kefPlaying.length === 0) return;
        return clock.start();
    });

    // Non-null while a finger/pointer is on the scrubber.
    let scrubSec = $state<number | null>(null);
    // A just-issued seek wins over the polled position until the speaker has
    // had time to report it — same idea as volOverride.
    let seekOverride: { sec: number; at: number } | null = $state(null);

    const activeState = $derived(activeGroup ? coordinatorOf(activeGroup)?.state : undefined);
    // Sources without a duration (radio, line-in, TV) can't be seeked.
    const durationSec = $derived(secs(activeState?.duration));

    const livePos = $derived.by(() => {
        if (scrubSec !== null) return scrubSec;
        void clock.beat; // re-derive once a second
        const now = Date.now();
        const ov = seekOverride;
        const base = ov && now - ov.at < 4000 ? ov.sec : secs(activeState?.position);
        const since = ov && now - ov.at < 4000 ? ov.at : polledAt;
        if (!isPlaying(activeGroup) || !since) return base;
        const advanced = base + (now - since) / 1000;
        return durationSec ? Math.min(durationSec, advanced) : advanced;
    });

    function commitSeek(g: SonosGroupView, sec: number) {
        const c = coordinatorOf(g);
        scrubSec = null;
        if (!c) return;
        seekOverride = { sec, at: Date.now() };
        api.sonosSeek(c.id, toClock(sec)).catch((e) => {
            seekOverride = null;
            toasts.error("Seek failed", (e as Error).message);
        });
    }

    // Drop the scrub/seek overrides when the track or the target changes, so
    // a new song never inherits the previous one's position. The guard
    // matters: every poll replaces the status objects, so this effect re-runs
    // on the 5s tick — without it, a drag in progress would be cancelled and
    // a fresh seek discarded each time a poll landed.
    let lastTrackKey = "";
    $effect(() => {
        const key = `${playerGroupId ?? ""}|${activeState?.track?.title ?? ""}`;
        if (key === lastTrackKey) return;
        lastTrackKey = key;
        scrubSec = null;
        seekOverride = null;
    });

    // ── Play modes ───────────────────────────────────────────────────────
    // Sonos stores shuffle and repeat as one composite value, so both axes
    // are always sent together; the patch fills in whichever isn't changing.
    const NEXT_REPEAT: Record<SonosRepeat, SonosRepeat> = { off: "all", all: "one", one: "off" };

    function setPlayMode(g: SonosGroupView, patch: { shuffle?: boolean; repeat?: SonosRepeat }) {
        const c = coordinatorOf(g);
        const gs = groupStateOf(g);
        if (!c || !gs) return;
        void run(
            "mode:" + c.id,
            () => api.sonosSetPlayMode(c.id, patch.shuffle ?? gs.shuffle, patch.repeat ?? gs.repeat),
            "Couldn't change play mode",
        );
    }
    function toggleCrossfade(g: SonosGroupView) {
        const c = coordinatorOf(g);
        const gs = groupStateOf(g);
        if (!c || !gs) return;
        void run("xfade:" + c.id, () => api.sonosSetCrossfade(c.id, !gs.crossfade), "Couldn't change crossfade");
    }
    function repeatLabel(r?: SonosRepeat): string {
        if (r === "all") return "Repeat all — tap for repeat one";
        if (r === "one") return "Repeat one — tap to turn repeat off";
        return "Repeat off — tap to repeat all";
    }

    // ── Queue ────────────────────────────────────────────────────────────
    let queuePane = $state(false);
    // The two panes share one scroll container, so switching has to rewind
    // it — otherwise the queue opens halfway down at the player's offset.
    let scrollEl = $state<HTMLElement | null>(null);
    $effect(() => {
        void queuePane;
        if (scrollEl) scrollEl.scrollTop = 0;
    });
    let queue = $state<SonosQueueItem[]>([]);
    let queueLoading = $state(false);
    let queueSeq = 0;

    async function loadQueue(coordinatorId: string, skeleton = false) {
        const seq = ++queueSeq;
        if (skeleton) queueLoading = true;
        try {
            const q = await api.sonosQueue(coordinatorId);
            if (seq !== queueSeq) return;
            queue = q;
        } catch {
            if (seq === queueSeq) queue = []; // an unreachable coordinator shows empty
        } finally {
            if (seq === queueSeq) queueLoading = false;
        }
    }

    // Load the queue whenever the player binds to a group: the "Up next" row
    // needs a real track name, not just a count.
    $effect(() => {
        const id = playerGroupId;
        if (id === null) {
            queueSeq++; // cancel any in-flight load
            queue = [];
            return;
        }
        void loadQueue(id, true);
    });

    // The first queued track after the one playing.
    const nextInQueue = $derived.by(() => {
        const cur = activeState?.queue_track ?? 0;
        return queue.find((q) => q.track > cur);
    });

    function jumpTo(g: SonosGroupView, track: number) {
        const c = coordinatorOf(g);
        if (!c) return;
        void run("jump:" + track, () => api.sonosSeekTrack(c.id, track), "Couldn't play that track");
    }

    async function removeQueued(g: SonosGroupView, track: number) {
        const c = coordinatorOf(g);
        if (!c) return;
        await run("qrm:" + track, () => api.sonosQueueRemove(c.id, track), "Couldn't remove that track");
        // Removing renumbers everything below it, so re-read rather than
        // splicing locally.
        void loadQueue(c.id);
    }

    async function clearQueue(g: SonosGroupView) {
        const c = coordinatorOf(g);
        if (!c) return;
        // Clearing stops playback, so it gets the same confirm treatment as
        // any other destructive action.
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Clear the queue?",
            message: `Every track queued on ${groupTitle(g)} will be removed, and playback stops.`,
            confirmLabel: "Clear queue",
            danger: true,
        });
        if (!ok) return;
        await run("qclear:" + c.id, () => api.sonosQueueClear(c.id), "Couldn't clear the queue");
        void loadQueue(c.id);
    }

    // Enqueue without disturbing what's playing. Used by search results and
    // favorites; `next` drops it in after the current track. The queue is a
    // Sonos group's, so this is only ever offered for a Sonos destination.
    async function enqueue(
        item: { uri: string; title?: string; service?: string; metadata?: string },
        next: boolean,
        target: string | null = sonosTarget,
    ) {
        if (!target) return;
        await busy.claim("q:" + item.uri, async () => {
            try {
                const added = await api.sonosQueueAdd(target, { ...item, next });
                const where = added.track
                    ? `position ${added.track} of ${added.length}`
                    : "the queue";
                toasts.success(
                    next ? "Playing next" : "Added to queue",
                    `${item.title ?? "Track"} · ${where}`,
                );
                if (playerGroupId === target) void loadQueue(target);
            } catch (e) {
                toasts.error("Couldn't add to the queue", (e as Error).message);
            }
        });
    }

    // ── Row overflow menus (search results, favorites) ───────────────────
    // Keyed by item URI: at most one menu is open at a time.
    let menuFor = $state<string | null>(null);
    $effect(() => {
        if (!menuFor) return;
        const close = () => (menuFor = null);
        // The opening click calls stopPropagation, so it never reaches here.
        document.addEventListener("click", close);
        return () => document.removeEventListener("click", close);
    });
    function toggleMenu(e: MouseEvent, uri: string) {
        e.stopPropagation();
        menuFor = menuFor === uri ? null : uri;
    }
    // An open menu takes focus and answers the arrow keys, so queueing a
    // result never means tabbing back through the whole results list.
    function menuNav(node: HTMLElement) {
        const items = () =>
            Array.from(node.querySelectorAll<HTMLButtonElement>("[role='menuitem']"));
        items()[0]?.focus();
        function onKey(e: KeyboardEvent) {
            if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
            e.preventDefault();
            const list = items();
            const i = list.indexOf(document.activeElement as HTMLButtonElement);
            const next = e.key === "ArrowDown" ? i + 1 : i - 1;
            list[(next + list.length) % list.length]?.focus();
        }
        node.addEventListener("keydown", onKey);
        return { destroy: () => node.removeEventListener("keydown", onKey) };
    }

    // ── Spotify search ───────────────────────────────────────────────────
    let spotify = $state<SpotifyStatus | null>(null);
    let spotifySetup = $state(false); // client-ID form expanded
    let clientId = $state("");
    let spotifySaving = $state(false);
    let query = $state("");
    let searching = $state(false);
    let results = $state<SpotifyResults | null>(null);
    let kindFilter = $state<"tracks" | "albums" | "playlists">("tracks");
    let myPlaylists = $state<SpotifyItem[]>([]);
    let myPlaylistsLoaded = false;

    async function loadSpotify() {
        try {
            spotify = await api.spotifyStatus();
            if (spotify.connected && !myPlaylistsLoaded) {
                myPlaylistsLoaded = true;
                myPlaylists = await api.spotifyMyPlaylists().catch(() => []);
            }
        } catch {
            spotify = null; // integration unavailable — hide the card
        }
    }

    // The OAuth callback bounces back to /#/music?spotify=… — surface the
    // outcome once, then clean the query off the URL.
    onMount(() => {
        const q = route.query;
        if (q.spotify === "connected") {
            toasts.success("Spotify connected");
            route.go("music");
        } else if (q.spotify_error) {
            toasts.error("Spotify login failed", q.spotify_error);
            route.go("music");
        }
        // Search now lives behind an icon, and the round trip to Spotify ends
        // here — land back on the sheet the user left, not on a Home screen
        // with no sign of what just happened.
        if (q.spotify || q.spotify_error) openSearch();
        void loadSpotify();
    });

    async function saveClientId() {
        if (spotifySaving || !clientId.trim()) return;
        spotifySaving = true;
        try {
            await api.spotifySetConfig(clientId.trim());
            spotifySetup = false;
            await loadSpotify();
            toasts.success("Client ID saved", "Now connect your Spotify account.");
        } catch (e) {
            toasts.error("Save failed", (e as Error).message);
        } finally {
            spotifySaving = false;
        }
    }

    let pasteUrl = $state("");
    let finishing = $state(false);
    let copied = $state(false);

    async function copyRedirect() {
        if (!spotify) return;
        if (await copyText(spotify.redirect_uri)) {
            copied = true;
            setTimeout(() => (copied = false), 1800);
        }
    }

    async function connectSpotify() {
        // Manual flow: keep this page open — the consent tab is opened
        // synchronously (before the await) so popup blockers allow it,
        // then pointed at the authorize URL once it arrives.
        const tab = spotify?.manual ? window.open("about:blank", "_blank") : null;
        try {
            const { url } = await api.spotifyLoginURL();
            if (spotify?.manual) {
                if (tab) tab.location.href = url;
                else window.location.href = url; // popup blocked — same tab still works
            } else {
                window.location.href = url; // bounces back here automatically
            }
        } catch (e) {
            tab?.close();
            toasts.error("Couldn't start Spotify login", (e as Error).message);
        }
    }

    async function finishConnect() {
        if (finishing || !pasteUrl.trim()) return;
        finishing = true;
        try {
            await api.spotifyExchange(pasteUrl);
            pasteUrl = "";
            toasts.success("Spotify connected");
            await loadSpotify();
        } catch (e) {
            toasts.error("Couldn't finish the login", (e as Error).message);
        } finally {
            finishing = false;
        }
    }

    async function disconnectSpotify() {
        // Confirm first: disconnecting drops the tokens, so the card drops
        // back to the connect page and the only way back is the full OAuth
        // flow again. An accidental tap must not strand the user there.
        const who = spotify?.display_name ? `"${spotify.display_name}"` : "Your Spotify account";
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Disconnect Spotify?",
            message: `${who} will be unlinked. To search again you'll need to reconnect through Spotify.`,
            confirmLabel: "Disconnect",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.spotifyDisconnect();
            results = null;
            query = "";
            myPlaylists = [];
            myPlaylistsLoaded = false;
            await loadSpotify();
        } catch (e) {
            toasts.error("Disconnect failed", (e as Error).message);
        }
    }

    let searchTimer: ReturnType<typeof setTimeout> | undefined;
    let searchSeq = 0;
    let searchEl = $state<HTMLInputElement | null>(null);

    // Focus the box on the way into Search — but only where a keyboard is
    // already there. On a phone an auto-focus throws up the software keyboard
    // over the results the user came to look at.
    function focusSearch() {
        if (!window.matchMedia("(pointer: fine)").matches) return;
        // The box may not be in the DOM yet — this can run on the way in.
        void flushDOM().then(() => searchEl?.focus());
    }
    function onQueryInput() {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(doSearch, 400);
    }
    // Enter runs the search now instead of waiting out the debounce; Escape
    // clears the box rather than closing something behind it.
    function onQueryKey(e: KeyboardEvent) {
        if (e.key === "Enter") {
            e.preventDefault();
            clearTimeout(searchTimer);
            const q = query.trim();
            if (q) addToHistory(q);
            void doSearch();
        } else if (e.key === "Escape" && query) {
            e.stopPropagation();
            clearQuery();
        }
    }
    function clearQuery() {
        clearTimeout(searchTimer);
        searchSeq++; // drop an in-flight search
        query = "";
        results = null;
        searching = false;
        searchEl?.focus();
    }
    async function doSearch() {
        const q = query.trim();
        const seq = ++searchSeq;
        if (!q) { results = null; searching = false; return; }
        searching = true;
        try {
            const r = await api.spotifySearch(q, 8);
            if (seq !== searchSeq) return;
            results = r;
        } catch (e) {
            if (seq !== searchSeq) return;
            toasts.error("Search failed", (e as Error).message);
        } finally {
            if (seq === searchSeq) searching = false;
        }
    }

    // ── Search history ───────────────────────────────────────────────────
    // Keyed by the room a search is played on (the destination), since "recent
    // searches" reads differently in the kitchen than in the bedroom. A
    // single-room home only ever has one key, which collapses this to a
    // plain, unscoped history without any extra code path.
    const HISTORY_KEY = "music.searchHistory.v1";
    const HISTORY_MAX = 8;
    function loadHistory(): Record<string, string[]> {
        try {
            const raw = localStorage.getItem(HISTORY_KEY);
            return raw ? JSON.parse(raw) : {};
        } catch { return {}; }
    }
    let searchHistory = $state<Record<string, string[]>>(loadHistory());
    $effect(() => {
        try { localStorage.setItem(HISTORY_KEY, JSON.stringify(searchHistory)); }
        catch { /* private mode */ }
    });
    // Falls back to one shared bucket when there's no destination yet (e.g. no
    // speakers loaded), so history still works before speakers are set up.
    const historyKey = $derived(destKey ?? "_all");
    const historyList = $derived(searchHistory[historyKey] ?? []);
    /** The same list for the player's row, kept to a row rather than a list. */
    const playerRecents = $derived(historyList.slice(0, 6));

    function addToHistory(q: string) {
        const key = historyKey;
        const rest = (searchHistory[key] ?? []).filter((x) => x.toLowerCase() !== q.toLowerCase());
        searchHistory = { ...searchHistory, [key]: [q, ...rest].slice(0, HISTORY_MAX) };
    }
    function removeHistoryEntry(q: string) {
        const key = historyKey;
        searchHistory = {
            ...searchHistory,
            [key]: (searchHistory[key] ?? []).filter((x) => x !== q),
        };
    }
    function clearHistory() {
        const key = historyKey;
        const next = { ...searchHistory };
        delete next[key];
        searchHistory = next;
    }
    function runHistoryQuery(q: string) {
        clearTimeout(searchTimer);
        query = q;
        addToHistory(q);
        void doSearch();
        searchEl?.focus();
    }

    const shownItems = $derived<SpotifyItem[]>(
        results ? results[kindFilter] : myPlaylists,
    );

    // A search result plays on whichever destination is selected. Same tap,
    // same body, two roads: a Sonos group loads it into its queue and streams
    // it with the household's linked account, while a KEF speaker is started
    // through Spotify Connect — its own API has no way to be handed content.
    function playItem(item: SpotifyItem) {
        const d = dest;
        if (!d) return;
        const body = { service: "Spotify", uri: item.uri, title: item.name };
        void startPlayback(
            "item:" + item.uri,
            () => (d.kind === "kef" ? api.kefPlayItem(d.id, body) : api.sonosPlayItem(d.id, body)),
            item.name,
            destName(d),
            d.kind,
        );
    }

    // One sheet for both bridges — it carries the brand picker when adding
    // and is locked to the owning bridge when editing.
    async function openSpeakerModal(sp?: SonosSpeakerView) {
        const changed = await openModal<boolean>(
            SpeakerModal,
            sp ? { existing: sp, brand: "sonos" as const } : {},
        );
        if (changed) {
            void refresh();
            void refreshKEF();
        }
    }

    async function openKEFModal(sp: KEFSpeakerView) {
        const changed = await openModal<boolean>(SpeakerModal, {
            existing: sp,
            brand: "kef" as const,
        });
        if (changed) {
            // A removed speaker must not leave the pane open on a row that
            // no longer exists.
            if (kefDetailId === sp.id) kefDetailId = null;
            void refreshKEF();
        }
    }

    // The push-status sheet. Retrying inside it can turn subscriptions on, and
    // that changes which poll interval this view should be using, so the
    // status is re-read on the way out.
    async function openEventsModal() {
        await openModal(SonosEventsModal, {});
        void refresh();
    }

    // ── Speakers screen ──────────────────────────────────────────────────
    // The device inventory: one row per registered speaker, reachable or not,
    // each opening that speaker's own settings. Ordered so the ones you can
    // actually do something with come first.
    const allSpeakers = $derived.by(() => {
        const list = [...(status?.speakers ?? [])];
        list.sort((a, b) => {
            if (a.reachable !== b.reachable) return a.reachable ? -1 : 1;
            return a.name.localeCompare(b.name);
        });
        return list;
    });

    // Device settings are a *sub-screen* of Speakers, not a sheet: they are
    // reached from the subnav like Home, Rooms and Search, and none of those
    // are sheets. Selecting a speaker swaps the list for its detail; the
    // subnav above stays put, so tapping another screen leaves the detail the
    // same way its back chip does.
    let detailId = $state<string | null>(null);
    const detailSpeaker = $derived(
        detailId ? (status?.speakers.find((s) => s.id === detailId) ?? null) : null,
    );

    // The KEF pane is a separate selection rather than a shared one keyed by
    // id: the two bridges' detail views take different props and answer
    // different questions, and one selection would mean deciding which
    // component to render from the shape of an id.
    let kefDetailId = $state<string | null>(null);
    const kefDetailSpeaker = $derived(
        kefDetailId ? (kefSpeakers.find((s) => s.id === kefDetailId) ?? null) : null,
    );
    const kefDetailSiblings = $derived(kefSpeakers.filter((s) => s.id !== kefDetailId));
    /** Whichever pane is open — the split layout folds the list away for both. */
    const anyDetail = $derived(!!detailSpeaker || !!kefDetailSpeaker);

    function openKEFSpeaker(sp: KEFSpeakerView) {
        detailId = null; // one pane at a time
        kefDetailId = sp.id;
        // Same reasoning as openPlayer: the speaker you just opened is where
        // you'd expect the next search result to land, so opening it sets the
        // destination too. Only when it can actually take one.
        if (sp.reachable) dest = { kind: "kef", id: sp.id };
    }

    // From 1024px up the list and the selected speaker's settings sit side by
    // side, because the width is there and the common job — the same change
    // across several rooms — is otherwise back-and-forward for every one of
    // them. Below that the settings replace the list and the detail carries
    // switcher chips instead.
    let paned = $state(false);
    $effect(() => {
        const mq = window.matchMedia("(min-width: 1024px)");
        const sync = () => (paned = mq.matches);
        sync();
        mq.addEventListener("change", sync);
        return () => mq.removeEventListener("change", sync);
    });
    // A blank right-hand pane is dead space, so the wide layout opens on the
    // first speaker that can answer. On a phone nothing is selected until the
    // user picks a row — there, selecting means leaving the list.
    $effect(() => {
        if (!paned || screen !== "speakers" || detailId || kefDetailId) return;
        const first = allSpeakers.find((s) => s.reachable);
        if (first) {
            detailId = first.id;
            return;
        }
        // A house with only KEF speakers still deserves an open pane.
        const firstKEF = kefSpeakers.find((s) => s.reachable);
        if (firstKEF) kefDetailId = firstKEF.id;
    });
    // Speakers other than the open one, for the phone switcher.
    const detailSiblings = $derived(allSpeakers.filter((s) => s.id !== detailId));
    // The sleep timer belongs to the zone, not the speaker (DESIGN.md §15), so
    // a follower is told which room owns it rather than being given a control
    // the coordinator would answer for.
    const detailSleepOwner = $derived.by(() => {
        const sp = detailSpeaker;
        if (!sp) return null;
        const g = groupOfSpeaker(sp.id);
        return g && g.coordinator_id !== sp.id
            ? (speakerById.get(g.coordinator_id)?.name ?? null)
            : null;
    });

    // An unreachable speaker has no settings to read, so its row goes where
    // the only useful action is instead: the registration form, which is
    // where a wrong address gets fixed. A form is a sheet, per §11.
    function openSpeaker(sp: SonosSpeakerView) {
        if (!sp.reachable) {
            void openSpeakerModal(sp);
            return;
        }
        kefDetailId = null; // one pane at a time
        detailId = sp.id;
    }

    // Speakers that turned out to have no picture of their own. Remembered per
    // id so a 404 isn't re-requested every time the list re-renders.
    let noImage = $state<Record<string, boolean>>({});
</script>

<svelte:window onkeydown={onWindowKey} onpopstate={onPopState} />

<!-- The live waveform — the music module's motif for "actually playing",
     used everywhere a plain status dot would otherwise sit. -->
{#snippet wave()}
    <span class="wave" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
{/snippet}

<!-- Anything a grouping gesture does that has no visible running commentary
     — the keyboard path especially — is said here instead. -->
<div class="sr-only" role="status" aria-live="polite">{liveMsg}</div>

{#if screen === "speakers"}
    <!-- ── Speakers — a screen, not a sheet ────────────────────────────
         Its rows open a speaker's settings one level further, and a sheet
         must never open another sheet. So it pushes from Home properly, with
         the §11 back chip that says so.

         On a phone an open speaker replaces the list, and that pane carries
         its own §11 head — two back chips on one screen would be one too
         many, so this one stands down. -->
    {#if !(anyDetail && !paned)}
        <div class="screen-head">
            <button class="icon-btn" aria-label="Back to Music" onclick={leaveSpeakers}>
                <Icon name="chevronLeft" size={18} />
            </button>
            <div class="screen-title">
                <h1>Speakers</h1>
                <span class="screen-sub">
                    <span class="mono">{totalSpeakers}</span>
                    registered · <span class="mono">{readyCount}</span> reachable
                </span>
            </div>
            <button class="icon-btn" aria-label="Add speaker" onclick={() => openSpeakerModal()}>
                <Icon name="plus" size={16} />
            </button>
        </div>
    {/if}
{:else}
    <Topbar
        title="Music"
        subtitle={status
            ? `${totalSpeakers} speaker${totalSpeakers === 1 ? "" : "s"} · ${playingCount + kefPlaying.length} playing`
            : "Sonos & KEF"}
    >
        {#snippet actions()}
            <!-- Whether speaker state is being pushed or polled. It rides in the
                 topbar because it qualifies everything below it — how quickly any
                 of this reflects reality — and it is the tap that explains the
                 difference and offers the fix. -->
            <!-- The chip reports the *Sonos* bridge's push status, so it only
                 appears when there are Sonos speakers for it to describe. KEF has
                 no notifications to subscribe to (its own API has none), and a
                 chip that said "Polling" about them would be reporting a fault
                 that doesn't exist. -->
            {#if loaded && (status?.speakers.length ?? 0) > 0}
                <LiveStatusChip live={livePush} onClosed={() => void refresh()} />
            {/if}
            <!-- Search rides in the header rather than in a subnav pill:
                 nothing sits below this header but content (DESIGN.md §15).
                 It wears its label wherever there is width for one — losing
                 the pill shouldn't cost desktop a *named* way in when the
                 room to name it was never the problem. On a phone the label
                 is what pushed the subtitle to a stub, so there it is the
                 icon alone. Registering a speaker isn't here at all: it
                 belongs on Speakers, with the rest of device management. -->
            {#if loaded && totalSpeakers > 0}
                <button class="chip act-search" onclick={openSearch}>
                    <Icon name="search" size={14} />
                    <span class="act-label">Search</span>
                </button>
            {:else}
                <button class="chip" onclick={() => openSpeakerModal()}>
                    <Icon name="plus" size={14} /> Add speaker
                </button>
            {/if}
        {/snippet}
    </Topbar>
{/if}

{#if !loaded}
    <section class="card"><div class="skeleton sk"></div></section>
{:else if totalSpeakers === 0}
    <EmptyState
        icon="speaker"
        title="No speakers yet"
        message="Add your Sonos or KEF speakers to control playback, volume and — on Sonos — grouping right here, with neither app needed."
    >
        <button class="btn btn-primary" onclick={() => openSpeakerModal()}>Add speaker</button>
    </EmptyState>
{/if}

{#if loaded && totalSpeakers > 0}
    {#if screen === "home"}
    <!-- ── Playing now ─────────────────────────────────────────────────
         Only what is actually playing. Idle zones are one tap away in the
         room chips below, so listing them here would just make the heading
         lie and bury the thing the user came for. -->
    <section class="block">
        <div class="eyrow">Playing now</div>
        {#if playingGroups.length === 0 && kefPlaying.length === 0}
            <div class="quiet-card">
                <span class="quiet-ico"><Icon name="speaker" size={20} /></span>
                <span class="quiet-meta">
                    <span class="quiet-title">Nothing playing</span>
                    <span class="quiet-sub">
                        <span class="mono">{readyCount}</span>
                        speaker{readyCount === 1 ? "" : "s"} ready —
                        {favorites.length > 0 && !kefTargetSpeaker
                            ? "start a favorite below"
                            : "pick a room to open it"}
                    </span>
                </span>
                {#if spotify}
                    <!-- Not gated on `connected`: the people who most need a
                         pointer at Spotify are the ones who haven't set it up,
                         and with the subnav gone this card and the header icon
                         are the only things that say the module searches at
                         all (DESIGN.md §15). -->
                    <button class="chip quiet-go" onclick={openSearch}>
                        {spotify.connected ? "Search" : "Set up Spotify"}
                    </button>
                {/if}
            </div>
        {:else}
            <div class="now-grid">
                {#each playingGroups as g (g.coordinator_id)}
                    {@const c = coordinatorOf(g)}
                    {@const st = c?.state}
                    {@const p = progressOf(g)}
                    <div
                        class="now-card playing"
                        use:dockAnchor={g.coordinator_id === dockGroup?.coordinator_id}
                        in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
                        out:fade={{ duration: dur(120) }}
                    >
                        <button class="now-open" onclick={() => openPlayer(g)}>
                            {#if st?.track?.art_uri}
                                <img class="now-art" src={st.track.art_uri} alt="" loading="lazy" />
                            {:else}
                                <div class="now-art placeholder">[ art ]</div>
                            {/if}
                            <span class="now-meta">
                                <span class="now-name" title={groupTitle(g)}>{groupTitle(g)}</span>
                                <span class="now-line">
                                    {@render wave()}
                                    <span class="now-track">
                                        {[st?.track?.title, st?.track?.artist].filter(Boolean).join(" · ")
                                            || "Live audio"}
                                    </span>
                                </span>
                            </span>
                        </button>
                        <!-- Skips ride along from 430px up, the same width
                             Home's card uses — a phone keeps play/pause and
                             gives the track title the room instead. -->
                        <div class="card-transport">
                            <button
                                class="mini-btn skip"
                                aria-label="Previous track"
                                disabled={!c || busy.is("previous:" + c?.id)}
                                onclick={() => skip(g, "previous")}
                            >
                                <Icon name="skipPrev" size={16} />
                            </button>
                            <button
                                class="mini-btn on"
                                aria-label={isPlaying(g) ? "Pause" : "Play"}
                                disabled={!c || busy.is("play:" + c?.id)}
                                onclick={() => togglePlay(g)}
                            >
                                <Icon name={isPlaying(g) ? "pause" : "play"} size={16} />
                            </button>
                            <button
                                class="mini-btn skip"
                                aria-label="Next track"
                                disabled={!c || busy.is("next:" + c?.id)}
                                onclick={() => skip(g, "next")}
                            >
                                <Icon name="skipNext" size={16} />
                            </button>
                        </div>
                        <!-- Where the track has got to, without opening
                             anything. Live streams report no duration and get
                             no line rather than a made-up one. -->
                        {#if p > 0}
                            <span class="prog" aria-hidden="true">
                                <i style:width="{p * 100}%"></i>
                            </span>
                        {/if}
                    </div>
                {/each}

                <!-- KEF speakers that are playing, in the same grid and with
                     the same card. It is a way in to a player like every
                     other card here — the sheet it opens drops the queue and
                     the group, which KEF hasn't got, and keeps the rest. -->
                {#each kefPlaying as sp (sp.id)}
                    {@const p = kefProgress(sp)}
                    <div
                        class="now-card playing"
                        in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
                        out:fade={{ duration: dur(120) }}
                    >
                        <button
                            class="now-open"
                            onclick={() => openKEFPlayer(sp)}
                        >
                            {#if sp.state?.track?.art_uri}
                                <img class="now-art" src={sp.state.track.art_uri} alt="" loading="lazy" />
                            {:else}
                                <div class="now-art placeholder">[ art ]</div>
                            {/if}
                            <span class="now-meta">
                                <span class="now-name" title={sp.name}>{sp.name}</span>
                                <span class="now-line">
                                    {@render wave()}
                                    <span class="now-track">
                                        {[kefNowLine(sp), kefSubLine(sp)].filter(Boolean).join(" · ")}
                                    </span>
                                </span>
                            </span>
                        </button>
                        <!-- Play/pause only, like the Sonos card below 430px:
                             the sheet is where the skips live. -->
                        <div class="card-transport">
                            <button
                                class="mini-btn on"
                                aria-label={kefIsPlaying(sp) ? "Pause" : "Play"}
                                disabled={busy.is("kefplay:" + sp.id)}
                                onclick={() => kefTogglePlay(sp)}
                            >
                                <Icon name={kefIsPlaying(sp) ? "pause" : "play"} size={16} />
                            </button>
                        </div>
                        {#if p > 0}
                            <span class="prog" aria-hidden="true">
                                <i style:width="{p * 100}%"></i>
                            </span>
                        {/if}
                    </div>
                {/each}
            </div>
        {/if}
    </section>

    <!-- ── Favorites ───────────────────────────────────────────────── -->
    {#if favorites.length > 0}
        <section class="block">
            <div class="block-head">
                <div class="eyrow">Favorites</div>
                {@render targetRow()}
            </div>
            {#if kefTargetSpeaker}
                <!-- "My Sonos" is a household list, and a KEF speaker has no
                     way to play an entry from it. A rail of disabled cards
                     would be a row of dead controls (§15), so the section
                     says what it needs instead — and the fix is one tap on
                     the destination row directly above. -->
                <div class="quiet-card">
                    <span class="quiet-ico"><Icon name="speaker" size={20} /></span>
                    <span class="quiet-meta">
                        <span class="quiet-title">Favorites need a Sonos room</span>
                        <span class="quiet-sub">
                            They come out of your Sonos household, so {kefTargetSpeaker.name} can't
                            play one — pick a Sonos room above{#if spotify?.connected}, or search to
                            play there{/if}.
                        </span>
                    </span>
                    {#if spotify}
                        <button class="chip quiet-go" onclick={openSearch}>
                            {spotify.connected ? "Search" : "Set up Spotify"}
                        </button>
                    {/if}
                </div>
            {:else}
                <div class="favs h-scroll">
                    {#each favorites as f (f.id)}
                        {@render favCard(f, sonosTarget)}
                    {/each}
                </div>
            {/if}
        </section>
    {/if}

    <!-- ── Zones at a glance (Home) ─────────────────────────────────
         "Zones", not "Rooms": the app-level nav already owns that word for
         the whole house, and reusing it here for speaker grouping was the
         confusing part (DESIGN.md §15). -->
    <section class="block">
        <div class="block-head">
            <div class="eyrow">Zones</div>
            <button class="link-btn" onclick={openZones}>Manage</button>
        </div>
        <div class="room-chips">
            {#each reachable as sp (sp.id)}
                {@const g = groupOfSpeaker(sp.id)}
                <button
                    class="room-chip"
                    class:on={speakerPlaying(sp.id)}
                    disabled={!g}
                    onclick={() => g && openPlayer(g)}
                >
                    {#if speakerPlaying(sp.id)}
                        {@render wave()}
                    {:else}
                        <Icon name="speaker" size={14} />
                    {/if}
                    <span>{sp.name}</span>
                </button>
            {/each}
            <!-- KEF speakers are rooms that play too, so they belong in this
                 row — and they open a player, like every chip beside them.
                 They are absent from Zones instead, which is honest: Zones
                 answers what plays together, and a KEF speaker never does. -->
            {#each kefReachable as sp (sp.id)}
                <button
                    class="room-chip"
                    class:on={kefIsPlaying(sp)}
                    onclick={() => openKEFPlayer(sp)}
                >
                    {#if kefIsPlaying(sp)}
                        {@render wave()}
                    {:else}
                        <Icon name="speaker" size={14} />
                    {/if}
                    <span>{sp.name}</span>
                </button>
            {/each}
        </div>
    </section>

    <!-- The way through to the device inventory. A plain row rather than a
         header icon, because what it opens is a screen — Speakers pushes,
         Search and Zones lift. -->
    <button class="lu-row" onclick={openSpeakers}>
        <span class="lu-ico"><Icon name="speaker" size={18} /></span>
        <span class="lu-meta">
            <span class="lu-title">Speakers</span>
            <span class="lu-sub">
                {#if offline.length > 0}
                    <span class="mono">{offline.length}</span>
                    unreachable — fix an address, or set one up
                {:else}
                    Names, addresses, tone and the status light
                {/if}
            </span>
        </span>
        <span class="lu-count mono">{totalSpeakers}</span>
        <span class="lu-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
    </button>

    {:else}
    <!-- ── Speakers — the device inventory and its settings ────────────
         Zones answers "what plays together"; this answers "what is each of
         these, and how is it set up".

         Two panes where the width allows, one where it doesn't: the list
         column stays put on desktop and the settings open beside it; on a
         phone the settings take over the screen and `has-detail` folds the
         list away. Not a sheet — its rows open one, and a sheet must never
         open another sheet; the content is also too long to spend its life
         at 92vh. -->
    <div class="sp-split" class:has-detail={anyDetail}>
    <div class="sp-col">
    {#if allSpeakers.length > 0}
    <section class="block">
        <div class="block-head">
            <!-- Named by bridge once there are two: "what is this thing and
                 how is it configured" has a different answer per protocol,
                 and the two lists don't interleave into anything meaningful. -->
            <div class="eyrow">{kefSpeakers.length > 0 ? "Sonos" : "Speakers"}</div>
            <span class="hint">
                <span class="mono">{reachable.length}</span>
                of <span class="mono">{allSpeakers.length}</span> reachable
            </span>
        </div>
        <div class="sp-list">
            <!-- One target per row, the §11 shape: chevron right, into that
                 speaker's settings. Editing its registration lives on the
                 detail's action chip rather than as a second control here. -->
            {#each allSpeakers as sp (sp.id)}
                {@const playing = speakerPlaying(sp.id)}
                <button
                    class="sp-row"
                    class:off={!sp.reachable}
                    class:sel={detailId === sp.id}
                    aria-current={detailId === sp.id ? "true" : undefined}
                    onclick={() => openSpeaker(sp)}
                >
                    <!-- The speaker's own portrait, served by the device.
                         No picture published means the striped placeholder
                         — never a guess at which model this is (§2). -->
                    {#if noImage[sp.id]}
                        <!-- §6.7's striped fill, without its caption: no
                             wording fits a 40px box, and the row's name
                             and model already say what this is. -->
                        <span class="shot placeholder" aria-hidden="true"></span>
                    {:else}
                        <img
                            class="shot"
                            src={api.sonosImageURL(sp.id)}
                            alt=""
                            loading="lazy"
                            onerror={() => (noImage[sp.id] = true)}
                        />
                    {/if}
                    <span class="sp-meta">
                        <span class="sp-name">{sp.name}</span>
                        <span class="sp-sub">
                            {#if !sp.reachable}
                                Unreachable · <span class="mono">{sp.ip}</span>
                            {:else}
                                {[sp.model, sp.room].filter(Boolean).join(" · ") || sp.ip}
                            {/if}
                        </span>
                    </span>
                    {#if playing}
                        {@render wave()}
                    {/if}
                    <span class="sp-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
                </button>
            {/each}
        </div>
        <p class="hint">
            Tone, night mode, the status light and the touch controls are the
            speaker's own settings — they stay set whatever is playing.
        </p>
    </section>
    {/if}

    <!-- ── KEF ─────────────────────────────────────────────────────────
         Its own list, not interleaved with the Sonos one: the row's sub-line
         means different things (a Sonos row leads with its zone, a KEF row
         with its input), and the screen each one opens answers a different
         set of questions. -->
    {#if kefSpeakers.length > 0}
    <section class="block">
        <div class="block-head">
            <div class="eyrow">KEF</div>
            <span class="hint">
                <span class="mono">{kefReachable.length}</span>
                of <span class="mono">{kefSpeakers.length}</span> reachable
            </span>
        </div>
        <div class="sp-list">
            {#each kefSpeakers as sp (sp.id)}
                <button
                    class="sp-row"
                    class:off={!sp.reachable}
                    class:sel={kefDetailId === sp.id}
                    aria-current={kefDetailId === sp.id ? "true" : undefined}
                    onclick={() => (sp.reachable ? openKEFSpeaker(sp) : openKEFModal(sp))}
                >
                    <!-- KEF publishes no picture of itself the way Sonos
                         does, so this is the §6.7 striped fill rather than a
                         stock photo that might show the wrong model (§2). -->
                    <span class="shot placeholder" aria-hidden="true"></span>
                    <span class="sp-meta">
                        <span class="sp-name">{sp.name}</span>
                        <span class="sp-sub">
                            {#if !sp.reachable}
                                Unreachable · <span class="mono">{sp.ip}</span>
                            {:else if !sp.state?.powered_on}
                                Standby · {[sp.model, sp.room].filter(Boolean).join(" · ") || sp.ip}
                            {:else}
                                {[kefSourceLabel(sp.state?.source), sp.model, sp.room]
                                    .filter(Boolean).join(" · ") || sp.ip}
                            {/if}
                        </span>
                    </span>
                    {#if kefIsPlaying(sp)}
                        {@render wave()}
                    {/if}
                    <span class="sp-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
                </button>
            {/each}
        </div>
        <p class="hint">
            KEF speakers stand alone — no grouping, no shared queue — so their
            input, volume and EQ all live on the speaker's own screen.
        </p>
    </section>
    {/if}

    <!-- ── Live updates ────────────────────────────────────────────────
         Speakers is where the devices are managed, so it is where the
         plumbing behind them belongs. The topbar chip says which state we're
         in; this row is the discoverable way in for someone who never
         noticed it. -->
    {#if allSpeakers.length > 0}
    <button class="lu-row" onclick={openEventsModal}>
        <span class="lu-ico" class:on={livePush}>
            <Icon name={livePush ? "bolt" : "radio"} size={18} />
        </span>
        <span class="lu-meta">
            <span class="lu-title">Live updates</span>
            <span class="lu-sub">
                {#if livePush}
                    Speakers push their changes — this app keeps up in real time
                {:else}
                    Speakers are being polled — changes take a few seconds to show
                {/if}
            </span>
        </span>
        <span class="lu-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
    </button>
    {/if}
    </div><!-- /.sp-col -->

    {#if detailSpeaker}
        <div class="sp-pane">
            <SonosSpeakerDetail
                speaker={detailSpeaker}
                sleepTimerOwner={detailSleepOwner}
                {paned}
                siblings={detailSiblings}
                onPick={(id) => (detailId = id)}
                onBack={() => (detailId = null)}
                onEdit={() => void openSpeakerModal(detailSpeaker)}
            />
        </div>
    {:else if kefDetailSpeaker}
        <div class="sp-pane">
            <KEFSpeakerDetail
                speaker={kefDetailSpeaker}
                {paned}
                siblings={kefDetailSiblings}
                onPick={(id) => (kefDetailId = id)}
                onBack={() => (kefDetailId = null)}
                onEdit={() => void openKEFModal(kefDetailSpeaker)}
                onChanged={() => void refreshKEF()}
            />
        </div>
    {/if}
    </div><!-- /.sp-split -->

    {/if}

    <!-- ── Docked mini-player ──────────────────────────────────────────
         Present everywhere — including over the Zones and Search sheets,
         which is where the transport would otherwise disappear — but stands
         down while the Home card it would duplicate is on screen. It also
         survives a pause: that is where a paused zone stays reachable once
         "Playing now" (which means playing, literally) has let go of it. -->
    {#if showDock && dockGroup}
        {@const c = coordinatorOf(dockGroup)}
        {@const st = c?.state}
        {@const dockPlaying = isPlaying(dockGroup)}
        {@const p = progressOf(dockGroup)}
        <div class="mini" class:paused={!dockPlaying} class:over-sheet={overSheet}
            transition:fly={{ y: 20, duration: dur(220), easing: cubicOut }}>
            <button class="mini-open" onclick={() => openPlayer(dockGroup)}>
                {#if st?.track?.art_uri}
                    <img class="mini-art" src={st.track.art_uri} alt="" loading="lazy" />
                {:else}
                    <div class="mini-art placeholder"></div>
                {/if}
                <div class="mini-meta">
                    <div class="mini-t">{st?.track?.title ?? "Playing"}</div>
                    <div class="mini-s">
                        {[st?.track?.artist, groupTitle(dockGroup)].filter(Boolean).join(" · ")}
                    </div>
                </div>
                <!-- Playing is a waveform; a zone the dock is holding open
                     after a pause gets the idle speaker icon instead. -->
                {#if dockPlaying}
                    {@render wave()}
                {:else}
                    <span class="mini-idle" aria-hidden="true"><Icon name="speaker" size={14} /></span>
                {/if}
            </button>
            <div class="card-transport">
                <button class="mini-btn skip" aria-label="Previous track"
                    disabled={!c || busy.is("previous:" + c?.id)}
                    onclick={() => skip(dockGroup, "previous")}>
                    <Icon name="skipPrev" size={16} />
                </button>
                <button class="mini-btn on" aria-label={dockPlaying ? "Pause" : "Play"}
                    disabled={!c || busy.is("play:" + c?.id)}
                    onclick={() => togglePlay(dockGroup)}>
                    <Icon name={dockPlaying ? "pause" : "play"} size={16} />
                </button>
                <button class="mini-btn skip" aria-label="Next track"
                    disabled={!c || busy.is("next:" + c?.id)}
                    onclick={() => skip(dockGroup, "next")}>
                    <Icon name="skipNext" size={16} />
                </button>
            </div>
            {#if p > 0}
                <span class="prog" aria-hidden="true"><i style:width="{p * 100}%"></i></span>
            {/if}
        </div>
    {/if}
{/if}

<!-- ── Room puck ─────────────────────────────────────────────────────
     One object, two gestures on the same target: tap opens that room's
     player, drag it onto another room groups them. There is no second
     control — dragging one thing onto another *is* the grouping gesture, so
     the select circle that used to sit in the corner has nothing left to say.

     The keyboard gets the gesture too, since a pointer-only one would leave
     grouping with no path at all: G picks the room up, Tab moves, Enter
     drops it in. -->
{#snippet puck(sp: SonosSpeakerView)}
    {@const playing = speakerPlaying(sp.id)}
    {@const g = groupOfSpeaker(sp.id)}
    {@const held = grabId === sp.id || puckDrag?.id === sp.id}
    {@const target = dropId === sp.id || (grabId !== null && grabId !== sp.id)}
    <button
        class="puck"
        class:playing
        class:held
        class:lifted={puckDrag?.id === sp.id}
        class:drop={dropId === sp.id}
        class:aiming={grabId !== null && grabId !== sp.id}
        data-speaker={sp.id}
        disabled={!g}
        aria-keyshortcuts="g"
        aria-label={grabId !== null && grabId !== sp.id
            ? `Group ${grabbedName} with ${sp.name}`
            : `${sp.name} — open player, or press G to pick it up for grouping`}
        onpointerdown={(e) => onPuckPointerDown(e, sp)}
        onpointermove={onPuckPointerMove}
        onpointerup={onPuckPointerUp}
        onpointercancel={endPuckDrag}
        onclickcapture={onPuckClickCapture}
        onkeydown={(e) => onPuckKey(e, sp)}
        onclick={() => g && openPlayer(g)}
    >
        <span class="puck-icon">
            {#if playing}{@render wave()}{:else}<Icon name="speaker" size={16} />{/if}
        </span>
        <!-- Says "this object moves", on hover only and to a pointer only:
             touch has the press-and-hold to discover, and a mouse has
             nothing but the cursor otherwise. Not a control — it takes no
             pointer events, so it can't be mistaken for the select circle
             that used to sit here. -->
        <span class="puck-grip" aria-hidden="true"><Icon name="grip" size={14} /></span>
        <span class="puck-body">
            <span class="puck-name">{sp.name}</span>
            <span class="puck-sub">
                {#if target && grabId !== null}
                    Drop {grabbedName} here
                {:else if held}
                    Held
                {:else}
                    {speakerNowLine(sp.id)}
                {/if}
            </span>
        </span>
    </button>
{/snippet}

<!-- ── Where playback lands ──────────────────────────────────────────
     One destination shared by favorites and search, always visible — a
     single room shows its name rather than hiding the answer entirely. Both
     bridges are in the same row because there is only ever one destination;
     the KEF speakers come after the Sonos zones behind one marker, so a name
     that exists on both sides is still tellable apart without giving every
     chip a badge it doesn't need. -->
{#snippet targetRow()}
    {#if destinations.length > 1}
        <div class="fav-targets" role="radiogroup" aria-label="Play on">
            <span class="t-label">Play on</span>
            {#each destinations as d, i (d.kind + d.id)}
                {#if i === groups.length && groups.length > 0}
                    <span class="t-label">KEF</span>
                {/if}
                {@const on = isDest(d)}
                <button class="chip" class:on role="radio" aria-checked={on}
                    aria-label={`Play on ${destName(d)}${d.kind === "kef" ? " (KEF)" : ""}`}
                    onclick={() => (dest = d)}>
                    {destName(d)}
                </button>
            {/each}
        </div>
    {:else if destinations.length === 1}
        <div class="fav-targets">
            <span class="t-label">Play on</span>
            <span class="t-one">{destName(destinations[0])}</span>
        </div>
    {/if}
{/snippet}

{#snippet searchBody()}
    <!-- ── Spotify search ──────────────────────────────────────────── -->
    {#if spotify}
        <section class="card">
            {#if !spotify.configured || spotifySetup}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Search Spotify's catalog and play straight to your speakers.
                    One-time setup — playback itself uses the Spotify account
                    already linked to your Sonos.
                </p>
                <ol class="sp-steps">
                    <li>
                        <a class="sp-link" href="https://developer.spotify.com/dashboard"
                            target="_blank" rel="noopener noreferrer">Open the Spotify dashboard</a>
                        and create an app (any name, "Web API" is enough).
                    </li>
                    <li>
                        Give the app this Redirect URI:
                        <span class="sp-redirect">
                            <code class="mono">{spotify.redirect_uri}</code>
                            <button type="button" class="chip" onclick={copyRedirect}>
                                <Icon name={copied ? "check" : "copy"} size={13} />
                                {copied ? "Copied" : "Copy"}
                            </button>
                        </span>
                    </li>
                    <li>Paste the app's Client ID here:</li>
                </ol>
                <form class="sp-config" onsubmit={(e) => { e.preventDefault(); saveClientId(); }}>
                    <input type="text" class="mono" placeholder="Client ID"
                        aria-label="Spotify client ID" bind:value={clientId} />
                    <button type="submit" class="btn btn-primary" disabled={spotifySaving || !clientId.trim()}>
                        {spotifySaving ? "Saving…" : "Save"}
                    </button>
                    {#if spotifySetup}
                        <button type="button" class="btn btn-ghost" onclick={() => (spotifySetup = false)}>Cancel</button>
                    {/if}
                </form>
            {:else if !spotify.connected}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Client ID saved — now connect your Spotify account. You'll
                    approve access once on Spotify's page{spotify.manual
                        ? "; it opens in a new tab and ends on an unreachable 127.0.0.1 address — that's expected."
                        : ", then land back here."}
                </p>
                <div class="sp-actions">
                    <button class="btn btn-primary" onclick={connectSpotify}>Connect Spotify</button>
                    <button class="btn btn-ghost" onclick={() => { clientId = ""; spotifySetup = true; }}>
                        Change client ID
                    </button>
                </div>
                {#if spotify.manual}
                    <div class="field sp-paste">
                        <label for="sp-paste-input">
                            After approving, copy the full address from that tab and paste it here to finish:
                        </label>
                        <div class="sp-config">
                            <input id="sp-paste-input" type="text" class="mono"
                                placeholder="http://127.0.0.1:…/api/spotify/callback?code=…"
                                bind:value={pasteUrl} />
                            <button type="button" class="btn btn-primary"
                                disabled={finishing || !pasteUrl.trim()} onclick={finishConnect}>
                                {finishing ? "Finishing…" : "Finish"}
                            </button>
                        </div>
                    </div>
                {/if}
            {:else}
                <!-- No <h2> here: the sheet's own head already says
                     "Search". This row only answers "as whom". -->
                <div class="card-header sp-head">
                    <div class="sp-account">
                        <span class="sp-conn" title="Connected to Spotify">
                            <span class="sp-dot" aria-hidden="true"></span>
                            <span class="sp-conn-label">Connected</span>
                            <span class="sp-user mono">{spotify.display_name || "Spotify"}</span>
                        </span>
                        <button class="chip" onclick={disconnectSpotify}
                            aria-label="Disconnect Spotify">Disconnect</button>
                    </div>
                </div>
                <div class="sp-search">
                    <Icon name="search" size={16} />
                    <input
                        type="text"
                        class="sp-input"
                        placeholder="Songs, albums, playlists…"
                        aria-label="Search Spotify"
                        autocomplete="off"
                        enterkeyhint="search"
                        bind:this={searchEl}
                        bind:value={query}
                        oninput={onQueryInput}
                        onkeydown={onQueryKey}
                    />
                    {#if query}
                        <button class="icon-btn sp-clear" aria-label="Clear search" onclick={clearQuery}>
                            <Icon name="close" size={14} />
                        </button>
                    {/if}
                </div>
                {#if !query && !results && historyList.length > 0}
                    <div class="sp-history">
                        <div class="sp-history-head">
                            <span class="sp-browse-label">
                                Recent searches{#if destinations.length > 1 && destLabel} · {destLabel}{/if}
                            </span>
                            <button type="button" class="chip sp-hist-clear" onclick={clearHistory}>Clear</button>
                        </div>
                        <div class="sp-history-list">
                            {#each historyList as h (h)}
                                <div class="sp-hist-chip">
                                    <button type="button" class="sp-hist-run" onclick={() => runHistoryQuery(h)}>
                                        <Icon name="search" size={12} />
                                        <span>{h}</span>
                                    </button>
                                    <button type="button" class="icon-btn sp-hist-x"
                                        aria-label={`Remove "${h}" from recent searches`}
                                        onclick={() => removeHistoryEntry(h)}>
                                        <Icon name="close" size={10} />
                                    </button>
                                </div>
                            {/each}
                        </div>
                    </div>
                {/if}
                <div class="sp-filters">
                    {#if results}
                        <button class="chip" class:active={kindFilter === "tracks"} onclick={() => (kindFilter = "tracks")}>Songs</button>
                        <button class="chip" class:active={kindFilter === "albums"} onclick={() => (kindFilter = "albums")}>Albums</button>
                        <button class="chip" class:active={kindFilter === "playlists"} onclick={() => (kindFilter = "playlists")}>Playlists</button>
                    {:else if myPlaylists.length > 0}
                        <span class="sp-browse-label">Your playlists</span>
                    {/if}
                    <div class="sp-targets" class:pushed={!!results}>{@render targetRow()}</div>
                </div>
                <!-- Playing on a KEF speaker goes out through Spotify Connect,
                     which needs a permission this login may predate. Saying so
                     before the tap beats a 409 after it, and reconnecting is
                     the only thing that fixes it. -->
                {#if kefTargetSpeaker && spotify && !spotify.playback}
                    <div class="sp-note">
                        <Icon name="info" size={14} />
                        <span>
                            Reconnect Spotify to start music on {kefTargetSpeaker.name} —
                            this login was made before HomeHub could ask for that.
                        </span>
                        <button class="chip" onclick={connectSpotify}>Reconnect</button>
                    </div>
                {/if}
                {#if searching}
                    <div class="skeleton sp-skeleton"></div>
                {:else if results && shownItems.length === 0}
                    <div class="sp-none">No {kindFilter} matched "{query.trim()}".</div>
                {:else if !results && shownItems.length === 0}
                    <!-- No query and no playlists to browse — say what this
                         box does rather than leaving a blank panel. -->
                    <div class="sp-none">
                        Search Spotify for a song, album or playlist. Tapping a result
                        plays it on the room shown above{#if !kefTargetSpeaker}; the row's
                        overflow menu queues it without interrupting{/if}.
                    </div>
                {:else}
                    <div class="sp-results">
                        {#each shownItems as item (item.uri)}
                            <div class="sp-row">
                                <button class="sp-open" disabled={busy.is("item:" + item.uri) || !dest}
                                    onclick={() => playItem(item)}>
                                    {#if item.art_url}
                                        <img class="sp-art" src={item.art_url} alt="" loading="lazy" />
                                    {:else}
                                        <div class="sp-art placeholder">[ art ]</div>
                                    {/if}
                                    <span class="sp-meta">
                                        <span class="sp-name">{item.name}</span>
                                        {#if item.sub}<span class="sp-sub">{item.sub}</span>{/if}
                                    </span>
                                    <span class="sp-play"><Icon name="play" size={16} /></span>
                                </button>
                                <!-- Tapping the row plays now; queueing without
                                     interrupting lives behind the overflow —
                                     and only for a Sonos destination, since
                                     the queue is a Sonos group's. A KEF
                                     speaker has none, so the control that
                                     would be refused isn't there at all. -->
                                {#if sonosTarget}
                                    <button class="icon-btn sp-more" aria-label="More for {item.name}"
                                        aria-haspopup="menu" aria-expanded={menuFor === item.uri}
                                        disabled={busy.is("q:" + item.uri)}
                                        onclick={(e) => toggleMenu(e, item.uri)}>
                                        <Icon name="more" size={16} />
                                    </button>
                                {/if}
                                {#if menuFor === item.uri}
                                    <div class="overflow-menu" role="menu" use:menuNav
                                        in:scale={{ start: 0.95, duration: dur(140), easing: cubicOut, opacity: 0 }}
                                        out:scale={{ start: 0.95, duration: dur(100), easing: cubicOut, opacity: 0 }}>
                                        <button class="overflow-item" role="menuitem"
                                            onclick={() => enqueue({ service: "Spotify", uri: item.uri, title: item.name }, true)}>
                                            <Icon name="skipNext" size={16} /><span>Play next</span>
                                        </button>
                                        <button class="overflow-item" role="menuitem"
                                            onclick={() => enqueue({ service: "Spotify", uri: item.uri, title: item.name }, false)}>
                                            <Icon name="queue" size={16} /><span>Add to queue</span>
                                        </button>
                                    </div>
                                {/if}
                            </div>
                        {/each}
                    </div>
                {/if}
            {/if}
        </section>
    {/if}
{/snippet}

<!-- ── Sheet chrome ─────────────────────────────────────────────────
     The grabber + centered head every one of Music's sheets wears, so Zones,
     Search and the player all read as the same object and answer the same
     swipe. §5: a sheet must look dismissible at a glance. -->
{#snippet sheetHead(title: string, sub: string)}
    <div
        class="sheet-top"
        role="none"
        onpointerdown={onTopPointerDown}
        onpointermove={onTopPointerMove}
        onpointerup={onTopPointerUp}
        onpointercancel={cancelDrag}
    >
        <div class="grabber" aria-hidden="true"></div>
        <header class="player-head">
            <button class="icon-btn p-icon" aria-label="Close {title}" onclick={dropSheet}>
                <Icon name="chevronDown" size={18} />
            </button>
            <div class="p-onair">
                <div class="p-onair-name">{title}</div>
                {#if sub}<div class="p-onair-sub">{sub}</div>{/if}
            </div>
            <!-- Balances the close button so the title stays centered. -->
            <span class="p-icon-gap" aria-hidden="true"></span>
        </header>
    </div>
{/snippet}

<!-- ── Zones sheet ──────────────────────────────────────────────────
     Grouping, and only grouping: what plays together. Opens over Home the
     same way the player does, and swaps to the player when a room is tapped
     rather than stacking a second sheet on top of itself. -->
{#if zonesOpen}
    <div class="scrim" transition:fade={{ duration: dur(200) }} onclick={dropSheet} aria-hidden="true"></div>
    <div
        class="sheet"
        class:dragging
        role="dialog"
        aria-modal="true"
        aria-label="Zones"
        tabindex="-1"
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ""}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging
            ? "none"
            : dragY > 0
              ? "transform 0.22s ease-in, opacity 0.22s ease-in"
              : "transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)"}
        in:sheet={{}}
        out:sheet={{ instant: dismissing }}
    >
        <div
            class="sheet-scroll"
            class:docked={showDock}
            role="none"
            bind:this={scrollEl}
            onpointerdown={onBodyPointerDown}
            onpointermove={onBodyPointerMove}
            onpointerup={onBodyPointerUp}
            onpointercancel={onBodyPointerCancel}
        >
            {@render sheetHead("Zones", "Tap a room to open it · drag one onto another to group")}

            <div class="rooms">
                {#each multiGroups as g (g.coordinator_id)}
                    <!-- The enclosure is a drop target in its own right:
                         "drag a third onto an existing group" reads as
                         dropping on the group, so the gap between its pucks
                         must not be a miss. -->
                    <div class="group-wrap" class:drop={dropZone === g.coordinator_id}
                        data-zone={g.coordinator_id}>
                        <div class="glabel">
                            <Icon name="check" size={11} />
                            <span>{groupTitle(g)}</span>
                            <button class="ungroup" disabled={busy.is("ungroup:" + g.coordinator_id)}
                                onclick={() => ungroup(g)}>Ungroup</button>
                        </div>
                        <div class="puck-grid">
                            {#each g.member_ids as id (id)}
                                {@const sp = speakerById.get(id)}
                                {#if sp}
                                    {@render puck(sp)}
                                {/if}
                            {/each}
                        </div>
                    </div>
                {/each}
                {#if soloSpeakers.length}
                    <div class="puck-grid">
                        {#each soloSpeakers as sp (sp.id)}
                            {@render puck(sp)}
                        {/each}
                    </div>
                {/if}
                <!-- Grouping is a Sonos capability. A house with only KEF
                     speakers would otherwise open a blank sheet with no
                     explanation, which reads as broken rather than as
                     "this doesn't apply to your speakers". -->
                {#if multiGroups.length === 0 && soloSpeakers.length === 0}
                    <div class="quiet-card">
                        <span class="quiet-ico"><Icon name="speaker" size={20} /></span>
                        <span class="quiet-meta">
                            <span class="quiet-title">Nothing to group</span>
                            <span class="quiet-sub">
                                {#if kefSpeakers.length > 0}
                                    KEF speakers stand alone — they have no zones to
                                    group. Their controls are on Speakers.
                                {:else}
                                    No Sonos speaker is answering right now — check
                                    them under Speakers.
                                {/if}
                            </span>
                        </span>
                        <button class="chip quiet-go" onclick={openSpeakers}>Speakers</button>
                    </div>
                {/if}

                <!-- Speakers the live topology never mentioned can't be pucks
                     and can't be grouped, so Zones only points at them. -->
                {#if offline.length > 0}
                    <button class="lu-row" onclick={openSpeakers}>
                        <span class="lu-ico"><Icon name="speaker" size={18} /></span>
                        <span class="lu-meta">
                            <span class="lu-title">
                                <span class="mono">{offline.length}</span>
                                speaker{offline.length === 1 ? "" : "s"} unreachable
                            </span>
                            <span class="lu-sub">
                                Not in the current Sonos topology — check them under Speakers
                            </span>
                        </span>
                        <span class="lu-chev" aria-hidden="true"><Icon name="chevronDown" size={18} /></span>
                    </button>
                {/if}

                <p class="hint zones-keys">
                    On a keyboard: press <kbd>G</kbd> on a room to pick it up,
                    <kbd>Tab</kbd> to another and <kbd>Enter</kbd> to group them.
                </p>
            </div>
        </div>
    </div>
{/if}

<!-- Drag ghost — a copy of the puck under the finger. Fixed to the viewport
     and outside the sheet's own scroll, so it never clips at the edges. -->
{#if puckDrag}
    <div
        class="puck puck-ghost"
        class:playing={puckDrag.playing}
        aria-hidden="true"
        style:width="{puckDrag.w}px"
        style:height="{puckDrag.h}px"
        style:left="{puckDrag.x}px"
        style:top="{puckDrag.y}px"
    >
        <span class="puck-icon">
            {#if puckDrag.playing}{@render wave()}{:else}<Icon name="speaker" size={16} />{/if}
        </span>
        <span class="puck-body">
            <span class="puck-name">{puckDrag.name}</span>
            <span class="puck-sub">{puckDrag.sub}</span>
        </span>
    </div>
{/if}

<!-- ── Search sheet ─────────────────────────────────────────────────
     Behind a plain search icon in Home's header, opening the same way
     everything else in Music opens. -->
{#if searchOpen}
    <div class="scrim" transition:fade={{ duration: dur(200) }} onclick={dropSheet} aria-hidden="true"></div>
    <div
        class="sheet"
        class:dragging
        role="dialog"
        aria-modal="true"
        aria-label="Search"
        tabindex="-1"
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ""}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging
            ? "none"
            : dragY > 0
              ? "transform 0.22s ease-in, opacity 0.22s ease-in"
              : "transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)"}
        in:sheet={{}}
        out:sheet={{ instant: dismissing }}
    >
        <div
            class="sheet-scroll"
            class:docked={showDock}
            role="none"
            bind:this={scrollEl}
            onpointerdown={onBodyPointerDown}
            onpointermove={onBodyPointerMove}
            onpointerup={onBodyPointerUp}
            onpointercancel={onBodyPointerCancel}
        >
            {@render sheetHead("Search", spotify?.connected ? "Spotify" : "")}
            {@render searchBody()}
        </div>
    </div>
{/if}

<!-- ── KEF player sheet ─────────────────────────────────────────────
     The same object as the Sonos player, minus the two things KEF hasn't
     got: a queue and a group. What it has instead is the input selector,
     which is the question a KEF speaker actually raises. Every room chip on
     Home now opens a player — the chips sit side by side and looked
     identical, so sending one of them to a settings screen two levels away
     was the module's worst seam (DESIGN.md §15). -->
{#if playerOpen && activeKef}
    {@const sp = activeKef}
    {@const st = sp.state}
    {@const p = kefProgress(sp)}
    {@const durMs = st?.duration_ms ?? 0}
    <div class="scrim" transition:fade={{ duration: dur(200) }} onclick={closePlayer} aria-hidden="true"></div>
    <div
        class="sheet"
        class:dragging
        role="dialog"
        aria-modal="true"
        aria-label="Now playing"
        tabindex="-1"
        bind:this={playerEl}
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ""}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging
            ? "none"
            : dragY > 0
              ? "transform 0.22s ease-in, opacity 0.22s ease-in"
              : "transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)"}
        in:sheet={{}}
        out:sheet={{ instant: dismissing }}
    >
        <div
            class="sheet-scroll"
            role="none"
            bind:this={scrollEl}
            onpointerdown={onBodyPointerDown}
            onpointermove={onBodyPointerMove}
            onpointerup={onBodyPointerUp}
            onpointercancel={onBodyPointerCancel}
        >
            <div
                class="sheet-top"
                role="none"
                onpointerdown={onTopPointerDown}
                onpointermove={onTopPointerMove}
                onpointerup={onTopPointerUp}
                onpointercancel={cancelDrag}
            >
                <div class="grabber" aria-hidden="true"></div>
                <header class="player-head">
                    <button class="icon-btn p-icon" aria-label="Collapse player" onclick={closePlayer}>
                        <Icon name="chevronDown" size={18} />
                    </button>
                    <div class="p-onair">
                        <div class="eyrow">Playing on</div>
                        <div class="p-onair-name">{sp.name}</div>
                    </div>
                    <!-- Tone, EQ and the rest are the speaker's own settings,
                         and they live on its screen. This is the way there,
                         and the sheet stands down so a screen can push. -->
                    <button
                        class="icon-btn p-icon"
                        aria-label="{sp.name} settings"
                        onclick={() => { hideSheet(); openSpeakers(); openKEFSpeaker(sp); }}
                    >
                        <Icon name="sliders" size={17} />
                    </button>
                </header>
            </div>

            <div class="p-art">
                {#if st?.track?.art_uri}
                    <img src={st.track.art_uri} alt="" draggable="false" />
                {:else}
                    <div class="p-art-ph">[ album art ]</div>
                {/if}
            </div>

            <div class="p-meta">
                {#if st?.track?.title}
                    <div class="p-title">{st.track.title}</div>
                    <div class="p-sub">
                        {[st.track.artist, st.track.album].filter(Boolean).join(" · ")
                            || kefSourceLabel(st.source)}
                    </div>
                {:else if !st?.powered_on}
                    <div class="p-title idle">Standby</div>
                    <div class="p-sub">Press play to wake it.</div>
                {:else}
                    <div class="p-title idle">{kefNowLine(sp)}</div>
                    <div class="p-sub">Pick an input below, or search Spotify.</div>
                {/if}
            </div>

            <!-- A read-only line, not a scrubber: KEF's API has no seek. The
                 physical inputs and live streams report no duration at all
                 and get no line rather than a made-up one — the same rule
                 Sonos radio follows one sheet over. -->
            {#if durMs > 0}
                <div class="p-scrub">
                    <span class="kef-rail" aria-hidden="true"><i style:width="{p * 100}%"></i></span>
                    <div class="p-times mono">
                        <span>{fmtSecs(kefPosMs(sp) / 1000)}</span><span>{fmtSecs(durMs / 1000)}</span>
                    </div>
                </div>
            {:else if st?.track?.title}
                <div class="p-live mono">no track position on this input</div>
            {/if}

            <div class="p-transport">
                <button class="icon-btn t-btn" aria-label="Previous track"
                    disabled={busy.is("kefprevious:" + sp.id)} onclick={() => kefSkip(sp, "previous")}>
                    <Icon name="skipPrev" size={22} />
                </button>
                <button class="p-play" class:playing={kefIsPlaying(sp)}
                    aria-label={kefIsPlaying(sp) ? "Pause" : "Play"} title="Play / pause (space)"
                    disabled={busy.is("kefplay:" + sp.id)} onclick={() => kefTogglePlay(sp)}>
                    <Icon name={kefIsPlaying(sp) ? "pause" : "play"} size={26} />
                </button>
                <button class="icon-btn t-btn" aria-label="Next track"
                    disabled={busy.is("kefnext:" + sp.id)} onclick={() => kefSkip(sp, "next")}>
                    <Icon name="skipNext" size={22} />
                </button>
            </div>

            <!-- Spotify Connect is the only road content takes to a KEF
                 speaker, so this row is the whole of "play something else"
                 here — and it sits above the input selector, which answers
                 the same question for the physical inputs. -->
            {@render startSomething(null)}

            <div class="p-speakers">
                <div class="eyrow">Volume</div>
                <div class="member">
                    <button class="icon-btn m-mute" aria-label={st?.muted ? "Unmute" : "Mute"}
                        aria-pressed={st?.muted ?? false}
                        disabled={busy.is("kefmute:" + sp.id)} onclick={() => kefToggleMute(sp)}>
                        <Icon name={st?.muted ? "volumeOff" : "volume"} size={17} />
                    </button>
                    <span class="m-name" class:muted={st?.muted}>{sp.name}</span>
                    <input type="range" min="0" max="100" step="1"
                        aria-label="Volume for {sp.name}"
                        value={kefShownVol(sp)}
                        oninput={(e) => (kefVol[sp.id] = e.currentTarget.valueAsNumber)}
                        onchange={(e) => kefSetVolume(sp, e.currentTarget.valueAsNumber)} />
                    <span class="vol-num mono">{kefShownVol(sp)}</span>
                </div>
            </div>

            <!-- The question a KEF speaker raises that a Sonos zone doesn't:
                 which input. It sits where the group's member volumes sit on
                 the other sheet, because it answers the same "where is this
                 coming out" question. -->
            <div class="p-speakers">
                <div class="eyrow">Input</div>
                <div class="src-row">
                    {#each KEF_SOURCES as src (src.value)}
                        <button class="chip" class:on={st?.source === src.value}
                            aria-pressed={st?.source === src.value}
                            disabled={busy.is("kefsrc:" + sp.id)}
                            onclick={() => kefSetSource(sp, src.value)}>{src.label}</button>
                    {/each}
                </div>
                <p class="hint">
                    No queue and no grouping — a KEF speaker plays alone, so
                    there is nothing to line up behind this or to play it with.
                </p>
            </div>
        </div>
    </div>
{/if}

<!-- ── Full player sheet ───────────────────────────────────────────── -->
{#if playerOpen && activeGroup}
    {@const g = activeGroup}
    {@const c = coordinatorOf(g)}
    {@const st = c?.state}
    {@const gs = c?.group_state}
    {@const grouped = g.member_ids.length > 1}
    <div class="scrim" transition:fade={{ duration: dur(200) }} onclick={closePlayer} aria-hidden="true"></div>
    <div
        class="sheet"
        class:dragging
        role="dialog"
        aria-modal="true"
        aria-label="Now playing"
        tabindex="-1"
        bind:this={playerEl}
        style:transform={dragY > 0 ? `translateY(${dragY}px)` : ""}
        style:opacity={dragY > 0 ? Math.max(0.4, 1 - dragY / 300) : undefined}
        style:transition={dragging
            ? "none"
            : dragY > 0
              ? "transform 0.22s ease-in, opacity 0.22s ease-in"
              : "transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1)"}
        in:sheet={{}}
        out:sheet={{ instant: dismissing }}
    >
        <div
            class="sheet-scroll"
            role="none"
            bind:this={scrollEl}
            onpointerdown={onBodyPointerDown}
            onpointermove={onBodyPointerMove}
            onpointerup={onBodyPointerUp}
            onpointercancel={onBodyPointerCancel}
        >
            <!-- Grabber + close X, per DESIGN.md §5 — the sheet must read as
                 dismissible at a glance, not only via the collapse chevron.
                 The bar is also the drag handle, and it sticks as one unit so
                 content dissolves under it rather than meeting a hard edge. -->
            <div
                class="sheet-top"
                role="none"
                onpointerdown={onTopPointerDown}
                onpointermove={onTopPointerMove}
                onpointerup={onTopPointerUp}
                onpointercancel={cancelDrag}
            >
                <div class="grabber" aria-hidden="true"></div>
                <header class="player-head">
                    <button
                        class="icon-btn p-icon"
                        aria-label={queuePane ? "Back to now playing" : "Collapse player"}
                        onclick={() => (queuePane ? (queuePane = false) : closePlayer())}
                    >
                        <Icon name={queuePane ? "chevronLeft" : "chevronDown"} size={18} />
                    </button>
                    <div class="p-onair">
                        <div class="eyrow">{queuePane ? "Queue" : "Playing on"}</div>
                        <div class="p-onair-name">{groupTitle(g)}</div>
                    </div>
                    <button class="icon-btn p-icon" aria-label="Close player" onclick={closePlayer}>
                        <Icon name="close" size={18} />
                    </button>
                </header>
            </div>

            {#if queuePane}
                <!-- ── Queue pane ──────────────────────────────────────── -->
                <div class="q-bar">
                    <span class="q-total mono">
                        {gs?.queue_length ?? queue.length}
                        {(gs?.queue_length ?? queue.length) === 1 ? "track" : "tracks"}
                    </span>
                    <button class="chip" disabled={!c || busy.is("qclear:" + c?.id) || queue.length === 0}
                        onclick={() => clearQueue(g)}>Clear</button>
                </div>

                {#if queueLoading}
                    <div class="skeleton q-skeleton"></div>
                {:else if queue.length === 0}
                    <p class="q-none">
                        Nothing queued. Play a favorite or a Spotify result and it lands here —
                        radio and line-in play straight through without a queue.
                    </p>
                {:else}
                    <div class="q-list">
                        {#each queue as item (item.track)}
                            {@const current = item.track === st?.queue_track}
                            <div class="q-row" class:current>
                                <button class="q-open" disabled={busy.is("jump:" + item.track)}
                                    onclick={() => jumpTo(g, item.track)}>
                                    <span class="q-num mono">
                                        {#if current && st?.playing}
                                            {@render wave()}
                                        {:else}
                                            {item.track}
                                        {/if}
                                    </span>
                                    <span class="q-meta">
                                        <span class="q-title">{item.title || "Unknown track"}</span>
                                        {#if item.artist}<span class="q-sub">{item.artist}</span>{/if}
                                    </span>
                                    {#if item.duration}
                                        <span class="q-dur mono">{trimClock(item.duration)}</span>
                                    {/if}
                                </button>
                                <button class="icon-btn q-rm"
                                    aria-label="Remove {item.title || 'track ' + item.track} from the queue"
                                    disabled={busy.is("qrm:" + item.track)} onclick={() => removeQueued(g, item.track)}>
                                    <Icon name="close" size={14} />
                                </button>
                            </div>
                        {/each}
                    </div>
                    {#if (gs?.queue_length ?? 0) > queue.length}
                        <div class="q-more mono">
                            showing the first {queue.length} of {gs?.queue_length}
                        </div>
                    {/if}
                {/if}
            {:else}
                <!-- ── Now playing ─────────────────────────────────────── -->
                <!-- Drag the art sideways to change track. -->
                <div
                    class="p-art"
                    class:swiping={artSwiping}
                    role="none"
                    onpointerdown={onArtPointerDown}
                    onpointermove={onArtPointerMove}
                    onpointerup={onArtPointerUp}
                    onpointercancel={onArtPointerCancel}
                    style:transform={artDX ? `translateX(${artDX}px)` : ""}
                    style:opacity={artSwiping ? Math.max(0.55, 1 - Math.abs(artDX) / 200) : undefined}
                >
                    {#if st?.track?.art_uri}
                        <img src={st.track.art_uri} alt="" draggable="false" />
                    {:else}
                        <div class="p-art-ph">[ album art ]</div>
                    {/if}
                </div>

                <div class="p-meta">
                    {#if st?.track?.title}
                        <div class="p-title">{st.track.title}</div>
                        <div class="p-sub">
                            {[st.track.artist, st.track.album].filter(Boolean).join(" · ")}
                        </div>
                    {:else}
                        <div class="p-title idle">Nothing playing</div>
                        <div class="p-sub">Start a favorite below, or search Spotify.</div>
                    {/if}
                </div>

                <!-- The rail is a real control only where the source reports a
                     duration. Radio and line-in don't, so they get an honest
                     label instead of a scrubber that would be refused. -->
                {#if durationSec > 0}
                    <div class="p-scrub">
                        <input
                            class="scrub"
                            type="range"
                            min="0"
                            max={durationSec}
                            step="1"
                            aria-label="Seek"
                            aria-valuetext="{fmtSecs(livePos)} of {fmtSecs(durationSec)}"
                            disabled={!c}
                            value={livePos}
                            oninput={(e) => (scrubSec = e.currentTarget.valueAsNumber)}
                            onchange={(e) => commitSeek(g, e.currentTarget.valueAsNumber)}
                        />
                        <div class="p-times mono">
                            <span>{fmtSecs(livePos)}</span><span>{fmtSecs(durationSec)}</span>
                        </div>
                    </div>
                {:else if st?.track?.title}
                    <div class="p-live mono">live stream — no track position</div>
                {/if}

                <div class="p-transport">
                    <button
                        class="icon-btn t-mode"
                        class:on={gs?.shuffle}
                        aria-label={gs?.shuffle ? "Shuffle on" : "Shuffle off"}
                        aria-pressed={gs?.shuffle ?? false}
                        title="Shuffle (s)"
                        disabled={!gs || !c || busy.is("mode:" + c?.id)}
                        onclick={() => setPlayMode(g, { shuffle: !gs?.shuffle })}
                    >
                        <Icon name="shuffle" size={18} />
                    </button>
                    <button class="icon-btn t-btn" aria-label="Previous track" title="Previous (shift ←)"
                        disabled={!c || busy.is("previous:" + c?.id)} onclick={() => skip(g, "previous")}>
                        <Icon name="skipPrev" size={22} />
                    </button>
                    <button class="p-play" class:playing={isPlaying(g)}
                        aria-label={isPlaying(g) ? "Pause" : "Play"} title="Play / pause (space)"
                        disabled={!c || busy.is("play:" + c?.id)} onclick={() => togglePlay(g)}>
                        <Icon name={isPlaying(g) ? "pause" : "play"} size={26} />
                    </button>
                    <button class="icon-btn t-btn" aria-label="Next track" title="Next (shift →)"
                        disabled={!c || busy.is("next:" + c?.id)} onclick={() => skip(g, "next")}>
                        <Icon name="skipNext" size={22} />
                    </button>
                    <button
                        class="icon-btn t-mode"
                        class:on={gs && gs.repeat !== "off"}
                        aria-label={repeatLabel(gs?.repeat)}
                        title="Repeat (r)"
                        disabled={!gs || !c || busy.is("mode:" + c?.id)}
                        onclick={() => setPlayMode(g, { repeat: NEXT_REPEAT[gs?.repeat ?? "off"] })}
                    >
                        <Icon name={gs?.repeat === "one" ? "repeatOne" : "repeat"} size={18} />
                    </button>
                </div>

                <!-- The keys are only worth advertising where there is a
                     keyboard; phones get the swipe gesture instead. -->
                <p class="p-keys mono" aria-hidden="true">
                    space play · ← → seek · ↑ ↓ volume · q queue
                </p>

                {#if gs}
                    <div class="p-extras">
                        <button class="chip" class:on={gs.crossfade} aria-pressed={gs.crossfade}
                            disabled={!c || busy.is("xfade:" + c?.id)} onclick={() => toggleCrossfade(g)}>
                            Crossfade
                        </button>
                        {#if gs.queue_length > 0}
                            <button class="p-upnext" onclick={() => (queuePane = true)}>
                                <Icon name="queue" size={17} />
                                <span class="up-body">
                                    <span class="up-label">Up next</span>
                                    <span class="up-track">
                                        {nextInQueue?.title ?? "End of the queue"}
                                    </span>
                                </span>
                                <span class="up-count mono">{gs.queue_length}</span>
                                <span class="up-go" aria-hidden="true"><Icon name="chevronLeft" size={16} /></span>
                            </button>
                        {/if}
                    </div>
                {/if}

                <!-- Somewhere to go, playing or not: swapping a song out is
                     as ordinary a thing to want here as starting the first
                     one. Favorites still only stand in for an empty player —
                     with a track up, the row that matters is the search. -->
                {@render startSomething(st?.track?.title ? null : g.coordinator_id)}

                <div class="p-speakers">
                    <div class="eyrow">Volume</div>
                    {#if grouped}
                        <div class="member">
                            <span class="m-icon" aria-hidden="true"><Icon name="volume" size={16} /></span>
                            <span class="m-name">All rooms</span>
                            <input type="range" min="0" max="100" step="1" aria-label="Group volume"
                                value={groupVol[g.coordinator_id] ?? 0}
                                oninput={(e) => (groupVol[g.coordinator_id] = e.currentTarget.valueAsNumber)}
                                onchange={(e) => setGroupVolume(g.coordinator_id, e.currentTarget.valueAsNumber)} />
                            <span class="vol-num mono">{groupVol[g.coordinator_id] ?? 0}</span>
                        </div>
                        <div class="m-divider" aria-hidden="true"></div>
                    {/if}
                    {#each g.member_ids as id (id)}
                        {@const sp = speakerById.get(id)}
                        {#if sp}
                            <div class="member">
                                <button class="icon-btn m-mute"
                                    aria-label={sp.state?.muted ? `Unmute ${sp.name}` : `Mute ${sp.name}`}
                                    disabled={busy.is("mute:" + sp.id)} onclick={() => toggleMute(sp)}>
                                    <Icon name={sp.state?.muted ? "volumeOff" : "volume"} size={16} />
                                </button>
                                <span class="m-name" class:muted={sp.state?.muted}>{sp.name}</span>
                                <input type="range" min="0" max="100" step="1" aria-label="{sp.name} volume"
                                    value={localVol[sp.id] ?? sp.state?.volume ?? 0}
                                    oninput={(e) => (localVol[sp.id] = e.currentTarget.valueAsNumber)}
                                    onchange={(e) => setVolume(sp.id, e.currentTarget.valueAsNumber)} />
                                <span class="vol-num mono">{localVol[sp.id] ?? sp.state?.volume ?? 0}</span>
                                {#if grouped}
                                    <button class="icon-btn m-act" aria-label="Remove {sp.name} from group"
                                        disabled={busy.is("leave:" + sp.id)} onclick={() => leave(sp.id)}>
                                        <Icon name="close" size={14} />
                                    </button>
                                {/if}
                            </div>
                        {/if}
                    {/each}
                    {#if joinables(g).length > 0}
                        <div class="joiners">
                            {#each joinables(g) as sp (sp.id)}
                                <button class="chip" disabled={busy.is("join:" + sp.id)} onclick={() => join(sp.id, g)}>
                                    <Icon name="plus" size={13} /> {sp.name}
                                </button>
                            {/each}
                        </div>
                    {/if}
                    {#if g.unregistered?.length}
                        <div class="unreg mono">
                            also in this group: {g.unregistered.join(", ")} — add them to control here
                        </div>
                    {/if}
                </div>
            {/if}
        </div>
    </div>
{/if}

<!-- ── Start something ─────────────────────────────────────────────────
     The way to start something new from inside the player, on both sheets.
     One row: the search that feeds this room, then the searches already made
     for it — the history is keyed by destination, so these are the kitchen's,
     not the house's.

     `favTarget` is the Sonos group whose favorites belong under it, and null
     when there are none to show — a playing group (it already has something)
     or a KEF speaker (favorites are a Sonos household list, DESIGN.md §15). -->
{#snippet startSomething(favTarget: string | null)}
    <!-- Nothing to offer is a reason to render nothing, not a heading over an
         empty row: with the Spotify integration absent and no favorites,
         there is no way to start something from here. -->
    {#if spotify || (favTarget && favorites.length > 0)}
        <div class="p-idle">
            <div class="eyrow">Start something</div>
            {#if spotify}
                <div class="start-row h-scroll">
                    <!-- Not gated on being connected, for the same reason
                         Home's card isn't: the people who most need the
                         pointer are the ones a gate would hide it from. -->
                    <button class="chip start-go" onclick={() => searchFromPlayer()}>
                        <Icon name="search" size={13} />
                        <span>{spotify.connected ? "Search Spotify" : "Set up Spotify"}</span>
                    </button>
                    {#if spotify.connected}
                        {#each playerRecents as h (h)}
                            <button class="chip start-recent" onclick={() => searchFromPlayer(h)}>
                                <span>{h}</span>
                            </button>
                        {/each}
                    {/if}
                </div>
            {/if}
            {#if favTarget && favorites.length > 0}
                <div class="favs h-scroll">
                    {#each favorites as f (f.id)}
                        {@render favCard(f, favTarget)}
                    {/each}
                </div>
            {/if}
        </div>
    {/if}
{/snippet}

<!-- ── Favorite card ───────────────────────────────────────────────────
     Shared by the Home shelf and the idle player: tap the art to play it
     on `target`, or the corner button to queue it without interrupting. -->
{#snippet favCard(f: SonosFavorite, target: string | null)}
    <div class="fav">
        <button class="fav-play" disabled={busy.is("fav:" + f.id) || !target}
            onclick={() => playFavorite(f, target)}>
            {#if f.art_uri}
                <img class="fav-art" src={f.art_uri} alt="" loading="lazy" />
            {:else}
                <div class="fav-art placeholder">[ art ]</div>
            {/if}
            <span class="fav-title">{f.title}</span>
            {#if f.service}<span class="fav-sub mono">{f.service}</span>{/if}
        </button>
        <button class="icon-btn fav-add" aria-label="Add {f.title} to the queue"
            disabled={busy.is("q:" + f.uri) || !target}
            onclick={() => enqueue({ uri: f.uri, title: f.title, metadata: f.metadata }, false, target)}>
            <Icon name="plus" size={14} />
        </button>
    </div>
{/snippet}

<style>
    .sk { height: 180px; border-radius: var(--r-md); }

    /* Announced, never drawn — the running commentary on a grouping gesture
       that has no visible one. */
    .sr-only {
        position: absolute;
        width: 1px; height: 1px;
        margin: -1px; padding: 0;
        overflow: hidden;
        clip-path: inset(50%);
        white-space: nowrap;
        border: 0;
    }

    /* ── Section scaffolding ── */
    .block { display: flex; flex-direction: column; gap: var(--space-3); }
    .block-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3); flex-wrap: wrap;
    }
    .eyrow {
        font-family: var(--font-mono);
        font-size: 11px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--on);
    }
    .hint { font-size: 12px; color: var(--text-mute); }
    .link-btn {
        background: none; border: 0; padding: 0;
        color: var(--text-mute); font-size: 12.5px; cursor: pointer;
    }
    .link-btn:hover { color: var(--text); }

    /* Only said once, at the foot of the Zones sheet — the gesture is the
       affordance, this is the footnote for the keyboard. */
    .zones-keys { margin-top: var(--space-2); }
    .zones-keys kbd {
        font-family: var(--font-mono);
        font-size: 11px;
        padding: 1px 5px;
        border-radius: 5px;
        background: var(--card-3);
        border: 1px solid var(--hairline);
        color: var(--text);
    }

    /* ── Header actions ──
       Search keeps its label wherever the header has room for it, and drops
       to the icon alone on a phone — where a third labelled chip is exactly
       what crushed the subtitle to a two-word stub. */
    .act-search { flex-shrink: 0; }
    @media (max-width: 620px) {
        .act-search {
            position: relative;
            width: 38px; height: 38px; padding: 0;
            justify-content: center; border-radius: 50%;
        }
        .act-label {
            position: absolute;
            width: 1px; height: 1px;
            margin: -1px; padding: 0;
            overflow: hidden;
            clip-path: inset(50%);
            white-space: nowrap;
        }
    }
    @media (max-width: 620px) and (pointer: coarse) {
        .act-search { width: 44px; height: 44px; }
    }

    /* ── Screen head (Speakers) ──
       The §11 detail shape — back chip, centered title, action chip — because
       Speakers is a screen pushed from Home, not a sheet lifted over it. */
    .screen-head {
        display: flex; align-items: center; gap: var(--space-3);
    }
    .screen-title {
        flex: 1; min-width: 0;
        display: flex; flex-direction: column; gap: 2px;
        text-align: center;
    }
    .screen-title h1 {
        font-family: var(--font-sans);
        font-size: 20px; font-weight: 600; letter-spacing: -0.02em;
        color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .screen-sub { font-size: 12px; color: var(--text-mute); }

    /* ── Zones at a glance (Home) ── */
    .room-chips {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
        gap: var(--space-2);
    }
    .room-chip {
        display: flex; align-items: center; justify-content: center; gap: 6px;
        min-height: 44px; padding: 10px var(--space-3);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute); font-size: 12.5px; cursor: pointer;
        transition: border-color var(--t-fast), color var(--t-fast);
    }
    .room-chip span {
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .room-chip.on { background: var(--on-soft); color: var(--on); border-color: transparent; }
    .room-chip:disabled { opacity: 0.5; cursor: default; }
    @media (hover: hover) {
        .room-chip:not(:disabled):hover { border-color: var(--border-strong); color: var(--text); }
        .room-chip.on:not(:disabled):hover { color: var(--on); }
    }

    /* ── Waveform motif ── */
    .wave { display: flex; align-items: flex-end; gap: 2.5px; height: 13px; flex-shrink: 0; }
    .wave i {
        display: block; width: 2.5px; border-radius: 1px;
        background: var(--on); height: 4px;
        animation: wv 950ms ease-in-out infinite;
    }
    .wave i:nth-child(1) { animation-delay: 0s; }
    .wave i:nth-child(2) { animation-delay: 0.15s; }
    .wave i:nth-child(3) { animation-delay: 0.3s; }
    .wave i:nth-child(4) { animation-delay: 0.1s; }
    @keyframes wv { 0%, 100% { height: 3px; } 50% { height: 13px; } }

    /* ── Playing-now cards ── */
    .now-grid {
        display: grid;
        /* Wide enough that the track title still has room next to the
           three-button transport — narrower columns crushed it to an
           ellipsis on desktop. */
        grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
        gap: var(--space-3);
    }
    .now-card {
        position: relative; overflow: hidden;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        transition: border-color var(--t-fast);
    }
    .now-card.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }

    /* ── Progress hairline ──
       How far the track has got, on the cards themselves — the one thing
       they couldn't say without opening the player. Sources that report no
       duration (radio, line-in, TV) get no line rather than a made-up one. */
    .prog {
        position: absolute; left: 0; right: 0; bottom: 0;
        height: 2px; background: var(--hairline);
        pointer-events: none;
    }
    .prog i {
        display: block; height: 100%;
        background: var(--on);
        /* Matches the 1s position tick, so the fill creeps instead of
           stepping. */
        transition: width 1s linear;
    }

    /* Nothing playing — a single honest row, not one dead card per zone. */
    .quiet-card {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
    }
    .quiet-ico {
        width: 44px; height: 44px; border-radius: var(--r-md);
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); color: var(--text-mute);
    }
    .quiet-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .quiet-title { font-size: 14px; font-weight: 600; }
    .quiet-sub { font-size: 12.5px; color: var(--text-mute); }
    .quiet-go { flex-shrink: 0; }
    @media (hover: hover) { .now-card:hover { border-color: var(--border-strong); } }
    .now-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        background: none; border: 0; padding: 0;
        color: var(--text); text-align: left; cursor: pointer;
        transition: transform var(--t-fast);
    }
    .now-open:active { transform: scale(0.99); }
    .now-art {
        width: 52px; height: 52px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-3);
        border: 1px solid var(--hairline); flex-shrink: 0;
    }
    div.now-art { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .now-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
    .now-name {
        font-size: 14px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .now-line { display: flex; align-items: center; gap: 6px; min-width: 0; }
    .now-track {
        font-size: 12.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    /* Card-level transport (Playing-now cards + the dock). Skips ride along
       from 430px up; a phone keeps play/pause and gives the title the room,
       the same trade Home's card makes. */
    .card-transport { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
    .mini-btn {
        width: 38px; height: 38px; border-radius: 50%;
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); border: 1px solid var(--hairline);
        color: var(--text); cursor: pointer;
        transition: transform var(--t-fast), background var(--t-fast);
    }
    .mini-btn.on { background: var(--on); color: var(--primary-fg); border-color: transparent; }
    .mini-btn:active:not(:disabled) { transform: scale(0.94); }
    .mini-btn:focus-visible { box-shadow: var(--focus-ring); }
    .mini-btn:disabled { opacity: 0.5; }
    @media (max-width: 430px) {
        .mini-btn.skip { display: none; }
    }

    /* ── Where playback lands ── */
    .fav-targets { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
    .t-label {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .t-one { font-size: 12.5px; color: var(--text-mute); }

    /* ── Favorites ── */
    .favs { display: flex; gap: var(--space-3); padding-bottom: var(--space-1); }
    .fav { position: relative; width: 112px; }
    .fav-play {
        display: flex; flex-direction: column; gap: 6px; width: 100%;
        background: transparent; border: 0; padding: 0;
        cursor: pointer; text-align: left; color: var(--text); font: inherit;
    }
    .fav-play:disabled { opacity: 0.5; cursor: default; }
    .fav-art {
        width: 112px; height: 112px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-2);
        border: 1px solid var(--hairline);
        transition: transform 120ms ease;
    }
    div.fav-art { display: grid; place-items: center; font-size: 10px; color: var(--text-dim); }
    @media (hover: hover) { .fav-play:hover .fav-art { transform: translateY(-1px); } }
    .fav-play:active .fav-art { transform: scale(0.97); }
    .fav-title {
        font-size: 12.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .fav-sub { font-size: 10px; color: var(--text-dim); letter-spacing: 0.04em; }
    /* Queue-without-interrupting, parked on the art's corner. */
    .fav-add {
        position: absolute; top: 6px; right: 6px;
        width: 30px; height: 30px; border-radius: 50%;
        background: var(--bg-bar); border: 1px solid var(--hairline);
        color: var(--text);
        backdrop-filter: blur(6px);
    }
    .fav-add:disabled { opacity: 0.4; }

    /* ── Room grid ── */
    .rooms { display: flex; flex-direction: column; gap: var(--space-3); }
    .puck-grid {
        display: grid;
        /* 140, not 160: inside the dashed group enclosure the extra padding
           tipped a phone's two-up grid into a single stacked column. */
        grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
        gap: var(--space-3);
    }
    .group-wrap {
        border: 1px dashed var(--tile-on-border);
        border-radius: var(--r-lg);
        padding: var(--space-2);
        display: flex; flex-direction: column; gap: var(--space-2);
        transition: border-color var(--t-fast), box-shadow var(--t-fast);
    }
    /* Aimed at as a whole — the dashed edge goes solid amber, the same
       statement a puck's drop ring makes. */
    .group-wrap.drop {
        border-style: solid;
        border-color: var(--on);
        box-shadow: 0 0 0 1px var(--on), 0 0 22px -6px var(--on-glow);
    }
    .glabel {
        display: flex; align-items: center; gap: 6px;
        padding: 2px 6px;
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--on);
    }
    .glabel span { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .ungroup {
        background: none; border: 0; padding: 2px 4px;
        color: var(--text-mute); font-family: var(--font-sans);
        font-size: 11px; letter-spacing: 0; text-transform: none;
        cursor: pointer;
    }
    .ungroup:hover { color: var(--text); }
    .ungroup:disabled { opacity: 0.5; }

    /* One element, one target: tap opens the room, drag groups it. The
       select circle that used to sit in the corner is gone, so the padding
       that reserved its space goes with it. */
    .puck {
        position: relative;
        width: 100%;
        display: flex; flex-direction: column; gap: 10px;
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        color: var(--text); text-align: left; cursor: grab; font: inherit;
        /* Vertical panning still belongs to the sheet — a puck only lifts on
           a press that stays put (see HOLD_MS). */
        touch-action: pan-y;
        -webkit-user-select: none; user-select: none;
        transition: border-color var(--t-fast), box-shadow var(--t-fast),
            opacity var(--t-fast), transform var(--t-fast);
    }
    .puck.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    .puck:active { transform: scale(0.98); }
    .puck:disabled { opacity: 0.6; cursor: default; }
    /* The one being carried: dimmed in place, so the grid keeps its shape
       while the ghost does the travelling. */
    .puck.lifted { opacity: 0.35; cursor: grabbing; transform: none; }
    .puck.held:not(.lifted) { border-color: var(--on); }
    /* Where it would land. Same ring for the pointer's drop target and the
       keyboard's candidates, because they mean the same thing. */
    .puck.drop {
        border-color: var(--on);
        box-shadow: 0 0 0 2px var(--on), 0 0 22px -4px var(--on-glow);
    }
    .puck.aiming { border-color: var(--tile-on-border); }
    .puck.aiming:focus-visible { box-shadow: var(--focus-ring), 0 0 0 2px var(--on); }
    /* Grip: the only thing on a puck that says "this moves" to a mouse.
       Hover-only so it is never permanent chrome, and inert so it is never
       a second target. */
    .puck-grip {
        position: absolute; top: 10px; right: 10px;
        display: flex;
        color: var(--text-dim);
        opacity: 0;
        pointer-events: none;
        transition: opacity var(--t-fast);
    }
    @media (hover: hover) {
        /* Not while lifted: the pointer is captured, so the source puck stays
           :hover for the whole drag and would keep the grip lit under the
           ghost that is already carrying it. */
        .puck:not(:disabled):not(.lifted):hover .puck-grip { opacity: 0.7; }
    }
    /* Keep the name clear of the grip while it is showing. */
    .puck-name { padding-right: 20px; }
    /* The travelling copy. Fixed to the viewport and inert, so hit-testing
       under the finger finds the room beneath it rather than itself. */
    .puck-ghost {
        position: fixed; z-index: 200;
        pointer-events: none;
        box-shadow: var(--shadow-lg);
        /* Fully opaque and slightly lifted: at 94% the room underneath read
           through the ghost and both sets of text fought. */
        opacity: 1;
        transform: scale(1.03);
        transition: none;
    }
    .puck-icon {
        width: 34px; height: 34px; border-radius: var(--r-md);
        display: grid; place-items: center;
        background: var(--card-3); color: var(--text-mute);
    }
    .puck.playing .puck-icon { background: var(--on); color: var(--primary-fg); }
    /* The waveform's bars are amber like every other one — on the filled
       amber icon tile they'd be invisible, so they take the tile's ink. */
    .puck.playing .puck-icon .wave i { background: var(--primary-fg); }
    .puck-body { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
    /* Clear of the hover grip in the corner. */
    .puck-name { font-size: 14px; font-weight: 600; padding-right: 20px; }
    .puck-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    /* ── Docked mini-player ── */
    .mini {
        position: sticky;
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: 30;
        overflow: hidden;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 9px 10px;
        margin-top: var(--space-2);
        background: var(--tile-on-gradient);
        border: 1px solid var(--tile-on-border);
        border-radius: var(--r-lg);
        box-shadow: var(--shadow-md);
        /* Padding animates so the bar glides into the gutter as the FAB it
           was dodging scales away, instead of snapping the moment it goes. */
        transition: background var(--t-med), border-color var(--t-med),
            padding-right var(--t-med);
    }
    /* Held open after a pause: nothing is playing, so it drops the "ON"
       surface a lit device gets and reads as a plain card. */
    .mini.paused { background: var(--card); border-color: var(--hairline); }
    .mini-idle { display: flex; color: var(--text-mute); flex-shrink: 0; }
    @media (max-width: 900px) {
        .mini {
            bottom: calc(var(--nav-clear) + var(--space-3));
            /* A reserved gutter for the assistant button, which shares this
               band — and the same reprieve when that button is switched off. */
            padding-right: max(10px, var(--fab-clear));
        }
    }
    /* Over the Zones and Search sheets the dock leaves the page flow and
       floats above them, because the transport has to persist across all
       three (DESIGN.md §15). Above the sheet's own z-index, below the
       player's, since tapping it swaps one for the other. */
    .mini.over-sheet {
        position: fixed;
        left: var(--space-4); right: var(--space-4);
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: 127;
        margin-top: 0;
    }
    @media (min-width: 601px) {
        .mini.over-sheet {
            left: 50%; right: auto;
            transform: translateX(-50%);
            width: min(440px, calc(100vw - 48px));
        }
    }
    .mini-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        background: none; border: 0; padding: 0;
        color: var(--text); text-align: left; cursor: pointer;
    }
    .mini-art {
        width: 40px; height: 40px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-3); flex-shrink: 0;
    }
    .mini-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .mini-t {
        font-size: 13px; font-weight: 600;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .mini-s {
        font-size: 11px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    /* ── Speaker list (Speakers) ──────────────────────────────────────
       The §11 list-row shape, with the device's own portrait standing in for
       the 36px icon — it is the one place in the app where a real photograph
       beats a glyph, because telling a Sonos One from a Five is exactly what
       the user is doing here. */
    /* Two panes from 1024px, one below it. The list column folds away on a
       phone once a speaker is open, which is what turns the same markup into
       a drill-down without a second copy of it. */
    .sp-split { display: flex; flex-direction: column; gap: var(--space-4); }
    .sp-col { display: flex; flex-direction: column; gap: var(--space-4); min-width: 0; }
    .sp-pane { min-width: 0; }
    @media (max-width: 1023px) {
        .sp-split.has-detail > .sp-col { display: none; }
    }
    @media (min-width: 1024px) {
        .sp-split {
            display: grid;
            grid-template-columns: minmax(260px, 330px) minmax(0, 1fr);
            gap: var(--space-5);
            align-items: start;
        }
        /* The list is the shorter column; let the settings scroll past it
           rather than stretching the rows to match. */
        .sp-col { position: sticky; top: 76px; }
    }

    .sp-list { display: flex; flex-direction: column; gap: 6px; }
    .sp-row {
        width: 100%;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 60px;
        padding: var(--space-3) var(--space-4);
        text-align: left;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        color: inherit;
        font: inherit;
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    .shot {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        border-radius: var(--radius-md);
        object-fit: contain;
        background: var(--surface);
    }
    /* Caption dropped (see markup); the striped fill carries the meaning. */
    .sp-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .sp-name {
        font-size: 14px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-sub {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-row.off .shot { opacity: 0.45; }
    .sp-row.off .sp-name { color: var(--text-mute); }
    /* Which row the pane is showing. An amber edge, not the .tile.on
       gradient — that treatment means "this device is on", and a selected
       row is a statement about the screen, not about the speaker. */
    .sp-row.sel { border-color: var(--on); background: var(--card-2); }
    .sp-chev { flex-shrink: 0; display: flex; color: var(--text-dim); transform: rotate(-90deg); }
    /* Beside the pane the chevron is redundant — the row's amber edge already
       says which one is open, and there is nowhere further to go. */
    @media (min-width: 1024px) {
        .sp-chev { display: none; }
    }
    @media (hover: hover) {
        .sp-row:hover { background: var(--bg-raised); border-color: var(--border-strong); }
    }

    /* ── Live-updates row (Speakers) ──────────────────────────────────
       The §11 list-row shape: icon left, content middle, chevron right.
       Live takes the sanctioned "ON" treatment rather than a status colour
       of its own — push being on is the same kind of fact as a lit lamp.
       Also used on Rooms as the pointer to unreachable speakers. */
    .lu-row {
        width: 100%;
        margin-top: var(--space-4);
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 60px;
        padding: var(--space-3) var(--space-4);
        text-align: left;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        color: inherit;
        font: inherit;
        cursor: pointer;
        transition: background var(--t-fast), border-color var(--t-fast);
    }
    .lu-ico {
        flex-shrink: 0;
        width: 36px;
        height: 36px;
        display: grid;
        place-items: center;
        border-radius: var(--radius-md);
        background: var(--surface);
        color: var(--text-mute);
    }
    .lu-ico.on {
        background: var(--primary-soft);
        color: var(--primary);
    }
    .lu-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .lu-title { font-size: 14px; font-weight: 600; letter-spacing: -0.01em; }
    .lu-sub { font-size: 12px; color: var(--text-mute); line-height: 1.4; }
    .lu-chev { flex-shrink: 0; display: flex; color: var(--text-dim); transform: rotate(-90deg); }
    @media (hover: hover) {
        .lu-row:hover { background: var(--bg-raised); border-color: var(--border-strong); }
    }

    /* ── Volume rows (player sheet) ── */
    .member { display: flex; align-items: center; gap: var(--space-3); min-height: 44px; }
    .member .m-name {
        font-size: 13.5px; font-weight: 500; width: 110px; flex-shrink: 0;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .member .m-name.muted { color: var(--text-dim); }
    .m-mute, .m-act { width: 36px; height: 36px; flex-shrink: 0; }
    .vol-num {
        font-size: 12px; font-feature-settings: "tnum" 1;
        color: var(--text-mute); width: 3ch; text-align: right; flex-shrink: 0;
    }

    input[type="range"] {
        flex: 1; min-width: 60px; appearance: none;
        height: 6px; border-radius: 3px; outline: none;
        background: var(--card-3); accent-color: var(--on);
    }
    input[type="range"]::-webkit-slider-thumb {
        appearance: none; width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible { box-shadow: 0 0 0 2px var(--on-soft); }

    .joiners { display: flex; flex-wrap: wrap; gap: var(--space-2); margin-top: var(--space-2); }
    .unreg { font-size: 11px; color: var(--text-dim); margin-top: var(--space-2); }

    /* ── Spotify search ── */
    .sp-help { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }
    .sp-steps {
        margin: 0; padding-left: 20px;
        display: flex; flex-direction: column; gap: var(--space-2);
        font-size: 12.5px; color: var(--text-mute); line-height: 1.5;
    }
    .sp-steps li::marker { font-family: var(--font-mono); color: var(--text-dim); }
    .sp-link { color: var(--on); text-decoration: underline; text-underline-offset: 2px; }
    .sp-redirect {
        display: flex; align-items: center; gap: var(--space-2);
        flex-wrap: wrap; margin-top: 4px;
    }
    .sp-redirect code {
        font-family: var(--font-mono); font-size: 12px; color: var(--text);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-sm); padding: 4px 8px;
        word-break: break-all; user-select: all;
    }
    .sp-paste label { font-size: 12.5px; color: var(--text-mute); }
    .sp-config { display: flex; gap: var(--space-2); align-items: center; }
    .sp-config input { flex: 1; min-width: 0; }
    .sp-actions { display: flex; gap: var(--space-2); }

    .sp-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .sp-account { display: flex; align-items: center; gap: var(--space-3); }
    /* Positive "you're connected" signal, so the neighbouring Disconnect
       button reads as an action and not as the account's status. */
    .sp-conn { display: flex; align-items: center; gap: 6px; min-width: 0; }
    .sp-dot {
        width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0;
        background: var(--on); box-shadow: 0 0 0 4px var(--on-soft);
    }
    .sp-conn-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--on);
    }
    .sp-user {
        font-size: 11px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    .sp-search {
        display: flex; align-items: center; gap: var(--space-2);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-md); padding: 10px var(--space-3);
        color: var(--text-mute);
    }
    .sp-input {
        flex: 1; min-width: 0; background: none; border: 0; outline: none;
        color: var(--text); font-size: 14px;
    }
    .sp-clear { width: 30px; height: 30px; flex-shrink: 0; color: var(--text-mute); }
    /* The box already frames the field, so the ring goes on the container —
       a second rounded shape drawn inside it read as a box in a box. */
    .sp-search:focus-within { border-color: var(--border-strong); box-shadow: var(--focus-ring); }
    .sp-input:focus, .sp-input:focus-visible { box-shadow: none; }
    .sp-filters { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
    .sp-browse-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-dim);
    }
    /* Sits opposite the kind filters when there are results to filter, and
       leads the row when there aren't. */
    .sp-targets.pushed { margin-left: auto; }
    .sp-skeleton { height: 120px; border-radius: var(--r-md); }
    .sp-none { font-size: 12.5px; color: var(--text-mute); }
    /* One-line explanation above the results, for a destination that needs
       something before it can play. Quiet: it isn't a fault, it's a step. */
    .sp-note {
        display: flex; align-items: center; gap: var(--space-2);
        padding: var(--space-2) var(--space-3);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        font-size: 12.5px; color: var(--text-mute);
    }
    .sp-note :global(svg) { flex: none; color: var(--text-dim); }
    .sp-note span { flex: 1; min-width: 0; }
    .sp-note .chip { flex: none; }

    .sp-history { display: flex; flex-direction: column; gap: var(--space-2); }
    .sp-history-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); }
    .sp-hist-clear { padding: 3px 10px; font-size: 11px; }
    .sp-history-list { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .sp-hist-chip {
        display: inline-flex; align-items: center;
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
    }
    .sp-hist-run {
        display: inline-flex; align-items: center; gap: 6px;
        padding: 7px 4px 7px 12px;
        background: transparent; border: 0; border-radius: var(--r-pill) 0 0 var(--r-pill);
        font: inherit; font-size: 12.5px; color: var(--text-mute); cursor: pointer;
    }
    @media (hover: hover) { .sp-hist-run:hover { color: var(--text); } }
    .sp-hist-chip .sp-hist-x { width: 26px; height: 26px; margin-right: 3px; color: var(--text-dim); }

    .sp-results { display: flex; flex-direction: column; gap: 2px; }
    /* The row is a container, not a control: tapping the body plays now,
       the trailing overflow queues without interrupting. */
    .sp-row {
        position: relative;
        display: flex; align-items: center; gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) { .sp-row:hover { background: var(--card-2); } }
    .sp-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 52px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .sp-open:active:not(:disabled) { background: var(--card-3); }
    .sp-open:disabled { opacity: 0.5; cursor: default; }
    .sp-more { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; }
    .sp-more:disabled { opacity: 0.4; }

    .overflow-menu {
        position: absolute; right: 8px; top: 46px; z-index: 12;
        min-width: 180px;
        display: flex; flex-direction: column;
        background: var(--card-2);
        border: 1px solid var(--border-strong);
        border-radius: var(--r-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--hairline);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .overflow-item:last-child { border-bottom: 0; }
    @media (hover: hover) { .overflow-item:hover { background: var(--card-3); } }
    .sp-art {
        width: 40px; height: 40px; border-radius: var(--r-sm);
        object-fit: cover; background: var(--card-2);
        border: 1px solid var(--hairline); flex-shrink: 0;
    }
    div.sp-art { display: grid; place-items: center; font-size: 8px; color: var(--text-dim); }
    .sp-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .sp-name {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-play {
        width: 36px; height: 36px; display: grid; place-items: center;
        border-radius: 50%; color: var(--text-mute); flex-shrink: 0;
        transition: color 150ms ease, background 150ms ease;
    }
    .sp-row:hover .sp-play { background: var(--on-soft); color: var(--on); }

    /* ── Full player sheet ── */
    /* Above the mobile nav bar (z 100) and the nav drawer (120), below the
       modal stack (150) — DESIGN.md §15 has the player covering the nav, and
       a "Clear queue" confirm still has to land on top of the player. */
    .scrim {
        position: fixed; inset: 0; z-index: 125;
        background: rgba(0, 0, 0, 0.5);
    }
    .sheet {
        position: fixed; z-index: 126;
        left: 0; right: 0; bottom: 0;
        max-height: 92vh;
        background: var(--bg);
        border-radius: var(--r-xl) var(--r-xl) 0 0;
        border: 1px solid var(--hairline); border-bottom: 0;
        box-shadow: var(--shadow-lg);
        outline: none;
        /* Keep scrolled content inside the top radius, and GPU-promote the
           sheet so the drag transform stays smooth. */
        overflow: hidden;
        will-change: transform;
    }
    .grabber {
        width: 38px; height: 4px; border-radius: 2px;
        background: var(--border-strong);
        margin: 8px auto 0;
        pointer-events: none;
    }
    .sheet-scroll {
        max-height: 92vh; overflow-y: auto;
        overscroll-behavior: contain;
        -webkit-overflow-scrolling: touch;
        padding: 0 var(--space-5)
            calc(var(--space-8) + env(safe-area-inset-bottom));
        display: flex; flex-direction: column; gap: var(--space-5);
    }
    /* The dock floats over Zones and Search, so the last row of either has to
       clear it rather than spending its life underneath. */
    .sheet-scroll.docked {
        padding-bottom: calc(var(--space-8) + 72px + env(safe-area-inset-bottom));
    }
    /* On desktop the sheet becomes a centered dialog. */
    @media (min-width: 601px) {
        .sheet {
            left: 50%; bottom: auto; top: 50%;
            transform: translate(-50%, -50%);
            width: min(440px, calc(100vw - 48px));
            max-height: 88vh;
            border-radius: var(--r-xl); border-bottom: 1px solid var(--hairline);
        }
        .sheet-scroll { max-height: 88vh; }
    }
    /* The bar is the drag handle on phones, so the browser must not claim
       the gesture for scrolling first. */
    @media (max-width: 600px) {
        .sheet-top { touch-action: none; cursor: grab; }
        .sheet.dragging .sheet-top { cursor: grabbing; }
        .sheet-scroll { touch-action: pan-y; }
    }

    /* Grabber + header travel together and stick, so a long queue never
       scrolls the way out off the screen. The band is translucent and
       blurred, and its bottom edge fades out — art and rows dissolve as they
       pass underneath instead of being cut off against an opaque slab. */
    .sheet-top {
        --fade: 22px;
        position: sticky; top: 0; z-index: 3;
        margin: 0 calc(var(--space-5) * -1) calc(var(--fade) * -1);
        padding: 0 var(--space-5) var(--fade);
        background: var(--bg-bar);
        backdrop-filter: blur(18px) saturate(1.3);
        -webkit-backdrop-filter: blur(18px) saturate(1.3);
        -webkit-mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
        mask-image: linear-gradient(to bottom, #000 calc(100% - var(--fade)), transparent);
    }
    .player-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
        padding: var(--space-2) 0 var(--space-3);
    }
    .p-icon { width: 38px; height: 38px; border-radius: 50%; background: var(--card-2); border: 1px solid var(--hairline); }
    /* Keeps the title centered on sheets whose head carries no action chip. */
    .p-icon-gap { width: 38px; flex-shrink: 0; }
    .p-onair { text-align: center; min-width: 0; }
    .p-onair-name { font-size: 13px; font-weight: 600; margin-top: 2px; }
    .p-onair-sub {
        font-size: 11.5px; color: var(--text-mute); margin-top: 2px;
        line-height: 1.35;
    }

    /* Art leads the sheet — it is the largest thing on screen, and the
       glow underneath is the same bulb glow a lit device gets. */
    .p-art {
        display: flex; justify-content: center; padding: var(--space-2) 0 0;
        /* Horizontal is the swipe-to-skip gesture's; vertical stays the
           sheet's (scroll, drag-to-dismiss). */
        touch-action: pan-y;
        transition: transform 260ms var(--spring), opacity var(--t-fast);
        will-change: transform;
    }
    .p-art.swiping { transition: none; }
    .p-art img { user-select: none; -webkit-user-drag: none; }
    .p-art img, .p-art-ph {
        width: min(340px, 78vw); aspect-ratio: 1;
        border-radius: var(--r-lg); object-fit: cover;
    }
    .p-art img {
        background: var(--card-3); border: 1px solid var(--tile-on-border);
        box-shadow: 0 18px 40px -18px var(--on-glow);
    }
    .p-art-ph {
        display: grid; place-items: center;
        background: var(--tile-on-gradient); border: 1px solid var(--tile-on-border);
        color: var(--text-dim); font-family: var(--font-mono); font-size: 11px;
    }

    .p-meta { text-align: center; display: flex; flex-direction: column; gap: 4px; }
    .p-title {
        font-size: 22px; font-weight: 600; letter-spacing: -0.02em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .p-title.idle { color: var(--text-mute); }
    .p-sub {
        font-size: 14px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    .p-scrub { display: flex; flex-direction: column; gap: 6px; }
    /* A range input, not a decorative bar: it drags, it takes arrow keys,
       and it inherits the volume sliders' touch sizing below. The selector
       has to out-specify the generic input[type="range"] rule, whose
       `flex: 1` would otherwise collapse it in this column. */
    input[type="range"].scrub { flex: none; width: 100%; }
    .p-times { display: flex; justify-content: space-between; font-size: 11px; color: var(--text-dim); }
    .p-live {
        text-align: center; font-size: 10.5px; letter-spacing: 0.08em;
        text-transform: uppercase; color: var(--text-dim);
    }

    .p-transport { display: flex; align-items: center; justify-content: center; gap: var(--space-4); }
    .t-btn { width: 48px; height: 48px; }
    .t-mode { width: 42px; height: 42px; border-radius: 50%; color: var(--text-mute); }
    .t-mode.on { background: var(--on-soft); color: var(--on); }
    .t-mode:disabled { opacity: 0.35; }
    .p-play {
        width: 66px; height: 66px; border-radius: 50%;
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--on); color: var(--primary-fg); border: 0;
        cursor: pointer; box-shadow: 0 0 24px -2px var(--on-glow);
    }
    .p-play:active { transform: scale(0.96); }
    .p-play:disabled { opacity: 0.5; }

    /* Keyboard hints, advertised only where there is a keyboard to press —
       phones get the swipe gesture on the art instead. */
    .p-keys { display: none; }
    @media (hover: hover) and (pointer: fine) {
        .p-keys {
            display: block; text-align: center;
            font-size: 10px; letter-spacing: 0.06em;
            color: var(--text-dim);
        }
    }

    .p-extras { display: flex; flex-direction: column; gap: var(--space-3); }
    .p-extras .chip { align-self: flex-start; }
    /* Up next doubles as the way into the queue pane. */
    .p-upnext {
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 56px; padding: 10px var(--space-3);
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        color: var(--text-mute); cursor: pointer; text-align: left; font: inherit;
        transition: border-color var(--t-fast);
    }
    @media (hover: hover) { .p-upnext:hover { border-color: var(--border-strong); } }
    .up-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .up-label {
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .up-track {
        font-size: 13px; color: var(--text);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .up-count { font-size: 12px; color: var(--text-dim); flex-shrink: 0; }
    .up-go { display: flex; transform: rotate(180deg); flex-shrink: 0; }

    .p-idle { display: flex; flex-direction: column; gap: var(--space-3); }

    /* ── Start something ──
       The search row inside the player. Chips rather than a search box: the
       box lives on the Search sheet, and a second one here would be a second
       thing to focus, keep and clear (DESIGN.md §15 — sheets swap, and the
       player hands over rather than growing a copy of what it hands over to). */
    .start-row { align-items: center; }
    /* The row's primary action, marked the way every other lead chip in the
       module is — a stronger edge and full-strength text, not a new colour. */
    .start-go { color: var(--text); border-color: var(--border-strong); }
    /* A recent search is whatever was typed, so it is capped rather than
       trusted to be short. */
    .start-recent > span { display: block; max-width: 52vw; overflow: hidden; text-overflow: ellipsis; }
    @media (pointer: coarse) {
        .start-row .chip { min-height: 44px; padding-inline: 14px; }
    }

    .p-speakers { display: flex; flex-direction: column; gap: 2px; }
    .p-speakers .eyrow { margin-bottom: var(--space-1); }

    /* ── KEF player ──
       A read-only progress line, because KEF's API has no seek — the Sonos
       sheet's scrubber is a range input in the same slot. */
    .kef-rail {
        display: block; height: 6px; border-radius: 3px;
        background: var(--card-3);
        overflow: hidden;
    }
    .kef-rail i {
        display: block; height: 100%;
        background: var(--on);
        /* Matches the 1s position tick, so the fill creeps instead of
           stepping. */
        transition: width 1s linear;
    }
    /* The input selector, where the group's member rows sit on the other
       sheet — it answers the same "where is this coming out" question. */
    .src-row { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .src-row .chip { flex-shrink: 0; }
    .p-speakers .hint { margin-top: var(--space-2); }
    .m-icon {
        width: 36px; height: 36px; flex-shrink: 0;
        display: grid; place-items: center; color: var(--text-mute);
    }
    .m-divider { height: 1px; background: var(--hairline); margin: var(--space-2) 0; }

    /* ── Queue pane ── */
    .q-bar { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    .q-total {
        font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-mute);
    }
    .q-skeleton { height: 220px; border-radius: var(--r-md); }
    .q-none { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }
    .q-list { display: flex; flex-direction: column; gap: 2px; }
    .q-row {
        display: flex; align-items: center; gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background 150ms ease;
    }
    @media (hover: hover) { .q-row:hover { background: var(--card-2); } }
    .q-row.current { background: var(--tile-on-gradient); }
    .q-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        min-height: 48px; padding: 6px var(--space-2);
        background: transparent; border: 0; border-radius: var(--r-md);
        color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .q-open:disabled { opacity: 0.5; cursor: default; }
    .q-num {
        width: 26px; flex-shrink: 0;
        display: flex; align-items: center; justify-content: center;
        font-size: 11.5px; color: var(--text-dim);
    }
    .q-row.current .q-num { color: var(--on); }
    .q-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .q-title {
        font-size: 13.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .q-row.current .q-title { color: var(--on); }
    .q-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .q-dur { font-size: 11px; color: var(--text-dim); flex-shrink: 0; }
    .q-rm { width: 36px; height: 36px; flex-shrink: 0; margin-right: 4px; color: var(--text-mute); }
    .q-rm:disabled { opacity: 0.4; }
    .q-more { font-size: 10.5px; color: var(--text-dim); text-align: center; }

    /* ── Touch: hit areas grow to the 44px floor ── */
    @media (pointer: coarse) {
        .t-btn { width: 52px; height: 52px; }
        .t-mode { width: 48px; height: 48px; }
        /* Five transport controls have to fit a 360px screen. */
        .p-transport { gap: var(--space-3); }
        .m-mute, .m-act, .m-icon { width: 44px; height: 44px; }
        input[type="range"] { height: 10px; border-radius: 5px; }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
        .member .m-name { width: 90px; }
        .sp-play { width: 44px; height: 44px; }
        .sp-more, .q-rm, .fav-add, .sp-clear { width: 44px; height: 44px; }
        .mini-btn { width: 44px; height: 44px; }
        .sp-input, .sp-config input { font-size: 16px; } /* prevents iOS auto-zoom */
    }

    @media (prefers-reduced-motion: reduce) {
        .wave i { animation: none; height: 8px; }
        .fav-art, .now-card, .puck, .p-play,
        .p-upnext, .q-row, .sp-row, .mini, .mini-btn, .p-art, .prog i, .kef-rail i {
            transition-duration: 0.001ms;
        }
        /* The sheet's drag snap-back is an inline style, so it needs its own
           override here rather than a transition-duration on the class. */
        .sheet { transition: none !important; }
    }
</style>
