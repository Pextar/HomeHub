<script lang="ts">
    /**
     * Music.
     *
     * The module speaks one noun — a **room** — and has three surfaces:
     *
     *   Home     the hero (what is playing, and every control for it) over a
     *            grid of rooms. Tap a room to focus it, tap it again to open
     *            it, drag one onto another to play them together.
     *   Browse   everywhere music comes from: favorites, recent searches,
     *            Spotify. It plays to the room the picker at its top names,
     *            which is the same room Home is focused on.
     *   Speakers the devices themselves — names, addresses, tone.
     *
     * Plus one player sheet, for any room, and one editor sheet for a room the
     * user built themselves.
     *
     * Underneath, three bridges that are genuinely different — a Sonos
     * household groups and shares a queue, a KEF speaker is a standalone
     * stereo pair with an input selector, and a HomeHub zone is the only thing
     * that can span both — but that split is `lib/music/rooms.svelte.ts`'s
     * problem now, not the screen's. Nothing in here branches on a make.
     */
    import { onMount, onDestroy } from "svelte";
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Icon from "../components/Icon.svelte";
    import FavoriteCard from "../components/music/FavoriteCard.svelte";
    import CardTransport from "../components/music/CardTransport.svelte";
    import StartSomething from "../components/music/StartSomething.svelte";
    import RoomPicker from "../components/music/RoomPicker.svelte";
    import RoomCard from "../components/music/RoomCard.svelte";
    import Player from "../components/music/Player.svelte";
    import ZoneEditor from "../components/music/ZoneEditor.svelte";
    import SearchScreen from "../components/music/SearchScreen.svelte";
    import SpeakersScreen from "../components/music/SpeakersScreen.svelte";
    import ArtistScreen from "../components/music/ArtistScreen.svelte";
    import ContextScreen from "../components/music/ContextScreen.svelte";
    import FavoriteBrowseScreen from "../components/music/FavoriteBrowseScreen.svelte";
    import MiniPlayer from "../components/music/MiniPlayer.svelte";
    import MusicHome from "../components/music/MusicHome.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import SpeakerModal from "../modals/SpeakerModal.svelte";
    import SonosEventsModal from "../modals/SonosEventsModal.svelte";
    import MusicQualityModal from "../modals/MusicQualityModal.svelte";
    import SpotifyConnectModal from "../modals/SpotifyConnectModal.svelte";
    import QobuzConnectModal from "../modals/QobuzConnectModal.svelte";
    import LiveStatusChip from "../components/LiveStatusChip.svelte";
    import { api } from "../lib/api";
    import { toasts, route, bottomBar } from "../lib/stores.svelte";
    import { onLive } from "../lib/live";
    import { openModal } from "../lib/modal.svelte";
    import { lockBodyScroll, unlockBodyScroll } from "../lib/scroll-lock";
    import { originOf, type Origin } from "../lib/motion";
    import * as sheetRun from "../lib/sheet-run";
    import type { SheetRun } from "../lib/sheet-run";
    import { settleScroll, restoreScroll, toTop } from "../lib/music/scroll";
    import { clock } from "../lib/music/clock.svelte";
    import { createBusy } from "../lib/music/busy.svelte";
    import { createSonosBridge } from "../lib/music/sonos.svelte";
    import { createKEFBridge } from "../lib/music/kef.svelte";
    import { createAirPlayBridge } from "../lib/music/airplay.svelte";
    import { createUPnPBridge } from "../lib/music/upnp.svelte";
    import { createZonesBridge } from "../lib/music/zones.svelte";
    import { createRooms } from "../lib/music/rooms.svelte";
    import type { Room } from "../lib/music/rooms.svelte";
    import { createRoomDrag } from "../lib/music/room-drag.svelte";
    import { createDestination } from "../lib/music/destination.svelte";
    import { createSearchHistory } from "../lib/music/history.svelte";
    import { createHeardLog } from "../lib/music/heard.svelte";
    import { createSpotify } from "../lib/music/spotify.svelte";
    import type {
        SonosSpeakerView,
        HeardTrack,
        SonosFavorite,
        KEFSpeakerView,
        AirPlaySpeakerView,
        SpotifyItem,
        SpotifyArtistDetail,
        SpotifyContextDetail,
        MediaZone,
    } from "../lib/types";

    // The three bridges, and the model that turns them into rooms. The busy
    // map is shared: its key namespace is what keeps a play on one room from
    // disabling the play on another.
    const busy = createBusy();
    const sonos = createSonosBridge(busy);
    const kef = createKEFBridge(busy);
    // No busy handle: an AirPlay receiver has no transport of its own to be
    // busy with — see lib/music/airplay.svelte.ts.
    const airplay = createAirPlayBridge();
    const upnp = createUPnPBridge();
    const zones = createZonesBridge(busy);
    const rooms = createRooms(sonos, kef, zones, busy);

    /** Every registered speaker across the bridges — what "is this empty" means. */
    const totalSpeakers = $derived(
        (sonos.status?.speakers.length ?? 0) + kef.speakers.length + airplay.receivers.length,
    );
    /**
     * Speakers that answered — the Home head's "ready".
     *
     * AirPlay receivers are counted as registered but never as reachable, and
     * that is the honest reading rather than an oversight: finding out whether
     * a receiver is there means opening a session, which would take it away
     * from whatever else is playing to it. Counting them as ready would be a
     * claim nothing checked.
     */
    const readyCount = $derived(sonos.reachable.length + kef.reachable.length);

    // One focused room for the whole module: the hero shows it, the player
    // opens it, and anything started on Browse goes to it.
    const destination = createDestination(rooms);
    $effect(() => destination.settle());

    onMount(() => {
        void sonos.refresh();
        void kef.refresh();
        void airplay.refresh();
        void upnp.refresh();
        void spotify.loadProviders();
        void zones.refresh();
        // The endpoint list is what the room editor picks from; it changes only
        // when a speaker is registered or removed, so it is read here and after
        // a registration, never on the poll.
        void zones.loadEndpoints();
        // Speaker changes arrive pushed — someone pressing play on the speaker
        // itself lands here in well under a second.
        stopLive = onLive("music", () => {
            void sonos.refresh();
            void kef.refresh();
            void zones.refresh();
            // Not on the live signal: nothing about a receiver changes
            // without HomeHub doing it, so this list only moves when a
            // registration or a cast does — both of which refresh it there.
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

    // The poll is the backstop, not the mechanism. When the speakers are
    // pushing their own changes it only has to catch what those don't carry —
    // the track position, which every surface extrapolates anyway — so it can
    // run four times slower.
    let pollTimer: ReturnType<typeof setInterval> | undefined;
    let stopLive: (() => void) | undefined;
    $effect(() => {
        clearInterval(pollTimer);
        pollTimer = setInterval(
            () => {
                void sonos.refresh();
                void kef.refresh();
                void zones.refresh();
            },
            sonos.livePush ? 20_000 : 5_000,
        );
        return () => clearInterval(pollTimer);
    });

    // ── Starting something ───────────────────────────────────────────────
    // The player is the confirmation: the track, the room and the route all
    // land on it as soon as the re-read below returns. It used to also say so
    // in a toast, which meant every tap on a search result was followed by a
    // card repeating what the screen already showed.
    async function startPlayback(
        key: string,
        fn: () => Promise<unknown>,
        kind: "sonos" | "kef" | "zone" = "sonos",
    ) {
        await busy.claim(key, async () => {
            try {
                await fn();
                await (kind === "kef"
                    ? kef.refresh()
                    : kind === "zone"
                      ? zones.refresh()
                      : sonos.refresh());
                // A KEF play answers as soon as *Spotify* accepted it — the
                // audio then goes out to the cloud and comes back — so the read
                // above still says "stopped". A streamed room has the same gap:
                // the decoder has to start and every speaker has to fill its
                // buffer. These are the backstop for an install where the
                // backend's own push isn't getting through.
                if (kind !== "sonos") {
                    const again = kind === "kef" ? kef.refresh : zones.refresh;
                    for (const ms of [1200, 4000]) followUp(ms, again);
                }
            } catch (e) {
                toasts.error("Couldn't play", (e as Error).message);
            }
        });
    }

    /** A delayed re-read that doesn't outlive the view. Nothing renders this
     *  set — it exists only so `onDestroy` can cancel what's still pending —
     *  so a reactive one would be bookkeeping for no reader. */
    // eslint-disable-next-line svelte/prefer-svelte-reactivity
    const followUps = new Set<ReturnType<typeof setTimeout>>();
    function followUp(ms: number, fn: () => void) {
        const t = setTimeout(() => {
            followUps.delete(t);
            fn();
        }, ms);
        followUps.add(t);
    }

    /**
     * A search result plays on the focused room. Same tap, three roads: a
     * Sonos room loads it into its queue and streams it with the household's
     * linked account; a KEF speaker is started through Spotify Connect,
     * because its own API has no way to be handed content; a HomeHub room
     * hands it to the media layer, which resolves a route across whatever
     * makes are in it and answers with the one it chose.
     */
    function playItem(item: SpotifyItem) {
        const r = destination.room;
        if (!r) return;
        const provider = item.provider ?? "spotify";
        if (r.zone) {
            const z = r.zone;
            void startPlayback(
                "item:" + item.uri,
                () => zones.play(z, { uri: item.uri, title: item.name, kind: item.kind, provider }),
                "zone",
            );
            return;
        }
        // A bare speaker is played through its own bridge, and those two doors
        // take a *native service* the speaker streams from its own account
        // link. Only Spotify has one here. Anything else has to go through the
        // media layer, which addresses zones — so the honest answer is to say
        // that rather than send a URI the speaker will ignore.
        if (provider !== "spotify") {
            toasts.error(
                `${item.name} can't play here`,
                "This service is decoded by HomeHub rather than by the speaker, so it plays to a zone. Put this speaker in a zone and pick that instead.",
            );
            return;
        }
        const body = { service: "Spotify", uri: item.uri, title: item.name };
        void startPlayback(
            "item:" + item.uri,
            () => (r.kind === "kef" ? api.kefPlayItem(r.id, body) : api.sonosPlayItem(r.id, body)),
            r.kind,
        );
    }

    /**
     * Something out of the listening log, played again in the room it was
     * heard in. It is a track and it carries the service URI the speaker was
     * given, so this is the same road a search result takes — the row was
     * only ever a remembered version of one.
     */
    function playHeard(t: HeardTrack) {
        if (!t.uri) return; // radio and line-in leave nothing to hand back
        playItem({
            kind: "track",
            uri: t.uri,
            name: t.title,
            sub: t.artist,
            art_url: t.art_uri,
        });
    }

    /** A room stops keeping a log. Destructive and unrecoverable, so it asks. */
    async function clearHeard(r: Room) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Clear what was played?",
            message: `${r.name} will forget the tracks it has been heard playing. What is playing now is unaffected, and the list starts again from the next song.`,
            confirmLabel: "Clear",
            danger: true,
        });
        if (!ok) return;
        await busy.run("heardclear:" + r.id, () => heard.clear(r.key), "Couldn't clear it");
    }

    /** Favorites are a Sonos household list, so only a Sonos room can take one. */
    function playFavorite(f: SonosFavorite, target: string | null = destination.sonosTarget) {
        if (!target) return;
        void startPlayback("fav:" + f.id, () => api.sonosPlayFavorite(target, f));
    }

    /**
     * Queue a search result or favorite without disturbing what's playing. The
     * toast is the point: queueing onto a room playing radio is legal but
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
        if (playerRoom?.id === target) void sonos.loadQueue(target);
    }

    // ── Screens ──────────────────────────────────────────────────────────
    // Home is the only fixed one. Everything else pushes over it with a back
    // chip; sheets lift from the bottom. A sheet never opens another sheet, so
    // anything that would be a second sheet is a screen instead.
    //
    // The pushed screens form a real stack — Browse opens an artist, the
    // artist opens an album — so "back" means *up one level*, the way every
    // music app's drill-down reads, never "all the way home". Only the top
    // of the stack renders; what each level was scrolled to is kept on its
    // entry and restored when it surfaces again.
    type Screen = "home" | "speakers" | "artist" | "favorite" | "browse" | "context";
    interface ScreenEntry {
        id: Exclude<Screen, "home">;
        /** Catalog screens: the artist / album / playlist URI they show. */
        uri?: string;
        /** Where this level was scrolled when something pushed over it. */
        scroll: number;
    }
    let stack = $state<ScreenEntry[]>([]);
    const screen = $derived<Screen>(stack.length ? stack[stack.length - 1].id : "home");
    const topEntry = $derived(stack.length ? stack[stack.length - 1] : undefined);

    /** Where Home was left, so coming back lands where you were. */
    let homeScrollY = 0;

    /**
     * The room to hand back to on the way out of a screen reached from that
     * room's open player — Browse, or an artist tapped inside it. Noted by
     * `pushScreen` from whether the player was up at the moment of the push,
     * so it takes no per-caller wiring and survives going deeper.
     */
    let playerReturn: string | null = null;

    function pushScreen(e: ScreenEntry) {
        if (sheets.open === "player") playerReturn = playerKey;
        hideSheet();
        if (stack.length === 0) homeScrollY = window.scrollY;
        else stack[stack.length - 1].scroll = window.scrollY;
        stack = [...stack, e];
        toTop();
    }
    function openSpeakers() {
        pushScreen({ id: "speakers", scroll: 0 });
    }
    /** True when Browse was opened to type in, rather than to read. */
    let searchWantsFocus = $state(false);
    function openBrowse(q?: string) {
        // A recent search is a request to *run* it, so it runs — and the caret
        // stays out of the way, since the results are what was asked for.
        searchWantsFocus = !q;
        pushScreen({ id: "browse", scroll: 0 });
        if (q) spotify.runQuery(q);
    }

    /** Per-level teardown, as a popped screen gives up its scratch state. */
    function onLeftScreen(e: ScreenEntry) {
        if (e.id === "speakers") {
            detailId = null;
            kefDetailId = null;
        } else if (e.id === "favorite") {
            browseFavorite = null;
            favoriteContext = null;
        }
    }

    /** Back means up one level — except when a player noted a room to come
     *  back to, which is owed exactly once the stack runs out. */
    function leaveScreen() {
        const leaving = stack[stack.length - 1];
        if (!leaving) return;
        onLeftScreen(leaving);
        if (stack.length > 1) {
            stack = stack.slice(0, -1);
            restoreScroll(stack[stack.length - 1].scroll);
            return;
        }
        stack = [];
        if (playerReturn) {
            const back = rooms.byKey(playerReturn);
            playerReturn = null;
            if (back) return openPlayer(back);
            // The room disappeared in the meantime (regrouped, removed) — fall
            // through to Home like any other missing target.
        }
        restoreScroll(homeScrollY);
    }

    // ── Sheets ───────────────────────────────────────────────────────────
    // Only ever one at a time. Sheets *swap* — they never stack — so there is
    // only ever one scrim, one Escape, one thing to swipe away.
    type Sheet = "player" | "room-edit";
    let sheets = $state<SheetRun<Sheet>>(sheetRun.closed());

    const openSheet = $derived(sheets.open);
    const editorOpen = $derived(sheets.open === "room-edit");
    const sheetUp = $derived(sheetRun.isUp(sheets));

    // ── Back closes one level ────────────────────────────────────────────
    // One history entry is held for the whole time Music is deeper than Home,
    // and re-taken after each step back while depth remains. So back always
    // means "up one", exactly like Escape and the back chip.
    const navDepth = $derived(
        (screen !== "home" ? 1 : 0) + (sheets.open ? (sheets.under ? 2 : 1) : 0),
    );
    let holdsEntry = false;

    $effect(() => {
        if (navDepth > 0) {
            if (!holdsEntry) {
                history.pushState({ musicNav: true }, "");
                holdsEntry = true;
            }
        } else if (holdsEntry) {
            holdsEntry = false;
            history.back();
        }
    });

    function onPopState() {
        if (navDepth === 0) return; // not our entry — a real route change
        holdsEntry = false; // the browser consumed it; the effect re-takes it
        if (sheetUp) dropSheet();
        else if (screen !== "home") leaveScreen();
    }

    // The body-scroll lock keys on *whether* a sheet is up, never on which — so
    // a swap doesn't release and retake it, which on iOS would unpin and re-pin
    // the body for a frame.
    $effect(() => {
        if (!sheetUp) return;
        lockBodyScroll();
        return unlockBodyScroll;
    });

    /** How far each sheet was scrolled when it handed over. */
    const sheetScroll: Partial<Record<Sheet, number>> = {};
    function rememberSheetScroll() {
        if (sheets.open) sheetScroll[sheets.open] = scrollEl?.scrollTop ?? 0;
    }
    function restoreSheetScroll(s: Sheet) {
        settleScroll(() => scrollEl, sheetScroll[s] ?? 0);
    }

    function dropSheet() {
        if (!sheetUp) return;
        const back = sheetRun.dismiss(sheets);
        sheets = back;
        if (back.open) restoreSheetScroll(back.open);
        if (back.open !== "player") playerKey = null;
        if (back.open !== "room-edit") editingZone = null;
        drag.release();
    }
    function hideSheet() {
        if (!sheetUp) return;
        sheets = sheetRun.closeAll(sheets);
        playerKey = null;
        editingZone = null;
        drag.release();
    }

    // ── The player ───────────────────────────────────────────────────────
    // Rendered inline (not via the modal stack) so it stays live against the
    // poll. Bound by room *key*, never by object: the list is rebuilt whenever
    // a poll lands, and a held object would go stale in a second.
    let playerKey = $state<string | null>(null);
    const playerRoom = $derived(rooms.byKey(playerKey));
    const playerOpen = $derived(openSheet === "player" && !!playerRoom);

    // What the player was opened from — the dock, a room card, the hero —
    // measured at the tap, because the opener is usually gone by the time the
    // sheet mounts. The sheet unfolds out of that frame and collapses back
    // into it, so the two surfaces read as one player at two sizes rather
    // than a second one arriving over the first. The body's scroll is locked
    // for the sheet's whole life, so the measurement is still true on the way
    // out. Reached any other way (the back gesture out of Browse, the
    // keyboard, reduced motion) it is null and the sheet slides as before.
    let playerOrigin = $state<Origin | null>(null);

    function openPlayer(r: Room, from?: HTMLElement | null) {
        // A room with nothing in it has no player worth opening — it stores,
        // but the media layer refuses to play to it. Editing it is the useful
        // thing instead.
        if (r.zone && r.members.length === 0) return openRoomEditor(r.zone);
        // An unreachable KEF speaker can't answer anything; fixing its address
        // is what the tap actually wants.
        if (r.speaker && !r.reachable) return void openKEFModal(r.speaker);
        playerOrigin = originOf(from);
        playerKey = r.key;
        destination.focus(r);
        // Opened from Browse, the player *replaces* that screen's sheet and
        // puts it back on the way out.
        rememberSheetScroll();
        sheets = sheetRun.swapTo(sheets, "player");
        sheetScroll.player = 0;
        sheetDismissing = false;
    }
    function closePlayer() {
        if (openSheet !== "player") return;
        dropSheet();
    }

    // A regroup between polls can retire the room the sheet is bound to. Close
    // rather than leaving an empty sheet behind — and, more importantly, a
    // permanently locked body scroll.
    $effect(() => {
        if (openSheet === "player" && playerKey !== null && !playerRoom) closePlayer();
    });
    // Move focus into the sheet when it opens so keyboard users land there.
    $effect(() => {
        if (playerOpen) playerEl?.focus();
    });

    // Load the queue whenever the player binds to a room that has one: the
    // "Up next" row needs a real track name, not just a count.
    $effect(() => {
        const r = playerRoom;
        if (!r?.canQueue) {
            sonos.dropQueue();
            return;
        }
        void sonos.loadQueue(r.id, true);
    });

    /** What the header's action opens: the room's own definition. */
    function configureRoom(r: Room) {
        if (r.zone) return openRoomEditor(r.zone);
        const sp = r.speaker;
        hideSheet();
        openSpeakers();
        if (sp) return openKEFSpeaker(sp);
        // A Sonos room's settings are its coordinator's, on the Speakers screen.
        kefDetailId = null;
        detailId = r.id;
    }

    /** Taking a natively grouped room apart, from inside its player. */
    async function ungroupRoom(r: Room) {
        await rooms.ungroup(r);
        announce(`${r.name} split into separate rooms.`);
        closePlayer();
    }

    // ── The docked mini-player ───────────────────────────────────────────
    // A fallback, never a duplicate: it carries the same track and transport
    // as the hero, so it stands down while the hero is on screen and appears
    // the moment it scrolls away — which is always, on every screen that
    // isn't Home.
    //
    // It **survives a pause**: the hero follows the focus, so the dock is
    // where whatever was last playing stays one tap from playing again.
    let lastLiveKey = $state<string | null>(null);
    $effect(() => {
        const r = rooms.playing[0];
        if (r) lastLiveKey = r.key;
    });
    const dock = $derived.by<Room | undefined>(() => {
        if (rooms.playing[0]) return rooms.playing[0];
        const held = rooms.byKey(lastLiveKey);
        if (held && rooms.hasTrack(held)) return held;
        return rooms.withTrack[0];
    });

    let heroOnScreen = $state(false);
    // The editor is the one sheet the dock stands down for as firmly as the
    // player does: it is a form with a sticky footer, and a floating transport
    // over it would land on top of Save.
    const showDock = $derived(!!dock && !heroOnScreen && !playerOpen && !editorOpen);

    // The dock runs the full width of the band the assistant FAB floats in.
    // While it is up the FAB stands down, so the transport gets the whole bar.
    $effect(() => {
        if (!showDock) return;
        return bottomBar.claim();
    });

    // ── Grouping ─────────────────────────────────────────────────────────
    // The gesture is its own module: pointer, hold, edge-scroll, keyboard and
    // live region. What a drop *means* is the room model's — natively where
    // Sonos can, as a HomeHub room where it can't.
    const drag = createRoomDrag({
        scroller: () => (scrollEl ?? document.scrollingElement) as HTMLElement | null,
        roomOf: (key) => rooms.byKey(key),
        canDrop: (a, b) => rooms.canGroup(a, b),
        describe: (r) => ({ playing: rooms.isPlaying(r), sub: rooms.nowLine(r) }),
        group: (source, target) => void groupRooms(source, target),
        announce: (msg) => announce(msg),
    });

    async function groupRooms(source: Room, target: Room) {
        const said = await rooms.group(source, target);
        if (!said) return;
        // `announce` is the screen-reader live region, not a toast — the
        // grouping rule the drop picked is genuinely new information, and
        // the cards visibly merge for everyone who can see them.
        announce(said);
    }

    // A room held for grouping that drops off the network can't be dropped
    // anywhere — let go of it rather than leaving a card lifted over nothing.
    $effect(() => {
        drag.prune(new Set(rooms.list.map((r) => r.key)));
    });

    /**
     * What a screen reader is told about a gesture it can't see. Drag has no
     * visible running commentary; the keyboard path needs one.
     */
    let liveMsg = $state("");
    let announceTimer: ReturnType<typeof setTimeout> | undefined;
    function announce(msg: string) {
        clearTimeout(announceTimer);
        liveMsg = msg;
        announceTimer = setTimeout(() => (liveMsg = ""), 4000);
    }

    // ── Room membership ──────────────────────────────────────────────────
    // A form, so it takes the sheet shape — and it reaches it by *swapping*
    // with whatever raised it, which puts that sheet back on the way out.
    /** The room being edited, or null while creating one. */
    let editingZone = $state<MediaZone | null>(null);

    function openRoomEditor(z: MediaZone | null) {
        editingZone = z;
        // A swap is not a close: the player hands over to the editor in place,
        // so it lets go of the frame it grew out of and leaves the way every
        // other swapping sheet does rather than collapsing into the dock while
        // the editor rises through it.
        playerOrigin = null;
        rememberSheetScroll();
        sheets = sheetRun.swapTo(sheets, "room-edit");
        sheetScroll["room-edit"] = 0;
        sheetDismissing = false;
        // Registering or removing a speaker is the only thing that changes the
        // picker's list, and it can have happened since the view mounted.
        void zones.loadEndpoints();
    }

    function zoneSaved(_z: MediaZone) {
        editingZone = null;
        dropSheet();
    }

    async function deleteZone(z: MediaZone) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: `Delete ${z.name}?`,
            message:
                "The room goes; the speakers stay exactly as they are. Anything playing on it keeps playing until you stop it.",
            confirmLabel: "Delete room",
            danger: true,
        });
        if (!ok) return;
        if (!(await zones.remove(z.id))) return;
        hideSheet();
    }

    /** Clearing stops playback, so it gets the same confirm any destructive
     *  action does — which is why it lives here and not on the bridge. */
    async function clearQueue(r: Room) {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Clear the queue?",
            message: `Every track queued on ${r.name} will be removed, and playback stops.`,
            confirmLabel: "Clear queue",
            danger: true,
        });
        if (!ok) return;
        await sonos.clearQueue(r.id);
    }

    // ── Catalog drill-ins ────────────────────────────────────────────────
    // Screens, not sheets: all are reached from a screen's worth of
    // navigation, and a sheet must never open another one.
    //
    // An artist or an album changes slowly and the stack wanders back and
    // forth (artist → album → back), so each detail is fetched once per URI
    // and kept for the session — coming back is instant rather than a
    // skeleton replaying itself.
    let artistCache = $state<Record<string, SpotifyArtistDetail>>({});
    let artistLoadingUri = $state<string | null>(null);
    const artistUri = $derived(topEntry?.id === "artist" ? topEntry.uri! : null);
    const artistDetail = $derived(artistUri ? (artistCache[artistUri] ?? null) : null);
    const artistLoading = $derived(!!artistUri && artistLoadingUri === artistUri);

    async function openArtist(uri: string) {
        if (topEntry?.id === "artist" && topEntry.uri === uri) return;
        pushScreen({ id: "artist", uri, scroll: 0 });
        if (artistCache[uri]) return; // been here — renders instantly
        artistLoadingUri = uri;
        try {
            artistCache[uri] = await api.spotifyArtist(uri);
        } catch (e) {
            toasts.error("Couldn't load artist", (e as Error).message);
            if (artistUri === uri) leaveScreen();
        } finally {
            if (artistLoadingUri === uri) artistLoadingUri = null;
        }
    }

    let contextCache = $state<Record<string, SpotifyContextDetail>>({});
    let contextLoadingUri = $state<string | null>(null);
    const contextUri = $derived(topEntry?.id === "context" ? topEntry.uri! : null);
    const contextDetail = $derived(contextUri ? (contextCache[contextUri] ?? null) : null);
    const contextLoading = $derived(!!contextUri && contextLoadingUri === contextUri);

    /** An album or a playlist tapped anywhere — search, an artist's
     *  discography — opens its own page rather than playing blind: the track
     *  listing is what a tap on a container is actually asking for. */
    async function openContext(uri: string) {
        if (topEntry?.id === "context" && topEntry.uri === uri) return;
        pushScreen({ id: "context", uri, scroll: 0 });
        if (contextCache[uri]) return;
        contextLoadingUri = uri;
        try {
            contextCache[uri] = await api.spotifyContext(uri);
        } catch (e) {
            toasts.error("Couldn't open it", (e as Error).message);
            if (contextUri === uri) leaveScreen();
        } finally {
            if (contextLoadingUri === uri) contextLoadingUri = null;
        }
    }

    let browseFavorite = $state<SonosFavorite | null>(null);
    let favoriteContext = $state<SpotifyContextDetail | null>(null);
    let favoriteLoading = $state(false);

    /** Tapping a favorite that is a Spotify playlist or album opens its tracks
     *  instead of playing outright — the corner mark on the card said so. */
    async function openFavoriteBrowse(f: SonosFavorite) {
        if (!f.spotify_uri) return;
        pushScreen({ id: "favorite", scroll: 0 });
        browseFavorite = f;
        favoriteContext = null;
        favoriteLoading = true;
        const uri = f.spotify_uri;
        try {
            const det = await api.spotifyContext(uri);
            if (browseFavorite?.spotify_uri !== uri) return; // superseded
            favoriteContext = det;
        } catch {
            if (browseFavorite?.spotify_uri !== uri) return;
            // Spotify's own error text means nothing to someone tapping a
            // favorite — some of its own algorithmic playlists 404 on this
            // lookup even though Sonos plays them fine. Point at what works.
            toasts.error("Can't preview this playlist", "Try playing it instead.");
            leaveScreen();
        } finally {
            if (browseFavorite?.spotify_uri === uri) favoriteLoading = false;
        }
    }

    // ── Keyboard ─────────────────────────────────────────────────────────
    // The player covers the whole screen, so while it is open it answers the
    // transport keys a music app is expected to answer to. Everything else
    // stays scoped: only Escape and "/" work from the view at large.
    function editableTarget(e: KeyboardEvent): HTMLElement | null {
        return ((e.target as HTMLElement | null)?.closest?.(
            "input, textarea, select, [contenteditable='true']",
        ) ?? null) as HTMLElement | null;
    }

    function onWindowKey(e: KeyboardEvent) {
        const field = editableTarget(e);
        // A range input (scrubber, volume) owns the arrow keys while the caret
        // is on it — we only borrow the ones it ignores.
        const slider = field instanceof HTMLInputElement && field.type === "range";
        const onControl = !!(e.target as HTMLElement | null)?.closest?.("button, a");
        const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

        if (key === "Escape") {
            if (closeCatalogMenu()) return;
            if (drag.drag || drag.grabKey) {
                // Put a held room back before leaving anything.
                const name = drag.grabbedName || drag.drag?.name || "Room";
                drag.release();
                announce(`${name} put back.`);
            } else if (openSheet) dropSheet();
            // Escape backs out of a speaker's settings the same way its back
            // chip does — a drill-down owes the user the key that leaves it.
            else if (speakersScreen?.closeDetail()) return;
            else if (screen !== "home") leaveScreen();
            return;
        }
        if (e.metaKey || e.ctrlKey || e.altKey) return;

        if (!playerOpen) {
            // "/" is the one shortcut that works from anywhere in the view.
            if (key === "/" && !field) {
                e.preventDefault();
                openBrowse();
            }
            return;
        }

        if (field && !slider) return; // typing, not controlling

        // "/" keeps its meaning inside the player — it just browses *for this
        // room*, and hands the sheet over rather than stacking one.
        if (key === "/") {
            e.preventDefault();
            openBrowse();
            return;
        }

        player?.handleKey(e, { slider, onControl });
    }

    // ── Scrubbing ────────────────────────────────────────────────────────
    // Positions are only polled every few seconds, so between polls every
    // surface showing progress extrapolates from the last reading;
    // `clock.beat` re-runs those derivations once a second. Held only while
    // something is actually moving.
    $effect(() => {
        if (!playerOpen && rooms.playing.length === 0) return;
        return clock.start();
    });

    /** The open sheet's scroll container, for scroll restore and edge-scroll. */
    let scrollEl = $state<HTMLElement | null>(null);
    let playerEl = $state<HTMLElement | null>(null);
    let player = $state<Player | null>(null);
    let searchScreen = $state<SearchScreen | null>(null);
    let artistScreen = $state<ArtistScreen | null>(null);
    let contextScreen = $state<ContextScreen | null>(null);
    let favoriteScreen = $state<FavoriteBrowseScreen | null>(null);
    let speakersScreen = $state<SpeakersScreen | null>(null);

    /** Every catalog screen carries row overflow menus, and Escape has to
     *  close one of those before it starts leaving screens. Only the top of
     *  the stack is mounted, so at most one of these answers. */
    function closeCatalogMenu(): boolean {
        return !!(
            searchScreen?.closeMenu() ||
            artistScreen?.closeMenu() ||
            contextScreen?.closeMenu() ||
            favoriteScreen?.closeMenu()
        );
    }
    // Set by the open sheet while a drag-down rides out. The art swipe stands
    // down for those 220ms; raising a sheet clears it.
    let sheetDismissing = $state(false);

    // ── Spotify ──────────────────────────────────────────────────────────
    // Recent searches are keyed by the focused room, so the kitchen's aren't
    // the bedroom's. A single-room home only ever has one key.
    const recents = createSearchHistory(() => destination.key);
    // What the rooms have actually been heard playing. Unlike the searches
    // above it lives on the hub, not in this browser: it is written from what
    // the speakers report, so it knows about music nobody started from here.
    const heard = createHeardLog();
    const spotify = createSpotify((q, art) => recents.add(q, art));

    onMount(() => {
        // The OAuth callback bounces back to /#/music?spotify=… — surface the
        // outcome once, then clean the query off the URL.
        const q = route.query;
        if (q.spotify === "connected") {
            route.go("music");
        } else if (q.spotify_error) {
            toasts.error("Spotify login failed", q.spotify_error);
            route.go("music");
        }
        // The round trip ends here — land back on the screen the user left.
        if (q.spotify || q.spotify_error) openBrowse();
        void spotify.load();
    });

    // ── Devices ──────────────────────────────────────────────────────────
    // One sheet for every bridge — it carries the brand picker when adding and
    // is locked to the owning bridge when editing.
    async function openSpeakerModal(sp?: SonosSpeakerView) {
        const changed = await openModal<boolean>(
            SpeakerModal,
            sp ? { existing: sp, brand: "sonos" as const } : {},
        );
        if (changed) {
            void sonos.refresh();
            void kef.refresh();
            // The add sheet carries a brand picker, so a registration made
            // from it could have been any of the four.
            void airplay.refresh();
            void upnp.refresh();
            // A new or removed speaker changes both what rooms hold (the
            // backend cascades a delete out of them) and what the picker can
            // offer, so both reads are due.
            void zones.refresh();
            void zones.loadEndpoints();
        }
    }

    /** A renderer's sheet. Same shape as the AirPlay one and for the same
     *  reason: adding or removing one changes which routes every zone can
     *  take, so the zone reads and the endpoint list both have to follow. */
    async function openUPnPModal(rn: UPnPRenderer) {
        const changed = await openModal<boolean>(SpeakerModal, {
            existing: rn,
            brand: "upnp" as const,
        });
        if (changed) {
            void upnp.refresh();
            void zones.refresh();
            void zones.loadEndpoints();
        }
    }

    async function openAirPlayModal(sp: AirPlaySpeakerView) {
        const changed = await openModal<boolean>(SpeakerModal, {
            existing: sp,
            brand: "airplay" as const,
        });
        if (changed) {
            void airplay.refresh();
            void upnp.refresh();
            void zones.refresh();
            void zones.loadEndpoints();
        }
    }

    async function openKEFModal(sp: KEFSpeakerView) {
        const changed = await openModal<boolean>(SpeakerModal, {
            existing: sp,
            brand: "kef" as const,
        });
        if (changed) {
            if (kefDetailId === sp.id) kefDetailId = null;
            void kef.refresh();
            void zones.refresh();
            void zones.loadEndpoints();
        }
    }

    /** What the audio actually is on each path, and the decode setting. Read
     *  fresh by the sheet itself: the answer depends on the route a zone would
     *  take, which changes with what is registered. */
    async function openQualityModal() {
        await openModal(MusicQualityModal, {});
        // A changed decode quality changes what every zone read reports.
        void zones.refresh();
    }

    /** The Connect picker. Reading the zones after it closes: a transfer made
     *  in there can take the account's session away from a room HomeHub was
     *  feeding, and the backend will have released that zone. */
    async function openConnectModal() {
        await openModal(SpotifyConnectModal, {});
        void zones.refresh();
    }

    /** Qobuz setup. Signing in changes what every zone read reports about
     *  quality — it is the one provider that can answer "lossless" — so the
     *  zones are re-read the same way the quality sheet re-reads them. */
    async function openQobuzModal() {
        await openModal(QobuzConnectModal, {});
        void zones.refresh();
    }

    /** The push-status sheet. Retrying inside it can turn subscriptions on, and
     *  that changes which poll interval this view should be using. */
    async function openEventsModal() {
        await openModal(SonosEventsModal, {});
        void sonos.refresh();
    }

    // Which speaker's settings the Speakers screen has open. Held here because
    // the player's configure action pushes the screen *and* opens a pane in one
    // gesture — it has to name the pane before the screen exists.
    let detailId = $state<string | null>(null);
    let kefDetailId = $state<string | null>(null);

    function openKEFSpeaker(sp: KEFSpeakerView) {
        detailId = null; // one pane at a time
        kefDetailId = sp.id;
    }
</script>

<svelte:window onkeydown={onWindowKey} onpopstate={onPopState} />

<!-- Anything a grouping gesture does that has no visible running commentary
     — the keyboard path especially — is said here instead. -->
<div class="sr-only" role="status" aria-live="polite">{liveMsg}</div>

{#if screen === "home"}
    <Topbar
        title="Music"
        subtitle={sonos.loaded
            ? `${totalSpeakers} speaker${totalSpeakers === 1 ? "" : "s"} · ${rooms.playing.length} playing`
            : "Sonos & KEF"}
    >
        {#snippet actions()}
            <!-- Whether speaker state is being pushed or polled. It rides in
                 the topbar because it qualifies everything below it — how
                 quickly any of this reflects reality. Only where there are
                 Sonos speakers for it to describe: KEF has no notifications to
                 subscribe to, and a chip saying "Polling" about them would be
                 reporting a fault that doesn't exist. -->
            {#if sonos.loaded && (sonos.status?.speakers.length ?? 0) > 0}
                <LiveStatusChip live={sonos.livePush} onClosed={() => void sonos.refresh()} />
            {/if}
            {#if sonos.loaded && totalSpeakers > 0}
                <button class="chip act-browse" onclick={() => openBrowse()}>
                    <Icon name="search" size={14} />
                    <span class="act-label">Browse</span>
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
        fill
        icon="speaker"
        title="No speakers yet"
        message="Add your Sonos or KEF speakers to control playback, volume and grouping right here, with neither app needed."
    >
        <button class="btn btn-primary" onclick={() => openSpeakerModal()}>Add speaker</button>
    </EmptyState>
{/if}

{#if sonos.loaded && totalSpeakers > 0}
    {#if screen === "home"}
        <MusicHome
            {rooms}
            {sonos}
            {destination}
            {drag}
            {totalSpeakers}
            {readyCount}
            dockKey={dock?.key}
            onDockVisible={(v) => (heroOnScreen = v)}
            onOpenPlayer={openPlayer}
            onBrowse={() => openBrowse()}
            onOpenSpeakers={openSpeakers}
            onNewRoom={() => openRoomEditor(null)}
        />
    {:else if screen === "speakers"}
        <SpeakersScreen
            {sonos}
            {kef}
            {airplay}
            {totalSpeakers}
            {readyCount}
            onBack={leaveScreen}
            onAdd={() => openSpeakerModal()}
            onEditSonos={(sp) => void openSpeakerModal(sp)}
            onEditKEF={(sp) => void openKEFModal(sp)}
            onEditAirPlay={(sp) => void openAirPlayModal(sp)}
            onEditUPnP={(rn) => void openUPnPModal(rn)}
            {upnp}
            onOpenEvents={openEventsModal}
            onOpenQuality={openQualityModal}
            onOpenConnect={openConnectModal}
            onOpenQobuz={openQobuzModal}
            spotifyPlayback={spotify.status?.playback ?? false}
            onKEFOpened={(sp) => {
                const r = rooms.byKey("kef:" + sp.id);
                if (r) destination.focus(r);
            }}
            bind:this={speakersScreen}
            bind:detailId
            bind:kefDetailId
        />
    {:else if screen === "artist"}
        <ArtistScreen
            artist={artistDetail}
            loading={artistLoading}
            {destination}
            {busy}
            {targetRow}
            onBack={leaveScreen}
            onPick={playItem}
            onEnqueue={(item, next) =>
                enqueue({ service: "Spotify", uri: item.uri, title: item.name }, next)}
            onOpenArtist={openArtist}
            onOpenContext={openContext}
            bind:this={artistScreen}
        />
    {:else if screen === "context"}
        <ContextScreen
            context={contextDetail}
            loading={contextLoading}
            {destination}
            {busy}
            {targetRow}
            onBack={leaveScreen}
            onPlayAll={() =>
                contextDetail &&
                playItem({
                    kind: contextDetail.kind,
                    uri: contextDetail.uri,
                    name: contextDetail.name,
                })}
            onPick={playItem}
            onEnqueue={(item, next) =>
                enqueue({ service: "Spotify", uri: item.uri, title: item.name }, next)}
            onOpenArtist={openArtist}
            bind:this={contextScreen}
        />
    {:else if screen === "favorite" && browseFavorite}
        <FavoriteBrowseScreen
            favorite={browseFavorite}
            context={favoriteContext}
            loading={favoriteLoading}
            {destination}
            {busy}
            {targetRow}
            onBack={leaveScreen}
            onPlayAll={() => playFavorite(browseFavorite!)}
            playAllBusy={busy.is("fav:" + browseFavorite.id)}
            onPick={playItem}
            onEnqueue={(item, next) =>
                enqueue({ service: "Spotify", uri: item.uri, title: item.name }, next)}
            onOpenArtist={openArtist}
            bind:this={favoriteScreen}
        />
    {:else if screen === "browse"}
        <SearchScreen
            {spotify}
            {recents}
            {destination}
            {busy}
            autofocus={searchWantsFocus}
            onBack={leaveScreen}
            onPlayItem={playItem}
            onEnqueue={(item, next) =>
                enqueue({ service: "Spotify", uri: item.uri, title: item.name }, next)}
            onOpenArtist={openArtist}
            onOpenContext={openContext}
            {targetRow}
            favorites={favShelf}
            bind:this={searchScreen}
        />
    {/if}

    <!-- ── The dock ─────────────────────────────────────────────────
         Present everywhere the hero isn't: Browse, Speakers, the catalog
         screens, and Home once the hero has scrolled away. -->
    {#if showDock && dock}
        {@const d = dock}
        <MiniPlayer
            title={rooms.title(d) || rooms.nowLine(d)}
            sub={[rooms.subLine(d), d.name].filter(Boolean).join(" · ")}
            artUri={rooms.art(d)}
            playing={rooms.isPlaying(d)}
            progress={rooms.progress(d)}
            seek={d.canSeek && rooms.durationSec(d) > 0
                ? {
                      position: rooms.livePosition(d),
                      duration: rooms.durationSec(d),
                      onSeek: (sec) => rooms.seek(d, sec),
                  }
                : undefined}
            volume={{
                value: rooms.volume(d),
                muted: rooms.muted(d),
                onInput: (v) => rooms.dragVolume(d, v),
                onChange: (v) => rooms.setVolume(d, v),
                onToggleMute: () => rooms.toggleMute(d),
            }}
            onOpen={(from) => openPlayer(d, from)}
        >
            {#snippet transport()}
                <CardTransport
                    playing={rooms.isPlaying(d)}
                    onToggle={() => rooms.togglePlay(d)}
                    toggleBusy={rooms.playBusy(d)}
                    onPrev={d.canSkip ? () => rooms.skip(d, "previous") : undefined}
                    prevBusy={rooms.prevBusy(d)}
                    onNext={d.canSkip ? () => rooms.skip(d, "next") : undefined}
                    nextBusy={rooms.nextBusy(d)}
                />
            {/snippet}
        </MiniPlayer>
    {/if}
{/if}

<!-- Every surface that can start something names where it will come out, so
     the picker is rendered through one snippet rather than placed by hand. -->
{#snippet targetRow()}
    <RoomPicker {destination} {rooms} />
{/snippet}

<!-- Tap the art to play it on `target`, the corner + to queue it. -->
{#snippet favTile(f: SonosFavorite, target: string | null)}
    <FavoriteCard
        favorite={f}
        {target}
        playBusy={busy.is("fav:" + f.id)}
        queueBusy={busy.is("q:" + f.uri)}
        onPlay={() => (f.spotify_uri ? openFavoriteBrowse(f) : playFavorite(f, target))}
        onQueue={() => enqueue({ uri: f.uri, title: f.title, metadata: f.metadata }, false, target)}
    />
{/snippet}

<!-- The favorites shelf, as Browse shows it. Favorites are a Sonos household
     list and only a Sonos room can be handed one, so the section says what it
     needs rather than offering a rail of dead cards. -->
{#snippet favShelf()}
    {#if sonos.favorites.length > 0}
        <section class="block">
            <div class="eyrow">Favorites</div>
            {#if destination.sonosTarget}
                <div class="favs h-scroll">
                    {#each sonos.favorites as f (f.id)}
                        {@render favTile(f, destination.sonosTarget)}
                    {/each}
                </div>
            {:else}
                <p class="hint">
                    Favorites come out of your Sonos household, so {destination.label ||
                        "this room"} can't play one — pick a Sonos room above, or search below.
                </p>
            {/if}
        </section>
    {/if}
{/snippet}

<!-- The player-side "Start something" row. -->
{#snippet startRow(favTarget: string | null)}
    <StartSomething
        spotifyAvailable={!!spotify.status}
        spotifyConnected={spotify.connected}
        recents={recents.recent}
        favorites={favTarget ? sonos.favorites : []}
        onSearch={openBrowse}
    >
        {#snippet favCard(f)}
            {@render favTile(f, favTarget)}
        {/snippet}
    </StartSomething>
{/snippet}

<!-- The travelling ghost lives out here, not inside a scroll container or a
     sheet: `.sheet` takes a transform while it is dragged, and a
     `position: fixed` descendant would be anchored to that. -->
{#if drag.drag}
    <RoomCard ghost={drag.drag} />
{/if}

{#if playerOpen && playerRoom}
    {@const r = playerRoom}
    <Player
        room={r}
        {rooms}
        {sonos}
        {kef}
        {busy}
        {heard}
        onPlayHeard={playHeard}
        onClearHeard={() => void clearHeard(r)}
        onClose={closePlayer}
        onConfigure={() => configureRoom(r)}
        onUngroup={r.group && r.grouped ? () => void ungroupRoom(r) : undefined}
        onClearQueue={() => clearQueue(r)}
        onStop={() => r.zone && void zones.stop(r.zone)}
        startSomething={startRow}
        origin={playerOrigin}
        bind:this={player}
        bind:scrollEl
        bind:sheetEl={playerEl}
        bind:dismissing={sheetDismissing}
    />
{/if}

<!-- A form, so it takes the sheet shape — and it *swaps* with the sheet that
     raised it, which is what keeps "edit a room from its own player" from
     being a sheet over a sheet. -->
{#if editorOpen}
    <ZoneEditor
        zone={editingZone}
        {zones}
        onCancel={dropSheet}
        onSaved={zoneSaved}
        onDelete={(z) => void deleteZone(z)}
        onOpenSpeakers={openSpeakers}
        bind:scrollEl
        bind:dismissing={sheetDismissing}
    />
{/if}

<style>
    .sk {
        height: 220px;
        border-radius: var(--r-lg);
    }

    /* Announced, never drawn — the running commentary on a grouping gesture
       that has no visible one. */
    .sr-only {
        position: absolute;
        width: 1px;
        height: 1px;
        margin: -1px;
        padding: 0;
        overflow: hidden;
        clip-path: inset(50%);
        white-space: nowrap;
        border: 0;
    }

    .favs {
        display: flex;
        gap: var(--space-3);
        padding-bottom: var(--space-1);
    }

    /* Browse keeps its label wherever the header has room for it, and drops to
       the icon alone on a phone — where a third labelled chip is exactly what
       crushed the subtitle to a two-word stub. */
    .act-browse {
        flex-shrink: 0;
    }
    @media (max-width: 620px) {
        .act-browse {
            position: relative;
            width: 38px;
            height: 38px;
            padding: 0;
            justify-content: center;
            border-radius: 50%;
        }
        .act-label {
            position: absolute;
            width: 1px;
            height: 1px;
            margin: -1px;
            padding: 0;
            overflow: hidden;
            clip-path: inset(50%);
            white-space: nowrap;
        }
    }
    @media (max-width: 620px) and (pointer: coarse) {
        .act-browse {
            width: 44px;
            height: 44px;
        }
    }
</style>
