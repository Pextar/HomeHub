<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Switch from "../components/Switch.svelte";
    import Icon from "../components/Icon.svelte";
    import ScheduleRow from "../components/ScheduleRow.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { formatDays } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import AutomationModal from "../modals/AutomationModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";
    import type { Automation, Schedule } from "../lib/types";

    const v = $derived(data.value);

    // ── Filter ──────────────────────────────────────────────────────────
    type Filter = "all" | "time" | "sensor" | "device";
    let filter = $state<Filter>("all");
    const FILTERS: { id: Filter; label: string }[] = [
        { id: "all",    label: "All" },
        { id: "time",   label: "Time" },
        { id: "sensor", label: "Sensor" },
        { id: "device", label: "Device" },
    ];

    // Schedules are time-only, so show them under "all" and "time" filters.
    const shownSchedules = $derived(
        filter === "all" || filter === "time" ? v.schedules : [],
    );
    const shownAutomations = $derived(
        filter === "all" ? v.automations : v.automations.filter(a => a.trigger.type === filter),
    );
    const nothingToShow  = $derived(shownSchedules.length === 0 && shownAutomations.length === 0);
    const totalRules     = $derived(v.schedules.length + v.automations.length);
    const enabledCount   = $derived(v.automations.filter(a => a.enabled).length);
    const anySchedEnabled = $derived(v.schedules.some(s => s.enabled));
    let pausing = $state(false);

    async function toggleAll() {
        if (pausing) return;
        pausing = true;
        const enable = !anySchedEnabled;
        try {
            const r = await api.setAllSchedules(enable);
            toasts.success(enable ? "Schedules resumed" : "Schedules paused",
                `${r.changed} schedule${r.changed === 1 ? "" : "s"} ${enable ? "enabled" : "disabled"}.`);
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
        } finally {
            pausing = false;
        }
    }

    // ── Today's 24h timeline (schedules only) ───────────────────────────
    const now = new Date();
    const todayIdx = now.getDay();
    const nowMin = now.getHours() * 60 + now.getMinutes();

    function minutesOf(s: Schedule): number | null {
        const t = (s.effective_time || s.time || "").trim();
        const m = t.match(/^(\d{1,2}):(\d{2})/);
        if (!m) return null;
        const h = +m[1], min = +m[2];
        if (h > 23 || min > 59) return null;
        return h * 60 + min;
    }
    const hhmm = (min: number) =>
        `${String(Math.floor(min / 60)).padStart(2, "0")}:${String(min % 60).padStart(2, "0")}`;

    function eventColor(action: string): string {
        if (action === "on") return "var(--on)";
        if (action === "off") return "var(--text-mute)";
        return "var(--cool)";
    }
    function schedTargetLabel(s: Schedule): string {
        if (s.target_type === "socket" && s.target_id) return v.sockets.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.target_type === "group"  && s.target_id) return v.groups.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.target_type === "scene"  && s.target_id) return v.scenes.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.socket_id) return v.sockets.find(x => x.id === s.socket_id)?.name ?? "Unknown";
        return "Unknown";
    }

    const todayEvents = $derived(
        v.schedules
            .filter(s => s.enabled)
            .filter(s => !s.days || s.days.length === 0 || s.days.includes(todayIdx))
            .map(s => ({ s, min: minutesOf(s) }))
            .filter((e): e is { s: Schedule; min: number } => e.min !== null)
            .sort((a, b) => a.min - b.min),
    );
    const upcoming  = $derived(todayEvents.filter(e => e.min >= nowMin));
    const nextEvent = $derived(upcoming[0] ?? null);

    // ── Automation display helpers ───────────────────────────────────────
    function socketName(id?: string) { return v.sockets.find(s => s.id === id)?.name ?? "device"; }
    function sensorById(id?: string) { return v.sensors.find(s => s.id === id); }
    function targetName(type: string, id: string) {
        if (type === "group") return v.groups.find(g => g.id === id)?.name ?? "group";
        if (type === "scene") return v.scenes.find(s => s.id === id)?.name ?? "scene";
        return socketName(id);
    }

    function whenText(a: Automation): string {
        const t = a.trigger;
        if (t.type === "time") {
            if (t.time_mode === "sunrise" || t.time_mode === "sunset") {
                if (a.effective_trigger_time) return `≈ ${a.effective_trigger_time}`;
                const off = t.solar_offset_minutes ?? 0;
                const suffix = off ? ` ${off < 0 ? "−" : "+"}${Math.abs(off)}m` : "";
                return `${t.time_mode}${suffix}`;
            }
            const d = formatDays(t.days ?? []);
            return `${t.time ?? ""}${d && d !== "Every day" ? ` ${d}` : ""}`;
        }
        if (t.type === "sensor") {
            const s = sensorById(t.sensor_id);
            return `${s?.name ?? "sensor"} ${t.op} ${t.value}${s?.unit ?? ""}`;
        }
        return `${socketName(t.socket_id)} turns ${t.to_state}`;
    }

    function thenText(a: Automation): string {
        if (a.actions.length === 0) return "—";
        const first = a.actions[0];
        const label = `${targetName(first.target_type, first.target_id)} ${first.action}`;
        return a.actions.length > 1 ? `${label} +${a.actions.length - 1}` : label;
    }

    function lastFiredText(a: Automation): string {
        if (!a.last_fired_at) return "";
        const diff = Math.max(0, Date.now() - new Date(a.last_fired_at).getTime());
        const h = Math.round(diff / 3.6e6);
        if (h < 1) return "last <1h ago";
        if (h < 24) return `last ${h}h ago`;
        return `last ${Math.round(h / 24)}d ago`;
    }

    async function toggleAuto(a: Automation, on: boolean) {
        try {
            await api.updateAutomation(a.id, { ...a, enabled: on });
            await data.refresh();
        } catch (e) {
            toasts.error("Failed", (e as Error).message);
            await data.refresh();
        }
    }
    async function runNow(a: Automation) {
        openId = null;
        try {
            await api.runAutomation(a.id);
            toasts.success("Automation ran", a.name);
            await data.refresh();
        } catch (e) { toasts.error("Run failed", (e as Error).message); }
    }
    async function confirmDelete(a: Automation) {
        openId = null;
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Delete automation?",
            message: `"${a.name}" will be removed.`,
            confirmLabel: "Delete",
            danger: true,
        });
        if (!ok) return;
        try {
            await api.deleteAutomation(a.id);
            toasts.success("Automation deleted", a.name);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }

    let openId = $state<string | null>(null);
    let listEl = $state<HTMLElement>();
    $effect(() => {
        if (openId === null) return;
        function onDoc(e: MouseEvent) { if (!listEl?.contains(e.target as Node)) openId = null; }
        function onKey(e: KeyboardEvent) { if (e.key === "Escape") openId = null; }
        document.addEventListener("click", onDoc, true);
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("click", onDoc, true);
            document.removeEventListener("keydown", onKey, true);
        };
    });
</script>

<Topbar title="Automations" subtitle="{totalRules} rule{totalRules === 1 ? '' : 's'} · {enabledCount} active">
    {#snippet actions()}
        {#if v.schedules.length > 0}
            <button class="btn btn-ghost pause-btn" onclick={toggleAll} disabled={pausing}>
                {anySchedEnabled ? "Pause schedules" : "Resume schedules"}
            </button>
        {/if}
        <button class="add-btn" aria-label="New automation" onclick={() => openModal(AutomationModal, {})}>
            <Icon name="plus" size={16} />
        </button>
    {/snippet}
</Topbar>

{#if totalRules === 0}
    <EmptyState icon="automation" title="No automations yet"
        message="Automations react to time, a sensor crossing a threshold, or a device turning on/off — then run actions.">
        <button class="btn btn-primary" onclick={() => openModal(AutomationModal, {})}>New automation</button>
    </EmptyState>
{:else}
    <!-- ── 24h timeline (schedule events only) ────────────────── -->
    {#if v.schedules.length > 0}
        <div class="card timeline-card">
            <div class="tl-head">
                <div>
                    <div class="tl-eyebrow mono">Today</div>
                    <div class="tl-title">
                        {upcoming.length} event{upcoming.length === 1 ? "" : "s"} ahead
                    </div>
                </div>
                {#if nextEvent}
                    <div class="tl-next">
                        Next: <span class="mono tl-next-time">{hhmm(nextEvent.min)}</span>
                    </div>
                {/if}
            </div>

            <div class="tl-rail">
                <div class="tl-grad"></div>
                {#each todayEvents as e (e.s.id)}
                    <div class="tl-mark" class:past={e.min < nowMin}
                        style="left: {(e.min / 1440) * 100}%; background: {eventColor(e.s.action)};"
                        title="{hhmm(e.min)} · {schedTargetLabel(e.s)} · {e.s.action} · {formatDays(e.s.days)}">
                    </div>
                {/each}
                <div class="tl-now" style="left: {(nowMin / 1440) * 100}%">
                    <span class="tl-now-dot"></span>
                </div>
            </div>

            <div class="tl-hours mono">
                <span>00</span><span>06</span><span>12</span><span>18</span><span>24</span>
            </div>
        </div>
    {/if}

    <!-- ── Filter chips ───────────────────────────────────────── -->
    <div class="filters h-scroll">
        {#each FILTERS as f}
            <button class="chip" class:active={filter === f.id} onclick={() => filter = f.id}>{f.label}</button>
        {/each}
    </div>

    {#if nothingToShow}
        <p class="field-help">No items match this filter.</p>
    {:else}
        <div class="list" bind:this={listEl}>

            <!-- ── Schedule rows ──────────────────────────────── -->
            {#if shownSchedules.length > 0}
                <div class="section-label">Schedules</div>
                {#each shownSchedules as s, i (s.id)}
                    <div
                        animate:flip={{ duration: dur(280), easing: cubicOut }}
                        in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                        out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                        <ScheduleRow schedule={s} />
                    </div>
                {/each}
            {/if}

            <!-- ── Automation cards ────────────────────────────── -->
            {#if shownAutomations.length > 0}
                {#if shownSchedules.length > 0}
                    <div class="section-label">Automations</div>
                {/if}
                {#each shownAutomations as a, i (a.id)}
                    <div class="auto" class:disabled={!a.enabled}
                        animate:flip={{ duration: dur(280), easing: cubicOut }}
                        in:fly={{ y: 12, duration: dur(220), delay: stagger(shownSchedules.length + i), easing: cubicOut }}
                        out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                        <button class="hit" onclick={() => openModal(AutomationModal, { existing: a })} aria-label="Edit {a.name}">
                            <span class="name-row">
                                <span class="dot" class:on={a.enabled}></span>
                                <span class="name">{a.name}</span>
                            </span>
                            <span class="rule mono">
                                <span class="kw when">WHEN</span>
                                <span class="val">{whenText(a)}</span>
                                <Icon name="chevronDown" size={12} />
                                <span class="kw then">THEN</span>
                                <span class="val">{thenText(a)}</span>
                            </span>
                            {#if (a.run_count ?? 0) > 0}
                                <span class="runs mono" title={a.last_fired_at ? new Date(a.last_fired_at).toLocaleString() : undefined}>ran {a.run_count}× · {lastFiredText(a)}</span>
                            {/if}
                        </button>

                        <div class="right">
                            <Switch checked={a.enabled} onChange={(c) => toggleAuto(a, c)} ariaLabel="Enable {a.name}" />
                            <button class="more-btn" aria-label="Automation actions"
                                onclick={(e) => { e.stopPropagation(); openId = openId === a.id ? null : a.id; }}>
                                <Icon name="more" size={16} />
                            </button>
                        </div>

                        {#if openId === a.id}
                            <div class="overflow-menu" role="menu"
                                in:scale={{ start: 0.95, duration: 140, easing: cubicOut, opacity: 0 }}
                                out:scale={{ start: 0.95, duration: 100, easing: cubicOut, opacity: 0 }}>
                                <button class="overflow-item" role="menuitem" onclick={() => runNow(a)}>
                                    <Icon name="power" size={16} /><span>Run now</span>
                                </button>
                                <button class="overflow-item" role="menuitem" onclick={() => { openId = null; openModal(AutomationModal, { existing: a }); }}>
                                    <Icon name="edit" size={16} /><span>Edit</span>
                                </button>
                                <button class="overflow-item danger" role="menuitem" onclick={() => confirmDelete(a)}>
                                    <Icon name="trash" size={16} /><span>Delete</span>
                                </button>
                            </div>
                        {/if}
                    </div>
                {/each}
            {/if}

        </div>
    {/if}
{/if}

<style>
    /* ── Topbar add button ──────────────────────────────────── */
    .pause-btn { font-size: 13px; }
    .add-btn {
        width: 38px; height: 38px;
        display: grid; place-items: center;
        border-radius: 50%;
        background: var(--on);
        color: #1a1813;
        border: 1px solid var(--on);
        cursor: pointer;
        touch-action: manipulation;
        transition: transform var(--t-fast), box-shadow var(--t-fast);
    }
    .add-btn:hover { box-shadow: 0 4px 14px var(--on-glow); }
    .add-btn:active { transform: scale(0.94); }

    /* ── 24h timeline card ──────────────────────────────────── */
    .timeline-card { padding: 18px; gap: 0; }
    .tl-head {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--space-3);
        margin-bottom: 14px;
    }
    .tl-eyebrow {
        color: var(--text-mute);
        font-size: 11.5px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
    }
    .tl-title { font-size: 16px; font-weight: 600; margin-top: 2px; }
    .tl-next { color: var(--text-mute); font-size: 12px; }
    .tl-next-time { color: var(--on); }

    .tl-rail { position: relative; height: 60px; }
    .tl-grad {
        position: absolute; inset: 0;
        border-radius: 12px;
        background: linear-gradient(90deg,
            #1a1d28 0%, #1a1d28 22%, #2a2618 28%, #3a2e1e 50%,
            #2a2618 72%, #1a1d28 78%, #1a1d28 100%);
    }
    :global([data-theme="light"]) .tl-grad {
        background: linear-gradient(90deg,
            #d9deec 0%, #d9deec 22%, #f0e4c4 28%, #ffe6b8 50%,
            #f0e4c4 72%, #d9deec 78%, #d9deec 100%);
    }
    .tl-mark {
        position: absolute;
        top: 20px;
        width: 10px; height: 20px;
        border-radius: 3px;
        transform: translateX(-50%);
        box-shadow: 0 1px 3px rgba(0,0,0,0.35);
    }
    .tl-mark.past { opacity: 0.4; }
    .tl-now {
        position: absolute;
        top: -6px; bottom: -6px;
        width: 2px;
        background: var(--text);
        border-radius: 1px;
        transform: translateX(-50%);
    }
    .tl-now-dot {
        position: absolute;
        top: -8px; left: 50%;
        width: 8px; height: 8px;
        border-radius: 50%;
        background: var(--text);
        transform: translateX(-50%);
    }
    .tl-hours {
        display: flex;
        justify-content: space-between;
        margin-top: 8px;
        color: var(--text-dim);
        font-size: 10px;
    }

    /* ── Filter chips ───────────────────────────────────────── */
    .filters { gap: 6px; padding-bottom: 2px; }

    /* ── List ───────────────────────────────────────────────── */
    .list { display: flex; flex-direction: column; gap: 10px; }

    .section-label {
        font-size: 11px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.09em;
        color: var(--text-mute);
        padding: 4px 2px 0;
    }

    /* ── Automation card ────────────────────────────────────── */
    .auto {
        position: relative;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        padding: 14px;
        display: flex;
        align-items: flex-start;
        gap: 12px;
        transition: opacity var(--t-fast), border-color var(--t-fast);
    }
    .auto.disabled { opacity: 0.6; }
    @media (hover: hover) { .auto:hover { border-color: var(--border-strong); } }

    .hit {
        all: unset;
        flex: 1;
        min-width: 0;
        cursor: pointer;
        touch-action: manipulation;
        display: flex;
        flex-direction: column;
        gap: 8px;
    }
    .hit:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }

    .name-row { display: flex; align-items: center; gap: 8px; }
    .dot { width: 6px; height: 6px; border-radius: 50%; background: var(--text-dim); }
    .dot.on { background: var(--on); box-shadow: 0 0 0 4px var(--on-soft); }
    .name { font-weight: 600; font-size: 14.5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

    .rule { display: flex; align-items: center; gap: 8px; font-size: 11px; color: var(--text-mute); flex-wrap: wrap; }
    .kw { text-transform: uppercase; letter-spacing: 0.06em; }
    .kw.when { color: var(--cool); }
    .kw.then { color: var(--on); }
    .rule .val { color: var(--text); }
    .rule :global(svg) { color: var(--text-dim); transform: rotate(-90deg); }

    .runs { font-size: 11.5px; color: var(--text-dim); }

    .right { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
    .more-btn {
        width: 28px; height: 28px;
        display: grid; place-items: center;
        border: 0; background: transparent;
        color: var(--text-mute); border-radius: var(--r-sm);
        cursor: pointer;
    }
    .more-btn:hover { background: var(--surface-hover); color: var(--text); }

    .overflow-menu {
        position: absolute;
        right: 12px; top: 48px;
        z-index: 10;
        min-width: 170px;
        display: flex; flex-direction: column;
        background: var(--bg-raised);
        border: 1px solid var(--border);
        border-radius: var(--radius-md);
        overflow: hidden;
        box-shadow: var(--shadow-md);
    }
    .overflow-item {
        display: flex; align-items: center; gap: var(--space-3);
        padding: 12px var(--space-4);
        background: transparent; border: 0;
        border-bottom: 1px solid var(--border);
        cursor: pointer; font: inherit; font-size: 14px;
        color: var(--text); text-align: left;
    }
    .overflow-item:last-child { border-bottom: none; }
    .overflow-item :global(svg) { color: var(--text-muted); flex-shrink: 0; }
    .overflow-item:hover { background: var(--surface-hover); }
    .overflow-item.danger { color: var(--danger); }
    .overflow-item.danger :global(svg) { color: var(--danger); }

    @media (pointer: coarse) {
        .overflow-item { padding: 14px var(--space-4); font-size: 15px; min-height: 52px; }
        .more-btn { width: 44px; height: 44px; }
    }
</style>
