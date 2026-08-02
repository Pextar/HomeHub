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
        /** Live, on every movement. */
        onInput,
        /** On release — the authoritative value. */
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
        background: var(--knob); border: 2px solid var(--knob-edge);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]::-moz-range-thumb {
        width: 18px; height: 18px; border-radius: 50%;
        background: var(--knob); border: 2px solid var(--knob-edge);
        cursor: pointer; box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
    }
    input[type="range"]:focus-visible { box-shadow: 0 0 0 2px var(--on-soft); }
    /* The scrubber owns its column; `flex: 1` would collapse it there. */
    input[type="range"].scrub { flex: none; width: 100%; }

    /* A range input's hit area is its *box*, not its thumb: a 10px-tall
       track drawn with a 26px knob still only accepts a 10px band of
       touches, which is well under the §2 floor and hardest to hit exactly
       where it matters most — a wall panel, at arm's length. The track
       keeps its drawn weight; the box grows around it with transparent
       padding, and the background is clipped to the content box so the
       rail doesn't fatten with it. */
    @media (pointer: coarse) {
        input[type="range"] {
            height: 44px;
            padding-block: 17px;
            border-radius: 5px;
            box-sizing: border-box;
            background-clip: content-box;
        }
        input[type="range"]::-webkit-slider-runnable-track {
            height: 10px;
            border-radius: 5px;
        }
        input[type="range"]::-moz-range-track {
            height: 10px;
            border-radius: 5px;
        }
        input[type="range"]::-webkit-slider-thumb { width: 26px; height: 26px; margin-top: -8px; }
        input[type="range"]::-moz-range-thumb { width: 26px; height: 26px; }
    }
</style>
