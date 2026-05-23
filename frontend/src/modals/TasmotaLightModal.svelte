<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Switch from "../components/Switch.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Socket, TasmotaState, TasmotaStateUpdate } from "../lib/types";
    import { onMount } from "svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    let deviceState = $state<TasmotaState | null>(null);
    let loading = $state(true);
    let error = $state<string | null>(null);

    // Local optimistic values — populated once we get device state.
    let on = $state(false);
    let dimmer = $state(100);
    let color = $state("#ffffff");
    let ct = $state(366);  // mireds, mid-range default

    const supportsDimmer = $derived(deviceState?.dimmer !== undefined && deviceState?.dimmer !== null);
    const supportsColor  = $derived(deviceState?.color  !== undefined && deviceState?.color  !== null);
    const supportsCT     = $derived(deviceState?.ct     !== undefined && deviceState?.ct     !== null);

    onMount(async () => {
        try {
            const s = await api.tasmotaGetState(socket.id);
            deviceState = s;
            on = s.on;
            if (s.dimmer != null) dimmer = s.dimmer;
            if (s.color)          color  = "#" + s.color.toLowerCase();
            if (s.ct != null)     ct     = s.ct;
        } catch (e) {
            error = (e as Error).message;
        } finally {
            loading = false;
        }
    });

    // --- Debounced sends ---
    let debounceTimer: ReturnType<typeof setTimeout> | undefined;
    let pending: TasmotaStateUpdate = {};
    function send(partial: TasmotaStateUpdate) {
        pending = { ...pending, ...partial };
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(async () => {
            const toSend = pending;
            pending = {};
            try {
                await api.tasmotaSetState(socket.id, toSend);
            } catch (e) {
                toasts.error("Update failed", (e as Error).message);
            }
        }, 120);
    }

    async function toggleOn() {
        const target = !on;
        on = target;
        try {
            if (target) await api.socketOn(socket.id);
            else        await api.socketOff(socket.id);
            await data.refresh();
        } catch (e) {
            on = !target;
            toasts.error("Toggle failed", (e as Error).message);
        }
    }

    function onDimmerInput()  { if (on) send({ dimmer }); }
    function onCTInput()      { if (on) send({ ct }); }
    function onColorInput()   {
        // Strip the leading # from the color picker's hex value.
        if (on) send({ color: color.replace("#", "").toUpperCase() });
    }

    // Gradient fills for the sliders.
    function ctGradient() {
        return "linear-gradient(to right, #ffd6a8 0%, #fff 50%, #b4d8ff 100%)";
    }
</script>

<Modal title={socket.name} subtitle={socket.room || "Unassigned"}>
    {#snippet body()}
        {#if loading}
            <div class="note">Loading device state…</div>
        {:else if error}
            <div class="note error">
                <strong>Could not reach device</strong>
                <span>{error}</span>
            </div>
        {:else if deviceState}
            <div class="row">
                <div class="swatch" class:dim={!on}
                    style:background={supportsColor ? color : "var(--surface)"}>
                </div>
                <div class="meta">
                    <div class="device-ip">{socket.code}</div>
                    {#if !supportsDimmer && !supportsColor && !supportsCT}
                        <div class="hint">This device supports on/off only.</div>
                    {/if}
                </div>
                <div class="toggle">
                    <span class="toggle-label">{on ? "On" : "Off"}</span>
                    <Switch checked={on} onChange={toggleOn} ariaLabel="Power" />
                </div>
            </div>

            {#if supportsDimmer}
                <div class="field">
                    <div class="label-row">
                        <label for="tas-dim">Brightness</label>
                        <span class="val">{dimmer}%</span>
                    </div>
                    <input id="tas-dim" type="range" min="1" max="100" step="1"
                        bind:value={dimmer} oninput={onDimmerInput} disabled={!on} />
                </div>
            {/if}

            {#if supportsCT}
                <div class="field">
                    <div class="label-row">
                        <label for="tas-ct">Warmth</label>
                        <span class="val">{Math.round(1_000_000 / ct)}K</span>
                    </div>
                    <input id="tas-ct" type="range" min="153" max="500" step="1"
                        bind:value={ct} oninput={onCTInput} disabled={!on}
                        style:background={ctGradient()} class="grad" />
                </div>
            {/if}

            {#if supportsColor}
                <div class="field">
                    <label for="tas-color">Color</label>
                    <div class="color-row">
                        <input id="tas-color" type="color" bind:value={color}
                            oninput={onColorInput} disabled={!on} />
                        <span class="val mono">{color.toUpperCase()}</span>
                    </div>
                </div>
            {/if}
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    .note {
        font-size: 13px; color: var(--text-muted);
        padding: var(--space-2) 0;
    }
    .note.error {
        display: flex; flex-direction: column; gap: 4px;
        color: var(--danger);
    }
    .row {
        display: flex; align-items: center; gap: var(--space-3);
    }
    .swatch {
        width: 44px; height: 44px;
        border-radius: 50%;
        border: 1px solid var(--border);
        flex-shrink: 0;
        transition: background 0.15s;
    }
    .swatch.dim { opacity: 0.3; }
    .meta { flex: 1; min-width: 0; }
    .device-ip { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); }
    .hint { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
    .toggle { display: flex; align-items: center; gap: 6px; cursor: pointer; }
    .toggle-label { font-size: 13px; font-weight: 500; }

    .field { display: flex; flex-direction: column; gap: 6px; }
    .label-row {
        display: flex; justify-content: space-between; align-items: baseline;
    }
    .field label { font-size: 13px; font-weight: 500; }
    .val { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); font-variant-numeric: tabular-nums; }
    .mono { font-family: var(--font-mono); }

    input[type="range"] {
        width: 100%;
        accent-color: var(--primary);
    }
    input[type="range"]:disabled { opacity: 0.4; }

    input[type="range"].grad {
        appearance: none;
        height: 14px;
        border-radius: 7px;
        border: 1px solid var(--border);
        outline: none;
    }
    input[type="range"].grad::-webkit-slider-thumb {
        appearance: none;
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid var(--text);
        cursor: pointer;
        box-shadow: 0 1px 3px rgba(0,0,0,.3);
    }
    input[type="range"].grad::-moz-range-thumb {
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid var(--text);
        cursor: pointer;
    }

    /* Touch screens: bigger track + thumb so brightness/CT are easy to drag. */
    @media (pointer: coarse) {
        input[type="range"].grad { height: 18px; border-radius: 9px; }
        input[type="range"].grad::-webkit-slider-thumb { width: 28px; height: 28px; }
        input[type="range"].grad::-moz-range-thumb { width: 28px; height: 28px; }
    }

    .color-row {
        display: flex; align-items: center; gap: var(--space-3);
    }
    input[type="color"] {
        width: 48px; height: 36px;
        border: 1px solid var(--border);
        border-radius: var(--radius-sm);
        padding: 2px;
        cursor: pointer;
        background: transparent;
    }
    input[type="color"]:disabled { opacity: 0.4; cursor: not-allowed; }
</style>
