<script lang="ts">
    /**
     * One entry from the Sonos household's favorites: tap the art to play it
     * on `target`, or the corner `+` to queue it without interrupting.
     *
     * Shared by the Home shelf and the idle player, which is the point of the
     * corner button being where it is — "queueing never interrupts"
     * (DESIGN.md §15) needs an affordance that doesn't compete with the tap.
     */
    import Icon from "../Icon.svelte";
    import type { SonosFavorite } from "../../lib/types";

    let {
        favorite: f,
        /** The Sonos coordinator this lands on. Null disables both controls. */
        target,
        playBusy = false,
        queueBusy = false,
        onPlay,
        onQueue,
    }: {
        favorite: SonosFavorite;
        target: string | null;
        playBusy?: boolean;
        queueBusy?: boolean;
        /** Plays the favorite outright, or — when it's a Spotify list — opens
         *  it to show the songs inside. Which one is the caller's call. */
        onPlay: () => void;
        onQueue: () => void;
    } = $props();

    /** A Spotify playlist/album favorite opens rather than plays outright —
     *  the corner mark says so before the tap, the same honesty the queue's
     *  "+" already carries. */
    const browsable = $derived(!!f.spotify_uri);
</script>

<div class="fav">
    <button class="fav-play" disabled={playBusy || !target} onclick={onPlay}
        aria-label={browsable ? `Open ${f.title}` : `Play ${f.title}`}>
        {#if f.art_uri}
            <img class="fav-art" src={f.art_uri} alt="" loading="lazy" />
        {:else}
            <div class="fav-art placeholder">[ art ]</div>
        {/if}
        {#if browsable}
            <span class="fav-list" aria-hidden="true"><Icon name="chevronLeft" size={12} /></span>
        {/if}
        <span class="fav-title">{f.title}</span>
        {#if f.service}<span class="fav-sub mono">{f.service}</span>{/if}
    </button>
    <button
        class="icon-btn fav-add"
        aria-label="Add {f.title} to the queue"
        disabled={queueBusy || !target}
        onclick={onQueue}
    >
        <Icon name="plus" size={14} />
    </button>
</div>

<style>
    .fav { position: relative; width: 112px; }
    .fav-play {
        display: flex; flex-direction: column; gap: 6px; width: 100%;
        background: transparent; border: 0; padding: 0;
        cursor: pointer; text-align: left; color: var(--text); font: inherit;
    }
    .fav-play:disabled { opacity: 0.5; cursor: default; }
    .fav-art {
        width: 112px; height: 112px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-2);
        border: 1px solid var(--hairline);
        transition: transform 120ms ease;
    }
    div.fav-art { display: grid; place-items: center; font-size: 10px; color: var(--text-dim); }
    @media (hover: hover) { .fav-play:hover .fav-art { transform: translateY(-1px); } }
    .fav-play:active .fav-art { transform: scale(0.97); }
    .fav-title {
        font-size: 12.5px; font-weight: 500;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .fav-sub { font-size: 10px; color: var(--text-dim); letter-spacing: 0.04em; }
    /* Queue-without-interrupting, parked on the art's corner. */
    .fav-add {
        position: absolute; top: 6px; right: 6px;
        width: 30px; height: 30px; border-radius: 50%;
        background: var(--bg-bar); border: 1px solid var(--hairline);
        color: var(--text);
        backdrop-filter: blur(6px);
    }
    .fav-add:disabled { opacity: 0.4; }
    @media (pointer: coarse) {
        .fav-add { width: 44px; height: 44px; }
    }
    /* "There's a list inside — tap to open it", parked on the art's bottom
       corner opposite the queue "+" so the two marks never collide. `top`
       rather than `bottom`, measured off the art's own fixed 112px height —
       `bottom` would drift with the title/subtitle text below it. */
    .fav-list {
        position: absolute; top: 84px; left: 6px;
        width: 22px; height: 22px; border-radius: 50%;
        display: grid; place-items: center; transform: rotate(180deg);
        background: var(--bg-bar); border: 1px solid var(--hairline);
        color: var(--text-mute);
        backdrop-filter: blur(6px);
    }
    @media (prefers-reduced-motion: reduce) {
        .fav-art { transition-duration: 0.001ms; }
    }
</style>
