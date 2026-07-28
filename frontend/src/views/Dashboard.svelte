<script lang="ts">
    import Icon from "../components/Icon.svelte";
    import NowPlaying from "../components/NowPlaying.svelte";
    import RoomCard from "../components/RoomCard.svelte";
    import SensorCard from "../components/SensorCard.svelte";
    import SocketCard from "../components/SocketCard.svelte";
    import Switch from "../components/Switch.svelte";
    import TimerRow from "../components/TimerRow.svelte";
    import { route, data, toasts, session } from "../lib/stores.svelte";
    import { api } from "../lib/api";
    import { runAction, describeTarget, haptic } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import EmptyState from "../components/EmptyState.svelte";
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
    // Derive from the minute, not the 1-second clock tick — otherwise the
    // whole schedule list is re-parsed and re-sorted every second.
    const nowMinute = $derived(now.getHours() * 60 + now.getMinutes());
    const nextEvent = $derived.by(() => {
        const nowMin = nowMinute;
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

    // The desktop top row is hero + however many stat tiles have real data.
    // Deriving the track list from that count keeps the row filled at every
    // width instead of leaving a hole where a missing tile would have sat.
    const statCount = $derived(
        (hasTemp ? 1 : 0) + (nextEvent ? 1 : 0) + (enabledAutomations > 0 ? 1 : 0),
    );
    const topCols = $derived(statCount > 0 ? `1.6fr ${"1fr ".repeat(statCount).trim()}` : "1fr");

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
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const onByRoom = new Map<string, number>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            onByRoom.set(r, (onByRoom.get(r) ?? 0) + (s.state ? 1 : 0));
        }
        return v.rooms.map(r => ({ ...r, on: onByRoom.get(r.name) ?? 0 }));
    });

    // ── Bulk actions ────────────────────────────────────────────────────────
    // No confirmation — the master switch is the app's flagship gesture, and a
    // dialog in front of it kills the moment. The action fires immediately and
    // offers Undo instead: every device returns to exactly its prior state.
    async function bulk(on: boolean) {
        haptic();
        const before = new Map(v.sockets.map(s => [s.id, s.state] as const));
        try {
            const r = on ? await api.allOn() : await api.allOff();
            await data.refresh();
            toasts.show({
                title: on ? "All on" : "All off",
                message: `${r.updated} updated${r.failures.length ? `, ${r.failures.length} failed` : ""}.`,
                tone: "success",
                timeoutMs: 8000,
                action: { label: "Undo", onClick: () => undoBulk(before) },
            });
        } catch (e) { toasts.error("Failed", (e as Error).message); }
    }
    async function undoBulk(before: ReadonlyMap<string, boolean>) {
        haptic();
        const restore = data.value.sockets.filter(s => before.has(s.id) && before.get(s.id) !== s.state);
        let failed = 0;
        await Promise.all(restore.map(async s => {
            try { await (before.get(s.id) ? api.socketOn(s.id) : api.socketOff(s.id)); }
            catch { failed++; }
        }));
        await data.refresh();
        if (failed) toasts.error("Undo", `${failed} device${failed === 1 ? "" : "s"} didn't respond.`);
        else toasts.success("Undone", `${restore.length} device${restore.length === 1 ? "" : "s"} back to how they were.`);
    }
    function toggleAllMaster() {
        bulk(!heroOn);
    }
    // A group's switch means "everything in it", matching the Groups view.
    function toggleGroup(g: { id: string; name: string }, on: boolean) {
        runAction(() => api.groupAction(g.id, on ? "on" : "off"), `${g.name} ${on ? "on" : "off"}`);
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
            <button class="chip search-chip" onclick={() => route.go("sockets", { focus: "search" })} aria-label="Search devices">
                <Icon name="search" size={14} />
                Search devices…
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
{#if !v.loaded}
    <!-- First load: shimmer placeholders instead of blank sections (§10). -->
    <div class="top-grid" aria-hidden="true">
        <div class="hero tile skel-hero">
            <div class="skeleton skel-line lg"></div>
            <div class="skeleton skel-line sm"></div>
        </div>
    </div>
    <section class="home-section" aria-hidden="true">
        <div class="rooms">
            {#each Array.from({ length: 4 }) as _, i (i)}
                <div class="skeleton skel-room"></div>
            {/each}
        </div>
    </section>
{:else if totalSockets === 0}
    <!-- First run: no devices at all — point at the add-device flow. -->
    <EmptyState fill icon="socket" title="No devices yet"
        message="Add your first RF socket or smart light to start controlling your home.">
        {#if session.isAdmin}
            <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add device</button>
        {/if}
    </EmptyState>
{:else}
<div class="top-grid" style="--top-cols: {topCols}">
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
{/if}

<!-- ── Playing now (owns its own Sonos poll; hides itself when there are
     no speakers, so it costs nothing on a home without them) ───────── -->
{#if v.loaded}
    <NowPlaying />
{/if}

<!-- ── Favorites ──────────────────────────────────────────────────── -->
{#if favoriteSockets.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="star" size={15} /></span>Favorites</h2>
            <span class="header-meta mono">{favoriteSockets.length}</span>
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

<!-- ── Rooms ──────────────────────────────────────────────────────── -->
{#if v.loaded}
<section class="home-section mobile-rooms">
    <div class="section-head">
        <h2><span class="section-ico"><Icon name="home" size={15} /></span>Rooms</h2>
        {#if liveRooms.length > 0}<span class="header-meta mono">{liveRooms.length}</span>{/if}
    </div>
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
{/if}

<!-- ── Groups ─────────────────────────────────────────────────────── -->
{#if groupsWithState.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="groups" size={15} /></span>Groups</h2>
            <button class="chip" onclick={() => route.go("groups")}>All</button>
        </div>
        <div class="groups">
            {#each groupsWithState as g, i (g.id)}
                {@const anyOn = g.on > 0}
                <div class="tile group-tile" class:on={anyOn}
                    animate:flip={{ duration: dur(280), easing: cubicOut }}
                    in:scale={{ start: 0.95, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <div class="gt-top">
                        <span class="gt-ico" class:on={anyOn}><Icon name="groups" size={17} /></span>
                        <Switch checked={anyOn} onChange={(c) => toggleGroup(g, c)}
                            ariaLabel="Toggle {g.name}" />
                    </div>
                    <button class="gt-body" onclick={() => route.go("groups")} aria-label="Open {g.name}">
                        <span class="gt-name">{g.name}</span>
                        <span class="gt-meta">
                            <span class="count" class:lit={anyOn}>{g.on}</span><span class="slash"> / {g.socket_ids.length}</span> on
                        </span>
                    </button>
                </div>
            {/each}
        </div>
    </section>
{/if}

<!-- ── Sensors ────────────────────────────────────────────────────── -->
{#if v.sensors.length > 0}
    <section class="home-section">
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="sensor" size={15} /></span>Sensors</h2>
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
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="timer" size={15} /></span>Pending timers</h2>
            <span class="header-meta mono">{v.timers.length}</span>
        </div>
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

<!-- ── Desktop: all devices with room filter ─────────────────────── -->
{#if v.sockets.length > 0}
    <section class="home-section desktop-devices">
        <div class="section-head">
            <h2><span class="section-ico"><Icon name="devices" size={15} /></span>Devices</h2>
            <div class="device-chips">
                <button class="chip" class:active={deviceRoom === ''} onclick={() => deviceRoom = ''}>All</button>
                <button class="chip" class:active={deviceRoom === 'on'} onclick={() => deviceRoom = 'on'}>On</button>
                {#each allDeviceRooms as r (r)}
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
    /* ── First-load skeletons ───────────────────────── */
    .skel-hero {
        min-height: 120px;
        justify-content: center;
        gap: 10px;
    }
    .skel-line { height: 14px; border-radius: var(--r-sm); width: 60%; }
    .skel-line.lg { height: 26px; width: 40%; }
    .skel-line.sm { width: 25%; }
    .skel-room { height: 72px; border-radius: var(--r-lg); }

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
    /* Hero spans full width on phones with the stat tiles as a compact strip
       beneath it; on desktop they share one row, matching the mockup. */
    .top-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--space-3); }
    .hero { grid-column: 1 / -1; }
    .stat {
        padding: 12px 14px;
        flex-direction: column;
        justify-content: space-between;
        gap: 4px;
        min-width: 0;
    }
    @media (min-width: 1024px) {
        /* One track per tile that actually rendered (--top-cols), so the row
           always spans the full width whatever the home has to report. */
        .top-grid { grid-template-columns: var(--top-cols); align-items: stretch; gap: var(--space-4); }
        .hero { grid-column: auto; }
        .stat { padding: 18px; gap: 8px; }
    }
    .stat-eyebrow {
        color: var(--text-mute);
        font-size: 10px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .stat-value { font-size: 20px; font-weight: 600; letter-spacing: -0.02em; font-family: var(--font-mono); font-feature-settings: "tnum" 1; }
    @media (min-width: 1024px) {
        .stat-eyebrow { font-size: 11px; }
        .stat-value { font-size: 32px; }
    }
    .stat-value.cool { color: var(--cool); }
    .stat-sub { color: var(--text-mute); font-size: 11px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    @media (min-width: 1024px) {
        .stat-sub { font-size: 12px; }
    }

    /* ── Whole-home hero ────────────────────────────── */
    .hero { padding: 20px; gap: 16px; transition: box-shadow var(--t-med); }
    /* When the home is lit the hero carries the room's glow with it — the
       "lit from within" beat, not just a warmer card. */
    .hero.tile.on { box-shadow: 0 12px 48px -14px var(--on-glow); }
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
        gap: 8px;
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
        color: var(--text-mute);
        background: var(--card-2);
        border: 1px solid var(--hairline);
        padding: 2px 9px;
        border-radius: var(--r-pill);
    }

    /* ── Favorites grid ─────────────────────────────── */
    .favorites {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .favorites { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-3); }
    }
    .favorite-item { display: flex; min-width: 0; }
    .favorite-item > :global(.tile) { flex: 1; min-width: 0; }

    /* ── Groups ─────────────────────────────────────── */
    /* Tiles, not flat rows: a group is an on/off surface like everything else
       on this page, so it gets the sanctioned .tile.on treatment and the same
       switch the Groups view uses. */
    .groups {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .groups { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-3); }
    }
    .group-tile { min-width: 0; }
    .gt-top { display: flex; justify-content: space-between; align-items: flex-start; }
    .gt-ico {
        width: 36px; height: 36px;
        border-radius: 10px;
        background: var(--card-3);
        color: var(--text-mute);
        display: grid; place-items: center;
        flex-shrink: 0;
        transition: background var(--t-med), color var(--t-med);
    }
    .gt-ico.on { background: var(--on); color: var(--primary-fg); }
    .gt-body {
        all: unset;
        cursor: pointer;
        touch-action: manipulation;
        display: flex;
        flex-direction: column;
        gap: 3px;
        min-width: 0;
    }
    .gt-body:focus-visible { box-shadow: var(--focus-ring); border-radius: var(--r-sm); }
    .gt-name {
        font-weight: 600; font-size: 15px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .gt-meta {
        color: var(--text-mute); font-size: 12.5px;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .count { font-family: var(--font-mono); font-feature-settings: "tnum" 1; color: var(--text-mute); }
    .count.lit { color: var(--on); }
    .slash { color: var(--text-dim); }

    /* ── Sensors ────────────────────────────────────── */
    .sensors {
        display: grid;
        grid-template-columns: repeat(2, minmax(0, 1fr));
        gap: 10px;
    }
    @media (min-width: 600px) {
        .sensors { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: var(--space-3); }
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
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: var(--space-3);
        }
    }
    /* A lone card never leaves half a row of nothing beside it. */
    .rooms > :only-child, .favorites > :only-child,
    .groups > :only-child, .sensors > :only-child { grid-column: 1 / -1; }
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
