<script lang="ts">
    /**
     * A line of text that rolls when it is too long for its box, and holds
     * still when it isn't.
     *
     * It exists for the panel's ambient face (DESIGN.md §16). Everywhere
     * else in the app a title is read at arm's length by someone who can
     * tap it, so an ellipsis is the right answer and truncation costs
     * nothing. The wall is the one surface with no one standing at it: it
     * shows one record for the length of a song, and a song named longer
     * than the column is wide was ending in "…" for three minutes at a
     * time with nothing to reveal the rest.
     *
     * Three rules it keeps:
     *
     * - **It only moves when it has to.** The overrun is measured, and a
     *   title that fits animates nothing at all — no layer, no repaint, on
     *   a surface that stays lit for years.
     * - **It rests at the start.** The cycle holds at both ends and comes
     *   back rather than looping the text past itself, so the glance you
     *   actually take — the one on the way through the room — lands on the
     *   beginning of the name far more often than on the middle of it.
     * - **It is transform-only.** The reference hardware drops frames on
     *   anything that repaints per frame (§16), so the travel is one
     *   composited transform and the timing is a CSS variable, not a
     *   per-frame style write.
     *
     * Under reduced motion it is a truncated line rather than a 0.001ms
     * animation: the whole point of the roll is to show the rest of the
     * name *over time*, and with the motion gone the honest fallback is
     * the ellipsis it replaced.
     */
    let {
        text,
        /** Reading pace in px per second. Slow — a wall is read across a
         *  room, and this is scenery rather than something being waited on. */
        speed = 34,
        /** Shortest a whole cycle may be. Without a floor, a title that
         *  overruns by two words darts out and back like a fault. */
        minCycle = 9,
        /** Longest, so a very long name doesn't crawl for a whole song. */
        maxCycle = 44,
    }: { text: string; speed?: number; minCycle?: number; maxCycle?: number } = $props();

    /** The share of the cycle spent moving (12→46% out, 58→92% back below).
     *  The rest is the two rests, which is what makes the pace readable. */
    const MOVE_SHARE = 0.34;

    let box = $state<HTMLElement | null>(null);
    let line = $state<HTMLElement | null>(null);
    /** How far the line overruns its box, in px. 0 means it fits. */
    let travel = $state(0);

    function measure() {
        if (!box || !line) return;
        // scrollWidth is the full text even while the line is clipped, and
        // stays the full text once it is rolling and no longer clipped —
        // so the reading can't chase its own tail.
        travel = Math.max(0, Math.ceil(line.scrollWidth - box.clientWidth));
    }

    $effect(() => {
        void text; // a new song is a new measurement
        if (!box) return;
        measure();
        // Geist arrives after first paint, and a fallback face measures
        // narrower than the one that ends up on screen.
        void document.fonts?.ready.then(measure);
        const ro = new ResizeObserver(measure);
        ro.observe(box);
        return () => ro.disconnect();
    });

    const cycle = $derived(
        travel > 0 ? Math.min(maxCycle, Math.max(minCycle, travel / (speed * MOVE_SHARE))) : 0,
    );
</script>

<span class="mq" bind:this={box}>
    <span
        class="mq-line"
        class:rolling={travel > 0}
        bind:this={line}
        style:--mq-travel="{-travel}px"
        style:--mq-cycle="{cycle}s">{text}</span
    >
</span>

<style>
    .mq {
        display: block;
        overflow: hidden;
    }
    /* At rest — and whenever the text fits — this is an ordinary truncated
       line. inline-block so it can outgrow the box once it rolls. */
    .mq-line {
        display: inline-block;
        max-width: 100%;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        vertical-align: bottom;
    }
    .mq-line.rolling {
        max-width: none;
        animation: mq-roll var(--mq-cycle) linear infinite;
        will-change: transform;
    }
    /* Rest, out, rest, back. Linear on purpose: an eased marquee reads as
       something being dragged rather than something being read. */
    @keyframes mq-roll {
        0%,
        12% {
            transform: translateX(0);
        }
        46%,
        58% {
            transform: translateX(var(--mq-travel));
        }
        92%,
        100% {
            transform: translateX(0);
        }
    }
    @media (prefers-reduced-motion: reduce) {
        .mq-line.rolling {
            max-width: 100%;
            animation: none;
            will-change: auto;
        }
    }
</style>
