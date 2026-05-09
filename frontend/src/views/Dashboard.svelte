<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import RoomCard from "../components/RoomCard.svelte";
    import SceneTile from "../components/SceneTile.svelte";
    import TimerRow from "../components/TimerRow.svelte";
    import { api } from "../lib/api";
    import { data, toasts } from "../lib/stores";
    import { runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import SocketModal from "../modals/SocketModal.svelte";
    import SceneModal from "../modals/SceneModal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";

    const v = $derived(data.value);
    const totalSockets = $derived(v.sockets.length);
    const onSockets = $derived(v.sockets.filter(s => s.state).length);
    const enabledSchedules = $derived(v.schedules.filter(s => s.enabled).length);
    const groupsAndScenes = $derived(v.groups.length + v.scenes.length);

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
    <div class="stat">
        <div class="ico" data-tone="primary"><Icon name="bolt" size={20} /></div>
        <div><div class="value">{totalSockets}</div><div class="label">Total sockets</div></div>
    </div>
    <div class="stat">
        <div class="ico" data-tone="success"><Icon name="check" size={20} /></div>
        <div><div class="value">{onSockets}</div><div class="label">Currently on</div></div>
    </div>
    <div class="stat">
        <div class="ico" data-tone="info"><Icon name="clock" size={20} /></div>
        <div><div class="value">{enabledSchedules}</div><div class="label">Active schedules</div></div>
    </div>
    <div class="stat">
        <div class="ico" data-tone="warn"><Icon name="groups" size={20} /></div>
        <div><div class="value">{groupsAndScenes}</div><div class="label">Groups &amp; scenes</div></div>
    </div>
</div>

<section class="card">
    <div class="card-header"><h2>Quick actions</h2></div>
    <div class="quick">
        <button class="btn btn-secondary" onclick={allOn}>Turn all on</button>
        <button class="btn btn-secondary" onclick={allOff}>Turn all off</button>
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
            {#each v.scenes as scene (scene.id)}
                <SceneTile {scene} />
            {/each}
        </div>
    {/if}
</section>

{#if v.timers.length > 0}
    <section class="card">
        <div class="card-header"><h2>Pending timers</h2></div>
        <div class="timers">
            {#each v.timers as timer (timer.id)}
                <TimerRow {timer} />
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
            {#each v.rooms as room (room.name)}
                <RoomCard {room} />
            {/each}
        </div>
    {/if}
</section>

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
    .rooms {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
        gap: var(--space-3);
    }
</style>
