<script lang="ts">
    import Icon from "./Icon.svelte";
    import Switch from "./Switch.svelte";
    import { api } from "../lib/api";
    import { describeTarget, formatDays } from "../lib/utils";
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

    // Icon reflects what the schedule does, matching the mockup's glyphs.
    const icon = $derived(
        schedule.time_mode === "sunrise" ? "sunrise" :
        schedule.time_mode === "sunset"  ? "sunset" :
        schedule.action === "off"        ? "moon" :
        schedule.action === "activate"   ? "scenes" :
        schedule.action === "toggle"     ? "power" : "light"
    );
    const verb = $derived(
        schedule.action === "on" ? "Turn on" :
        schedule.action === "off" ? "Turn off" :
        schedule.action === "toggle" ? "Toggle" : "Run"
    );

    function formatOffset(min: number): string {
        if (!min) return "";
        const sign = min < 0 ? "−" : "+";
        const abs = Math.abs(min);
        const h = Math.floor(abs / 60);
        const m = abs % 60;
        return `${sign}${[h && `${h}h`, m && `${m}m`].filter(Boolean).join("")}`;
    }

    const timeText = $derived(
        isSolar
            ? (schedule.effective_time ? `≈ ${schedule.effective_time}` : (schedule.time_mode === "sunrise" ? "Sunrise" : "Sunset"))
            : schedule.time
    );

    async function toggleEnabled(checked: boolean) {
        try {
            await api.updateSchedule(schedule.id, { ...schedule, enabled: checked });
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
            await data.refresh();
        }
    }

    function openEdit() { moreOpen = false; openModal(ScheduleModal, { existing: schedule }); }
    async function confirmDelete() {
        moreOpen = false;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete schedule?",
            message: `${verb} ${target.label} (${timeText}).`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteSchedule(schedule.id);
            toasts.success("Schedule deleted");
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    let moreOpen = $state(false);
    let rowEl = $state<HTMLElement>();
    $effect(() => {
        if (!moreOpen) return;
        function onDoc(e: MouseEvent) { if (!rowEl?.contains(e.target as Node)) moreOpen = false; }
        function onKey(e: KeyboardEvent) { if (e.key === "Escape") moreOpen = false; }
        document.addEventListener("click", onDoc, true);
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("click", onDoc, true);
            document.removeEventListener("keydown", onKey, true);
        };
    });
</script>

<div class="sched" class:disabled={!schedule.enabled} bind:this={rowEl}>
    <button class="hit" onclick={openEdit} aria-label="Edit schedule for {target.label}">
        <span class="ico" class:on={schedule.enabled}>
            <Icon name={icon} size={18} />
        </span>
        <span class="main">
            <span class="line1">
                <span class="name">{target.label}</span>
                <span class="time mono" class:lit={schedule.enabled}>{timeText}{#if schedule.random_offset_minutes}<span class="rnd">+{schedule.random_offset_minutes < 60 ? `${schedule.random_offset_minutes}m` : `${schedule.random_offset_minutes / 60}h`}</span>{/if}</span>
            </span>
            <span class="sub">
                <span>{verb}{#if isSolar && schedule.solar_offset_minutes} {formatOffset(schedule.solar_offset_minutes)}{/if}</span>
                <span class="dotsep">·</span>
                <span>{formatDays(schedule.days)}</span>
            </span>
        </span>
    </button>

    <Switch checked={schedule.enabled} onChange={toggleEnabled} ariaLabel="Enable schedule" />

    <div class="more-wrap">
        <button class="icon-btn more-btn" class:open={moreOpen}
            aria-label="More options" aria-expanded={moreOpen}
            onclick={(e) => { e.stopPropagation(); moreOpen = !moreOpen; }}>
            <Icon name="more" size={18} />
        </button>
        {#if moreOpen}
            <div class="overflow-menu" role="menu"
                in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                <button class="overflow-item" role="menuitem" onclick={openEdit}>
                    <Icon name="edit" size={16} /><span>Edit</span>
                </button>
                <button class="overflow-item danger" role="menuitem" onclick={confirmDelete}>
                    <Icon name="trash" size={16} /><span>Delete</span>
                </button>
            </div>
        {/if}
    </div>
</div>

<style>
    .sched {
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: 14px;
        display: flex;
        align-items: center;
        gap: 14px;
        position: relative;
        transition: opacity var(--t-fast), border-color var(--t-fast);
    }
    .sched.disabled { opacity: 0.55; }
    @media (hover: hover) { .sched:hover { border-color: var(--border-strong); } }

    .hit {
        all: unset;
        flex: 1;
        min-width: 0;
        display: flex;
        align-items: center;
        gap: 14px;
        cursor: pointer;
        touch-action: manipulation;
    }
    .hit:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }

    .ico {
        width: 40px; height: 40px;
        border-radius: 12px;
        background: var(--card-3);
        color: var(--text-mute);
        display: grid; place-items: center;
        flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .ico.on { background: var(--on-soft); color: var(--on); }

    .main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
    .line1 { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; }
    .name { font-weight: 600; font-size: 14.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .time { font-size: 13px; color: var(--text-mute); white-space: nowrap; flex-shrink: 0; }
    .time.lit { color: var(--on); }
    .rnd { color: var(--text-dim); margin-left: 2px; }
    .sub {
        color: var(--text-mute); font-size: 12px; margin-top: 2px;
        display: flex; gap: 6px; min-width: 0;
    }
    .sub > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .dotsep { color: var(--text-dim); flex-shrink: 0; }

    .more-wrap { position: relative; flex-shrink: 0; }
    .more-btn.open { background: var(--surface-hover); color: var(--text); }
    .overflow-menu {
        position: absolute;
        top: calc(100% + 4px);
        right: 0;
        z-index: 50;
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        box-shadow: var(--shadow-md);
        min-width: 150px;
        overflow: hidden;
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: none;
        border-bottom: 1px solid var(--border);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left; width: 100%;
        touch-action: manipulation;
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:hover { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    @media (pointer: coarse) {
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
    }
</style>
