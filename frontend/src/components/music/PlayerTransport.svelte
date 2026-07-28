<script lang="ts">
    /**
     * The player's transport: prev, the amber play button, next — flanked by
     * shuffle and repeat where the source has them.
     *
     * Shuffle and repeat are `.t-mode` circles that take `--on-soft` + `--on`
     * when engaged, deliberately quieter than the play button. They are
     * group-level, so they only render when the coordinator reported a
     * `group_state`; a follower's view never carries one, and a KEF speaker
     * has no play modes at all.
     *
     * The skips are optional for the same reason the modes are: a zone HomeHub
     * is streaming to has no track to step to — the Spotify session that does
     * is HomeHub's own — and a control the source would refuse is worse than
     * one that isn't there (§15).
     */
    import Icon from "../Icon.svelte";
    import { haptic } from "../../lib/utils";
    import type { SonosRepeat } from "../../lib/types";

    let {
        playing,
        onToggle,
        toggleBusy = false,
        onPrev = undefined,
        prevBusy = false,
        onNext = undefined,
        nextBusy = false,
        /** True only where a bare arrow key seeks instead of skipping, so the
         *  track shortcut needs shift to tell the two apart. KEF and a zone
         *  have no seek at all, and a Sonos live stream has nothing to seek
         *  either — in all three, a bare arrow already skips. */
        seekable = false,
        /** Absent for a source with no play modes. */
        modes = undefined,
    }: {
        playing: boolean;
        onToggle: () => void;
        toggleBusy?: boolean;
        onPrev?: () => void;
        prevBusy?: boolean;
        onNext?: () => void;
        nextBusy?: boolean;
        seekable?: boolean;
        modes?: {
            shuffle: boolean;
            repeat: SonosRepeat;
            /** Reports repeat's *next* state, since that is what a tap does. */
            repeatLabel: string;
            busy: boolean;
            onShuffle: () => void;
            onRepeat: () => void;
        };
    } = $props();
</script>

<div class="p-transport">
    {#if modes}
        <button
            class="icon-btn t-mode"
            class:on={modes.shuffle}
            aria-label={modes.shuffle ? "Shuffle on" : "Shuffle off"}
            aria-pressed={modes.shuffle}
            title="Shuffle (s)"
            disabled={modes.busy}
            onclick={modes.onShuffle}
        >
            <Icon name="shuffle" size={18} />
        </button>
    {/if}
    {#if onPrev}
        <button
            class="icon-btn t-btn"
            aria-label="Previous track"
            title={seekable ? "Previous (shift ←)" : "Previous (←)"}
            disabled={prevBusy}
            onclick={onPrev}
        >
            <Icon name="skipPrev" size={22} />
        </button>
    {/if}
    <button
        class="p-play"
        class:playing
        aria-label={playing ? "Pause" : "Play"}
        title="Play / pause (space)"
        disabled={toggleBusy}
        onclick={() => { haptic(); onToggle(); }}
    >
        <Icon name={playing ? "pause" : "play"} size={26} />
    </button>
    {#if onNext}
        <button
            class="icon-btn t-btn"
            aria-label="Next track"
            title={seekable ? "Next (shift →)" : "Next (→)"}
            disabled={nextBusy}
            onclick={onNext}
        >
            <Icon name="skipNext" size={22} />
        </button>
    {/if}
    {#if modes}
        <button
            class="icon-btn t-mode"
            class:on={modes.repeat !== "off"}
            aria-label={modes.repeatLabel}
            title="Repeat (r)"
            disabled={modes.busy}
            onclick={modes.onRepeat}
        >
            <Icon name={modes.repeat === "one" ? "repeatOne" : "repeat"} size={18} />
        </button>
    {/if}
</div>

<style>
    .p-transport { display: flex; align-items: center; justify-content: center; gap: var(--space-4); }
    .t-btn { width: 48px; height: 48px; }
    .t-mode { width: 42px; height: 42px; border-radius: 50%; color: var(--text-mute); }
    .t-mode.on { background: var(--on-soft); color: var(--on); }
    .t-mode:disabled { opacity: 0.35; }
    .p-play {
        width: 66px; height: 66px; border-radius: 50%;
        display: grid; place-items: center; flex-shrink: 0;
        background: var(--on); color: var(--primary-fg); border: 0;
        cursor: pointer; box-shadow: 0 0 24px -2px var(--on-glow);
    }
    .p-play:active { transform: scale(0.96); }
    .p-play:disabled { opacity: 0.5; }

    @media (pointer: coarse) {
        .t-btn { width: 52px; height: 52px; }
        .t-mode { width: 48px; height: 48px; }
        /* Five transport controls have to fit a 360px screen. */
        .p-transport { gap: var(--space-3); }
    }
    @media (prefers-reduced-motion: reduce) {
        .p-play { transition-duration: 0.001ms; }
    }
</style>
