<script lang="ts">
    /**
     * The player, while the results have the screen.
     *
     * Cover, one line of what is on, and the transport. Same rule as every
     * other region on this surface — a hairline above it rather than a gap,
     * its own padding, and no card of its own. It is 88px, which is what a
     * 64px cover and §2's target floor come to, and it does not grow: the
     * whole point of the mode is that the height belongs to the list.
     *
     * With the keyboard up it becomes one line. It cannot simply go: the
     * queue buttons on the rows are tappable while typing, and queueing is
     * the one action here that changes nothing on screen — this is where
     * that answer lives, so removing it would make a tap answer with
     * nothing. So it keeps the answer and gives up everything else: a 36px
     * cover, one line, and play/pause. The skips go too — you are typing,
     * not conducting.
     *
     * The dense mode arrives as a prop rather than being inherited from an
     * ancestor's class, because a component's styles are scoped and the
     * depth's `.kb-open .d-art` would no longer reach in here.
     */
    import { fly } from "svelte/transition";
    import Icon from "../Icon.svelte";
    import { dur } from "../../lib/motion";
    import type { PanelRooms, PanelSource, PanelTransport } from "../../lib/panel-music.svelte";

    let {
        music,
        featured,
        kbOpen = false,
        queuedLine = null,
        onBack,
    }: {
        music: PanelRooms & PanelTransport;
        featured: PanelSource | undefined;
        /** The software keyboard is up, so the dock goes to one line. */
        kbOpen?: boolean;
        /** The freshest true thing to say instead of the track line — a
         *  queued track changes nothing else on this screen, and the card
         *  that used to say so is what this strip replaced. */
        queuedLine?: string | null;
        /** Leave the search and go back to the player column. */
        onBack: () => void;
    } = $props();
</script>

<div class="b-dock" class:kb-open={kbOpen} in:fly={{ y: 14, duration: dur(180) }}>
    {#if featured}
        <button class="d-open" onclick={onBack} aria-label="Back to the player">
            {#if featured.art}
                <img class="d-art" src={featured.art} alt="" />
            {:else}
                <span class="d-art placeholder">[ art ]</span>
            {/if}
            <span class="d-meta">
                <span class="d-title">
                    {featured.trackTitle ??
                        (featured.playing ? "Playing" : "Nothing playing")}
                </span>
                <!-- One line, and the freshest true thing goes on
                     it: a queued track changes nothing else on
                     this screen, and the card that used to say so
                     is the thing this strip replaced. -->
                <span class="d-sub" class:said={!!queuedLine}>
                    {queuedLine ?? `${featured.trackSub || "—"} · ${featured.title}`}
                </span>
            </span>
        </button>
        <div class="d-transport">
            {#if featured.canSkip}
                <button
                    class="d-btn"
                    aria-label="Previous"
                    disabled={!!music.busy["previous:" + featured.id]}
                    onclick={() => music.skip(featured, "previous")}
                >
                    <Icon name="skipPrev" size={18} />
                </button>
            {/if}
            <button
                class="d-btn d-play"
                aria-label={featured.playing ? "Pause" : "Play"}
                disabled={!!music.busy["play:" + featured.id]}
                onclick={() => music.togglePlay(featured)}
            >
                <Icon name={featured.playing ? "pause" : "play"} size={20} />
            </button>
            {#if featured.canSkip}
                <button
                    class="d-btn"
                    aria-label="Next"
                    disabled={!!music.busy["next:" + featured.id]}
                    onclick={() => music.skip(featured, "next")}
                >
                    <Icon name="skipNext" size={18} />
                </button>
            {/if}
        </div>
    {:else}
        <p class="d-nosrc">No speaker is answering</p>
    {/if}
</div>

<style>
    .b-dock {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        min-height: 88px;
        padding: var(--space-3) var(--space-6);
        border-top: 1px solid var(--hairline);
    }
    /* The cover is the way back to the column — the same bargain the band's
       cover makes one depth out, so the biggest thing on the strip is the
       control rather than a 44px chip beside it. */
    .d-open {
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: var(--space-4);
        padding: var(--space-1);
        border: 0;
        border-radius: var(--r-md);
        background: none;
        color: inherit;
        font: inherit;
        text-align: left;
        cursor: pointer;
        transition: background var(--t-fast);
    }
    .d-art {
        width: 64px;
        height: 64px;
        flex-shrink: 0;
        display: block;
        object-fit: cover;
        border-radius: var(--r-sm);
        border: 1px solid var(--hairline);
        background: var(--card-2);
    }
    span.d-art {
        display: grid;
        place-items: center;
        font-size: 10px;
        color: var(--text-dim);
    }
    .d-meta {
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 3px;
    }
    .d-title {
        font-size: 16px;
        font-weight: 600;
        letter-spacing: -0.01em;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .d-sub {
        font-size: 12.5px;
        color: var(--text-mute);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    /* The queued line takes the same slot in the ON ink for its few
       seconds: it is the answer to the tap that was just made — so it
       arrives rather than appearing, which is what tells a glance that
       something just happened. A class-triggered animation, not a keyed
       block: swapping the node would put two lines in the flex column for
       the length of a crossfade and move the title above it. */
    .d-sub.said {
        color: var(--on);
        animation: dock-said 160ms ease-out both;
    }
    @keyframes dock-said {
        from {
            opacity: 0;
            transform: translateY(3px);
        }
        to {
            opacity: 1;
            transform: none;
        }
    }
    .d-transport {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        flex-shrink: 0;
    }
    .d-btn {
        width: 48px;
        height: 48px;
        display: grid;
        place-items: center;
        border: 1px solid var(--hairline);
        border-radius: 50%;
        background: var(--card-2);
        color: var(--text);
        cursor: pointer;
        transition:
            background var(--t-fast),
            transform var(--t-fast);
    }
    .d-btn:active {
        transform: scale(0.94);
        transition-duration: 80ms;
    }
    .d-btn:disabled {
        opacity: 0.5;
    }
    .d-play {
        width: 56px;
        height: 56px;
        background: var(--card-3);
    }
    .d-nosrc {
        margin: 0;
        font-size: 13px;
        color: var(--text-dim);
    }

    /* With the keyboard up the dock is one line. It cannot simply go: the
       queue buttons on the rows are tappable while typing, and queueing is
       the one action here that changes nothing on screen — the dock is
       where that answer lives, so removing it would make a tap answer with
       nothing. So it keeps the answer and gives up everything else: a
       36px cover, one line, and play/pause. The skips go too — you are
       typing, not conducting — and the second line only appears when it
       has something to confirm. */
    .b-dock.kb-open {
        min-height: 52px;
        gap: var(--space-3);
        padding: 4px var(--space-6);
    }
    .b-dock.kb-open .d-art {
        width: 36px;
        height: 36px;
    }
    .b-dock.kb-open .d-title {
        font-size: 14px;
    }
    .b-dock.kb-open .d-sub:not(.said) {
        display: none;
    }
    .b-dock.kb-open .d-sub.said {
        font-size: 12px;
    }
    .b-dock.kb-open .d-btn:not(.d-play) {
        display: none;
    }
    .b-dock.kb-open .d-play {
        width: 44px;
        height: 44px;
    }
    @media (hover: hover) {
        .d-open:hover {
            background: var(--card-2);
        }
        .d-btn:hover {
            background: var(--card-3);
        }
    }

    /* Collapsed, not removed: the state it animates into is the state
       either way. */
    @media (prefers-reduced-motion: reduce) {
        .d-sub.said {
            animation-duration: 0.001ms;
        }
    }
</style>
