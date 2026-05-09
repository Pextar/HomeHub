<script lang="ts">
    import Icon from "./Icon.svelte";
    import { api } from "../lib/api";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { toasts, data } from "../lib/stores";
    import type { Socket } from "../lib/types";
    import SocketModal from "../modals/SocketModal.svelte";
    import TimerModal from "../modals/TimerModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { socket: Socket; }
    let { socket }: Props = $props();

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
</script>

<article class="card" class:on={socket.state}>
    <div class="head">
        <div class="title">
            <div class="name" title={socket.name}>{socket.name}</div>
            <div class="meta">{socket.room || "Unassigned"}</div>
        </div>
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
        transition: border-color var(--t-fast);
    }
    .card:hover { border-color: var(--border-strong); }
    .card.on { border-color: var(--success); box-shadow: inset 0 0 0 1px var(--success-soft); }

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

    .controls {
        display: grid;
        grid-template-columns: 1fr 1fr 1fr;
        gap: var(--space-2);
    }
</style>
