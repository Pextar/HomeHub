<script lang="ts">
    import Icon from "./Icon.svelte";
    import Switch from "./Switch.svelte";
    import { api } from "../lib/api";
    import { describeTarget, formatDays } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { toasts, data } from "../lib/stores";
    import type { Schedule } from "../lib/types";
    import ScheduleModal from "../modals/ScheduleModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { schedule: Schedule; }
    let { schedule }: Props = $props();

    const target = $derived(describeTarget(schedule.target_type, schedule.target_id, schedule.socket_id));

    async function toggleEnabled(checked: boolean) {
        try {
            await api.updateSchedule(schedule.id, { ...schedule, enabled: checked });
            toasts.success(checked ? "Schedule enabled" : "Schedule disabled");
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
            await data.refresh();
        }
    }

    async function confirmDelete() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete schedule?",
            message: `${schedule.action.toUpperCase()} ${target.label} at ${schedule.time}.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSchedule(schedule.id);
            toasts.success("Schedule deleted");
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        }
    }
</script>

<div class="row">
    <div class="time">{schedule.time}</div>
    <div class="info">
        <div class="target">{target.kind}: {target.label}</div>
        <div class="meta">{target.sub} · {formatDays(schedule.days)}</div>
    </div>
    <span class="action" data-action={schedule.action}>{schedule.action}</span>
    <Switch checked={schedule.enabled} onChange={toggleEnabled} ariaLabel="Enable schedule" />
    <div class="buttons">
        <button class="icon-btn" aria-label="Edit schedule"
            onclick={() => openModal(ScheduleModal, { existing: schedule })}>
            <Icon name="edit" size={16} />
        </button>
        <button class="icon-btn danger" aria-label="Delete schedule" onclick={confirmDelete}>
            <Icon name="trash" size={16} />
        </button>
    </div>
</div>

<style>
    .row {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-4);
        display: grid;
        grid-template-columns: auto 1fr auto auto auto;
        gap: var(--space-4);
        align-items: center;
    }
    .time {
        font-family: var(--font-mono);
        font-size: 1.25rem;
        font-weight: 600;
    }
    .info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
    .target { font-weight: 500; }
    .meta { color: var(--text-muted); font-size: 12px; }
    .action {
        padding: 2px 10px;
        border-radius: 999px;
        font-size: 12px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.04em;
    }
    .action[data-action="on"] { background: var(--success-soft); color: var(--success); }
    .action[data-action="off"] { background: var(--danger-soft); color: var(--danger); }
    .action[data-action="toggle"], .action[data-action="activate"] {
        background: var(--info-soft); color: var(--info);
    }
    .buttons { display: flex; gap: 4px; }

    @media (max-width: 700px) {
        .row {
            grid-template-columns: auto 1fr auto;
            grid-template-areas:
                "time info buttons"
                "switch action action";
            row-gap: var(--space-2);
        }
        .time { grid-area: time; }
        .info { grid-area: info; }
        .buttons { grid-area: buttons; justify-self: end; }
        .action { grid-area: action; justify-self: end; }
        .row > :global(label) { grid-area: switch; }
    }
</style>
