<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Icon from "../components/Icon.svelte";
    import Waveform from "../components/music/Waveform.svelte";
    import ProgressLine from "../components/music/ProgressLine.svelte";
    import QuietCard from "../components/music/QuietCard.svelte";
    import NavRow from "../components/music/NavRow.svelte";
    import CardTransport from "../components/music/CardTransport.svelte";
    import NowCard from "../components/music/NowCard.svelte";
    import FavoriteCard from "../components/music/FavoriteCard.svelte";
    import DestinationRow from "../components/music/DestinationRow.svelte";
    import StartSomething from "../components/music/StartSomething.svelte";
    import SonosPlayer from "../components/music/SonosPlayer.svelte";
    import KEFPlayer from "../components/music/KEFPlayer.svelte";
    import ZonesSheet from "../components/music/ZonesSheet.svelte";
    import RoomPuck from "../components/music/RoomPuck.svelte";
    import SearchSheet from "../components/music/SearchSheet.svelte";
    import SpeakersScreen from "../components/music/SpeakersScreen.svelte";
    import { createPuckDrag } from "../lib/music/puck-drag.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import SpeakerModal from "../modals/SpeakerModal.svelte";
    import SonosEventsModal from "../modals/SonosEventsModal.svelte";
    import LiveStatusChip from "../components/LiveStatusChip.svelte";
    import { api } from "../lib/api";
    import { toasts, route, bottomBar } from "../lib/stores.svelte";
    import { onLive } from "../lib/live";
    import { openModal } from "../lib/modal.svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../lib/motion";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
    import * as sheetRun from "../lib/sheet-run";
    import type { SheetRun } from "../lib/sheet-run";
    import { settleScroll, restoreScroll, toTop } from "../lib/music/scroll";
    import { clock } from "../lib/music/clock.svelte";
    import { createBusy } from "../lib/music/busy.svelte";
    import { createSonosBridge } from "../lib/music/sonos.svelte";
    import { createKEFBridge } from "../lib/music/kef.svelte";
    import { createDestination } from "../lib/music/destination.svelte";
    import { createSearchHistory } from "../lib/music/history.svelte";
    import { createSpotify } from "../lib/music/spotify.svelte";
    import type {
        SonosSpeakerView, SonosGroupView, SonosFavorite,
        KEFSpeakerView, SpotifyItem,
    } from "../lib/types";

    // Both bridges, as state. They sit beside each other rather than one
    // folded into the other: a Sonos household is zones that group and share a
    // sonos.queue, a KEF speaker is one standalone stereo pair with an input
    // selector, and DESIGN.md §15 is explicit that neither should have to
    // pretend to be the other. The busy map is shared, since the key namespace
    // is what keeps their controls from disabling each other.
    const busy = createBusy();
    const sonos = createSonosBridge(busy);
    const kef = createKEFBridge(busy);

    /** Speakers that answered, across both bridges — "ready" on the Home head. */
    const readyCount = $derived(sonos.reachable.length + kef.reachable.length);
    /** Every registered speaker across both bridges — what "is this view empty" means. */
    const totalSpeakers = $derived((sonos.status?.speakers.length ?? 0) + kef.speakers.length);
    const playingCount = $derived(sonos.playingGroups.length);

    // ── Where playback lands ─────────────────────────────────────────────
    // One destination for the whole module (DESIGN.md §15, "one visible
    // destination"), and it spans both bridges — so it carries a kind rather
    // than being a bare id. A Sonos zone is started through its coordinator's
    // sonos.queue; a KEF speaker through Spotify Connect, because its own API can
    // play and pause but has nothing to be handed. The two are not
    // interchangeable, which is exactly why the destination says which it is.
    const destination = createDestination(sonos, kef);
    $effect(() => destination.settle());

    onMount(() => {
        void sonos.refresh();
        void kef.refresh();
        // Speaker changes arrive pushed — someone pressing play on the
        // speaker itself lands here in well under a second instead of
        // whenever the next poll happens to run.
        stopLive = onLive("music", () => {
            void sonos.refresh();
            // The KEF poller publishes on the same topic when a speaker
            // actually changes, so this catches both bridges.
            void kef.refresh();
        });
    });
    onDestroy(() => {
        clearInterval(pollTimer);
        stopLive?.();
        clearTimeout(announceTimer);
        for (const t of followUps) clearTimeout(t);
        followUps.clear();
        drag.end(); // takes the document-level touchmove block with it
        // The body-scroll lock is the sheet effect's, and its teardown runs on
        // unmount — releasing it here as well would decrement it twice.
    });

    // The poll is the backstop, not the mechanism. When the backend has the
    // speakers' notifications it only has to catch what those don't carry —
    // the track position, which the player extrapolates between reads
    // anyway — so it can run four times slower. When it doesn't, this is
    // the only thing keeping the view current, and stays at the old rate.
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let stopLive: (() => void) | undefined;
    $effect(() => {
        clearInterval(pollTimer);
        pollTimer = setInterval(
            () => {
                void sonos.refresh();
                // KEF has no push to subscribe to, but the backend polls the
                // speakers once for the whole process and pushes `music` on a
                // real change — so this is a backstop for both, not the
                // mechanism, and rides the same interval.
                void kef.refresh();
            },
            sonos.livePush ? 20_000 : 5_000,
        );
        return () => clearInterval(pollTimer);
    });

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
                await (bridge === "kef" ? kef.refresh() : sonos.refresh());
                // A KEF play answers as soon as *Spotify* accepted it — the
                // audio then goes out to the cloud and comes back to the
                // speaker, so the read above still says "stopped" and the
                // toast promised music no card was showing yet. The backend
                // re-reads at 0.6s and 3s and publishes `music` when it finds
                // the change; these are the backstop for an install where that
                // push isn't getting through.
                if (bridge === "kef") {
                    for (const ms of [1200, 4000]) followUp(ms, kef.refresh);
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
    function playFavorite(f: SonosFavorite, target: string | null = destination.sonosTarget) {
        if (!target) return;
        const g = sonos.groupById(target);
        void startPlayback(
            "fav:" + f.id,
            () => api.sonosPlayFavorite(target, f),
            f.title,
            g ? sonos.groupTitle(g) : "",
        );
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
        sheetDismissing = false;
    }
    /** Close the open sheet — back to the one it was raised over, if any. */
    function dropSheet() {
        if (!sheetUp) return;
        const back = sheetRun.dismiss(sheets);
        sheets = back;
        if (back.open) restoreSheetScroll(back.open);
        if (back.open !== "player") { playerGroupId = null; playerKefId = null; }
        drag.release();
    }
    /** Leave sheets entirely, whatever is up and whatever is under it. */
    function hideSheet() {
        if (!sheetUp) return;
        sheets = sheetRun.closeAll(sheets);
        playerGroupId = null;
        playerKefId = null;
        drag.release();
    }

    /** True when Search was opened to type in, rather than to read. */
    let searchWantsFocus = $state(false);

    function openSearch() {
        searchWantsFocus = true; // you came here to type
        showSheet("search");
    }
    function openZones() {
        showSheet("zones");
    }

    // ── Zones: drag one room onto another to group ───────────────────────
    // The gesture engine is its own module: it is the whole of grouping
    // (pointer, hold, edge-scroll, keyboard, live region), and its ghost has
    // to render outside the sheet, whose drag transform would otherwise
    // re-anchor it.
    const drag = createPuckDrag({
        scroller: () => scrollEl,
        zoneOf: (id) => sonos.groupOfSpeaker(id)?.coordinator_id,
        describe: (id) => ({
            playing: sonos.speakerPlaying(id),
            sub: sonos.speakerNowLine(id),
        }),
        group: (source, target) => void groupOnto(source, target),
        announce: (msg) => announce(msg),
    });

    async function groupOnto(sourceId: string, targetId: string) {
        const done = await sonos.groupOnto(sourceId, targetId);
        // A drag has no running commentary of its own, so the one thing this
        // view adds over the bridge call is saying out loud what happened.
        if (done) announce(`${done.source} now plays with ${done.target}.`);
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
    // play/pause/next/previous. What it hasn't got is a sonos.queue and a group,
    // so its sheet drops those two sections and keeps the rest.
    let playerGroupId = $state<string | null>(null);
    let playerKefId = $state<string | null>(null);
    const activeGroup = $derived(
        sonos.groups.find((g) => g.coordinator_id === playerGroupId),
    );
    const activeKef = $derived(
        playerKefId ? (kef.speakers.find((s) => s.id === playerKefId) ?? null) : null,
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
        const g = sonos.playingGroups[0];
        if (g) lastLiveId = g.coordinator_id;
    });
    const pausedGroup = $derived(
        sonos.groups.find((g) => {
            if (g.coordinator_id !== lastLiveId) return false;
            const st = sonos.coordinatorOf(g)?.state;
            return !!st?.track?.title && st.transport_state !== "STOPPED";
        }),
    );
    const dockGroup = $derived(sonos.playingGroups[0] ?? pausedGroup);

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

    let playerEl = $state<HTMLElement | null>(null);
    // Whichever player is mounted — only ever one — so the keyboard router can
    // hand the transport keys to it.
    let sonosPlayer = $state<SonosPlayer | null>(null);
    let kefPlayer = $state<KEFPlayer | null>(null);
    let searchSheet = $state<SearchSheet | null>(null);
    let speakersScreen = $state<SpeakersScreen | null>(null);
    // Set by the open sheet while a drag-down rides out. The art swipe stands
    // down for those 220ms; raising a sheet clears it, since the flag belongs
    // to the sheet that is leaving, not to the one arriving.
    let sheetDismissing = $state(false);

    function openPlayer(g: SonosGroupView) {
        playerKefId = null; // one player at a time
        playerGroupId = g.coordinator_id;
        raisePlayer();
        // The room you just opened is also where you'd expect the next
        // favorite or search result to land, so opening the player sets the
        // destination too — one choice instead of two.
        destination.current = { kind: "sonos", id: g.coordinator_id };
    }
    /** The same gesture for a KEF room, so the chips beside it don't lie. */
    function openKEFPlayer(sp: KEFSpeakerView) {
        if (!sp.reachable) return void openKEFModal(sp); // fix the address instead
        playerGroupId = null;
        playerKefId = sp.id;
        raisePlayer();
        destination.current = { kind: "kef", id: sp.id };
    }
    function raisePlayer() {
        // Opened from Zones or Search, the player *replaces* that sheet and
        // puts it back on the way out — a swap, so there is never a sheet over
        // a sheet, and never a lost place either. "Its place" includes how far
        // it was scrolled, which is why the outgoing offset is kept.
        rememberSheetScroll();
        sheets = sheetRun.swapTo(sheets, "player");
        sheetScroll.player = 0;
        sheetDismissing = false;
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
        // A recent search is a request to *run* it, so it runs — and the caret
        // stays out of the way, keyboard and all, since the results are what
        // was asked for.
        searchWantsFocus = !q;
        sheets = sheetRun.swapTo(sheets, "search");
        sheetScroll.search = 0;
        sheetDismissing = false;
        if (q) spotify.runQuery(q);
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
            if (searchSheet?.closeMenu()) return;
            if (drag.drag || drag.grabId) {
                // Put a held room back before leaving the sheet it was held in.
                const name =
                    drag.grabbedName((id) => sonos.speakerById.get(id)?.name) ||
                    drag.drag?.name ||
                    "Room";
                drag.release();
                announce(`${name} put back.`);
            } else if (openSheet) dropSheet();
            // Escape backs out of a speaker's settings the same way its back
            // chip does — a drill-down owes the user the key that leaves it.
            else if (speakersScreen?.closeDetail()) return;
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

        // Past this point the keys belong to whichever player is up. Each one
        // binds what its hardware can answer — a KEF has no seek, no queue and
        // no play modes — so the shell routes rather than deciding.
        kefPlayer?.handleKey(e, { slider, onControl });
        sonosPlayer?.handleKey(e, { slider, onControl });
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
        drag.prune(new Set(sonos.reachable.map((s) => s.id)));
    });
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
        if (!playerOpen && playingCount === 0 && kef.playing.length === 0) return;
        return clock.start();
    });

    /** The open sheet's scroll container, for scroll restore and edge-scroll. */
    let scrollEl = $state<HTMLElement | null>(null);

    // Load the queue whenever the player binds to a group: the "Up next" row
    // needs a real track name, not just a count.
    $effect(() => {
        const id = playerGroupId;
        if (id === null) {
            sonos.dropQueue();
            return;
        }
        void sonos.loadQueue(id, true);
    });

    /**
     * Clearing stops playback, so it gets the same confirm treatment as any
     * other destructive action — which is why this stays here rather than on
     * the bridge: the bridge has no business raising a dialog.
     */
    async function clearQueue(g: SonosGroupView) {
        const c = sonos.coordinatorOf(g);
        if (!c) return;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Clear the sonos.queue?",
            message: `Every track queued on ${sonos.groupTitle(g)} will be removed, and playback stops.`,
            confirmLabel: "Clear sonos.queue",
            danger: true,
        });
        if (!ok) return;
        await sonos.clearQueue(c.id);
    }

    /**
     * Queue a search result or favorite without disturbing what's playing.
     * The toast is the point: queueing onto a group playing radio is legal but
     * silent, so the feedback has to name where it landed.
     */
    async function enqueue(
        item: { uri: string; title?: string; service?: string; metadata?: string },
        next: boolean,
        target: string | null = destination.sonosTarget,
    ) {
        if (!target) return;
        const added = await sonos.enqueue(target, item, next);
        if (!added) return;
        const where = added.track ? `position ${added.track} of ${added.length}` : "the sonos.queue";
        toasts.success(
            next ? "Playing next" : "Added to sonos.queue",
            `${item.title ?? "Track"} · ${where}`,
        );
        if (playerGroupId === target) void sonos.loadQueue(target);
    }

    // ── Spotify search ───────────────────────────────────────────────────
    // Keyed by the destination, so "recent searches" in the kitchen aren't the
    // bedroom's. A single-room home only ever has one key.
    const recents = createSearchHistory(() => destination.key);
    const spotify = createSpotify((q) => recents.add(q));

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
        // Search lives behind an icon, and the round trip to Spotify ends
        // here — land back on the sheet the user left, not on a Home screen
        // with no sign of what just happened.
        if (q.spotify || q.spotify_error) openSearch();
        void spotify.load();
    });

    /**
     * Disconnecting drops the tokens, so the card falls back to the connect
     * page and the only way forward is the full OAuth flow again. An
     * accidental tap must not strand the user there — hence the confirm, which
     * lives here rather than in the store: raising a dialog is a surface's job.
     */
    async function disconnectSpotify() {
        const who = spotify.status?.display_name
            ? `"${spotify.status.display_name}"`
            : "Your Spotify account";
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Disconnect Spotify?",
            message: `${who} will be unlinked. To search again you'll need to reconnect through Spotify.`,
            confirmLabel: "Disconnect",
            danger: true,
        });
        if (ok) await spotify.disconnect();
    }

    // A search result plays on whichever destination is selected. Same tap,
    // same body, two roads: a Sonos group loads it into its queue and streams
    // it with the household's linked account, while a KEF speaker is started
    // through Spotify Connect — its own API has no way to be handed content.
    function playItem(item: SpotifyItem) {
        const d = destination.current;
        if (!d) return;
        const body = { service: "Spotify", uri: item.uri, title: item.name };
        void startPlayback(
            "item:" + item.uri,
            () => (d.kind === "kef" ? api.kefPlayItem(d.id, body) : api.sonosPlayItem(d.id, body)),
            item.name,
            destination.name(d),
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
            void sonos.refresh();
            void kef.refresh();
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
            void kef.refresh();
        }
    }

    // The push-status sheet. Retrying inside it can turn subscriptions on, and
    // that changes which poll interval this view should be using, so the
    // status is re-read on the way out.
    async function openEventsModal() {
        await openModal(SonosEventsModal, {});
        void sonos.refresh();
    }

    // ── Speakers screen ──────────────────────────────────────────────────
    // Which speaker's settings the screen has open. Held here rather than
    // inside it because the KEF player's settings chip pushes the screen *and*
    // opens a pane in one gesture — it has to be able to name the pane before
    // the screen exists.
    let detailId = $state<string | null>(null);
    let kefDetailId = $state<string | null>(null);

    function openKEFSpeaker(sp: KEFSpeakerView) {
        detailId = null; // one pane at a time
        kefDetailId = sp.id;
        // Same reasoning as openPlayer: the speaker you just opened is where
        // you'd expect the next search result to land. Only when it can
        // actually take one.
        if (sp.reachable) destination.current = { kind: "kef", id: sp.id };
    }

</script>

<svelte:window onkeydown={onWindowKey} onpopstate={onPopState} />

<!-- Anything a grouping gesture does that has no visible running commentary
     — the keyboard path especially — is said here instead. -->
<div class="sr-only" role="status" aria-live="polite">{liveMsg}</div>

{#if screen === "home"}
    <Topbar
        title="Music"
        subtitle={sonos.status
            ? `${totalSpeakers} speaker${totalSpeakers === 1 ? "" : "s"} · ${playingCount + kef.playing.length} playing`
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
            {#if sonos.loaded && (sonos.status?.speakers.length ?? 0) > 0}
                <LiveStatusChip live={sonos.livePush} onClosed={() => void sonos.refresh()} />
            {/if}
            <!-- Search rides in the header rather than in a subnav pill:
                 nothing sits below this header but content (DESIGN.md §15).
                 It wears its label wherever there is width for one — losing
                 the pill shouldn't cost desktop a *named* way in when the
                 room to name it was never the problem. On a phone the label
                 is what pushed the subtitle to a stub, so there it is the
                 icon alone. Registering a speaker isn't here at all: it
                 belongs on Speakers, with the rest of device management. -->
            {#if sonos.loaded && totalSpeakers > 0}
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

{#if !sonos.loaded}
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

{#if sonos.loaded && totalSpeakers > 0}
    {#if screen === "home"}
    <!-- ── Playing now ─────────────────────────────────────────────────
         Only what is actually playing. Idle zones are one tap away in the
         room chips below, so listing them here would just make the heading
         lie and bury the thing the user came for. -->
    <section class="block">
        <div class="eyrow">Playing now</div>
        {#if sonos.playingGroups.length === 0 && kef.playing.length === 0}
            <QuietCard
                title="Nothing playing"
                action={spotify.status
                    ? {
                          // Not gated on `connected`: the people who most need
                          // a pointer at Spotify are the ones who haven't set
                          // it up, and with the subnav gone this card and the
                          // header icon are the only things that say the
                          // module searches at all (DESIGN.md §15).
                          label: spotify.connected ? "Search" : "Set up Spotify",
                          onClick: openSearch,
                      }
                    : undefined}
            >
                <span class="mono">{readyCount}</span>
                speaker{readyCount === 1 ? "" : "s"} ready —
                {sonos.favorites.length > 0 && !destination.kefSpeaker
                    ? "start a favorite below"
                    : "pick a room to open it"}
            </QuietCard>
        {:else}
            <div class="now-grid">
                {#each sonos.playingGroups as g (g.coordinator_id)}
                    {@const c = sonos.coordinatorOf(g)}
                    {@const st = c?.state}
                    <NowCard
                        name={sonos.groupTitle(g)}
                        line={[st?.track?.title, st?.track?.artist].filter(Boolean).join(" · ") ||
                            "Live audio"}
                        artUri={st?.track?.art_uri}
                        playing
                        progress={sonos.progressOf(g)}
                        onOpen={() => openPlayer(g)}
                        isDock={g.coordinator_id === dockGroup?.coordinator_id}
                        onDockVisible={(v) => (dockCardOnScreen = v)}
                    >
                        {#snippet transport()}
                            <CardTransport
                                playing={sonos.isPlaying(g)}
                                onToggle={() => sonos.togglePlay(g)}
                                toggleBusy={!c || busy.is("play:" + c?.id)}
                                onPrev={() => sonos.skip(g, "previous")}
                                prevBusy={!c || busy.is("previous:" + c?.id)}
                                onNext={() => sonos.skip(g, "next")}
                                nextBusy={!c || busy.is("next:" + c?.id)}
                            />
                        {/snippet}
                    </NowCard>
                {/each}

                <!-- KEF speakers that are playing, in the same grid and with
                     the same card. It is a way in to a player like every
                     other card here — the sheet it opens drops the queue and
                     the group, which KEF hasn't got, and keeps the rest. -->
                {#each kef.playing as sp (sp.id)}
                    <NowCard
                        name={sp.name}
                        line={[kef.nowLine(sp), kef.subLine(sp)].filter(Boolean).join(" · ")}
                        artUri={sp.state?.track?.art_uri}
                        playing
                        progress={kef.progress(sp)}
                        onOpen={() => openKEFPlayer(sp)}
                    >
                        {#snippet transport()}
                            <!-- Play/pause only, like the Sonos card below
                                 430px: the sheet is where the skips live. -->
                            <CardTransport
                                playing={kef.isPlaying(sp)}
                                onToggle={() => kef.togglePlay(sp)}
                                toggleBusy={busy.is("kefplay:" + sp.id)}
                            />
                        {/snippet}
                    </NowCard>
                {/each}
            </div>
        {/if}
    </section>

    <!-- ── Favorites ───────────────────────────────────────────────── -->
    {#if sonos.favorites.length > 0}
        <section class="block">
            <div class="block-head">
                <div class="eyrow">Favorites</div>
                {@render targetRow()}
            </div>
            {#if destination.kefSpeaker}
                <!-- "My Sonos" is a household list, and a KEF speaker has no
                     way to play an entry from it. A rail of disabled cards
                     would be a row of dead controls (§15), so the section
                     says what it needs instead — and the fix is one tap on
                     the destination row directly above. -->
                <QuietCard
                    title="Favorites need a Sonos room"
                    action={spotify.status
                        ? {
                              label: spotify.connected ? "Search" : "Set up Spotify",
                              onClick: openSearch,
                          }
                        : undefined}
                >
                    They come out of your Sonos household, so {destination.kefSpeaker.name} can't
                    play one — pick a Sonos room above{#if spotify.connected}, or search to play
                        there{/if}.
                </QuietCard>
            {:else}
                <div class="favs h-scroll">
                    {#each sonos.favorites as f (f.id)}
                        {@render favShelf(f, destination.sonosTarget)}
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
            {#each sonos.reachable as sp (sp.id)}
                {@const g = sonos.groupOfSpeaker(sp.id)}
                <button
                    class="room-chip"
                    class:on={sonos.speakerPlaying(sp.id)}
                    disabled={!g}
                    onclick={() => g && openPlayer(g)}
                >
                    {#if sonos.speakerPlaying(sp.id)}
                        <Waveform />
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
            {#each kef.reachable as sp (sp.id)}
                <button
                    class="room-chip"
                    class:on={kef.isPlaying(sp)}
                    onclick={() => openKEFPlayer(sp)}
                >
                    {#if kef.isPlaying(sp)}
                        <Waveform />
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
    <NavRow icon="speaker" title="Speakers" count={totalSpeakers} onClick={openSpeakers}>
        {#snippet sub()}
            {#if sonos.offline.length > 0}
                <span class="mono">{sonos.offline.length}</span>
                unreachable — fix an address, or set one up
            {:else}
                Names, addresses, tone and the status light
            {/if}
        {/snippet}
    </NavRow>

    {:else}
    <SpeakersScreen
        {sonos}
        {kef}
        {totalSpeakers}
        {readyCount}
        onBack={leaveSpeakers}
        onAdd={() => openSpeakerModal()}
        onEditSonos={(sp) => void openSpeakerModal(sp)}
        onEditKEF={(sp) => void openKEFModal(sp)}
        onOpenEvents={openEventsModal}
        onKEFOpened={(sp) => (destination.current = { kind: "kef", id: sp.id })}
        bind:this={speakersScreen}
        bind:detailId
        bind:kefDetailId
    />
    {/if}

    <!-- ── Docked mini-player ──────────────────────────────────────────
         Present everywhere — including over the Zones and Search sheets,
         which is where the transport would otherwise disappear — but stands
         down while the Home card it would duplicate is on screen. It also
         survives a pause: that is where a paused zone stays reachable once
         "Playing now" (which means playing, literally) has let go of it. -->
    {#if showDock && dockGroup}
        {@const c = sonos.coordinatorOf(dockGroup)}
        {@const st = c?.state}
        {@const dockPlaying = sonos.isPlaying(dockGroup)}
        {@const p = sonos.progressOf(dockGroup)}
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
                        {[st?.track?.artist, sonos.groupTitle(dockGroup)].filter(Boolean).join(" · ")}
                    </div>
                </div>
                <!-- Playing is a waveform; a zone the dock is holding open
                     after a pause gets the idle speaker icon instead. -->
                {#if dockPlaying}
                    <Waveform />
                {:else}
                    <span class="mini-idle" aria-hidden="true"><Icon name="speaker" size={14} /></span>
                {/if}
            </button>
            <CardTransport
                playing={dockPlaying}
                onToggle={() => sonos.togglePlay(dockGroup)}
                toggleBusy={!c || busy.is("play:" + c?.id)}
                onPrev={() => sonos.skip(dockGroup, "previous")}
                prevBusy={!c || busy.is("previous:" + c?.id)}
                onNext={() => sonos.skip(dockGroup, "next")}
                nextBusy={!c || busy.is("next:" + c?.id)}
            />
            <ProgressLine value={p} />
        </div>
    {/if}
{/if}

<!-- Both surfaces that start something — the favorites shelf and the search
     results — point at the same destination row, so it is rendered through one
     snippet rather than placed twice. -->
{#snippet targetRow()}
    <DestinationRow {destination} kefStart={sonos.groups.length} />
{/snippet}

<!-- Tap the art to play it on `target`, the corner + to queue it. Shared by
     the Home shelf and the idle player. -->
{#snippet favShelf(f: SonosFavorite, target: string | null)}
    <FavoriteCard
        favorite={f}
        {target}
        playBusy={busy.is("fav:" + f.id)}
        queueBusy={busy.is("q:" + f.uri)}
        onPlay={() => playFavorite(f, target)}
        onQueue={() => enqueue({ uri: f.uri, title: f.title, metadata: f.metadata }, false, target)}
    />
{/snippet}

{#if zonesOpen}
    <ZonesSheet
        {sonos}
        {kef}
        {busy}
        {drag}
        docked={showDock}
        onDismiss={dropSheet}
        onOpenRoom={openPlayer}
        onOpenSpeakers={openSpeakers}
        bind:scrollEl
    />
{/if}

<!-- The travelling ghost lives out here, not in the sheet: `.sheet` takes a
     transform while it is dragged, and a `position: fixed` descendant would be
     anchored to that rather than to the viewport. -->
{#if drag.drag}
    <RoomPuck ghost={drag.drag} />
{/if}

<!-- ── Search sheet ─────────────────────────────────────────────────
     Behind a plain search icon in Home's header, opening the same way
     everything else in Music opens. -->
{#if searchOpen}
    <SearchSheet
        {spotify}
        {recents}
        {destination}
        {busy}
        autofocus={searchWantsFocus}
        docked={showDock}
        onDismiss={dropSheet}
        onDisconnect={disconnectSpotify}
        onPlayItem={playItem}
        onEnqueue={(item, next) =>
            enqueue({ service: "Spotify", uri: item.uri, title: item.name }, next)}
        {targetRow}
        bind:this={searchSheet}
        bind:scrollEl
    />
{/if}

<!-- ── The players ──────────────────────────────────────────────────
     Two sheets, not one: a Sonos zone has a queue, a group and play modes,
     and a KEF speaker has an input selector instead. They share every section
     that is genuinely the same object (art, meta, transport, volume rows,
     "Start something"), and differ where the hardware does. -->
{#if playerOpen && activeKef}
    <KEFPlayer
        speaker={activeKef}
        {kef}
        {busy}
        onClose={closePlayer}
        onSettings={() => {
            const sp = activeKef;
            hideSheet();
            openSpeakers();
            if (sp) openKEFSpeaker(sp);
        }}
        bind:this={kefPlayer}
        bind:scrollEl
        bind:sheetEl={playerEl}
        bind:dismissing={sheetDismissing}
    >
        {#snippet startSomething()}
            {@render startRow(null)}
        {/snippet}
    </KEFPlayer>
{/if}

{#if playerOpen && activeGroup}
    <SonosPlayer
        group={activeGroup}
        {sonos}
        {busy}
        onClose={closePlayer}
        onClearQueue={() => activeGroup && clearQueue(activeGroup)}
        bind:this={sonosPlayer}
        bind:scrollEl
        bind:sheetEl={playerEl}
        bind:dismissing={sheetDismissing}
    >
        {#snippet startSomething(favTarget: string | null)}
            {@render startRow(favTarget)}
        {/snippet}
    </SonosPlayer>
{/if}

<!-- The player-side "Start something" row, shared by both sheets. -->
{#snippet startRow(favTarget: string | null)}
    <StartSomething
        spotifyAvailable={!!spotify.status}
        spotifyConnected={spotify.connected}
        recents={recents.recent}
        favorites={favTarget ? sonos.favorites : []}
        onSearch={searchFromPlayer}
    >
        {#snippet favCard(f)}
            {@render favShelf(f, favTarget)}
        {/snippet}
    </StartSomething>
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

    .link-btn {
        background: none; border: 0; padding: 0;
        color: var(--text-mute); font-size: 12.5px; cursor: pointer;
    }
    .link-btn:hover { color: var(--text); }

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

    /* ── Playing-now cards ── */
    .now-grid {
        display: grid;
        /* Wide enough that the track title still has room next to the
           three-button transport — narrower columns crushed it to an
           ellipsis on desktop. */
        grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
        gap: var(--space-3);
    }
    /* ── Favorites ── */
    .favs { display: flex; gap: var(--space-3); padding-bottom: var(--space-1); }
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

    /* ── Touch: hit areas grow to the 44px floor ── */
    @media (prefers-reduced-motion: reduce) {
        .mini { transition-duration: 0.001ms; }
    }
</style>
