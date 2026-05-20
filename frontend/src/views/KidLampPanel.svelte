<script lang="ts">
    // A big, playful color + brightness playground for one smart bulb,
    // shown full-screen when a kid taps a Matter/Tasmota lamp. It speaks to
    // both protocols (Matter uses `level`, Tasmota uses `dimmer`) through a
    // tiny normalisation layer so the kid UI doesn't have to care which.
    import { onMount } from "svelte";
    import { fly, fade } from "svelte/transition";
    import { backOut } from "svelte/easing";
    import { api } from "../lib/api";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Socket } from "../lib/types";

    interface Props { socket: Socket; onClose: () => void; }
    let { socket, onClose }: Props = $props();

    const isMatter = $derived(socket.protocol === "matter" || socket.protocol === "matter-thread");

    let loading = $state(true);
    let on = $state(false);
    let brightness = $state(100);
    let color = $state("#FFD23F");
    let whiteMode = $state(false);   // true when lamp is in CT / white mode
    let supportsColor = $state(false);
    let supportsLevel = $state(false);

    // Big, saturated, kid-friendly colors (3×3 grid together with the white swatch).
    const SWATCHES = [
        { name: "Red",    hex: "FF4D4D" },
        { name: "Orange", hex: "FF9F1C" },
        { name: "Yellow", hex: "FFD23F" },
        { name: "Green",  hex: "3DDC97" },
        { name: "Cyan",   hex: "2EC4D6" },
        { name: "Blue",   hex: "4D9BFF" },
        { name: "Purple", hex: "B15DFF" },
        { name: "Pink",   hex: "FF5DA2" },
    ];

    onMount(async () => {
        try {
            const s = isMatter ? await api.matterGetState(socket.id) : await api.tasmotaGetState(socket.id);
            if (s.on != null) on = s.on;
            const lvl = isMatter ? (s as any).level : (s as any).dimmer;
            if (lvl != null) { brightness = lvl; supportsLevel = true; }
            if (s.color != null) { color = "#" + s.color.toUpperCase(); supportsColor = true; }
            else if (s.ct != null) { whiteMode = true; supportsColor = true; }
        } catch (e) {
            toasts.error("Couldn't reach the lamp", (e as Error).message);
        } finally {
            loading = false;
        }
    });

    // Debounce slider writes so dragging doesn't flood the bridge.
    let timer: ReturnType<typeof setTimeout> | undefined;
    let pending: Record<string, unknown> = {};
    function send(partial: Record<string, unknown>) {
        pending = { ...pending, ...partial };
        clearTimeout(timer);
        timer = setTimeout(async () => {
            const toSend = pending;
            pending = {};
            try {
                if (isMatter) await api.matterSetState(socket.id, toSend as any);
                else await api.tasmotaSetState(socket.id, toSend as any);
            } catch (e) {
                toasts.error("Oops!", (e as Error).message);
            }
        }, 120);
    }

    async function toggle() {
        const target = !on;
        on = target;
        try {
            if (target) await api.socketOn(socket.id);
            else await api.socketOff(socket.id);
            socket.state = target;
        } catch (e) {
            on = !target;
            toasts.error("Oops!", (e as Error).message);
        }
    }

    function pickColor(hex: string) {
        whiteMode = false;
        color = "#" + hex;
        on = true;
        socket.state = true;
        const key = isMatter ? "level" : "dimmer";
        send({ on: true, color: hex, [key]: brightness });
    }

    function pickWhite() {
        whiteMode = true;
        on = true;
        socket.state = true;
        const key = isMatter ? "level" : "dimmer";
        send({ on: true, ct: 370, [key]: brightness });
    }

    function onBrightness() {
        if (!on) { on = true; socket.state = true; }
        const key = isMatter ? "level" : "dimmer";
        send({ on: true, [key]: brightness });
    }

    const lampEmoji = $derived(socket.emoji && socket.emoji.trim() ? socket.emoji : "💡");

    // The big preview disc shows the chosen color, dimmed by brightness.
    const previewColor = $derived.by(() => {
        if (!on) return "var(--surface)";
        if (whiteMode) {
            // Warm white (~2700 K): #FFF9DC scaled by brightness
            const k = Math.max(0.25, brightness / 100);
            return `rgb(${Math.round(255 * k)}, ${Math.round(249 * k)}, ${Math.round(220 * k)})`;
        }
        const h = color.replace(/^#/, "");
        const r = parseInt(h.slice(0, 2), 16);
        const g = parseInt(h.slice(2, 4), 16);
        const b = parseInt(h.slice(4, 6), 16);
        const k = Math.max(0.25, brightness / 100);
        return `rgb(${Math.round(r * k)}, ${Math.round(g * k)}, ${Math.round(b * k)})`;
    });
</script>

<div class="overlay" transition:fade={{ duration: 200 }}>
    <div class="panel" in:fly={{ y: 40, duration: 350, easing: backOut }}>
        <header>
            <h2>{socket.name}</h2>
            <button class="close" onclick={onClose} aria-label="Close">✕</button>
        </header>

        <button class="preview" class:on onclick={toggle} aria-pressed={on}
            style:background={previewColor}>
            <span class="preview-emoji">{lampEmoji}</span>
            <span class="preview-state">{on ? "ON" : "OFF"}</span>
        </button>

        {#if loading}
            <p class="hint">Loading…</p>
        {:else}
            {#if supportsColor}
                <div class="swatches" role="group" aria-label="Pick a color">
                    <button
                        class="swatch white"
                        class:active={on && whiteMode}
                        aria-label="White"
                        onclick={pickWhite}
                    ></button>
                    {#each SWATCHES as s (s.hex)}
                        <button
                            class="swatch"
                            class:active={on && !whiteMode && color.toUpperCase() === "#" + s.hex}
                            style:background={"#" + s.hex}
                            aria-label={s.name}
                            onclick={() => pickColor(s.hex)}
                        ></button>
                    {/each}
                </div>
            {/if}

            {#if supportsLevel}
                <div class="bright">
                    <span class="sun small">☀️</span>
                    <input type="range" min="1" max="100" step="1"
                        bind:value={brightness} oninput={onBrightness}
                        aria-label="Brightness" />
                    <span class="sun big">☀️</span>
                </div>
            {/if}

            {#if !supportsColor && !supportsLevel}
                <p class="hint">Tap the big button to turn it on and off!</p>
            {/if}
        {/if}
    </div>
</div>

<style>
    .overlay {
        position: fixed;
        inset: 0;
        z-index: 150;
        background: rgba(10, 12, 24, 0.55);
        backdrop-filter: blur(6px);
        display: flex;
        align-items: flex-end;
        justify-content: center;
    }
    @media (min-width: 700px) { .overlay { align-items: center; } }
    .panel {
        width: 100%;
        max-width: 560px;
        background: var(--bg-elevated);
        border-top-left-radius: var(--radius-xl);
        border-top-right-radius: var(--radius-xl);
        border-radius: var(--radius-xl);
        padding: var(--space-5);
        padding-bottom: calc(var(--space-5) + env(safe-area-inset-bottom));
        display: flex;
        flex-direction: column;
        gap: var(--space-5);
        box-shadow: var(--shadow-lg);
    }
    header { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); }
    header h2 { font-size: clamp(1.4rem, 5vw, 2rem); font-weight: 800; }
    .close {
        width: 48px; height: 48px;
        border-radius: 50%;
        border: none;
        background: var(--surface-hover);
        color: var(--text);
        font-size: 1.4rem;
        font-weight: 700;
        cursor: pointer;
        flex-shrink: 0;
    }
    .close:active { transform: scale(0.92); }

    .preview {
        align-self: center;
        width: clamp(140px, 45vw, 200px);
        height: clamp(140px, 45vw, 200px);
        border-radius: 50%;
        border: 4px solid var(--border);
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: 6px;
        cursor: pointer;
        color: rgba(0, 0, 0, 0.55);
        transition: transform 0.18s ease, box-shadow 0.25s ease, background 0.25s ease, border-color 0.25s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .preview:active { transform: scale(0.95); }
    .preview.on {
        border-color: rgba(255, 255, 255, 0.6);
        box-shadow: 0 0 60px 4px var(--shadow-glow, rgba(255, 210, 63, 0.6)), 0 16px 50px rgba(0,0,0,0.3);
    }
    .preview:not(.on) { color: var(--text-faint); }
    .preview-emoji { font-size: clamp(3rem, 16vw, 5rem); line-height: 1; }
    .preview-state { font-size: 1rem; font-weight: 800; letter-spacing: 0.12em; }

    .swatches {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: var(--space-3);
    }
    .swatch {
        aspect-ratio: 1;
        border-radius: 50%;
        border: 4px solid transparent;
        cursor: pointer;
        box-shadow: inset 0 -6px 12px rgba(0,0,0,0.2), 0 4px 10px rgba(0,0,0,0.25);
        transition: transform 0.15s ease, border-color 0.15s ease;
        -webkit-tap-highlight-color: transparent;
    }
    .swatch:active { transform: scale(0.9); }
    .swatch.active { border-color: var(--text); transform: scale(1.08); }
    .swatch.white {
        background: radial-gradient(circle at 38% 38%, #ffffff, #fff8d6 55%, #ffe89a);
        border-color: #d4c060;
        box-shadow: inset 0 -4px 10px rgba(0,0,0,0.08), 0 4px 10px rgba(0,0,0,0.18);
    }
    .swatch.white.active { border-color: var(--text); }

    .bright {
        display: flex;
        align-items: center;
        gap: var(--space-3);
    }
    .sun.small { font-size: 1.1rem; opacity: 0.6; }
    .sun.big { font-size: 1.9rem; }
    .bright input[type="range"] {
        flex: 1;
        appearance: none;
        height: 26px;
        border-radius: 13px;
        background: linear-gradient(to right, var(--surface), #ffd23f);
        border: 1px solid var(--border);
        outline: none;
    }
    .bright input[type="range"]::-webkit-slider-thumb {
        appearance: none;
        width: 38px; height: 38px;
        border-radius: 50%;
        background: #fff;
        border: 3px solid #ffd23f;
        box-shadow: 0 2px 8px rgba(0,0,0,0.35);
        cursor: pointer;
    }
    .bright input[type="range"]::-moz-range-thumb {
        width: 38px; height: 38px;
        border-radius: 50%;
        background: #fff;
        border: 3px solid #ffd23f;
        box-shadow: 0 2px 8px rgba(0,0,0,0.35);
        cursor: pointer;
    }
    .hint { text-align: center; color: var(--text-muted); font-size: 1.05rem; font-weight: 600; }

    @media (prefers-reduced-motion: reduce) {
        .preview, .swatch, .close { transition: none; }
    }
</style>
