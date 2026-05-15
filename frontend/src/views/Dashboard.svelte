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
    import { Tween } from "svelte/motion";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);
    const totalSockets = $derived(v.sockets.length);
    const onSockets = $derived(v.sockets.filter(s => s.state).length);
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

    async function allOn() {
        const ok = await openModal<boolean>(ConfirmModal, {
            title: "Turn all sockets ON?",
            message: `This will switch on ${totalSockets} socket${totalSockets === 1 ? "" : "s"}.`,
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
            title: "Turn all sockets OFF?",
            message: `This will switch off ${totalSockets} socket${totalSockets === 1 ? "" : "s"}.`,
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

<Topbar title="Dashboard" subtitle="Overview of your RF sockets">
    {#snippet actions()}
        <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add socket</button>
    {/snippet}
</Topbar>

<div class="stats">
    <div class="stat" in:fly={{ y: 14, duration: dur(280), delay: stagger(0, 60), easing: cubicOut }}>
        <div class="ico" data-tone="primary"><Icon name="bolt" size={20} /></div>
        <div><div class="value">{Math.round(totalT.current)}</div><div class="label">Total sockets</div></div>
    </div>
    <div class="stat" in:fly={{ y: 14, duration: dur(280), delay: stagger(1, 60), easing: cubicOut }}>
        <div class="ico" data-tone="success"><Icon name="check" size={20} /></div>
        <div><div class="value">{Math.round(onT.current)}</div><div class="label">Currently on</div></div>
    </div>
    <div class="stat" in:fly={{ y: 14, duration: dur(280), delay: stagger(2, 60), easing: cubicOut }}>
        <div class="ico" data-tone="info"><Icon name="clock" size={20} /></div>
        <div><div class="value">{Math.round(schedT.current)}</div><div class="label">Active schedules</div></div>
    </div>
    <div class="stat" in:fly={{ y: 14, duration: dur(280), delay: stagger(3, 60), easing: cubicOut }}>
        <div class="ico" data-tone="warn"><Icon name="groups" size={20} /></div>
        <div><div class="value">{Math.round(gsT.current)}</div><div class="label">Groups &amp; scenes</div></div>
    </div>
</div>

<section class="card">
    <div class="card-header"><h2>Quick actions</h2></div>
    <div class="quick">
        <button class="btn btn-secondary" onclick={allOn}>Turn all on</button>
        <button class="btn btn-secondary" onclick={allOff}>Turn all off</button>
        <button class="btn btn-ghost" onclick={() => openModal(ShortcutsModal, {})}>iOS Shortcuts</button>
        <button class="btn btn-ghost" onclick={() => data.refresh()}>Refresh</button>
    </div>
</section>

<section class="card">
    <div class="card-header">
        <h2>Scenes</h2>
        <button class="btn btn-ghost" onclick={() => openModal(SceneModal, {})}>New scene</button>
    </div>
    {#if v.scenes.length === 0}
        <p class="field-help">No scenes yet. Click “New scene” to combine a few sockets into a one-tap action.</p>
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
        <p class="field-help">No rooms yet. Create sockets and assign rooms to them.</p>
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
    .stats {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
        gap: var(--space-4);
    }
    .stat {
        background: var(--bg-elevated);
        border: 1px solid var(--border);
        border-radius: var(--radius-lg);
        padding: var(--space-4) var(--space-5);
        display: flex;
        align-items: center;
        gap: var(--space-4);
    }
    .ico {
        width: 40px; height: 40px;
        border-radius: var(--radius-md);
        display: grid; place-items: center;
    }
    .ico[data-tone="primary"] { background: var(--info-soft);    color: var(--info);    }
    .ico[data-tone="success"] { background: var(--success-soft); color: var(--success); }
    .ico[data-tone="info"]    { background: var(--info-soft);    color: var(--info);    }
    .ico[data-tone="warn"]    { background: var(--warn-soft);    color: var(--warn);    }
    .value { font-size: 1.5rem; font-weight: 700; line-height: 1; }
    .label { color: var(--text-muted); font-size: 13px; margin-top: 4px; }

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
