<script lang="ts">
    /**
     * Everything the search column shows below the box: the results when
     * there are some, and the shelves that answer "put something on" when
     * there are not.
     *
     * The idle order is the whole point of the pane. What this room played
     * lately leads — HomeHub's own per-room memory, ranked by what it keeps
     * coming back to and, where it has a habit at this hour, by what it
     * plays *then*. Spotify's history is one list for the household and
     * cannot say that the kitchen gets radio at breakfast, which is exactly
     * the question a wall is asked. Then the room's recent searches, the
     * household's favorites, what the account plays most, and its playlists
     * as an art grid (§15.9: everything but songs is a grid).
     *
     * A search that is running keeps the previous results up and dims them
     * rather than blanking: on a wall the list is read from across the
     * room, and blanking it on every letter is the worst thing this depth
     * could do to someone mid-glance.
     *
     * Both layout modes arrive as props. A component's styles are scoped,
     * so the depth's `.browse.full .s-rows` would no longer reach in here.
     */
    import Icon from "../Icon.svelte";
    import EmptyState from "../EmptyState.svelte";
    import MediaCard from "../music/MediaCard.svelte";
    import PanelResultRow from "./PanelResultRow.svelte";
    import { rowSub } from "../../lib/music/catalog";
    import { playCount } from "../../lib/music/format";
    import type { SpotifyStore } from "../../lib/music/spotify.svelte";
    import type { SearchHistory } from "../../lib/music/history.svelte";
    import type {
        PanelMemory,
        PanelQueue,
        PanelRooms,
        PanelSource,
        PanelStarting,
    } from "../../lib/panel-music.svelte";
    import type { MediaPlay, SonosFavorite, SpotifyItem } from "../../lib/types";

    let {
        music,
        spotify,
        recents,
        featured,
        favorites,
        sections,
        artistLead,
        roomPlays,
        roomPlaysLabel,
        roomPlaysHousehold,
        booted,
        kbOpen = false,
        fullBleed = false,
        resultsEl = $bindable(null),
        onPick,
        onOpenArtist,
        onRunRecent,
    }: {
        music: PanelRooms & PanelStarting & PanelMemory & PanelQueue;
        spotify: SpotifyStore;
        recents: SearchHistory;
        featured: PanelSource | undefined;
        /** The household's Sonos list — empty unless a Sonos room is featured. */
        favorites: SonosFavorite[];
        /** The result shelves that matched, in the order they are shown. */
        sections: { id: "tracks" | "albums" | "playlists" | "artists"; label: string; items: SpotifyItem[] }[];
        /** The typed name's own artist, shelved above the songs it turns up in. */
        artistLead: SpotifyItem | null;
        roomPlays: MediaPlay[];
        roomPlaysLabel: string;
        /** True when the shelf is the household's rather than this room's. */
        roomPlaysHousehold: boolean;
        /** False until the Spotify status read has answered either way. */
        booted: boolean;
        kbOpen?: boolean;
        fullBleed?: boolean;
        /** The scrollport — the depth stashes and restores its position as
         *  the catalog stack is pushed and popped. */
        resultsEl?: HTMLElement | null;
        onPick: (item: SpotifyItem) => void;
        onOpenArtist: (uri: string, art?: { art_url?: string; round?: boolean }) => void;
        onRunRecent: (q: string) => void;
    } = $props();

    /** Nothing at all to idle on — a different panel from a thin one. */
    const idleEmpty = $derived(
        recents.list.length === 0 &&
            favorites.length === 0 &&
            roomPlays.length === 0 &&
            spotify.myPlaylists.length === 0 &&
            spotify.recentTracks.length === 0 &&
            spotify.topTracks.length === 0 &&
            spotify.savedAlbums.length === 0 &&
            spotify.topArtists.length === 0 &&
            spotify.newReleases.length === 0,
    );
</script>

<div
    class="s-results"
    class:full={fullBleed}
    class:kb-open={kbOpen}
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
                <PanelResultRow
                    item={spotify.topResult}
                    big
                    {kbOpen}
                    full={fullBleed}
                    {music}
                    {featured}
                    onOpenArtist={(uri, art) => onOpenArtist(uri, art)}
                    onPick={onPick}
                />
            {/if}
            {#if artistLead}
                <!-- The name that was typed, before the
                     songs it turns up in: an artist is
                     shelved last and is the one thing
                     type mode cannot otherwise reach. -->
                <h3 class="s-label">Artist</h3>
                <PanelResultRow
                    item={artistLead}
                    lead
                    {kbOpen}
                    full={fullBleed}
                    {music}
                    {featured}
                    onOpenArtist={(uri, art) => onOpenArtist(uri, art)}
                    onPick={onPick}
                />
            {/if}
            {#each sections as sec (sec.id)}
                <h3 class="s-label">{sec.label}</h3>
                <!-- One shelf's rows, in a box of their
                     own so the full-bleed layout can put
                     them in two columns without the
                     labels between them being dealt into
                     the same grid. -->
                <div class="s-rows">
                    {#each sec.items as item (item.uri)}
                        <PanelResultRow
                            item={item}
                            {kbOpen}
                            full={fullBleed}
                            {music}
                            {featured}
                            onOpenArtist={(uri, art) => onOpenArtist(uri, art)}
                            onPick={onPick}
                        />
                    {/each}
                </div>
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
        <!-- The room in front of you leads: HomeHub's own
             per-room memory, ranked by what this room
             keeps coming back to and — where it has a
             habit at this hour — by what it plays at
             this hour. Spotify's history is one list for
             the whole household and cannot say that the
             kitchen gets radio at breakfast, which is
             exactly the question a wall is asked. -->
        {#if roomPlays.length > 0}
            <h3 class="s-label">{roomPlaysLabel}</h3>
            <!-- One row of it in the two-column body,
                 where a second row of covers would be
                 the whole column and everything under it
                 a scroll away; the full-bleed layout has
                 the width for the rest. -->
            <div class="s-grid">
                {#each roomPlays.slice(0, fullBleed ? 8 : 4) as p (p.uri)}
                    {@render playTile(p)}
                {/each}
            </div>
        {/if}
        {#if spotify.recentTracks.length > 0}
            <h3 class="s-label">Played recently</h3>
            <div class="s-rows">
                {#each spotify.recentTracks.slice(0, 6) as item (item.uri)}
                    <PanelResultRow
                        item={item}
                        {kbOpen}
                        full={fullBleed}
                        {music}
                        {featured}
                        onOpenArtist={(uri, art) => onOpenArtist(uri, art)}
                        onPick={onPick}
                    />
                {/each}
            </div>
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
                            onclick={() => onRunRecent(h.q)}
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
            <div class="s-rows">
                {#each spotify.topTracks.slice(0, 6) as item (item.uri)}
                    <PanelResultRow
                        item={item}
                        {kbOpen}
                        full={fullBleed}
                        {music}
                        {featured}
                        onOpenArtist={(uri, art) => onOpenArtist(uri, art)}
                        onPick={onPick}
                    />
                {/each}
            </div>
        {/if}
        {#if spotify.myPlaylists.length > 0}
            <h3 class="s-label">Your playlists</h3>
            <div class="s-grid">
                {#each spotify.myPlaylists as item (item.uri)}
                    <MediaCard
                        {item}
                        sub={rowSub(item)}
                        onOpen={() => onPick(item)}
                    />
                {/each}
            </div>
        {/if}
        <!-- The rest of the collection. Each shelf is its
             own read on the backend and its own absence
             here: an account with no saved albums and a
             login whose grant predates a scope both draw
             nothing, which is the honest answer to
             both (§15.9). -->
        {#if spotify.savedAlbums.length > 0}
            <h3 class="s-label">Your albums</h3>
            <div class="s-grid">
                {#each spotify.savedAlbums as item (item.uri)}
                    <MediaCard
                        {item}
                        sub={rowSub(item)}
                        onOpen={() => onPick(item)}
                    />
                {/each}
            </div>
        {/if}
        {#if spotify.topArtists.length > 0}
            <!-- Artists open their page rather than
                 playing, here as everywhere else on this
                 depth: no speaker takes an artist URI,
                 and the page is where the records are. -->
            <h3 class="s-label">Artists you play</h3>
            <div class="s-grid">
                {#each spotify.topArtists as item (item.uri)}
                    <MediaCard
                        {item}
                        round
                        sub={rowSub(item)}
                        onOpen={() =>
                            onOpenArtist(
                                item.uri,
                                item.art_url
                                    ? { art_url: item.art_url, round: true }
                                    : undefined,
                            )}
                    />
                {/each}
            </div>
        {/if}
        {#if spotify.newReleases.length > 0}
            <!-- The one shelf here that is not about this
                 household at all — and so the only
                 answer on an evening when nobody wants
                 to hear anything they already know. -->
            <h3 class="s-label">Out this week</h3>
            <div class="s-grid">
                {#each spotify.newReleases as item (item.uri)}
                    <MediaCard
                        {item}
                        sub={rowSub(item)}
                        onOpen={() => onPick(item)}
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

     songs is a grid). One tap plays it outright — a station is the one
     thing on this depth that needs no typing at all — and there is no
     queue affordance here on purpose: most of what a home stars is radio,
     and a live stream has no place in a queue. Rendered only for a Sonos
     destination, because the list is a Sonos household's. -->
{#snippet favoriteShelf()}
    {#if favorites.length > 0}
        <h3 class="s-label">Favorites</h3>
        <div class="s-grid">
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

<!-- One thing this room has played, as a tile. The same square the
     household's favorites wear, because on a wall a cover is a far better
     target than a title — with the tally under it where there is one, since
     that is what separates the record a room lives on from the one somebody
     tried once. A favorite replays through the favorites path it came from,
     which the store handles; a favorite since deleted stops being offered

     rather than becoming a tile that fails. -->
{#snippet playTile(p: MediaPlay)}
    <button
        class="s-fav-play"
        aria-label="Play {p.title} again on {featured?.title ?? 'this room'}"
        disabled={!!music.busy["hist:" + p.uri]}
        onclick={() => music.playFromHistory(p)}
    >
        {#if p.art_uri}
            <img class="s-fav-art" src={p.art_uri} alt="" loading="lazy" />
        {:else}
            <span class="s-fav-art placeholder">[ art ]</span>
        {/if}
        <span class="s-fav-title">{p.title}</span>
        <span class="s-fav-sub">
            {roomPlaysHousehold && p.room_name ? p.room_name : (p.sub ?? "")}
            {#if playCount(p) > 1}<span class="mono"> ×{playCount(p)}</span>{/if}
        </span>
    </button>
{/snippet}

<style>
    .s-results.full {
        animation: results-widen 140ms ease-out both;
    }
    .s-note {
        margin: var(--space-4) 0 0;
        font-size: 12.5px;
        line-height: 1.5;
        color: var(--text-dim);
    }
    .s-results.kb-open .sk-row {
        min-height: 48px;
    }
    .s-results.kb-open .sk-art {
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
    .s-results.stale {
        opacity: 0.45;
        pointer-events: none;
    }

    .s-more {
        display: flex;
        justify-content: center;
        margin-top: var(--space-3);
    }
    .s-rows {
        display: flex;
        flex-direction: column;
    }
    .s-results.full .s-rows {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
        column-gap: var(--space-5);
    }
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
    .s-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
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
@media (prefers-reduced-motion: reduce) {

        .s-results {
            transition-duration: 0.001ms;
        }
        .s-results.full {
            animation-duration: 0.001ms;
        }
}

    /* Portrait: the depth stacks, and every scrollport gives up its own
       scroll to the page. */
    @media (orientation: portrait), (max-width: 760px) {
        .s-results {
            overflow: visible;
            flex: none;
        }
    }
</style>
