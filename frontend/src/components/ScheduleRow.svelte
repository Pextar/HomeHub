<script lang="ts">
    import Icon from "./Icon.svelte";
    import Switch from "./Switch.svelte";
    import { api } from "../lib/api";
    import { describeTarget, formatDays, formatAgo } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import { toasts, data } from "../lib/stores.svelte";
    import type { Schedule } from "../lib/types";
    import ScheduleModal from "../modals/ScheduleModal.svelte";
    import ConfirmModal from "./ConfirmModal.svelte";

    interface Props { schedule: Schedule; }
    let { schedule }: Props = $props();

    const target = $derived(describeTarget(schedule.target_type, schedule.target_id, schedule.socket_id));
    const isSolar = $derived(schedule.time_mode === "sunrise" || schedule.time_mode === "sunset");

    function formatOffset(min: number): string {
        if (!min) return "";
        const sign = min < 0 ? "−" : "+";
        const abs = Math.abs(min);
        const h = Math.floor(abs / 60);
        const m = abs % 60;
        const parts = [h && `${h}h`, m && `${m}m`].filter(Boolean).join("");
        return `${sign}${parts}`;
    }

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
        const when = isSolar
            ? `at ${schedule.time_mode}${formatOffset(schedule.solar_offset_minutes ?? 0)}`
            : `at ${schedule.time}`;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete schedule?",
            message: `${schedule.action.toUpperCase()} ${target.label} ${when}.`,
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
    <div class="time" class:solar={isSolar}>
        {#if isSolar}
            <span class="solar-icon" aria-hidden="true">
                <Icon name={schedule.time_mode === "sunrise" ? "sunrise" : "sunset"} size={20} />
            </span>
            <span class="solar-label">
                {schedule.time_mode === "sunrise" ? "Sunrise" : "Sunset"}{#if schedule.solar_offset_minutes}<span class="solar-offset"> {formatOffset(schedule.solar_offset_minutes)}</span>{/if}
            </span>
            {#if schedule.effective_time}
                <span class="solar-time">~{schedule.effective_time}</span>
            {/if}
        {:else}
            {schedule.time}
        {/if}
        {#if schedule.random_offset_minutes}
            <span class="offset">+{schedule.random_offset_minutes < 60 ? `${schedule.random_offset_minutes}m` : `${schedule.random_offset_minutes / 60}h`}</span>
        {/if}
    </div>
    <div class="info">
        <div class="target">{target.kind}: {target.label}</div>
        <div class="meta">
            {target.sub} · {formatDays(schedule.days)}
            {#if schedule.last_fired_at}<span class="last-fired">· fired {formatAgo(schedule.last_fired_at)}</span>{/if}
        </div>
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
        display: flex;
        align-items: baseline;
        gap: 4px;
    }
    .time.solar {
        font-family: inherit;
        font-size: 0.95rem;
        align-items: center;
        gap: 6px;
        color: var(--text);
    }
    .solar-icon { display: inline-flex; color: var(--primary); }
    .solar-label { white-space: nowrap; }
    .solar-offset { color: var(--text-muted); font-weight: 500; }
    .solar-time {
        font-family: var(--font-mono);
        font-size: 0.8rem;
        color: var(--text-muted);
        white-space: nowrap;
    }
    .offset {
        font-size: 0.65rem;
        font-weight: 500;
        color: var(--text-muted);
        background: var(--bg-sunken);
        border-radius: 4px;
        padding: 1px 4px;
        letter-spacing: 0.02em;
    }
    .info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
    .target { font-weight: 500; }
    .meta { color: var(--text-muted); font-size: 12px; }
    .last-fired { color: var(--text-faint); }
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
