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
    import HueLightModal from "../modals/HueLightModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

    const isHue = $derived(socket.protocol === "hue");

    // One-shot "pulse" ring whenever the socket's state flips, so a
    // remote toggle or schedule fire is visually obvious.
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

    // --- Hue inline brightness ---
    // Lazy-loaded on first render of a Hue card. Slider edits are
    // debounced; we never reset back to bridge state, so a stale fetch
    // can't clobber a fresh user drag.
    let hueBri = $state<number | null>(null);
    let userTouched = $state(false);
    $effect(() => {
        if (!isHue || hueBri !== null || userTouched) return;
        // Fire-and-forget; ignore failures so the card still works without a bridge.
        api.hueGetLight(socket.code).then(l => {
            if (!userTouched && l.state.bri != null) hueBri = l.state.bri;
        }).catch(() => {});
    });

    let briTimer: ReturnType<typeof setTimeout> | undefined;
    function onBriInput() {
        userTouched = true;
        if (hueBri === null) return;
        clearTimeout(briTimer);
        const value = hueBri;
        briTimer = setTimeout(async () => {
            try {
                await api.hueSetState(socket.code, { bri: value });
            } catch (e) {
                toasts.error("Brightness update failed", (e as Error).message);
            }
        }, 150);
    }

    async function confirmDelete() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete socket?",
            message: `“${socket.name}” and any schedules pointing to it will be removed.`,
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

    function openControls() {
        if (!isHue) return;
        openModal(HueLightModal, { socket });
    }
</script>

<article class="card" class:on={socket.state} class:pulsing class:clickable={isHue}>
    <div class="head">
        {#if isHue}
            <button class="title-btn" onclick={openControls} title="Open lamp controls">
                <div class="title">
                    <div class="name" title={socket.name}>{socket.name}</div>
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
        <span class="code-chip" title="RF code">
            {socket.protocol || "raw"} · {socket.code}
        </span>
    </div>
    {#if isHue}
        <div class="bri-row" class:disabled={!socket.state}>
            <Icon name="sun" size={14} />
            <input type="range" min="1" max="254" step="1"
                bind:value={hueBri}
                oninput={onBriInput}
                disabled={!socket.state || hueBri === null}
                aria-label="Brightness" />
            <span class="bri-val">{hueBri == null ? "—" : `${Math.round((hueBri / 254) * 100)}%`}</span>
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

    /* State-change pulse — an expanding ring that fades out. */
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
    .name { font-weight: 600; font-size: 1rem; line-height: 1.3; }
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
    .card.on .dot {
        background: var(--success);
        box-shadow: 0 0 0 4px var(--success-soft);
    }
    .card.on .state { color: var(--success); font-weight: 600; }

    .bri-row {
        display: flex;
        align-items: center;
        gap: var(--space-2);
        color: var(--text-muted);
        font-size: 12px;
    }
    .bri-row.disabled { opacity: 0.45; }
    .bri-row input[type="range"] {
        flex: 1;
        accent-color: var(--accent, #60a5fa);
    }
    .bri-val {
        font-variant-numeric: tabular-nums;
        min-width: 36px;
        text-align: right;
    }

    .controls {
        display: grid;
        grid-template-columns: 1fr 1fr 1fr;
        gap: var(--space-2);
    }
</style>
