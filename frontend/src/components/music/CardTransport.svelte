<script lang="ts">
    /**
     * The transport a card carries: play/pause, flanked by skips where there
     * is width for them.
     *
     * Worn by the "Playing now" cards and by the dock, and the fact that it is
     * one component is the rule from DESIGN.md §15 made structural — the dock
     * is a *fallback*, never a duplicate, so it must never be the richer
     * control. Both drop their skips below 430px for the same reason.
     *
     * Omit `onPrev`/`onNext` entirely for a source that has no skips to offer:
     * a KEF card carries play/pause only, like a Sonos card on a phone.
     */
    import Icon from "../Icon.svelte";
    import { haptic } from "../../lib/utils";

    let {
        playing,
        onToggle,
        toggleBusy = false,
        onPrev = undefined,
        prevBusy = false,
        onNext = undefined,
        nextBusy = false,
    }: {
        playing: boolean;
        onToggle: () => void;
        toggleBusy?: boolean;
        onPrev?: () => void;
        prevBusy?: boolean;
        onNext?: () => void;
        nextBusy?: boolean;
    } = $props();
</script>

<div class="card-transport">
    {#if onPrev}
        <button
            class="mini-btn skip"
            aria-label="Previous track"
            disabled={prevBusy}
            onclick={onPrev}
        >
            <Icon name="skipPrev" size={16} />
        </button>
    {/if}
    <button
        class="mini-btn on"
        aria-label={playing ? "Pause" : "Play"}
        disabled={toggleBusy}
        onclick={() => { haptic(); onToggle(); }}
    >
        <Icon name={playing ? "pause" : "play"} size={16} />
    </button>
    {#if onNext}
        <button class="mini-btn skip" aria-label="Next track" disabled={nextBusy} onclick={onNext}>
            <Icon name="skipNext" size={16} />
        </button>
    {/if}
</div>

<style>
    .card-transport { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
    .mini-btn {
        width: 38px; height: 38px; border-radius: 50%;
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--card-3); border: 1px solid var(--hairline);
        color: var(--text); cursor: pointer;
        transition: transform var(--t-fast), background var(--t-fast);
    }
    .mini-btn.on { background: var(--on); color: var(--primary-fg); border-color: transparent; }
    .mini-btn:active:not(:disabled) { transform: scale(0.94); }
    .mini-btn:focus-visible { box-shadow: var(--focus-ring); }
    .mini-btn:disabled { opacity: 0.5; }
    /* A phone keeps play/pause and gives the track title the room instead —
       the same trade Home's card makes. */
    @media (max-width: 430px) {
        .mini-btn.skip { display: none; }
    }
    @media (pointer: coarse) {
        .mini-btn { width: 44px; height: 44px; }
    }
    @media (prefers-reduced-motion: reduce) {
        .mini-btn { transition-duration: 0.001ms; }
    }
</style>
