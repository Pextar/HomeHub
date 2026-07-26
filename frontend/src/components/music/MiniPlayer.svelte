<script lang="ts">
    /**
     * The docked mini-player: art, track, waveform, transport, stuck to the
     * bottom of the view.
     *
     * It is a **fallback, never a duplicate** (DESIGN.md §15). It carries the
     * same track and the same transport as Home's "Playing now" card — both
     * gain prev/next from 430px up and drop them below it, so neither is ever
     * the richer control — and so it stands down while that card is on screen
     * and appears the moment the card scrolls away.
     *
     * It **survives a pause**: "Playing now" means playing and lets go of the
     * zone, so the dock is where a paused zone stays one tap from playing
     * again. Paused, it drops the `.tile.on` surface for a plain card and
     * swaps the waveform for the idle speaker icon — nothing is moving, so
     * nothing should say it is.
     */
    import type { Snippet } from "svelte";
    import { fly } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import Icon from "../Icon.svelte";
    import Waveform from "./Waveform.svelte";
    import ProgressLine from "./ProgressLine.svelte";
    import { dur } from "../../lib/motion";

    let {
        title,
        sub,
        artUri = undefined,
        playing,
        progress = 0,
        /** True while a sheet is up: the dock leaves the page flow and floats
         *  over it, because that is exactly where the transport would
         *  otherwise disappear. */
        overSheet = false,
        onOpen,
        transport,
    }: {
        title: string;
        sub: string;
        artUri?: string;
        playing: boolean;
        progress?: number;
        overSheet?: boolean;
        onOpen: () => void;
        transport: Snippet;
    } = $props();
</script>

<div
    class="mini"
    class:paused={!playing}
    class:over-sheet={overSheet}
    transition:fly={{ y: 20, duration: dur(220), easing: cubicOut }}
>
    <button class="mini-open" onclick={onOpen}>
        {#if artUri}
            <img class="mini-art" src={artUri} alt="" loading="lazy" />
        {:else}
            <div class="mini-art placeholder"></div>
        {/if}
        <div class="mini-meta">
            <div class="mini-t">{title}</div>
            <div class="mini-s">{sub}</div>
        </div>
        <!-- Playing is a waveform; a zone the dock is holding open after a
             pause gets the idle speaker icon instead. -->
        {#if playing}
            <Waveform />
        {:else}
            <span class="mini-idle" aria-hidden="true"><Icon name="speaker" size={14} /></span>
        {/if}
    </button>
    {@render transport()}
    <ProgressLine value={progress} />
</div>

<style>
    .mini {
        position: sticky;
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: 30;
        overflow: hidden;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 9px 10px;
        margin-top: var(--space-2);
        background: var(--tile-on-gradient);
        border: 1px solid var(--tile-on-border);
        border-radius: var(--r-lg);
        box-shadow: var(--shadow-md);
        /* Padding animates so the bar glides into the gutter as the FAB it
           was dodging scales away, instead of snapping the moment it goes. */
        transition: background var(--t-med), border-color var(--t-med),
            padding-right var(--t-med);
    }
    /* Held open after a pause: nothing is playing, so it drops the "ON"
       surface a lit device gets and reads as a plain card. */
    .mini.paused { background: var(--card); border-color: var(--hairline); }
    .mini-idle { display: flex; color: var(--text-mute); flex-shrink: 0; }
    @media (max-width: 900px) {
        .mini {
            bottom: calc(var(--nav-clear) + var(--space-3));
            /* A reserved gutter for the assistant button, which shares this
               band — and the same reprieve when that button is switched off. */
            padding-right: max(10px, var(--fab-clear));
        }
    }
    /* Over the Zones and Search sheets the dock leaves the page flow and
       floats above them, because the transport has to persist across all
       three (DESIGN.md §15). Above the sheet's own z-index, below the
       player's, since tapping it swaps one for the other. */
    .mini.over-sheet {
        position: fixed;
        left: var(--space-4); right: var(--space-4);
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: 127;
        margin-top: 0;
    }
    @media (min-width: 601px) {
        .mini.over-sheet {
            left: 50%; right: auto;
            transform: translateX(-50%);
            width: min(440px, calc(100vw - 48px));
        }
    }
    .mini-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        background: none; border: 0; padding: 0;
        color: var(--text); text-align: left; cursor: pointer;
    }
    .mini-art {
        width: 40px; height: 40px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-3); flex-shrink: 0;
    }
    .mini-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .mini-t {
        font-size: 13px; font-weight: 600;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .mini-s {
        font-size: 11px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }


    @media (prefers-reduced-motion: reduce) {
        .mini { transition-duration: 0.001ms; }
    }
</style>
