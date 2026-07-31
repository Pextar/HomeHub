<script lang="ts">
    /**
     * The panel's music depth (DESIGN.md §16): the whole music player, on
     * the kiosk — no chrome, no sheets, no app shell. The featured room's
     * player rides on the right; the left is the work area, switched by
     * chip between three panes:
     *
     *   Search  Spotify's catalog, with the room's recent searches and the
     *           account's playlists while the box is empty. A song plays
     *           outright; an artist opens their page — the app's own
     *           catalog screens (§15.9), one level deeper in this same
     *           column, with a record or a related artist going deeper
     *           still and back climbing one level. Queueing without
     *           interrupting lives behind the row's overflow, and only
     *           for a Sonos destination — the queue is a Sonos group's.
     *   Queue   The featured group's queue: tap to jump, X to remove,
     *           two taps to clear (there is no confirm modal on a kiosk).
     *   Rooms   Every room, with Sonos-native grouping: join the featured
     *           room, split one apart, or step a single speaker out.
     *           Cross-vendor HomeHub rooms stay in the full Music view —
     *           making a persistent routed room is configuration.
     *
     * The back chip returns to the panel dashboard — or climbs one level
     * of the catalog stack first; Escape does the same, unless a row menu
     * has it first.
     */
    import { onMount, tick as flushDOM } from "svelte";
    import { fade, scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import EmptyState from "../EmptyState.svelte";
    import QueuePane from "../music/QueuePane.svelte";
    import MediaCard from "../music/MediaCard.svelte";
    import ArtistScreen from "../music/ArtistScreen.svelte";
    import ContextScreen from "../music/ContextScreen.svelte";
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import { api } from "../../lib/api";
    import { route, toasts } from "../../lib/stores.svelte";
    import { createSpotify } from "../../lib/music/spotify.svelte";
    import { createSearchHistory } from "../../lib/music/history.svelte";
    import { fmtCount, fmtMs, capFirst } from "../../lib/music/format";
    import { dur } from "../../lib/motion";
    import { kefSourceLabel } from "../../lib/kef";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyArtistDetail, SpotifyContextDetail, SpotifyItem } from "../../lib/types";

    let { music }: { music: PanelMusicStore } = $props();

    // Recent searches, keyed by the featured room with the same key format
    // the app uses — a search run on the wall lands in the same per-room
    // history as one run from a phone, and follows the room chips.
    const recents = createSearchHistory(() => {
        const f = music.featured;
        return f ? `${f.kind}:${f.id}` : null;
    });
    const spotify = createSpotify((q, art) => recents.add(q, art));
    // `status` is null both while loading and when the endpoint refuses
    // (the Spotify routes are admin-only); `booted` separates the two so a
    // refusal doesn't hang on skeletons.
    let booted = $state(false);
    onMount(() => {
        void spotify.load().finally(() => {
            booted = true;
        });
    });

    const featured = $derived(music.featured);

    // ── Panes ────────────────────────────────────────────────────────────
    type Pane = "search" | "queue" | "rooms";
    let pane = $state<Pane>("search");
    const queueCount = $derived(featured?.groupState?.queue_length ?? 0);

    function back() {
        route.go("panel");
    }

    // ── The catalog stack: artist and record pages, one level deeper ────
    // The app's own catalog screens ride in this column (§15.9's stack on
    // §16's wall): an artist opens from a search row, their records and
    // the artists their fans also play open from there, and back climbs
    // one level instead of leaving the depth. Each detail is fetched once
    // per URI and kept for the session — coming back is instant rather
    // than a skeleton replaying itself.
    type Level = { kind: "artist" | "context"; uri: string; scroll: number };
    let stack = $state<Level[]>([]);
    const topLevel = $derived(stack.length ? stack[stack.length - 1] : null);

    let artistCache = $state<Record<string, SpotifyArtistDetail>>({});
    let artistLoadingUri = $state<string | null>(null);
    const artistUri = $derived(topLevel?.kind === "artist" ? topLevel.uri : null);
    const artistDetail = $derived(artistUri ? (artistCache[artistUri] ?? null) : null);
    const artistLoading = $derived(!!artistUri && artistLoadingUri === artistUri);

    let contextCache = $state<Record<string, SpotifyContextDetail>>({});
    let contextLoadingUri = $state<string | null>(null);
    const contextUri = $derived(topLevel?.kind === "context" ? topLevel.uri : null);
    const contextDetail = $derived(contextUri ? (contextCache[contextUri] ?? null) : null);
    const contextLoading = $derived(!!contextUri && contextLoadingUri === contextUri);

    // Scroll follows the level: pushing stashes where the outgoing list
    // was, popping puts it back — the search results count as level zero.
    let resultsEl = $state<HTMLElement | null>(null);
    let stackEl = $state<HTMLElement | null>(null);
    let searchScroll = 0;

    function pushLevel(kind: Level["kind"], uri: string) {
        if (stack.length === 0) searchScroll = resultsEl?.scrollTop ?? 0;
        else stack[stack.length - 1].scroll = stackEl?.scrollTop ?? 0;
        stack = [...stack, { kind, uri, scroll: 0 }];
    }

    async function popLevel() {
        if (!stack.length) return;
        stack = stack.slice(0, -1);
        await flushDOM();
        if (stack.length === 0) resultsEl?.scrollTo(0, searchScroll);
        else stackEl?.scrollTo(0, stack[stack.length - 1].scroll);
    }

    async function openArtist(uri: string, art?: { art_url?: string; round?: boolean }) {
        if (topLevel?.kind === "artist" && topLevel.uri === uri) return;
        actedOnResult(art);
        searchEl?.blur(); // chosen — the keyboard's job is done
        pushLevel("artist", uri);
        if (artistCache[uri]) return; // been here — renders instantly
        artistLoadingUri = uri;
        try {
            artistCache[uri] = await api.spotifyArtist(uri);
        } catch (e) {
            toasts.error("Couldn't load artist", (e as Error).message);
            if (artistUri === uri) void popLevel();
        } finally {
            if (artistLoadingUri === uri) artistLoadingUri = null;
        }
    }

    async function openContext(uri: string) {
        if (topLevel?.kind === "context" && topLevel.uri === uri) return;
        pushLevel("context", uri);
        if (contextCache[uri]) return;
        contextLoadingUri = uri;
        try {
            contextCache[uri] = await api.spotifyContext(uri);
        } catch (e) {
            toasts.error("Couldn't open it", (e as Error).message);
            if (contextUri === uri) void popLevel();
        } finally {
            if (contextLoadingUri === uri) contextLoadingUri = null;
        }
    }

    // The catalog screens were built for the app's stores; these two answer
    // their props from the panel's own instead — the featured source is the
    // destination, and `busy` reads the same map the rows disable on.
    const catalogDest = {
        get current() {
            const f = featured;
            return f ? { kind: f.kind, id: f.id } : null;
        },
        get sonosTarget() {
            const f = featured;
            return f?.kind === "sonos" ? f.id : null;
        },
    };
    const catalogBusy: Busy = {
        is: (k) => !!music.busy[k],
        async claim(k, fn) {
            if (music.busy[k]) return undefined;
            music.busy[k] = true;
            try {
                return await fn();
            } finally {
                music.busy[k] = false;
            }
        },
        async run(k, fn, errTitle, after) {
            await catalogBusy.claim(k, async () => {
                try {
                    await fn();
                    await after?.();
                } catch (e) {
                    toasts.error(errTitle, (e as Error).message);
                }
            });
        },
    };

    /** The opened record as an item, for the one-tap "Play album/playlist"
     *  — the same call a tap on its search row makes. */
    function contextItem(c: SpotifyContextDetail): SpotifyItem {
        return { kind: c.kind, uri: c.uri, name: c.name, sub: c.sub, art_url: c.art_url };
    }

    // The screens' row overflow menus answer Escape before the stack does.
    let artistScr = $state<ArtistScreen | null>(null);
    let contextScr = $state<ContextScreen | null>(null);

    // Escape leaves the depth — a row menu open at the time gets the key
    // first, then each level of the catalog stack, and the search box gets
    // it when there is a query to clear.
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (menuFor) {
                menuFor = null;
                return;
            }
            if (artistScr?.closeMenu() || contextScr?.closeMenu()) return;
            if (stack.length) {
                void popLevel();
                return;
            }
            back();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    });

    // ── The box behaves like a search box ────────────────────────────────
    let searchEl = $state<HTMLInputElement | null>(null);

    // The caret goes in the box only where a keyboard is already there —
    // auto-focus on the iPad would throw the software keyboard over the
    // results.
    $effect(() => {
        if (!window.matchMedia("(pointer: fine)").matches) return;
        void flushDOM().then(() => searchEl?.focus());
    });

    // ── Type mode: the box knows the software keyboard is up ─────────────
    // The iPad's docked keyboard takes ~350pt off the bottom of the depth,
    // which used to leave the results a one-row strip. While it is up the
    // depth re-floors to just above it (--kb) and the results go dense,
    // and the keyboard is dismissed the moment typing is over — Enter, or
    // a tap on a result — so the rich layout returns for the choosing.
    // visualViewport measures the real thing: docked, floating or split,
    // and it degrades to zero (no type mode) where there is no software
    // keyboard at all.
    let kb = $state(0);
    const kbOpen = $derived(kb > 150);
    onMount(() => {
        const vv = window.visualViewport;
        if (!vv) return;
        const measure = () => {
            kb = Math.max(0, Math.round(window.innerHeight - vv.height - vv.offsetTop));
        };
        vv.addEventListener("resize", measure);
        vv.addEventListener("scroll", measure);
        return () => {
            vv.removeEventListener("resize", measure);
            vv.removeEventListener("scroll", measure);
        };
    });

    // A fresh page of results while typing starts the dense list from the
    // top — the first rows are the ones being aimed at.
    $effect(() => {
        void spotify.results;
        if (kbOpen) resultsEl?.scrollTo(0, 0);
    });

    function onQueryKey(e: KeyboardEvent) {
        if (e.key === "Enter") {
            e.preventDefault();
            spotify.runNow();
            resultsEl?.scrollTo(0, 0);
            // Submitted — typing is over, so the keyboard leaves with it
            // and the results get the whole column back.
            searchEl?.blur();
        } else if (e.key === "Escape" && spotify.query) {
            e.stopPropagation();
            spotify.clearQuery();
            searchEl?.focus();
        } else if (e.key === "ArrowDown") {
            // Hand the caret to the first result (or the first recent) —
            // the way a search box should (§15.8).
            const first = resultsEl?.querySelector<HTMLButtonElement>(".r-open, .s-recent-run");
            if (first) {
                e.preventDefault();
                first.focus();
            }
        }
    }
    function clearQuery() {
        spotify.clearQuery();
        searchEl?.focus();
    }
    function runRecent(q: string) {
        spotify.runQuery(q);
        // The query was chosen by chip, so typing is over: the software
        // keyboard leaves if it was up (a no-op blur when it wasn't), and
        // the caret goes back in the box only where a keyboard is already
        // there — on the iPad a refocus throws the software keyboard over
        // the results the tap just fetched.
        if (window.matchMedia("(pointer: fine)").matches) searchEl?.focus();
        else searchEl?.blur();
    }

    // ── Results ──────────────────────────────────────────────────────────
    const KINDS = [
        { id: "tracks", label: "Songs" },
        { id: "albums", label: "Albums" },
        { id: "playlists", label: "Playlists" },
        { id: "artists", label: "Artists" },
    ] as const;

    // Songs lead — playing one is the commonest reason to search at all —
    // and only shelves that matched are rendered.
    const sections = $derived.by(() => {
        const r = spotify.results;
        if (!r) return [];
        const all = KINDS.map((k) => ({ ...k, items: r[k.id] as SpotifyItem[] }));
        if (spotify.kindFilter === "all") return all.filter((s) => s.items.length > 0);
        return all.filter((s) => s.id === spotify.kindFilter);
    });

    /** What a row says under its name — different per kind, because what
     *  makes each one worth choosing is different. */
    function sub(item: SpotifyItem): string {
        if (item.kind === "artist") {
            if (item.followers) return `${fmtCount(item.followers)} followers`;
            return item.genres?.[0] ? capFirst(item.genres[0]) : "";
        }
        if (item.kind === "album") {
            return [item.sub, item.year, item.total_tracks ? `${item.total_tracks} songs` : ""]
                .filter(Boolean)
                .join(" · ");
        }
        if (item.kind === "playlist") {
            return [item.sub, item.total_tracks ? `${item.total_tracks} songs` : ""]
                .filter(Boolean)
                .join(" · ");
        }
        return [item.sub, item.album].filter(Boolean).join(" · ");
    }

    const KIND_LABEL: Record<string, string> = {
        artist: "Artist",
        album: "Album",
        playlist: "Playlist",
        track: "Song",
    };

    /** The top result's own line — the kind first, then the one stat that
     *  identifies it fastest (§15.9). */
    function topLine(item: SpotifyItem): string {
        const bits = [KIND_LABEL[item.kind]];
        if (item.kind === "artist") {
            if (item.followers) bits.push(`${fmtCount(item.followers)} followers`);
        } else {
            if (item.sub) bits.push(item.sub);
            if (item.year) bits.push(item.year);
            if (item.album) bits.push(item.album);
            if (item.duration_ms) bits.push(fmtMs(item.duration_ms));
            if (item.total_tracks) bits.push(`${item.total_tracks} songs`);
        }
        return bits.filter(Boolean).join(" · ");
    }

    /** A search that led somewhere is worth remembering. The store's own
     *  remembering is §15.8's submission — Enter, or a chip re-run — but
     *  the wall's flow is type → tap the result, with no Enter in between,
     *  so acting on a result remembers the query behind it too — picture
     *  included, straight from the row that was tapped. */
    function actedOnResult(art?: { art_url?: string; round?: boolean }) {
        const q = spotify.query.trim();
        if (q) recents.add(q, art);
    }

    function pick(item: SpotifyItem) {
        actedOnResult(item.art_url ? { art_url: item.art_url, round: false } : undefined);
        searchEl?.blur(); // chosen — the keyboard's job is done
        const s = featured;
        if (!s) {
            toasts.error("Couldn't play", "No speaker is reachable right now.");
            return;
        }
        // A KEF starts through Spotify Connect, which this login may not
        // have the scopes for — said before the tap, not after a 403 (§15).
        if (s.kind === "kef" && spotify.status && !spotify.status.playback) {
            toasts.error(
                "Reconnect Spotify",
                "This login can't start playback — reconnect it in the Music view.",
            );
            return;
        }
        void music.playItem(item);
    }

    // ── Row overflow menus (queue actions, Sonos destinations only) ──────
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
    /** An open menu takes focus and answers the arrow keys, so queueing a
     *  result never means tabbing back through the whole list. */
    function menuNav(node: HTMLElement) {
        const items = () =>
            Array.from(node.querySelectorAll<HTMLButtonElement>("[role='menuitem']"));
        items()[0]?.focus();
        function onKey(e: KeyboardEvent) {
            if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
            e.preventDefault();
            const list = items();
            const i = list.findIndex((b) => b === document.activeElement);
            list[(i + (e.key === "ArrowDown" ? 1 : list.length - 1)) % list.length]?.focus();
        }
        node.addEventListener("keydown", onKey);
        return { destroy: () => node.removeEventListener("keydown", onKey) };
    }

    // ── Rooms: tap-based Sonos grouping ──────────────────────────────────
    // Split is the one destructive gesture here and there is no confirm
    // modal on a kiosk, so it arms for a few seconds instead.
    let splitArmed = $state(false);
    let splitTimer: ReturnType<typeof setTimeout> | undefined;
    function splitClick() {
        if (splitArmed) {
            splitArmed = false;
            clearTimeout(splitTimer);
            music.ungroupFeatured();
            return;
        }
        splitArmed = true;
        clearTimeout(splitTimer);
        splitTimer = setTimeout(() => (splitArmed = false), 3000);
    }

    function roomSub(s: PanelSource): string {
        if (s.kind === "kef") return ["KEF", kefSourceLabel(s.input)].filter(Boolean).join(" · ");
        return s.members && s.members.length > 1 ? `${s.members.length} speakers` : "Sonos";
    }
</script>

<div class="browse" class:kb-open={kbOpen} style:--kb="{kb}px" in:fade={{ duration: dur(160) }}>
    <header class="b-head">
        <button class="back" onclick={back} aria-label="Back to the panel">
            <Icon name="chevronLeft" size={16} /><span>Panel</span>
        </button>
        <h2>Music</h2>
    </header>

    <div class="b-body">
        <section class="b-left">
            {#if !topLevel}
                <div class="p-panes" role="group" aria-label="Music panes">
                    <button
                        class="k-chip"
                        class:active={pane === "search"}
                        onclick={() => (pane = "search")}
                    >
                        Search
                    </button>
                    <button
                        class="k-chip"
                        class:active={pane === "queue"}
                        onclick={() => (pane = "queue")}
                    >
                        Queue{#if featured?.kind === "sonos" && queueCount > 0}
                            <span class="mono">{queueCount}</span>{/if}
                    </button>
                    <button
                        class="k-chip"
                        class:active={pane === "rooms"}
                        onclick={() => (pane = "rooms")}
                    >
                        Rooms <span class="mono">{music.sources.length}</span>
                    </button>
                </div>
            {/if}

            {#if topLevel}
                <!-- One level deeper: the app's own artist/record pages ride
                     in this column; their back chip climbs the stack. -->
                {@const level = topLevel}
                <div class="b-stack" bind:this={stackEl}>
                    {#if level.kind === "artist"}
                        <ArtistScreen
                            artist={artistDetail}
                            loading={artistLoading}
                            destination={catalogDest}
                            busy={catalogBusy}
                            targetRow={playOnRow}
                            onBack={popLevel}
                            onPick={pick}
                            onEnqueue={(item, next) => music.enqueue(item, next)}
                            onOpenArtist={(uri) => void openArtist(uri)}
                            onOpenContext={(uri) => void openContext(uri)}
                            bind:this={artistScr}
                        />
                    {:else}
                        <ContextScreen
                            context={contextDetail}
                            loading={contextLoading}
                            destination={catalogDest}
                            busy={catalogBusy}
                            targetRow={playOnRow}
                            onBack={popLevel}
                            onPlayAll={() => contextDetail && pick(contextItem(contextDetail))}
                            onPick={pick}
                            onEnqueue={(item, next) => music.enqueue(item, next)}
                            onOpenArtist={(uri) => void openArtist(uri)}
                            bind:this={contextScr}
                        />
                    {/if}
                </div>
            {:else if pane === "search"}
                <div class="b-search">
                    <div class="s-box">
                        <Icon name="search" size={20} />
                        <input
                            bind:this={searchEl}
                            value={spotify.query}
                            placeholder="Search songs, artists, albums"
                            aria-label="Search music"
                            autocapitalize="off"
                            autocomplete="off"
                            spellcheck="false"
                            enterkeyhint="search"
                            oninput={(e) => {
                                spotify.query = e.currentTarget.value;
                                spotify.onQueryInput();
                            }}
                            onkeydown={onQueryKey}
                        />
                        {#if spotify.query}
                            <button class="s-clear" onclick={clearQuery} aria-label="Clear search">
                                <Icon name="close" size={16} />
                            </button>
                        {/if}
                    </div>

                    {#if spotify.results}
                        {@const r = spotify.results}
                        <div class="s-kinds">
                            <button
                                class="k-chip"
                                class:active={spotify.kindFilter === "all"}
                                onclick={() => (spotify.kindFilter = "all")}>All</button
                            >
                            {#each KINDS as k (k.id)}
                                {#if r[k.id].length > 0}
                                    <button
                                        class="k-chip"
                                        class:active={spotify.kindFilter === k.id}
                                        onclick={() => (spotify.kindFilter = k.id)}
                                        >{k.label}
                                        <span class="mono">{r[k.id].length}</span></button
                                    >
                                {/if}
                            {/each}
                        </div>
                    {/if}

                    <div class="s-results" bind:this={resultsEl}>
                        {#if !booted || spotify.searching}
                            <div class="sk-list" aria-hidden="true">
                                {#each Array(6) as _, i (i)}
                                    <div class="sk-row">
                                        <span class="skeleton sk-art"></span>
                                        <span class="sk-lines">
                                            <span class="skeleton sk-l1"></span>
                                            <span class="skeleton sk-l2"></span>
                                        </span>
                                    </div>
                                {/each}
                            </div>
                        {:else if !spotify.connected}
                            <div class="s-empty">
                                <EmptyState
                                    icon="musicNotes"
                                    title="Spotify isn't connected"
                                    message="Search and playback are set up in the Music view."
                                    compact
                                />
                            </div>
                        {:else if spotify.results}
                            {#if sections.length === 0}
                                <div class="s-empty">
                                    <EmptyState
                                        icon="search"
                                        title="Nothing found for “{spotify.query}”"
                                        message="Try another name, song or album."
                                        compact
                                    />
                                </div>
                            {:else}
                                {#if !kbOpen && spotify.kindFilter === "all" && spotify.topResult}
                                    <!-- The one thing this search was almost
                                         certainly after, at full size (§15.9).
                                         Type mode folds it back into the
                                         shelves: at one row tall the card
                                         would be the only thing visible. -->
                                    <h3 class="s-label">Top result</h3>
                                    {@render resultRow(spotify.topResult, true)}
                                {/if}
                                {#each sections as sec (sec.id)}
                                    <h3 class="s-label">{sec.label}</h3>
                                    {#each sec.items as item (item.uri)}
                                        {@render resultRow(item, false)}
                                    {/each}
                                {/each}
                            {/if}
                        {:else}
                            <!-- Idle shelves: the room's recent searches,
                                 then the account's own playlists as an art
                                 grid — §15.9's rule, since a playlist is
                                 chosen by its cover as much as its name. A
                                 tap still plays it whole on the featured
                                 room, like every container on the wall. -->
                            {#if recents.list.length > 0}
                                <div class="s-shelf-head">
                                    <h3 class="s-label">Recent searches</h3>
                                    <button class="k-chip" onclick={() => recents.clear()}>
                                        Clear
                                    </button>
                                </div>
                                <div class="s-recents">
                                    {#each recents.list as h (h.q)}
                                        <span class="s-recent">
                                            <button
                                                class="s-recent-run"
                                                onclick={() => runRecent(h.q)}
                                            >
                                                {#if h.art_url}
                                                    <img
                                                        class="s-recent-art"
                                                        class:round={h.round}
                                                        src={h.art_url}
                                                        alt=""
                                                    />
                                                {:else}
                                                    <Icon name="search" size={14} />
                                                {/if}
                                                <span>{h.q}</span>
                                            </button>
                                            <button
                                                class="s-recent-x"
                                                aria-label="Remove “{h.q}” from recent searches"
                                                onclick={() => recents.remove(h.q)}
                                            >
                                                <Icon name="close" size={13} />
                                            </button>
                                        </span>
                                    {/each}
                                </div>
                            {/if}
                            {#if spotify.myPlaylists.length > 0}
                                <h3 class="s-label">Your playlists</h3>
                                <div class="s-pl-grid">
                                    {#each spotify.myPlaylists as item (item.uri)}
                                        <MediaCard
                                            {item}
                                            sub={sub(item)}
                                            onOpen={() => pick(item)}
                                        />
                                    {/each}
                                </div>
                            {:else if recents.list.length === 0}
                                <div class="s-empty">
                                    <EmptyState
                                        icon="search"
                                        title="Search Spotify"
                                        message="Songs, albums, artists and playlists — played on {featured?.title ??
                                            'this room'}."
                                        compact
                                    />
                                </div>
                            {/if}
                        {/if}
                    </div>
                </div>
            {:else if pane === "queue"}
                <div class="b-pane">
                    {#if featured?.kind !== "sonos"}
                        <div class="s-empty">
                            <EmptyState
                                icon="queue"
                                title="Queues are Sonos-only"
                                message="{featured?.title ??
                                    'This room'} plays straight through — there is no queue to manage."
                                compact
                            />
                        </div>
                    {:else}
                        <h3 class="s-label">Queue — {featured.title}</h3>
                        <QueuePane
                            items={music.queue}
                            loading={music.queueLoading}
                            total={queueCount || music.queue.length}
                            currentTrack={featured.queueTrack}
                            playing={featured.playing}
                            confirmClear
                            clearBusy={!!music.busy["qclear:" + featured.id]}
                            isBusy={(k) => !!music.busy[k]}
                            onJump={(t) => music.jumpTo(t)}
                            onRemove={(t) => music.removeQueued(t)}
                            onClear={() => music.clearQueue()}
                        />
                    {/if}
                </div>
            {:else}
                <div class="b-pane">
                    <h3 class="s-label">Rooms</h3>
                    <div class="rm-list">
                        {#each music.sources as s (s.key)}
                            {@const isFeatured = featured?.key === s.key}
                            <div class="rm-row" class:active={isFeatured}>
                                <button
                                    class="rm-main"
                                    aria-label="Feature {s.title}"
                                    onclick={() => (music.selected = s.key)}
                                >
                                    <span class="rm-meta">
                                        <span class="rm-name">{s.title}</span>
                                        <span class="rm-sub">{roomSub(s)}</span>
                                    </span>
                                    {#if s.playing}
                                        <span class="rm-wave"><Waveform /></span>
                                    {/if}
                                </button>
                                {#if s.kind === "sonos" && featured?.kind === "sonos" && !isFeatured}
                                    <button
                                        class="k-chip rm-join"
                                        disabled={!!music.busy["join:" + s.id]}
                                        onclick={() => music.joinSource(s)}
                                    >
                                        Join {featured.title}
                                    </button>
                                {:else if isFeatured && s.kind === "sonos" && (s.members?.length ?? 0) > 1}
                                    <button
                                        class="k-chip"
                                        class:on={splitArmed}
                                        disabled={!!music.busy["ungroup:" + s.id]}
                                        onclick={splitClick}
                                    >
                                        {splitArmed ? "Split?" : "Split"}
                                    </button>
                                {/if}
                            </div>
                            {#if isFeatured && s.kind === "sonos" && (s.members?.length ?? 0) > 1}
                                <div class="rm-members">
                                    {#each s.members ?? [] as m (m.id)}
                                        <span class="rm-mchip">
                                            {m.name}{#if m.coordinator}
                                                <span class="rm-lead mono">lead</span>
                                            {:else}
                                                <button
                                                    class="rm-x"
                                                    aria-label="Remove {m.name} from the group"
                                                    disabled={!!music.busy["leave:" + m.id]}
                                                    onclick={() => music.leaveMember(m.id)}
                                                >
                                                    <Icon name="close" size={12} />
                                                </button>
                                            {/if}
                                        </span>
                                    {/each}
                                </div>
                            {/if}
                        {/each}
                    </div>
                    <p class="rm-note">
                        Sonos rooms group natively. Playing a KEF speaker together with Sonos takes
                        a HomeHub room — those are made in the Music view.
                    </p>
                </div>
            {/if}
        </section>

        <section class="b-player" aria-label="Now playing">
            {#if featured}
                <PanelPlayerCard
                    {music}
                    full
                    onShowQueue={() => {
                        stack = [];
                        pane = "queue";
                    }}
                />
            {:else}
                <div class="p-nosrc">
                    <Icon name="speaker" size={28} />
                    <p>No speakers reachable</p>
                </div>
            {/if}
        </section>
    </div>
</div>

<!-- The catalog screens name where they'll sound; on the wall that's
     always the featured source — its chips ride on the player column. -->
{#snippet playOnRow()}
    <span class="play-on">
        <Icon name="speaker" size={14} />
        <span>{featured ? `Plays on ${featured.title}` : "No speaker reachable"}</span>
    </span>
{/snippet}

<!-- The one row shape for everything the catalog returns (§14): a song or
     a container plays outright, an artist opens their page, and the
     trailing overflow queues without interrupting — for a Sonos
     destination only, the queue being a Sonos group's. `big` is the
     search's top result: the same row at full size, saying what it is
     and where the tap goes. -->
{#snippet resultRow(item: SpotifyItem, big: boolean)}
    <div class="row" class:big>
        <button
            class="r-open"
            disabled={item.kind !== "artist" && music.busy["item:" + item.uri]}
            onclick={() =>
                item.kind === "artist"
                    ? void openArtist(item.uri, item.art_url ? { art_url: item.art_url, round: true } : undefined)
                    : pick(item)}
        >
            {#if item.art_url}
                <img
                    class="r-art"
                    class:round={item.kind === "artist"}
                    src={item.art_url}
                    alt=""
                    loading="lazy"
                />
            {:else}
                <span class="r-art placeholder" class:round={item.kind === "artist"}>[ art ]</span>
            {/if}
            <span class="r-meta">
                <span class="r-name">{item.name}</span>
                {#if big}
                    <span class="r-line">{topLine(item)}</span>
                    {#if item.kind === "artist"}
                        <span class="r-cta"
                            >See top tracks &amp; albums <Icon
                                name="chevronRight"
                                size={13}
                            /></span
                        >
                    {/if}
                {:else if sub(item)}
                    <span class="r-sub">{sub(item)}</span>
                {/if}
            </span>
            <span class="r-tail">
                {#if !big && item.duration_ms}
                    <span class="r-dur mono">{fmtMs(item.duration_ms)}</span>
                {/if}
                {#if !(big && item.kind === "artist")}
                    <!-- A song plays; an artist opens — the tail says which. -->
                    <Icon name={item.kind === "artist" ? "chevronRight" : "play"} size={16} />
                {/if}
            </span>
        </button>
        {#if featured?.kind === "sonos" && item.kind !== "artist"}
            <button
                class="r-more"
                aria-label="More for {item.name}"
                aria-haspopup="menu"
                aria-expanded={menuFor === item.uri}
                disabled={music.busy["q:" + item.uri]}
                onclick={(e) => toggleMenu(e, item.uri)}
            >
                <Icon name="more" size={16} />
            </button>
            {#if menuFor === item.uri}
                <div
                    class="r-menu"
                    role="menu"
                    use:menuNav
                    in:scale={{
                        start: 0.95,
                        duration: dur(140),
                        easing: cubicOut,
                        opacity: 0,
                    }}
                    out:scale={{
                        start: 0.95,
                        duration: dur(100),
                        easing: cubicOut,
                        opacity: 0,
                    }}
                >
                    <button
                        class="r-menu-item"
                        role="menuitem"
                        onclick={() => {
                            menuFor = null;
                            music.enqueue(item, true);
                        }}
                    >
                        <Icon name="skipNext" size={16} /><span>Play next</span>
                    </button>
                    <button
                        class="r-menu-item"
                        role="menuitem"
                        onclick={() => {
                            menuFor = null;
                            music.enqueue(item, false);
                        }}
                    >
                        <Icon name="queue" size={16} /><span>Add to queue</span>
                    </button>
                </div>
            {/if}
        {/if}
    </div>
{/snippet}

<style>
    .browse {
        /* The depth takes the whole panel grid, whatever columns the
           dashboard depth is sized for. */
        grid-column: 1 / -1;
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        min-height: 0;
        min-width: 0;
    }
    /* Type mode: while the iPad's keyboard is up, the depth re-floors to
       just above it instead of running underneath. --kb is the measured
       keyboard height; + one panel padding lands the depth's new bottom
       edge on the keyboard's top edge exactly. */
    .browse.kb-open {
        align-self: start;
        max-height: calc(100% - var(--kb) + var(--space-6));
    }

    .b-head {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        flex-shrink: 0;
    }
    /* The way back to the dashboard depth — same quiet pill as the
       panel's Exit chip, mirrored to the leading edge like a detail
       screen's back chevron. */
    .back {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        height: 44px;
        padding: 0 var(--space-4);
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-mute);
        font-size: 13px;
        font-weight: 500;
        font-family: inherit;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .back:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    h2 {
        margin: 0;
        font-size: 20px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }

    .b-body {
        flex: 1;
        min-height: 0;
        display: grid;
        grid-template-columns: minmax(0, 1fr) 380px;
        gap: var(--space-5);
    }

    .b-left {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        min-height: 0;
        min-width: 0;
    }
    .p-panes {
        display: flex;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .k-chip {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 8px 14px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 500;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            border-color var(--t-fast),
            transform var(--t-fast);
    }
    .k-chip:active {
        transform: scale(0.95);
        transition-duration: 80ms;
    }
    .k-chip.active {
        background: var(--text);
        color: var(--bg);
        border-color: var(--text);
    }
    .k-chip.on {
        background: var(--on-soft);
        border-color: var(--tile-on-border);
        color: var(--on);
    }
    .k-chip:disabled {
        opacity: 0.55;
    }

    .b-search {
        display: flex;
        flex-direction: column;
        gap: var(--space-3);
        min-height: 0;
        flex: 1;
    }
    .b-pane {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding-bottom: var(--space-2);
    }
    /* The catalog stack owns the column's scroll the same way. */
    .b-stack {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding-bottom: var(--space-2);
    }
    .play-on {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    .s-box {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        height: 56px;
        padding: 0 var(--space-2) 0 var(--space-4);
        border-radius: var(--r-md);
        border: 1px solid var(--hairline);
        background: var(--card);
        color: var(--text-dim);
        flex-shrink: 0;
    }
    .s-box:focus-within {
        border-color: var(--border);
        box-shadow: var(--focus-ring);
    }
    input {
        flex: 1;
        min-width: 0;
        border: 0;
        background: none;
        color: var(--text);
        font-family: inherit;
        font-size: 17px; /* ≥16px keeps iOS from auto-zooming on focus */
        outline: none;
    }
    input::placeholder {
        color: var(--text-dim);
    }
    .s-clear {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        cursor: pointer;
        border-radius: var(--r-sm);
    }

    .s-kinds {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        flex-shrink: 0;
    }

    /* Type mode's dense results: single-line rows and nothing that isn't
       a match — the shelf labels and kind chips return with the keyboard,
       because filtering and browsing are what the choosing phase is for. */
    .kb-open .s-kinds,
    .kb-open .s-label {
        display: none;
    }
    .kb-open .r-open {
        min-height: 48px;
        padding: var(--space-1) var(--space-2);
    }
    .kb-open .r-art {
        width: 36px;
        height: 36px;
    }
    .kb-open .r-sub {
        display: none;
    }
    .kb-open .r-menu {
        top: 48px;
    }
    .kb-open .sk-row {
        min-height: 48px;
    }
    .kb-open .sk-art {
        width: 36px;
        height: 36px;
    }

    .s-results {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        padding-bottom: var(--space-2);
    }
    .s-label {
        margin: var(--space-4) 0 var(--space-2);
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .s-label:first-child {
        margin-top: 0;
    }

    /* A shelf head with a trailing action (the recents' Clear) — the
       label's own margin moves onto the row. */
    .s-shelf-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-2);
        margin: var(--space-4) 0 var(--space-2);
    }
    .s-shelf-head:first-child {
        margin-top: 0;
    }
    .s-shelf-head .s-label {
        margin: 0;
    }

    /* Recent searches: a chip cloud, distance-scaled like everything else
       on the wall — one tap re-runs, the × forgets one, Clear forgets the
       room's list. */
    .s-recents {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .s-recent {
        display: inline-flex;
        align-items: stretch;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        overflow: hidden;
    }
    .s-recent-run {
        display: inline-flex;
        align-items: center;
        gap: 8px;
        min-height: 48px;
        padding: 0 4px 0 16px;
        border: 0;
        background: none;
        color: var(--text);
        font: inherit;
        font-size: 15px;
        cursor: pointer;
        transition: background var(--t-fast);
    }
    .s-recent-run :global(svg) {
        color: var(--text-dim);
        flex-shrink: 0;
    }
    /* The query's own top result, once the search behind it has answered —
       round for an artist's photo, square for everything else's cover art. */
    .s-recent-art {
        width: 28px;
        height: 28px;
        border-radius: var(--r-sm);
        object-fit: cover;
        flex-shrink: 0;
        background: var(--card-3);
    }
    .s-recent-art.round {
        border-radius: 50%;
    }
    .s-recent-run span {
        max-width: 240px;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .s-recent-x {
        width: 44px;
        align-self: stretch;
        display: grid;
        place-items: center;
        border: 0;
        background: none;
        color: var(--text-mute);
        cursor: pointer;
    }
    @media (hover: hover) {
        .s-recent-run:hover {
            background: var(--card-3);
        }
        .s-recent-x:hover {
            color: var(--text);
        }
    }

    /* The account's playlists, idle: covers on a grid (§15.9 — everything
       but songs is a grid), three across the work column. */
    .s-pl-grid {
        display: grid;
        grid-template-columns: repeat(3, minmax(0, 1fr));
        gap: var(--space-4) var(--space-3);
    }

    /* The row is a container, not a control: a song plays outright, an
       artist opens their page, and the trailing overflow queues without
       interrupting. */
    .row {
        position: relative;
        display: flex;
        align-items: center;
        gap: var(--space-1);
        border-radius: var(--r-md);
        transition: background var(--t-fast);
    }
    @media (hover: hover) {
        .row:hover {
            background: var(--card-2);
        }
    }
    .r-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 64px;
        padding: var(--space-2);
        border: 1px solid transparent;
        border-radius: var(--r-md);
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .r-open:active {
        transform: scale(0.98);
        background: var(--card-2);
        transition-duration: 80ms;
    }
    .r-open:disabled {
        opacity: 0.55;
    }
    .r-art {
        width: 48px;
        height: 48px;
        border-radius: var(--r-sm);
        object-fit: cover;
        flex-shrink: 0;
        display: block;
    }
    /* An artist's art is a portrait — round, the way the app reads them,
       and a quiet tell for the one kind that opens rather than plays. */
    .r-art.round {
        border-radius: 50%;
    }
    span.r-art {
        font-size: 10px;
    }
    .r-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .r-name {
        font-size: 16px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .r-sub {
        font-size: 13px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .r-tail {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        flex-shrink: 0;
        color: var(--text-dim);
        padding-right: var(--space-2);
    }
    .r-dur {
        font-size: 13px;
        color: var(--text-mute);
    }

    /* The top result: the same row, card-sized — the biggest tappable
       thing in the results, because it is the answer most searches were
       after (§15.9). */
    .row.big {
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        padding: var(--space-2);
    }
    @media (hover: hover) {
        .row.big:hover {
            background: var(--card-2);
            border-color: var(--border-strong);
        }
    }
    .row.big .r-open {
        gap: var(--space-4);
    }
    .row.big .r-art {
        width: 76px;
        height: 76px;
        border-radius: var(--r-md);
    }
    .row.big .r-art.round {
        border-radius: 50%;
    }
    .row.big .r-name {
        font-size: 19px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    .row.big .r-meta {
        gap: 4px;
    }
    .r-line {
        font-family: var(--font-mono);
        font-size: 11px;
        letter-spacing: 0.05em;
        text-transform: uppercase;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* Says where the tap goes, so "open" never has to be guessed from a
       chevron alone. */
    .r-cta {
        display: inline-flex;
        align-items: center;
        gap: 4px;
        margin-top: 2px;
        font-size: 13px;
        color: var(--on);
    }
    .row.big .r-menu {
        top: 84px;
    }

    .r-more {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        flex-shrink: 0;
        margin-right: 4px;
        border: 0;
        border-radius: var(--r-sm);
        background: none;
        color: var(--text-mute);
        cursor: pointer;
    }
    .r-more:disabled {
        opacity: 0.4;
    }
    .r-menu {
        position: absolute;
        right: 8px;
        top: 56px;
        z-index: var(--z-menu);
        min-width: 190px;
        display: flex;
        flex-direction: column;
        background: var(--card-2);
        border: 1px solid var(--border-strong);
        border-radius: var(--r-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .r-menu-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 14px var(--space-4);
        background: transparent;
        border: 0;
        border-bottom: 1px solid var(--hairline);
        cursor: pointer;
        font: inherit;
        font-size: 14px;
        color: var(--text);
        text-align: left;
    }
    .r-menu-item:last-child {
        border-bottom: 0;
    }
    @media (hover: hover) {
        .r-menu-item:hover {
            background: var(--card-3);
        }
    }

    .s-empty {
        height: 100%;
        display: grid;
        place-items: center;
    }

    .sk-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .sk-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 64px;
        padding: var(--space-2);
    }
    .sk-art {
        width: 48px;
        height: 48px;
        border-radius: var(--r-sm);
        flex-shrink: 0;
    }
    .sk-lines {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .sk-l1 {
        width: 55%;
        height: 13px;
    }
    .sk-l2 {
        width: 35%;
        height: 11px;
    }

    /* ── Rooms pane ── */
    .rm-list {
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    .rm-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        background: var(--card);
        transition:
            border-color var(--t-fast),
            background var(--t-fast);
    }
    .rm-row.active {
        border-color: var(--tile-on-border);
        background: var(--tile-on-gradient);
    }
    .rm-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 56px;
        padding: var(--space-2);
        border: 0;
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-sm);
    }
    .rm-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .rm-name {
        font-size: 16px;
        font-weight: 500;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rm-sub {
        font-size: 12.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rm-wave {
        display: inline-flex;
        flex-shrink: 0;
        padding: 6px 8px;
        border-radius: var(--r-sm);
        background: var(--bg-bar);
    }
    .rm-join {
        flex-shrink: 0;
        max-width: 45%;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .rm-members {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
        padding: var(--space-1) var(--space-2) var(--space-2);
    }
    .rm-mchip {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        padding: 7px 12px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text-mute);
        font-size: 12.5px;
        font-weight: 500;
    }
    .rm-lead {
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    .rm-x {
        width: 24px;
        height: 24px;
        display: grid;
        place-items: center;
        border: 0;
        border-radius: 50%;
        background: var(--card-3);
        color: var(--text-mute);
        cursor: pointer;
    }
    .rm-x:disabled {
        opacity: 0.4;
    }
    .rm-note {
        margin: var(--space-4) 0 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-dim);
    }

    .b-player {
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
    }
    .p-nosrc {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--space-3);
        border: 1px dashed var(--border);
        border-radius: var(--r-lg);
        color: var(--text-dim);
        font-size: 14px;
    }
    .p-nosrc p {
        margin: 0;
    }

    /* Portrait / narrow fallback: search first, the player under it, and
       the page scrolls (the panel is designed landscape-first). */
    @media (orientation: portrait), (max-width: 760px) {
        .browse {
            min-height: 100%;
        }
        .b-body {
            grid-template-columns: minmax(0, 1fr);
        }
        .s-results,
        .b-pane,
        .b-stack {
            overflow: visible;
            flex: none;
        }
    }
</style>
