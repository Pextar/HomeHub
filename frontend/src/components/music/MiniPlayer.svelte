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
    import TrackRail from "./TrackRail.svelte";
    import Slider from "./Slider.svelte";
    import { haptic } from "../../lib/utils";
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
        /** The desktop player bar's scrubber — present only when the room
         *  has a track with a length it can seek into. */
        seek = undefined,
        /** The desktop player bar's volume cluster. */
        volume = undefined,
        /** Handed the bar itself, so the player can unfold out of it rather
         *  than arriving over it as a second surface. */
        onOpen,
        transport,
    }: {
        title: string;
        sub: string;
        artUri?: string;
        playing: boolean;
        progress?: number;
        overSheet?: boolean;
        seek?: { position: number; duration: number; onSeek: (sec: number) => void };
        volume?: {
            value: number;
            muted: boolean;
            onInput: (v: number) => void;
            onChange: (v: number) => void;
            onToggleMute: () => void;
        };
        onOpen: (from?: HTMLElement) => void;
        transport: Snippet;
    } = $props();

    let bar = $state<HTMLElement | null>(null);
    const open = () => onOpen(bar ?? undefined);
</script>

<div
    class="mini"
    class:paused={!playing}
    class:over-sheet={overSheet}
    class:has-scrub={!!seek}
    bind:this={bar}
    transition:fly={{ y: 20, duration: dur(220), easing: cubicOut }}
>
    <button class="mini-open" onclick={open}>
        {#if artUri}
            <!-- The one element the dock and the player share: it flies
                 between the two sizes rather than being redrawn. -->
            <img class="mini-art" data-morph src={artUri} alt="" loading="lazy" />
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
    <div class="mini-center">
        {@render transport()}
        {#if seek}
            <div class="mini-scrub">
                <TrackRail
                    inline
                    position={seek.position}
                    duration={seek.duration}
                    seekable
                    onSeek={seek.onSeek}
                />
            </div>
        {/if}
    </div>
    <div class="mini-right">
        {#if volume}
            <button
                class="icon-btn mini-mute"
                aria-label={volume.muted ? "Unmute" : "Mute"}
                aria-pressed={volume.muted}
                onclick={() => {
                    haptic();
                    volume.onToggleMute();
                }}
            >
                <Icon name={volume.muted ? "volumeOff" : "volume"} size={16} />
            </button>
            <div class="mini-vol">
                <Slider
                    value={volume.value}
                    label="Volume"
                    onInput={volume.onInput}
                    onChange={volume.onChange}
                />
            </div>
            <span class="mini-volnum mono">{volume.value}</span>
        {/if}
        <button class="icon-btn mini-expand" aria-label="Open the player" onclick={open}>
            <Icon name="chevronUp" size={16} />
        </button>
    </div>
    <ProgressLine value={progress} />
</div>

<style>
    .mini {
        position: sticky;
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: var(--z-menu);
        overflow: hidden;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 9px 10px;
        margin-top: var(--space-2);
        background: var(--tile-on-gradient);
        border: 1px solid var(--tile-on-border);
        border-radius: var(--r-lg);
        box-shadow: var(--shadow-md);
        /* Padding animates so the bar glides into the gutter as the FAB it
           was dodging scales away, instead of snapping the moment it goes. */
        transition:
            background var(--t-med),
            border-color var(--t-med),
            padding-right var(--t-med);
    }
    /* Held open after a pause: nothing is playing, so it drops the "ON"
       surface a lit device gets and reads as a plain card. */
    .mini.paused {
        background: var(--card);
        border-color: var(--hairline);
    }
    .mini-idle {
        display: flex;
        color: var(--text-mute);
        flex-shrink: 0;
    }
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
        left: var(--space-4);
        right: var(--space-4);
        bottom: calc(var(--space-4) + env(safe-area-inset-bottom));
        z-index: var(--z-dock);
        margin-top: 0;
    }
    @media (min-width: 601px) {
        .mini.over-sheet {
            left: 50%;
            right: auto;
            transform: translateX(-50%);
            width: min(440px, calc(100vw - 48px));
        }
    }
    .mini-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-3);
        background: none;
        border: 0;
        padding: 0;
        color: var(--text);
        text-align: left;
        cursor: pointer;
    }
    .mini-art {
        width: 40px;
        height: 40px;
        border-radius: var(--r-md);
        object-fit: cover;
        background: var(--card-3);
        flex-shrink: 0;
    }
    .mini-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 1px;
    }
    .mini-t {
        font-size: 13px;
        font-weight: 600;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }
    .mini-s {
        font-size: 11px;
        color: var(--text-mute);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    /* The desktop player bar. Below the shell's breakpoint the three zones
       collapse back into the phone's single row: the center's contents join
       it directly, the right cluster and the scrubber wait for width. */
    .mini-center {
        display: contents;
    }
    .mini-right,
    .mini-scrub {
        display: none;
    }
    @media (min-width: 901px) {
        .mini {
            bottom: var(--space-4);
            gap: var(--space-4);
            padding: 10px 12px;
        }
        .mini-open {
            flex: 0 1 300px;
            min-width: 128px;
        }
        .mini-art {
            width: 46px;
            height: 46px;
        }
        .mini-center {
            flex: 1;
            min-width: 0;
            display: flex;
            align-items: center;
            justify-content: center;
            gap: var(--space-4);
        }
        .mini-scrub {
            display: block;
            flex: 1;
            min-width: 96px;
            max-width: 520px;
        }
        .mini-right {
            display: flex;
            align-items: center;
            gap: var(--space-2);
            flex-shrink: 0;
        }
        .mini-mute,
        .mini-expand {
            width: 38px;
            height: 38px;
            border-radius: 50%;
            color: var(--text-mute);
        }
        .mini-expand {
            background: var(--card-3);
            border: 1px solid var(--hairline);
            color: var(--text);
        }
        /* The volume slider waits for real width; the mute and the way into
           the player fit anywhere. */
        .mini-vol,
        .mini-volnum {
            display: none;
        }
        /* The hairline yields to the real scrubber. */
        .mini.has-scrub :global(.prog) {
            display: none;
        }
    }
    @media (min-width: 1150px) {
        .mini-vol {
            display: block;
            width: 104px;
        }
        .mini-volnum {
            display: block;
            font-size: 12px;
            color: var(--text-mute);
            width: 3ch;
            text-align: right;
        }
    }

    @media (prefers-reduced-motion: reduce) {
        .mini {
            transition-duration: 0.001ms;
        }
    }
</style>
