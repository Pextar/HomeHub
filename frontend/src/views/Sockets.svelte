<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import SocketCard from "../components/SocketCard.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import Segmented from "../components/Segmented.svelte";
    import { api } from "../lib/api";
    import { data, route } from "../lib/stores.svelte";
    import { groupSocketsByRoom, runAction } from "../lib/utils";
    import { openModal } from "../lib/modal.svelte";
    import SocketModal from "../modals/SocketModal.svelte";
    import { scale, fade } from "svelte/transition";
    import { flip } from "svelte/animate";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    let search = $state("");
    // Initialised from a ?room= deep link (e.g. tapping a room card on Home).
    let roomFilter = $state(route.query.room ?? "");
    let statusFilter = $state("all");

    // ?focus=search (the dashboard's search chip) drops the caret straight
    // into the field instead of just landing on the view.
    let searchEl = $state<HTMLInputElement>();
    $effect(() => {
        if (route.query.focus === "search") searchEl?.focus();
    });

    const allRooms = $derived(
        [...new Set(v.sockets.map(s => s.room || "Unassigned"))].sort()
    );
    const roomCounts = $derived.by(() => {
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient local Map, built and consumed synchronously
        const m = new Map<string, number>();
        for (const s of v.sockets) {
            const r = s.room || "Unassigned";
            m.set(r, (m.get(r) ?? 0) + 1);
        }
        return m;
    });

    const filtered = $derived.by(() => {
        let list = v.sockets;
        if (search) {
            const q = search.toLowerCase();
            list = list.filter(s =>
                s.name.toLowerCase().includes(q) ||
                (s.room || "").toLowerCase().includes(q) ||
                s.code.toLowerCase().includes(q)
            );
        }
        if (roomFilter) {
            list = list.filter(s => (s.room || "Unassigned") === roomFilter);
        }
        if (statusFilter === "on")  list = list.filter(s => s.state);
        if (statusFilter === "off") list = list.filter(s => !s.state);
        return list;
    });

    const groups = $derived(groupSocketsByRoom(filtered));
    const sortedRooms = $derived([...groups.keys()].sort((a, b) => a.localeCompare(b)));
</script>

<Topbar title="Devices" subtitle="{v.sockets.length} configured · RF, Wi-Fi &amp; Matter">
    {#snippet actions()}
        <button class="chip" onclick={() => openModal(SocketModal, {})}><Icon name="plus" size={14} /> Add</button>
    {/snippet}
</Topbar>

{#if v.sockets.length === 0}
    <EmptyState fill icon="socket" title="No devices yet" message="Add your first device — RF, Tasmota, or Matter.">
        <button class="btn btn-primary" onclick={() => openModal(SocketModal, {})}>Add device</button>
    </EmptyState>
{:else}
    <div class="toolbar">
        <div class="search">
            <Icon name="search" size={16} />
            <input type="search" placeholder="Search devices…" bind:value={search} bind:this={searchEl} aria-label="Search devices" />
        </div>
        <select bind:value={roomFilter} aria-label="Filter by room" class="room-filter">
            <option value="">All rooms</option>
            {#each allRooms as r (r)}
                <option value={r}>{r}</option>
            {/each}
        </select>
        <!-- Rooms are chip filters on touch (§2), not a form select. -->
        <div class="room-chips h-scroll" role="group" aria-label="Filter by room">
            <button class="chip" class:active={roomFilter === ""} onclick={() => roomFilter = ""}>
                All rooms <span class="mono chip-n">{v.sockets.length}</span>
            </button>
            {#each allRooms as r (r)}
                <button class="chip" class:active={roomFilter === r} onclick={() => roomFilter = r}>
                    {r} <span class="mono chip-n">{roomCounts.get(r) ?? 0}</span>
                </button>
            {/each}
        </div>
        <Segmented
            name="socket-status"
            bind:value={statusFilter}
            options={[
                { value: "all", label: "All" },
                { value: "on",  label: "On" },
                { value: "off", label: "Off" },
            ]}
        />
    </div>

    {#if filtered.length === 0}
        <EmptyState compact title="No matches" message="Try a different search or clear the filters." />
    {:else}
        {#each sortedRooms as room (room)}
            {@const items = [...(groups.get(room) ?? [])].sort((a, b) =>
                a.state === b.state ? a.name.localeCompare(b.name) : a.state ? -1 : 1)}
            {@const onCount = items.filter(s => s.state).length}
            <section class="room-section"
                animate:flip={{ duration: dur(280), easing: cubicOut }}
                in:fade={{ duration: dur(180) }}>
                <div class="room-header">
                    <h3>
                        <span class="room-title">{room}</span>
                        <span class="room-meta">
                            <span class="mono on-count" class:lit={onCount > 0}>{onCount}</span><span class="slash"> / </span><span class="mono total">{items.length}</span> on
                        </span>
                    </h3>
                    <div class="room-actions">
                        <button class="btn btn-ghost"
                            onclick={() => runAction(() => api.roomOn(room), `${room} on`)}>All on</button>
                        <button class="btn btn-ghost"
                            onclick={() => runAction(() => api.roomOff(room), `${room} off`)}>All off</button>
                    </div>
                </div>
                <div class="grid">
                    {#each items as s, i (s.id)}
                        <div class="grid-item"
                            animate:flip={{ duration: dur(280), easing: cubicOut }}
                            in:scale={{ start: 0.96, opacity: 0, duration: dur(240), delay: stagger(i), easing: cubicOut }}>
                            <SocketCard socket={s} />
                        </div>
                    {/each}
                </div>
            </section>
        {/each}
    {/if}
{/if}

<style>
    .toolbar {
        display: flex;
        gap: var(--space-3);
        flex-wrap: wrap;
        padding: var(--space-3);
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--radius-lg);
        align-items: center;
    }
    .search {
        flex: 1;
        min-width: 220px;
        display: flex;
        align-items: center;
        gap: var(--space-2);
        padding: 0 var(--space-3);
        background: var(--card-2);
        border: 1px solid var(--hairline);
        border-radius: var(--r-pill);
        color: var(--text-muted);
    }
    .search input {
        flex: 1;
        border: 0;
        background: transparent;
        padding: 9px 0;
        color: var(--text);
    }
    .search input::placeholder { color: var(--text-faint); }
    .room-filter { max-width: 200px; }
    .room-chips { display: none; }
    .chip-n { font-size: 11px; opacity: 0.7; }

    @media (max-width: 600px) {
        .toolbar { flex-direction: column; align-items: stretch; gap: var(--space-2); }
        .search { min-width: 0; }
        .room-filter { display: none; }
        .room-chips { display: flex; margin: 0 calc(-1 * var(--space-3)); padding: 0 var(--space-3); }
    }
    /* Touch: ensure search row is 44px tall */
    @media (pointer: coarse) {
        .search input { padding: 12px 0; font-size: 16px; }
    }

    .room-section { display: flex; flex-direction: column; gap: var(--space-3); }
    .room-section + .room-section { margin-top: var(--space-6); }
    .room-header {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3);
    }
    .room-header h3 {
        display: flex;
        align-items: baseline;
        gap: var(--space-3);
        color: var(--text);
        font-size: 17px;
        font-weight: 600;
        letter-spacing: -0.02em;
    }
    .room-header .room-meta {
        font-size: 12.5px;
        font-weight: 500;
        color: var(--text-mute);
        letter-spacing: 0;
    }
    .room-header .on-count { color: var(--text-mute); }
    .room-header .on-count.lit { color: var(--on); }
    .room-header .slash, .room-header .total { color: var(--text-dim); }
    .room-actions { display: flex; gap: var(--space-2); }
    @media (pointer: coarse) {
        .room-header { padding: var(--space-1) 0; }
        .room-actions :global(.btn) { min-height: 40px; }
    }
    .grid {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
        gap: var(--space-3);
    }
    /* Compact device tiles flow two-up on phones, matching the mockup. */
    @media (max-width: 600px) {
        .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
    }
    .grid-item { display: flex; min-width: 0; }
    .grid-item > :global(.tile) { flex: 1; min-width: 0; }
</style>
