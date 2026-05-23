<script lang="ts">
    interface Props {
        checked: boolean;
        onChange?: (checked: boolean) => void;
        ariaLabel?: string;
        disabled?: boolean;
    }
    let { checked = $bindable(), onChange, ariaLabel, disabled = false }: Props = $props();

    function handle(e: Event) {
        const v = (e.currentTarget as HTMLInputElement).checked;
        checked = v;
        onChange?.(v);
    }
</script>

<label class="switch">
    <input
        type="checkbox"
        checked={checked}
        onchange={handle}
        aria-label={ariaLabel}
        {disabled}
    />
    <span class="track" aria-hidden="true"></span>
</label>

<style>
    .switch {
        position: relative;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        /* Visual toggle is 44×26 — slightly larger than before but still compact */
        width: 44px;
        height: 26px;
        flex-shrink: 0;
        /* Expand touch target to iOS HIG 44×44 minimum without breaking layout */
        cursor: pointer;
        touch-action: manipulation;
    }
    .switch::after {
        content: '';
        position: absolute;
        inset: -9px -8px; /* extra tappable padding */
    }
    .switch input { opacity: 0; width: 0; height: 0; position: absolute; }
    .track {
        position: absolute; inset: 0;
        background: var(--card-3);
        border-radius: 999px;
        transition: background var(--t-fast), box-shadow var(--t-fast);
        pointer-events: none;
    }
    .track::after {
        content: "";
        position: absolute;
        width: 20px; height: 20px;
        background: #b5b1a8;
        border-radius: 50%;
        top: 3px; left: 3px;
        transition: transform 0.22s var(--spring), background var(--t-fast);
        box-shadow: 0 1px 3px rgba(0,0,0,0.25);
    }
    .switch input:checked + .track { background: var(--on); }
    .switch input:checked + .track::after { transform: translateX(18px); background: #fff; }
    .switch input:focus-visible + .track { box-shadow: var(--focus-ring); }
    .switch input:disabled + .track { opacity: 0.5; cursor: not-allowed; }
    .switch:active .track { box-shadow: 0 0 0 4px var(--primary-glow); }
</style>
