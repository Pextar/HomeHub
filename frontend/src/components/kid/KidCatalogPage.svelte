<script lang="ts">
    /**
     * One level deeper than the kid's search results (DESIGN.md §17): an
     * artist's page, or a record's track listing.
     *
     * Both are the same page with different shelves — a big cover, the name,
     * one fact under it, and one obvious button — so they are one component.
     * Split in two they would each carry a copy of the hero, which is how the
     * two versions of a thing start disagreeing about how big the cover is.
     *
     * The two differences are stated rather than styled around. An artist's
     * picture is round and a record's is square. And no speaker takes an
     * artist URI, so an artist's button starts their top song and *says* so
     * underneath, rather than promising "play" and doing something the reader
     * didn't pick; a record plays whole, which is what "Play all" means.
     *
     * Back climbs exactly one level, never out of the module.
     */
    import KidTrackRow from "./KidTrackRow.svelte";
    import KidMediaCard from "./KidMediaCard.svelte";
    import { KIND_EMOJI } from "./kind-emoji";
    import { contextItem } from "../../lib/music/catalog-cache.svelte";
    import { fmtCount, capFirst } from "../../lib/music/format";
    import type { CatalogStack } from "../../lib/music/catalog-stack.svelte";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        catalog,
        music,
        kbOpen = false,
        /** Which row has its queue drawer open, and which just flashed. Held
         *  by the pane, since a row's drawer outlives a level. */
        queueFor,
        queuedFlash,
        onPick,
        onToggleQueue,
        onQueue,
        /** A tap on a related artist: opens rather than plays. */
        onAct,
    }: {
        catalog: CatalogStack;
        music: PanelMusicStore;
        kbOpen?: boolean;
        queueFor: string | null;
        queuedFlash: string | null;
        onPick: (item: SpotifyItem) => void;
        onToggleQueue: (uri: string) => void;
        onQueue: (item: SpotifyItem, next: boolean) => void;
        onAct: (item: SpotifyItem) => void;
    } = $props();

    const level = $derived(catalog.top);
</script>

{#snippet shelfLabel(text: string)}
    <h3 class="kms-label">{text}</h3>
{/snippet}

{#snippet rows(items: SpotifyItem[], numbered: boolean)}
    {#each items as t, i (t.uri)}
        <KidTrackRow
            item={t}
            num={numbered ? i + 1 : null}
            {music}
            {kbOpen}
            queueOpen={queueFor === t.uri}
            flashed={queuedFlash === t.uri}
            onPick={onPick}
            onToggleQueue={onToggleQueue}
            onQueue={onQueue}
        />
    {/each}
{/snippet}

{#snippet grid(items: SpotifyItem[], pick: (item: SpotifyItem) => void)}
    <div class="kms-grid">
        {#each items as it (it.uri)}
            <KidMediaCard item={it} onPick={pick} />
        {/each}
    </div>
{/snippet}

<button class="kms-back" onclick={() => void catalog.pop()} aria-label="Back one level">‹ Back</button>

{#if level?.kind === "artist"}
    {#if catalog.artistLoading && !catalog.artistDetail}
        <div class="kms-skel-hero" aria-hidden="true"></div>
    {:else if catalog.artistDetail}
        {@const d = catalog.artistDetail}
        <header class="kms-hero">
            {#if d.art_url}
                <img class="kms-hero-art round" src={d.art_url} alt="" />
            {:else}
                <span class="kms-hero-art kms-card-none round" aria-hidden="true">{KIND_EMOJI.artist}</span>
            {/if}
            <span class="kms-hero-name">{d.name}</span>
            {#if d.followers || d.genres?.length}
                <span class="kms-hero-sub">
                    {[
                        d.followers ? `${fmtCount(d.followers)} followers` : "",
                        d.genres?.[0] ? capFirst(d.genres[0]) : "",
                    ]
                        .filter(Boolean)
                        .join(" · ")}
                </span>
            {/if}
            {#if d.top_tracks[0]}
                <button
                    class="kms-bigplay"
                    disabled={!!music.busy["item:" + d.uri]}
                    onclick={() => onPick({ kind: "artist", uri: d.uri, name: d.name, art_url: d.art_url })}
                >
                    ▶️ Play
                </button>
                <span class="kms-playnote">Plays “{d.top_tracks[0].name}”</span>
            {/if}
        </header>

        {#if d.top_tracks.length > 0}
            {@render shelfLabel(`${KIND_EMOJI.track} Popular songs`)}
            {@render rows(d.top_tracks, true)}
        {/if}
        {#if d.albums.length > 0}
            {@render shelfLabel(`${KIND_EMOJI.album} Albums`)}
            {@render grid(d.albums, (x) => void catalog.openContext(x.uri))}
        {/if}
        {#if d.singles.length > 0}
            {@render shelfLabel(`${KIND_EMOJI.album} Singles`)}
            {@render grid(d.singles, (x) => void catalog.openContext(x.uri))}
        {/if}
        {#if d.related.length > 0}
            {@render shelfLabel(`${KIND_EMOJI.artist} More like them`)}
            {@render grid(d.related, onAct)}
        {/if}
    {/if}
{:else if catalog.contextLoading && !catalog.contextDetail}
    <div class="kms-skel-hero" aria-hidden="true"></div>
{:else if catalog.contextDetail}
    {@const d = catalog.contextDetail}
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
            onclick={() => onPick(contextItem(d))}
        >
            ▶️ Play all
        </button>
    </header>

    {@render shelfLabel(`${KIND_EMOJI.track} Songs`)}
    <!-- An album is numbered because its order is the record's; a playlist
         is not, because its order is whoever made it. -->
    {@render rows(d.tracks, d.kind === "album")}
{/if}

<style>
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
    .kms-card-none {
        background: var(--surface-hover);
        display: grid;
        place-items: center;
        font-size: 2.6rem;
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

    .kms-label {
        font-size: 1.05rem;
        font-weight: 800;
        letter-spacing: -0.01em;
        color: var(--text);
        margin-top: var(--space-3);
    }
    .kms-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(min(140px, 45%), 1fr));
        gap: var(--space-3);
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
    @media (prefers-reduced-motion: reduce) {
        .kms-skel-hero { animation: none; }
    }
</style>
