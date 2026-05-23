<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import ScheduleRow from "../components/ScheduleRow.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { openModal } from "../lib/modal.svelte";
    import ScheduleModal from "../modals/ScheduleModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";
    import { formatDays } from "../lib/utils";
    import type { Schedule } from "../lib/types";

    const v = $derived(data.value);
    const anyEnabled = $derived(v.schedules.some(s => s.enabled));
    let pausing = $state(false);

    // ── Today's timeline ────────────────────────────────────────────────────
    // Plot each enabled schedule that fires today on a 24h day/night rail.
    const now = new Date();
    const todayIdx = now.getDay();                 // 0=Sun … matches schedule.days
    const nowMin = now.getHours() * 60 + now.getMinutes();

    // Resolve a schedule to its clock minute-of-day. Solar schedules carry a
    // server-computed effective_time once resolved; skip ones we can't place.
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
        return "var(--cool)"; // toggle / activate
    }

    function targetLabel(s: Schedule): string {
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
            .sort((a, b) => a.min - b.min)
    );
    const upcoming = $derived(todayEvents.filter(e => e.min >= nowMin));
    const nextEvent = $derived(upcoming[0] ?? null);

    // "Vacation mode": flip every schedule off (or back on) in one call.
    async function toggleAll() {
        if (pausing) return;
        pausing = true;
        const enable = !anyEnabled;
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
</script>

<Topbar title="Schedules" subtitle="{v.schedules.length} configured">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(ScheduleModal, {})}>Add schedule</button>
    {/snippet}
</Topbar>

{#if v.schedules.length === 0}
    <EmptyState icon="clock" title="No schedules yet"
        message="Schedule your sockets, groups or scenes to fire automatically.">
        <button class="btn btn-primary" onclick={() => openModal(ScheduleModal, {})}>Add schedule</button>
    </EmptyState>
{:else}
    <!-- ── Today's 24h timeline ─────────────────────────────────────────── -->
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
                    title="{hhmm(e.min)} · {targetLabel(e.s)} · {e.s.action} · {formatDays(e.s.days)}">
                </div>
            {/each}
            {#if nowMin >= 0}
                <div class="tl-now" style="left: {(nowMin / 1440) * 100}%">
                    <span class="tl-now-dot"></span>
                </div>
            {/if}
        </div>

        <div class="tl-hours mono">
            <span>00</span><span>06</span><span>12</span><span>18</span><span>24</span>
        </div>
    </div>

    <!-- Controls + list share a wrapper so the gap between them stays tight
         while the view's normal gap separates this block from the topbar. -->
    <div class="schedule-section">
        <div class="section-controls">
            <h2 class="section-head">Automations</h2>
            <button class="btn btn-ghost" onclick={toggleAll} disabled={pausing}>
                {anyEnabled ? "Pause all" : "Resume all"}
            </button>
        </div>
        <div class="list">
            {#each v.schedules as s, i (s.id)}
                <div
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:fly={{ y: 12, duration: dur(240), delay: stagger(i), easing: cubicOut }}
                    out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                    <ScheduleRow schedule={s} />
                </div>
            {/each}
        </div>
    </div>
{/if}

<style>
    /* ── Today's timeline card ──────────────────────────────── */
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
        /* night → dawn → midday → dusk → night */
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

    .schedule-section { display: flex; flex-direction: column; gap: var(--space-3); }
    .section-controls {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .section-head {
        margin: 0;
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.01em;
        color: var(--text);
    }
    .list { display: flex; flex-direction: column; gap: var(--space-2); }
</style>
