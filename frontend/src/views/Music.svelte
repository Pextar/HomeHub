<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Icon from "../components/Icon.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import SonosSpeakerModal from "../modals/SonosSpeakerModal.svelte";
    import Segmented from "../components/Segmented.svelte";
    import { api } from "../lib/api";
    import { toasts, route } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import { copyText } from "../lib/clipboard";
    import { fly, fade, scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur, sheet } from "../lib/motion";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
    import type {
        SonosStatus, SonosSpeakerView, SonosGroupView, SonosFavorite,
        SonosQueueItem, SonosRepeat,
        SpotifyStatus, SpotifyItem, SpotifyResults,
    } from "../lib/types";

    let status = $state<SonosStatus | null>(null);
    let loaded = $state(false);
    let favorites = $state<SonosFavorite[]>([]);
    let favsLoaded = $state(false);
    // Which group's coordinator receives a tapped favorite. Defaults to the
    // first group; shown as chips when there is more than one group.
    let favTarget = $state<string | null>(null);

    // Volume the user just set, keyed by speaker id. The 5s poll must not
    // yank the slider back to a stale value while the command is still
    // propagating, so recent local sets win over polled state briefly.
    let volOverride: Record<string, { v: number; at: number }> = {};
    let localVol = $state<Record<string, number>>({});
    let groupVol = $state<Record<string, number>>({});

    // Actions in flight (play/pause/join/…) keyed by "<action>:<id>".
    let busy = $state<Record<string, boolean>>({});

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
    const playingGroups = $derived(groups.filter((g) => coordinatorOf(g)?.state?.playing));
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
    function speakerPlaying(id: string): boolean {
        const g = groupOfSpeaker(id);
        return g ? !!coordinatorOf(g)?.state?.playing : false;
    }
    function speakerNowLine(id: string): string {
        const g = groupOfSpeaker(id);
        const st = g && coordinatorOf(g)?.state;
        return st?.playing && st.track?.title ? st.track.title : "Idle";
    }

    // ── Data loading ─────────────────────────────────────────────────────
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let statusSeq = 0;

    async function refresh() {
        const seq = ++statusSeq;
        try {
            const st = await api.sonosStatus();
            if (seq !== statusSeq) return;
            status = st;
            polledAt = Date.now();
            const now = polledAt;
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
            if (!favTarget || !st.groups.some((g) => g.coordinator_id === favTarget)) {
                favTarget = st.groups[0]?.coordinator_id ?? null;
            }
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

    onMount(() => {
        void refresh();
        pollTimer = setInterval(refresh, 5000);
    });
    onDestroy(() => {
        clearInterval(pollTimer);
        unlockBodyScroll();
    });

    // ── Actions ──────────────────────────────────────────────────────────
    async function run(key: string, fn: () => Promise<unknown>, errTitle: string) {
        if (busy[key]) return;
        busy[key] = true;
        try {
            await fn();
            await refresh();
        } catch (e) {
            toasts.error(errTitle, (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    function togglePlay(g: SonosGroupView) {
        const c = coordinatorOf(g);
        if (!c) return;
        const playing = c.state?.playing;
        void run(
            "play:" + c.id,
            () => (playing ? api.sonosPause(c.id) : api.sonosPlay(c.id)),
            playing ? "Pause failed" : "Play failed",
        );
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

    function join(speakerId: string, g: SonosGroupView) {
        void run("join:" + speakerId, () => api.sonosJoin(speakerId, g.coordinator_id), "Grouping failed");
    }

    function leave(speakerId: string) {
        void run("leave:" + speakerId, () => api.sonosLeave(speakerId), "Ungrouping failed");
    }

    // Favorites play on the chip-selected target, except inside the player
    // sheet, where the group being viewed is the obvious destination.
    function playFavorite(f: SonosFavorite, target: string | null = favTarget) {
        if (!target) return;
        void run("fav:" + f.id, () => api.sonosPlayFavorite(target, f), "Couldn't play favorite");
    }

    // ── Screens ──────────────────────────────────────────────────────────
    // Music has three screens of its own. They ride a subnav inside the view
    // rather than reshaping the global tab bar — Music is one destination
    // among six, so the app-level nav never changes shape here.
    type Screen = "home" | "rooms" | "search";
    let screen = $state<Screen>("home");
    const SCREENS = [
        { value: "home", label: "Home" },
        { value: "rooms", label: "Rooms" },
        { value: "search", label: "Search" },
    ];
    function goto(s: Screen) {
        screen = s;
        if (s !== "rooms") selectedIds = []; // selection is a Rooms-screen mode
    }

    // ── Room grid: tap-to-select grouping ────────────────────────────────
    let selectedIds = $state<string[]>([]);
    const selectedNames = $derived(
        selectedIds.map((id) => speakerById.get(id)?.name).filter(Boolean).join(", "),
    );
    function toggleSelect(id: string) {
        selectedIds = selectedIds.includes(id)
            ? selectedIds.filter((x) => x !== id)
            : [...selectedIds, id];
    }
    async function groupSelected() {
        if (selectedIds.length < 2) return;
        // The first tapped speaker anchors the group; if it already leads a
        // zone the others join that zone, otherwise it becomes the new
        // coordinator. Joining sequentially keeps Sonos' topology consistent.
        const first = selectedIds[0];
        const target = groupOfSpeaker(first)?.coordinator_id ?? first;
        const key = "group:" + target;
        if (busy[key]) return;
        busy[key] = true;
        try {
            for (const id of selectedIds) {
                if (id !== target) await api.sonosJoin(id, target);
            }
            selectedIds = [];
            await refresh();
        } catch (e) {
            toasts.error("Grouping failed", (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }
    async function ungroup(g: SonosGroupView) {
        const key = "ungroup:" + g.coordinator_id;
        if (busy[key]) return;
        busy[key] = true;
        try {
            for (const id of g.member_ids) {
                if (id !== g.coordinator_id) await api.sonosLeave(id);
            }
            await refresh();
        } catch (e) {
            toasts.error("Ungrouping failed", (e as Error).message);
        } finally {
            busy[key] = false;
        }
    }

    // ── Player sheet ─────────────────────────────────────────────────────
    // The docked mini-player expands into a full sheet. Rendered inline (not
    // via the modal stack) so it stays live against the 5s status poll.
    let playerGroupId = $state<string | null>(null);
    const playerOpen = $derived(playerGroupId !== null);
    const activeGroup = $derived(
        groups.find((g) => g.coordinator_id === playerGroupId),
    );
    // The group the docked mini-player represents: first thing playing.
    const dockGroup = $derived(playingGroups[0]);

    let playerEl = $state<HTMLElement | null>(null);

    function openPlayer(g: SonosGroupView) {
        playerGroupId = g.coordinator_id;
        lockBodyScroll();
    }
    function closePlayer() {
        if (playerGroupId === null) return;
        playerGroupId = null;
        queuePane = false;
        scrubSec = null;
        unlockBodyScroll();
    }
    function onWindowKey(e: KeyboardEvent) {
        if (e.key !== "Escape") return;
        // Escape always leaves the player outright rather than stepping back
        // through the queue pane — the sheet covers the nav, so one press
        // must always be enough to get out (DESIGN.md §15).
        if (playerOpen) closePlayer();
        else if (menuFor) menuFor = null;
    }
    // A regroup between polls can retire the coordinator the sheet is bound
    // to. Close instead of leaving an empty sheet — and, more importantly,
    // a permanently locked body scroll.
    $effect(() => {
        if (playerGroupId !== null && !activeGroup) closePlayer();
    });
    // Move focus into the sheet when it opens so keyboard users land there.
    $effect(() => {
        if (playerOpen) playerEl?.focus();
    });
    // Drop selections for speakers that dropped off the network.
    $effect(() => {
        const live = new Set(reachable.map((s) => s.id));
        if (selectedIds.some((id) => !live.has(id))) {
            selectedIds = selectedIds.filter((id) => live.has(id));
        }
    });
    // Speakers outside the active group that could join it.
    function joinables(g: SonosGroupView): SonosSpeakerView[] {
        return reachable.filter((s) => !g.member_ids.includes(s.id));
    }

    // "0:03:12" → seconds; "" / undefined → 0.
    function secs(t?: string): number {
        if (!t) return 0;
        const p = t.split(":").map(Number);
        return p.reduce((acc, n) => acc * 60 + (Number.isFinite(n) ? n : 0), 0);
    }
    // "0:03:12" → "3:12" (Sonos always sends leading hours)
    function clock(t?: string): string {
        if (!t) return "";
        return t.replace(/^0:0?/, "");
    }
    // seconds → "3:12" for display
    function fmtSecs(t: number): string {
        const total = Math.max(0, Math.round(t));
        const s = String(total % 60).padStart(2, "0");
        const m = Math.floor(total / 60);
        if (m < 60) return `${m}:${s}`;
        return `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}:${s}`;
    }
    // seconds → "0:03:12", the H:MM:SS form the seek endpoint takes
    function toClock(t: number): string {
        const total = Math.max(0, Math.round(t));
        return `${Math.floor(total / 3600)}:${String(Math.floor(total / 60) % 60).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
    }

    // ── Scrubbing ────────────────────────────────────────────────────────
    // The position is only polled every 5s, so between polls the player
    // extrapolates from the last reading. `tick` exists purely to re-run that
    // derivation once a second, and only while the sheet is open.
    let tick = $state(0);
    $effect(() => {
        if (!playerOpen) return;
        const t = setInterval(() => tick++, 1000);
        return () => clearInterval(t);
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
        void tick; // re-derive once a second
        const now = Date.now();
        const ov = seekOverride;
        const base = ov && now - ov.at < 4000 ? ov.sec : secs(activeState?.position);
        const since = ov && now - ov.at < 4000 ? ov.at : polledAt;
        if (!activeState?.playing || !since) return base;
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
    // favorites; `next` drops it in after the current track.
    async function enqueue(
        item: { uri: string; title?: string; service?: string; metadata?: string },
        next: boolean,
        target: string | null = favTarget,
    ) {
        if (!target) return;
        const key = "q:" + item.uri;
        if (busy[key]) return;
        busy[key] = true;
        try {
            const added = await api.sonosQueueAdd(target, { ...item, next });
            const where = added.track ? `position ${added.track} of ${added.length}` : "the queue";
            toasts.success(next ? "Playing next" : "Added to queue", `${item.title ?? "Track"} · ${where}`);
            if (playerGroupId === target) void loadQueue(target);
        } catch (e) {
            toasts.error("Couldn't add to the queue", (e as Error).message);
        } finally {
            busy[key] = false;
        }
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
    function onQueryInput() {
        clearTimeout(searchTimer);
        searchTimer = setTimeout(doSearch, 400);
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

    const shownItems = $derived<SpotifyItem[]>(
        results ? results[kindFilter] : myPlaylists,
    );

    function playItem(item: SpotifyItem) {
        if (!favTarget) return;
        void run(
            "item:" + item.uri,
            () => api.sonosPlayItem(favTarget!, { service: "Spotify", uri: item.uri, title: item.name }),
            "Couldn't play",
        );
    }

    async function openSpeakerModal(sp?: SonosSpeakerView) {
        const changed = await openModal<boolean>(SonosSpeakerModal, sp ? { existing: sp } : {});
        if (changed) void refresh();
    }
</script>

<svelte:window onkeydown={onWindowKey} />

<!-- The live waveform — the music module's motif for "actually playing",
     used everywhere a plain status dot would otherwise sit. -->
{#snippet wave()}
    <span class="wave" aria-hidden="true"><i></i><i></i><i></i><i></i></span>
{/snippet}

<Topbar
    title="Music"
    subtitle={status
        ? `${status.speakers.length} speaker${status.speakers.length === 1 ? "" : "s"} · ${playingCount} playing`
        : "Sonos"}
>
    {#snippet actions()}
        <button class="chip" onclick={() => openSpeakerModal()}>
            <Icon name="plus" size={14} /> Add speaker
        </button>
    {/snippet}
</Topbar>

{#if !loaded}
    <section class="card"><div class="skeleton sk"></div></section>
{:else if (status?.speakers.length ?? 0) === 0}
    <EmptyState
        icon="speaker"
        title="No speakers yet"
        message="Add your Sonos speakers to control playback, volume and grouping right here — no Sonos app needed."
    >
        <button class="btn btn-primary" onclick={() => openSpeakerModal()}>Add speaker</button>
    </EmptyState>
{:else}
    <!-- Music's own three screens. Rides inside the view — the global tab
         bar keeps its shape (DESIGN.md §15). -->
    <div class="subnav">
        <Segmented
            name="music-screen"
            value={screen}
            options={SCREENS}
            onChange={(v) => goto(v as Screen)}
            full
            accent
        />
    </div>
{/if}

{#if loaded && (status?.speakers.length ?? 0) > 0}
    {#if screen === "home"}
    <!-- ── Playing now ─────────────────────────────────────────────── -->
    <section class="block">
        <div class="eyrow">Playing now</div>
        <div class="now-grid">
            {#each groups as g (g.coordinator_id)}
                {@const c = coordinatorOf(g)}
                {@const st = c?.state}
                <div
                    class="now-card"
                    class:playing={st?.playing}
                    in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
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
                                {#if st?.playing && st.track?.title}
                                    {@render wave()}
                                    <span class="now-track">
                                        {[st.track.title, st.track.artist].filter(Boolean).join(" · ")}
                                    </span>
                                {:else}
                                    <span class="now-track idle">Nothing playing</span>
                                {/if}
                            </span>
                        </span>
                    </button>
                    <button
                        class="mini-btn"
                        class:on={st?.playing}
                        aria-label={st?.playing ? "Pause" : "Play"}
                        disabled={!c || busy["play:" + c?.id]}
                        onclick={() => togglePlay(g)}
                    >
                        <Icon name={st?.playing ? "pause" : "play"} size={16} />
                    </button>
                </div>
            {/each}
        </div>
    </section>

    <!-- ── Favorites ───────────────────────────────────────────────── -->
    {#if favorites.length > 0}
        <section class="block">
            <div class="block-head">
                <div class="eyrow">Favorites</div>
                {#if groups.length > 1}
                    <div class="fav-targets" role="radiogroup" aria-label="Play favorites on">
                        {#each groups as g (g.coordinator_id)}
                            <button class="chip" class:on={favTarget === g.coordinator_id}
                                onclick={() => (favTarget = g.coordinator_id)}>
                                {groupTitle(g)}
                            </button>
                        {/each}
                    </div>
                {/if}
            </div>
            <div class="favs h-scroll">
                {#each favorites as f (f.id)}
                    {@render favCard(f, favTarget)}
                {/each}
            </div>
        </section>
    {/if}

    <!-- ── Rooms at a glance (Home) ────────────────────────────────── -->
    <section class="block">
        <div class="block-head">
            <div class="eyrow">Rooms</div>
            <button class="link-btn" onclick={() => goto("rooms")}>Manage</button>
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
        </div>
    </section>

    {:else if screen === "rooms"}
    <!-- ── Rooms — tap-to-group grid ───────────────────────────────── -->
    <section class="block">
        <div class="block-head">
            <div class="eyrow">Rooms</div>
            <span class="hint">Tap rooms to select, then group them</span>
        </div>
        <div class="rooms">
            {#each multiGroups as g (g.coordinator_id)}
                <div class="group-wrap">
                    <div class="glabel">
                        <Icon name="check" size={11} />
                        <span>{groupTitle(g)}</span>
                        <button class="ungroup" disabled={busy["ungroup:" + g.coordinator_id]}
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
        </div>
    </section>

    <!-- ── Unreachable ─────────────────────────────────────────────── -->
    {#if offline.length > 0}
        <section class="card">
            <div class="card-header"><h2>Unreachable</h2></div>
            <div class="members">
                {#each offline as sp (sp.id)}
                    <div class="member off">
                        <Icon name="speaker" size={16} />
                        <span class="m-name">{sp.name}</span>
                        <span class="off-ip mono">{sp.ip}</span>
                        <button class="icon-btn m-act" aria-label="Edit {sp.name}"
                            onclick={() => openSpeakerModal(sp)}>
                            <Icon name="edit" size={14} />
                        </button>
                    </div>
                {/each}
            </div>
        </section>
    {/if}

    {:else}
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
                <div class="card-header sp-head">
                    <h2>Search</h2>
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
                        type="search"
                        class="sp-input"
                        placeholder="Songs, albums, playlists…"
                        aria-label="Search Spotify"
                        bind:value={query}
                        oninput={onQueryInput}
                    />
                </div>
                <div class="sp-filters">
                    {#if results}
                        <button class="chip" class:active={kindFilter === "tracks"} onclick={() => (kindFilter = "tracks")}>Songs</button>
                        <button class="chip" class:active={kindFilter === "albums"} onclick={() => (kindFilter = "albums")}>Albums</button>
                        <button class="chip" class:active={kindFilter === "playlists"} onclick={() => (kindFilter = "playlists")}>Playlists</button>
                    {:else if myPlaylists.length > 0}
                        <span class="sp-browse-label">Your playlists</span>
                    {/if}
                    {#if groups.length > 1}
                        <div class="fav-targets sp-targets" role="radiogroup" aria-label="Play on">
                            {#each groups as g (g.coordinator_id)}
                                <button class="chip" class:on={favTarget === g.coordinator_id}
                                    onclick={() => (favTarget = g.coordinator_id)}>
                                    {groupTitle(g)}
                                </button>
                            {/each}
                        </div>
                    {/if}
                </div>
                {#if searching}
                    <div class="skeleton sp-skeleton"></div>
                {:else if results && shownItems.length === 0}
                    <div class="sp-none">No {kindFilter} matched "{query.trim()}".</div>
                {:else}
                    <div class="sp-results">
                        {#each shownItems as item (item.uri)}
                            <div class="sp-row">
                                <button class="sp-open" disabled={busy["item:" + item.uri] || !favTarget}
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
                                     interrupting lives behind the overflow. -->
                                <button class="icon-btn sp-more" aria-label="More for {item.name}"
                                    aria-haspopup="menu" aria-expanded={menuFor === item.uri}
                                    disabled={busy["q:" + item.uri] || !favTarget}
                                    onclick={(e) => toggleMenu(e, item.uri)}>
                                    <Icon name="more" size={16} />
                                </button>
                                {#if menuFor === item.uri}
                                    <div class="overflow-menu" role="menu"
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
    {/if}

    <!-- ── Docked mini-player (persists across all three screens) ───── -->
    {#if dockGroup}
        {@const c = coordinatorOf(dockGroup)}
        {@const st = c?.state}
        <div class="mini" transition:fly={{ y: 20, duration: dur(220), easing: cubicOut }}>
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
                {@render wave()}
            </button>
            <button class="mini-btn on" aria-label="Pause" disabled={!c || busy["play:" + c?.id]}
                onclick={() => togglePlay(dockGroup)}>
                <Icon name="pause" size={16} />
            </button>
        </div>
    {/if}
{/if}

<!-- ── Room puck ───────────────────────────────────────────────────── -->
{#snippet puck(sp: SonosSpeakerView)}
    {@const playing = speakerPlaying(sp.id)}
    {@const selected = selectedIds.includes(sp.id)}
    <button
        class="puck"
        class:playing
        class:selected
        aria-pressed={selected}
        onclick={() => toggleSelect(sp.id)}
    >
        <span class="check" aria-hidden="true"><Icon name="check" size={12} /></span>
        <span class="puck-icon">
            {#if playing}{@render wave()}{:else}<Icon name="speaker" size={16} />{/if}
        </span>
        <span class="puck-body">
            <span class="puck-name">{sp.name}</span>
            <span class="puck-sub">{speakerNowLine(sp.id)}</span>
        </span>
    </button>
{/snippet}

<!-- ── Selection bar (grouping) ────────────────────────────────────── -->
{#if screen === "rooms" && selectedIds.length >= 2}
    <div class="selbar" transition:fly={{ y: 16, duration: dur(200), easing: cubicOut }}>
        <span class="sel-count mono">{selectedIds.length} selected</span>
        <span class="sel-names">{selectedNames}</span>
        <button class="btn btn-primary sel-go" onclick={groupSelected}>Group</button>
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
        class="player"
        role="dialog"
        aria-modal="true"
        aria-label="Now playing"
        tabindex="-1"
        bind:this={playerEl}
        transition:sheet={{}}
    >
        <!-- Grabber + close X, per DESIGN.md §5 — the sheet must read as
             dismissible at a glance, not only via the collapse chevron. -->
        <div class="grabber" aria-hidden="true"></div>
        <div class="player-scroll" bind:this={scrollEl}>
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

            {#if queuePane}
                <!-- ── Queue pane ──────────────────────────────────────── -->
                <div class="q-bar">
                    <span class="q-total mono">
                        {gs?.queue_length ?? queue.length}
                        {(gs?.queue_length ?? queue.length) === 1 ? "track" : "tracks"}
                    </span>
                    <button class="chip" disabled={!c || busy["qclear:" + c?.id] || queue.length === 0}
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
                                <button class="q-open" disabled={busy["jump:" + item.track]}
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
                                        <span class="q-dur mono">{clock(item.duration)}</span>
                                    {/if}
                                </button>
                                <button class="icon-btn q-rm"
                                    aria-label="Remove {item.title || 'track ' + item.track} from the queue"
                                    disabled={busy["qrm:" + item.track]} onclick={() => removeQueued(g, item.track)}>
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
                <div class="p-art">
                    {#if st?.track?.art_uri}
                        <img src={st.track.art_uri} alt="" />
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
                        disabled={!gs || !c || busy["mode:" + c?.id]}
                        onclick={() => setPlayMode(g, { shuffle: !gs?.shuffle })}
                    >
                        <Icon name="shuffle" size={18} />
                    </button>
                    <button class="icon-btn t-btn" aria-label="Previous track"
                        disabled={!c || busy["previous:" + c?.id]} onclick={() => skip(g, "previous")}>
                        <Icon name="skipPrev" size={22} />
                    </button>
                    <button class="p-play" class:playing={st?.playing}
                        aria-label={st?.playing ? "Pause" : "Play"}
                        disabled={!c || busy["play:" + c?.id]} onclick={() => togglePlay(g)}>
                        <Icon name={st?.playing ? "pause" : "play"} size={26} />
                    </button>
                    <button class="icon-btn t-btn" aria-label="Next track"
                        disabled={!c || busy["next:" + c?.id]} onclick={() => skip(g, "next")}>
                        <Icon name="skipNext" size={22} />
                    </button>
                    <button
                        class="icon-btn t-mode"
                        class:on={gs && gs.repeat !== "off"}
                        aria-label={repeatLabel(gs?.repeat)}
                        disabled={!gs || !c || busy["mode:" + c?.id]}
                        onclick={() => setPlayMode(g, { repeat: NEXT_REPEAT[gs?.repeat ?? "off"] })}
                    >
                        <Icon name={gs?.repeat === "one" ? "repeatOne" : "repeat"} size={18} />
                    </button>
                </div>

                {#if gs}
                    <div class="p-extras">
                        <button class="chip" class:on={gs.crossfade} aria-pressed={gs.crossfade}
                            disabled={!c || busy["xfade:" + c?.id]} onclick={() => toggleCrossfade(g)}>
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

                <!-- Idle groups get somewhere to go rather than a dead end. -->
                {#if !st?.track?.title && favorites.length > 0}
                    <div class="p-idle">
                        <div class="eyrow">Start something</div>
                        <div class="favs h-scroll">
                            {#each favorites as f (f.id)}
                                {@render favCard(f, g.coordinator_id)}
                            {/each}
                        </div>
                    </div>
                {/if}

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
                                    disabled={busy["mute:" + sp.id]} onclick={() => toggleMute(sp)}>
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
                                        disabled={busy["leave:" + sp.id]} onclick={() => leave(sp.id)}>
                                        <Icon name="close" size={14} />
                                    </button>
                                {/if}
                            </div>
                        {/if}
                    {/each}
                    {#if joinables(g).length > 0}
                        <div class="joiners">
                            {#each joinables(g) as sp (sp.id)}
                                <button class="chip" disabled={busy["join:" + sp.id]} onclick={() => join(sp.id, g)}>
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

<!-- ── Favorite card ───────────────────────────────────────────────────
     Shared by the Home shelf and the idle player: tap the art to play it
     on `target`, or the corner button to queue it without interrupting. -->
{#snippet favCard(f: SonosFavorite, target: string | null)}
    <div class="fav">
        <button class="fav-play" disabled={busy["fav:" + f.id] || !target}
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
            disabled={busy["q:" + f.uri] || !target}
            onclick={() => enqueue({ uri: f.uri, title: f.title, metadata: f.metadata }, false, target)}>
            <Icon name="plus" size={14} />
        </button>
    </div>
{/snippet}

<style>
    .sk { height: 180px; border-radius: var(--r-md); }

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

    /* ── Subnav — Music's own three screens ── */
    .subnav {
        position: sticky; top: var(--space-2); z-index: 15;
        /* Bleeds slightly wider than the content so the sticky pill reads as
           a bar rather than a floating control when it detaches. */
        padding: var(--space-1) 0;
        background: var(--bg);
    }

    /* ── Rooms at a glance (Home) ── */
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
        grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
        gap: var(--space-3);
    }
    .now-card {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        transition: border-color var(--t-fast);
    }
    .now-card.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
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
    .now-track.idle { color: var(--text-dim); }

    .mini-btn {
        width: 38px; height: 38px; border-radius: 50%;
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); border: 1px solid var(--hairline);
        color: var(--text); cursor: pointer;
    }
    .mini-btn.on { background: var(--on); color: var(--primary-fg); border-color: transparent; }
    .mini-btn:disabled { opacity: 0.5; }

    /* ── Favorites ── */
    .fav-targets { display: flex; gap: var(--space-2); flex-wrap: wrap; }
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
        grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
        gap: var(--space-3);
    }
    .group-wrap {
        border: 1px dashed var(--tile-on-border);
        border-radius: var(--r-lg);
        padding: var(--space-2);
        display: flex; flex-direction: column; gap: var(--space-2);
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

    .puck {
        position: relative;
        display: flex; flex-direction: column; gap: 10px;
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        color: var(--text); text-align: left; cursor: pointer;
        transition: border-color var(--t-fast), box-shadow var(--t-fast), transform var(--t-fast);
    }
    .puck.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    .puck.selected { border-color: var(--on); box-shadow: 0 0 0 1px var(--on); }
    .puck:active { transform: scale(0.98); }
    .check {
        position: absolute; top: 12px; right: 12px;
        width: 20px; height: 20px; border-radius: 50%;
        display: grid; place-items: center;
        background: var(--card-3); border: 1.5px solid var(--border-strong);
        color: transparent;
    }
    .puck.selected .check { background: var(--on); border-color: var(--on); color: var(--primary-fg); }
    .puck-icon {
        width: 34px; height: 34px; border-radius: var(--r-md);
        display: grid; place-items: center;
        background: var(--card-3); color: var(--text-mute);
    }
    .puck.playing .puck-icon { background: var(--on); color: var(--primary-fg); }
    .puck-body { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
    .puck-name { font-size: 14px; font-weight: 600; }
    .puck-sub {
        font-size: 11.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }

    /* ── Selection bar ── */
    .selbar {
        position: fixed; left: 50%; transform: translateX(-50%);
        bottom: calc(var(--space-5) + env(safe-area-inset-bottom));
        z-index: 45;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 10px 10px 10px 16px;
        max-width: min(440px, calc(100vw - 32px));
        background: var(--card); border: 1px solid var(--on);
        border-radius: var(--r-lg);
        box-shadow: var(--shadow-lg);
    }
    .sel-count { font-size: 13px; font-weight: 600; flex-shrink: 0; }
    .sel-names {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sel-go { margin-left: auto; border-radius: var(--r-pill); flex-shrink: 0; }
    @media (max-width: 900px) {
        .selbar {
            bottom: calc(var(--nav-clear) + var(--space-3));
            /* Clear the floating assistant button (56px @ right:16px), which
               shares this band — otherwise it covers the primary action. */
            padding-right: 64px;
        }
    }

    /* ── Docked mini-player ── */
    .mini {
        position: sticky;
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: 30;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 9px 10px;
        margin-top: var(--space-2);
        background: var(--tile-on-gradient);
        border: 1px solid var(--tile-on-border);
        border-radius: var(--r-lg);
        box-shadow: var(--shadow-md);
    }
    @media (max-width: 900px) {
        .mini {
            bottom: calc(var(--nav-clear) + var(--space-3));
            /* Same reserved gutter as .selbar — keep the play/pause control
               out from under the floating assistant button. */
            padding-right: 64px;
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

    /* ── Members / volume rows (shared: mini list + player sheet) ── */
    .members { display: flex; flex-direction: column; gap: 2px; }
    .member { display: flex; align-items: center; gap: var(--space-3); min-height: 44px; }
    .member .m-name {
        font-size: 13.5px; font-weight: 500; width: 110px; flex-shrink: 0;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .member .m-name.muted { color: var(--text-dim); }
    .m-mute, .m-act { width: 36px; height: 36px; flex-shrink: 0; }
    .member.off { color: var(--text-mute); }
    .member.off .m-name { width: auto; }
    .off-ip { margin-left: auto; font-size: 11px; color: var(--text-dim); }
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
    .sp-filters { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; }
    .sp-browse-label {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .sp-targets { margin-left: auto; }
    .sp-skeleton { height: 120px; border-radius: var(--r-md); }
    .sp-none { font-size: 12.5px; color: var(--text-mute); }

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
    .player {
        position: fixed; z-index: 126;
        left: 0; right: 0; bottom: 0;
        max-height: 92vh;
        background: var(--bg);
        border-radius: var(--r-xl) var(--r-xl) 0 0;
        border: 1px solid var(--hairline); border-bottom: 0;
        box-shadow: var(--shadow-lg);
        outline: none;
    }
    .grabber {
        width: 38px; height: 4px; border-radius: 2px;
        background: var(--border-strong);
        margin: 8px auto 0;
    }
    .player-scroll {
        max-height: calc(92vh - 12px); overflow-y: auto;
        padding: 0 var(--space-5)
            calc(var(--space-8) + env(safe-area-inset-bottom));
        display: flex; flex-direction: column; gap: var(--space-5);
    }
    /* On desktop the sheet becomes a centered dialog. */
    @media (min-width: 601px) {
        .player {
            left: 50%; bottom: auto; top: 50%;
            transform: translate(-50%, -50%);
            width: min(440px, calc(100vw - 48px));
            max-height: 88vh;
            border-radius: var(--r-xl); border-bottom: 1px solid var(--hairline);
        }
        .player-scroll { max-height: calc(88vh - 12px); }
    }

    /* Sticky so a long queue never scrolls the way out of the sheet away.
       The negative margin bleeds it over the scroll container's horizontal
       padding, so rows pass fully underneath it. */
    .player-head {
        position: sticky; top: 0; z-index: 2;
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
        margin: 0 calc(var(--space-5) * -1);
        padding: var(--space-3) var(--space-5);
        background: var(--bg);
    }
    .p-icon { width: 38px; height: 38px; border-radius: 50%; background: var(--card-2); border: 1px solid var(--hairline); }
    .p-onair { text-align: center; min-width: 0; }
    .p-onair-name { font-size: 13px; font-weight: 600; margin-top: 2px; }

    /* Art leads the sheet — it is the largest thing on screen, and the
       glow underneath is the same bulb glow a lit device gets. */
    .p-art { display: flex; justify-content: center; padding: var(--space-2) 0 0; }
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

    .p-speakers { display: flex; flex-direction: column; gap: 2px; }
    .p-speakers .eyrow { margin-bottom: var(--space-1); }
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
        .sp-more, .q-rm, .fav-add { width: 44px; height: 44px; }
        .sp-input, .sp-config input { font-size: 16px; } /* prevents iOS auto-zoom */
    }

    @media (prefers-reduced-motion: reduce) {
        .wave i { animation: none; height: 8px; }
        .fav-art, .now-card, .puck, .p-play,
        .p-upnext, .q-row, .sp-row { transition-duration: 0.001ms; }
    }
</style>
