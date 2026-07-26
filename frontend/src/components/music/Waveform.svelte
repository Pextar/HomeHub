<script lang="ts">
    /**
     * The §6.8 waveform: four bars on a staggered loop, marking anything that
     * is **actually playing**.
     *
     * It replaces the §6.6 status dot in Music and nowhere else — a dot says
     * "on", a waveform says "audio is moving" — and this animated motif, not
     * colour, is what marks Music as its own module. Its one satellite outside
     * the module is Home's "Playing now" card, which is Music's surface there
     * and so carries the module's motifs rather than inventing quieter ones.
     *
     * One component because it was two verbatim copies: this markup, this CSS
     * and this keyframe lived in both Music.svelte and NowPlaying.svelte, and
     * the whole point of a motif is that it is the same everywhere.
     */
    let {
        /**
         * Bars in the surface's ink rather than amber. For the one place the
         * waveform sits *on* amber — a playing room puck's filled icon tile,
         * where amber-on-amber would be invisible.
         */
        ink = false,
    }: { ink?: boolean } = $props();
</script>

<span class="wave" class:ink aria-hidden="true"><i></i><i></i><i></i><i></i></span>

<style>
    .wave { display: flex; align-items: flex-end; gap: 2.5px; height: 13px; flex-shrink: 0; }
    .wave i {
        display: block; width: 2.5px; border-radius: 1px;
        background: var(--on); height: 4px;
        animation: wv 950ms ease-in-out infinite;
    }
    .wave i:nth-child(1) { animation-delay: 0s; }
    .wave i:nth-child(2) { animation-delay: 0.15s; }
    .wave i:nth-child(3) { animation-delay: 0.3s; }
    .wave i:nth-child(4) { animation-delay: 0.1s; }
    .wave.ink i { background: var(--primary-fg); }
    @keyframes wv { 0%, 100% { height: 3px; } 50% { height: 13px; } }

    @media (prefers-reduced-motion: reduce) {
        .wave i { animation: none; height: 8px; }
    }
</style>
