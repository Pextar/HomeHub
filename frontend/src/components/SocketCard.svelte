<script lang="ts">
    import Icon from "./Icon.svelte";
    import { untrack } from "svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Socket } from "../lib/types";
    import SocketModal from "../modals/SocketModal.svelte";
    import TimerModal from "../modals/TimerModal.svelte";
    import TasmotaLightModal from "../modals/TasmotaLightModal.svelte";
    import MatterLightModal from "../modals/MatterLightModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    const isTasmota = $derived(socket.protocol === "tasmota");
    const isMatter  = $derived(socket.protocol === "matter");
    const isSmartLight = $derived(isTasmota || isMatter);

    // One-shot "pulse" ring whenever the socket's state flips.
    let prevState = untrack(() => socket.state);
    let pulsing = $state(false);
    $effect(() => {
        const s = socket.state;
        if (s !== prevState) {
            prevState = s;
            pulsing = true;
            const t = setTimeout(() => { pulsing = false; }, 550);
            return () => clearTimeout(t);
        }
    });

    // --- Inline brightness (Tasmota + Matter share this row) ---
    // Lazy-loaded; the userTouched flag prevents a stale bridge response
    // from overwriting a value the user is actively dragging. We also
    // remember the current color (Matter) so the slider track can be tinted
    // to match the bulb — makes a wall of cards much easier to scan.
    let brightness = $state<number | null>(null);
    let tintColor = $state<string | null>(null);
    let userTouched = $state(false);
    $effect(() => {
        if (!isSmartLight || brightness !== null || userTouched) return;
        if (isTasmota) {
            api.tasmotaGetState(socket.id).then(s => {
                if (!userTouched && s.dimmer != null) brightness = s.dimmer;
                if (s.color) tintColor = "#" + s.color.toLowerCase();
            }).catch(() => {});
        } else if (isMatter) {
            api.matterGetState(socket.id).then(s => {
                if (!userTouched && s.level != null) brightness = s.level;
                if (s.color) tintColor = "#" + s.color.toLowerCase();
            }).catch(() => {});
        }
    });

    let dimmerTimer: ReturnType<typeof setTimeout> | undefined;
    function onDimmerInput() {
        userTouched = true;
        if (brightness === null) return;
        const value = brightness;
        clearTimeout(dimmerTimer);
        dimmerTimer = setTimeout(async () => {
            try {
                if (isTasmota)     await api.tasmotaSetState(socket.id, { dimmer: value });
                else if (isMatter) await api.matterSetState(socket.id,  { level: value });
            } catch (e) {
                toasts.error("Brightness update failed", (e as Error).message);
            }
        }, 150);
    }

    async function toggleFavorite() {
        try {
            await api.socketToggleFavorite(socket.id);
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }

    async function confirmDelete() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete device?",
            message: `"${socket.name}" and any schedules pointing to it will be removed.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSocket(socket.id);
            toasts.success("Device deleted", socket.name);
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }
</script>

<article class="card" class:on={socket.state} class:pulsing>
    <div class="head">
        {#if isTasmota}
            <button class="title-btn" onclick={() => openModal(TasmotaLightModal, { socket })}
                title="Open device controls">
                <div class="title">
                    <div class="name">{socket.name}</div>
                    <div class="meta">{socket.room || "Unassigned"}</div>
                </div>
            </button>
        {:else if isMatter}
            <button class="title-btn" onclick={() => openModal(MatterLightModal, { socket })}
                title="Open device controls">
                <div class="title">
                    <div class="name">{socket.name}</div>
                    <div class="meta">{socket.room || "Unassigned"}</div>
                </div>
            </button>
        {:else}
            <div class="title">
                <div class="name" title={socket.name}>{socket.name}</div>
                <div class="meta">{socket.room || "Unassigned"}</div>
            </div>
        {/if}
        <div class="menu">
            <button class="icon-btn fav-btn" class:fav={socket.favorite}
                title={socket.favorite ? "Remove from favorites" : "Add to favorites"}
                aria-label={socket.favorite ? "Remove from favorites" : "Add to favorites"}
                aria-pressed={socket.favorite}
                onclick={toggleFavorite}>
                <Icon name={socket.favorite ? "star" : "starOutline"} size={16} />
            </button>
            <button class="icon-btn" title="Set timer" aria-label="Set timer"
                onclick={() => openModal(TimerModal, { socket })}>
                <Icon name="timer" size={16} />
            </button>
            <button class="icon-btn" title="Edit" aria-label="Edit"
                onclick={() => openModal(SocketModal, { existing: socket })}>
                <Icon name="edit" size={16} />
            </button>
            <button class="icon-btn danger" title="Delete" aria-label="Delete"
                onclick={confirmDelete}>
                <Icon name="trash" size={16} />
            </button>
        </div>
    </div>
    <div class="status">
        <span class="dot"></span>
        <span class="state">{socket.state ? "ON" : "OFF"}</span>
        <span class="code-chip"
            data-proto={isTasmota ? "tasmota" : isMatter ? "matter" : "rf"}
            title={isTasmota ? "Tasmota device IP" : isMatter ? "Matter device" : "RF code"}>
            {socket.protocol || "rf"} · {socket.code}
        </span>
    </div>
    {#if isSmartLight}
        <div class="dim-row" class:disabled={!socket.state} class:loading={brightness === null}>
            <Icon name="sun" size={14} />
            <input type="range" min="1" max="100" step="1"
                value={brightness ?? 50}
                oninput={(e) => { brightness = +(e.currentTarget as HTMLInputElement).value; onDimmerInput(); }}
                disabled={!socket.state || brightness === null}
                aria-label="Brightness"
                style:--tint={tintColor || "var(--accent, #60a5fa)"} />
            <span class="dim-val">{brightness === null ? "—" : brightness + "%"}</span>
        </div>
    {/if}
    <div class="controls">
        <button class="btn btn-success" disabled={socket.state}
            onclick={() => runAction(() => api.socketOn(socket.id), `Turned on ${socket.name}`)}>
            On
        </button>
        <button class="btn btn-danger" disabled={!socket.state}
            onclick={() => runAction(() => api.socketOff(socket.id), `Turned off ${socket.name}`)}>
            Off
        </button>
        <button class="btn"
            onclick={() => runAction(() => api.socketToggle(socket.id), `Toggled ${socket.name}`)}>
            Toggle
        </button>
    </div>
</article>

<style>
    .card {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        transition: border-color var(--t-fast), transform var(--t-fast), box-shadow var(--t-fast);
    }
    .card:hover { border-color: var(--border-strong); }
    @media (hover: hover) {
        .card:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .card.on { border-color: var(--success); box-shadow: inset 0 0 0 1px var(--success-soft); }

    .card.pulsing.on { animation: pulse-on 0.55s ease-out; }
    .card.pulsing:not(.on) { animation: pulse-off 0.55s ease-out; }
    @keyframes pulse-on {
        0%   { box-shadow: inset 0 0 0 1px var(--success-soft), 0 0 0 0 rgba(52, 211, 153, 0.55); }
        100% { box-shadow: inset 0 0 0 1px var(--success-soft), 0 0 0 16px rgba(52, 211, 153, 0); }
    }
    @keyframes pulse-off {
        0%   { box-shadow: 0 0 0 0 rgba(148, 163, 184, 0.45); }
        100% { box-shadow: 0 0 0 14px rgba(148, 163, 184, 0); }
    }

    .head {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .title { min-width: 0; }
    .name { font-weight: 600; font-size: 1rem; line-height: 1.3; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .meta { color: var(--text-muted); font-size: 12px; margin-top: 2px; }
    .menu { display: flex; gap: 4px; }
    .fav-btn.fav { color: #f5c518; }
    .fav-btn.fav:hover { color: #fbbf24; }

    .title-btn {
        all: unset;
        cursor: pointer;
        flex: 1;
        min-width: 0;
        border-radius: var(--radius-sm);
        padding: 2px 4px;
        margin: -2px -4px;
    }
    .title-btn:focus-visible { outline: 2px solid var(--accent, #60a5fa); }
    .title-btn:hover .name { text-decoration: underline; text-decoration-color: var(--border-strong); }

    .status {
        display: flex; align-items: center; gap: var(--space-2);
        color: var(--text-muted); font-size: 13px;
    }
    .dot {
        width: 10px; height: 10px;
        border-radius: 50%;
        background: var(--text-faint);
        transition: background var(--t-fast), box-shadow var(--t-fast);
    }
    .card.on .dot { background: var(--success); box-shadow: 0 0 0 4px var(--success-soft); }
    .card.on .state { color: var(--success); font-weight: 600; }

    .code-chip[data-proto="tasmota"] {
        color: var(--accent-wifi);
        border-color: var(--accent-wifi-soft);
        background: var(--accent-wifi-soft);
    }
    .code-chip[data-proto="matter"] {
        color: var(--accent-matter);
        border-color: var(--accent-matter-soft);
        background: var(--accent-matter-soft);
    }
    .code-chip[data-proto="rf"] {
        color: var(--accent-rf);
        border-color: var(--accent-rf-soft);
        background: var(--accent-rf-soft);
    }

    .dim-row {
        display: flex; align-items: center; gap: var(--space-2);
        color: var(--text-muted); font-size: 12px;
        padding: 4px 0;
    }
    .dim-row.disabled { opacity: 0.4; }
    .dim-row.loading { opacity: 0.5; }
    .dim-row input[type="range"] {
        flex: 1;
        appearance: none;
        height: 8px;
        border-radius: 4px;
        background: linear-gradient(to right,
            color-mix(in srgb, var(--tint) 65%, transparent),
            var(--tint));
        outline: none;
        border: 1px solid var(--border);
        cursor: pointer;
    }
    .dim-row input[type="range"]:disabled { cursor: not-allowed; }
    .dim-row input[type="range"]::-webkit-slider-thumb {
        appearance: none;
        width: 16px; height: 16px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0,0,0,0.3);
        cursor: pointer;
        box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
    .dim-row input[type="range"]::-moz-range-thumb {
        width: 16px; height: 16px;
        border-radius: 50%;
        background: #fff;
        border: 2px solid rgba(0,0,0,0.3);
        cursor: pointer;
        box-shadow: 0 1px 3px rgba(0,0,0,0.3);
    }
    .dim-val { font-variant-numeric: tabular-nums; min-width: 36px; text-align: right; }

    .controls {
        display: grid;
        grid-template-columns: 1fr 1fr 1fr;
        gap: var(--space-2);
    }
</style>
