<script lang="ts">
    import { DAY_NAMES, DAY_SHORT } from "../lib/utils";

    interface Props {
        days: number[];
    }
    let { days = $bindable() }: Props = $props();

    function toggle(i: number) {
        if (days.includes(i)) days = days.filter(d => d !== i);
        else days = [...days, i].sort((a, b) => a - b);
    }
    const set = (vals: number[]) => () => { days = vals; };
</script>

<div class="picker" role="group" aria-label="Days of week">
    {#each DAY_SHORT as label, i}
        <button
            type="button"
            class="day-chip"
            data-selected={days.includes(i)}
            aria-pressed={days.includes(i)}
            aria-label={DAY_NAMES[i]}
            title={DAY_NAMES[i]}
            onclick={() => toggle(i)}
        >{label}</button>
    {/each}
</div>

<div class="presets">
    <button type="button" class="preset" onclick={set([0, 1, 2, 3, 4, 5, 6])}>Every day</button>
    <button type="button" class="preset" onclick={set([1, 2, 3, 4, 5])}>Weekdays</button>
    <button type="button" class="preset" onclick={set([0, 6])}>Weekends</button>
    <button type="button" class="preset" onclick={set([])}>Clear</button>
</div>

<style>
    .picker {
        display: flex;
        gap: 6px;
        flex-wrap: wrap;
    }
    .day-chip {
        display: inline-flex;
        align-items: center; justify-content: center;
        width: 36px; height: 36px;
        border-radius: 50%;
        background: var(--surface);
        border: 1px solid var(--border);
        color: var(--text-muted);
        font-size: 12px;
        font-weight: 600;
        cursor: pointer;
        user-select: none;
        transition: background var(--t-fast), color var(--t-fast),
                    border-color var(--t-fast);
    }
    .day-chip[data-selected="true"] {
        background: var(--primary);
        color: var(--primary-fg);
        border-color: transparent;
    }
    .day-chip:active { transform: scale(0.92); transition-duration: 60ms; }
    .presets {
        display: flex;
        gap: var(--space-2);
        margin-top: var(--space-2);
        flex-wrap: wrap;
    }
    .preset {
        background: transparent;
        border: 1px solid var(--border);
        border-radius: 999px;
        padding: 4px 10px;
        color: var(--text-muted);
        font-size: 12px;
        cursor: pointer;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .preset:hover { background: var(--surface-hover); color: var(--text); }
    .preset:active { transform: scale(0.96); transition-duration: 60ms; }

    /* Touch screens: meet the 44px minimum target. */
    @media (pointer: coarse) {
        .picker { gap: var(--space-2); }
        .day-chip { width: 44px; height: 44px; font-size: 14px; }
        .preset { padding: 10px 16px; font-size: 14px; }
    }
</style>
