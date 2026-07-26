<script lang="ts">
    /**
     * The styled range input the player uses for both jobs it has: volume
     * rows, and the scrubber.
     *
     * One component because they are the same control — DESIGN.md §15 says the
     * scrubber "inherits the volume sliders' coarse-pointer sizing", and that
     * is a sentence about a shared thing, not a coincidence to re-type. The
     * rest of the app styles its ranges per component; Music has three of them
     * in one sheet, which is enough to be worth naming.
     *
     * `variant="scrub"` is the one difference: the scrubber sits in a column
     * and must fill it, where a volume row's slider flexes beside a label.
     */
    let {
        value,
        max = 100,
        min = 0,
        label,
        valueText = undefined,
        disabled = false,
        variant = "volume",
        /** Live, on every movement — update the local value, send nothing. */
        onInput,
        /** On release — this is what goes to the speaker. */
        onChange,
    }: {
        value: number;
        max?: number;
        min?: number;
        label: string;
        valueText?: string;
        disabled?: boolean;
        variant?: "volume" | "scrub";
        onInput: (v: number) => void;
        onChange: (v: number) => void;
    } = $props();
</script>

<input
    type="range"
    class:scrub={variant === "scrub"}
    {min}
    {max}
    step="1"
    aria-label={label}
    aria-valuetext={valueText}
    {disabled}
    {value}
    oninput={(e) => onInput(e.currentTarget.valueAsNumber)}
    onchange={(e) => onChange(e.currentTarget.valueAsNumber)}
/>

<style>
    input[type="range"] {
        flex: 1; min-width: 60px; appearance: none;
        height: 6px; border-radius: 3px; outline: none;
        background: var(--card-3); accent-color: var(--on);
    }
    input[type="range"]::-webkit-slider-thumb {
        appearance: none; width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px; border-radius: 50%;
        background: #fff; border: 2px solid rgba(0, 0, 0, 0.35);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible { box-shadow: 0 0 0 2px var(--on-soft); }
    /* The scrubber owns its column; `flex: 1` would collapse it there. */
    input[type="range"].scrub { flex: none; width: 100%; }

    @media (pointer: coarse) {
        input[type="range"] { height: 10px; border-radius: 5px; }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
    }
</style>
