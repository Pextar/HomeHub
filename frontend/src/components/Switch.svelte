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
        display: inline-block;
        width: 36px;
        height: 20px;
        flex-shrink: 0;
    }
    .switch input { opacity: 0; width: 0; height: 0; }
    .track {
        position: absolute; inset: 0;
        background: var(--border-strong);
        border-radius: 999px;
        transition: background var(--t-fast);
        cursor: pointer;
    }
    .track::after {
        content: "";
        position: absolute;
        width: 16px; height: 16px;
        background: #fff;
        border-radius: 50%;
        top: 2px; left: 2px;
        transition: transform var(--t-fast);
        box-shadow: 0 1px 2px rgba(0,0,0,0.2);
    }
    .switch input:checked + .track { background: var(--success); }
    .switch input:checked + .track::after { transform: translateX(16px); }
    .switch input:focus-visible + .track { box-shadow: var(--focus-ring); }
    .switch input:disabled + .track { opacity: 0.5; cursor: not-allowed; }
</style>
