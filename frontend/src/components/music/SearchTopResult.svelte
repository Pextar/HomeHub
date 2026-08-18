<script lang="ts">
    /**
     * The one thing you almost certainly meant, at full size (DESIGN.md
     * §15.9): the biggest object on the search screen, because it is the
     * answer most searches were after.
     *
     * Two rules about what a tap here does, and both are the reason the card
     * has the shape it has.
     *
     * **Nothing plays a guess.** The card's body *opens* — an artist to their
     * page, an album or playlist to its track listing — and only a song plays
     * outright. So the card says where the tap goes in words rather than
     * leaving it to a chevron: "See top tracks & albums", "See what's on it".
     *
     * **A container is both a place and a thing to play**, so an album or a
     * playlist gets an explicit Play beside the way in. An artist doesn't:
     * there is no artist URI a speaker takes, so the button would have to
     * pick something on their behalf.
     */
    import Icon from "../Icon.svelte";
    import { topLine } from "../../lib/music/catalog";
    import type { Busy } from "../../lib/music/busy.svelte";
    import type { SpotifyItem } from "../../lib/types";

    let {
        item,
        busy,
        /** Where a tap would play, and whether there is anywhere at all. */
        canPlay,
        destinationLabel,
        /** True for the kinds a speaker can be handed directly. */
        playable,
        onOpen,
        onPlay,
    }: {
        item: SpotifyItem;
        busy: Busy;
        canPlay: boolean;
        destinationLabel: string;
        playable: boolean;
        onOpen: (item: SpotifyItem) => void;
        onPlay: (item: SpotifyItem) => void;
    } = $props();
</script>

<div class="sp-top">
    <button
        class="sp-top-open"
        onclick={() => onOpen(item)}
        aria-label={item.kind === "track" ? `Play ${item.name}` : `Open ${item.name}`}
    >
        {#if item.art_url}
            <img class="sp-top-art-img" class:round={item.kind === "artist"} src={item.art_url} alt="" />
        {:else}
            <div class="sp-top-art-img placeholder" class:round={item.kind === "artist"}>[ art ]</div>
        {/if}
        <span class="sp-top-meta">
            <span class="sp-top-name">{item.name}</span>
            <span class="sp-top-line">{topLine(item)}</span>
            {#if item.kind !== "track"}
                <span class="sp-top-cta">
                    {item.kind === "artist" ? "See top tracks & albums" : "See what's on it"}
                    <Icon name="chevronLeft" size={13} />
                </span>
            {/if}
        </span>
    </button>
    {#if playable}
        <button
            class="sp-top-play"
            disabled={busy.is("item:" + item.uri) || !canPlay}
            aria-label={`Play ${item.name}${destinationLabel ? " on " + destinationLabel : ""}`}
            onclick={() => onPlay(item)}
        >
            <Icon name="play" size={20} />
        </button>
    {/if}
</div>

<style>
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

    @media (prefers-reduced-motion: reduce) {
        .sp-top, .sp-top-play { transition-duration: 0.001ms; }
    }
</style>
