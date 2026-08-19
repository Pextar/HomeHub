<script lang="ts">
    /**
     * One volume row in the kid module (DESIGN.md §17): a round mute button,
     * a fat slider, and the number it is at.
     *
     * The room's own fader and each speaker's fader under it are the same
     * row — the member's is a little smaller and carries a name — so they
     * are one component rather than two blocks of markup sharing a
     * stylesheet by accident.
     *
     * The kid's faders top out below the room's real maximum (§17), which is
     * the caller's business: it passes `max`, and passes a `value` clamped to
     * it. A speaker a grown-up already turned up past that reads its real
     * number in the readout and can only be turned down.
     */
    import KidSlider from "./KidSlider.svelte";

    let {
        value,
        max,
        /** The number to show. Not always `value`: a fader clamped to the
         *  kid's ceiling still tells the truth about how loud it is. */
        readout,
        label,
        /** A member row names its speaker; the room's row doesn't. */
        name = undefined,
        muted,
        muteBusy = false,
        muteLabel,
        onMute,
        onInput,
        onChange,
    }: {
        value: number;
        max: number;
        readout: number;
        label: string;
        name?: string;
        muted: boolean;
        muteBusy?: boolean;
        muteLabel: string;
        onMute: () => void;
        onInput: (v: number) => void;
        onChange: (v: number) => void;
    } = $props();
</script>

<div class="km-volume">
    <button
        class="km-vol-btn"
        class:small={!!name}
        class:mute={muted}
        aria-label={muteLabel}
        disabled={muteBusy}
        onclick={onMute}
    >
        {muted ? "🔇" : "🔊"}
    </button>
    {#if name}
        <span class="km-member-name">{name}</span>
    {/if}
    <KidSlider {value} {max} {label} valueText="{readout}%" {onInput} {onChange} />
    <span class="km-vol-val mono">{readout}</span>
</div>

<style>
    .km-volume {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .km-vol-btn {
        width: 52px;
        height: 52px;
        border-radius: 50%;
        border: 2px solid var(--border);
        background: var(--surface);
        font-size: 1.3rem;
        display: grid;
        place-items: center;
        cursor: pointer;
        flex-shrink: 0;
        transition: transform 0.12s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .km-vol-btn.small { width: 46px; height: 46px; font-size: 1.1rem; }
    .km-vol-btn:active { transform: scale(0.9); }
    .km-vol-btn.mute { border-color: var(--kid-pink); }
    .km-member-name {
        width: 88px;
        flex-shrink: 0;
        font-size: 0.9rem;
        font-weight: 800;
        color: var(--text-muted);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
    }
    .km-vol-val {
        font-family: var(--font-mono);
        font-feature-settings: "tnum" 1;
        font-size: 0.95rem;
        font-weight: 700;
        color: var(--text-muted);
        min-width: 3ch;
        text-align: right;
        flex-shrink: 0;
    }
</style>
