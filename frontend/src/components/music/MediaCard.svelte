<script lang="ts">
    /**
     * A catalog card — big art over a name and one line of context — for
     * artists, albums and playlists wherever they're browsed: the search
     * carousels, an artist's discography, the "Fans also like" rail.
     *
     * The sub line is computed by the parent, because what makes a card
     * informative differs by kind: an artist card says its following, an
     * album card its year, a playlist card its owner and size.
     *
     * An artist's picture reads as a portrait (round); everything else as a
     * cover (rounded square).
     */
    import type { SpotifyItem } from "../../lib/types";

    let {
        item,
        round = false,
        sub,
        onOpen,
    }: {
        item: SpotifyItem;
        round?: boolean;
        sub?: string;
        onOpen: () => void;
    } = $props();
</script>

<button class="mc" onclick={onOpen}>
    {#if item.art_url}
        <img class="mc-art" class:round src={item.art_url} alt="" loading="lazy" />
    {:else}
        <div class="mc-art placeholder" class:round>[ art ]</div>
    {/if}
    <span class="mc-name">{item.name}</span>
    {#if sub}<span class="mc-sub">{sub}</span>{/if}
</button>

<style>
    .mc {
        display: flex; flex-direction: column; gap: 4px;
        width: 100%;
        background: transparent; border: 0; border-radius: var(--r-md);
        padding: 2px; color: var(--text); cursor: pointer;
        text-align: left; font: inherit;
    }
    .mc-art {
        width: 100%; aspect-ratio: 1;
        border-radius: var(--r-sm); object-fit: cover;
        background: var(--card-2); border: 1px solid var(--hairline);
        transition: transform 150ms ease;
    }
    div.mc-art { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .mc-art.round { border-radius: 50%; }
    .mc-name {
        margin-top: 4px;
        font-size: 12.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .mc-sub {
        font-size: 11px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    @media (hover: hover) {
        .mc:hover .mc-art { transform: scale(1.03); }
        .mc:hover .mc-name { color: var(--on); }
    }
    .mc:active { transform: scale(0.97); transition-duration: 80ms; }
    @media (prefers-reduced-motion: reduce) {
        .mc, .mc-art { transition-duration: 0.001ms; }
        .mc:hover .mc-art { transform: none; }
    }
</style>
