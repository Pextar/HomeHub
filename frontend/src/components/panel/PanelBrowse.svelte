<script lang="ts">
    /**
     * The panel's music depth (DESIGN.md §16): the whole music player, on
     * the kiosk — no chrome, no sheets, no app shell. The featured room's
     * player rides on the right; the left is the work area, switched by
     * chip between three panes:
     *
     *   Search  Spotify's catalog, with the room's recent searches, the
     *           household's Sonos favorites and the account's playlists
     *           while the box is empty. A song plays outright; an artist
     *           opens their page — the app's own catalog screens (§15.9),
     *           one level deeper in this same column, with a record or a
     *           related artist going deeper still and back climbing one
     *           level. Queueing without interrupting is two named buttons
     *           on the row, and only for a Sonos destination — the queue
     *           is a Sonos group's.
     *   Queue   The featured group's queue: tap to jump, X to remove,
     *           two taps to clear (there is no confirm modal on a kiosk).
     *   Rooms   Every room — HomeHub zones first, then Sonos groups, then
     *           the KEF speakers standing alone — with Sonos-native
     *           grouping: join the featured room, split one apart, or step
     *           a single speaker out. A zone is *played* here but never
     *           built here: arranging one is configuration, and that stays
     *           in the full Music view.
     *
     * The back chip returns to the panel dashboard — or climbs one level
     * of the catalog stack first; Escape does the same, unless a row menu
     * has it first.
     */
    import { onMount, tick as flushDOM } from "svelte";
    import { fade } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import Waveform from "../music/Waveform.svelte";
    import EmptyState from "../EmptyState.svelte";
    import QueuePane from "../music/QueuePane.svelte";
    import MediaCard from "../music/MediaCard.svelte";
    import ArtistScreen from "../music/ArtistScreen.svelte";
    import ContextScreen from "../music/ContextScreen.svelte";
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
    import PanelRoomSettings from "./PanelRoomSettings.svelte";
    import { api } from "../../lib/api";
    import { route, toasts } from "../../lib/stores.svelte";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";
    import { fmtCount, fmtMs, capFirst } from "../../lib/music/format";
    import { SEARCH_KINDS as KINDS, topLine } from "../../lib/music/catalog";
    import { dur } from "../../lib/motion";
    import { kefSourceLabel } from "../../lib/kef";
    import type { PanelMusicStore, PanelSource } from "../../lib/panel-music.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyArtistDetail, SpotifyContextDetail, SpotifyItem } from "../../lib/types";

    // The catalog and the room's history are the panel's, not this
    // component's: the depth is a route away, and a route away and back
    // used to throw a half-typed search out with the component.
    let {
        music,
        spotify,
        recents,
        booted,
    }: {
        music: PanelMusicStore;
        spotify: SpotifyStore;
        recents: SearchHistory;
        /** False until the Spotify status read has answered either way. */
        booted: boolean;
    } = $props();

    // Arriving in the depth pins the destination. Without it the featured
    // room is only ever a fallback — "whatever is playing" — so a speaker
    // that starts up elsewhere mid-search would re-point the room chips,
    // the queue and the next tap at a room nobody chose.
    onMount(() => music.latchFeatured());

    const featured = $derived(music.featured);

    /** What a screen reader is told when the list changes under a box that
     *  still has the caret. */
    const liveMessage = $derived.by(() => {
        if (spotify.pending) return "Searching…";
        if (spotify.error) return "Search failed.";
        if (!spotify.results) return "";
        const n = sections.reduce((sum, s) => sum + s.items.length, 0);
        if (n === 0) return `No results for ${spotify.resultsQuery}.`;
        return `${n} result${n === 1 ? "" : "s"} for ${spotify.resultsQuery}.`;
    });

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
    /** The stack is the Search pane's — it was opened from a search row and
     *  it is where the back chip climbs to. With the switcher on the header
     *  it is reachable while a page is open, so Queue and Rooms answer over
     *  the top of it and Search comes back to where it was. */
    const catalogOpen = $derived(pane === "search" ? topLevel : null);

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

    // Escape leaves the depth — a catalog screen's row menu open at the
    // time gets the key first, then each level of the catalog stack, and
    // the search box gets it when there is a query to clear. The search
    // rows here have no menu of their own to close: queueing is two named
    // buttons on the row, not a menu to open and dismiss.
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (artistScr?.closeMenu() || contextScr?.closeMenu()) return;
            // Only while the stack is the thing on screen: a page left open
            // behind the Queue pane is not what an Escape over the queue is
            // aimed at.
            if (catalogOpen) {
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
        if (s.kind === "zone") {
            // A zone says what it is made of and how it is being driven —
            // "buffered" is a real difference from a native group, and the
            // backend already worded it, so the wall repeats it rather than
            // inferring one (§15).
            const n = s.members?.length ?? 0;
            const how = s.route === "stream" ? "streamed together" : "played together";
            return [n > 1 ? `${n} speakers` : "HomeHub room", how].filter(Boolean).join(" · ");
        }
        return s.members && s.members.length > 1 ? `${s.members.length} speakers` : "Sonos";
    }

    // ── Favorites: the household's own list ──────────────────────────────
    // Radio stations and whatever was starred in the Sonos app. On the wall
    // they matter twice over: a station is a one-tap job search can't be,
    // and without a linked Spotify account they are the only thing this
    // depth could start at all. Sonos-only, because the list is.
    const favTarget = $derived(featured?.kind === "sonos");
    const favorites = $derived(favTarget ? music.favorites : []);

    /** Nothing at all to idle on — a different panel from a thin one. */
    const idleEmpty = $derived(
        recents.list.length === 0 &&
            favorites.length === 0 &&
            spotify.myPlaylists.length === 0 &&
            spotify.recentTracks.length === 0 &&
            spotify.topTracks.length === 0,
    );
</script>

<div class="browse" class:kb-open={kbOpen} style:--kb="{kb}px" in:fade={{ duration: dur(160) }}>
    <!-- The depth's own band, drawn the way the dashboard's status strip is:
         a fixed 72px row, edge to edge, divided from the body by a hairline
         rather than floated over it. It carries everything about the surface
         that isn't the work itself — the way back, where a tap plays, and
         which pane the work area is showing. -->
    <header class="b-head">
        <button class="back" onclick={back} aria-label="Back to the panel">
            <Icon name="chevronLeft" size={18} />
        </button>
        <h2 class="sr-only">Music</h2>
        <!-- Where a tap plays. Full-width here rather than stacked in the
             player column, where six rooms cost three rows of the cover's
             height (§16); the search below still names the same room in
             its "Plays on {…}" line. -->
        <PanelRoomChips {music} />
        <!-- The pane switcher rides the header's trailing edge as one
             segmented control. In the work area it was a band above the
             results, and on a 656px column every band above the results is a
             result you can't see; on the header it costs the column nothing
             and holds the same place whichever pane is up. -->
        <div class="p-panes" role="group" aria-label="Music panes">
            <button
                class="p-pane"
                class:active={pane === "search"}
                aria-pressed={pane === "search"}
                onclick={() => (pane = "search")}
            >
                Search
            </button>
            <button
                class="p-pane"
                class:active={pane === "queue"}
                aria-pressed={pane === "queue"}
                onclick={() => (pane = "queue")}
            >
                Queue{#if featured?.kind === "sonos" && queueCount > 0}
                    <span class="mono">{queueCount}</span>{/if}
            </button>
            <button
                class="p-pane"
                class:active={pane === "rooms"}
                aria-pressed={pane === "rooms"}
                onclick={() => (pane = "rooms")}
            >
                Rooms <span class="mono">{music.sources.length}</span>
            </button>
        </div>
    </header>

    <div class="b-body">
        <section class="b-left">
            {#if catalogOpen}
                <!-- One level deeper: the app's own artist/record pages ride
                     in this column; their back chip climbs the stack. The
                     stack belongs to the Search pane, so stepping over to
                     Queue and back finds the artist's page where it was
                     left rather than at the search results. -->
                {@const level = catalogOpen}
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

                    <!-- Where a tap lands, said under the box the tapping
                         starts in: the results are otherwise the one place on
                         the wall that never names their own destination, and
                         a wall is the surface most likely to be used by
                         whoever walked past it last. It rides on a line of
                         its own now that the pane switcher has left the
                         column for the header — one thin band replacing a
                         whole row, which the results keep. -->
                    <p class="s-dest">
                        <Icon name="speaker" size={14} />
                        <span
                            >{featured
                                ? `Plays on ${featured.title}`
                                : "No speaker is answering"}</span
                        >
                    </p>

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

                    <p class="sr-only" role="status" aria-live="polite">{liveMessage}</p>

                    <div
                        class="s-results"
                        class:stale={spotify.stale}
                        bind:this={resultsEl}
                        aria-busy={spotify.searching}
                    >
                        {#if !booted || spotify.pending}
                            <!-- Only with nothing on screen yet. A search
                                 that runs while results are up keeps them
                                 and dims them: on a wall the list is read
                                 from across the room, and blanking it on
                                 every letter is the worst thing this depth
                                 could do to someone mid-glance. -->
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
                        {:else if spotify.error}
                            <!-- Sized for the wall: the retry is the point,
                                 and it is a target, not a line of text. -->
                            <div class="s-empty">
                                <div class="s-fail">
                                    <Icon name="info" size={22} />
                                    <p class="s-fail-line">Couldn't reach Spotify</p>
                                    <p class="s-fail-why">{spotify.error}</p>
                                    <button class="s-retry" onclick={() => spotify.retry()}>
                                        Try again
                                    </button>
                                </div>
                            </div>
                        {:else if !spotify.connected}
                            <!-- Setup stays in the full view, but a wall
                                 with nothing playable on it is a dead end,
                                 and this home may well have a favorites
                                 list that needs no account at all. It leads;
                                 the pointer to setup follows it. -->
                            {#if favorites.length > 0}
                                {@render favoriteShelf()}
                                <p class="s-note">
                                    Spotify isn't connected — search is set up in the Music view.
                                </p>
                            {:else}
                                <div class="s-empty">
                                    <EmptyState
                                        icon="musicNotes"
                                        title="Spotify isn't connected"
                                        message="Search and playback are set up in the Music view."
                                        compact
                                    />
                                </div>
                            {/if}
                        {:else if spotify.results}
                            {#if sections.length === 0}
                                <div class="s-empty">
                                    <EmptyState
                                        icon="search"
                                        title="Nothing found for “{spotify.resultsQuery}”"
                                        message={spotify.kindFilter === "all"
                                            ? "Try another name, song or album."
                                            : "Nothing of that kind matched — the other chips may have it."}
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
                                    <!-- Spotify answers ten per kind and no
                                         more, so a shelf pages for the rest
                                         rather than making its own count a
                                         number nobody can act on. -->
                                    {#if spotify.hasMore(sec.id)}
                                        <div class="s-more">
                                            <button
                                                class="k-chip"
                                                disabled={spotify.loadingMore}
                                                onclick={() => spotify.loadMore(sec.id)}
                                            >
                                                {spotify.loadingMore
                                                    ? "Loading…"
                                                    : `More ${sec.label.toLowerCase()}`}
                                            </button>
                                        </div>
                                    {/if}
                                {/each}
                            {/if}
                        {:else}
                            <!-- Idle shelves, in the order they answer "put
                                 something on" without typing — which is the
                                 whole job on a wall. What was playing lately
                                 leads, then this room's recent searches, the
                                 household's favorites, what the account plays
                                 most, and its playlists as an art grid
                                 (§15.9: everything but songs is a grid). A
                                 tap plays it on the featured room, like
                                 every container on the wall. -->
                            {#if spotify.recentTracks.length > 0}
                                <h3 class="s-label">Played recently</h3>
                                {#each spotify.recentTracks.slice(0, 6) as item (item.uri)}
                                    {@render resultRow(item, false)}
                                {/each}
                            {/if}
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
                            {@render favoriteShelf()}
                            {#if spotify.topTracks.length > 0}
                                <h3 class="s-label">You play these most</h3>
                                {#each spotify.topTracks.slice(0, 6) as item (item.uri)}
                                    {@render resultRow(item, false)}
                                {/each}
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
                            {/if}
                            {#if idleEmpty}
                                <div class="s-empty">
                                    <EmptyState
                                        icon="search"
                                        title="Search Spotify"
                                        message="Songs, albums, artists and playlists — played on {featured?.title ??
                                            'this room'}."
                                        compact
                                    />
                                </div>
                            {:else if spotify.needsListeningScope}
                                <!-- Configuration stays in the full view, so
                                     the wall names the fix rather than
                                     offering it. -->
                                <p class="s-note">
                                    Reconnect Spotify in the Music view to see what you've been
                                    playing here — this login was made before HomeHub could ask.
                                </p>
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
                                title="No queue in this room"
                                message={featured?.kind === "zone"
                                    ? `${featured.title} is a HomeHub room — the speakers in it play what they are handed, one thing at a time.`
                                    : `${featured?.title ?? "This room"} plays straight through — there is no queue to manage.`}
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
                    <!-- The featured room's own preferences lead the pane:
                         this is the "which device" surface, and they used to
                         be stacked under the cover in the player column
                         where they cost it two thirds of its height. -->
                    <PanelRoomSettings {music} />
                    <h3 class="s-label">Rooms</h3>
                    <div class="rm-list">
                        {#each music.sources as s (s.key)}
                            {@const isFeatured = featured?.key === s.key}
                            <!-- Chosen is an edge, not a glow: the ON
                                 gradient means "playing" everywhere else in
                                 the app (§6.1), and a silent room that
                                 merely has the focus must not wear it. -->
                            <div class="rm-row" class:active={isFeatured} class:live={s.playing}>
                                <button
                                    class="rm-main"
                                    aria-label="Feature {s.title}"
                                    aria-pressed={isFeatured}
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
                        Sonos rooms group natively — join or split them here. A HomeHub room plays
                        any mix of speakers together and can be started from this panel, but
                        arranging one is done in the Music view.
                    </p>
                </div>
            {/if}
        </section>

        <section class="b-player" aria-label="Now playing">
            {#if featured}
                <PanelPlayerCard
                    {music}
                    onExpand={() => route.go("panel", { music: "1", player: "1" })}
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

<!-- The household's own list, as covers on a grid (§15.9: everything but
     songs is a grid). One tap plays it outright — a station is the one
     thing on this depth that needs no typing at all — and there is no
     queue affordance here on purpose: most of what a home stars is radio,
     and a live stream has no place in a queue. Rendered only for a Sonos
     destination, because the list is a Sonos household's. -->
{#snippet favoriteShelf()}
    {#if favorites.length > 0}
        <h3 class="s-label">Favorites</h3>
        <div class="s-fav-grid">
            {#each favorites as f (f.id)}
                <button
                    class="s-fav-play"
                    aria-label="Play {f.title} on {featured?.title ?? 'this room'}"
                    disabled={!!music.busy["fav:" + f.id]}
                    onclick={() => music.playFavorite(f)}
                >
                    {#if f.art_uri}
                        <img class="s-fav-art" src={f.art_uri} alt="" loading="lazy" />
                    {:else}
                        <span class="s-fav-art placeholder">[ art ]</span>
                    {/if}
                    <span class="s-fav-title">{f.title}</span>
                    {#if f.service}<span class="s-fav-sub mono">{f.service}</span>{/if}
                </button>
            {/each}
        </div>
    {/if}
{/snippet}

<!-- The catalog screens name where they'll sound; on the wall that's
     always the featured source — its chips ride on the player column. -->
{#snippet playOnRow()}
    <span class="play-on">
        <Icon name="speaker" size={14} />
        <span>{featured ? `Plays on ${featured.title}` : "No speaker reachable"}</span>
    </span>
{/snippet}

<!-- The one row shape for everything the catalog returns (§14): a song or
     a container plays outright, an artist opens their page, and the two
     trailing buttons queue without interrupting — for a Sonos destination
     only, the queue being a Sonos group's. They are buttons rather than an
     overflow menu because this is a wall: a menu costs a tap to open, a
     tap to choose and a tap to dismiss, and at arm's length two named
     44px targets beat all three. `big` is the search's top result: the
     same row at full size, saying what it is and where the tap goes. -->
{#snippet resultRow(item: SpotifyItem, big: boolean)}
    <div class="row" class:big>
        <button
            class="r-open"
            disabled={item.kind !== "artist" && music.busy["item:" + item.uri]}
            onclick={() =>
                item.kind === "artist"
                    ? void openArtist(
                          item.uri,
                          item.art_url ? { art_url: item.art_url, round: true } : undefined,
                      )
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
                class="r-q"
                aria-label="Play {item.name} next"
                disabled={music.busy["q:" + item.uri]}
                onclick={() => music.enqueue(item, true)}
            >
                <Icon name="skipNext" size={16} />
            </button>
            <button
                class="r-q"
                aria-label="Add {item.name} to the queue"
                disabled={music.busy["q:" + item.uri]}
                onclick={() => music.enqueue(item, false)}
            >
                <Icon name="plus" size={16} />
            </button>
        {/if}
    </div>
{/snippet}

<style>
    .browse {
        /* The depth takes the whole panel grid — every row of it, so the
           surface is the screen and each region owns its own overflow. Like
           the dashboard's bands it runs edge to edge and lets each region pad
           itself: a wall panel has no page around it to show, and a margin
           there is only screen the depth isn't using (§16). */
        grid-column: 1 / -1;
        grid-row: 1 / -1;
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
    }
    /* Type mode: while the iPad's keyboard is up, the depth re-floors to
       just above it instead of running underneath. --kb is the measured
       keyboard height, and the depth's new bottom edge lands on the
       keyboard's top edge exactly. */
    .browse.kb-open {
        align-self: start;
        max-height: calc(100% - var(--kb));
    }

    /* The depth's header band: the same 72px row on both depths, drawn like
       the dashboard's status strip — a hairline under it rather than a gap,
       and its own inline padding. */
    .b-head {
        height: 72px;
        flex-shrink: 0;
        display: flex;
        align-items: center;
        gap: var(--space-4);
        min-width: 0;
        padding: 0 var(--space-8);
        border-bottom: 1px solid var(--hairline);
    }
    /* The chip row keeps its own one-line, shrink-then-scroll behaviour
       (PanelRoomChips) on every surface that carries it; here it just takes
       the space between the back chip and the pane switcher. */
    .b-head :global(.p-sources) {
        flex: 1 1 auto;
    }
    /* The way back to the dashboard depth: a round chevron chip on the
       leading edge, the same shape the full player's header wears. */
    .back {
        width: 40px;
        height: 40px;
        flex-shrink: 0;
        display: grid;
        place-items: center;
        border-radius: 50%;
        border: 1px solid var(--hairline);
        background: var(--card-2);
        color: var(--text);
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .back:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }

    /* The pane switcher as one segmented control: a track around the three,
       so they read as one choice rather than as three chips that happen to
       sit together. */
    .p-panes {
        display: flex;
        gap: 6px;
        flex-shrink: 0;
        padding: 4px;
        border-radius: var(--r-pill);
        border: 1px solid var(--hairline);
        background: var(--card-2);
    }
    .p-pane {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        min-height: 36px;
        padding: 0 16px;
        border: 0;
        border-radius: var(--r-pill);
        background: none;
        color: var(--text-mute);
        font-family: inherit;
        font-size: 13px;
        font-weight: 600;
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast);
    }
    .p-pane.active {
        background: var(--text);
        color: var(--bg);
    }
    .p-pane:focus-visible {
        outline: none;
        box-shadow: var(--focus-ring);
    }

    .b-body {
        flex: 1;
        min-height: 0;
        display: grid;
        /* The catalog on the left takes what is left; the player holds a
           fixed 420px on the right, divided by a hairline rather than a gap
           — the two are regions of one surface, not two cards on a page. */
        grid-template-columns: minmax(0, 1fr) 420px;
    }

    .b-left {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        min-height: 0;
        min-width: 0;
        padding: var(--space-5) var(--space-6);
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
        min-width: 0;
        flex: 1;
    }
    .b-pane {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        /* The same queue list lives here, and rounds the same way: without
           this, `overflow-y: auto` computes the x axis to `auto` too and a
           row that measures 432.03px in a 432px pane pans sideways by the
           pixel the rounding invents (see `.fp-queue`). */
        overflow-x: hidden;
        padding-bottom: var(--space-2);
    }
    /* The catalog stack owns the column's scroll the same way — including
       the one-axis rule: an artist's page is rows of text too, and the
       pixel rounding invents pans it sideways exactly like the queue
       (§12). */
    .b-stack {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        overflow-x: hidden;
        padding-bottom: var(--space-2);
    }
    .play-on {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12.5px;
        color: var(--text-mute);
    }
    /* The box was 56px when it shared the column with the pane switcher and
       was the thing that had to be found. The switcher is on the header now
       and the results below are what this column is for, so the box gives
       back the height it was only using to be large. Its text stays at 17px
       — that floor is iOS's, not a preference (§16). */
    .s-box {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        height: 48px;
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
        /* The kiosk root disables selection (user-select: none) so the wall
           panel can't be text-selected by a stray touch. Safari treats that
           as inherited focus suppression too: without opting this input back
           in, a tap never raises the software keyboard. */
        user-select: text;
        -webkit-user-select: text;
        /* The kiosk root also disables the touch callout (-webkit-touch-callout:
           none) to suppress the copy/paste bubble on a stray touch. That's a
           second, independent suppressor from user-select: with it inherited
           here, taps focus the input (caret shows) but Safari still withholds
           the software keyboard. Opt back in to actually raise it. */
        -webkit-touch-callout: default;
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

    /* Where a tap lands. Quiet — it is a caption, not a control — but
       always there, because the room chips live in the other column. */
    .s-dest {
        display: flex;
        align-items: center;
        gap: 6px;
        margin: 0;
        font-size: 12.5px;
        color: var(--text-mute);
        flex-shrink: 0;
    }
    /* A note under a shelf that stands in for an empty state. */
    .s-note {
        margin: var(--space-4) 0 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-dim);
    }

    /* Type mode's dense results: single-line rows and nothing that isn't
       a match — the shelf labels and kind chips return with the keyboard,
       because filtering and browsing are what the choosing phase is for. */
    .kb-open .s-kinds,
    .kb-open .s-dest,
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
        min-width: 0;
        overflow-y: auto;
        overflow-x: hidden;
        padding-bottom: var(--space-2);
        transition: opacity var(--t-fast);
    }
    /* A newer search running behind the list: it stays and dims, and stops
       taking taps — a row tapped while it is being replaced would play
       whatever landed in its place. */
    .s-results.stale {
        opacity: 0.45;
        pointer-events: none;
    }

    .s-more {
        display: flex;
        justify-content: center;
        margin-top: var(--space-3);
    }

    /* The search didn't get through. On a wall the retry has to be a target
       rather than a sentence with a link in it. */
    .s-fail {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-2);
        text-align: center;
        color: var(--text-dim);
    }
    .s-fail-line {
        margin: 0;
        font-size: 16px;
        color: var(--text-mute);
    }
    .s-fail-why {
        margin: 0;
        font-size: 12.5px;
        max-width: 42ch;
    }
    .s-retry {
        margin-top: var(--space-2);
        min-height: 48px;
        padding: 0 var(--space-5);
        border-radius: var(--r-pill);
        border: 1px solid var(--border-strong);
        background: var(--card-2);
        color: var(--text);
        font-family: inherit;
        font-size: 14px;
        font-weight: 500;
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .s-retry:active {
        transform: scale(0.96);
        transition-duration: 80ms;
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
    /* The column count follows the width rather than being stated: four on
       the depth's 556px column, which draws a 130px tile — near enough the
       dashboard band shelf's 132 that a cover is the same size wherever the
       wall offers one — and three in the portrait fallback's narrower one.
       The 110px floor is the breakpoint knob, not the tile size: the tiles
       are `1fr` and land well above it at both widths. It used to state
       three outright, which on the depth made them posters in a column that
       has more to show than four playlists. */
    .s-pl-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
        gap: var(--space-4) var(--space-3);
    }

    /* The household's favorites: the same grid, four across — a station's
       cover carries less to read than a playlist's, so they sit smaller. */
    .s-fav-grid {
        display: grid;
        grid-template-columns: repeat(4, minmax(0, 1fr));
        gap: var(--space-4) var(--space-3);
    }
    .s-fav-play {
        display: flex;
        flex-direction: column;
        gap: 4px;
        width: 100%;
        padding: 2px;
        border: 0;
        border-radius: var(--r-md);
        background: transparent;
        color: var(--text);
        font: inherit;
        text-align: left;
        cursor: pointer;
        transition: transform var(--t-fast);
    }
    .s-fav-play:active {
        transform: scale(0.97);
        transition-duration: 80ms;
    }
    .s-fav-play:disabled {
        opacity: 0.5;
    }
    .s-fav-art {
        width: 100%;
        aspect-ratio: 1;
        border-radius: var(--r-sm);
        object-fit: cover;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        display: block;
    }
    span.s-fav-art {
        display: grid;
        place-items: center;
        font-size: 9px;
        color: var(--text-dim);
    }
    .s-fav-title {
        margin-top: 4px;
        font-size: 12.5px;
        font-weight: 500;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .s-fav-sub {
        font-size: 10px;
        letter-spacing: 0.04em;
        color: var(--text-dim);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    @media (hover: hover) {
        .s-fav-play:hover .s-fav-title {
            color: var(--on);
        }
    }

    /* The row is a container, not a control: a song plays outright, an
       artist opens their page, and the trailing overflow queues without
       interrupting. */
    .row {
        position: relative;
        display: flex;
        align-items: center;
        gap: var(--space-1);
        min-width: 0;
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

    /* Queue without interrupting: two named targets on the row, no menu to
       open or dismiss. */
    .r-q {
        width: 44px;
        height: 44px;
        display: grid;
        place-items: center;
        flex-shrink: 0;
        border: 1px solid var(--hairline);
        border-radius: 50%;
        background: var(--card-2);
        color: var(--text-mute);
        cursor: pointer;
        transition:
            background var(--t-fast),
            color var(--t-fast),
            transform var(--t-fast);
    }
    .r-q:last-child {
        margin-right: 4px;
    }
    .r-q:active {
        transform: scale(0.92);
        transition-duration: 80ms;
    }
    .r-q:disabled {
        opacity: 0.4;
    }
    @media (hover: hover) {
        .r-q:hover {
            color: var(--text);
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
    /* Playing and chosen are independent states, so they are drawn in two
       different registers: audio gets the §6.1 ON gradient, the chosen
       room gets a ring. A silent room that merely holds the focus used to
       wear "on", and a playing room that wasn't chosen looked identical to
       the one that was. */
    .rm-row.live {
        border-color: var(--tile-on-border);
        background: var(--tile-on-gradient);
    }
    .rm-row.active {
        border-color: var(--border-strong);
        box-shadow: inset 0 0 0 2px var(--border-strong);
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
        position: relative;
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

    /* The player's column: a hairline off the work area, and its own
       padding. The player inside it is flat — the column is already a region
       of this surface, and a bordered card inside a hairline-divided column
       draws the same edge twice (§16). */
    .b-player {
        display: flex;
        flex-direction: column;
        min-height: 0;
        min-width: 0;
        padding: var(--space-7);
        border-left: 1px solid var(--hairline);
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

    /* Distance-scaled targets (§2's floor, §16's reason): this is a wall,
       so the chips clear 44px rather than inheriting a phone's sizing, and
       the one control too small to grow — the × that steps a speaker out of
       a group — grows its hit area instead of its box, so the member chips
       keep their shape. */
    @media (pointer: coarse) {
        .k-chip {
            min-height: 44px;
            padding-inline: 16px;
        }
        .back {
            width: 44px;
            height: 44px;
        }
        .p-pane {
            min-height: 44px;
        }
        .rm-x::after {
            content: "";
            position: absolute;
            inset: -10px;
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .s-results {
            transition-duration: 0.001ms;
        }
    }

    /* Portrait / narrow fallback: search first, the player under it, and
       the page scrolls (the panel is designed landscape-first). */
    @media (orientation: portrait), (max-width: 760px) {
        .browse {
            min-height: 100%;
        }
        .b-head {
            height: auto;
            flex-wrap: wrap;
            padding: var(--space-4) var(--space-5);
        }
        .b-body {
            grid-template-columns: minmax(0, 1fr);
        }
        /* Stacked, the divider is above the player rather than beside it. */
        .b-player {
            border-left: 0;
            border-top: 1px solid var(--hairline);
            padding: var(--space-5);
        }
        .s-results,
        .b-pane,
        .b-stack {
            overflow: visible;
            flex: none;
        }
    }
</style>
