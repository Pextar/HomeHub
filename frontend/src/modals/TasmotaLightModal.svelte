<!--
  Smart-light control surface for a Tasmota Wi-Fi device.

  Layout (top to bottom):
    1. Preview disc + on/off toggle (the disc shows the current color/CT,
       dimmed proportional to brightness).
    2. Color / White mode tabs — only shown if the bulb supports both;
       the picker switches between an HSV wheel and a CT gradient slider.
    3. Brightness slider with a gradient track.
    4. Preset scene chips (Reading, Relax, Daylight, …) — a one-tap way
       to jump to a sensible color+CT+brightness combo without fiddling.

  We debounce outbound writes (120 ms) so dragging a slider doesn't hammer
  the device with a request on every pixel of movement.
-->
<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import ColorWheel from "../components/ColorWheel.svelte";
    import Icon from "../components/Icon.svelte";
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

    let on = $state(false);
    let dimmer = $state(100);
    let color = $state("#FFFFFF");
    let ct = $state(366);
    // "color" or "white" — only meaningful when the device supports both.
    let mode = $state<"color" | "white">("white");

    const supportsDimmer = $derived(deviceState?.dimmer !== undefined && deviceState?.dimmer !== null);
    const supportsColor  = $derived(deviceState?.color  !== undefined && deviceState?.color  !== null);
    const supportsCT     = $derived(deviceState?.ct     !== undefined && deviceState?.ct     !== null);
    const supportsBoth   = $derived(supportsColor && supportsCT);

    onMount(async () => {
        try {
            const s = await api.tasmotaGetState(socket.id);
            deviceState = s;
            on = s.on;
            if (s.dimmer != null) dimmer = s.dimmer;
            if (s.color)          { color = "#" + s.color.toUpperCase(); mode = "color"; }
            if (s.ct != null)     { ct = s.ct; if (!s.color) mode = "white"; }
            if (!s.color && !s.ct && supportsColor) mode = "color";
        } catch (e) {
            error = (e as Error).message;
        } finally {
            loading = false;
        }
    });

    // Coalesce rapid updates while a slider is being dragged. We track each
    // field independently so e.g. a color change doesn't drop a pending CT.
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

    function onDimmerInput() { if (on) send({ dimmer }); }
    function onCTInput()     { if (on) send({ ct }); mode = "white"; }
    function onColorChange(hex: string) {
        color = hex;
        if (on) send({ color: hex.replace("#", "").toUpperCase() });
        mode = "color";
    }

    // --- Preset scenes ----------------------------------------------------
    interface Preset {
        key: string;
        label: string;
        kind: "white" | "color";
        dimmer: number;
        ct?: number;       // mireds
        color?: string;    // RRGGBB
    }
    const presets: Preset[] = [
        { key: "read",        label: "Reading",     kind: "white", dimmer: 100, ct: 250 },
        { key: "concentrate", label: "Concentrate", kind: "white", dimmer: 100, ct: 180 },
        { key: "relax",       label: "Relax",       kind: "white", dimmer: 40,  ct: 400 },
        { key: "night",       label: "Night",       kind: "white", dimmer: 12,  ct: 454 },
        { key: "warm",        label: "Warm",        kind: "white", dimmer: 80,  ct: 370 },
        { key: "daylight",    label: "Daylight",    kind: "white", dimmer: 100, ct: 200 },
        { key: "sunset",      label: "Sunset",      kind: "color", dimmer: 70,  color: "FF6A3D" },
        { key: "forest",      label: "Forest",      kind: "color", dimmer: 60,  color: "3DBF6A" },
        { key: "ocean",       label: "Ocean",       kind: "color", dimmer: 70,  color: "3DAFFF" },
        { key: "lavender",    label: "Lavender",    kind: "color", dimmer: 60,  color: "B47CFF" },
        { key: "rose",        label: "Rose",        kind: "color", dimmer: 60,  color: "FF6FA3" },
    ];
    const availablePresets = $derived(
        presets.filter(p => p.kind === "white" ? supportsCT : supportsColor)
    );

    async function applyPreset(p: Preset) {
        const update: TasmotaStateUpdate = { on: true, dimmer: p.dimmer };
        if (p.kind === "color" && supportsColor && p.color) update.color = p.color;
        if (p.kind === "white" && supportsCT    && p.ct != null) update.ct = p.ct;
        on = true;
        dimmer = p.dimmer;
        if (p.color) color = "#" + p.color;
        if (p.ct)    ct = p.ct;
        mode = p.kind;
        try {
            await api.tasmotaSetState(socket.id, update);
        } catch (e) {
            toasts.error("Preset failed", (e as Error).message);
        }
    }

    // The preview disc reflects the current selection.
    const previewColor = $derived.by(() => {
        if (!on) return "var(--surface)";
        if (mode === "color") return tintForLevel(color, dimmer);
        return ctToCss(ct, dimmer);
    });

    function tintForLevel(hex: string, lvl: number): string {
        const h = hex.replace(/^#/, "");
        const r = parseInt(h.slice(0, 2), 16);
        const g = parseInt(h.slice(2, 4), 16);
        const b = parseInt(h.slice(4, 6), 16);
        const k = Math.max(0.18, lvl / 100);
        return `rgb(${Math.round(r * k)}, ${Math.round(g * k)}, ${Math.round(b * k)})`;
    }
    function ctToCss(mireds: number, lvl: number): string {
        // 153 mired = cool (≈6500K, ~#cee9ff), 500 mired = warm (≈2000K, ~#ffb86b).
        const t = Math.max(0, Math.min(1, (mireds - 153) / (500 - 153)));
        const cool = [206, 233, 255];
        const warm = [255, 184, 107];
        const r = Math.round(cool[0] + (warm[0] - cool[0]) * t);
        const g = Math.round(cool[1] + (warm[1] - cool[1]) * t);
        const b = Math.round(cool[2] + (warm[2] - cool[2]) * t);
        const k = Math.max(0.18, lvl / 100);
        return `rgb(${Math.round(r * k)}, ${Math.round(g * k)}, ${Math.round(b * k)})`;
    }

    const kelvinLabel = $derived(`${Math.round(1_000_000 / ct / 50) * 50}K`);
</script>

<!-- Controls apply live — nothing to "discard" on dismiss. -->
<Modal title={socket.name} subtitle={socket.room || "Unassigned"} guardUnsaved={false}>
    {#snippet body()}
        {#if loading}
            <div class="note">Loading device state…</div>
        {:else if error}
            <div class="note error">
                <strong>Could not reach device</strong>
                <span>{error}</span>
            </div>
        {:else if deviceState}
            <div class="preview-row">
                <button class="preview" onclick={toggleOn} aria-pressed={on}
                    aria-label={on ? "Turn off" : "Turn on"}>
                    <div class="halo" style:background={previewColor} class:off={!on}></div>
                    <div class="bulb" style:background={previewColor} class:off={!on}>
                        <Icon name="light" size={36} />
                    </div>
                    <div class="state-text">{on ? "ON" : "OFF"}</div>
                </button>
                <div class="meta-col">
                    <div class="device-ip">{socket.code}</div>
                    {#if !supportsDimmer && !supportsColor && !supportsCT}
                        <div class="hint">On/off only</div>
                    {/if}
                </div>
            </div>

            {#if supportsBoth}
                <div class="mode-tabs" role="tablist">
                    <button class="tab" class:active={mode === "color"}
                        role="tab" aria-selected={mode === "color"}
                        onclick={() => mode = "color"}>Color</button>
                    <button class="tab" class:active={mode === "white"}
                        role="tab" aria-selected={mode === "white"}
                        onclick={() => mode = "white"}>White</button>
                </div>
            {/if}

            {#if supportsColor && (mode === "color" || !supportsCT)}
                <div class="wheel-center">
                    <ColorWheel {color} onChange={onColorChange} disabled={!on} size={240} />
                    <div class="hex-label">{color}</div>
                </div>
            {/if}

            {#if supportsCT && (mode === "white" || !supportsColor)}
                <div class="field">
                    <div class="label-row">
                        <label for="tas-ct">Warmth</label>
                        <span class="val">{kelvinLabel}</span>
                    </div>
                    <input id="tas-ct" type="range" min="153" max="500" step="1"
                        bind:value={ct} oninput={onCTInput} disabled={!on}
                        class="ct-slider" />
                </div>
            {/if}

            {#if supportsDimmer}
                <div class="field">
                    <div class="label-row">
                        <label for="tas-dim">
                            <Icon name="sun" size={14} /> Brightness
                        </label>
                        <span class="val">{dimmer}%</span>
                    </div>
                    <input id="tas-dim" type="range" min="1" max="100" step="1"
                        bind:value={dimmer} oninput={onDimmerInput} disabled={!on}
                        class="level-slider"
                        style:--track-color={previewColor} />
                </div>
            {/if}

            {#if availablePresets.length > 0}
                <div class="presets">
                    <div class="presets-label">Scenes</div>
                    <div class="preset-grid">
                        {#each availablePresets as p (p.key)}
                            <button class="preset" onclick={() => applyPreset(p)}
                                style:--swatch={p.kind === "color"
                                    ? tintForLevel("#" + (p.color ?? "FFFFFF"), p.dimmer)
                                    : ctToCss(p.ct ?? 366, p.dimmer)}>
                                <span class="preset-dot"></span>
                                <span class="preset-label">{p.label}</span>
                            </button>
                        {/each}
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
    .note { font-size: 13px; color: var(--text-muted); padding: var(--space-2) 0; }
    .note.error { display: flex; flex-direction: column; gap: 4px; color: var(--danger); }

    /* --- preview row ---------------------------------------------------- */
    .preview-row {
        display: flex;
        align-items: center;
        gap: var(--space-4);
        padding: var(--space-2) 0 var(--space-1);
    }
    .preview {
        all: unset;
        position: relative;
        width: 96px;
        height: 96px;
        flex-shrink: 0;
        cursor: pointer;
        display: grid;
        place-items: center;
        border-radius: 50%;
    }
    .preview:focus-visible { outline: 2px solid var(--primary); outline-offset: 4px; }
    .halo {
        position: absolute;
        inset: -6px;
        border-radius: 50%;
        filter: blur(16px);
        opacity: 0.55;
        transition: background 0.2s, opacity 0.2s;
    }
    .halo.off { opacity: 0.15; }
    .bulb {
        position: relative;
        width: 88px;
        height: 88px;
        border-radius: 50%;
        display: grid;
        place-items: center;
        color: rgba(0,0,0,0.55);
        border: 1px solid rgba(255,255,255,0.18);
        box-shadow: inset 0 -8px 16px rgba(0,0,0,0.2), inset 0 8px 16px rgba(255,255,255,0.25);
        transition: background 0.2s;
    }
    .bulb.off {
        color: var(--text-faint);
        background: var(--bg-elevated) !important;
        box-shadow: inset 0 0 0 1px var(--border);
    }
    .state-text {
        position: absolute;
        bottom: -22px;
        left: 50%;
        transform: translateX(-50%);
        font-size: 11px;
        letter-spacing: 0.06em;
        font-weight: 600;
        color: var(--text-muted);
    }
    .meta-col {
        display: flex; flex-direction: column; gap: 4px;
        flex: 1; min-width: 0;
    }
    .device-ip { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); }
    .hint { font-size: 12px; color: var(--text-muted); }

    /* --- mode tabs ------------------------------------------------------ */
    .mode-tabs {
        display: flex;
        gap: 4px;
        padding: 4px;
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        margin-top: var(--space-4);
    }
    .tab {
        all: unset;
        flex: 1;
        text-align: center;
        padding: 8px 0;
        font-size: 13px;
        font-weight: 500;
        color: var(--text-muted);
        border-radius: calc(var(--radius-md) - 4px);
        cursor: pointer;
        transition: background 0.15s, color 0.15s;
    }
    .tab:hover { color: var(--text); }
    .tab.active {
        background: var(--bg-elevated);
        color: var(--text);
        box-shadow: 0 1px 2px rgba(0,0,0,0.15);
    }

    /* --- color wheel ---------------------------------------------------- */
    .wheel-center {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-4) 0 var(--space-2);
    }
    .hex-label {
        font-family: var(--font-mono);
        font-size: 12px;
        color: var(--text-muted);
        letter-spacing: 0.05em;
        font-variant-numeric: tabular-nums;
    }

    /* --- sliders -------------------------------------------------------- */
    .field {
        display: flex; flex-direction: column; gap: 8px;
        margin-top: var(--space-3);
    }
    .label-row { display: flex; justify-content: space-between; align-items: center; }
    .field label {
        font-size: 13px; font-weight: 500;
        display: inline-flex; align-items: center; gap: 6px;
        color: var(--text);
    }
    .val { font-size: 12px; color: var(--text-muted); font-family: var(--font-mono); font-variant-numeric: tabular-nums; }

    input[type="range"] {
        width: 100%;
        appearance: none;
        height: 14px;
        border-radius: 7px;
        outline: none;
        background: var(--surface);
        border: 1px solid var(--border);
    }
    input[type="range"]:disabled { opacity: 0.4; }
    input[type="range"]::-webkit-slider-thumb {
        appearance: none;
        width: 22px; height: 22px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0,0,0,0.4);
        cursor: pointer;
        box-shadow: 0 2px 6px rgba(0,0,0,0.35);
        margin-top: 0;
    }
    input[type="range"]::-moz-range-thumb {
        width: 22px; height: 22px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0,0,0,0.4);
        cursor: pointer;
        box-shadow: 0 2px 6px rgba(0,0,0,0.35);
    }
    .ct-slider {
        background: linear-gradient(to right, #cee9ff 0%, #ffffff 50%, #ffb86b 100%);
        border: 1px solid rgba(0,0,0,0.15);
    }
    .level-slider {
        background: linear-gradient(
            to right,
            rgba(0,0,0,0.5),
            var(--track-color, var(--primary))
        );
        border: 1px solid rgba(0,0,0,0.15);
    }

    /* --- preset chips --------------------------------------------------- */
    .presets {
        margin-top: var(--space-5);
        padding-top: var(--space-4);
        border-top: 1px solid var(--border);
    }
    .presets-label {
        font-size: 11px;
        font-weight: 600;
        color: var(--text-muted);
        font-family: var(--font-mono);
        letter-spacing: 0.08em;
        text-transform: uppercase;
        margin-bottom: var(--space-3);
    }
    .preset-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
        gap: var(--space-2);
    }
    .preset {
        all: unset;
        cursor: pointer;
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 8px 10px;
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        background: var(--bg-elevated);
        font-size: 12px;
        font-weight: 500;
        color: var(--text);
        transition: border-color 0.15s, transform 0.15s, background 0.15s;
    }
    .preset:hover { border-color: var(--border-strong); transform: translateY(-1px); }
    .preset:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
    .preset:active { transform: scale(0.97); }
    .preset-dot {
        width: 14px; height: 14px; border-radius: 50%;
        background: var(--swatch, #888);
        box-shadow: 0 1px 2px rgba(0,0,0,0.3), inset 0 0 0 1px rgba(255,255,255,0.18);
        flex-shrink: 0;
    }
    .preset-label {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    /* Touch screens: tabs, presets and slider thumbs need bigger targets. */
    @media (pointer: coarse) {
        .tab { padding: 12px 0; font-size: 15px; }
        .preset { padding: 12px; min-height: 44px; font-size: 14px; }
        input[type="range"] { height: 18px; }
        input[type="range"]::-webkit-slider-thumb { width: 28px; height: 28px; }
        input[type="range"]::-moz-range-thumb { width: 28px; height: 28px; }
    }
</style>
