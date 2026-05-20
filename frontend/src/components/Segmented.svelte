<script lang="ts">
    interface Option {
        value: string;
        label: string;
        disabled?: boolean;
    }
    interface Props {
        name: string;
        value: string;
        options: Option[];
        onChange?: (value: string) => void;
    }
    let { name, value = $bindable(), options, onChange }: Props = $props();

    function pick(v: string) {
        value = v;
        onChange?.(v);
    }
</script>

<div class="segmented" role="radiogroup">
    {#each options as opt (opt.value)}
        <input
            type="radio"
            id="{name}_{opt.value}"
            {name}
            value={opt.value}
            checked={value === opt.value}
            disabled={opt.disabled}
            onchange={() => pick(opt.value)}
        />
        <label for="{name}_{opt.value}" class:disabled={opt.disabled}>
            {opt.label}
        </label>
    {/each}
</div>

<style>
    .segmented {
        display: inline-flex;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: 3px;
        gap: 2px;
    }
    .segmented input { display: none; }
    .segmented label {
        padding: 6px 14px;
        border-radius: 7px;
        cursor: pointer;
        color: var(--text-muted);
        font-weight: 500;
        font-size: 13px;
        touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast);
        user-select: none;
    }
    .segmented input:checked + label {
        background: var(--bg-elevated);
        color: var(--text);
        box-shadow: var(--shadow-sm);
    }
    .segmented label.disabled { opacity: 0.4; cursor: not-allowed; }
    /* Touch: taller labels */
    @media (pointer: coarse) {
        .segmented { padding: 4px; }
        .segmented label { padding: 9px 16px; font-size: 14px; }
    }
</style>
