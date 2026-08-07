<script lang="ts">
    import { trimClock } from "../../lib/music/time";
    import type { PanelMusicStore } from "../../lib/panel-music.svelte";

    /**
     * The music band's foot: one row of large art tiles that answer the
     * question the transport can't — *what next* (DESIGN.md §16).
     *
     * The band's player is a strip, because a cover wide enough to fill the
     * band's height starves the title column beside it (the row is 960px and
     * the four columns have to share it). That leaves the band with real
     * height and nothing to put in it, and air is the wrong answer on the one
     * surface a house walks up to: the wall's most-wanted gesture after
     * play/pause is *put something on*, and until now that cost a trip one
     * depth in.
     *
     * One slot, and the room's state picks what fills it:
     *
     *   Up next   The queue past the track playing — tap a tile to jump to
     *             it. Only where the order is known: shuffle and repeat-one
     *             make "next" a guess, and the wall doesn't guess.
     *   Favorites The household's Sonos list — radio and whatever was
     *             starred. Tap plays it here. This is what a room that isn't
     *             playing from a queue gets, and it is the only thing a home
     *             without a linked Spotify account can start at all.
     *   Played    What this room played before (HomeHub's own memory, per
     *             room). The fallback that finally gives a KEF speaker or a
     *             HomeHub zone a shelf: both lists above are Sonos-only
     *             because both are Sonos' own, and until this existed those
     *             rooms spent a third of the wall's height on air.
     *
     * The last one is honest about whose plays it is showing. A room with
     * none of its own falls back to the household's, and the label says
     * "Played recently" rather than "Played here" — a wall must never imply
     * a room played something it didn't.
     */
    let { music }: { music: PanelMusicStore } = $props();

    const featured = $derived(music.featured);
    const sonos = $derived(featured?.kind === "sonos");

    /** Everything after the track playing. The head of the queue is already
     *  on the cover three feet to the left. */
    const upNext = $derived.by(() => {
        if (!sonos || !music.queueOrderKnown) return [];
        const at = featured?.queueTrack ?? 0;
        return music.queue.filter((q) => q.track > at);
    });
    const favorites = $derived(sonos ? music.favorites : []);
    const played = $derived(music.history);
    const mode = $derived(
        upNext.length > 0
            ? "next"
            : favorites.length > 0
              ? "fav"
              : played.length > 0
                ? "played"
                : "none",
    );
    const label = $derived(
        mode === "next"
            ? "Up next"
            : mode === "fav"
              ? "Favorites"
              : music.historyHousehold
                ? "Played recently"
                : "Played here",
    );
</script>

{#if mode !== "none"}
    <section class="shelf" aria-label={label}>
        <h3 class="s-label">{label}</h3>
        <div class="s-row">
            {#if mode === "next"}
                {#each upNext as q (q.track)}
                    <button
                        class="s-tile"
                        aria-label="Play {q.title || 'this track'}{q.artist
                            ? ` by ${q.artist}`
                            : ''} on {featured?.title ?? 'this room'}"
                        disabled={!!music.busy["jump:" + q.track]}
                        onclick={() => music.jumpTo(q.track)}
                    >
                        {#if q.art_uri}
                            <img class="s-art" src={q.art_uri} alt="" loading="lazy" />
                        {:else}
                            <span class="s-art placeholder">[ art ]</span>
                        {/if}
                        <span class="s-title">{q.title || "Untitled"}</span>
                        <span class="s-sub">
                            <span class="s-artist">{q.artist || ""}</span>
                            {#if q.duration}
                                <span class="s-dur mono">{trimClock(q.duration)}</span>
                            {/if}
                        </span>
                    </button>
                {/each}
            {:else if mode === "fav"}
                {#each favorites as f (f.id)}
                    <button
                        class="s-tile"
                        aria-label="Play {f.title} on {featured?.title ?? 'this room'}"
                        disabled={!!music.busy["fav:" + f.id]}
                        onclick={() => music.playFavorite(f)}
                    >
                        {#if f.art_uri}
                            <img class="s-art" src={f.art_uri} alt="" loading="lazy" />
                        {:else}
                            <span class="s-art placeholder">[ art ]</span>
                        {/if}
                        <span class="s-title">{f.title}</span>
                        {#if f.service}
                            <span class="s-sub"><span class="s-artist">{f.service}</span></span>
                        {/if}
                    </button>
                {/each}
            {:else}
                {#each played as p (p.uri)}
                    <button
                        class="s-tile"
                        aria-label="Play {p.title} again on {featured?.title ?? 'this room'}"
                        disabled={!!music.busy["hist:" + p.uri]}
                        onclick={() => music.playFromHistory(p)}
                    >
                        {#if p.art_uri}
                            <img class="s-art" src={p.art_uri} alt="" loading="lazy" />
                        {:else}
                            <span class="s-art placeholder">[ art ]</span>
                        {/if}
                        <span class="s-title">{p.title}</span>
                        {#if p.sub || (music.historyHousehold && p.room_name)}
                            <span class="s-sub">
                                <!-- On the household's list the room is the
                                     more useful second line than the artist:
                                     it is the part that says this shelf is
                                     not about the room you are looking at. -->
                                <span class="s-artist">
                                    {music.historyHousehold && p.room_name ? p.room_name : p.sub}
                                </span>
                            </span>
                        {/if}
                    </button>
                {/each}
            {/if}
        </div>
    </section>
{/if}

<style>
    .shelf {
        flex: none;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: var(--space-2);
    }
    /* The §4 micro-label. It is the only thing that says which of the two
       lists this is, and the two behave differently on a tap, so it is not
       decoration. */
    .s-label {
        margin: 0;
        font-family: var(--font-mono);
        font-size: 11px;
        font-weight: 500;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    /* One row, sideways past what fits — the same bargain the room band
       makes. A wrapped second row would eat the player's height above it,
       and the band's allocation is the panel's whole argument (§16). */
    .s-row {
        display: flex;
        gap: var(--space-3);
        overflow-x: auto;
        overflow-y: hidden;
        scrollbar-width: none;
        padding-bottom: 2px;
    }
    .s-row::-webkit-scrollbar {
        display: none;
    }

    .s-tile {
        flex: none;
        width: 132px;
        display: flex;
        flex-direction: column;
        gap: 7px;
        padding: 0;
        border: 0;
        background: none;
        color: inherit;
        font-family: inherit;
        text-align: left;
        cursor: pointer;
        border-radius: var(--r-md);
        transition: transform var(--t-fast);
    }
    .s-tile:active {
        transform: scale(0.96);
        transition-duration: 80ms;
    }
    .s-tile:disabled {
        opacity: 0.55;
    }
    .s-tile:focus-visible {
        box-shadow: var(--focus-ring);
    }

    /* The art is the tile. At arm's length on a wall a cover is a far better
       target than its title, so the square gets the tile's whole width and
       the words ride under it. */
    .s-art {
        display: block;
        width: 132px;
        height: 132px;
        object-fit: cover;
        border-radius: var(--r-md);
        /* A hairline so a cover that hasn't drawn — or a station that has
           none — still reads as a tile with an edge, rather than as a hole
           in the band. */
        border: 1px solid var(--hairline);
        background: var(--card-2);
    }
    span.s-art {
        display: grid;
        place-items: center;
        font-size: 11px;
        color: var(--text-dim);
    }
    .s-title {
        font-size: 13.5px;
        font-weight: 600;
        letter-spacing: -0.01em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .s-sub {
        display: flex;
        align-items: baseline;
        gap: var(--space-2);
        min-width: 0;
        font-size: 11.5px;
        color: var(--text-mute);
    }
    .s-artist {
        min-width: 0;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .s-dur {
        flex-shrink: 0;
        color: var(--text-dim);
    }

    /* Portrait: the page scrolls and the band is no longer an allocation, so
       the shelf keeps its shape at a size a phone can hold. */
    @media (orientation: portrait), (max-width: 900px) {
        .s-tile,
        .s-art {
            width: 108px;
        }
        .s-art {
            height: 108px;
        }
    }
</style>
