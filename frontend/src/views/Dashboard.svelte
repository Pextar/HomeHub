<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import RoomCard from "../components/RoomCard.svelte";
    import SceneTile from "../components/SceneTile.svelte";
    import SensorCard from "../components/SensorCard.svelte";
    import TimerRow from "../components/TimerRow.svelte";
    import { route } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores.svelte";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import SocketModal from "../modals/SocketModal.svelte";
    import SceneModal from "../modals/SceneModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import ShortcutsModal from "../modals/ShortcutsModal.svelte";
    import type { Schedule } from "../lib/types";
    import { Tween } from "svelte/motion";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);
    const totalSockets    = $derived(v.sockets.length);
    const onSockets       = $derived(v.sockets.filter(s => s.state).length);
    const enabledSchedules = $derived(v.schedules.filter(s => s.enabled).length);
    const groupsAndScenes = $derived(v.groups.length + v.scenes.length);

    // Count-up animation for the four stat numbers.
    const totalT = new Tween(0, { duration: dur(700), easing: cubicOut });
    const onT    = new Tween(0, { duration: dur(700), easing: cubicOut });
    const schedT = new Tween(0, { duration: dur(700), easing: cubicOut });
    const gsT    = new Tween(0, { duration: dur(700), easing: cubicOut });
    $effect(() => { totalT.target = totalSockets; });
    $effect(() => { onT.target = onSockets; });
    $effect(() => { schedT.target = enabledSchedules; });
    $effect(() => { gsT.target = groupsAndScenes; });

    // ── Stat detail panel ──────────────────────────────────────────────────
    type StatKey = "devices" | "active" | "schedules" | "automations";
    let selectedStat = $state<StatKey | null>(null);

    function toggleStat(key: StatKey) {
        selectedStat = selectedStat === key ? null : key;
    }

    function goAndClose(r: Parameters<typeof route.go>[0]) {
        selectedStat = null;
        route.go(r);
    }

    // Devices panel data
    const rfCount     = $derived(v.sockets.filter(s => s.protocol !== "tasmota" && s.protocol !== "matter").length);
    const wifiCount   = $derived(v.sockets.filter(s => s.protocol === "tasmota").length);
    const matterCount = $derived(v.sockets.filter(s => s.protocol === "matter").length);

    const devicesByRoom = $derived.by(() => {
        const map = new Map<string, { total: number; on: number }>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            const cur = map.get(r) ?? { total: 0, on: 0 };
            map.set(r, { total: cur.total + 1, on: cur.on + (s.state ? 1 : 0) });
        }
        return [...map.entries()].sort((a, b) => b[1].total - a[1].total);
    });

    // Active now panel data
    const activeDevices = $derived(
        v.sockets.filter(s => s.state).sort((a, b) => (a.room ?? "").localeCompare(b.room ?? ""))
    );

    // Schedules panel data
    const enabledSchedulesList = $derived(v.schedules.filter(s => s.enabled));

    const DAY_NAMES = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

    function formatDays(days: number[]): string {
        if (!days?.length) return "–";
        if (days.length === 7) return "Every day";
        if (days.length === 5 && !days.includes(0) && !days.includes(6)) return "Weekdays";
        if (days.length === 2 && days.includes(0) && days.includes(6)) return "Weekends";
        return [...days].sort((a, b) => a - b).map(d => DAY_NAMES[d] ?? "?").join(" · ");
    }

    function formatScheduleTime(s: Schedule): string {
        if (!s.time_mode || s.time_mode === "fixed") return s.time ?? "";
        const offset = s.solar_offset_minutes ?? 0;
        const suffix = offset !== 0 ? ` ${offset > 0 ? "+" : ""}${offset}m` : "";
        return s.time_mode === "sunrise" ? `Sunrise${suffix}` : `Sunset${suffix}`;
    }

    function getTargetLabel(s: Schedule): string {
        if (s.target_type === "socket" && s.target_id)
            return v.sockets.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.target_type === "group" && s.target_id)
            return v.groups.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.target_type === "scene" && s.target_id)
            return v.scenes.find(x => x.id === s.target_id)?.name ?? "Unknown";
        if (s.socket_id)
            return v.sockets.find(x => x.id === s.socket_id)?.name ?? "Unknown";
        return "Unknown";
    }

    // Automations panel data
    const groupsWithState = $derived(
        v.groups.map(g => ({
            ...g,
            on: g.socket_ids.filter(id => v.sockets.find(s => s.id === id)?.state).length,
        }))
    );

    // ── Bulk actions ───────────────────────────────────────────────────────
    async function allOn() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Turn all devices ON?",
            message: `This will switch on ${totalSockets} device${totalSockets === 1 ? "" : "s"}.`,
            confirmLabel: "Turn all on",
        });
        if (!ok) return;
        try {
            const r = await api.allOn();
            toasts.success("All on", `${r.updated} updated, ${r.failures.length} failed.`);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }
    async function allOff() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Turn all devices OFF?",
            message: `This will switch off ${totalSockets} device${totalSockets === 1 ? "" : "s"}.`,
            confirmLabel: "Turn all off",
            danger: true,
        });
        if (!ok) return;
        try {
            const r = await api.allOff();
            toasts.success("All off", `${r.updated} updated, ${r.failures.length} failed.`);
            await data.refresh();
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }
</script>

<Topbar title="Dashboard" subtitle="Your connected home">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add device</button>
    {/snippet}
</Topbar>

<!-- ── Stat cards (clickable) ─────────────────────────────────────── -->
<div class="stats">
    <button class="stat" data-tone="primary"
        class:selected={selectedStat === "devices"}
        aria-expanded={selectedStat === "devices"}
        aria-controls="stat-detail"
        onclick={() => toggleStat("devices")}
        in:fly={{ y: 14, duration: dur(280), delay: stagger(0, 60), easing: cubicOut }}>
        <div class="ico" data-tone="primary"><Icon name="home" size={20} /></div>
        <div class="stat-text">
            <div class="value">{Math.round(totalT.current)}</div>
            <div class="label">Devices</div>
        </div>
        <div class="caret" class:open={selectedStat === "devices"}>
            <Icon name="chevronDown" size={14} />
        </div>
    </button>

    <button class="stat" data-tone="success"
        class:selected={selectedStat === "active"}
        aria-expanded={selectedStat === "active"}
        aria-controls="stat-detail"
        onclick={() => toggleStat("active")}
        in:fly={{ y: 14, duration: dur(280), delay: stagger(1, 60), easing: cubicOut }}>
        <div class="ico" data-tone="success"><Icon name="bolt" size={20} /></div>
        <div class="stat-text">
            <div class="value">{Math.round(onT.current)}</div>
            <div class="label">Active now</div>
        </div>
        <div class="caret" class:open={selectedStat === "active"}>
            <Icon name="chevronDown" size={14} />
        </div>
    </button>

    <button class="stat" data-tone="info"
        class:selected={selectedStat === "schedules"}
        aria-expanded={selectedStat === "schedules"}
        aria-controls="stat-detail"
        onclick={() => toggleStat("schedules")}
        in:fly={{ y: 14, duration: dur(280), delay: stagger(2, 60), easing: cubicOut }}>
        <div class="ico" data-tone="info"><Icon name="clock" size={20} /></div>
        <div class="stat-text">
            <div class="value">{Math.round(schedT.current)}</div>
            <div class="label">Schedules</div>
        </div>
        <div class="caret" class:open={selectedStat === "schedules"}>
            <Icon name="chevronDown" size={14} />
        </div>
    </button>

    <button class="stat" data-tone="warn"
        class:selected={selectedStat === "automations"}
        aria-expanded={selectedStat === "automations"}
        aria-controls="stat-detail"
        onclick={() => toggleStat("automations")}
        in:fly={{ y: 14, duration: dur(280), delay: stagger(3, 60), easing: cubicOut }}>
        <div class="ico" data-tone="warn"><Icon name="scenes" size={20} /></div>
        <div class="stat-text">
            <div class="value">{Math.round(gsT.current)}</div>
            <div class="label">Groups &amp; scenes</div>
        </div>
        <div class="caret" class:open={selectedStat === "automations"}>
            <Icon name="chevronDown" size={14} />
        </div>
    </button>
</div>

<!-- ── Expandable detail panel ────────────────────────────────────── -->
{#if selectedStat}
    <div id="stat-detail" class="detail" data-tone={
            selectedStat === "devices"     ? "primary" :
            selectedStat === "active"      ? "success" :
            selectedStat === "schedules"   ? "info"    : "warn"
        }
        in:fly={{ y: -10, duration: dur(220), easing: cubicOut }}
        out:fly={{ y: -10, duration: dur(160) }}>

        <!-- header -->
        <div class="dh">
            <div class="dh-left">
                {#if selectedStat === "devices"}
                    <div class="dico" data-tone="primary"><Icon name="home" size={14} /></div>
                    <span class="dt">Device breakdown</span>
                {:else if selectedStat === "active"}
                    <div class="dico" data-tone="success"><Icon name="bolt" size={14} /></div>
                    <span class="dt">Active devices</span>
                {:else if selectedStat === "schedules"}
                    <div class="dico" data-tone="info"><Icon name="clock" size={14} /></div>
                    <span class="dt">Active schedules</span>
                {:else}
                    <div class="dico" data-tone="warn"><Icon name="scenes" size={14} /></div>
                    <span class="dt">Groups &amp; scenes</span>
                {/if}
            </div>
            <button class="icon-btn" onclick={() => selectedStat = null} aria-label="Close">
                <Icon name="close" size={16} />
            </button>
        </div>

        <!-- body -->
        <div class="db">

            <!-- DEVICES -->
            {#if selectedStat === "devices"}
                <div class="proto-row">
                    {#if rfCount > 0}
                        <span class="pbadge" data-proto="rf">
                            <Icon name="radio" size={13} /> RF · {rfCount}
                        </span>
                    {/if}
                    {#if wifiCount > 0}
                        <span class="pbadge" data-proto="tasmota">
                            <Icon name="wifi" size={13} /> Wi-Fi · {wifiCount}
                        </span>
                    {/if}
                    {#if matterCount > 0}
                        <span class="pbadge" data-proto="matter">
                            <Icon name="devices" size={13} /> Matter · {matterCount}
                        </span>
                    {/if}
                    {#if totalSockets === 0}
                        <span class="note">No devices added yet</span>
                    {/if}
                </div>

                {#if devicesByRoom.length > 0}
                    <div class="room-grid">
                        {#each devicesByRoom as [room, counts]}
                            <div class="rg-row">
                                <span class="rg-name">{room}</span>
                                <div class="rg-counts">
                                    {#if counts.on > 0}
                                        <span class="rg-on">{counts.on} on</span>
                                    {/if}
                                    <span class="rg-total">{counts.total} device{counts.total === 1 ? "" : "s"}</span>
                                </div>
                            </div>
                        {/each}
                    </div>
                {/if}

                <div class="df">
                    <button class="btn btn-ghost" onclick={() => goAndClose("sockets")}>
                        View all devices →
                    </button>
                </div>

            <!-- ACTIVE NOW -->
            {:else if selectedStat === "active"}
                {#if activeDevices.length === 0}
                    <div class="dempty">
                        <Icon name="moon" size={24} />
                        <p>Everything is off</p>
                    </div>
                {:else}
                    <ul class="dev-list">
                        {#each activeDevices as s (s.id)}
                            <li class="dev-row"
                                animate:flip={{ duration: dur(260), easing: cubicOut }}
                                in:fly={{ x: -6, duration: dur(180), easing: cubicOut }}>
                                <span class="dev-dot"></span>
                                <span class="dev-name">{s.name}</span>
                                {#if s.room}<span class="room-chip">{s.room}</span>{/if}
                                <button class="btn btn-danger btn-xs"
                                    onclick={() => runAction(() => api.socketOff(s.id), `Turned off ${s.name}`)}>
                                    Off
                                </button>
                            </li>
                        {/each}
                    </ul>
                {/if}

                <div class="df">
                    {#if activeDevices.length > 0}
                        <button class="btn btn-danger" onclick={allOff}>Turn all off</button>
                    {/if}
                    <button class="btn btn-ghost" onclick={() => goAndClose("sockets")}>View devices →</button>
                </div>

            <!-- SCHEDULES -->
            {:else if selectedStat === "schedules"}
                {#if enabledSchedulesList.length === 0}
                    <div class="dempty">
                        <Icon name="clock" size={24} />
                        <p>No active schedules</p>
                    </div>
                {:else}
                    <ul class="sched-list">
                        {#each enabledSchedulesList as s (s.id)}
                            <li class="sched-row">
                                <span class="action-pill" data-action={s.action}>{s.action}</span>
                                <span class="sched-target">{getTargetLabel(s)}</span>
                                <span class="sched-time">{formatScheduleTime(s)}</span>
                                <span class="sched-days">{formatDays(s.days)}</span>
                            </li>
                        {/each}
                    </ul>
                {/if}

                <div class="df">
                    <button class="btn btn-ghost" onclick={() => goAndClose("schedules")}>
                        View schedules →
                    </button>
                </div>

            <!-- AUTOMATIONS -->
            {:else if selectedStat === "automations"}
                {#if v.groups.length === 0 && v.scenes.length === 0}
                    <div class="dempty">
                        <Icon name="scenes" size={24} />
                        <p>No groups or scenes yet</p>
                    </div>
                {:else}
                    {#if groupsWithState.length > 0}
                        <div class="auto-section">
                            <div class="auto-label">Groups</div>
                            {#each groupsWithState as g (g.id)}
                                <div class="auto-row">
                                    <div class="auto-info">
                                        <span class="auto-name">{g.name}</span>
                                        <span class="auto-sub">
                                            {g.socket_ids.length} device{g.socket_ids.length === 1 ? "" : "s"}
                                            {#if g.on > 0}· <span class="auto-on">{g.on} on</span>{/if}
                                        </span>
                                    </div>
                                    <div class="auto-btns">
                                        <button class="btn btn-success btn-xs"
                                            onclick={() => runAction(() => api.groupAction(g.id, "on"), `${g.name} on`)}>
                                            On
                                        </button>
                                        <button class="btn btn-danger btn-xs"
                                            onclick={() => runAction(() => api.groupAction(g.id, "off"), `${g.name} off`)}>
                                            Off
                                        </button>
                                    </div>
                                </div>
                            {/each}
                        </div>
                    {/if}

                    {#if v.scenes.length > 0}
                        <div class="auto-section">
                            <div class="auto-label">Scenes</div>
                            {#each v.scenes as sc (sc.id)}
                                <div class="auto-row">
                                    <div class="auto-info">
                                        <span class="auto-name">{sc.name}</span>
                                        <span class="auto-sub">{sc.actions.length} action{sc.actions.length === 1 ? "" : "s"}</span>
                                    </div>
                                    <button class="btn btn-primary btn-xs"
                                        onclick={() => runAction(() => api.activateScene(sc.id), `Activated ${sc.name}`)}>
                                        Activate
                                    </button>
                                </div>
                            {/each}
                        </div>
                    {/if}
                {/if}

                <div class="df">
                    <button class="btn btn-ghost" onclick={() => goAndClose("groups")}>Groups →</button>
                    <button class="btn btn-ghost" onclick={() => goAndClose("scenes")}>Scenes →</button>
                </div>
            {/if}
        </div>
    </div>
{/if}

<!-- ── Rest of dashboard ──────────────────────────────────────────── -->
<section class="card">
    <div class="card-header"><h2>Quick actions</h2></div>
    <div class="quick">
        <button class="btn btn-success" onclick={allOn}>All on</button>
        <button class="btn btn-danger"  onclick={allOff}>All off</button>
        <button class="btn btn-ghost"   onclick={() => openModal(ShortcutsModal, {})}>iOS Shortcuts</button>
        <button class="btn btn-ghost"   onclick={() => data.refresh()}>Refresh</button>
    </div>
</section>

<section class="card">
    <div class="card-header">
        <h2>Scenes</h2>
        <button class="btn btn-ghost" onclick={() => openModal(SceneModal, {})}>New scene</button>
    </div>
    {#if v.scenes.length === 0}
        <p class="field-help">No scenes yet. Click "New scene" to combine a few devices into a one-tap action.</p>
    {:else}
        <div class="scenes">
            {#each v.scenes as scene, i (scene.id)}
                <div class="scene-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <SceneTile {scene} />
                </div>
            {/each}
        </div>
    {/if}
</section>

{#if v.sensors.length > 0}
    <section class="card">
        <div class="card-header">
            <h2>Sensors</h2>
            <button class="btn btn-ghost" onclick={() => route.go("sensors")}>View all</button>
        </div>
        <div class="sensors">
            {#each v.sensors.slice(0, 6) as sensor, i (sensor.id)}
                <div class="sensor-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <SensorCard {sensor} compact />
                </div>
            {/each}
        </div>
    </section>
{/if}

{#if v.timers.length > 0}
    <section class="card">
        <div class="card-header"><h2>Pending timers</h2></div>
        <div class="timers">
            {#each v.timers as timer, i (timer.id)}
                <div
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:fly={{ y: 10, duration: dur(220), delay: stagger(i), easing: cubicOut }}
                    out:scale={{ start: 0.97, opacity: 0, duration: dur(160) }}>
                    <TimerRow {timer} />
                </div>
            {/each}
        </div>
    </section>
{/if}

<section class="card">
    <div class="card-header"><h2>Rooms</h2></div>
    {#if v.rooms.length === 0}
        <p class="field-help">No rooms yet. Create devices and assign rooms to them.</p>
    {:else}
        <div class="rooms">
            {#each v.rooms as room, i (room.name)}
                <div class="room-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <RoomCard {room} />
                </div>
            {/each}
        </div>
    {/if}
</section>

{#if v.activity.length > 0}
    <section class="card">
        <div class="card-header"><h2>Recent activity</h2></div>
        <ul class="activity">
            {#each v.activity as a (a.id)}
                <li class="event" data-status={a.status}
                    animate:flip={{ duration: dur(260), easing: cubicOut }}
                    in:fly={{ x: -10, duration: dur(220), easing: cubicOut }}>
                    <span class="src" data-source={a.source}>{a.source}</span>
                    <div class="info">
                        <div class="line">
                            <span class="act" data-action={a.action}>{a.action}</span>
                            <span class="label">{a.label}</span>
                        </div>
                        {#if a.error}<div class="err">{a.error}</div>{/if}
                    </div>
                    <time class="when">{new Date(a.time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</time>
                </li>
            {/each}
        </ul>
    </section>
{/if}

<style>
    /* ── Stat cards ─────────────────────────────────── */
    .stats {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
        gap: var(--space-4);
    }
    .stat {
        all: unset;
        box-sizing: border-box;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-4) var(--space-5);
        display: flex;
        align-items: center;
        gap: var(--space-4);
        cursor: pointer;
        width: 100%;
        transition: border-color var(--t-fast), box-shadow var(--t-fast), transform var(--t-fast);
        position: relative;
    }
    @media (hover: hover) {
        .stat:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
    }
    .stat:focus-visible { box-shadow: var(--focus-ring); }

    /* Selected state — tone-specific glow */
    .stat.selected[data-tone="primary"] { border-color: var(--primary); box-shadow: 0 0 0 3px var(--primary-glow); }
    .stat.selected[data-tone="success"] { border-color: var(--success); box-shadow: 0 0 0 3px var(--success-soft); }
    .stat.selected[data-tone="info"]    { border-color: var(--info);    box-shadow: 0 0 0 3px var(--info-soft);    }
    .stat.selected[data-tone="warn"]    { border-color: var(--warn);    box-shadow: 0 0 0 3px var(--warn-soft);    }

    .ico {
        width: 40px; height: 40px;
        border-radius: var(--radius-md);
        display: grid; place-items: center;
        flex-shrink: 0;
    }
    .ico[data-tone="primary"] { background: var(--primary-soft); color: var(--primary); }
    .ico[data-tone="success"] { background: var(--success-soft); color: var(--success); }
    .ico[data-tone="info"]    { background: var(--info-soft);    color: var(--info);    }
    .ico[data-tone="warn"]    { background: var(--warn-soft);    color: var(--warn);    }

    .stat-text { flex: 1; min-width: 0; text-align: left; }
    .value { font-size: 1.5rem; font-weight: 700; line-height: 1; }
    .label { color: var(--text-muted); font-size: 13px; margin-top: 4px; }

    .caret {
        margin-left: auto;
        color: var(--text-faint);
        transition: transform var(--t-med), color var(--t-fast);
        flex-shrink: 0;
    }
    .caret.open { transform: rotate(180deg); color: var(--text-muted); }

    /* ── Detail panel ───────────────────────────────── */
    .detail {
        background: var(--bg-elevated);
        border-radius: var(--radius-lg);
        border: 1px solid var(--border);
        overflow: hidden;
    }
    /* Left accent bar by tone */
    .detail[data-tone="primary"] { box-shadow: inset 4px 0 0 var(--primary); }
    .detail[data-tone="success"] { box-shadow: inset 4px 0 0 var(--success); }
    .detail[data-tone="info"]    { box-shadow: inset 4px 0 0 var(--info);    }
    .detail[data-tone="warn"]    { box-shadow: inset 4px 0 0 var(--warn);    }

    /* header */
    .dh {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--space-4) var(--space-5) var(--space-3);
        border-bottom: 1px solid var(--border);
    }
    .dh-left { display: flex; align-items: center; gap: var(--space-2); }
    .dico {
        width: 24px; height: 24px;
        border-radius: var(--radius-sm);
        display: grid; place-items: center;
    }
    .dico[data-tone="primary"] { background: var(--primary-soft); color: var(--primary); }
    .dico[data-tone="success"] { background: var(--success-soft); color: var(--success); }
    .dico[data-tone="info"]    { background: var(--info-soft);    color: var(--info);    }
    .dico[data-tone="warn"]    { background: var(--warn-soft);    color: var(--warn);    }
    .dt { font-weight: 600; font-size: 0.875rem; }

    /* body */
    .db {
        padding: var(--space-4) var(--space-5);
        display: flex;
        flex-direction: column;
        gap: var(--space-4);
        max-height: 340px;
        overflow-y: auto;
    }

    /* footer */
    .df {
        display: flex;
        gap: var(--space-2);
        flex-wrap: wrap;
        padding-top: var(--space-2);
        border-top: 1px solid var(--border);
        margin-top: var(--space-1);
    }

    /* ── Devices panel ─────────────────────── */
    .proto-row { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .pbadge {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        font-weight: 600;
        padding: 4px 10px;
        border-radius: 999px;
        border: 1px solid;
    }
    .pbadge[data-proto="rf"]     { color: var(--accent-rf);     background: var(--accent-rf-soft);     border-color: var(--accent-rf-soft);     }
    .pbadge[data-proto="tasmota"]{ color: var(--accent-wifi);   background: var(--accent-wifi-soft);   border-color: var(--accent-wifi-soft);   }
    .pbadge[data-proto="matter"] { color: var(--accent-matter); background: var(--accent-matter-soft); border-color: var(--accent-matter-soft); }

    .note { font-size: 13px; color: var(--text-faint); }

    .room-grid { display: flex; flex-direction: column; gap: 4px; }
    .rg-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        padding: 6px 0;
        border-bottom: 1px solid var(--border);
        font-size: 13px;
    }
    .rg-row:last-child { border-bottom: none; }
    .rg-name { font-weight: 500; }
    .rg-counts { display: flex; gap: var(--space-3); color: var(--text-muted); }
    .rg-on { color: var(--success); font-weight: 600; }
    .rg-total { color: var(--text-faint); }

    /* ── Active now panel ──────────────────── */
    .dev-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
    .dev-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 6px 8px;
        border-radius: var(--radius-sm);
        background: var(--surface);
        font-size: 13px;
    }
    .dev-dot {
        width: 8px; height: 8px;
        border-radius: 50%;
        background: var(--success);
        box-shadow: 0 0 0 3px var(--success-soft);
        flex-shrink: 0;
    }
    .dev-name { flex: 1; font-weight: 500; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .room-chip {
        font-size: 11px;
        padding: 2px 8px;
        background: var(--surface-hover);
        border: 1px solid var(--border);
        border-radius: 999px;
        color: var(--text-muted);
        white-space: nowrap;
    }

    /* ── Schedules panel ───────────────────── */
    .sched-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
    .sched-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: 6px 8px;
        border-radius: var(--radius-sm);
        background: var(--surface);
        font-size: 13px;
        flex-wrap: wrap;
    }
    .action-pill {
        font-size: 10px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        padding: 2px 7px;
        border-radius: 999px;
        background: var(--surface-hover);
        color: var(--text-muted);
        flex-shrink: 0;
    }
    .action-pill[data-action="on"]       { background: var(--success-soft); color: var(--success); }
    .action-pill[data-action="off"]      { background: var(--danger-soft);  color: var(--danger);  }
    .action-pill[data-action="activate"] { background: var(--info-soft);    color: var(--info);    }
    .sched-target { flex: 1; font-weight: 500; min-width: 80px; }
    .sched-time { color: var(--text-muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
    .sched-days { color: var(--text-faint); font-size: 11px; white-space: nowrap; }

    /* ── Automations panel ─────────────────── */
    .auto-section { display: flex; flex-direction: column; gap: 4px; }
    .auto-section + .auto-section { padding-top: var(--space-3); border-top: 1px solid var(--border); }
    .auto-label {
        font-size: 11px;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--text-faint);
        margin-bottom: var(--space-1);
    }
    .auto-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
        padding: 7px 8px;
        border-radius: var(--radius-sm);
        background: var(--surface);
        font-size: 13px;
    }
    .auto-info { min-width: 0; flex: 1; }
    .auto-name { font-weight: 500; display: block; }
    .auto-sub { color: var(--text-faint); font-size: 11px; }
    .auto-on { color: var(--success); }
    .auto-btns { display: flex; gap: var(--space-1); }

    /* xs button variant */
    :global(.btn-xs) { padding: 4px 10px; font-size: 12px; }

    /* ── Empty state inside panel ──────────── */
    .dempty {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--space-2);
        padding: var(--space-6) 0;
        color: var(--text-faint);
        text-align: center;
    }
    .dempty p { font-size: 13px; margin: 0; }

    /* ── Rest of dashboard ─────────────────────────── */
    .quick { display: flex; flex-wrap: wrap; gap: var(--space-2); }
    .scenes {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
        gap: var(--space-3);
    }
    .timers { display: flex; flex-direction: column; gap: var(--space-2); }
    .sensors {
        display: grid;
        grid-template-columns: 1fr;
        gap: var(--space-3);
    }
    @media (min-width: 600px) {
        .sensors { grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); }
    }
    .sensor-item { display: flex; }
    .sensor-item > :global(.card) { flex: 1; }
    .rooms {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
        gap: var(--space-3);
    }
    .scene-item, .room-item { display: flex; }
    .scene-item > :global(*), .room-item > :global(.card) { flex: 1; }

    .activity { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
    .event {
        display: grid;
        grid-template-columns: auto 1fr auto;
        align-items: center;
        gap: var(--space-3);
        padding: 8px 10px;
        border-radius: var(--radius-sm);
        background: var(--surface);
    }
    .event[data-status="error"] { background: var(--danger-soft); }
    .src {
        font-size: 11px;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        font-weight: 600;
        color: var(--text-muted);
        padding: 2px 8px;
        border-radius: 999px;
        background: var(--bg-elevated);
        border: 1px solid var(--border);
    }
    .src[data-source="schedule"] { color: var(--info); }
    .src[data-source="timer"]    { color: var(--warn); }
    .info { min-width: 0; }
    .line { display: flex; align-items: baseline; gap: var(--space-2); flex-wrap: wrap; }
    .act {
        font-weight: 600;
        text-transform: uppercase;
        font-size: 12px;
        letter-spacing: 0.04em;
        color: var(--text-muted);
    }
    .act[data-action="on"] { color: var(--success); }
    .act[data-action="off"] { color: var(--danger); }
    .act[data-action="activate"] { color: var(--info); }
    .label { color: var(--text); }
    .err { color: var(--danger); font-size: 12px; margin-top: 2px; }
    .when { color: var(--text-faint); font-size: 12px; font-variant-numeric: tabular-nums; }
</style>
