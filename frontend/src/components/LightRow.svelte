<script lang="ts">
    /**
     * How bright, and what colour — the one control the whole app uses to say
     * it.
     *
     * The rule editor drew this five times: for a single smart socket, for a
     * group or room set uniformly (twice, once per branch of the perLamp
     * choice), for the "set all" bar over a per-lamp matrix, and again on
     * every row of that matrix. Five copies of a slider and eleven preset
     * chips, differing only in where the value came from and what to call the
     * slider for a screen reader.
     *
     * The presets are the reason it matters that they stay identical: a rule
     * that says "Relax" and a lamp a person set to "Relax" by hand have to
     * mean the same light, so the palette is one list (`MATTER_PRESETS`) drawn
     * by one component.
     *
     * Brightness and colour arrive separately because they are set
     * separately — the slider moves the level alone, a preset chip sets both
     * at once, and the "—" chip clears the colour back to whatever the lamp
     * does on its own.
     */
    import Icon from "./Icon.svelte";
    import { MATTER_PRESETS } from "../lib/rules";

    let {
        level,
        color,
        /** Names the slider and each chip for a screen reader: "for Hallway",
         *  "for all lamps", or empty where the row is the only one on screen. */
        forWhat = "",
        onLevel,
        onPreset,
    }: {
        level: number;
        color: string;
        forWhat?: string;
        onLevel: (level: number) => void;
        onPreset: (level: number, color: string) => void;
    } = $props();

    const suffix = $derived(forWhat ? ` for ${forWhat}` : "");
</script>

<div class="bright">
    <span class="bright-ico"><Icon name="sun" size={14} /></span>
    <input
        type="range"
        min="1"
        max="100"
        step="1"
        value={level}
        oninput={(e) => onLevel(parseInt((e.target as HTMLInputElement).value, 10))}
        aria-label="Brightness{suffix}"
    />
    <span class="bright-val mono">{level}%</span>
</div>

<div class="preset-chips" role="group" aria-label="Lighting preset{suffix}">
    <!-- No preset: the lamp keeps whatever colour it had. Distinct from
         picking white, which is a colour. -->
    <button
        type="button"
        class="preset-chip auto"
        class:active={!color}
        title="No preset"
        aria-label="No lighting preset{suffix}"
        aria-pressed={!color}
        onclick={() => onPreset(100, "")}
    >
        —
    </button>
    {#each MATTER_PRESETS as p (p.label)}
        <button
            type="button"
            class="preset-chip"
            class:active={color === p.color}
            style="--pc: {p.cssColor}"
            title="{p.label} · {p.level}%"
            aria-label="{p.label} preset{suffix}"
            aria-pressed={color === p.color}
            onclick={() => onPreset(p.level, p.color)}
        >
            <span class="preset-dot" style="background:{p.cssColor}"></span>
            {p.label}
        </button>
    {/each}
</div>

<style>
    .bright { display: flex; align-items: center; gap: 8px; }
    .bright-ico { color: var(--on); display: inline-flex; flex-shrink: 0; }
    .bright input[type="range"] { flex: 1; }
    .bright-val { font-size: 12px; color: var(--text-muted); min-width: 38px; text-align: right; }

    .preset-chips {
        display: flex;
        flex-wrap: wrap;
        gap: 5px;
    }
    .preset-chip {
        display: inline-flex;
        align-items: center;
        gap: 5px;
        padding: 3px 9px;
        font-size: 12px;
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-mute);
        cursor: pointer;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap;
    }
    .preset-chip:hover { background: var(--card-3); color: var(--text); }
    .preset-chip.active {
        background: var(--card-3);
        color: var(--text);
        box-shadow: 0 0 0 1px var(--border-strong) inset;
    }
    .preset-chip.auto { color: var(--text-dim); font-size: 13px; }
    .preset-dot {
        width: 10px;
        height: 10px;
        border-radius: 50%;
        flex-shrink: 0;
        border: 1px solid rgba(255,255,255,0.15);
    }
    @media (max-width: 600px) {
        .bright input[type="range"] { height: 28px; }
    }
    @media (pointer: coarse) {
        .bright input[type="range"] { height: 28px; }
        .preset-chip { padding: 6px 12px; font-size: 13px; min-height: 36px; }
        .preset-dot { width: 12px; height: 12px; }
    }
</style>
