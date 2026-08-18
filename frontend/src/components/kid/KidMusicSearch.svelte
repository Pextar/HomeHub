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
     * The results go dense while the software keyboard is up — no sub-lines,
     * no shelf labels, and the box itself pins to the top of what's left of
     * the screen, since the pane chips stand down for the duration. Whether
     * the keyboard is up is measured once, by the view (KidMusic), because
     * the mini bar and the chips answer to it too. Typing ends with the
     * keyboard leaving: Enter, or a tap on a result.
     */
    import { onMount } from "svelte";
    import { toasts } from "../../lib/stores.svelte";
    import { createSpotify } from "../../lib/music/spotify.svelte";
    import { createSearchHistory } from "../../lib/music/history.svelte";
    import KidTrackRow from "./KidTrackRow.svelte";
    import KidMediaCard from "./KidMediaCard.svelte";
    import KidCatalogPage from "./KidCatalogPage.svelte";
    import { KIND_EMOJI } from "./kind-emoji";
    import { createCatalogStack } from "../../lib/music/catalog-stack.svelte";
    import { SEARCH_KINDS, searchSections, topLine } from "../../lib/music/catalog";
    import type { SearchKind } from "../../lib/music/catalog";
    import { haptic } from "../../lib/utils";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let { music, kbOpen = false }: { music: PanelMusicStore; kbOpen?: boolean } = $props();

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

    // ── Results ──────────────────────────────────────────────────────────
    // Shelved by the catalog's own rule, each shelf labelled with the emoji
    // for the kind it holds.
    const shelfEmoji = (id: SearchKind) =>
        KIND_EMOJI[SEARCH_KINDS.find((k) => k.id === id)!.kind];
    const sections = $derived(
        searchSections(spotify.results, spotify.kindFilter).map((s) => ({
            ...s,
            emoji: shelfEmoji(s.id),
        })),
    );

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
            void catalog.openArtist(
                item.uri,
                item.art_url ? { art_url: item.art_url, round: true } : undefined,
            );
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
    // The ladder itself is shared with the wall's depth; what is this
    // module's own is the haptic under each rung. Levels here don't scroll
    // independently, so there is no scroll to stash.
    const catalog = createCatalogStack({
        onOpened: (art) => {
            actedOnResult(art);
            searchEl?.blur(); // chosen — the keyboard's job is done
        },
        onPop: haptic,
        artistError: "Couldn't load the artist",
    });
    const topLevel = $derived(catalog.top);

    // Escape climbs the ladder: queue actions, then each level, then the
    // box's query (handled on the input itself).
    onMount(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (queueFor) {
                queueFor = null;
                return;
            }
            if (catalog.depth) void catalog.pop();
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    });
</script>

{#snippet shelfLabel(text: string)}
    <h3 class="kms-label">{text}</h3>
{/snippet}

<div class="kms" class:kb-open={kbOpen}>
    {#if topLevel}
        <KidCatalogPage
            {catalog}
            {music}
            {kbOpen}
            {queueFor}
            {queuedFlash}
            onPick={pick}
            onToggleQueue={toggleQueueFor}
            onQueue={queueIt}
            onAct={act}
        />
    {:else}
        <!-- The wrapper only earns its keep while the keyboard is up, when it
             becomes the band the box pins to (see .kb-open below). -->
        <div class="kms-boxwrap">
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
        </div>

        {#if spotify.results}
            {@const r = spotify.results}
            <div class="kms-kinds" role="group" aria-label="Show only">
                <button
                    class="kms-kind"
                    class:active={spotify.kindFilter === "all"}
                    onclick={() => (spotify.kindFilter = "all")}>All</button>
                {#each SEARCH_KINDS as k (k.id)}
                    {#if r[k.id].length > 0}
                        <button
                            class="kms-kind"
                            class:active={spotify.kindFilter === k.id}
                            onclick={() => (spotify.kindFilter = k.id)}
                            >{KIND_EMOJI[k.kind]} {k.label} <span class="mono">{r[k.id].length}</span></button>
                    {/if}
                {/each}
            </div>
        {/if}

        <div class="kms-results" class:stale={spotify.stale}>
        {#if !booted || spotify.pending}
            <!-- Only when there is nothing on screen yet. A search that runs
                 while results are up keeps them and dims them instead — a
                 list that vanishes every time another letter is typed is
                 hard enough for a grown-up to follow. -->
            <div class="kms-sklist" aria-hidden="true">
                {#each Array(4) as _, i (i)}
                    <div class="kms-skel"></div>
                {/each}
            </div>
        {:else if spotify.error}
            <div class="kms-empty">
                <div class="kms-empty-emoji">📡</div>
                <p>Couldn't reach the music!<br />Let's try that again.</p>
                <button class="kms-connect-btn" onclick={() => spotify.retry()}>Try again</button>
            </div>
        {:else if !spotify.connected}
            {#if spotify.status?.configured}
                <!-- The kid's own account links here (DESIGN.md §17): a
                     grown-up does the sign-in once, on this device, and
                     from then on the kid's searches run as that account —
                     with its own settings, explicit filter included. Over
                     plain HTTP the flow can't bounce back on its own, so
                     the last hop is pasted back by hand. -->
                <div class="kms-empty kms-connect">
                    <div class="kms-empty-emoji">🎧</div>
                    <p>Connect your Spotify to find songs!<br />A grown-up signs in with the kid's account.</p>
                    <ol class="kms-steps">
                        <li>
                            <button class="kms-connect-btn" onclick={() => spotify.connect()}>
                                1. Connect Spotify
                            </button>
                        </li>
                        {#if spotify.status?.manual}
                            <li>
                                <span class="kms-paste-label">Paste the web address it sends you to:</span>
                                <span class="kms-paste">
                                    <input
                                        type="url"
                                        placeholder="http://127.0.0.1…"
                                        aria-label="The web address Spotify sent you to"
                                        autocapitalize="off"
                                        autocomplete="off"
                                        spellcheck="false"
                                        enterkeyhint="done"
                                        bind:value={spotify.pasteUrl}
                                        onkeydown={(e) => {
                                            if (e.key === "Enter") void spotify.finishConnect();
                                        }}
                                    />
                                    <button
                                        class="kms-paste-btn"
                                        disabled={spotify.finishing || !spotify.pasteUrl.trim()}
                                        onclick={() => void spotify.finishConnect()}
                                    >
                                        {spotify.finishing ? "…" : "Done"}
                                    </button>
                                </span>
                            </li>
                        {/if}
                    </ol>
                </div>
            {:else}
                <!-- No developer app configured at all — that part is the
                     grown-ups', in the full app. -->
                <div class="kms-empty">
                    <div class="kms-empty-emoji">🎵</div>
                    <p>Music isn't set up yet —<br />ask a grown-up!</p>
                </div>
            {/if}
        {:else if spotify.results}
            {#if sections.length === 0}
                <div class="kms-empty">
                    <div class="kms-empty-emoji">🔎</div>
                    <p>No matches for “{spotify.resultsQuery}”<br />Try something else!</p>
                </div>
            {:else}
                {#if !kbOpen && spotify.kindFilter === "all" && spotify.topResult}
                    <!-- The one thing this search was almost certainly
                         after, at full size. Type mode folds it back into
                         the shelves: at one row tall the card would be the
                         only thing visible. -->
                    {@const top = spotify.topResult}
                    {@render shelfLabel("⭐ Best match")}
                    <button
                        class="kms-top"
                        class:starting={!!music.busy["item:" + top.uri]}
                        disabled={!!music.busy["item:" + top.uri]}
                        onclick={() => act(top)}
                    >
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
                            <KidTrackRow
    item={item}
    num={null}
    {music}
    {kbOpen}
    queueOpen={queueFor === (item).uri}
    flashed={queuedFlash === (item).uri}
    onPick={pick}
    onToggleQueue={toggleQueueFor}
    onQueue={queueIt}
/>
                        {/each}
                    {:else}
                        <!-- Songs are a list; everything else is a grid —
                             a container is chosen by its cover. -->
                        <div class="kms-grid">
                            {#each sec.items as item (item.uri)}
                                <KidMediaCard item={item} onPick={act} />
                            {/each}
                        </div>
                    {/if}
                {/each}
            {/if}
        {:else}
            <!-- Idle: what was on lately, then the room's recent searches,
                 then the playlists as a cover grid. "Again" leads because
                 it needs no reading and no typing at all — the shortest
                 path this screen has to sound coming out. -->
            {#if spotify.recentTracks.length > 0}
                {@render shelfLabel("🔁 Play it again")}
                {#each spotify.recentTracks.slice(0, 6) as item (item.uri)}
                    <KidTrackRow
    item={item}
    num={null}
    {music}
    {kbOpen}
    queueOpen={queueFor === (item).uri}
    flashed={queuedFlash === (item).uri}
    onPick={pick}
    onToggleQueue={toggleQueueFor}
    onQueue={queueIt}
/>
                {/each}
            {/if}
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
                        <KidMediaCard item={item} onPick={act} />
                    {/each}
                </div>
            {/if}

            {#if recents.list.length === 0 && spotify.myPlaylists.length === 0 && spotify.recentTracks.length === 0}
                <div class="kms-empty">
                    <div class="kms-empty-emoji">🔎</div>
                    <p>Type a song or a singer<br />to find them!</p>
                </div>
            {/if}
        {/if}
        </div>
    {/if}
</div>

<style>
    .kms { display: flex; flex-direction: column; gap: var(--space-3); }

    /* A newer search running behind the list: it stays, dimmed, and stops
       taking taps — a row tapped while it is being replaced would play
       whatever landed in its place. */
    .kms-results { display: flex; flex-direction: column; gap: var(--space-3); }
    .kms-results.stale { opacity: 0.45; pointer-events: none; transition: opacity var(--t-fast); }
@media (prefers-reduced-motion: reduce) {






        .kms-results.stale { transition-duration: 0.001ms; }
}

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
    /* A tapped song can take a moment to reach the speaker — the cover
       breathes until the player (or the mini bar) names it. */
    .kms-top.starting .kms-top-art { animation: kms-start 0.9s ease-in-out infinite; }
    @keyframes kms-start {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.4; }
    }
    /* On a phone the length was being paid for by the title and artist you
       choose a song by — the trade DESIGN.md §15.9 already settled for the
       app's rows, applied to the kid's. */

    /* ── Cover grids ── */
    .kms-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(min(140px, 45%), 1fr));
        gap: var(--space-3);
    }
    .kms-card-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 2.6rem;
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
    .kms-top:disabled { opacity: 0.75; }
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

    /* ── Skeletons & empties ── */
    .kms-sklist { display: flex; flex-direction: column; gap: var(--space-3); }
    .kms-skel {
        height: 64px;
        border-radius: var(--radius-lg);
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

    /* ── Connect Spotify ── */
    .kms-connect { display: flex; flex-direction: column; align-items: center; gap: var(--space-4); }
    .kms-steps {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-4);
        list-style: none;
        margin: 0;
        padding: 0;
        width: 100%;
    }
    .kms-steps li { display: flex; flex-direction: column; align-items: center; gap: var(--space-2); width: 100%; }
    .kms-connect-btn {
        font-size: 1.1rem;
        font-weight: 800;
        padding: 16px 32px;
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
    .kms-connect-btn:active { transform: scale(0.94); }
    .kms-paste-label { font-size: 0.95rem; font-weight: 700; }
    .kms-paste {
        display: flex;
        gap: var(--space-2);
        width: 100%;
        max-width: 420px;
    }
    .kms-paste input {
        flex: 1;
        min-width: 0;
        padding: 14px 18px;
        border-radius: 999px;
        border: 3px solid var(--border);
        background: var(--bg-elevated);
        color: var(--text);
        font-size: 1.05rem;
        font-weight: 600;
        outline: none;
    }
    .kms-paste input:focus { border-color: var(--kid-accent); }
    .kms-paste input::placeholder { color: var(--text-faint); }
    .kms-paste-btn {
        font-size: 1rem;
        font-weight: 800;
        padding: 14px 22px;
        min-height: 52px;
        border-radius: 999px;
        border: 2px solid var(--kid-accent);
        background: var(--kid-accent-soft);
        color: var(--kid-accent);
        cursor: pointer;
        flex-shrink: 0;
        -webkit-tap-highlight-color: transparent;
    }
    .kms-paste-btn:active { transform: scale(0.94); }
    .kms-paste-btn:disabled { opacity: 0.5; }

    /* ── Type mode: the software keyboard is up, so results go dense —
         no sub-lines, no labels, no kind chips — and more matches fit.
         The view has hidden its pane chips for the duration, which frees
         the top of the scrollport for the box to pin to: typing is exactly
         when the thing you must not lose sight of is the box. ── */
    .kb-open .kms-boxwrap {
        position: sticky;
        top: 0;
        z-index: var(--z-sticky);
        /* Out to the screen edges, on the gutter the view publishes, so the
           results pass behind a band rather than around a pill's corners. */
        margin: 0 calc(-1 * var(--km-gutter, 0px));
        padding: var(--space-2) var(--km-gutter, 0px);
        background: var(--dock-fill);
        backdrop-filter: blur(20px) saturate(1.6);
        -webkit-backdrop-filter: blur(20px) saturate(1.6);
        border-bottom: 1px solid var(--dock-edge);
    }
    .kb-open .kms-label,
    .kb-open .kms-kinds,
    .kb-open .kms-shelf-head { display: none; }
    .kb-open .kms-grid { grid-template-columns: repeat(auto-fill, minmax(min(110px, 30%), 1fr)); }
    @media (prefers-reduced-motion: reduce) {
        .kms-skel { animation: none; }
        .kms-top.starting .kms-top-art {
            animation: none;
        }
    }
</style>
