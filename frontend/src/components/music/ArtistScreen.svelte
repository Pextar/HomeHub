<script lang="ts">
    /**
     * An artist's page, reached by tapping an artist anywhere in the catalog —
     * and the surface this whole module was missing: who they are (the
     * following, the genres), what to play right now (the most-played tracks,
     * numbered, with lengths), everything they released (albums and singles on
     * their own shelves, by year), and who else to try.
     *
     * A screen, not a sheet: Browse is itself a screen with a stack, and this
     * pushes onto it, so an album opened from here goes one level deeper and
     * `back` climbs the same ladder (DESIGN.md §15.6).
     *
     * What it plays, and what it merely opens, follows the capability rule:
     * a track plays on the room below, an album opens its listing, a related
     * artist opens their page. There is no artist URI a speaker will take, so
     * "play this artist" starts their top track and says so.
     */
    import type { Snippet } from "svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import TrackList from "./TrackList.svelte";
    import MediaCard from "./MediaCard.svelte";
    import { dur } from "../../lib/motion";
    import { fmtCount, fmtFollowers, capFirst } from "../../lib/music/format";
    import type { Destination } from "../../lib/music/destination.svelte";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyArtistDetail, SpotifyItem } from "../../lib/types";

    let {
        artist,
        loading,
        destination,
        busy,
        targetRow,
        onBack,
        onPick,
        onEnqueue,
        onOpenArtist,
        onOpenContext,
    }: {
        artist: SpotifyArtistDetail | null;
        loading: boolean;
        destination: Destination;
        busy: Busy;
        targetRow: Snippet;
        onBack: () => void;
        onPick: (item: SpotifyItem) => void;
        onEnqueue: (item: SpotifyItem, next: boolean) => void;
        onOpenArtist: (uri: string) => void;
        onOpenContext: (uri: string) => void;
    } = $props();

    /** Top tracks past this stay behind "Show all" — ten rows is a page, not
     *  a shelf, and the discography below it is why you came. */
    const TOP_SHOWN = 5;
    let allTop = $state(false);
    // A different artist is a different page: whatever was expanded on the
    // last one has nothing to do with this one.
    $effect(() => {
        artist?.uri;
        allTop = false;
    });

    const top = $derived(artist?.top_tracks ?? []);
    const shownTop = $derived(allTop ? top : top.slice(0, TOP_SHOWN));

    /** Both discography shelves, only where the service answered for one. */
    const shelves = $derived(
        [
            { label: "Albums", items: artist?.albums ?? [] },
            { label: "Singles & EPs", items: artist?.singles ?? [] },
        ].filter((s) => s.items.length > 0),
    );

    const anything = $derived(
        top.length > 0 || shelves.length > 0 || (artist?.related?.length ?? 0) > 0,
    );

    /** Playing "the artist" means playing what they're best known for — the
     *  only honest reading, since no speaker takes an artist URI. */
    const starter = $derived(top[0]);

    let topList = $state<TrackList | null>(null);

    /** Escape closes an open row menu before it leaves the screen. */
    export function closeMenu(): boolean {
        return !!topList?.closeMenu();
    }
</script>

<div class="screen-head">
    <button class="icon-btn" aria-label="Back" onclick={onBack}>
        <Icon name="chevronLeft" size={18} />
    </button>
    <div class="screen-title">
        <h1>{artist?.name ?? "Artist"}</h1>
        <span class="screen-sub">Artist</span>
    </div>
    <span class="head-spacer" aria-hidden="true"></span>
</div>

<div class="ar" in:fly={{ y: 10, duration: dur(240), easing: cubicOut }}>
    {#if loading || !artist}
        <div class="skeleton sk-hero"></div>
        <div class="skeleton sk-row"></div>
        <div class="skeleton sk-row"></div>
        <div class="skeleton sk-row"></div>
    {:else}
        <!-- ── Who this is ────────────────────────────────────────────
             Portrait, name, the following spelled out, and the genres as
             plain chips. Everything the service answered for, nothing
             invented for the ones it didn't. -->
        <section class="card ar-hero">
            {#if artist.art_url}
                <img class="ar-face" src={artist.art_url} alt="" />
            {:else}
                <div class="ar-face placeholder">[ artist ]</div>
            {/if}
            <div class="ar-id">
                <h2 class="ar-name">{artist.name}</h2>
                {#if artist.followers}
                    <!-- The compact count reads at a glance; the exact one is
                         on hover for anyone who wants it. -->
                    <span class="ar-stat" title={fmtFollowers(artist.followers)}>
                        <span class="mono">{fmtCount(artist.followers)}</span> followers
                    </span>
                {/if}
                {#if artist.genres?.length}
                    <div class="ar-genres">
                        {#each artist.genres.slice(0, 3) as g (g)}
                            <span class="ar-genre">{capFirst(g)}</span>
                        {/each}
                    </div>
                {/if}
            </div>
        </section>

        <!-- Where anything started here comes out, plus the one-tap start.
             The picker sits above the button it feeds, not inside the head. -->
        <section class="card ar-play">
            <div class="ar-where">{@render targetRow()}</div>
            {#if starter}
                <button
                    class="btn btn-primary ar-start"
                    disabled={busy.is("item:" + starter.uri) || !destination.current}
                    onclick={() => onPick(starter)}
                >
                    <Icon name="play" size={15} />
                    Play {artist.name}
                </button>
                <p class="hint ar-start-note">
                    Starts <span class="ar-em">{starter.name}</span> — their most played.
                </p>
            {/if}
        </section>

        {#if !anything}
            <p class="ar-empty">
                Spotify didn't return any tracks or records for {artist.name}.
            </p>
        {/if}

        <!-- ── Popular ────────────────────────────────────────────────
             Numbered, because the order *is* the information here. -->
        {#if top.length > 0}
            <section class="block">
                <div class="ar-shelf-head">
                    <div class="eyrow">Popular</div>
                    {#if top.length > TOP_SHOWN}
                        <button class="chip" onclick={() => (allTop = !allTop)}>
                            {allTop ? "Show less" : `Show all ${top.length}`}
                        </button>
                    {/if}
                </div>
                <TrackList
                    items={shownTop}
                    numbered
                    {busy}
                    canPlay={!!destination.current}
                    queueTarget={destination.sonosTarget}
                    onPick={onPick}
                    {onEnqueue}
                    bind:this={topList}
                />
            </section>
        {/if}

        <!-- ── The records ────────────────────────────────────────────
             Albums and singles on separate shelves, the way Spotify splits
             them; a card says its year and its size, and a tap opens the
             listing rather than firing the whole record at a room. -->
        {#each shelves as shelf (shelf.label)}
            <section class="block">
                <div class="eyrow">{shelf.label}</div>
                <div class="ar-grid">
                    {#each shelf.items as item (item.uri)}
                        <MediaCard
                            {item}
                            sub={[item.year, item.total_tracks ? `${item.total_tracks} songs` : ""]
                                .filter(Boolean)
                                .join(" · ")}
                            onOpen={() => onOpenContext(item.uri)}
                        />
                    {/each}
                </div>
            </section>
        {/each}

        <!-- Spotify retired this endpoint for newer apps, so the shelf is
             absent rather than empty when it refuses. -->
        {#if artist.related?.length}
            <section class="block">
                <div class="eyrow">Fans also like</div>
                <div class="ar-grid">
                    {#each artist.related.slice(0, 8) as item (item.uri)}
                        <MediaCard
                            {item}
                            round
                            sub={item.followers ? `${fmtCount(item.followers)} followers` : "Artist"}
                            onOpen={() => onOpenArtist(item.uri)}
                        />
                    {/each}
                </div>
            </section>
        {/if}
    {/if}
</div>

<style>
    /* ── Screen head — the §11 shape ── */
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
        font-family: var(--font-mono);
        font-size: 10px; letter-spacing: 0.1em; text-transform: uppercase;
        color: var(--text-dim);
    }
    .head-spacer { width: 32px; height: 32px; flex-shrink: 0; }

    .ar { display: flex; flex-direction: column; gap: var(--space-5); margin-top: var(--space-4); }
    .sk-hero { height: 140px; border-radius: var(--r-lg); }
    .sk-row { height: 52px; border-radius: var(--r-md); }

    /* ── Identity ── */
    .ar-hero { display: flex; align-items: center; gap: var(--space-4); }
    .ar-face {
        width: 96px; height: 96px; flex-shrink: 0;
        border-radius: 50%; object-fit: cover;
        background: var(--card-2); border: 1px solid var(--hairline);
    }
    div.ar-face { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .ar-id { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 6px; }
    .ar-name {
        font-size: 24px; font-weight: 600; letter-spacing: -0.03em; line-height: 1.1;
        color: var(--text);
        overflow: hidden; text-overflow: ellipsis;
    }
    .ar-stat { font-size: 12.5px; color: var(--text-mute); }
    .ar-stat .mono { color: var(--text); font-feature-settings: "tnum" 1; }
    .ar-genres { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 2px; }
    .ar-genre {
        padding: 3px 9px; border-radius: var(--r-pill);
        background: var(--card-2); border: 1px solid var(--hairline);
        font-size: 11px; color: var(--text-mute);
    }

    /* ── Start it ── */
    .ar-play { display: flex; flex-direction: column; gap: var(--space-3); }
    .ar-where { display: flex; }
    .ar-start { align-self: flex-start; padding: 9px 18px; }
    .ar-start-note { margin-top: -4px; }
    .ar-em { color: var(--text); }

    .ar-empty { font-size: 12.5px; color: var(--text-mute); }

    .ar-shelf-head {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-2);
    }

    .ar-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(104px, 1fr));
        gap: var(--space-3);
    }
    @media (min-width: 700px) {
        .ar-grid { grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); }
    }
</style>
