<script lang="ts">
    import Icon from "../components/Icon.svelte";
    import RoomCard from "../components/RoomCard.svelte";
    import SceneTile from "../components/SceneTile.svelte";
    import SensorCard from "../components/SensorCard.svelte";
    import SocketCard from "../components/SocketCard.svelte";
    import TimerRow from "../components/TimerRow.svelte";
    import { route, data, toasts, session } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { runAction, describeTarget } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import ConfirmModal from "../components/ConfirmModal.svelte";
    import SocketModal from "../modals/SocketModal.svelte";
    import { fly, scale } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { Tween } from "svelte/motion";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    // ── Live clock ───────────────────────────────────────────────────────────
    let now = $state(new Date());
    $effect(() => {
        const id = setInterval(() => { now = new Date(); }, 1000);
        return () => clearInterval(id);
    });
    const greeting = $derived(
        now.getHours() < 12 ? "Good morning" :
        now.getHours() < 18 ? "Good afternoon" : "Good evening"
    );
    const dateLabel = $derived(
        now.toLocaleDateString([], { weekday: "long" }) + ", " +
        now.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })
    );
    const name = $derived(session.user?.username || "there");

    // ── Whole-home hero ─────────────────────────────────────────────────────
    const favoriteSockets = $derived(v.sockets.filter(s => s.favorite));
    const totalSockets = $derived(v.sockets.length);
    const onSockets = $derived(v.sockets.filter(s => s.state).length);
    const heroOn = $derived(onSockets > 0);

    // Animated on-count: tween to avoid jarring jumps on toggle.
    const tweenedOn = new Tween(0);
    let _onInit = true;
    $effect(() => {
        const d = _onInit ? 0 : dur(500);
        _onInit = false;
        tweenedOn.set(onSockets, { duration: d, easing: cubicOut });
    });

    const powerSensors = $derived(v.sensors.filter(s => s.kind === "power" && s.last_value != null));
    const hasPower = $derived(powerSensors.length > 0);
    const powerWatts = $derived(Math.round(powerSensors.reduce((sum, s) => sum + (s.last_value ?? 0), 0)));
    const tempSensors = $derived(v.sensors.filter(s => s.kind === "temperature" && s.last_value != null));
    const hasTemp = $derived(tempSensors.length > 0);
    const insideTemp = $derived(
        hasTemp ? Math.round(tempSensors.reduce((sum, s) => sum + (s.last_value ?? 0), 0) / tempSensors.length) : 0
    );

    // Desktop stat strip (wide screens only): real metrics that complement the
    // hero — how many automations are active and the next scheduled event.
    const enabledAutomations = $derived(v.automations.filter(a => a.enabled).length);
    const nextEvent = $derived.by(() => {
        const nowMin = now.getHours() * 60 + now.getMinutes();
        const parse = (s?: string) => {
            if (!s || !/^\d\d:\d\d/.test(s)) return -1;
            const [h, m] = s.split(":").map(Number);
            return h * 60 + m;
        };
        const items = v.schedules
            .filter(s => s.enabled)
            .map(s => ({
                min: parse(s.effective_time || s.time),
                time: s.effective_time || s.time,
                label: describeTarget(s.target_type, s.target_id, s.socket_id).label,
            }))
            .filter(x => x.min >= 0);
        if (items.length === 0) return null;
        const upcoming = items.filter(x => x.min >= nowMin).sort((a, b) => a.min - b.min);
        return upcoming[0] ?? items.sort((a, b) => a.min - b.min)[0];
    });

    // Desktop device grid — filterable by room on the dashboard.
    let deviceRoom = $state('');
    const allDeviceRooms = $derived([...new Set(v.sockets.map(s => s.room || 'Unassigned'))].sort());
    const filteredDevices = $derived.by(() => {
        if (deviceRoom === 'on') return v.sockets.filter(s => s.state);
        if (deviceRoom) return v.sockets.filter(s => (s.room || 'Unassigned') === deviceRoom);
        return v.sockets;
    });

    // Groups with a live on-count for the groups section.
    const groupsWithState = $derived(
        v.groups.map(g => ({
            ...g,
            on: g.socket_ids.filter(id => v.sockets.find(s => s.id === id)?.state).length,
        }))
    );

    // Live room on-counts derived from socket state so RoomCards stay in sync
    // with optimistic toggles rather than waiting for the next server refresh.
    const liveRooms = $derived.by(() => {
        const onByRoom = new Map<string, number>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            onByRoom.set(r, (onByRoom.get(r) ?? 0) + (s.state ? 1 : 0));
        }
        return v.rooms.map(r => ({ ...r, on: onByRoom.get(r.name) ?? 0 }));
    });

    // ── Bulk actions ────────────────────────────────────────────────────────
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
    function toggleAllMaster() {
        if (heroOn) allOff(); else allOn();
    }
</script>

<!-- ── Greeting header ────────────────────────────────────────────── -->
<header class="greeting">
    <div class="greet-text">
        <div class="greet-date mono">{dateLabel}</div>
        <h1 class="greet-title">{greeting},<br /><span class="greet-name">{name}</span></h1>
    </div>
    {#if session.isAdmin}
        <div class="greet-actions">
            <button class="chip search-chip" onclick={() => route.go("sockets")} aria-label="Search devices and scenes">
                <Icon name="search" size={14} />
                Search devices, scenes…
                <kbd class="search-key">⌘K</kbd>
            </button>
            <button class="chip add-device" onclick={() => openModal(SocketModal, {})}>
                <Icon name="plus" size={14} /> Add device
            </button>
            <button class="chip icon-chip" aria-label="Activity" onclick={() => route.go("activity")}>
                <Icon name="activity" size={16} />
            </button>
        </div>
    {/if}
</header>

<!-- ── Whole-home hero + desktop stat strip ───────────────────────── -->
<div class="top-grid">
    <div class="hero tile" class:on={heroOn}
        in:fly={{ y: 14, duration: dur(280), easing: cubicOut }}>
        <div class="hero-top">
            <div class="hero-lead">
                <div class="hero-eyebrow mono">Whole home</div>
                <div class="hero-count">
                    <span class="num-display">{Math.round(tweenedOn.current)}</span>
                    <span class="hero-of">of {totalSockets} on</span>
                </div>
            </div>
            <button class="sw-big" class:on={heroOn} onclick={toggleAllMaster}
                aria-label={heroOn ? "Turn all devices off" : "Turn all devices on"}
                aria-pressed={heroOn}></button>
        </div>
        {#if hasPower || hasTemp}
            <div class="hero-meta">
                {#if hasPower}
                    <span class="hero-stat">
                        <Icon name="bolt" size={13} />
                        <span class="mono hero-em">{powerWatts} W</span> now
                    </span>
                {/if}
                {#if hasPower && hasTemp}<span class="hero-sep">·</span>{/if}
                {#if hasTemp}
                    <span class="hero-stat"><span class="mono hero-em">{insideTemp}°</span> inside</span>
                {/if}
            </div>
        {/if}
    </div>

    {#if hasTemp}
        <div class="stat tile">
            <div class="stat-eyebrow mono">Temperature</div>
            <div class="stat-value cool">{insideTemp}°</div>
            <div class="stat-sub">{tempSensors[0]?.name ?? 'Inside'}</div>
        </div>
    {/if}
    {#if nextEvent}
        <div class="stat tile">
            <div class="stat-eyebrow mono">Next event</div>
            <div class="stat-value">{nextEvent.time}</div>
            <div class="stat-sub">{nextEvent.label}</div>
        </div>
    {/if}
    {#if enabledAutomations > 0}
        <div class="stat tile">
            <div class="stat-eyebrow mono">Automations</div>
            <div class="stat-value">{enabledAutomations}</div>
            <div class="stat-sub">active</div>
        </div>
    {/if}
</div>

<!-- ── Scenes scroller ────────────────────────────────────────────── -->
{#if v.scenes.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2>Scenes</h2>
            <button class="chip" onclick={() => route.go("scenes")}>All</button>
        </div>
        <div class="scene-scroll h-scroll">
            {#each v.scenes as scene (scene.id)}
                <div class="scene-cell"><SceneTile {scene} /></div>
            {/each}
        </div>
    </section>
{/if}

<!-- ── Favorites ──────────────────────────────────────────────────── -->
{#if favoriteSockets.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2><Icon name="star" size={16} /> Favorites</h2>
            <span class="header-meta">{favoriteSockets.length}</span>
        </div>
        <div class="favorites">
            {#each favoriteSockets as socket, i (socket.id)}
                <div class="favorite-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <SocketCard {socket} />
                </div>
            {/each}
        </div>
    </section>
{/if}

<!-- ── Groups ─────────────────────────────────────────────────────── -->
{#if groupsWithState.length > 0}
    <section class="home-section">
        <div class="section-head"><h2><span class="section-ico"><Icon name="groups" size={15} /></span>Groups</h2></div>
        <div class="group-list">
            {#each groupsWithState as g, i (g.id)}
                <div class="group-row"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:fly={{ y: 8, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <div class="group-info">
                        <span class="group-name">{g.name}</span>
                        <span class="group-meta">
                            <span class="mono">{g.socket_ids.length}</span> socket{g.socket_ids.length === 1 ? '' : 's'}
                            {#if g.on > 0}<span class="group-on">· <span class="mono">{g.on}</span> on</span>{/if}
                        </span>
                    </div>
                    <div class="group-actions">
                        <button class="btn btn-success"
                            disabled={g.on === g.socket_ids.length}
                            title={g.on === g.socket_ids.length ? "All already on" : undefined}
                            onclick={() => runAction(() => api.groupAction(g.id, 'on'), `${g.name} on`)}>On</button>
                        <button class="btn btn-danger"
                            disabled={g.on === 0}
                            title={g.on === 0 ? "All already off" : undefined}
                            onclick={() => runAction(() => api.groupAction(g.id, 'off'), `${g.name} off`)}>Off</button>
                    </div>
                </div>
            {/each}
        </div>
    </section>
{/if}

<!-- ── Sensors ────────────────────────────────────────────────────── -->
{#if v.sensors.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2>Sensors</h2>
            <button class="chip" onclick={() => route.go("sensors")}>All</button>
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

<!-- ── Pending timers ─────────────────────────────────────────────── -->
{#if v.timers.length > 0}
    <section class="home-section">
        <div class="section-head"><h2>Pending timers</h2></div>
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

<!-- ── Rooms ──────────────────────────────────────────────────────── -->
<section class="home-section mobile-rooms">
    <div class="section-head"><h2><span class="section-ico"><Icon name="home" size={15} /></span>Rooms</h2></div>
    {#if liveRooms.length === 0}
        <p class="field-help">No rooms yet. Create devices and assign rooms to them.</p>
    {:else}
        <div class="rooms">
            {#each liveRooms as room, i (room.name)}
                <div class="room-item"
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <RoomCard {room} />
                </div>
            {/each}
        </div>
    {/if}
</section>

<!-- ── Desktop: all devices with room filter ─────────────────────── -->
{#if v.sockets.length > 0}
    <section class="home-section desktop-devices">
        <div class="section-head">
            <h2>Devices</h2>
            <div class="device-chips">
                <button class="chip" class:active={deviceRoom === ''} onclick={() => deviceRoom = ''}>All</button>
                <button class="chip" class:active={deviceRoom === 'on'} onclick={() => deviceRoom = 'on'}>On</button>
                {#each allDeviceRooms as r}
                    <button class="chip" class:active={deviceRoom === r} onclick={() => deviceRoom = r}>{r}</button>
                {/each}
            </div>
        </div>
        {#if filteredDevices.length === 0}
            <p class="field-help">No devices match this filter.</p>
        {:else}
            <div class="device-grid">
                {#each filteredDevices as socket (socket.id)}
                    <SocketCard {socket} />
                {/each}
            </div>
        {/if}
    </section>
{/if}

<style>
    /* ── Greeting ───────────────────────────────────── */
    .greeting {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .greet-date { color: var(--text-mute); font-size: 13px; font-weight: 500; }
    .greet-title {
        font-size: 30px;
        font-weight: 600;
        letter-spacing: -0.03em;
        margin-top: 4px;
        line-height: 1.1;
    }
    .greet-name { color: var(--text-mute); }
    .greet-actions { display: flex; gap: var(--space-2); flex-shrink: 0; align-items: center; }
    .icon-chip {
        width: 38px; height: 38px;
        padding: 0;
        justify-content: center;
    }
    /* Search chip and "Add device" are desktop-only labels */
    .search-chip { display: none; }
    .add-device { display: none; }
    @media (min-width: 700px) { .add-device { display: inline-flex; } }
    @media (min-width: 1024px) {
        .search-chip {
            display: inline-flex;
            gap: 8px;
            padding: 9px 14px;
            color: var(--text-mute);
        }
    }
    .search-key {
        margin-left: 20px;
        font-family: var(--font-mono);
        font-size: 11px;
        color: var(--text-dim);
        background: none;
        border: none;
        padding: 0;
        cursor: pointer;
    }

    /* Hero spans full width on phones; on desktop it shares a row with the
       real-data stat tiles, matching the desktop dashboard mockup. */
    .top-grid { display: grid; grid-template-columns: 1fr; gap: var(--space-4); }
    .stat { display: none; }
    @media (min-width: 1024px) {
        /* 2 stat tiles → 3 columns */
        .top-grid { grid-template-columns: 1.6fr 1fr 1fr; align-items: stretch; }
        .stat { display: flex; }
    }
    @media (min-width: 1280px) {
        /* 3 stat tiles → 4 columns */
        .top-grid { grid-template-columns: 1.6fr 1fr 1fr 1fr; }
    }
    .stat {
        padding: 18px;
        flex-direction: column;
        justify-content: space-between;
        gap: 8px;
    }
    .stat-eyebrow {
        color: var(--text-mute);
        font-size: 11px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
    }
    .stat-value { font-size: 32px; font-weight: 600; letter-spacing: -0.02em; font-family: var(--font-mono); }
    .stat-value.cool { color: var(--cool); }
    .stat-sub { color: var(--text-mute); font-size: 12px; }

    /* ── Whole-home hero ────────────────────────────── */
    .hero { padding: 20px; gap: 16px; }
    .hero-top {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: 12px;
    }
    .hero-lead { min-width: 0; }
    .hero-eyebrow {
        color: var(--on);
        font-size: 11px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
    }
    .hero-count {
        margin-top: 8px;
        display: flex;
        align-items: baseline;
        gap: 10px;
        white-space: nowrap;
    }
    .hero-count .num-display { font-size: 56px; }
    .hero-of { color: var(--text-mute); font-size: 14px; }
    .hero .sw-big {
        flex-shrink: 0;
        border: 0; padding: 0;
        appearance: none; -webkit-appearance: none;
        cursor: pointer;
    }
    .hero .sw-big:focus-visible { box-shadow: var(--focus-ring); }
    .hero-meta {
        display: flex;
        align-items: center;
        gap: 8px;
        color: var(--text-mute);
        font-size: 12px;
        white-space: nowrap;
    }
    .hero-stat { display: inline-flex; align-items: center; gap: 6px; }
    .hero-stat :global(svg) { color: var(--on); }
    .hero-em { color: var(--text); }
    .hero-sep { color: var(--text-dim); }

    /* ── Sections ───────────────────────────────────── */
    .home-section { display: flex; flex-direction: column; gap: var(--space-3); }
    .section-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--space-3);
    }
    .section-head h2 {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 17px;
        font-weight: 600;
    }
    .section-ico {
        width: 24px; height: 24px;
        border-radius: var(--r-sm);
        display: grid; place-items: center;
        background: var(--on-soft);
        color: var(--on);
        flex-shrink: 0;
    }
    .header-meta {
        font-size: 12px;
        color: var(--text-muted);
        background: var(--surface);
        padding: 2px 8px;
        border-radius: 999px;
        font-variant-numeric: tabular-nums;
    }

    /* ── Scenes: horizontal scroller on phones, grid on wide screens ── */
    .scene-scroll { padding-bottom: 2px; }
    .scene-cell { width: 160px; display: flex; }
    .scene-cell > :global(*) { flex: 1; min-width: 0; }
    @media (min-width: 700px) {
        .scene-scroll {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
            gap: var(--space-3);
            overflow: visible;
        }
        .scene-cell { width: auto; }
    }

    /* ── Favorites grid ─────────────────────────────── */
    .favorites {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .favorites { grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: var(--space-3); }
    }
    .favorite-item { display: flex; min-width: 0; }
    .favorite-item > :global(.tile) { flex: 1; min-width: 0; }

    /* ── Groups ─────────────────────────────────────── */
    .group-list { display: flex; flex-direction: column; gap: var(--space-2); }
    .group-row {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        padding: var(--space-2) var(--space-3);
        background: var(--surface);
        border-radius: var(--radius-md);
        min-height: 48px;
    }
    @media (pointer: coarse) {
        .group-row { padding: var(--space-3); min-height: 60px; }
    }
    .group-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .group-name { font-weight: 500; }
    .group-meta { color: var(--text-muted); font-size: 12px; }
    .group-on { color: var(--on); font-weight: 600; }
    .group-actions { display: flex; gap: var(--space-2); flex-shrink: 0; }

    /* ── Sensors ────────────────────────────────────── */
    .sensors {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .sensors { grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: var(--space-3); }
    }
    .sensor-item { display: flex; min-width: 0; }
    .sensor-item > :global(.sensor) { flex: 1; min-width: 0; }

    .timers { display: flex; flex-direction: column; gap: var(--space-2); }

    /* ── Rooms ──────────────────────────────────────── */
    .rooms {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: var(--space-2);
    }
    @media (min-width: 560px) {
        .rooms {
            grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
            gap: var(--space-3);
        }
    }
    .room-item { display: flex; min-width: 0; }
    .room-item > :global(.room) { flex: 1; min-width: 0; }

    /* ── Desktop devices section ────────────────────── */
    /* Rooms section hides on desktop (replaced by the filterable device grid) */
    @media (min-width: 1024px) {
        .mobile-rooms { display: none; }
    }

    .desktop-devices { display: none; }
    @media (min-width: 1024px) {
        .desktop-devices { display: flex; }
    }

    .device-chips { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }

    .device-grid {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 12px;
    }
    @media (min-width: 1280px) {
        .device-grid { grid-template-columns: repeat(4, 1fr); }
    }

</style>
