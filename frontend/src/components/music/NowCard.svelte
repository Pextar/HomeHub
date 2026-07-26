<script lang="ts">
    /**
     * One "Playing now" card: art, zone name, what is on, a transport, and a
     * progress hairline along the bottom edge.
     *
     * The same card for both bridges — a playing KEF speaker is a peer here,
     * not a lesser citizen (DESIGN.md §15) — which is why it takes a title, a
     * line and a transport rather than a group or a speaker. What differs
     * between them is only which of those it is handed: a Sonos zone gets
     * skips, a KEF speaker doesn't, because the sheet is where its skips are.
     *
     * The whole card sits on the sanctioned `.tile.on` surface while playing.
     * No separate music gradient exists or should be invented.
     */
    import type { Snippet } from "svelte";
    import { fly, fade } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur } from "../../lib/motion";
    import Waveform from "./Waveform.svelte";
    import ProgressLine from "./ProgressLine.svelte";

    let {
        /** The zone or speaker name. */
        name,
        /** Track and artist, already joined — "Live audio" when there is none. */
        line,
        artUri = undefined,
        playing,
        /** 0–1; zero means the source reports no duration, so no line. */
        progress = 0,
        onOpen,
        /** The card's transport. Passed in so the card doesn't have to know
         *  which bridge's play/pause it is wiring. */
        transport,
        /** True on the one card the dock stands down behind. */
        isDock = false,
        /** Reports whether that card is on screen, so the dock can appear the
         *  moment it scrolls away. */
        onDockVisible = undefined,
    }: {
        name: string;
        line: string;
        artUri?: string;
        playing: boolean;
        progress?: number;
        onOpen: () => void;
        transport: Snippet;
        isDock?: boolean;
        onDockVisible?: (visible: boolean) => void;
    } = $props();

    /**
     * The dock is a fallback, never a duplicate (DESIGN.md §15), so it appears
     * only once the card it repeats has left the screen. The observer is
     * attached to every card but live only on the dock group's, and its bottom
     * inset discounts the band the dock and the tab bar occupy — a card
     * sitting behind them counts as gone rather than as visible.
     */
    function dockAnchor(node: HTMLElement, on: boolean) {
        let obs: IntersectionObserver | undefined;
        let active = false;
        function attach(next: boolean) {
            obs?.disconnect();
            obs = undefined;
            if (active && !next) onDockVisible?.(false);
            active = next;
            if (!next) return;
            obs = new IntersectionObserver(
                ([entry]) => onDockVisible?.(entry.isIntersecting),
                { threshold: 0.5, rootMargin: "0px 0px -96px 0px" },
            );
            obs.observe(node);
        }
        attach(on);
        return {
            update: (next: boolean) => attach(next),
            destroy: () => attach(false),
        };
    }
</script>

<div
    class="now-card"
    class:playing
    use:dockAnchor={isDock}
    in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
    out:fade={{ duration: dur(120) }}
>
    <button class="now-open" onclick={onOpen}>
        {#if artUri}
            <img class="now-art" src={artUri} alt="" loading="lazy" />
        {:else}
            <div class="now-art placeholder">[ art ]</div>
        {/if}
        <span class="now-meta">
            <span class="now-name" title={name}>{name}</span>
            <span class="now-line">
                <Waveform />
                <span class="now-track">{line}</span>
            </span>
        </span>
    </button>
    {@render transport()}
    <ProgressLine value={progress} />
</div>

<style>
    .now-card {
        position: relative; overflow: hidden;
        display: flex; align-items: center; gap: var(--space-3);
        padding: 14px;
        background: var(--card); border: 1px solid var(--hairline);
        border-radius: var(--r-lg);
        transition: border-color var(--t-fast);
    }
    .now-card.playing { background: var(--tile-on-gradient); border-color: var(--tile-on-border); }
    @media (hover: hover) { .now-card:hover { border-color: var(--border-strong); } }
    .now-open {
        flex: 1; min-width: 0;
        display: flex; align-items: center; gap: var(--space-3);
        background: none; border: 0; padding: 0;
        color: var(--text); text-align: left; cursor: pointer;
        transition: transform var(--t-fast);
    }
    .now-open:active { transform: scale(0.99); }
    .now-art {
        width: 52px; height: 52px; border-radius: var(--r-md);
        object-fit: cover; background: var(--card-3);
        border: 1px solid var(--hairline); flex-shrink: 0;
    }
    div.now-art { display: grid; place-items: center; font-size: 9px; color: var(--text-dim); }
    .now-meta { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
    .now-name {
        font-size: 14px; font-weight: 600; letter-spacing: -0.01em;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .now-line { display: flex; align-items: center; gap: 6px; min-width: 0; }
    .now-track {
        font-size: 12.5px; color: var(--text-mute);
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    @media (prefers-reduced-motion: reduce) {
        .now-card { transition-duration: 0.001ms; }
    }
</style>
