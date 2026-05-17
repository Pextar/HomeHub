<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Socket, HueLight, HueStateUpdate } from "../lib/types";
    import { onMount } from "svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    let light = $state<HueLight | null>(null);
    let loading = $state(true);
    let error = $state<string | null>(null);

    // Local optimistic values, populated once the light is loaded.
    let on = $state(socket.state);
    let bri = $state(127);
    let ct = $state(366);
    let hueVal = $state(0);
    let sat = $state(254);
    let mode = $state<"ct" | "color">("ct");

    const supportsBri = $derived(light?.state.bri !== undefined && light?.state.bri !== null);
    const supportsCT = $derived(light?.state.ct !== undefined && light?.state.ct !== null);
    const supportsColor = $derived(light?.state.hue !== undefined && light?.state.hue !== null);

    onMount(async () => {
        try {
            const l = await api.hueGetLight(socket.code);
            light = l;
            on = l.state.on;
            if (l.state.bri != null) bri = l.state.bri;
            if (l.state.ct != null) ct = l.state.ct;
            if (l.state.hue != null) hueVal = l.state.hue;
            if (l.state.sat != null) sat = l.state.sat;
            mode = l.state.colormode === "hs" || l.state.colormode === "xy" ? "color" : "ct";
        } catch (e) {
            error = (e as Error).message;
        } finally {
            loading = false;
        }
    });

    // --- Debounced send ---
    let debounceTimer: ReturnType<typeof setTimeout> | undefined;
    let pending: HueStateUpdate = {};
    function send(partial: HueStateUpdate) {
        pending = { ...pending, ...partial };
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(async () => {
            const toSend = pending;
            pending = {};
            try {
                await api.hueSetState(socket.code, toSend);
            } catch (e) {
                toasts.error("Hue update failed", (e as Error).message);
            }
        }, 120);
    }

    // --- Handlers ---
    async function toggleOn() {
        const target = !on;
        on = target; // optimistic
        try {
            if (target) await api.socketOn(socket.id);
            else        await api.socketOff(socket.id);
            await data.refresh();
        } catch (e) {
            on = !target;
            toasts.error("Toggle failed", (e as Error).message);
        }
    }

    function onBriInput() {
        if (!on) return;
        send({ bri });
    }
    function onCTInput() {
        if (!on) return;
        mode = "ct";
        send({ ct });
    }
    function onHueInput() {
        if (!on) return;
        mode = "color";
        send({ hue: hueVal, sat });
    }
    function onSatInput() {
        if (!on) return;
        mode = "color";
        send({ hue: hueVal, sat });
    }

    // --- Color preview helpers ---
    // ctToHex maps a mired color-temperature to an approximate sRGB hex,
    // just for the slider's gradient fill. ~2000K (500 mired) = warm,
    // ~6500K (153 mired) = cool.
    function ctGradient(): string {
        return "linear-gradient(to right, #b4d8ff 0%, #ffffff 50%, #ffd6a8 100%)";
    }
    function hueGradient(): string {
        // 7 stops across the 0..65535 hue range.
        return "linear-gradient(to right, #ff0000, #ffff00, #00ff00, #00ffff, #0000ff, #ff00ff, #ff0000)";
    }
    function satGradient(): string {
        const baseH = Math.round((hueVal / 65535) * 360);
        return `linear-gradient(to right, hsl(${baseH}, 0%, 80%), hsl(${baseH}, 100%, 50%))`;
    }
    function colorSwatch(): string {
        if (mode === "ct") return "var(--bg-base)";
        const h = Math.round((hueVal / 65535) * 360);
        const s = Math.round((sat / 254) * 100);
        const l = 50;
        return `hsl(${h}, ${s}%, ${l}%)`;
    }
</script>

<Modal title={socket.name} subtitle={socket.room || "Unassigned"}>
    {#snippet body()}
        {#if loading}
            <div class="loading">Loading lamp state…</div>
        {:else if error}
            <div class="error">
                <p>Couldn't reach the Hue bridge.</p>
                <p class="msg">{error}</p>
            </div>
        {:else if light}
            <div class="row">
                <div class="swatch" style:background={colorSwatch()} class:dim={!on}></div>
                <div class="meta">
                    <div class="type">{light.type || "Hue light"}</div>
                    {#if !light.state.reachable}
                        <div class="warn">Bridge reports light unreachable</div>
                    {/if}
                </div>
                <label class="switch">
                    <input type="checkbox" checked={on} onchange={toggleOn} />
                    <span>{on ? "On" : "Off"}</span>
                </label>
            </div>

            {#if supportsBri}
                <div class="field">
                    <div class="label-row">
                        <label for="hue-bri">Brightness</label>
                        <span class="value">{Math.round((bri / 254) * 100)}%</span>
                    </div>
                    <input id="hue-bri" type="range" min="1" max="254" step="1"
                        bind:value={bri} oninput={onBriInput} disabled={!on} />
                </div>
            {/if}

            {#if supportsCT}
                <div class="field">
                    <div class="label-row">
                        <label for="hue-ct">Warmth</label>
                        <span class="value">{Math.round(1_000_000 / ct)}K</span>
                    </div>
                    <input id="hue-ct" type="range" min="153" max="500" step="1"
                        bind:value={ct} oninput={onCTInput} disabled={!on}
                        style:background={ctGradient()} class="gradient-slider" />
                </div>
            {/if}

            {#if supportsColor}
                <div class="field">
                    <div class="label-row">
                        <label for="hue-hue">Color</label>
                        <span class="value">{Math.round((hueVal / 65535) * 360)}°</span>
                    </div>
                    <input id="hue-hue" type="range" min="0" max="65535" step="1"
                        bind:value={hueVal} oninput={onHueInput} disabled={!on}
                        style:background={hueGradient()} class="gradient-slider" />
                </div>
                <div class="field">
                    <div class="label-row">
                        <label for="hue-sat">Saturation</label>
                        <span class="value">{Math.round((sat / 254) * 100)}%</span>
                    </div>
                    <input id="hue-sat" type="range" min="0" max="254" step="1"
                        bind:value={sat} oninput={onSatInput} disabled={!on}
                        style:background={satGradient()} class="gradient-slider" />
                </div>
            {/if}

            {#if !supportsBri && !supportsCT && !supportsColor}
                <div class="loading">This light only supports on/off.</div>
            {/if}
        {/if}
    {/snippet}
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => closeModal()}>Done</button>
    {/snippet}
</Modal>

<style>
    .loading, .error {
        color: var(--text-muted);
        font-size: 13px;
        padding: var(--space-3);
    }
    .error .msg { font-family: ui-monospace, monospace; margin-top: 4px; }
    .row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .swatch {
        width: 48px; height: 48px;
        border-radius: 50%;
        border: 1px solid var(--border);
        flex-shrink: 0;
        transition: background 0.15s;
        box-shadow: 0 0 0 4px var(--bg-base);
    }
    .swatch.dim { opacity: 0.3; }
    .meta { flex: 1; min-width: 0; }
    .type { font-size: 13px; color: var(--text-muted); }
    .warn { font-size: 12px; color: var(--warn, #f59e0b); margin-top: 2px; }
    .switch {
        display: flex; align-items: center; gap: 6px;
        font-size: 13px; font-weight: 500;
    }

    .field { display: flex; flex-direction: column; gap: 6px; }
    .label-row {
        display: flex; justify-content: space-between; align-items: baseline;
    }
    .label-row label { font-size: 13px; font-weight: 500; }
    .value { font-size: 12px; color: var(--text-muted); font-variant-numeric: tabular-nums; }

    input[type="range"] {
        width: 100%;
        accent-color: var(--accent, #60a5fa);
    }
    input[type="range"]:disabled { opacity: 0.4; }
    input[type="range"].gradient-slider {
        appearance: none;
        height: 14px;
        border-radius: 7px;
        border: 1px solid var(--border);
        outline: none;
    }
    input[type="range"].gradient-slider::-webkit-slider-thumb {
        appearance: none;
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid var(--text);
        cursor: pointer;
        box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
    input[type="range"].gradient-slider::-moz-range-thumb {
        width: 18px; height: 18px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid var(--text);
        cursor: pointer;
        box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
</style>
