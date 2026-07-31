<script lang="ts">
    /**
     * The kid music search (DESIGN.md §17): the same catalog the grown-up
     * panel searches, spoken kid — big art, big words, one obvious thing to
     * tap. A song plays outright; an album or playlist plays whole; an
     * artist opens their own page one level in (popular songs, albums, more
     * like them), and a record goes one level deeper still. Back climbs one
     * level, never out of the module.
     *
     * While the box is empty the pane idles on the room's recent searches
     * (shared with the wall and the phone — one per-room history, keyed
     * `sonos:{coordinator}` like the app) and the account's playlists as a
     * cover grid. Queueing without interrupting lives behind a row's big ＋
     * — "Play next" or "Add to the end" — and says so with a 🎉, since a
     * kid can't watch a count change three taps away.
     *
     * The box knows when the software keyboard is up (visualViewport) and
     * the results go dense for it — no sub-lines, no shelf labels — and
     * typing ends with the keyboard leaving: Enter, or a tap on a result.
     */
    import { onMount } from "svelte";
    import { api } from "../../lib/api";
    import { toasts } from "../../lib/stores.svelte";
    import { createSpotify } from "../../lib/music/spotify.svelte";
    import { createSearchHistory } from "../../lib/music/history.svelte";
    import { fmtCount, fmtMs, capFirst } from "../../lib/music/format";
    import { haptic } from "../../lib/utils";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { SpotifyArtistDetail, SpotifyContextDetail, SpotifyItem } from "../../lib/types";

    let { music }: { music: PanelMusicStore } = $props();

    // Recent searches, keyed by the featured room with the same key format
    // the app uses — a search run here lands in the same per-room history
    // as one run from the wall or a phone. Kid sources are always Sonos.
    const recents = createSearchHistory(() => {
        const f = music.featured;
        return f ? `sonos:${f.id}` : null;
    });
    const spotify = createSpotify((q, art) => recents.add(q, art));
    let booted = $state(false);
    onMount(() => {
        void spotify.load().finally(() => {
            booted = true;
        });
    });

    const KIND_EMOJI: Record<string, string> = {
        track: "🎵",
        album: "💿",
        playlist: "📃",
        artist: "🎤",
    };

    // ── The box behaves like a search box ────────────────────────────────
    let searchEl = $state<HTMLInputElement | null>(null);

    function onQueryKey(e: KeyboardEvent) {
        if (e.key === "Enter") {
            e.preventDefault();
            spotify.runNow();
            // Submitted — typing is over, so the keyboard leaves with it.
            searchEl?.blur();
        } else if (e.key === "Escape" && spotify.query) {
            e.stopPropagation();
            spotify.clearQuery();
            searchEl?.focus();
        }
    }
    function clearQuery() {
        haptic();
        spotify.clearQuery();
        searchEl?.focus();
    }
    function runRecent(q: string) {
        haptic();
        spotify.runQuery(q);
        // Chosen by chip, so typing is over: the software keyboard leaves
        // on touch; a real keyboard keeps the caret for the next word.
        if (window.matchMedia("(pointer: fine)").matches) searchEl?.focus();
        else searchEl?.blur();
    }

    // ── Type mode: dense results while the software keyboard is up ──────
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

    // ── Results ──────────────────────────────────────────────────────────
    const KINDS = [
        { id: "tracks", label: "Songs", emoji: "🎵" },
        { id: "albums", label: "Albums", emoji: "💿" },
        { id: "playlists", label: "Playlists", emoji: "📃" },
        { id: "artists", label: "Artists", emoji: "🎤" },
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

    const KIND_WORD: Record<string, string> = {
        artist: "Artist",
        album: "Album",
        playlist: "Playlist",
        track: "Song",
    };

    /** The top result's own line — the kind first, then the one stat that
     *  identifies it fastest. */
    function topLine(item: SpotifyItem): string {
        const bits = [KIND_WORD[item.kind]];
        if (item.kind === "artist") {
            if (item.followers) bits.push(`${fmtCount(item.followers)} followers`);
        } else {
            if (item.sub) bits.push(item.sub);
            if (item.year) bits.push(item.year);
            if (item.duration_ms) bits.push(fmtMs(item.duration_ms));
            if (item.total_tracks) bits.push(`${item.total_tracks} songs`);
        }
        return bits.join(" · ");
    }

    /** A search that led somewhere is worth remembering: the store remembers
     *  submissions (Enter, chip re-runs), but the kid flow is type → tap a
     *  result with no Enter in between, so acting on one remembers the query
     *  behind it too — picture included, straight from the row tapped. */
    function actedOnResult(art?: { art_url?: string; round?: boolean }) {
        const q = spotify.query.trim();
        if (q) recents.add(q, art);
    }

    // A tap plays a song or a whole container; an artist opens instead.
    function act(item: SpotifyItem) {
        if (item.kind === "artist") {
            void openArtist(item.uri, item.art_url ? { art_url: item.art_url, round: true } : undefined);
        } else {
            pick(item);
        }
    }

    function pick(item: SpotifyItem) {
        actedOnResult(item.art_url ? { art_url: item.art_url, round: false } : undefined);
        searchEl?.blur(); // chosen — the keyboard's job is done
        if (!music.featured) {
            toasts.error("Oops!", "No speaker is reachable right now.");
            return;
        }
        void music.playItem(item);
    }

    // ── Queue without interrupting: the row's big ＋ ─────────────────────
    let queueFor = $state<string | null>(null);
    let queuedFlash = $state<string | null>(null);
    let flashTimer: ReturnType<typeof setTimeout> | undefined;

    function toggleQueueFor(uri: string) {
        haptic();
        queueFor = queueFor === uri ? null : uri;
    }
    function queueIt(item: SpotifyItem, next: boolean) {
        haptic();
        music.enqueue(item, next);
        queueFor = null;
        // The queue is a pane away and the count change is quiet — say it.
        queuedFlash = item.uri;
        clearTimeout(flashTimer);
        flashTimer = setTimeout(() => (queuedFlash = null), 1800);
    }

    // ── The catalog stack: artist and record pages, one level deeper ────
    type Level = { kind: "artist" | "context"; uri: string };
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

    function pushLevel(kind: Level["kind"], uri: string) {
        stack = [...stack, { kind, uri }];
    }
    function popLevel() {
        haptic();
        stack = stack.slice(0, -1);
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
            toasts.error("Couldn't load the artist", (e as Error).message);
            if (artistUri === uri) popLevel();
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
            if (contextUri === uri) popLevel();
        } finally {
            if (contextLoadingUri === uri) contextLoadingUri = null;
        }
    }

    /** The opened record as an item, for the big "Play all" — the same call
     *  a tap on its search card makes. */
    function contextItem(c: SpotifyContextDetail): SpotifyItem {
        return { kind: c.kind, uri: c.uri, name: c.name, sub: c.sub, art_url: c.art_url };
    }

    // Escape climbs the ladder: queue actions, then each level, then the
    // box's query (handled on the input itself).
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (queueFor) {
                queueFor = null;
                return;
            }
            if (stack.length) popLevel();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    });
</script>

{#snippet trackRow(item: SpotifyItem, num: number | null)}
    <!-- The one song row for every list in the kid module: tap plays it,
         the big ＋ queues it without stopping what's on. -->
    <div class="kms-row">
        <button
            class="kms-main"
            disabled={!!music.busy["item:" + item.uri]}
            onclick={() => pick(item)}
        >
            {#if num !== null}
                <span class="kms-num mono">{num}</span>
            {/if}
            {#if item.art_url}
                <img class="kms-art" src={item.art_url} alt="" loading="lazy" />
            {:else}
                <span class="kms-art kms-art-none" aria-hidden="true">🎵</span>
            {/if}
            <span class="kms-names">
                <span class="kms-name">{item.name}</span>
                {#if sub(item)}
                    <span class="kms-sub">{sub(item)}</span>
                {/if}
            </span>
            {#if item.duration_ms}
                <span class="kms-dur mono">{fmtMs(item.duration_ms)}</span>
            {/if}
        </button>
        <button
            class="kms-plus"
            class:open={queueFor === item.uri}
            aria-label="Queue “{item.name}”"
            aria-expanded={queueFor === item.uri}
            onclick={() => toggleQueueFor(item.uri)}
        >
            ＋
        </button>
    </div>
    {#if queueFor === item.uri}
        <div class="kms-qactions" role="group" aria-label="Queue options">
            <button class="kms-qbtn" onclick={() => queueIt(item, true)}>▶️ Play next</button>
            <button class="kms-qbtn" onclick={() => queueIt(item, false)}>➕ Add to the end</button>
        </div>
    {/if}
    {#if queuedFlash === item.uri}
        <p class="kms-flash" role="status">Added to the queue 🎉</p>
    {/if}
{/snippet}

{#snippet mediaCard(item: SpotifyItem)}
    <!-- The one cover card: albums, playlists and artists are chosen by
         their picture as much as their name. -->
    <button class="kms-card" onclick={() => act(item)}>
        {#if item.art_url}
            <img class="kms-card-art" class:round={item.kind === "artist"} src={item.art_url} alt="" loading="lazy" />
        {:else}
            <span class="kms-card-art kms-card-none" class:round={item.kind === "artist"} aria-hidden="true">
                {KIND_EMOJI[item.kind]}
            </span>
        {/if}
        <span class="kms-card-name">{item.name}</span>
        {#if sub(item)}
            <span class="kms-card-sub">{sub(item)}</span>
        {/if}
    </button>
{/snippet}

{#snippet shelfLabel(text: string)}
    <h3 class="kms-label">{text}</h3>
{/snippet}

<div class="kms" class:kb-open={kbOpen}>
    {#if topLevel}
        <!-- One level deeper: an artist's page or a record's listing. The
             back button climbs exactly one level. -->
        <button class="kms-back" onclick={popLevel} aria-label="Back one level">‹ Back</button>

        {#if topLevel.kind === "artist"}
            {#if artistLoading && !artistDetail}
                <div class="kms-skel-hero" aria-hidden="true"></div>
            {:else if artistDetail}
                {@const d = artistDetail}
                <header class="kms-hero">
                    {#if d.art_url}
                        <img class="kms-hero-art round" src={d.art_url} alt="" />
                    {:else}
                        <span class="kms-hero-art kms-card-none round" aria-hidden="true">🎤</span>
                    {/if}
                    <span class="kms-hero-name">{d.name}</span>
                    {#if d.followers || d.genres?.length}
                        <span class="kms-hero-sub">
                            {[d.followers ? `${fmtCount(d.followers)} followers` : "", d.genres?.[0] ? capFirst(d.genres[0]) : ""]
                                .filter(Boolean)
                                .join(" · ")}
                        </span>
                    {/if}
                    {#if d.top_tracks[0]}
                        <!-- No speaker takes an artist URI, so the button
                             starts their top song — and names it. -->
                        <button
                            class="kms-bigplay"
                            disabled={!!music.busy["item:" + d.uri]}
                            onclick={() => pick({ kind: "artist", uri: d.uri, name: d.name, art_url: d.art_url })}
                        >
                            ▶️ Play
                        </button>
                        <span class="kms-playnote">Plays “{d.top_tracks[0].name}”</span>
                    {/if}
                </header>

                {#if d.top_tracks.length > 0}
                    {@render shelfLabel("🎵 Popular songs")}
                    {#each d.top_tracks as t, i (t.uri)}
                        {@render trackRow(t, i + 1)}
                    {/each}
                {/if}
                {#if d.albums.length > 0}
                    {@render shelfLabel("💿 Albums")}
                    <div class="kms-grid">
                        {#each d.albums as a (a.uri)}
                            <button class="kms-card" onclick={() => void openContext(a.uri)}>
                                {#if a.art_url}
                                    <img class="kms-card-art" src={a.art_url} alt="" loading="lazy" />
                                {:else}
                                    <span class="kms-card-art kms-card-none" aria-hidden="true">💿</span>
                                {/if}
                                <span class="kms-card-name">{a.name}</span>
                                {#if sub(a)}
                                    <span class="kms-card-sub">{sub(a)}</span>
                                {/if}
                            </button>
                        {/each}
                    </div>
                {/if}
                {#if d.singles.length > 0}
                    {@render shelfLabel("💿 Singles")}
                    <div class="kms-grid">
                        {#each d.singles as sg (sg.uri)}
                            <button class="kms-card" onclick={() => void openContext(sg.uri)}>
                                {#if sg.art_url}
                                    <img class="kms-card-art" src={sg.art_url} alt="" loading="lazy" />
                                {:else}
                                    <span class="kms-card-art kms-card-none" aria-hidden="true">💿</span>
                                {/if}
                                <span class="kms-card-name">{sg.name}</span>
                                {#if sub(sg)}
                                    <span class="kms-card-sub">{sub(sg)}</span>
                                {/if}
                            </button>
                        {/each}
                    </div>
                {/if}
                {#if d.related.length > 0}
                    {@render shelfLabel("🎤 More like them")}
                    <div class="kms-grid">
                        {#each d.related as ra (ra.uri)}
                            <button class="kms-card" onclick={() => void openArtist(ra.uri, ra.art_url ? { art_url: ra.art_url, round: true } : undefined)}>
                                {#if ra.art_url}
                                    <img class="kms-card-art round" src={ra.art_url} alt="" loading="lazy" />
                                {:else}
                                    <span class="kms-card-art kms-card-none round" aria-hidden="true">🎤</span>
                                {/if}
                                <span class="kms-card-name">{ra.name}</span>
                            </button>
                        {/each}
                    </div>
                {/if}
            {/if}
        {:else}
            {#if contextLoading && !contextDetail}
                <div class="kms-skel-hero" aria-hidden="true"></div>
            {:else if contextDetail}
                {@const d = contextDetail}
                <header class="kms-hero">
                    {#if d.art_url}
                        <img class="kms-hero-art" src={d.art_url} alt="" />
                    {:else}
                        <span class="kms-hero-art kms-card-none" aria-hidden="true">{KIND_EMOJI[d.kind]}</span>
                    {/if}
                    <span class="kms-hero-name">{d.name}</span>
                    <span class="kms-hero-sub">
                        {[d.sub, d.year, d.total_tracks ? `${d.total_tracks} songs` : ""].filter(Boolean).join(" · ")}
                    </span>
                    <button
                        class="kms-bigplay"
                        disabled={!!music.busy["item:" + d.uri]}
                        onclick={() => pick(contextItem(d))}
                    >
                        ▶️ Play all
                    </button>
                </header>

                {@render shelfLabel("🎵 Songs")}
                {#each d.tracks as t, i (t.uri)}
                    {@render trackRow(t, d.kind === "album" ? i + 1 : null)}
                {/each}
            {/if}
        {/if}
    {:else}
        <div class="kms-box">
            <span class="kms-box-emoji" aria-hidden="true">🔎</span>
            <input
                bind:this={searchEl}
                value={spotify.query}
                placeholder="Find a song…"
                aria-label="Find a song"
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
                <button class="kms-clear" aria-label="Clear search" onclick={clearQuery}>✕</button>
            {/if}
        </div>

        {#if spotify.results}
            {@const r = spotify.results}
            <div class="kms-kinds" role="group" aria-label="Show only">
                <button
                    class="kms-kind"
                    class:active={spotify.kindFilter === "all"}
                    onclick={() => (spotify.kindFilter = "all")}>All</button>
                {#each KINDS as k (k.id)}
                    {#if r[k.id].length > 0}
                        <button
                            class="kms-kind"
                            class:active={spotify.kindFilter === k.id}
                            onclick={() => (spotify.kindFilter = k.id)}
                            >{k.emoji} {k.label} <span class="mono">{r[k.id].length}</span></button>
                    {/if}
                {/each}
            </div>
        {/if}

        {#if !booted || spotify.searching}
            <div class="kms-sklist" aria-hidden="true">
                {#each Array(4) as _, i (i)}
                    <div class="kms-skel"></div>
                {/each}
            </div>
        {:else if !spotify.connected}
            <div class="kms-empty">
                <div class="kms-empty-emoji">🎵</div>
                <p>Music isn't set up yet —<br />ask a grown-up!</p>
            </div>
        {:else if spotify.results}
            {#if sections.length === 0}
                <div class="kms-empty">
                    <div class="kms-empty-emoji">🙈</div>
                    <p>Nothing found for “{spotify.query}”<br />Try another name!</p>
                </div>
            {:else}
                {#if !kbOpen && spotify.kindFilter === "all" && spotify.topResult}
                    <!-- The one thing this search was almost certainly
                         after, at full size. Type mode folds it back into
                         the shelves: at one row tall the card would be the
                         only thing visible. -->
                    {@const top = spotify.topResult}
                    {@render shelfLabel("⭐ Best match")}
                    <button class="kms-top" onclick={() => act(top)}>
                        {#if top.art_url}
                            <img class="kms-top-art" class:round={top.kind === "artist"} src={top.art_url} alt="" />
                        {:else}
                            <span class="kms-top-art kms-card-none" class:round={top.kind === "artist"} aria-hidden="true">
                                {KIND_EMOJI[top.kind]}
                            </span>
                        {/if}
                        <span class="kms-top-meta">
                            <span class="kms-top-name">{top.name}</span>
                            <span class="kms-top-line">{topLine(top)}</span>
                            {#if top.kind === "artist"}
                                <span class="kms-top-cta">See songs & albums ›</span>
                            {:else}
                                <span class="kms-top-cta">Tap to play ▶️</span>
                            {/if}
                        </span>
                    </button>
                {/if}

                {#each sections as sec (sec.id)}
                    {@render shelfLabel(`${sec.emoji} ${sec.label}`)}
                    {#if sec.id === "tracks"}
                        {#each sec.items as item (item.uri)}
                            {@render trackRow(item, null)}
                        {/each}
                    {:else}
                        <!-- Songs are a list; everything else is a grid —
                             a container is chosen by its cover. -->
                        <div class="kms-grid">
                            {#each sec.items as item (item.uri)}
                                {@render mediaCard(item)}
                            {/each}
                        </div>
                    {/if}
                {/each}
            {/if}
        {:else}
            <!-- Idle: the room's recent searches, then the account's
                 playlists as a cover grid. -->
            {#if recents.list.length > 0}
                <div class="kms-shelf-head">
                    {@render shelfLabel("🕘 Recent searches")}
                    <button class="kms-clearall" onclick={() => recents.clear()}>Clear</button>
                </div>
                <div class="kms-recents">
                    {#each recents.list as h (h.q)}
                        <span class="kms-recent">
                            <button class="kms-recent-run" onclick={() => runRecent(h.q)}>
                                {#if h.art_url}
                                    <img class="kms-recent-art" class:round={h.round} src={h.art_url} alt="" />
                                {:else}
                                    <span aria-hidden="true">🔎</span>
                                {/if}
                                <span>{h.q}</span>
                            </button>
                            <button
                                class="kms-recent-x"
                                aria-label="Forget “{h.q}”"
                                onclick={() => recents.remove(h.q)}
                            >✕</button>
                        </span>
                    {/each}
                </div>
            {/if}

            {#if spotify.myPlaylists.length > 0}
                {@render shelfLabel("📃 Your playlists")}
                <div class="kms-grid">
                    {#each spotify.myPlaylists as item (item.uri)}
                        {@render mediaCard(item)}
                    {/each}
                </div>
            {/if}

            {#if recents.list.length === 0 && spotify.myPlaylists.length === 0}
                <div class="kms-empty">
                    <div class="kms-empty-emoji">🔎</div>
                    <p>Type a song or a singer<br />to find them!</p>
                </div>
            {/if}
        {/if}
    {/if}
</div>

<style>
    .kms { display: flex; flex-direction: column; gap: var(--space-3); }

    /* ── Search box ── */
    .kms-box {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 0 var(--space-4);
        min-height: 60px;
        border-radius: 999px;
        border: 3px solid var(--border);
        background: var(--bg-elevated);
    }
    .kms-box:focus-within { border-color: var(--kid-accent); }
    .kms-box-emoji { font-size: 1.3rem; flex-shrink: 0; }
    .kms-box input {
        flex: 1;
        min-width: 0;
        border: none;
        outline: none;
        background: transparent;
        color: var(--text);
        /* 18px: comfortably over the 16px that stops iOS auto-zoom. */
        font-size: 1.125rem;
        font-weight: 700;
        padding: 16px 0;
    }
    .kms-box input::placeholder { color: var(--text-faint); }
    .kms-clear {
        width: 44px;
        height: 44px;
        border-radius: 50%;
        border: none;
        background: var(--surface-hover);
        color: var(--text-muted);
        font-size: 1rem;
        font-weight: 800;
        cursor: pointer;
        flex-shrink: 0;
    }
    .kms-clear:active { transform: scale(0.9); }

    /* ── Kind chips ── */
    .kms-kinds {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .kms-kind {
        font-size: 0.95rem;
        font-weight: 800;
        padding: 10px 16px;
        min-height: 46px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--text-muted);
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-kind:active { transform: scale(0.94); }
    .kms-kind.active {
        background: var(--kid-accent-grad);
        border-color: var(--kid-accent);
        color: var(--kid-on-text);
    }

    /* ── Shelf labels ── */
    .kms-label {
        font-size: 1.05rem;
        font-weight: 800;
        letter-spacing: -0.01em;
        color: var(--text);
        margin-top: var(--space-3);
    }
    .kms-shelf-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .kms-clearall {
        font-size: 0.9rem;
        font-weight: 800;
        padding: 8px 16px;
        min-height: 44px;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: transparent;
        color: var(--text-muted);
        cursor: pointer;
    }
    .kms-clearall:active { transform: scale(0.94); }

    /* ── Track rows ── */
    .kms-row {
        display: flex;
        align-items: stretch;
        gap: var(--space-2);
    }
    .kms-main {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        min-height: 64px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-main:active { transform: scale(0.98); border-color: var(--kid-accent); }
    .kms-main:disabled { opacity: 0.6; }
    .kms-num {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-faint);
        width: 2ch;
        text-align: right;
        flex-shrink: 0;
    }
    .kms-art {
        width: 48px;
        height: 48px;
        border-radius: var(--radius-md);
        object-fit: cover;
        flex-shrink: 0;
    }
    .kms-art-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 1.4rem;
    }
    .kms-names {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
    }
    .kms-name {
        font-size: 1rem;
        font-weight: 800;
        color: var(--text);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kms-sub {
        font-size: 0.82rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .kms-dur {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.85rem;
        font-weight: 700;
        color: var(--text-muted);
        flex-shrink: 0;
    }
    .kms-plus {
        width: 52px;
        min-height: 52px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        color: var(--kid-accent);
        font-size: 1.4rem;
        font-weight: 800;
        cursor: pointer;
        flex-shrink: 0;
        align-self: center;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-plus:active { transform: scale(0.9); }
    .kms-plus.open { border-color: var(--kid-accent); background: var(--kid-accent-soft); }

    .kms-qactions {
        display: flex;
        gap: var(--space-2);
        padding-left: var(--space-3);
    }
    .kms-qbtn {
        flex: 1;
        font-size: 1rem;
        font-weight: 800;
        padding: 12px 16px;
        min-height: 52px;
        border-radius: var(--radius-lg);
        border: 2px solid var(--kid-accent);
        background: var(--kid-accent-soft);
        color: var(--kid-accent);
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-qbtn:active { transform: scale(0.96); }
    .kms-flash {
        font-size: 0.9rem;
        font-weight: 800;
        color: var(--kid-green);
        padding-left: var(--space-3);
    }

    /* ── Cover grids ── */
    .kms-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(min(140px, 45%), 1fr));
        gap: var(--space-3);
    }
    .kms-card {
        display: flex;
        flex-direction: column;
        align-items: stretch;
        gap: var(--space-2);
        padding: var(--space-3);
        border-radius: var(--radius-lg);
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-card:active { transform: scale(0.96); border-color: var(--kid-accent); }
    .kms-card-art {
        width: 100%;
        aspect-ratio: 1;
        border-radius: var(--radius-md);
        object-fit: cover;
    }
    .kms-card-art.round { border-radius: 50%; }
    .kms-card-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 2.6rem;
    }
    .kms-card-name {
        font-size: 0.95rem;
        font-weight: 800;
        color: var(--text);
        line-height: 1.2;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }
    .kms-card-sub {
        font-size: 0.78rem;
        font-weight: 600;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }

    /* ── Top result ── */
    .kms-top {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        padding: var(--space-4);
        border-radius: var(--radius-xl);
        border: 3px solid var(--kid-accent);
        background: var(--bg-elevated);
        box-shadow: 0 0 0 4px var(--kid-ring);
        cursor: pointer;
        text-align: left;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-top:active { transform: scale(0.98); }
    .kms-top-art {
        width: 92px;
        height: 92px;
        border-radius: var(--radius-lg);
        object-fit: cover;
        flex-shrink: 0;
    }
    .kms-top-art.round { border-radius: 50%; }
    .kms-top-meta {
        display: flex;
        flex-direction: column;
        gap: 4px;
        min-width: 0;
    }
    .kms-top-name {
        font-size: 1.25rem;
        font-weight: 800;
        letter-spacing: -0.02em;
        color: var(--text);
        line-height: 1.15;
    }
    .kms-top-line {
        font-size: 0.85rem;
        font-weight: 600;
        color: var(--text-muted);
    }
    .kms-top-cta {
        font-size: 0.9rem;
        font-weight: 800;
        color: var(--kid-accent);
        margin-top: 2px;
    }

    /* ── Recents ── */
    .kms-recents {
        display: flex;
        flex-wrap: wrap;
        gap: var(--space-2);
    }
    .kms-recent {
        display: inline-flex;
        align-items: center;
        border-radius: 999px;
        border: 2px solid var(--border);
        background: var(--bg-elevated);
        overflow: hidden;
    }
    .kms-recent-run {
        display: inline-flex;
        align-items: center;
        gap: var(--space-2);
        padding: 10px 6px 10px 14px;
        min-height: 48px;
        border: none;
        background: transparent;
        color: var(--text);
        font-size: 0.95rem;
        font-weight: 800;
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-recent-art {
        width: 26px;
        height: 26px;
        border-radius: 6px;
        object-fit: cover;
    }
    .kms-recent-art.round { border-radius: 50%; }
    .kms-recent-x {
        width: 44px;
        height: 44px;
        border: none;
        background: transparent;
        color: var(--text-muted);
        font-size: 0.9rem;
        font-weight: 800;
        cursor: pointer;
    }
    .kms-recent-x:active { color: var(--kid-pink); }

    /* ── Drill-down hero ── */
    .kms-back {
        align-self: flex-start;
        font-size: 1rem;
        font-weight: 800;
        padding: 12px 18px;
        min-height: 48px;
        border-radius: 999px;
        border: none;
        background: var(--surface-hover);
        color: var(--text);
        cursor: pointer;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-back:active { transform: scale(0.93); }
    .kms-hero {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-2);
        text-align: center;
        padding: var(--space-2) 0 var(--space-3);
    }
    .kms-hero-art {
        width: 132px;
        height: 132px;
        border-radius: var(--radius-xl);
        object-fit: cover;
        box-shadow: 0 10px 34px rgba(0, 0, 0, 0.35);
    }
    .kms-hero-art.round { border-radius: 50%; }
    .kms-hero-name {
        font-size: 1.5rem;
        font-weight: 800;
        letter-spacing: -0.02em;
        color: var(--text);
        line-height: 1.15;
    }
    .kms-hero-sub {
        font-size: 0.9rem;
        font-weight: 600;
        color: var(--text-muted);
    }
    .kms-bigplay {
        margin-top: var(--space-2);
        font-size: 1.15rem;
        font-weight: 800;
        padding: 16px 40px;
        min-height: 60px;
        border-radius: 999px;
        border: none;
        background: var(--kid-accent-grad);
        color: var(--kid-on-text);
        box-shadow: 0 0 0 4px var(--kid-ring), 0 10px 30px var(--kid-glow);
        cursor: pointer;
        transition: transform 0.12s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-bigplay:active { transform: scale(0.94); }
    .kms-bigplay:disabled { opacity: 0.6; }
    .kms-playnote {
        font-size: 0.85rem;
        font-weight: 600;
        color: var(--text-muted);
    }

    /* ── Skeletons & empties ── */
    .kms-sklist { display: flex; flex-direction: column; gap: var(--space-3); }
    .kms-skel {
        height: 64px;
        border-radius: var(--radius-lg);
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: kms-shimmer 1.5s linear infinite;
    }
    .kms-skel-hero {
        height: 220px;
        border-radius: var(--radius-xl);
        background: linear-gradient(90deg, var(--surface) 0%, var(--surface-hover) 50%, var(--surface) 100%);
        background-size: 200% 100%;
        animation: kms-shimmer 1.5s linear infinite;
    }
    @keyframes kms-shimmer {
        0% { background-position: 200% 0; }
        100% { background-position: -200% 0; }
    }
    .kms-empty {
        text-align: center;
        color: var(--text-muted);
        padding: var(--space-6) var(--space-4);
    }
    .kms-empty-emoji { font-size: 3rem; margin-bottom: var(--space-3); }
    .kms-empty p { font-size: 1.05rem; font-weight: 700; line-height: 1.5; }

    /* ── Type mode: the software keyboard is up, so results go dense —
         no sub-lines, no labels, no kind chips — and more matches fit. ── */
    .kb-open .kms-sub,
    .kb-open .kms-dur,
    .kb-open .kms-label,
    .kb-open .kms-kinds,
    .kb-open .kms-shelf-head { display: none; }
    .kb-open .kms-main { min-height: 52px; }
    .kb-open .kms-art { width: 36px; height: 36px; }
    .kb-open .kms-grid { grid-template-columns: repeat(auto-fill, minmax(min(110px, 30%), 1fr)); }

    @media (prefers-reduced-motion: reduce) {
        .kms-skel, .kms-skel-hero { animation: none; }
    }
</style>
