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
    import EmptyState from "../EmptyState.svelte";
    import QueuePane from "../music/QueuePane.svelte";
    import ArtistScreen from "../music/ArtistScreen.svelte";
    import ContextScreen from "../music/ContextScreen.svelte";
    import PanelPlayerCard from "./PanelPlayerCard.svelte";
    import PanelRoomChips from "./PanelRoomChips.svelte";
    import PanelBrowseRooms from "./PanelBrowseRooms.svelte";
    import PanelSearchDock from "./PanelSearchDock.svelte";
    import PanelSearchBox from "./PanelSearchBox.svelte";
    import PanelSearchResults from "./PanelSearchResults.svelte";
    import { api } from "../../lib/api";
    import { route, toasts } from "../../lib/stores.svelte";
    import { clock } from "../../lib/music/clock.svelte";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";
    import { createCatalogCache, contextItem } from "../../lib/music/catalog-cache.svelte";
    import { fmtHour } from "../../lib/music/format";
    import { searchSections } from "../../lib/music/catalog";
    import { dur } from "../../lib/motion";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    // The catalog and the room's history are the panel's, not this
    // component's: the depth is a route away, and a route away and back
    // used to throw a half-typed search out with the component.
    let {
        music,
        spotify,
        recents,
        booted,
        openArtistNamed = "",
        openPane = "",
    }: {
        music: PanelMusicStore;
        spotify: SpotifyStore;
        recents: SearchHistory;
        /** False until the Spotify status read has answered either way. */
        booted: boolean;
        /** An artist to open on arrival, by name — how the full player hands
         *  "who is this?" back to the depth (§16). A speaker reports what it
         *  is playing in words and not in catalog ids, so the name has to be
         *  resolved to a page here; a name nothing matches lands on the
         *  search results for it, which is the honest fallback. */
        openArtistNamed?: string;
        /** Which pane to arrive on, when the way in named one. The dashboard
         *  band's sleep chip is the case this exists for: it states a fact
         *  about the featured room whose controls live on the Rooms pane, so
         *  the tap on it has to land there rather than on the search box a
         *  step short of it. Empty means the depth's own default. */
        openPane?: string;
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

    // ── Searching takes the screen ───────────────────────────────────────
    // The results lived in the left half of a two-column body: a 556px
    // column on the reference wall, which is eight rows of a list that is
    // read from across a room — while a 420px player sat beside it showing
    // a record nobody is looking at, because looking at a record is not
    // what someone typing is doing.
    //
    // Searching is a different job from listening, so the surface changes
    // job with it. The moment the box has the caret the player column folds
    // down into a dock along the foot and the results take the whole width
    // of the wall, in two columns — twice the answers, at the same size,
    // for the gesture this depth exists for. Nothing is lost: the dock
    // keeps the transport and what is playing, the room chips never move
    // (they are on the header, and where a tap plays must not change
    // because a search started), and the dock itself is the way back.
    //
    // It ends where the *searching* ends — Done, Escape, or opening an
    // artist's page — and never on a play. Choosing three songs in a row is
    // one search, and a wall that reflowed after each one would move the
    // next target out from under the finger.
    let searching = $state(false);

    function endSearch() {
        searching = false;
        searchEl?.blur();
    }

    function showPane(p: Pane) {
        pane = p;
        // Stepping over to the queue is not searching any more. Coming back
        // finds the box exactly as it was left — the query survives, only
        // the layout gives way.
        if (p !== "search") endSearch();
    }

    /** The one thing queueing changes that can be seen. It normally rides
     *  on the player card, which is exactly what the dock replaced — and
     *  queueing from a search row is the likeliest thing to happen in this
     *  mode, so the dock has to carry it or the tap answers with nothing. */
    const QUEUED_MS = 5000;
    const queuedLine = $derived.by(() => {
        void clock.beat;
        const q = music.lastQueued;
        if (!q || Date.now() - q.at > QUEUED_MS) return null;
        return `${q.next ? "Playing next" : "Added to the queue"} — ${q.title}`;
    });

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

    /** The catalog's own pages are column-shaped and belong beside the
     *  player, and the other two panes were never the reason the results
     *  column was short. So the full-bleed layout is the Search pane's
     *  alone, and only while it is showing results rather than a page. */
    const fullBleed = $derived(searching && pane === "search" && !catalogOpen);

    // The pages themselves are read and kept by the shared cache; the stack
    // above stays this pane's, since only it knows what a level looks like
    // and where it was scrolled.
    const catalog = createCatalogCache({
        artistUri: () => (topLevel?.kind === "artist" ? topLevel.uri : null),
        contextUri: () => (topLevel?.kind === "context" ? topLevel.uri : null),
        onFail: () => void popLevel(),
    });

    // Scroll follows the level: pushing stashes where the outgoing list
    // was, popping puts it back — the search results count as level zero.
    let resultsEl = $state<HTMLElement | null>(null);
    let stackEl = $state<HTMLElement | null>(null);
    let searchScroll = 0;

    function pushLevel(kind: Level["kind"], uri: string) {
        if (stack.length === 0) searchScroll = resultsEl?.scrollTop ?? 0;
        else stack[stack.length - 1].scroll = stackEl?.scrollTop ?? 0;
        stack = [...stack, { kind, uri, scroll: 0 }];
        // A page opened is a question answered: the typing is over, and the
        // player comes back beside the discography being read.
        searching = false;
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
        await catalog.loadArtist(uri);
    }

    /** Resolve a name to an artist page. The panel arrives here from the
     *  full player, where all that is known about who is playing is what
     *  the speaker said. */
    let resolvedName = "";
    async function openArtistByName(name: string) {
        pane = "search";
        try {
            const res = await api.spotifySearch(name, 5, { kind: "artists" });
            const hit = res.artists?.[0];
            if (hit) {
                await openArtist(hit.uri, { art_url: hit.art_url, round: true });
                return;
            }
        } catch {
            // Fall through: the results for the name are still an answer,
            // and a failed lookup should never leave the wall on nothing.
        }
        spotify.runQuery(name);
    }

    $effect(() => {
        const name = openArtistNamed.trim();
        if (!name || name === resolvedName) return;
        resolvedName = name;
        void openArtistByName(name);
    });

    // Arriving on a named pane. Once, on the way in: the switcher above is
    // the pane's owner from then on, and a route parameter that kept
    // reasserting itself would be a segmented control that snapped back.
    onMount(() => {
        if (openPane === "queue" || openPane === "rooms") showPane(openPane);
    });

    async function openContext(uri: string) {
        if (topLevel?.kind === "context" && topLevel.uri === uri) return;
        pushLevel("context", uri);
        await catalog.loadContext(uri);
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
            // The full-bleed search is a rung of the same ladder: Escape
            // gives the player its column back before it leaves the depth.
            if (searching) {
                endSearch();
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

    // On a touch wall the box arrives unfocused (above) and *looks* exactly
    // like it would once tapped — nothing about a bordered box reading
    // "Search songs, artists, albums" says the keyboard hasn't been asked
    // for yet. So the placeholder itself carries that: idle, it reads as an
    // instruction rather than a finished sentence, and the moment the box
    // is actually focused it switches to naming what to type, same as it
    // always has for a pointing device that arrived pre-focused.

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
    const sections = $derived(searchSections(spotify.results, spotify.kindFilter));

    /** The artist the query names, pulled out of the shelf it would
     *  otherwise sit in.
     *
     *  Songs lead the shelves and artists close them, which is right for
     *  "play this song" and wrong for the other thing people type into a
     *  search box: a name. Typing "adele" answered with ten songs, ten
     *  albums and ten playlists before her page — and while the keyboard
     *  is up the kind chips, the labels and the top-result card are all
     *  folded away, so there was no way to her at all without dismissing
     *  the keyboard first. So the artist being named rides at the top of
     *  the results, one row, saying what it is.
     *
     *  Not when a chip has narrowed the list — that is an explicit choice
     *  about what to see — and not when the top-result card is already
     *  showing that same artist at full size. */
    const artistLead = $derived.by(() => {
        if (spotify.kindFilter !== "all") return null;
        const a = spotify.artistMatch;
        if (!a) return null;
        if (!kbOpen && spotify.topResult?.uri === a.uri) return null;
        return a;
    });

    /** What a row says under its name — different per kind, because what
     *  makes each one worth choosing is different. */
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


    // ── Favorites: the household's own list ──────────────────────────────
    // Radio stations and whatever was starred in the Sonos app. On the wall
    // they matter twice over: a station is a one-tap job search can't be,
    // and without a linked Spotify account they are the only thing this
    // depth could start at all. Sonos-only, because the list is.
    const favTarget = $derived(featured?.kind === "sonos");
    const favorites = $derived(favTarget ? music.favorites : []);

    // ── This room's own memory ───────────────────────────────────────────
    // The shelf the band has had since it grew one, now on the depth too —
    // and ranked rather than listed, because "what does this room keep
    // coming back to" is a better answer to *put something on* than "what
    // was on last". The label never claims more than the answer carries:
    // a habit at this hour says the hour, a ranking without one says
    // "most", and the household's fallback says it is the household's.
    const roomPlays = $derived(music.topPlays.length > 0 ? music.topPlays : music.history);
    const roomPlaysLabel = $derived(
        music.topPlays.length > 0
            ? music.topPlaysByHour
                ? `Played here around ${fmtHour(music.topPlaysHour)}`
                : "Played here most"
            : music.historyHousehold
              ? "Played recently in this house"
              : "Played here",
    );
    /** The plain list is the only one that can be somebody else's room. */
    const roomPlaysHousehold = $derived(music.topPlays.length === 0 && music.historyHousehold);

</script>

<div
    class="browse"
    class:kb-open={kbOpen}
    class:full={fullBleed}
    style:--kb="{kb}px"
    in:fade={{ duration: dur(160) }}
>
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
                onclick={() => showPane("search")}
            >
                Search
            </button>
            <button
                class="p-pane"
                class:active={pane === "queue"}
                aria-pressed={pane === "queue"}
                onclick={() => showPane("queue")}
            >
                Queue{#if featured?.kind === "sonos" && queueCount > 0}
                    <span class="mono">{queueCount}</span>{/if}
            </button>
            <button
                class="p-pane"
                class:active={pane === "rooms"}
                aria-pressed={pane === "rooms"}
                onclick={() => showPane("rooms")}
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
                            artist={catalog.artistDetail}
                            loading={catalog.artistLoading}
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
                            context={catalog.contextDetail}
                            loading={catalog.contextLoading}
                            destination={catalogDest}
                            busy={catalogBusy}
                            targetRow={playOnRow}
                            onBack={popLevel}
                            onPlayAll={() => catalog.contextDetail && pick(contextItem(catalog.contextDetail))}
                            onPick={pick}
                            onEnqueue={(item, next) => music.enqueue(item, next)}
                            onOpenArtist={(uri) => void openArtist(uri)}
                            bind:this={contextScr}
                        />
                    {/if}
                </div>
            {:else if pane === "search"}
                <div class="b-search">
                    <PanelSearchBox
                        {spotify}
                        {featured}
                        {fullBleed}
                        {kbOpen}
                        {liveMessage}
                        bind:searchEl
                        onTyping={() => (searching = true)}
                        {onQueryKey}
                        onClear={clearQuery}
                        onDone={endSearch}
                    />
                    <p class="sr-only" role="status" aria-live="polite">{liveMessage}</p>

                    <PanelSearchResults
                        {music}
                        {spotify}
                        {recents}
                        {featured}
                        {favorites}
                        {sections}
                        {artistLead}
                        {roomPlays}
                        {roomPlaysLabel}
                        {roomPlaysHousehold}
                        {booted}
                        {kbOpen}
                        {fullBleed}
                        bind:resultsEl
                        onPick={pick}
                        onOpenArtist={(uri, art) => void openArtist(uri, art)}
                        onRunRecent={runRecent}
                    />
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
                <PanelBrowseRooms
                    {music}
                    {featured}
                    onArtist={(name) => void openArtistByName(name)}
                />
            {/if}
        </section>

        {#if fullBleed}
            <!-- The player, folded to a strip along the foot while the
                 results have the wall. Everything the column was carrying
                 that a search needs is still here — what is on, whether it
                 is playing, what just went into the queue — and the cover is
                 the way back to the column, the same bargain the band's
                 cover makes one depth out. -->
            <!-- In only, and it is a rule rather than an omission (§16's
                 motion budget): on the way out the body is already back to
                 two columns, so a dock still holding a grid row for the
                 length of an exit would squeeze the results for 180ms.
                 Animate what arrives; let what leaves go. -->
            <PanelSearchDock
                {music}
                {featured}
                {kbOpen}
                {queuedLine}
                onBack={endSearch}
            />
        {:else}
            <!-- Same rule the other way: the column fades in as it takes
                 its 420px back, and is gone in a frame when the search
                 claims them. One element, opacity only. -->
            <section class="b-player" aria-label="Now playing" in:fade={{ duration: dur(140) }}>
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
        {/if}
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

<style>
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
    /* Searching: one column, and a dock along the foot. The results get the
       whole width of the wall for as long as the box has the caret, and the
       player becomes the strip it can be while nobody is looking at it. */
    .browse.full .b-body {
        grid-template-columns: minmax(0, 1fr);
        grid-template-rows: minmax(0, 1fr) auto;
    }
    /* The reflow into two columns is one layout pass and it is not worth
       animating — a grid-template that tweens is a layout on every frame,
       which is exactly what an A8X cannot spend (§16). The list fades over
       its own reflow instead: one element, opacity only, and the eye reads
       the whole thing as the results taking the screen rather than as rows
       jumping sideways. */
@keyframes results-widen {

        from {
            opacity: 0.4;
        }
        to {
            opacity: 1;
        }
}

    .b-left {
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        min-height: 0;
        min-width: 0;
        padding: var(--space-5) var(--space-6);
    }

    /* Type mode's dense results: single-line rows and nothing that isn't
       a match — the kind chips and the destination line return with the
       keyboard, because filtering is what the choosing phase is for.

       The arithmetic this is answering: the reference wall is 768px tall
       and its docked landscape keyboard takes ~350 of them. What is left
       has to hold the header (72), the box (48) and the dock, and
       everything else is the list. Every rule below is a row bought back
       out of that. */
    .kb-open .s-label {
        display: none;
    }
    /* Typing on the wall, with the results across the whole of it: the two
       modes compose, and where they meet the width buys back what the
       keyboard took. The rows' own half of it travels with the row
       (PanelResultRow); what stays here is the shelf label. */
    /* And the shelf labels come back, tight. In one column the sections
       arrive in a known order and a row's own shape says what it is; dealt
       into two columns, an album and a song look alike and the label is
       the only thing that separates them. 24px for that is a fair trade
       where a whole row is not. */
    .browse.full.kb-open .s-label {
        display: block;
        margin: var(--space-2) 0 4px;
    }
    /* A newer search running behind the list: it stays and dims, and stops
       taking taps — a row tapped while it is being replaced would play
       whatever landed in its place. */

    /* One shelf's rows. Stacked in the two-column body, where the results
       column is 556px and a row is a row; dealt into columns once the
       search has the whole wall, because at 1024px a single column of
       64px rows spends half the screen on margin and still shows eight
       answers. The floor is stated in the row's own terms: below ~360px a
       title and its artist stop fitting on one line, and a shelf of
       truncated names is worse than a shorter list. */

    /* The search didn't get through. On a wall the retry has to be a target
       rather than a sentence with a link in it. */
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

    /* Recent searches: a chip cloud, distance-scaled like everything else
       on the wall — one tap re-runs, the × forgets one, Clear forgets the
       room's list. */
    /* The query's own top result, once the search behind it has answered —
       round for an artist's photo, square for everything else's cover art. */

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
    /* Every shelf of covers on this pane uses it — the room's own plays,
       the household's favorites, the playlists, the albums, the artists,
       what came out this week. One grid rather than one per shelf, so a
       cover is the same size wherever the wall offers one, and the column
       count follows the width instead of being stated: four on the depth's
       556px column, eight once the search takes the wall. */


    .s-empty {
        height: 100%;
        display: grid;
        place-items: center;
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
       so the depth's own controls clear 44px rather than inheriting a
       phone's sizing. The chip carries its own floor from app.css now that
       every pane draws one. */
@media (pointer: coarse) {

        .back {
            width: 44px;
            height: 44px;
        }
        .p-pane {
            min-height: 44px;
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
        .b-pane,
        .b-stack {
            overflow: visible;
            flex: none;
        }
}
</style>
