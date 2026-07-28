<script lang="ts">
    /**
     * How far through the track we are — as a scrubber where the room can
     * actually seek, and as a read-only rail where it can't.
     *
     * The difference is not cosmetic and not a matter of make: a Sonos group
     * playing a queued track can seek, the same group playing radio cannot,
     * and neither a KEF speaker nor a streamed zone has a seek endpoint at all.
     * A rail you can drag but that gets refused is worse than one you can't, so
     * the same component draws both and the room says which it is.
     *
     * A source with no duration at all (radio, line-in, TV) gets one honest
     * line instead of a made-up position.
     */
    import Slider from "./Slider.svelte";
    import { fmtSecs } from "../../lib/music/time";

    let {
        position,
        duration,
        seekable = false,
        /** Said instead of a rail when the source reports no duration. */
        liveLabel = "live stream — no track position",
        /** Nothing at all when there is no track loaded to describe. */
        idle = false,
        /** The desktop player bar's shape: times flanking the rail in one
         *  row. A source with no duration just renders nothing — the bar
         *  doesn't need the caption. */
        inline = false,
        onSeek = undefined,
    }: {
        position: number;
        duration: number;
        seekable?: boolean;
        liveLabel?: string;
        idle?: boolean;
        inline?: boolean;
        onSeek?: (sec: number) => void;
    } = $props();

    /** Non-null while a finger is on the rail; the poll loses for that time. */
    let scrub = $state<number | null>(null);
    const shown = $derived(scrub ?? position);

    function commit(sec: number) {
        scrub = null;
        onSeek?.(sec);
    }

    // A new track must never inherit the previous one's drag.
    let lastDuration = 0;
    $effect(() => {
        if (duration === lastDuration) return;
        lastDuration = duration;
        scrub = null;
    });
</script>

{#if duration > 0}
    {#if inline}
        <div class="rail-inline">
            <span class="rt mono">{fmtSecs(shown)}</span>
            {#if seekable && onSeek}
                <Slider
                    variant="scrub"
                    max={duration}
                    value={shown}
                    label="Seek"
                    valueText="{fmtSecs(shown)} of {fmtSecs(duration)}"
                    onInput={(v) => (scrub = v)}
                    onChange={commit}
                />
            {:else}
                <span class="ro-rail" aria-hidden="true">
                    <i style:width="{Math.min(100, (shown / duration) * 100)}%"></i>
                </span>
            {/if}
            <span class="rt mono">{fmtSecs(duration)}</span>
        </div>
    {:else}
        <div class="rail-box">
            {#if seekable && onSeek}
                <Slider
                    variant="scrub"
                    max={duration}
                    value={shown}
                    label="Seek"
                    valueText="{fmtSecs(shown)} of {fmtSecs(duration)}"
                    onInput={(v) => (scrub = v)}
                    onChange={commit}
                />
            {:else}
                <span class="ro-rail" aria-hidden="true">
                    <i style:width="{Math.min(100, (shown / duration) * 100)}%"></i>
                </span>
            {/if}
            <div class="rail-times mono">
                <span>{fmtSecs(shown)}</span><span>{fmtSecs(duration)}</span>
            </div>
        </div>
    {/if}
{:else if !idle && !inline}
    <div class="rail-live mono">{liveLabel}</div>
{/if}

<style>
    .rail-box {
        display: flex;
        flex-direction: column;
        gap: 6px;
    }
    .rail-times {
        display: flex;
        justify-content: space-between;
        font-size: 11px;
        color: var(--text-dim);
    }
    /* Read-only: a bar, not a track with a knob on it — nothing here moves. */
    .ro-rail {
        display: block;
        height: 6px;
        border-radius: 3px;
        background: var(--card-3);
        overflow: hidden;
    }
    .ro-rail i {
        display: block;
        height: 100%;
        background: var(--on);
        transition: width 1s linear;
    }
    .rail-live {
        text-align: center;
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-dim);
    }
    /* Inline: one row, times flanking the rail — the player bar's shape. */
    .rail-inline {
        display: flex;
        align-items: center;
        gap: 10px;
        width: 100%;
    }
    .rail-inline .rt {
        font-size: 11px;
        color: var(--text-dim);
        flex-shrink: 0;
        min-width: 32px;
    }
    .rail-inline .rt:last-child {
        text-align: right;
    }
    .rail-inline .ro-rail {
        flex: 1;
    }
    /* The scrubber's `width: 100%` is a column rule — in the row it would
       push the end time clean out of the box. */
    .rail-inline :global(input[type="range"].scrub) {
        width: auto;
        flex: 1;
    }
    @media (prefers-reduced-motion: reduce) {
        .ro-rail i {
            transition-duration: 0.001ms;
        }
    }
</style>
