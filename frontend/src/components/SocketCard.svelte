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
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    const isTasmota = $derived(socket.protocol === "tasmota");

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

    // --- Tasmota inline brightness ---
    // Lazy-loaded; the userTouched flag prevents a stale bridge response
    // from overwriting a value the user is actively dragging.
    let tasmotaDimmer = $state<number | null>(null);
    let userTouched = $state(false);
    $effect(() => {
        if (!isTasmota || tasmotaDimmer !== null || userTouched) return;
        api.tasmotaGetState(socket.id).then(s => {
            if (!userTouched && s.dimmer != null) tasmotaDimmer = s.dimmer;
        }).catch(() => {});
    });

    let dimmerTimer: ReturnType<typeof setTimeout> | undefined;
    function onDimmerInput() {
        userTouched = true;
        if (tasmotaDimmer === null) return;
        const value = tasmotaDimmer;
        clearTimeout(dimmerTimer);
        dimmerTimer = setTimeout(async () => {
            try {
                await api.tasmotaSetState(socket.id, { dimmer: value });
            } catch (e) {
                toasts.error("Brightness update failed", (e as Error).message);
            }
        }, 150);
    }

    async function confirmDelete() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete socket?",
            message: `"${socket.name}" and any schedules pointing to it will be removed.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSocket(socket.id);
            toasts.success("Socket deleted", socket.name);
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
        {:else}
            <div class="title">
                <div class="name" title={socket.name}>{socket.name}</div>
                <div class="meta">{socket.room || "Unassigned"}</div>
            </div>
        {/if}
        <div class="menu">
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
        <span class="code-chip" title={isTasmota ? "Device IP" : "RF code"}>
            {socket.protocol || "raw"} · {socket.code}
        </span>
    </div>
    {#if isTasmota && tasmotaDimmer !== null}
        <div class="dim-row" class:disabled={!socket.state}>
            <Icon name="sun" size={14} />
            <input type="range" min="1" max="100" step="1"
                bind:value={tasmotaDimmer}
                oninput={onDimmerInput}
                disabled={!socket.state}
                aria-label="Brightness" />
            <span class="dim-val">{tasmotaDimmer}%</span>
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

    .dim-row {
        display: flex; align-items: center; gap: var(--space-2);
        color: var(--text-muted); font-size: 12px;
    }
    .dim-row.disabled { opacity: 0.4; }
    .dim-row input[type="range"] { flex: 1; accent-color: var(--accent, #60a5fa); }
    .dim-val { font-variant-numeric: tabular-nums; min-width: 32px; text-align: right; }

    .controls {
        display: grid;
        grid-template-columns: 1fr 1fr 1fr;
        gap: var(--space-2);
    }
</style>
