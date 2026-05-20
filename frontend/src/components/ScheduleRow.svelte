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
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";

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

    // ── Mobile overflow menu ──────────────────────────────────────
    let moreOpen = $state(false);
    let rowEl = $state<HTMLElement>();

    $effect(() => {
        if (!moreOpen) return;
        function onDocClick(e: MouseEvent) {
            if (!rowEl?.contains(e.target as Node)) moreOpen = false;
        }
        document.addEventListener("click", onDocClick, true);
        return () => document.removeEventListener("click", onDocClick, true);
    });
</script>

<div class="row" bind:this={rowEl}>

    <!--
      Desktop: 5-column single row
        [time] [info] [action] [switch] [edit/del]

      Mobile (≤700 px): 2-row compact card
        Row 1:  target name      ·  switch  ·  ⋯
        Row 2:  time + meta      ·  action badge
    -->

    <!-- ── Time (desktop col 1 / mobile row-2 left) ── -->
    <div class="time" class:solar={isSolar}>
        {#if isSolar}
            <span class="solar-icon" aria-hidden="true">
                <Icon name={schedule.time_mode === "sunrise" ? "sunrise" : "sunset"} size={18} />
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
        <!-- Mobile: days + fired age run inline with the time -->
        <span class="mobile-meta">
            · {formatDays(schedule.days)}{#if schedule.last_fired_at} · {formatAgo(schedule.last_fired_at)}{/if}
        </span>
    </div>

    <!-- ── Info (desktop col 2 / mobile row-1) ── -->
    <div class="info">
        <div class="target">{target.kind}: {target.label}</div>
        <!-- Desktop only: full meta below the target -->
        <div class="meta desktop-meta">
            {target.sub} · {formatDays(schedule.days)}
            {#if schedule.last_fired_at}<span class="last-fired">· fired {formatAgo(schedule.last_fired_at)}</span>{/if}
        </div>
    </div>

    <!-- ── Action badge ── -->
    <span class="action" data-action={schedule.action}>{schedule.action}</span>

    <!-- ── Enable switch ── -->
    <Switch checked={schedule.enabled} onChange={toggleEnabled} ariaLabel="Enable schedule" />

    <!-- ── Desktop: icon buttons ── -->
    <div class="buttons desktop-btns">
        <button class="icon-btn" aria-label="Edit schedule"
            onclick={() => openModal(ScheduleModal, { existing: schedule })}>
            <Icon name="edit" size={16} />
        </button>
        <button class="icon-btn danger" aria-label="Delete schedule" onclick={confirmDelete}>
            <Icon name="trash" size={16} />
        </button>
    </div>

    <!-- ── Mobile: ⋯ overflow menu ── -->
    <div class="mobile-more">
        <button class="icon-btn more-btn" class:open={moreOpen}
            aria-label="More options" aria-expanded={moreOpen}
            onclick={(e) => { e.stopPropagation(); moreOpen = !moreOpen; }}>
            <Icon name="more" size={18} />
        </button>
        {#if moreOpen}
            <div class="overflow-menu" role="menu"
                in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                <button class="overflow-item" role="menuitem"
                    onclick={() => { moreOpen = false; openModal(ScheduleModal, { existing: schedule }); }}>
                    <Icon name="edit" size={16} /><span>Edit</span>
                </button>
                <button class="overflow-item danger" role="menuitem"
                    onclick={() => { moreOpen = false; confirmDelete(); }}>
                    <Icon name="trash" size={16} /><span>Delete</span>
                </button>
            </div>
        {/if}
    </div>

</div>

<style>
    /* ── Card shell ────────────────────────────── */
    .row {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        padding: var(--space-4);
        display: grid;
        grid-template-columns: auto 1fr auto auto auto auto;
        gap: var(--space-4);
        align-items: center;
        position: relative;
    }

    /* ── Time block ────────────────────────────── */
    .time {
        font-family: var(--font-mono);
        font-size: 1.25rem;
        font-weight: 600;
        display: flex;
        align-items: baseline;
        gap: 4px;
        white-space: nowrap;
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
        background: var(--surface);
        border-radius: 4px;
        padding: 1px 4px;
        letter-spacing: 0.02em;
    }

    /* Hide mobile-only extras on desktop */
    .mobile-meta { display: none; }
    .mobile-more  { display: none; }

    /* ── Info block ─────────────────────────────── */
    .info { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
    .target { font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .meta { color: var(--text-muted); font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .last-fired { color: var(--text-faint); }

    /* ── Action badge ──────────────────────────── */
    .action {
        padding: 2px 10px;
        border-radius: 999px;
        font-size: 12px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        white-space: nowrap;
    }
    .action[data-action="on"]     { background: var(--success-soft); color: var(--success); }
    .action[data-action="off"]    { background: var(--danger-soft);  color: var(--danger);  }
    .action[data-action="toggle"],
    .action[data-action="activate"] { background: var(--info-soft); color: var(--info); }

    /* ── Desktop buttons ───────────────────────── */
    .buttons { display: flex; gap: 4px; }

    /* ── Overflow menu ─────────────────────────── */
    .more-btn.open { background: var(--surface-hover); color: var(--text); }
    .overflow-menu {
        position: absolute;
        top: calc(100% + 4px);
        right: 0;
        z-index: 50;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        box-shadow: var(--shadow-md);
        min-width: 140px;
        overflow: hidden;
    }
    .overflow-item {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 14px var(--space-4);
        background: transparent;
        border: none;
        border-bottom: 1px solid var(--border);
        cursor: pointer;
        font: inherit;
        font-size: 15px;
        color: var(--text);
        text-align: left;
        width: 100%;
        min-height: 52px;
        touch-action: manipulation;
        transition: background var(--t-fast);
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:active { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    /* ══ Mobile: 2-row compact card ══════════════
       Row 1:  [target name]          [switch] [⋯]
       Row 2:  [time · days · fired]  [action]
    */
    @media (max-width: 700px) {
        .row {
            grid-template-columns: 1fr auto auto;
            grid-template-rows: auto auto;
            grid-template-areas:
                "info   switch more"
                "time   action action";
            column-gap: var(--space-3);
            row-gap: var(--space-2);
            padding: var(--space-3) var(--space-4);
        }

        .info    { grid-area: info; }
        .time    { grid-area: time; align-self: center; }
        .action  { grid-area: action; justify-self: end; align-self: center; }
        .row > :global(label) { grid-area: switch; align-self: center; }
        .mobile-more  { display: block; grid-area: more; align-self: center; position: relative; }
        .desktop-btns { display: none; }
        .desktop-meta { display: none; }

        /* Time shrinks and shows the inline meta (days + fired) */
        .time {
            font-size: 0.875rem;
            font-weight: 600;
            flex-wrap: wrap;
            white-space: normal;
            gap: 0 4px;
        }
        .time.solar { font-size: 0.875rem; }
        .mobile-meta {
            display: inline;
            font-family: var(--font-sans);
            font-size: 0.8rem;
            font-weight: 400;
            color: var(--text-muted);
        }
    }
</style>
