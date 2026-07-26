<script lang="ts">
    /**
     * The way to start something new from inside the player, on both sheets.
     *
     * One row: the search that feeds this room, then the searches already made
     * for it. The history is keyed by destination, so these are the kitchen's,
     * not the house's.
     *
     * Chips rather than a search box — the box lives on the Search sheet, and
     * a second one here would be a second thing to focus, keep and clear.
     * Sheets swap, so the player hands over rather than growing a copy of what
     * it hands over to.
     *
     * Nothing to offer is a reason to render nothing, not a heading over an
     * empty row.
     */
    import type { Snippet } from "svelte";
    import Icon from "../Icon.svelte";
    import type { SonosFavorite } from "../../lib/types";

    let {
        /** Null when Spotify isn't set up at all — then the row isn't shown. */
        spotifyAvailable,
        spotifyConnected,
        /** This destination's recent searches, already cut to a row. */
        recents,
        /**
         * Favorites to offer below the row, and empty when there are none to
         * show — a playing group (it already has something) or a KEF speaker
         * (favorites are a Sonos household list).
         */
        favorites = [],
        onSearch,
        favCard,
    }: {
        spotifyAvailable: boolean;
        spotifyConnected: boolean;
        recents: string[];
        favorites?: SonosFavorite[];
        /** `q` runs that search straight away rather than only opening the box. */
        onSearch: (q?: string) => void;
        favCard: Snippet<[SonosFavorite]>;
    } = $props();
</script>

{#if spotifyAvailable || favorites.length > 0}
    <div class="p-idle">
        <div class="eyrow">Start something</div>
        {#if spotifyAvailable}
            <div class="start-row h-scroll">
                <!-- Not gated on being connected, for the same reason Home's
                     card isn't: the people who most need the pointer are the
                     ones a gate would hide it from. -->
                <button class="chip start-go" onclick={() => onSearch()}>
                    <Icon name="search" size={13} />
                    <span>{spotifyConnected ? "Search Spotify" : "Set up Spotify"}</span>
                </button>
                {#if spotifyConnected}
                    {#each recents as h (h)}
                        <button class="chip start-recent" onclick={() => onSearch(h)}>
                            <span>{h}</span>
                        </button>
                    {/each}
                {/if}
            </div>
        {/if}
        {#if favorites.length > 0}
            <div class="favs h-scroll">
                {#each favorites as f (f.id)}
                    {@render favCard(f)}
                {/each}
            </div>
        {/if}
    </div>
{/if}

<style>
    .p-idle { display: flex; flex-direction: column; gap: var(--space-3); }
    .start-row { align-items: center; }
    /* The row's primary action, marked the way every other lead chip in the
       module is — a stronger edge and full-strength text, not a new colour. */
    .start-go { color: var(--text); border-color: var(--border-strong); }
    /* A recent search is whatever was typed, so it is capped rather than
       trusted to be short. */
    .start-recent > span { display: block; max-width: 52vw; overflow: hidden; text-overflow: ellipsis; }
    .favs { display: flex; gap: var(--space-3); padding-bottom: var(--space-1); }
    @media (pointer: coarse) {
        .start-row .chip { min-height: 44px; padding-inline: 14px; }
    }
</style>
