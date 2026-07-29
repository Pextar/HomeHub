<script lang="ts">
    /**
     * Browse — everywhere music comes *from*, in one place: your Sonos
     * favorites, your recent searches, and Spotify's catalog behind the box at
     * the top. Home is about rooms; this is about music; a tap here starts
     * whatever is playing in whichever room the picker names.
     *
     * The results are meant to be as informative as Spotify's own and easier
     * to act on. A search for a name answers with a **top result** card first —
     * the one thing you almost certainly meant, at full size, with its own
     * stats — then one shelf per kind: songs as a queueable list carrying the
     * album and the running time, and artists, albums and playlists as grids
     * of cards that say how big the name is, what year the record is, how many
     * songs the playlist holds.
     *
     * Nothing here plays a guess. An artist opens their page, an album or a
     * playlist opens its track listing; only a song (and an explicit Play on a
     * container) starts audio. That is the difference between browsing a
     * catalog and firing blind into a room.
     *
     * The box behaves like a search box: typing debounces, Enter runs the
     * query immediately, a clear X appears once there is something to clear
     * (Escape does the same from inside the field), and arriving here puts the
     * caret in the box — on `(pointer: fine)` only, since auto-focus on a
     * phone throws the software keyboard over the results.
     */
    import type { Snippet } from "svelte";
    import { tick as flushDOM } from "svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import TrackList from "./TrackList.svelte";
    import MediaCard from "./MediaCard.svelte";
    import { dur } from "../../lib/motion";
    import { fmtCount, fmtMs, capFirst } from "../../lib/music/format";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        spotify,
        recents,
        destination,
        busy,
        /** True when the screen was opened to type in, rather than to read. */
        autofocus = false,
        onBack,
        onPlayItem,
        onEnqueue,
        /** An artist row opens their page rather than playing outright — top
         *  tracks and albums to pick from, not a single thing to start. */
        onOpenArtist,
        /** An album or playlist opens its own track listing, for the same
         *  reason: a container is a place to look, not a thing to fire. */
        onOpenContext,
        targetRow,
        /** The Sonos favorites shelf, shown while nothing is being searched. */
        favorites = undefined,
    }: {
        spotify: SpotifyStore;
        recents: SearchHistory;
        destination: Destination;
        busy: Busy;
        autofocus?: boolean;
        onBack: () => void;
        onPlayItem: (item: SpotifyItem) => void;
        onEnqueue: (item: SpotifyItem, next: boolean) => void;
        onOpenArtist: (uri: string) => void;
        onOpenContext: (uri: string) => void;
        targetRow: Snippet;
        favorites?: Snippet;
    } = $props();

    /** Nothing typed and nothing returned — the shelf's moment. */
    const idle = $derived(!spotify.query && !spotify.results);

    let searchEl = $state<HTMLInputElement | null>(null);
    let resultsEl = $state<HTMLDivElement | null>(null);

    // Only where a keyboard is already there. On a phone an auto-focus throws
    // up the software keyboard over the results the user came to look at.
    $effect(() => {
        if (!autofocus || !spotify.connected) return;
        if (!window.matchMedia("(pointer: fine)").matches) return;
        void flushDOM().then(() => searchEl?.focus());
    });

    // Enter runs the search now instead of waiting out the debounce; Escape
    // clears the box rather than closing something behind it; ArrowDown hands
    // the caret off to the first result, the way a desktop search box should.
    function onQueryKey(e: KeyboardEvent) {
        if (e.key === "Enter") {
            e.preventDefault();
            spotify.runNow();
        } else if (e.key === "Escape" && spotify.query) {
            e.stopPropagation();
            spotify.clearQuery();
            searchEl?.focus();
        } else if (e.key === "ArrowDown" && spotify.shownItems.length > 0) {
            e.preventDefault();
            resultsEl?.querySelector<HTMLButtonElement>("button")?.focus();
        }
    }
    function runHistoryQuery(q: string) {
        spotify.runQuery(q);
        searchEl?.focus();
    }
    function clearQuery() {
        spotify.clearQuery();
        searchEl?.focus();
    }

    /**
     * One tap, routed by kind. A song plays; everything else opens, because a
     * container tapped is a request to see inside it, and an artist has no
     * single thing to start (DESIGN.md §15 — a control that would be refused
     * is worse than one that isn't there, and Sonos refuses an artist URI
     * outright).
     */
    function open(item: SpotifyItem) {
        if (item.kind === "artist") return onOpenArtist(item.uri);
        if (item.kind === "album" || item.kind === "playlist") return onOpenContext(item.uri);
        onPlayItem(item);
    }

    /** What a card says under its name — different per kind, because what
     *  makes each one worth choosing is different. */
    function cardSub(item: SpotifyItem): string {
        if (item.kind === "artist") {
            if (item.followers) return `${fmtCount(item.followers)} followers`;
            return item.genres?.[0] ? capFirst(item.genres[0]) : "Artist";
        }
        if (item.kind === "album") return [item.sub, item.year].filter(Boolean).join(" · ");
        if (item.kind === "playlist") {
            const n = item.total_tracks ? `${item.total_tracks} songs` : "";
            return [item.sub, n].filter(Boolean).join(" · ");
        }
        return item.sub ?? "";
    }

    const KIND_LABEL: Record<string, string> = {
        artist: "Artist",
        album: "Album",
        playlist: "Playlist",
        track: "Song",
    };

    /**
     * The top result's own line: what identifies it fastest, and nothing
     * beyond that. An artist's genres were here and pushed the line past a
     * phone's width — so the one stat that sizes a name stays and the genres
     * wait for the artist page, where they have a row of their own.
     */
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

    /** A container can be started outright as well as opened — an album is
     *  both a place and a thing to play. An artist can't: there is no artist
     *  URI a speaker will take. */
    const playable = (item: SpotifyItem) => item.kind !== "artist";

    /** Every shelf the grouped overview can show, in the order it shows
     *  them. Songs lead — playing one is the commonest reason to search at
     *  all — and only shelves that matched are rendered. */
    const shelves = $derived.by(() => {
        const r = spotify.results;
        if (!r) return [];
        return [
            { kind: "artists" as const, label: "Artists", items: r.artists, round: true },
            { kind: "albums" as const, label: "Albums", items: r.albums, round: false },
            { kind: "playlists" as const, label: "Playlists", items: r.playlists, round: false },
        ].filter((s) => s.items.length > 0);
    });

    /** The flat single-kind view: a list for songs, a grid for the rest. */
    const flatItems = $derived(spotify.kindFilter === "all" ? [] : spotify.shownItems);

    let songList = $state<TrackList | null>(null);
    let flatList = $state<TrackList | null>(null);

    /**
     * Escape closes an open row menu before it closes the screen, so the
     * shell asks here first. Answers whether it consumed the key.
     */
    export function closeMenu(): boolean {
        return !!(songList?.closeMenu() || flatList?.closeMenu());
    }
</script>

<div class="screen-head">
    <button class="icon-btn" aria-label="Back to Music" onclick={onBack}>
        <Icon name="chevronLeft" size={18} />
    </button>
    <div class="screen-title">
        <h1>Browse</h1>
        <span class="screen-sub">{destination.label || "no room"}</span>
    </div>
    <span class="head-spacer" aria-hidden="true"></span>
</div>

<div class="search-body" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    <!-- Where anything started here will come out. Once, at the top, for the
         whole screen — rather than a copy of it inside every section. -->
    <div class="sp-where">{@render targetRow()}</div>

    <!-- ── Favorites ───────────────────────────────────────────────
         Your Sonos household's own list. It leads, because it is the one
         thing here that needs no typing and no account. -->
    {#if favorites && idle}
        {@render favorites()}
    {/if}

    <!-- ── Spotify search ──────────────────────────────────────────── -->
    {#if spotify.status}
        <section class="card">
            {#if !spotify.status?.configured || spotify.setupOpen}
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
                            <code class="mono">{spotify.status?.redirect_uri}</code>
                            <button type="button" class="chip" onclick={() => spotify.copyRedirect()}>
                                <Icon name={spotify.copied ? "check" : "copy"} size={13} />
                                {spotify.copied ? "Copied" : "Copy"}
                            </button>
                        </span>
                    </li>
                    <li>Paste the app's Client ID here:</li>
                </ol>
                <form class="sp-config" onsubmit={(e) => { e.preventDefault(); void spotify.saveClientId(); }}>
                    <input type="text" class="mono" placeholder="Client ID"
                        aria-label="Spotify client ID" bind:value={spotify.clientId} />
                    <button type="submit" class="btn btn-primary" disabled={spotify.saving || !spotify.clientId.trim()}>
                        {spotify.saving ? "Saving…" : "Save"}
                    </button>
                    {#if spotify.setupOpen}
                        <button type="button" class="btn btn-ghost" onclick={() => (spotify.setupOpen = false)}>Cancel</button>
                    {/if}
                </form>
            {:else if !spotify.connected}
                <div class="card-header"><h2>Spotify search</h2></div>
                <p class="sp-help">
                    Client ID saved — now connect your Spotify account. You'll
                    approve access once on Spotify's page{spotify.status?.manual
                        ? "; it opens in a new tab and ends on an unreachable 127.0.0.1 address — that's expected."
                        : ", then land back here."}
                </p>
                <div class="sp-actions">
                    <button class="btn btn-primary" onclick={() => spotify.connect()}>Connect Spotify</button>
                    <button class="btn btn-ghost" onclick={() => { spotify.clientId = ""; spotify.setupOpen = true; }}>
                        Change client ID
                    </button>
                </div>
                {#if spotify.status?.manual}
                    <div class="field sp-paste">
                        <label for="sp-paste-input">
                            After approving, copy the full address from that tab and paste it here to finish:
                        </label>
                        <div class="sp-config">
                            <input id="sp-paste-input" type="text" class="mono"
                                placeholder="http://127.0.0.1:…/api/spotify/callback?code=…"
                                bind:value={spotify.pasteUrl} />
                            <button type="button" class="btn btn-primary"
                                disabled={spotify.finishing || !spotify.pasteUrl.trim()} onclick={() => spotify.finishConnect()}>
                                {spotify.finishing ? "Finishing…" : "Finish"}
                            </button>
                        </div>
                    </div>
                {/if}
            {:else}
                <!-- No <h2> here: the screen's own head already says
                     "Browse". This row only answers "as whom". -->
                <div class="card-header sp-head">
                    <div class="sp-account">
                        <span class="sp-conn" title="Connected to Spotify">
                            <span class="sp-dot" aria-hidden="true"></span>
                            <span class="sp-conn-label">Connected</span>
                            <span class="sp-user mono">{spotify.status?.display_name || "Spotify"}</span>
                        </span>
                    </div>
                </div>
                <div class="sp-search">
                    <Icon name="search" size={16} />
                    <input
                        type="text"
                        class="sp-input"
                        placeholder="Songs, albums, artists, playlists…"
                        aria-label="Search Spotify"
                        autocomplete="off"
                        enterkeyhint="search"
                        bind:this={searchEl}
                        bind:value={spotify.query}
                        oninput={() => spotify.onQueryInput()}
                        onkeydown={onQueryKey}
                    />
                    {#if spotify.query}
                        <button class="icon-btn sp-clear" aria-label="Clear search" onclick={clearQuery}>
                            <Icon name="close" size={14} />
                        </button>
                    {/if}
                </div>
                {#if !spotify.query && !spotify.results && recents.list.length > 0}
                    <div class="sp-history">
                        <div class="sp-history-head">
                            <span class="eylabel">
                                Recent searches{#if destination.list.length > 1 && destination.label} · {destination.label}{/if}
                            </span>
                            <button type="button" class="chip sp-hist-clear" onclick={() => recents.clear()}>Clear</button>
                        </div>
                        <div class="sp-history-list">
                            {#each recents.list as h (h)}
                                <div class="sp-hist-chip">
                                    <button type="button" class="sp-hist-run" onclick={() => runHistoryQuery(h)}>
                                        <Icon name="search" size={12} />
                                        <span>{h}</span>
                                    </button>
                                    <button type="button" class="icon-btn sp-hist-x"
                                        aria-label={`Remove "${h}" from recent searches`}
                                        onclick={() => recents.remove(h)}>
                                        <Icon name="close" size={10} />
                                    </button>
                                </div>
                            {/each}
                        </div>
                    </div>
                {/if}
                <!-- The filter is a chip row, and it says how much is behind
                     each one — "Albums 12" is a decision, "Albums" is a
                     guess. -->
                <div class="sp-filters">
                    {#if spotify.results}
                        {@const r = spotify.results}
                        <button class="chip" class:active={spotify.kindFilter === "all"}
                            onclick={() => (spotify.kindFilter = "all")}>All</button>
                        {#each [
                            { k: "tracks" as const, label: "Songs", n: r.tracks.length },
                            { k: "artists" as const, label: "Artists", n: r.artists.length },
                            { k: "albums" as const, label: "Albums", n: r.albums.length },
                            { k: "playlists" as const, label: "Playlists", n: r.playlists.length },
                        ] as f (f.k)}
                            {#if f.n > 0}
                                <button class="chip" class:active={spotify.kindFilter === f.k}
                                    onclick={() => (spotify.kindFilter = f.k)}>
                                    {f.label}
                                    <span class="chip-n mono">{f.n}</span>
                                </button>
                            {/if}
                        {/each}
                    {:else if spotify.myPlaylists.length > 0}
                        <span class="eylabel">Your playlists</span>
                    {/if}
                </div>
                <!-- Playing on a KEF speaker goes out through Spotify Connect,
                     which needs a permission this login may predate. Saying so
                     before the tap beats a 409 after it, and reconnecting is
                     the only thing that fixes it. -->
                {#if destination.room?.speaker && spotify.status && !spotify.status.playback}
                    <div class="sp-note">
                        <Icon name="info" size={14} />
                        <span>
                            Reconnect Spotify to start music on {destination.room.name} —
                            this login was made before HomeHub could ask for that.
                        </span>
                        <button class="chip" onclick={() => spotify.connect()}>Reconnect</button>
                    </div>
                {:else if destination.room?.zone?.problem}
                    <!-- A zone that nothing can serve, said in the backend's own
                         words — they name which speaker blocked which route,
                         which is the part a user can act on. The same sentence
                         is on the zone's card; this is it before the tap. -->
                    <div class="sp-note">
                        <Icon name="info" size={14} />
                        <span>{destination.room.zone.problem}</span>
                        {#if spotify.status && !spotify.status.playback}
                            <button class="chip" onclick={() => spotify.connect()}>Reconnect</button>
                        {/if}
                    </div>
                {/if}

                <div bind:this={resultsEl}>
                    {#if spotify.searching}
                        <!-- Skeletons in the shape of what is coming: one hero,
                             then rows. Never a spinner. -->
                        <div class="sp-groups">
                            <div class="skeleton sk-hero"></div>
                            <div class="sp-shelf">
                                <div class="skeleton sk-row"></div>
                                <div class="skeleton sk-row"></div>
                                <div class="skeleton sk-row"></div>
                            </div>
                        </div>
                    {:else if spotify.results && spotify.shownItems.length === 0}
                        <div class="sp-none">
                            {#if spotify.kindFilter === "all"}
                                No results for "{spotify.query.trim()}".
                            {:else}
                                No {spotify.kindFilter} matched "{spotify.query.trim()}".
                            {/if}
                        </div>
                    {:else if !spotify.results && spotify.myPlaylists.length === 0}
                        <!-- No query and no playlists to browse — say what this
                             box does rather than leaving a blank panel. -->
                        <div class="sp-none">
                            Search Spotify for a song, album, artist or playlist. Songs play on the
                            room above; artists, albums and playlists open so you can see what's on
                            them first.
                        </div>
                    {:else if !spotify.results}
                        <!-- The account's own playlists, as cards: this is
                             browsing, not a lookup, so it gets the same grid
                             the search shelves use. -->
                        <div class="sp-grid">
                            {#each spotify.myPlaylists as item (item.uri)}
                                <MediaCard {item} sub={cardSub(item)} onOpen={() => open(item)} />
                            {/each}
                        </div>
                    {:else if spotify.kindFilter === "all"}
                        <!-- The overview: best single match up top, then one
                             shelf per kind that actually matched — songs as a
                             queueable list, the rest as grids of cards. -->
                        <div class="sp-groups">
                            {#if spotify.topResult}
                                {@const top = spotify.topResult}
                                <div class="sp-shelf">
                                    <span class="eylabel">Top result</span>
                                    <div class="sp-top">
                                        <button
                                            class="sp-top-open"
                                            onclick={() => open(top)}
                                            aria-label={top.kind === "track"
                                                ? `Play ${top.name}`
                                                : `Open ${top.name}`}
                                        >
                                            {#if top.art_url}
                                                <img class="sp-top-art-img" class:round={top.kind === "artist"}
                                                    src={top.art_url} alt="" />
                                            {:else}
                                                <div class="sp-top-art-img placeholder"
                                                    class:round={top.kind === "artist"}>[ art ]</div>
                                            {/if}
                                            <span class="sp-top-meta">
                                                <span class="sp-top-name">{top.name}</span>
                                                <span class="sp-top-line">{topLine(top)}</span>
                                                {#if top.kind !== "track"}
                                                    <span class="sp-top-cta">
                                                        {top.kind === "artist" ? "See top tracks & albums" : "See what's on it"}
                                                        <Icon name="chevronLeft" size={13} />
                                                    </span>
                                                {/if}
                                            </span>
                                        </button>
                                        <!-- An album or playlist is both a place
                                             and a thing to play, so it gets an
                                             explicit Play beside the way in. An
                                             artist has no URI a speaker takes. -->
                                        {#if playable(top)}
                                            <button
                                                class="sp-top-play"
                                                disabled={busy.is("item:" + top.uri) || !destination.current}
                                                aria-label={`Play ${top.name}${destination.label ? " on " + destination.label : ""}`}
                                                onclick={() => onPlayItem(top)}
                                            >
                                                <Icon name="play" size={20} />
                                            </button>
                                        {/if}
                                    </div>
                                </div>
                            {/if}

                            {#if spotify.results.tracks.length > 0}
                                <div class="sp-shelf">
                                    <div class="sp-shelf-head">
                                        <span class="eylabel">Songs</span>
                                        {#if spotify.results.tracks.length > 5}
                                            <button class="chip" onclick={() => (spotify.kindFilter = "tracks")}>
                                                See all <span class="chip-n mono">{spotify.results.tracks.length}</span>
                                            </button>
                                        {/if}
                                    </div>
                                    <TrackList
                                        items={spotify.results.tracks.slice(0, 5)}
                                        {busy}
                                        canPlay={!!destination.current}
                                        queueTarget={destination.sonosTarget}
                                        onPick={onPlayItem}
                                        {onEnqueue}
                                        bind:this={songList}
                                    />
                                </div>
                            {/if}

                            {#each shelves as shelf (shelf.kind)}
                                <div class="sp-shelf">
                                    <div class="sp-shelf-head">
                                        <span class="eylabel">{shelf.label}</span>
                                        {#if shelf.items.length > 6}
                                            <button class="chip" onclick={() => (spotify.kindFilter = shelf.kind)}>
                                                See all <span class="chip-n mono">{shelf.items.length}</span>
                                            </button>
                                        {/if}
                                    </div>
                                    <div class="sp-grid">
                                        {#each shelf.items.slice(0, 6) as item (item.uri)}
                                            <MediaCard {item} round={shelf.round}
                                                sub={cardSub(item)} onOpen={() => open(item)} />
                                        {/each}
                                    </div>
                                </div>
                            {/each}
                        </div>
                    {:else if spotify.kindFilter === "tracks"}
                        <TrackList
                            items={flatItems}
                            {busy}
                            canPlay={!!destination.current}
                            queueTarget={destination.sonosTarget}
                            onPick={onPlayItem}
                            {onEnqueue}
                            bind:this={flatList}
                        />
                    {:else}
                        <div class="sp-grid">
                            {#each flatItems as item (item.uri)}
                                <MediaCard {item} round={spotify.kindFilter === "artists"}
                                    sub={cardSub(item)} onOpen={() => open(item)} />
                            {/each}
                        </div>
                    {/if}
                </div>
            {/if}
        </section>
    {/if}
</div>

<style>
    /* ── Screen head — the §11 shape, matching Speakers ── */
    .screen-head { display: flex; align-items: center; gap: var(--space-3); }
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
    .screen-sub {
        font-size: 12px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Balances the back chip so the title stays centred with nothing on the right. */
    .head-spacer { width: 32px; height: 32px; flex-shrink: 0; }

    .search-body {
        display: flex; flex-direction: column; gap: var(--space-5);
        margin-top: var(--space-4);
    }

    /* ── Setup / account ── */
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

    /* ── The box ── */
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
    /* A count riding inside a chip: mono and tabular, like every number. */
    .chip-n {
        margin-left: 5px;
        font-size: 10.5px; opacity: 0.7;
        font-feature-settings: "tnum" 1;
    }
    .eylabel {
        font-family: var(--font-mono);
        font-size: 10.5px; letter-spacing: 0.08em; text-transform: uppercase;
        color: var(--text-dim);
    }
    /* One picker for the screen, at the top of it. */
    .sp-where { display: flex; }
    .sp-none { font-size: 12.5px; color: var(--text-mute); line-height: 1.5; }

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

    /* ── Recent searches ── */
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

    /* ── Results ── */
    .sp-groups { display: flex; flex-direction: column; gap: var(--space-6); }
    .sp-shelf { display: flex; flex-direction: column; gap: var(--space-2); }
    .sp-shelf-head { display: flex; align-items: center; justify-content: space-between; gap: var(--space-2); }

    .sk-hero { height: 104px; border-radius: var(--r-lg); }
    .sk-row { height: 52px; border-radius: var(--r-md); }

    /* The top result: the biggest thing on the screen, because it is the
       answer most searches were after. */
    .sp-top {
        position: relative;
        display: flex; align-items: center; gap: var(--space-2);
        background: var(--card-2); border: 1px solid var(--hairline);
        border-radius: var(--r-lg); padding: var(--space-3);
        transition: border-color 150ms ease;
    }
    @media (hover: hover) { .sp-top:hover { border-color: var(--border-strong); } }
    .sp-top-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-4);
        background: transparent; border: 0; border-radius: var(--r-md);
        padding: 0; color: var(--text); cursor: pointer; text-align: left; font: inherit;
    }
    .sp-top-art-img {
        width: 84px; height: 84px; flex-shrink: 0;
        border-radius: var(--r-md); object-fit: cover;
        background: var(--card-3); border: 1px solid var(--hairline);
    }
    div.sp-top-art-img { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .sp-top-art-img.round { border-radius: 50%; }
    /* Desktop has the room, and the card is the screen's answer — it earns
       the extra size rather than floating in a wide empty row. */
    @media (min-width: 700px) {
        .sp-top { padding: var(--space-4); }
        .sp-top-art-img { width: 108px; height: 108px; }
        .sp-top-name { font-size: 22px; }
    }
    .sp-top-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
    .sp-top-name {
        font-size: 18px; font-weight: 600; letter-spacing: -0.02em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sp-top-line {
        font-family: var(--font-mono); font-size: 10.5px;
        letter-spacing: 0.05em; text-transform: uppercase; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    /* Says where the tap goes, so "open" never has to be guessed from a
       chevron alone. */
    .sp-top-cta {
        display: flex; align-items: center; gap: 3px;
        margin-top: 2px;
        font-size: 12px; color: var(--on);
    }
    .sp-top-cta :global(svg) { transform: rotate(180deg); }
    .sp-top-play {
        flex-shrink: 0;
        width: 52px; height: 52px; display: grid; place-items: center;
        border-radius: 50%; border: 0;
        background: var(--on); color: var(--primary-fg);
        cursor: pointer;
        transition: transform 150ms ease, box-shadow 150ms ease;
    }
    @media (hover: hover) {
        .sp-top-play:not(:disabled):hover { box-shadow: 0 4px 16px var(--on-glow); }
    }
    .sp-top-play:active:not(:disabled) { transform: scale(0.94); transition-duration: 80ms; }
    .sp-top-play:disabled { opacity: 0.45; cursor: default; }

    /* Cards, not a carousel: a grid shows every match at once and reflows on
       a phone, where a horizontal rail hid half of them behind a swipe. */
    .sp-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(104px, 1fr));
        gap: var(--space-3);
    }
    @media (min-width: 700px) {
        .sp-grid { grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); }
    }

    /* ── Touch: hit areas grow to the 44px floor ── */
    @media (pointer: coarse) {
        .sp-clear { width: 44px; height: 44px; }
        .sp-input, .sp-config input { font-size: 16px; } /* prevents iOS auto-zoom */
    }
    @media (prefers-reduced-motion: reduce) {
        .sp-top, .sp-top-play { transition-duration: 0.001ms; }
    }
</style>
