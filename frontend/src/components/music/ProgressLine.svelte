<script lang="ts">
    /**
     * How far the track has got, as a 2px hairline along the bottom edge of a
     * card that carries a transport — the one thing those cards couldn't say
     * without opening the player.
     *
     * Renders nothing at zero. Sources that report no duration (radio,
     * line-in, TV) come through as zero and get no line rather than a made-up
     * one — the same honesty the scrubber owes them.
     */
    let { value }: { /** 0–1. */ value: number } = $props();
</script>

{#if value > 0}
    <span class="prog" aria-hidden="true"><i style:width="{value * 100}%"></i></span>
{/if}

<style>
    .prog {
        position: absolute; left: 0; right: 0; bottom: 0;
        height: 2px; background: var(--hairline);
        pointer-events: none;
    }
    .prog i {
        display: block; height: 100%;
        background: var(--on);
        /* Matches the 1s position tick, so the fill creeps instead of
           stepping. */
        transition: width 1s linear;
    }
    @media (prefers-reduced-motion: reduce) {
        .prog i { transition-duration: 0.001ms; }
    }
</style>
