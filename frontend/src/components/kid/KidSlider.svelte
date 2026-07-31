<script lang="ts">
    /**
     * The kid module's big friendly slider (DESIGN.md §17): a fat track the
     * thumb rides on, filled up to the value so even the progress rail reads
     * at arm's length. One primitive for every fader in the kid surfaces —
     * volume, per-speaker volume, the seek rail — the way KidLampPanel's
     * brightness row is the lamp module's one.
     *
     * The input itself is a real range control (keyboard, screen reader and
     * drag all come free); the track and fill are paint underneath it, since
     * a native range can't draw its own fill. `onInput` fires on every
     * movement (live display, throttled sends), `onChange` on release (the
     * authoritative value) — the same contract as the Music module's Slider.
     */
    interface Props {
        value: number;
        min?: number;
        max?: number;
        disabled?: boolean;
        /** aria-label — the control is icon-adjacent, so it always needs one. */
        label: string;
        valueText?: string;
        onInput: (v: number) => void;
        onChange: (v: number) => void;
    }
    let {
        value,
        min = 0,
        max = 100,
        disabled = false,
        label,
        valueText,
        onInput,
        onChange,
    }: Props = $props();

    const pct = $derived(max > min ? Math.min(100, Math.max(0, ((value - min) / (max - min)) * 100)) : 0);
</script>

<div class="ks" class:disabled>
    <span class="ks-track" aria-hidden="true"></span>
    <span class="ks-fill" style:width="{pct}%" aria-hidden="true"></span>
    <input
        type="range"
        {min}
        {max}
        {value}
        {disabled}
        aria-label={label}
        aria-valuetext={valueText}
        oninput={(e) => onInput(+e.currentTarget.value)}
        onchange={(e) => onChange(+e.currentTarget.value)}
    />
</div>

<style>
    .ks {
        position: relative;
        height: 48px;
        flex: 1;
        min-width: 0;
    }
    .ks-track,
    .ks-fill {
        position: absolute;
        top: 50%;
        transform: translateY(-50%);
        left: 0;
        height: 16px;
        border-radius: 999px;
        pointer-events: none;
    }
    .ks-track {
        right: 0;
        background: var(--surface-hover);
    }
    .ks-fill {
        background: var(--kid-accent-grad);
        box-shadow: 0 0 12px var(--kid-glow);
        transition: width 0.08s linear;
    }
    .ks input[type="range"] {
        position: absolute;
        inset: 0;
        width: 100%;
        margin: 0;
        -webkit-appearance: none;
        appearance: none;
        background: transparent;
        cursor: pointer;
    }
    .ks input[type="range"]::-webkit-slider-thumb {
        -webkit-appearance: none;
        appearance: none;
        width: 36px;
        height: 36px;
        border-radius: 50%;
        background: var(--kid-accent-grad);
        border: 3px solid rgba(255, 255, 255, 0.85);
        box-shadow: 0 3px 12px var(--kid-glow);
    }
    .ks input[type="range"]::-moz-range-thumb {
        width: 30px;
        height: 30px;
        border-radius: 50%;
        background: #ffd23f;
        border: 3px solid rgba(255, 255, 255, 0.85);
        box-shadow: 0 3px 12px var(--kid-glow);
    }
    .ks.disabled { opacity: 0.45; }
    .ks.disabled input[type="range"] { cursor: default; }
    @media (prefers-reduced-motion: reduce) {
        .ks-fill { transition: none; }
    }
</style>
